package v0

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"auth-service/internal/api/internal/response"
	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"auth-service/internal/service/decision"
	"auth-service/internal/service/fga"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestDecisionErrorsHTTP(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid", model.ErrInvalidDecisionRequest, 400, "invalid_request"},
		{"unauthorized", decision.ErrUnauthorized, 401, "unauthorized"},
		{"forbidden", decision.ErrForbidden, 403, "forbidden"},
		{"mismatch", decision.ErrSubjectMismatch, 403, "subject_mismatch"},
		{"unknown user", decision.ErrUserNotFound, 404, "user_not_found"},
		{"identity store", decision.ErrIdentityUnavailable, 503, "identity_store_unavailable"},
		{"openfga", decision.ErrOpenFGAUnavailable, 503, "openfga_unavailable"},
		{"policy", decision.ErrPolicyUnavailable, 503, "policy_unavailable"},
		{"unexpected", errors.New("secret internal detail"), 500, "internal_error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			id := uuid.New()
			subject := uuid.New()
			resource := uuid.New()
			req := model.CheckDecisionRequest{RequestID: id, Subject: model.DecisionSubject{Type: model.ResourceTypeUser, ID: subject}, Action: model.DecisionAction{Name: model.ActionNameRead}, Resource: model.DecisionResource{Type: model.ResourceTypeNote, ID: resource}}
			raw, err := json.Marshal(req)
			require.NoError(t, err)
			svc := mocks.NewMockdecisionService(gomock.NewController(t))
			svc.EXPECT().Check(gomock.Any(), decision.Caller{ClientID: "client", TelegramID: "42"}, req).Return(model.CheckDecisionResponse{}, tt.err)
			h := &ResourceHandler{decisionService: svc}
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v0/decisions/check", strings.NewReader(string(raw)))
			r.Header.Set(userIDHeader, "42")
			r = r.WithContext(context.WithValue(r.Context(), decisionClientKey{}, "client"))
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(r, rec)
			require.NoError(t, h.CheckDecision(c))
			require.Equal(t, tt.status, rec.Code)

			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Len(t, body, 3)
			require.Equal(t, tt.code, body["error_code"])
			require.Equal(t, id.String(), body["request_id"])
			require.NotEmpty(t, body["message"])
			require.NotContains(t, rec.Body.String(), "secret")
		})
	}
}

func TestMalformedDecisionNeverCallsService(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`{}`, `null`, `{"request_id":null}`, `{} {}`, strings.Repeat(" ", 1<<20+1)} {
		t.Run(body[:min(len(body), 20)], func(t *testing.T) {
			t.Parallel()
			svc := mocks.NewMockdecisionService(gomock.NewController(t))
			h := &ResourceHandler{decisionService: svc}
			rec := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v0/decisions/check", strings.NewReader(body))
			require.NoError(t, h.CheckDecision(echo.New().NewContext(r, rec)))
			require.Equal(t, 400, rec.Code)
			require.JSONEq(t, `{"error_code":"invalid_request","message":"Invalid decision request"}`, rec.Body.String())
		})
	}
}

func TestDecisionAuthentication(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		expect func(*mocks.MockAuthService)
		status int
	}{
		{"token invalid", func(a *mocks.MockAuthService) {
			a.EXPECT().CheckToken(gomock.Any(), "Bearer sample").Return(nil, errors.New("secret"))
		}, 401},
		{"no claims", func(a *mocks.MockAuthService) {
			a.EXPECT().CheckToken(gomock.Any(), "Bearer sample").Return(&jwt.Token{}, nil)
			a.EXPECT().GetPayload(gomock.Any()).Return(nil, false)
		}, 401},
		{"no subject", func(a *mocks.MockAuthService) { authClaims(a, jwt.MapClaims{"scope": "bot"}) }, 401},
		{"bad scope type", func(a *mocks.MockAuthService) {
			authClaims(a, jwt.MapClaims{"sub": "client", "scope": []string{"bot"}})
		}, 401},
		{"missing bot", func(a *mocks.MockAuthService) {
			authClient(a, "read", model.ServiceClient{IsActive: true, Scopes: []string{"read"}}, nil)
		}, 403},
		{"inactive", func(a *mocks.MockAuthService) { authClient(a, "bot", model.ServiceClient{}, nil) }, 403},
		{"unknown client", func(a *mocks.MockAuthService) { authClient(a, "bot", model.ServiceClient{}, storage.ErrNotFound) }, 403},
		{"store down", func(a *mocks.MockAuthService) { authClient(a, "bot", model.ServiceClient{}, context.DeadlineExceeded) }, 503},
		{"ungranted extra scope", func(a *mocks.MockAuthService) {
			authClient(a, "bot admin", model.ServiceClient{IsActive: true, Scopes: []string{"bot"}}, nil)
		}, 403},
		{"bot token", func(a *mocks.MockAuthService) {
			authClient(a, "bot", model.ServiceClient{IsActive: true, Scopes: []string{"bot"}}, nil)
		}, 204},
		{"multiple scopes", func(a *mocks.MockAuthService) {
			authClient(a, " read  bot ", model.ServiceClient{IsActive: true, Scopes: []string{"bot", "read"}}, nil)
		}, 204},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := mocks.NewMockAuthService(gomock.NewController(t))
			tt.expect(a)
			h, err := NewMiddlewareHandler(WithMiddlewareAuthService(a))
			require.NoError(t, err)

			invoked := false
			next := func(c echo.Context) error {
				invoked = true

				require.Equal(t, "client", decisionClient(c.Request().Context()))

				return c.NoContent(204)
			}
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v0/decisions/check", nil)
			r.Header.Set("Authorization", "Bearer sample")

			rec := httptest.NewRecorder()
			require.NoError(t, h.CheckDecisionToken(next)(echo.New().NewContext(r, rec)))
			require.Equal(t, tt.status, rec.Code)
			require.Equal(t, tt.status == 204, invoked)
			require.NotContains(t, rec.Body.String(), "secret")
		})
	}
}

func authClaims(a *mocks.MockAuthService, claims jwt.MapClaims) {
	a.EXPECT().CheckToken(gomock.Any(), "Bearer sample").Return(&jwt.Token{}, nil)
	a.EXPECT().GetPayload(gomock.Any()).Return(claims, true)
}
func authClient(a *mocks.MockAuthService, scopes string, client model.ServiceClient, err error) {
	authClaims(a, jwt.MapClaims{"sub": "client", "scope": scopes})
	a.EXPECT().GetServiceClient(gomock.Any(), "client").Return(client, err)
}

func TestSpacePrivacyErrorKeepsResourceEnvelope(t *testing.T) {
	t.Parallel()

	req := model.UpdateResourceRequest{RequestID: uuid.New(), Resource: model.Resource{Type: model.ResourceTypeSpace, ID: uuid.New()}, Operation: model.OperationDelete}
	data, err := json.Marshal(req)
	require.NoError(t, err)
	svc := mocks.NewMockresourceService(gomock.NewController(t))
	svc.EXPECT().UpdateResource(gomock.Any(), gomock.Any()).Return(model.UpdateResourceResponse{}, fga.ErrSpacePrivacy)
	handler, err := NewResourceHandler(WithResourceService(svc))
	require.NoError(t, err)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v0/resources/update", strings.NewReader(string(data)))
	request.Header.Set(userIDHeader, "42")
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	require.NoError(t, handler.UpdateResource(echo.New().NewContext(request, recorder)))
	require.Equal(t, 400, recorder.Code)

	var result response.ResourceBody
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.Equal(t, req.RequestID, result.RequestID)
	require.Equal(t, model.StatusError, result.Status)
	require.Equal(t, model.ResultFailed, result.Result)
	require.Equal(t, audit.ErrCodeValidationFailed, result.ErrorMessage.Code)
	require.Equal(t, model.OperationDelete, result.ErrorMessage.Details.Operation)
	require.NotContains(t, recorder.Body.String(), `"error_code"`)
}
