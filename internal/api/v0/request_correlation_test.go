package v0

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestRejectedRequestID(t *testing.T) {
	t.Parallel()

	id := uuid.New()

	for _, tt := range []struct {
		name string
		body io.ReadCloser
		want uuid.UUID
	}{
		{"absent body", nil, uuid.Nil},
		{"HTTP empty body", http.NoBody, uuid.Nil},
		{"empty stream", io.NopCloser(strings.NewReader("")), uuid.Nil},
		{"valid", io.NopCloser(strings.NewReader(fmt.Sprintf(`{"request_id":%q}`, id))), id},
		{"missing", io.NopCloser(strings.NewReader("{}")), uuid.Nil},
		{"invalid UUID", io.NopCloser(strings.NewReader(`{"request_id":"bad"}`)), uuid.Nil},
		{"trailing document", io.NopCloser(strings.NewReader(fmt.Sprintf(`{"request_id":%q} {}`, id))), uuid.Nil},
		{"large valid body", io.NopCloser(strings.NewReader(strings.Repeat(" ", 1<<20) + fmt.Sprintf(`{"request_id":%q}`, id))), id},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), "POST", "/", nil)
			r.Body = tt.body
			c := echo.New().NewContext(r, httptest.NewRecorder())
			require.Equal(t, tt.want, rejectedRequestID(c))
		})
	}
}
