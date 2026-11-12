package decision_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"auth-service/internal/model"
	"auth-service/internal/service/decision"
	"auth-service/internal/service/decision/mocks"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"
	"auth-service/pkg/audit/testaudit"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	users     *mocks.MockidentityStore
	relations *mocks.MockrelationChecker
	fields    *mocks.MockfieldPolicy
	events    *mocks.Mockauditor
	req       model.CheckDecisionRequest
	caller    decision.Caller
}

func newFixture(t *testing.T) (*fixture, *decision.Service) {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &fixture{users: mocks.NewMockidentityStore(ctrl), relations: mocks.NewMockrelationChecker(ctrl), fields: mocks.NewMockfieldPolicy(ctrl), events: mocks.NewMockauditor(ctrl), caller: decision.Caller{ClientID: "bot", TelegramID: "42"}}
	f.req = model.CheckDecisionRequest{RequestID: uuid.New(), Subject: model.DecisionSubject{ID: uuid.New(), Type: model.ResourceTypeUser}, Action: model.DecisionAction{Name: model.ActionNameUpdate, Attributes: model.DecisionActionAttributes{FieldMask: []string{"text", "title"}}}, Resource: model.DecisionResource{ID: uuid.New(), Type: model.ResourceTypeNote}, Options: model.DecisionOptions{IncludeReasons: true}}
	f.relations.EXPECT().ModelID().Return("pinned-model").AnyTimes()
	f.fields.EXPECT().Version().Return("casbin:sha256:test").AnyTimes()
	svc, err := decision.New(f.users, f.relations, f.fields, f.events)
	require.NoError(t, err)

	return f, svc
}

func (f *fixture) identity() {
	f.users.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(f.req.Subject.ID, nil)
}
func (f *fixture) primary(allowed bool, err error) *gomock.Call {
	return f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Subject.ID.String(), "can_update", "note:"+f.req.Resource.ID.String(), model.HigherConsistency).Return(allowed, err)
}

//nolint:funlen // Таблица проверяет независимые отказы и отсутствие последующих внешних вызовов.
func TestCheckStages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		expect  func(*fixture)
		wantErr error
		allow   bool
		code    string
	}{
		{name: "allow", allow: true, code: "allowed_via_fga_relation", expect: func(f *fixture) {
			f.identity()
			first := f.primary(true, nil)
			f.fields.EXPECT().CheckFields("note", "update", []string{"text", "title"}).After(first).Return([]string{}, nil)
		}},
		{name: "relation deny", code: "denied_by_fga_relation", expect: func(f *fixture) { f.identity(); f.primary(false, nil) }},
		{name: "whole mask denied", code: "forbidden_fields", expect: func(f *fixture) {
			f.identity()
			f.primary(true, nil)
			f.fields.EXPECT().CheckFields("note", "update", gomock.Any()).Return([]string{"created_at", "timestamp"}, nil)
		}},
		{name: "invalid request", wantErr: model.ErrInvalidDecisionRequest, expect: func(f *fixture) { f.req.RequestID = uuid.Nil }},
		{name: "missing client", wantErr: decision.ErrUnauthorized, expect: func(f *fixture) { f.caller.ClientID = "" }},
		{name: "invalid header", wantErr: model.ErrInvalidDecisionRequest, expect: func(f *fixture) { f.caller.TelegramID = "-42" }},
		{name: "telegram mismatch", wantErr: decision.ErrSubjectMismatch, expect: func(f *fixture) { value := "43"; f.req.Subject.Attributes.TelegramUserID = &value }},
		{name: "subject mismatch", wantErr: decision.ErrSubjectMismatch, expect: func(f *fixture) { f.users.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(uuid.New(), nil) }},
		{name: "unregistered", wantErr: decision.ErrUserNotFound, expect: func(f *fixture) {
			f.users.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(uuid.Nil, storage.ErrNotFound)
		}},
		{name: "nil identity", wantErr: decision.ErrUserNotFound, expect: func(f *fixture) { f.users.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(uuid.Nil, nil) }},
		{name: "database unavailable", wantErr: decision.ErrIdentityUnavailable, expect: func(f *fixture) {
			f.users.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(uuid.Nil, errors.New("db secret must stay internal"))
		}},
		{name: "fga unavailable", wantErr: decision.ErrOpenFGAUnavailable, expect: func(f *fixture) { f.identity(); f.primary(false, context.DeadlineExceeded) }},
		{name: "policy unavailable", wantErr: decision.ErrPolicyUnavailable, expect: func(f *fixture) {
			f.identity()
			f.primary(true, nil)
			f.fields.EXPECT().CheckFields("note", "update", gomock.Any()).Return(nil, errors.New("engine failed"))
		}},
		{name: "space create", allow: true, code: "allowed_for_registered_user", expect: func(f *fixture) {
			f.req.Action = model.DecisionAction{Name: model.ActionNameCreate}
			f.req.Resource.Type = model.ResourceTypeSpace
			f.identity()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, svc := newFixture(t)
			tt.expect(f)

			if tt.wantErr == nil {
				f.events.EXPECT().Create(gomock.Any()).Return(testaudit.NewAuditor(t).Create(t.Context()))
			}

			resp, err := svc.Check(t.Context(), f.caller, f.req)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Empty(t, resp)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.allow, resp.Allow)
			require.Equal(t, tt.code, resp.Reasons[0].Code)
			require.NotEqual(t, uuid.Nil, resp.DecisionID)
			require.Equal(t, f.req.RequestID, resp.RequestID)
			require.Equal(t, "user:"+f.req.Subject.ID.String(), resp.Meta.Subject)
			require.Equal(t, "pinned-model", resp.Meta.AuthorizationModelID)
			require.GreaterOrEqual(t, resp.Meta.LatencyMS, int64(0))

			if tt.name == "whole mask denied" {
				require.Equal(t, "User cannot modify fields: created_at, timestamp", resp.Reasons[0].Message)
			}
		})
	}
}

func TestMemberGuardAndConsistency(t *testing.T) {
	t.Parallel()

	for _, owner := range []bool{false, true} {
		for _, consistency := range []model.Consistency{model.HigherConsistency, model.MinimizeLatency} {
			t.Run(string(consistency)+"/"+map[bool]string{true: "owner", false: "member"}[owner], func(t *testing.T) {
				t.Parallel()
				f, svc := newFixture(t)
				space := uuid.New()
				role := "admin"
				f.req.Resource = model.DecisionResource{ID: f.req.Subject.ID, Type: model.ResourceTypeMember, Attributes: model.DecisionResourceAttributes{SpaceID: &space}}
				f.req.Action = model.DecisionAction{Name: model.ActionNameEdit, Attributes: model.DecisionActionAttributes{Role: &role}}
				f.req.Options.Consistency = &consistency
				f.identity()
				primary := f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Subject.ID.String(), "can_manage_members", "space:"+space.String(), consistency).Return(true, nil)
				f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Resource.ID.String(), "owner", "space:"+space.String(), consistency).After(primary).Return(owner, nil)
				f.events.EXPECT().Create(gomock.Any()).Return(testaudit.NewAuditor(t).Create(t.Context()))
				resp, err := svc.Check(t.Context(), f.caller, f.req)
				require.NoError(t, err)
				require.Equal(t, !owner, resp.Allow)
				require.Equal(t, "member:"+f.req.Resource.ID.String(), resp.Meta.Resource)
			})
		}
	}
}

func TestRepeatRecomputesAndAuditFailureDoesNotChangeDecision(t *testing.T) {
	t.Parallel()
	f, svc := newFixture(t)
	f.req.Options.IncludeReasons = false
	f.identity()
	f.identity()
	f.primary(true, nil)
	f.primary(false, nil)
	f.fields.EXPECT().CheckFields("note", "update", gomock.Any()).Return([]string{}, nil)

	publisher := &auditPublisher{err: errors.New("rabbit unavailable")}
	sender, err := audit.NewSender(audit.WithClient(publisher), audit.WithTopic("test"))
	require.NoError(t, err)

	events := audit.NewAuditor(audit.WithSender(sender))
	f.events.EXPECT().Create(gomock.Any()).DoAndReturn(events.Create).Times(2)
	first, err := svc.Check(t.Context(), f.caller, f.req)
	require.NoError(t, err)
	require.True(t, first.Allow)
	require.Empty(t, first.Reasons)
	require.NotNil(t, first.Reasons)
	second, err := svc.Check(t.Context(), f.caller, f.req)
	require.NoError(t, err)
	require.False(t, second.Allow)
	require.NotEqual(t, first.DecisionID, second.DecisionID)
	require.Len(t, publisher.data, 2)
	require.True(t, publisher.bounded)
	require.Contains(t, publisher.data[0], first.DecisionID.String())
	require.Contains(t, publisher.data[0], `"caller":"bot"`)
	require.Contains(t, publisher.data[0], `"checked_relation":"can_update"`)
	require.Contains(t, publisher.data[0], `"subject":"user:`+f.req.Subject.ID.String()+`"`)
}

type auditPublisher struct {
	err     error
	data    []string
	bounded bool
}

func (p *auditPublisher) Publish(ctx context.Context, _ string, data []byte) error {
	p.data = append(p.data, string(data))
	_, p.bounded = ctx.Deadline()

	return p.err
}

//nolint:funlen // Два случая сравнивают полный контракт события space:create и member.
func TestAuditUsesTrustedMetadata(t *testing.T) {
	t.Parallel()

	for _, kind := range []model.ResourceType{model.ResourceTypeSpace, model.ResourceTypeMember} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			f, svc := newFixture(t)
			timestamp := "2030-01-01T00:00:00Z"
			f.req.Context = model.DecisionContext{Client: "untrusted", Service: "untrusted", Time: &timestamp}
			f.req.Resource.Type = kind
			f.req.Action = model.DecisionAction{Name: model.ActionNameCreate}
			object := ""
			relation := ""

			if kind == model.ResourceTypeMember {
				space := uuid.New()
				role := "admin"
				f.req.Resource.Attributes.SpaceID = &space
				f.req.Action = model.DecisionAction{Name: model.ActionNameEdit, Attributes: model.DecisionActionAttributes{Role: &role}}
				object = "space:" + space.String()
				relation = "can_manage_members"
				f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Subject.ID.String(), relation, object, model.HigherConsistency).Return(true, nil)
				f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Resource.ID.String(), "owner", object, model.HigherConsistency).Return(false, nil)
			}

			f.identity()

			publisher := &auditPublisher{}
			sender, err := audit.NewSender(audit.WithClient(publisher), audit.WithTopic("test"))
			require.NoError(t, err)

			events := audit.NewAuditor(audit.WithSender(sender))
			f.events.EXPECT().Create(gomock.Any()).DoAndReturn(events.Create)
			resp, err := svc.Check(t.Context(), f.caller, f.req)
			require.NoError(t, err)
			require.True(t, resp.Allow)
			require.Len(t, publisher.data, 1)

			var event map[string]any
			require.NoError(t, json.Unmarshal([]byte(publisher.data[0]), &event))
			fields, ok := event["context"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "bot", fields["caller"])
			require.Equal(t, resp.Meta.Subject, fields["subject"])
			require.Equal(t, resp.Meta.Resource, fields["resource"])
			require.Equal(t, resp.Meta.PolicyVersion, fields["policy_version"])
			require.Equal(t, resp.Meta.AuthorizationModelID, fields["authorization_model_id"])
			require.Equal(t, f.req.RequestID.String(), event["request_id"])
			require.NotEqual(t, timestamp, fields["timestamp"])
			require.NotContains(t, publisher.data[0], "untrusted")

			if kind == model.ResourceTypeSpace {
				require.Nil(t, fields["checked_object"])
				require.Nil(t, fields["checked_relation"])

				return
			}

			require.Equal(t, object, fields["checked_object"])
			require.Equal(t, relation, fields["checked_relation"])
			require.Equal(t, f.req.Resource.ID.String(), fields["target_user_id"])
			require.Equal(t, "admin", fields["role"])
		})
	}
}

func TestMemberOwnerCheckFailure(t *testing.T) {
	t.Parallel()
	f, svc := newFixture(t)
	space := uuid.New()
	f.req.Resource = model.DecisionResource{Type: model.ResourceTypeMember, ID: uuid.New(), Attributes: model.DecisionResourceAttributes{SpaceID: &space}}
	f.req.Action = model.DecisionAction{Name: model.ActionNameRemove}
	f.identity()
	f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Subject.ID.String(), "can_manage_members", "space:"+space.String(), model.HigherConsistency).Return(true, nil)
	f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Resource.ID.String(), "owner", "space:"+space.String(), model.HigherConsistency).Return(false, context.DeadlineExceeded)
	resp, err := svc.Check(t.Context(), f.caller, f.req)
	require.ErrorIs(t, err, decision.ErrOpenFGAUnavailable)
	require.Empty(t, resp)
}

func TestUninitializedDecisionServiceIsRejected(t *testing.T) {
	t.Parallel()

	svc, err := decision.New(nil, nil, nil, nil)
	require.Error(t, err)
	require.Nil(t, svc)
	ctrl := gomock.NewController(t)
	users := mocks.NewMockidentityStore(ctrl)
	relations := mocks.NewMockrelationChecker(ctrl)
	fields := mocks.NewMockfieldPolicy(ctrl)

	relations.EXPECT().ModelID().Return("")
	svc, err = decision.New(users, relations, fields, testaudit.NewAuditor(t))
	require.Error(t, err)
	require.Nil(t, svc)
}
