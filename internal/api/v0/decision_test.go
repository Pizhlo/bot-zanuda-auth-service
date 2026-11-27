package v0

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"auth-service/internal/service/decision"

	"auth-service/pkg/audit/testaudit"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func validationRequest() model.CheckDecisionRequest {
	return model.CheckDecisionRequest{
		RequestID: uuid.New(),
		Subject:   model.DecisionSubject{Type: model.ResourceTypeUser, ID: uuid.New()},
		Action:    model.DecisionAction{Name: model.ActionNameDelete},
		Resource:  model.DecisionResource{Type: model.ResourceTypeSpace, ID: uuid.New()},
	}
}

func TestCheckDecision(t *testing.T) {
	t.Parallel()

	input := validationRequest()
	raw, err := json.Marshal(input)
	require.NoError(t, err)

	respond := func(serviceErr error) func(*mocks.MockdecisionService) {
		return func(svc *mocks.MockdecisionService) {
			caller := decision.Caller{ClientID: "client", TelegramID: "42"}
			svc.EXPECT().Check(gomock.Any(), caller, input).Return(model.CheckDecisionResponse{RequestID: input.RequestID}, serviceErr)
		}
	}
	noCall := func(*mocks.MockdecisionService) {}

	withAudit := (*MiddlewareHandler).AuditDecision
	for _, tc := range []struct {
		name   string
		audit  func(*MiddlewareHandler, echo.HandlerFunc) echo.HandlerFunc
		body   io.Reader
		expect func(*mocks.MockdecisionService)
		status int
		want   map[string]any
	}{
		{"missing audit rejects before reading", withoutRequestAudit, iotest.ErrReader(io.ErrUnexpectedEOF), noCall, http.StatusInternalServerError, map[string]any{"error_code": "internal_error"}},
		{"success", withAudit, bytes.NewReader(raw), respond(nil), http.StatusOK, map[string]any{"request_id": input.RequestID.String()}},
		{"body above one MiB", withAudit, io.MultiReader(strings.NewReader(strings.Repeat(" ", 1<<20)), bytes.NewReader(raw)), respond(nil), http.StatusOK, map[string]any{"request_id": input.RequestID.String()}},
		{"invalid JSON", withAudit, strings.NewReader("{"), noCall, http.StatusBadRequest, map[string]any{"message": "body contains incomplete JSON"}},
		{"read error", withAudit, iotest.ErrReader(errors.New("private transport error")), noCall, http.StatusBadRequest, map[string]any{"message": "body could not be read"}},
		{"ambiguous action", withAudit, strings.NewReader(strings.Replace(string(raw), `"name":"delete"`, `"name":"delete","name":"read"`, 1)), noCall, http.StatusBadRequest, map[string]any{"error_code": "invalid_request", "request_id": input.RequestID.String(), "message": "action.name must appear only once"}},
		{"noncanonical object", withAudit, strings.NewReader(strings.Replace(string(raw), `"subject":`, `"Subject":`, 1)), noCall, http.StatusBadRequest, map[string]any{"error_code": "invalid_request", "request_id": input.RequestID.String(), "message": "body contains an unknown field"}},
		{"empty target", withAudit, strings.NewReader(strings.Replace(string(raw), input.Resource.ID.String(), "", 1)), noCall, http.StatusBadRequest, map[string]any{"message": "resource.id must be a nonzero UUID"}},
		{"invalid service request", withAudit, bytes.NewReader(raw), respond(model.InvalidDecisionRequest("X-Telegram-User-Id header is required")), http.StatusBadRequest, map[string]any{"message": "X-Telegram-User-Id header is required"}},
		{"database unavailable", withAudit, bytes.NewReader(raw), respond(decision.ErrDecisionStoreUnavailable), http.StatusServiceUnavailable, map[string]any{"request_id": input.RequestID.String(), "error_code": "decision_store_unavailable"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := mocks.NewMockdecisionService(gomock.NewController(t))
			tc.expect(svc)
			h := ResourceHandler{decisionService: svc}

			ctx := context.WithValue(t.Context(), clientIDKey{}, "client")
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v0/decisions/check", tc.body)
			req.Header.Set(userIDHeader, "42")

			rec := httptest.NewRecorder()

			err := tc.audit(testAuditMiddleware(t), h.CheckDecision)(echo.New().NewContext(req, rec))
			require.NoError(t, err)

			require.Equal(t, tc.status, rec.Code)

			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

			require.Subset(t, body, tc.want)
		})
	}
}

func TestWithDecisionService(t *testing.T) {
	t.Parallel()

	svc := mocks.NewMockdecisionService(gomock.NewController(t))

	var h ResourceHandler

	WithDecisionService(svc)(&h)

	require.Same(t, svc, h.decisionService)
}

func TestDecisionError(t *testing.T) {
	t.Parallel()

	id, transport := uuid.New(), uuid.New()
	for _, tc := range []struct {
		name      string
		requestID uuid.UUID
		transport string
		cause     error
		status    int
		code      string
		want      func(*testing.T, uuid.UUID, map[string]string)
	}{
		{"parsed body takes priority", id, transport.String(), decision.ErrForbidden, http.StatusForbidden, "forbidden", func(t *testing.T, header uuid.UUID, body map[string]string) {
			t.Helper()
			require.Equal(t, id, header)
			require.Equal(t, id.String(), body["request_id"])
		}},
		{"unparsed body with transport correlation", uuid.Nil, transport.String(), decision.ErrUnauthorized, http.StatusUnauthorized, "unauthorized", func(t *testing.T, header uuid.UUID, body map[string]string) {
			t.Helper()
			require.Equal(t, transport, header)
			require.NotContains(t, body, "request_id")
		}},
		{"generated correlation", uuid.Nil, "", decision.ErrUnauthorized, http.StatusUnauthorized, "unauthorized", func(t *testing.T, header uuid.UUID, body map[string]string) {
			t.Helper()
			require.NotEqual(t, uuid.Nil, header)
			require.NotContains(t, body, "request_id")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil), rec)

			err := decisionError(withRequestID(t.Context(), tc.transport), c, tc.cause, tc.requestID)
			require.NoError(t, err)

			require.Equal(t, tc.status, rec.Code)
			correlation, err := uuid.Parse(rec.Header().Get(echo.HeaderXRequestID))
			require.NoError(t, err)

			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

			require.Equal(t, tc.code, body["error_code"])
			tc.want(t, correlation, body)
		})
	}
}

func TestWriteDecisionError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil), rec)
	id := uuid.New()
	require.NoError(t, writeDecisionError(c, decision.ErrForbidden, uuid.Nil, id))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, id.String(), rec.Header().Get(echo.HeaderXRequestID))
	require.NotContains(t, rec.Body.String(), "request_id")
}

func TestDecisionHTTPErrorHandler(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	recovered := errors.New("recovered panic")
	before := func(t *testing.T, c echo.Context) {
		t.Helper()
		recordTestDecisionPanic(t, c, id)
	}

	noBody := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		require.Empty(t, rec.Body.String())
	}
	for _, tc := range []struct {
		name        string
		err         error
		prepare     func(*testing.T, echo.Context)
		status      int
		correlation string
		wantBody    func(*testing.T, *httptest.ResponseRecorder)
	}{
		{"ordinary HTTP error", echo.ErrNotFound, func(*testing.T, echo.Context) {}, http.StatusNotFound, "", func(t *testing.T, rec *httptest.ResponseRecorder) {
			t.Helper()
			require.JSONEq(t, `{"message":"Not Found"}`, rec.Body.String())
		}},
		{"recovered decision panic", recovered, before, http.StatusInternalServerError, id.String(), wantPanicJSON(id)},
		{"early panic with header", recovered, func(t *testing.T, c echo.Context) {
			t.Helper()
			c.Request().Header.Set(echo.HeaderXRequestID, id.String())
			recordEarlyDecisionPanic(t, c, id)
		}, http.StatusInternalServerError, id.String(), wantPanicJSON(id)},
		{"early panic without header", recovered, func(t *testing.T, c echo.Context) {
			t.Helper()
			recordEarlyDecisionPanic(t, c, id)
		}, http.StatusInternalServerError, id.String(), wantPanicJSON(uuid.Nil)},
		{"response already committed", recovered, func(t *testing.T, c echo.Context) {
			t.Helper()
			before(t, c)
			require.NoError(t, c.NoContent(http.StatusAccepted))
		}, http.StatusAccepted, "", noBody},
		{"response write failed", recovered, func(t *testing.T, c echo.Context) {
			t.Helper()
			before(t, c)
			c.Response().Writer = brokenDecisionWriter{c.Response().Writer}
		}, http.StatusInternalServerError, id.String(), noBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v0/decisions/check", nil), rec)
			tc.prepare(t, c)

			DecisionHTTPErrorHandler(tc.err, c)

			require.Equal(t, tc.status, rec.Code)
			require.Equal(t, tc.correlation, rec.Header().Get(echo.HeaderXRequestID))
			tc.wantBody(t, rec)
		})
	}
}

func recordTestDecisionPanic(t *testing.T, c echo.Context, id uuid.UUID) {
	t.Helper()
	require.Panics(t, func() {
		_ = decision.AuditRequest(t.Context(), testaudit.NewAuditor(t), decision.OperationCheckDecision, uuid.Nil, func(ctx context.Context) error {
			c.SetRequest(c.Request().WithContext(ctx))
			decision.AuditRejection(ctx, id, decision.ErrUnauthorized)
			panic("private failure")
		})
	})
}

func recordEarlyDecisionPanic(t *testing.T, c echo.Context, transportID uuid.UUID) {
	t.Helper()
	require.Panics(t, func() {
		_ = decision.AuditRequest(t.Context(), testaudit.NewAuditor(t), decision.OperationCheckDecision, transportID, func(ctx context.Context) error {
			c.SetRequest(c.Request().WithContext(ctx))
			panic("private failure")
		})
	})
}

func wantPanicJSON(requestID uuid.UUID) func(*testing.T, *httptest.ResponseRecorder) {
	return func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()

		want := map[string]string{"error_code": "internal_error", "message": "Internal server error"}
		if requestID != uuid.Nil {
			want["request_id"] = requestID.String()
		}

		var body map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

		require.Equal(t, want, body)
	}
}
