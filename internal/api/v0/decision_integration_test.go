package v0

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fgamodel "auth-service/fga"
	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"auth-service/internal/service/decision"
	"auth-service/internal/service/enforcer"
	"auth-service/internal/service/fga"
	"auth-service/internal/testfga"
	"auth-service/pkg/audit"
	"auth-service/pkg/audit/testaudit"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
)

type decisionIdentity struct{ id uuid.UUID }

func (s decisionIdentity) GetUserIDByTelegramID(context.Context, string) (uuid.UUID, error) {
	return s.id, nil
}

func integrationClient(t *testing.T, api *sdk.OpenFgaClient, actor uuid.UUID) *fga.Client {
	t.Helper()

	store, err := api.GetStoreId()
	require.NoError(t, err)
	modelID, err := api.GetAuthorizationModelId()
	require.NoError(t, err)
	client, err := fga.NewClient(fga.WithAPIURL(api.GetConfig().ApiUrl), fga.WithStoreID(store), fga.WithModelID(modelID), fga.WithAuthorizationModel("unused-pinned-model"), fga.WithUserRepo(decisionIdentity{actor}), fga.WithAuditor(testaudit.NewAuditor(t)))
	require.NoError(t, err)
	require.NoError(t, client.Connect(t.Context()))
	require.NoError(t, client.EnsureDecisionModel(t.Context()))

	return client
}

func syncResource(t *testing.T, client *fga.Client, resource model.Resource, owner uuid.UUID, parent model.Resource) {
	t.Helper()
	resp, err := client.UpdateResource(t.Context(), model.UpdateResourceRequest{RequestID: uuid.New(), DecisionID: uuid.New(), IdempotencyKey: uuid.NewString(), TelegramID: 42, Resource: resource, Relations: model.Relation{Owner: model.Resource{Type: model.ResourceTypeUser, ID: owner}, Parent: parent}, Operation: model.OperationCreate, ChangeType: model.ChangeTypeResourceAdded, Context: model.Context{EventType: model.FormatEventType(strings.ToUpper(string(resource.Type)), model.EventTypeOperationCreatedPostfix)}})
	require.NoError(t, err)
	require.Equal(t, client.ModelID(), resp.Meta.AuthModelID)

	if resource.Type == model.ResourceTypeSpace {
		require.Len(t, resp.WrittenTuples, 2)
	}
}

//nolint:funlen // Сквозные сценарии связывают одну подготовленную модель, sync, Casbin, middleware и HTTP.
func TestDecisionHTTPWithOpenFGAAndCasbin(t *testing.T) {
	t.Parallel()
	api := testfga.New(t, fgamodel.Strict)
	owner, foreign, editor, admin := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	personal := model.Resource{Type: model.ResourceTypeSpace, ID: uuid.New(), Attributes: map[string]any{"space_type": "personal"}}
	shared := model.Resource{Type: model.ResourceTypeSpace, ID: uuid.New(), Attributes: map[string]any{"space_type": "shared"}}
	note := model.Resource{Type: model.ResourceTypeNote, ID: uuid.New()}
	reminder := model.Resource{Type: model.ResourceTypeReminder, ID: uuid.New()}
	writer := integrationClient(t, api, owner)
	syncResource(t, writer, personal, owner, model.Resource{})
	syncResource(t, writer, shared, owner, model.Resource{})
	syncResource(t, writer, note, foreign, personal)
	syncResource(t, writer, reminder, foreign, shared)
	testfga.Write(t, api, sdk.ClientTupleKey{User: "user:" + editor.String(), Relation: "editor", Object: "space:" + shared.ID.String()}, sdk.ClientTupleKey{User: "user:" + admin.String(), Relation: "admin", Object: "space:" + shared.ID.String()})

	role := "admin"

	tests := []struct {
		name   string
		actor  uuid.UUID
		kind   model.ResourceType
		id     uuid.UUID
		action model.ActionName
		fields []string
		parent *uuid.UUID
		role   *string
		allow  bool
		reason string
	}{
		{name: "personal cannot be deleted", actor: owner, kind: model.ResourceTypeSpace, id: personal.ID, action: model.ActionNameDelete, reason: "denied_by_fga_relation"},
		{name: "personal note can be deleted", actor: owner, kind: model.ResourceTypeNote, id: note.ID, action: model.ActionNameDelete, allow: true, reason: "allowed_via_fga_relation"},
		{name: "foreign direct owner denied", actor: foreign, kind: model.ResourceTypeNote, id: note.ID, action: model.ActionNameRead, reason: "denied_by_fga_relation"},
		{name: "personal note text and title", actor: owner, kind: model.ResourceTypeNote, id: note.ID, action: model.ActionNameUpdate, fields: []string{"text", "title"}, allow: true, reason: "allowed_via_fga_relation"},
		{name: "reminder title and date", actor: editor, kind: model.ResourceTypeReminder, id: reminder.ID, action: model.ActionNameUpdate, fields: []string{"remind_at", "title"}, allow: true, reason: "allowed_via_fga_relation"},
		{name: "whole update rejected", actor: editor, kind: model.ResourceTypeReminder, id: reminder.ID, action: model.ActionNameUpdate, fields: []string{"title", "timestamp"}, reason: "forbidden_fields"},
		{name: "owner membership protected", actor: admin, kind: model.ResourceTypeMember, id: owner, action: model.ActionNameEdit, parent: &shared.ID, role: &role, reason: "protected_space_owner"},
		{name: "admin self removal", actor: admin, kind: model.ResourceTypeMember, id: admin, action: model.ActionNameRemove, parent: &shared.ID, allow: true, reason: "allowed_via_fga_relation"},
		{name: "personal member denied", actor: owner, kind: model.ResourceTypeMember, id: foreign, action: model.ActionNameAdd, parent: &personal.ID, role: &role, reason: "denied_by_fga_relation"},
		{name: "existing parent cannot override privacy", actor: editor, kind: model.ResourceTypeNote, id: note.ID, action: model.ActionNameRead, parent: &shared.ID, reason: "denied_by_fga_relation"},
		{name: "shared record owner without membership", actor: foreign, kind: model.ResourceTypeReminder, id: reminder.ID, action: model.ActionNameStop, allow: true, reason: "allowed_via_fga_relation"},
		{name: "registered user creates space", actor: foreign, kind: model.ResourceTypeSpace, id: uuid.New(), action: model.ActionNameCreate, allow: true, reason: "allowed_for_registered_user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := integrationClient(t, api, tt.actor)
			policy, err := enforcer.NewFieldPolicy()
			require.NoError(t, err)

			publication := &decisionPublication{}
			sender, err := audit.NewSender(audit.WithClient(publication), audit.WithTopic("decisions"))
			require.NoError(t, err)
			svc, err := decision.New(decisionIdentity{tt.actor}, client, policy, audit.NewAuditor(audit.WithSender(sender)))
			require.NoError(t, err)
			handler, err := NewResourceHandler(WithResourceService(client), WithDecisionService(svc))
			require.NoError(t, err)
			auth := mocks.NewMockAuthService(gomock.NewController(t))
			auth.EXPECT().CheckToken(gomock.Any(), "Bearer test").Return(&jwt.Token{}, nil)
			auth.EXPECT().GetPayload(gomock.Any()).Return(jwt.MapClaims{"sub": "bot-client", "scope": "bot read"}, true)
			auth.EXPECT().GetServiceClient(gomock.Any(), "bot-client").Return(model.ServiceClient{IsActive: true, Scopes: []string{"read", "bot"}}, nil)
			middleware, err := NewMiddlewareHandler(WithMiddlewareAuthService(auth))
			require.NoError(t, err)

			req := model.CheckDecisionRequest{RequestID: uuid.New(), Subject: model.DecisionSubject{Type: model.ResourceTypeUser, ID: tt.actor}, Action: model.DecisionAction{Name: tt.action, Attributes: model.DecisionActionAttributes{FieldMask: tt.fields, Role: tt.role}}, Resource: model.DecisionResource{Type: tt.kind, ID: tt.id, Attributes: model.DecisionResourceAttributes{SpaceID: tt.parent}}, Options: model.DecisionOptions{IncludeReasons: true}}
			body, err := json.Marshal(req)
			require.NoError(t, err)

			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v0/decisions/check", strings.NewReader(string(body)))
			request.Header.Set("Authorization", "Bearer test")
			request.Header.Set(userIDHeader, "42")

			rec := httptest.NewRecorder()
			e := echo.New()
			e.POST("/v0/decisions/check", handler.CheckDecision, middleware.CheckDecisionToken)
			e.ServeHTTP(rec, request)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			var result model.CheckDecisionResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			require.Equal(t, tt.allow, result.Allow)
			require.Equal(t, tt.reason, result.Reasons[0].Code)
			require.Equal(t, string(tt.kind)+":"+tt.id.String(), result.Meta.Resource)
			require.Equal(t, client.ModelID(), result.Meta.AuthorizationModelID)
			require.Contains(t, publication.data, result.DecisionID.String())
			require.Contains(t, publication.data, result.Meta.Subject)
			require.Contains(t, publication.data, `"caller":"bot-client"`)
		})
	}
}

type decisionPublication struct{ data string }

func (p *decisionPublication) Publish(_ context.Context, _ string, data []byte) error {
	p.data = string(data)
	return nil
}
