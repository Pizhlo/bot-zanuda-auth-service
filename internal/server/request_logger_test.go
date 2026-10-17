package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestRequestLogger(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			e := echo.New()

			var output bytes.Buffer
			e.Logger.SetOutput(&output)
			e.Use(requestLogger())
			e.POST("/test", func(c echo.Context) error {
				if status != http.StatusOK {
					return echo.NewHTTPError(status, "bad request")
				}

				return c.String(status, "ok")
			})

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/test", bytes.NewBufferString("data"))
			req.Header.Set(echo.HeaderContentLength, "4")
			req.Header.Set(echo.HeaderXRequestID, "request-id")
			req.Header.Set(echo.HeaderAuthorization, "Bearer private-token")

			res := httptest.NewRecorder()
			e.ServeHTTP(res, req)
			require.Equal(t, status, res.Code)

			var entry map[string]any
			require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
			require.Len(t, entry, 13)
			require.Equal(t, "request-id", entry["id"])
			require.Equal(t, float64(status), entry["status"])
			require.Equal(t, float64(4), entry["bytes_in"])
			require.Equal(t, float64(res.Body.Len()), entry["bytes_out"])
			require.Equal(t, "/test", entry["uri"])

			if status == http.StatusOK {
				require.Empty(t, entry["error"])
			} else {
				require.Contains(t, entry["error"], "bad request")
			}

			require.NotContains(t, output.String(), "private-token")
		})
	}
}
