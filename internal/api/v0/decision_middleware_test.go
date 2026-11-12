package v0

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"auth-service/internal/service/decision"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestDecisionClientError(t *testing.T) {
	t.Parallel()

	noExtraCheck := func(*testing.T, error) {}
	for _, tc := range []struct {
		cause, want error
		check       func(*testing.T, error)
	}{
		{errInvalidClientID, decision.ErrUnauthorized, noExtraCheck},
		{errInvalidTokenScope, decision.ErrUnauthorized, noExtraCheck},
		{storage.ErrNotFound, decision.ErrForbidden, noExtraCheck},
		{errInactiveClient, decision.ErrForbidden, noExtraCheck},
		{errMissingScope, decision.ErrForbidden, noExtraCheck},
		{context.DeadlineExceeded, decision.ErrIdentityUnavailable, func(t *testing.T, err error) {
			t.Helper()
			require.ErrorIs(t, err, context.DeadlineExceeded)
		}},
	} {
		t.Run(tc.cause.Error(), func(t *testing.T) {
			t.Parallel()

			err := decisionClientError(tc.cause)
			require.ErrorIs(t, err, tc.want)
			tc.check(t, err)
		})
	}
}

func TestAuditDecision(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		check func(*testing.T)
	}{
		{"unavailable audit stops before authentication", func(t *testing.T) {
			t.Helper()

			h := MiddlewareHandler{}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
			id := "00000000-0000-4000-8000-000000000001"
			req.Header.Set(echo.HeaderXRequestID, id)

			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			err := h.AuditDecision(func(echo.Context) error { t.Fatal("handler called without audit"); return nil })(c)
			require.NoError(t, err)

			require.Equal(t, http.StatusInternalServerError, rec.Code)

			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, "internal_error", body["error_code"])
			require.Equal(t, id, body["request_id"])
			require.Equal(t, id, rec.Header().Get(echo.HeaderXRequestID))
		}},
		{"interruption before authentication", func(t *testing.T) {
			t.Helper()

			var output bytes.Buffer

			h := MiddlewareHandler{requestAuditor: audit.NewAuditor(audit.WithOutput(&output))}
			c := echo.New().NewContext(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil), httptest.NewRecorder())

			id := "00000000-0000-4000-8000-000000000002"
			c.Request().Header.Set(echo.HeaderXRequestID, id)
			err := h.AuditDecision(func(echo.Context) error {
				require.Contains(t, output.String(), "status=started")
				require.NotContains(t, output.String(), "status=failed")

				return io.ErrClosedPipe
			})(c)
			require.ErrorIs(t, err, io.ErrClosedPipe)

			require.Contains(t, output.String(), "stage:receive_request")
			require.Contains(t, strings.Split(strings.TrimSpace(output.String()), "\n")[1], id)
			require.Equal(t, 1, strings.Count(output.String(), "status=failed"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) { t.Parallel(); tc.check(t) })
	}
}
