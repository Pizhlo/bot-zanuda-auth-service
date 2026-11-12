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
	"time"

	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"auth-service/internal/service/fga"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"
	"auth-service/pkg/audit/testaudit"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestConnectionHook(t *testing.T) {
	t.Parallel()

	full := withTraceID(t.Context(), "trace-1")
	full = withRequestID(full, "req-1")
	full = withIPAddress(full, "127.0.0.1")
	full = withUserAgent(full, "curl/8.0")
	full = withUserID(full, "42")

	service := func() audit.Stash { return audit.Stash{}.Append(audit.ServiceName("auth-service")) }
	for _, tc := range []struct {
		name     string
		ctx      context.Context
		in, want audit.Stash
	}{
		{"all connection fields", full, audit.Stash{}, service().Append(audit.TraceID("trace-1")).Append(audit.RequestID("req-1")).Append(
			audit.ContextField(audit.EventContext{"ip_address": "127.0.0.1", "user_agent": "curl/8.0", "user_id": "42"}, audit.Stash{}),
		)},
		{"no connection fields", t.Context(), audit.Stash{}, service()},
		{"partial request preserves existing context", withUserID(t.Context(), "42"),
			audit.Stash{}.Append(audit.ContextField(audit.EventContext{"source": "test"}, audit.Stash{})),
			service().Append(audit.ContextField(audit.EventContext{"source": "test", "user_id": "42"}, audit.Stash{}))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ConnectionHook(tc.ctx, tc.in)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseUserID(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"string", "123", "123"},
		{"empty string", "", ""},
		{"integer", 123, ""},
		{"missing value", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			id := parseUserID(tc.value)

			require.Equal(t, tc.want, id)
		})
	}
}

func TestWithUserID(t *testing.T) {
	t.Parallel()

	ctx := withUserID(t.Context(), "123")
	id, ok := userIDFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "123", id)
}

func checkRejectedRequestAudit(path, operation string, wrap func(*MiddlewareHandler, echo.HandlerFunc) echo.HandlerFunc) func(*testing.T, echo.MiddlewareFunc) {
	return func(t *testing.T, check echo.MiddlewareFunc) {
		t.Helper()

		var log bytes.Buffer

		sink := &captureDecisionAudit{}
		sender, err := audit.NewSender(audit.WithClient(sink), audit.WithTopic("test"))
		require.NoError(t, err)

		source := audit.NewAuditor(audit.WithOutput(&log), audit.WithSender(sender), audit.WithDeliveryTimeout(time.Second))

		mw := &MiddlewareHandler{requestAuditor: source}

		id := uuid.NewString()

		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path,
			strings.NewReader(`{"request_id":"`+id+`"}`))
		req.Header.Set("Authorization", "Bearer private")
		ctx, cancel := context.WithCancel(req.Context())
		cancel()

		c := echo.New().NewContext(req.WithContext(ctx), brokenDecisionWriter{httptest.NewRecorder()})
		h := wrap(mw, check(func(echo.Context) error { return io.ErrUnexpectedEOF }))
		require.ErrorContains(t, h(c), "private write error")
		require.Equal(t, http.StatusUnauthorized, c.Response().Status)
		require.Len(t, sink.events, 1)
		require.True(t, sink.liveWithDeadline)
		require.NotContains(t, log.String(), "private")

		var event map[string]any
		require.NoError(t, json.Unmarshal(sink.events[0], &event))
		require.Equal(t, id, event["request_id"])
		require.Equal(t, "failed", event["status"])
		require.Equal(t, "AUTH_TOKEN_INVALID", event["error_code"])
		require.Equal(t, operation, event["operation"])
		require.Equal(t, "warn", event["level"])
		require.Equal(t, audit.MessageTokenVerificationFailed, event["cause"])

		fields := event["context"].(map[string]any)
		require.Equal(t, "authenticate", fields["stage"])
		require.Equal(t, true, fields["response_write_failed"])
		require.Equal(t, "canceled", fields["context_error"])
	}
}

func TestWithRequestAuditor(t *testing.T) {
	t.Parallel()

	source := audit.NewAuditor()

	var h MiddlewareHandler

	WithRequestAuditor(source)(&h)

	require.Same(t, source, h.requestAuditor)
}

func TestNewMiddlewareHandler(t *testing.T) {
	t.Parallel()

	wantError := func(message string) require.ErrorAssertionFunc {
		return func(t require.TestingT, err error, _ ...any) { require.EqualError(t, err, message) }
	}
	wantNil := func(t *testing.T, h *MiddlewareHandler) { t.Helper(); require.Nil(t, h) }

	for _, tc := range []struct {
		name    string
		options func(AuthService, audit.Source) []option
		wantErr require.ErrorAssertionFunc
		want    func(*testing.T, *MiddlewareHandler)
	}{
		{"configured", func(auth AuthService, source audit.Source) []option {
			return []option{WithMiddlewareAuthService(auth), WithRequestAuditor(source)}
		}, require.NoError, func(t *testing.T, h *MiddlewareHandler) {
			t.Helper()
			require.NotNil(t, h)
			require.NotNil(t, h.authService)
			require.NotNil(t, h.requestAuditor)
		}},
		{"missing auditor", func(auth AuthService, _ audit.Source) []option { return []option{WithMiddlewareAuthService(auth)} }, wantError("request auditor is required"), wantNil},
		{"missing auth service", func(_ AuthService, source audit.Source) []option { return []option{WithRequestAuditor(source)} }, wantError("auth service is required"), wantNil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			auth := mocks.NewMockAuthService(gomock.NewController(t))
			opts := tc.options(auth, testaudit.NewAuditor(t))

			h, err := NewMiddlewareHandler(opts...)
			tc.wantErr(t, err)

			tc.want(t, h)
		})
	}
}

func TestAuditResourceUpdate(t *testing.T) {
	t.Parallel()

	checkRouteAudit(t, (*MiddlewareHandler).AuditResourceUpdate, fga.OperationUpdateResource)
}

func TestAuditNotesFilter(t *testing.T) {
	t.Parallel()

	checkRouteAudit(t, (*MiddlewareHandler).AuditNotesFilter, audit.OperationFilterNotes)
}

func checkRouteAudit(t *testing.T, start func(*MiddlewareHandler, echo.HandlerFunc) echo.HandlerFunc, operation string) {
	t.Helper()

	h := MiddlewareHandler{requestAuditor: testaudit.NewAuditor(t)}
	c := echo.New().NewContext(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil), httptest.NewRecorder())
	err := start(&h, func(c echo.Context) error {
		require.NotNil(t, audit.Current(c.Request().Context(), operation))
		return io.ErrClosedPipe
	})(c)
	require.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestAuditRequest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		source     func(*testing.T) audit.Source
		wantErr    error
		wantCalled bool
	}{
		{"audited handler", func(t *testing.T) audit.Source { t.Helper(); return testaudit.NewAuditor(t) }, io.ErrClosedPipe, true},
		{"absent auditor", func(*testing.T) audit.Source { return nil }, audit.ErrMissingAttempt, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := MiddlewareHandler{requestAuditor: tc.source(t)}
			c := echo.New().NewContext(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil), httptest.NewRecorder())
			called := false

			err := h.auditRequest("update", func(c echo.Context) error {
				called = true

				require.NotNil(t, audit.Current(c.Request().Context(), "update"))

				return io.ErrClosedPipe
			})(c)
			require.ErrorIs(t, err, tc.wantErr)

			require.Equal(t, tc.wantCalled, called)
		})
	}
}

func TestTokenRejection(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		startAudit func(*MiddlewareHandler, echo.HandlerFunc) echo.HandlerFunc
		status     int
		wantLog    func(*testing.T, string, string)
	}{
		{"no audit attached", withoutRequestAudit, http.StatusUnauthorized, func(t *testing.T, log, _ string) {
			t.Helper()
			require.Empty(t, log)
		}},
		{"token rejected", (*MiddlewareHandler).AuditResourceUpdate, http.StatusUnauthorized, func(t *testing.T, log, id string) {
			t.Helper()
			require.Contains(t, log, "request_id="+id)
			require.Contains(t, log, "error_code="+string(audit.ErrCodeTokenInvalid))
			require.Contains(t, log, "kind="+audit.KindValidation.String())
		}},
		{"service unavailable", (*MiddlewareHandler).AuditResourceUpdate, http.StatusInternalServerError, func(t *testing.T, log, id string) {
			t.Helper()
			require.Contains(t, log, "request_id="+id)
			require.Contains(t, log, "error_code="+string(audit.ErrCodeServiceUnavailable))
			require.Contains(t, log, "kind="+audit.KindInfra.String())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			id := uuid.NewString()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(`{"request_id":"`+id+`"}`))
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)

			var output bytes.Buffer

			h := MiddlewareHandler{requestAuditor: audit.NewAuditor(audit.WithOutput(&output))}
			err := tc.startAudit(&h, func(c echo.Context) error {
				return tokenRejection(c, tc.status, errors.New("public error"), audit.ErrCodeTokenInvalid, "Token verification failed")
			})(c)
			require.NoError(t, err)

			require.Equal(t, tc.status, rec.Code)
			require.JSONEq(t, `{"error":"public error"}`, rec.Body.String())
			tc.wantLog(t, output.String(), id)
		})
	}
}

func checkVerifiedCallerAudit(t *testing.T, check echo.MiddlewareFunc) {
	t.Helper()

	sink := &captureDecisionAudit{}
	sender, err := audit.NewSender(audit.WithClient(sink), audit.WithTopic("test"))
	require.NoError(t, err)

	h := MiddlewareHandler{requestAuditor: audit.NewAuditor(audit.WithSender(sender))}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v0/auth/notes/filter", nil)
	req.Header.Set("Authorization", "Bearer sample")

	rec := httptest.NewRecorder()
	require.NoError(t, h.AuditNotesFilter(check(nil))(echo.New().NewContext(req, rec)))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Len(t, sink.events, 1)

	var event struct {
		Context map[string]any `json:"context"`
	}
	require.NoError(t, json.Unmarshal(sink.events[0], &event))
	require.Equal(t, "client", event.Context["caller"])
}

func TestVerifiedClient(t *testing.T) {
	t.Parallel()

	active := model.ServiceClient{IsActive: true, Scopes: []string{"bot", "read"}}
	noLookup := func(*mocks.MockAuthService) {}

	lookup := func(client model.ServiceClient, err error) func(*mocks.MockAuthService) {
		return func(auth *mocks.MockAuthService) {
			auth.EXPECT().GetServiceClient(gomock.Any(), "client").Return(client, err)
		}
	}

	for _, tt := range []struct {
		name     string
		claims   jwt.MapClaims
		expect   func(*mocks.MockAuthService)
		required model.Scope
		want     error
		wantID   string
	}{
		{"missing subject", jwt.MapClaims{"scope": "bot"}, noLookup, model.BotScope, errInvalidClientID, ""},
		{"empty subject", jwt.MapClaims{"sub": "", "scope": "bot"}, noLookup, model.BotScope, errInvalidClientID, ""},
		{"numeric subject", jwt.MapClaims{"sub": 123, "scope": "bot"}, noLookup, model.BotScope, errInvalidClientID, ""},
		{"invalid scope type", jwt.MapClaims{"sub": "client", "scope": []string{"bot"}}, noLookup, model.BotScope, errInvalidTokenScope, ""},
		{"issued empty scope", jwt.MapClaims{"sub": "client", "scope": ""}, lookup(model.ServiceClient{IsActive: true}, nil), model.BotScope, errMissingScope, ""},
		{"tab separated scopes", jwt.MapClaims{"sub": "client", "scope": "bot\tread"}, noLookup, model.BotScope, errInvalidTokenScope, ""},
		{"empty scope list", jwt.MapClaims{"sub": "client", "scope": "  "}, noLookup, model.BotScope, errInvalidTokenScope, ""},
		{"admin only", jwt.MapClaims{"sub": "client", "scope": "admin"}, lookup(model.ServiceClient{IsActive: true, Scopes: []string{"bot", "admin"}}, nil), model.BotScope, errMissingScope, ""},
		{"inactive", jwt.MapClaims{"sub": "client", "scope": "bot"}, lookup(model.ServiceClient{}, nil), model.BotScope, errInactiveClient, ""},
		{"missing client", jwt.MapClaims{"sub": "client", "scope": "bot"}, lookup(model.ServiceClient{}, storage.ErrNotFound), model.BotScope, storage.ErrNotFound, ""},
		{"unavailable", jwt.MapClaims{"sub": "client", "scope": "bot"}, lookup(model.ServiceClient{}, context.DeadlineExceeded), model.BotScope, context.DeadlineExceeded, ""},
		{"ungranted scope", jwt.MapClaims{"sub": "client", "scope": "bot admin"}, lookup(active, nil), model.BotScope, errMissingScope, ""},
		{"multiple granted scopes", jwt.MapClaims{"sub": "client", "scope": " read bot "}, lookup(active, nil), model.BotScope, nil, "client"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := mocks.NewMockAuthService(gomock.NewController(t))
			tt.expect(a)

			h := MiddlewareHandler{authService: a}

			got, err := h.verifiedClient(t.Context(), tt.claims, tt.required)
			require.ErrorIs(t, err, tt.want)

			require.Equal(t, tt.wantID, got)
		})
	}
}

type captureDecisionAudit struct {
	events           [][]byte
	liveWithDeadline bool
}

func (s *captureDecisionAudit) Publish(ctx context.Context, _ string, data []byte) error {
	_, bounded := ctx.Deadline()
	s.liveWithDeadline = bounded && ctx.Err() == nil
	s.events = append(s.events, data)

	return nil
}

type brokenDecisionWriter struct{ http.ResponseWriter }

func (w brokenDecisionWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (brokenDecisionWriter) Write([]byte) (int, error) { return 0, errors.New("private write error") }

func TestVerifiedPayload(t *testing.T) {
	t.Parallel()

	token := &jwt.Token{}
	claims := jwt.MapClaims{"sub": "client"}

	for _, tc := range []struct {
		name    string
		expect  func(*mocks.MockAuthService)
		want    jwt.MapClaims
		wantErr error
	}{
		{"invalid JWT", func(s *mocks.MockAuthService) {
			s.EXPECT().CheckToken(gomock.Any(), "token").Return(nil, io.ErrUnexpectedEOF)
		}, nil, io.ErrUnexpectedEOF},
		{"missing claims", func(s *mocks.MockAuthService) {
			s.EXPECT().CheckToken(gomock.Any(), "token").Return(token, nil)
			s.EXPECT().GetPayload(token).Return(nil, false)
		}, nil, errNoTokenPayload},
		{"verified", func(s *mocks.MockAuthService) {
			s.EXPECT().CheckToken(gomock.Any(), "token").Return(token, nil)
			s.EXPECT().GetPayload(token).Return(claims, true)
		}, claims, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := mocks.NewMockAuthService(gomock.NewController(t))
			tc.expect(svc)
			h := MiddlewareHandler{authService: svc}

			got, err := h.verifiedPayload(t.Context(), "token")
			require.ErrorIs(t, err, tc.wantErr)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestConnectionContext(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, forwarded, agent, wantIP, wantAgent string }{
		{"headers", "10.0.0.1", "bot", "10.0.0.1", "bot"},
		{"defaults", "", "", "127.0.0.1:54321", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
			r.RemoteAddr = "127.0.0.1:54321"
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			r.Header.Set("User-Agent", tc.agent)
			r.Header.Set("X-Trace-ID", "trace")
			r.Header.Set("X-Request-ID", "request")

			ctx := connectionContext(r)

			require.Equal(t, tc.wantIP, ctx.Value(ipAddressKey{}))
			require.Equal(t, tc.wantAgent, ctx.Value(userAgentKey{}))
			require.Equal(t, "trace", ctx.Value(traceIDKey{}))
			require.Equal(t, "request", ctx.Value(requestIDKey{}))
		})
	}
}

func TestFillCtx(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	c := echo.New().NewContext(r, httptest.NewRecorder())
	require.NoError(t, FillCtx(func(c echo.Context) error {
		require.Equal(t, "unknown", c.Request().Context().Value(userAgentKey{}))
		return nil
	})(c))

	before := c.Request()

	err := FillCtx(func(c echo.Context) error {
		require.Same(t, before, c.Request())
		return io.ErrClosedPipe
	})(c)
	require.ErrorIs(t, err, io.ErrClosedPipe)
}

func authClaims(a *mocks.MockAuthService, claims jwt.MapClaims) {
	a.EXPECT().CheckToken(gomock.Any(), "Bearer sample").Return(&jwt.Token{}, nil)
	a.EXPECT().GetPayload(gomock.Any()).Return(claims, true)
}
func authClient(a *mocks.MockAuthService) {
	authClaims(a, jwt.MapClaims{"sub": "client", "scope": "bot"})
	a.EXPECT().GetServiceClient(gomock.Any(), "client").Return(model.ServiceClient{IsActive: true, Scopes: []string{"bot"}}, nil)
}

func TestServiceClientID(t *testing.T) {
	t.Parallel()

	require.Empty(t, serviceClientID(t.Context()))

	ctx := context.WithValue(t.Context(), clientIDKey{}, "admin")
	require.Equal(t, "admin", serviceClientID(ctx))
}

// Проверяет запись в глобальный logger последовательно.
//
//nolint:paralleltest // Тест временно меняет вывод глобального логгера.
func TestCheckToken_ExpiredTokenCorrelation(t *testing.T) {
	output := captureTokenWarnings(t)

	headerID := uuid.NewString()
	bodyID := uuid.NewString()

	for _, tc := range []struct {
		name, header string
		wantID       func(*testing.T, string, map[string]string)
	}{
		{"valid header", headerID, func(t *testing.T, id string, body map[string]string) {
			t.Helper()
			require.Equal(t, headerID, id)
			require.Equal(t, id, body["request_id"])
		}},
		{"no header", "", wantGeneratedCorrelation},
		{"invalid header", "invalid", wantGeneratedCorrelation},
		{"zero UUID", uuid.Nil.String(), wantGeneratedCorrelation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output.Reset()

			var auditLog bytes.Buffer

			auth := mocks.NewMockAuthService(gomock.NewController(t))
			auth.EXPECT().CheckToken(gomock.Any(), "Bearer expired").Return(nil, errors.New("expired"))
			h, err := NewMiddlewareHandler(WithRequestAuditor(audit.NewAuditor(audit.WithOutput(&auditLog), audit.WithHook(ConnectionHook))), WithMiddlewareAuthService(auth))
			require.NoError(t, err)

			payload := strings.NewReader(`{"request_id":"` + bodyID + `"}`)
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v0/decisions/check", payload)
			r.Header.Set("Authorization", "Bearer expired")

			if tc.header != "" {
				r.Header.Set(echo.HeaderXRequestID, tc.header)
			}

			rec := httptest.NewRecorder()
			require.NoError(t, h.AuditDecision(h.CheckToken(DecisionToken)(nil))(echo.New().NewContext(r, rec)))

			require.Equal(t, http.StatusUnauthorized, rec.Code)
			require.EqualValues(t, payload.Size(), payload.Len())

			id := rec.Header().Get(echo.HeaderXRequestID)
			require.NotEqual(t, bodyID, id)

			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, "unauthorized", body["error_code"])
			tc.wantID(t, id, body)

			require.Contains(t, output.String(), id)
			require.Contains(t, output.String(), "unauthorized")
			require.Equal(t, 2, strings.Count(auditLog.String(), "request_id="+id))
			require.Equal(t, 1, strings.Count(auditLog.String(), "status=failed"))
			require.Contains(t, auditLog.String(), "stage:authenticate")
		})
	}
}

func captureTokenWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()

	var output bytes.Buffer

	logger := logrus.StandardLogger()
	old := logger.Out
	logger.SetOutput(&output)
	t.Cleanup(func() { logger.SetOutput(old) })

	return &output
}

func wantGeneratedCorrelation(t *testing.T, id string, body map[string]string) {
	t.Helper()

	parsed, err := uuid.Parse(id)
	require.NoError(t, err)

	require.NotEqual(t, uuid.Nil, parsed)
	require.NotContains(t, body, "request_id")
}

func TestCheckToken(t *testing.T) {
	t.Parallel()

	noAuth := func(*mocks.MockAuthService) {}
	noUserCheck := func(*testing.T, context.Context) {}
	resource, notes, decisionRoute := (*MiddlewareHandler).AuditResourceUpdate, (*MiddlewareHandler).AuditNotesFilter, (*MiddlewareHandler).AuditDecision
	denyToken := func(header string) func(*mocks.MockAuthService) {
		return func(a *mocks.MockAuthService) {
			a.EXPECT().CheckToken(gomock.Any(), header).Return(nil, errors.New("private token error"))
		}
	}

	for _, tc := range []struct {
		name     string
		contract TokenContract
		expect   func(*mocks.MockAuthService)
		run      func(*testing.T, echo.MiddlewareFunc)
	}{
		{"resource requires audit before authentication", ResourceToken, noAuth, checkTokenResponse(withoutRequestAudit, "42", http.StatusInternalServerError, noUserCheck)},
		{"decision requires audit before authentication", DecisionToken, noAuth, checkTokenResponse(withoutRequestAudit, "42", http.StatusInternalServerError, noUserCheck)},
		{"token rejected", ResourceToken, denyToken("Bearer sample"), checkTokenResponse(resource, "42", http.StatusUnauthorized, noUserCheck)},
		{"resource rejects token without bot", ResourceToken, expectGrantedScope("read"), checkTokenResponse(resource, "42", http.StatusUnauthorized, noUserCheck)},
		{"resource checks Telegram header", ResourceToken, expectGrantedScope("bot"), checkTokenResponse(resource, "", http.StatusUnauthorized, noUserCheck)},
		{"resource accepts granted scopes including bot", ResourceToken, expectGrantedScope("bot read"), checkTokenResponse(resource, "42", http.StatusNoContent, wantTelegramUser)},
		{"decision delegates Telegram verification to service", DecisionToken, expectGrantedScope("read bot"), checkTokenResponse(decisionRoute, "", http.StatusNoContent, noUserCheck)},
		{"decision requires bot scope", DecisionToken, expectGrantedScope("admin"), checkTokenResponse(decisionRoute, "42", http.StatusForbidden, noUserCheck)},
		{"denied resource records interrupted response", ResourceToken, denyToken("Bearer private"), checkRejectedRequestAudit("/api/v0/resources/update", fga.OperationUpdateResource, resource)},
		{"denied notes record interrupted response", ResourceToken, denyToken("Bearer private"), checkRejectedRequestAudit("/api/v0/auth/notes/filter", audit.OperationFilterNotes, notes)},
		{"failed user check records verified service client", ResourceToken, authClient, checkVerifiedCallerAudit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			auth := mocks.NewMockAuthService(gomock.NewController(t))
			tc.expect(auth)
			h := MiddlewareHandler{authService: auth}

			check := h.CheckToken(tc.contract)

			tc.run(t, check)
		})
	}
}

func checkTokenResponse(startAudit func(*MiddlewareHandler, echo.HandlerFunc) echo.HandlerFunc, telegramID string, status int, wantUser func(*testing.T, context.Context)) func(*testing.T, echo.MiddlewareFunc) {
	return func(t *testing.T, check echo.MiddlewareFunc) {
		t.Helper()

		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer sample")
		req.Header.Set(userIDHeader, telegramID)

		called := false
		next := func(c echo.Context) error {
			called = true

			require.Equal(t, "client", serviceClientID(c.Request().Context()))
			wantUser(t, c.Request().Context())

			return c.NoContent(http.StatusNoContent)
		}
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)

		err := startAudit(testAuditMiddleware(t), check(next))(c)
		require.NoError(t, err)

		require.Equal(t, status, rec.Code)
		require.Equal(t, status == http.StatusNoContent, called)
	}
}

func expectGrantedScope(scope string) func(*mocks.MockAuthService) {
	return func(a *mocks.MockAuthService) {
		authClaims(a, jwt.MapClaims{"sub": "client", "scope": scope})
		a.EXPECT().GetServiceClient(gomock.Any(), "client").Return(model.ServiceClient{
			IsActive: true, Scopes: []string{"bot", "read", "admin"},
		}, nil)
	}
}

func TestRejectInvalidToken(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		contract    TokenContract
		cause       error
		field, want string
	}{
		{"resource token", ResourceToken, errors.New("expired token"), "error", "expired token"},
		{"resource claims", ResourceToken, errNoTokenPayload, "error", errNoTokenPayload.Error()},
		{"decision claims", DecisionToken, errNoTokenPayload, "error_code", "unauthorized"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)

			require.NoError(t, rejectInvalidToken(c, tc.contract, tc.cause))
			require.Equal(t, http.StatusUnauthorized, rec.Code)

			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tc.want, body[tc.field])
		})
	}
}

func TestRejectServiceClient(t *testing.T) {
	t.Parallel()

	headerID := uuid.NewString()

	for _, tc := range []struct {
		name        string
		contract    TokenContract
		cause       error
		status      int
		field, want string
		wantHeader  string
	}{
		{"resource unknown client", ResourceToken, storage.ErrNotFound, 401, "error", "invalid client", ""},
		{"resource inactive client", ResourceToken, errInactiveClient, 401, "error", errInactiveClient.Error(), ""},
		{"resource denied scope", ResourceToken, errMissingScope, 401, "error", errMissingScope.Error(), ""},
		{"resource unavailable storage", ResourceToken, context.DeadlineExceeded, 500, "error", "internal server error", ""},
		{"decision invalid client claim", DecisionToken, errInvalidClientID, 401, "error_code", "unauthorized", headerID},
		{"decision forbidden", DecisionToken, errMissingScope, 403, "error_code", "forbidden", headerID},
		{"decision unavailable storage", DecisionToken, context.DeadlineExceeded, 503, "error_code", "identity_store_unavailable", headerID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			payload := strings.NewReader(`{"request_id":"` + uuid.NewString() + `"}`)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", payload)
			req.Header.Set(echo.HeaderXRequestID, headerID)
			c := echo.New().NewContext(req, rec)

			require.NoError(t, rejectServiceClient(c, tc.contract, tc.cause))
			require.Equal(t, tc.status, rec.Code)

			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tc.want, body[tc.field])
			require.Equal(t, tc.wantHeader, rec.Header().Get(echo.HeaderXRequestID))
			require.Equal(t, tc.wantHeader, body["request_id"])
			require.EqualValues(t, payload.Size(), payload.Len())
		})
	}
}

func TestServeAuthenticated(t *testing.T) {
	t.Parallel()

	wantNoTelegram := func(t *testing.T, ctx context.Context) {
		t.Helper()

		_, ok := userIDFromContext(ctx)
		require.False(t, ok)
	}

	for _, tc := range []struct {
		name       string
		contract   TokenContract
		header     string
		wantUser   func(*testing.T, context.Context)
		wantCalled bool
		wantErr    error
		wantStatus int
	}{
		{"resource sets Telegram identity", ResourceToken, "42", wantTelegramUser, true, io.ErrClosedPipe, http.StatusOK},
		{"resource rejects zero", ResourceToken, "0", func(*testing.T, context.Context) {}, false, nil, http.StatusBadRequest},
		{"resource rejects signed ID", ResourceToken, "-1", func(*testing.T, context.Context) {}, false, nil, http.StatusBadRequest},
		{"resource requires Telegram identity", ResourceToken, "", func(*testing.T, context.Context) {}, false, nil, http.StatusUnauthorized},
		{"decision passes a missing header to the service", DecisionToken, "", wantNoTelegram, true, io.ErrClosedPipe, http.StatusOK},
		{"decision defers a present header to the service", DecisionToken, "42", wantNoTelegram, true, io.ErrClosedPipe, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
			req.Header.Set(userIDHeader, tc.header)

			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			called := false

			err := serveAuthenticated(c, tc.contract, "verified-client", func(c echo.Context) error {
				called = true
				ctx := c.Request().Context()
				require.Equal(t, "verified-client", serviceClientID(ctx))
				tc.wantUser(t, ctx)

				return io.ErrClosedPipe
			})
			require.ErrorIs(t, err, tc.wantErr)

			require.Equal(t, tc.wantCalled, called)
			require.Equal(t, tc.wantStatus, rec.Code)
		})
	}
}

func wantTelegramUser(t *testing.T, ctx context.Context) {
	t.Helper()

	id, ok := userIDFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "42", id)
}

func withoutRequestAudit(_ *MiddlewareHandler, next echo.HandlerFunc) echo.HandlerFunc {
	return next
}

func testAuditMiddleware(t *testing.T) *MiddlewareHandler {
	t.Helper()
	return &MiddlewareHandler{requestAuditor: testaudit.NewAuditor(t)}
}

func TestBeginAuthentication(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		contract TokenContract
		open     func(*MiddlewareHandler, echo.HandlerFunc) echo.HandlerFunc
		want     error
	}{
		{"missing resource", ResourceToken, func(_ *MiddlewareHandler, next echo.HandlerFunc) echo.HandlerFunc { return next }, audit.ErrMissingAttempt},
		{"missing decision", DecisionToken, func(_ *MiddlewareHandler, next echo.HandlerFunc) echo.HandlerFunc { return next }, audit.ErrMissingAttempt},
		{"notes", ResourceToken, (*MiddlewareHandler).AuditNotesFilter, nil},
		{"decision", DecisionToken, (*MiddlewareHandler).AuditDecision, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := echo.New().NewContext(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil), httptest.NewRecorder())
			err := tc.open(testAuditMiddleware(t), func(c echo.Context) error {
				return beginAuthentication(c.Request().Context(), tc.contract)
			})(c)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestRecordResponseWriteFailure(t *testing.T) {
	t.Parallel()

	withAudit := func(t *testing.T, check func(context.Context)) string {
		t.Helper()

		var output bytes.Buffer

		ctx, attempt, err := audit.Begin(t.Context(), audit.NewAuditor(audit.WithOutput(&output)), audit.OperationFilterNotes, audit.StageAuthenticate.String())
		require.NoError(t, err)

		check(ctx)
		attempt.Finish(ctx, &err)

		return output.String()
	}
	withoutAudit := func(t *testing.T, check func(context.Context)) string {
		t.Helper()
		check(t.Context())

		return ""
	}
	absent := func(t *testing.T, log string) {
		t.Helper()
		require.NotContains(t, log, audit.KeyResponseWriteFailed)
	}

	for _, tc := range []struct {
		name      string
		err       error
		committed bool
		run       func(*testing.T, func(context.Context)) string
		want      func(*testing.T, string)
	}{
		{"response written", nil, true, withAudit, absent},
		{"before headers", io.ErrClosedPipe, false, withAudit, absent},
		{"write interrupted", io.ErrClosedPipe, true, withAudit, func(t *testing.T, log string) {
			t.Helper()
			require.Contains(t, log, audit.KeyResponseWriteFailed)
		}},
		{"decision uses its own event", io.ErrClosedPipe, true, withoutAudit, func(t *testing.T, log string) {
			t.Helper()
			require.Empty(t, log)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			log := tc.run(t, func(ctx context.Context) {
				recordResponseWriteFailure(audit.FromContext(ctx), &echo.Response{Committed: tc.committed}, &tc.err)
			})

			tc.want(t, log)
		})
	}
}

func TestValidScopeList(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, scope string
		valid       bool
	}{
		{"no grants", "", true}, {"single permission", "bot", true},
		{"multiple permissions", " bot  admin ", true}, {"spaces only", " ", false},
		{"tab", "bot\tadmin", false}, {"line break", "bot\nadmin", false},
		{"non-breaking space", "bot\u00a0admin", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.valid, validScopeList(tc.scope))
		})
	}
}
