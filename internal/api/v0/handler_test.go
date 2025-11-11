package v0

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест
func TestNew(t *testing.T) {
	t.Parallel()

	type test struct {
		name      string
		version   string
		buildDate string
		gitCommit string
		wantErr   require.ErrorAssertionFunc
		want      *Handler
	}

	tests := []test{
		{
			name:      "success",
			version:   "1.0.0",
			buildDate: "2021-01-01",
			gitCommit: "1234567890",
			wantErr:   require.NoError,
			want:      &Handler{version: "1.0.0", buildDate: "2021-01-01", gitCommit: "1234567890", apiVersion: Version0},
		},
		{
			name:      "version is required",
			version:   "",
			buildDate: "2021-01-01",
			gitCommit: "1234567890",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "version is required")
			},
			want: nil,
		},
		{
			name:      "buildDate is required",
			version:   "1.0.0",
			buildDate: "",
			gitCommit: "1234567890",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "buildDate is required")
			},
			want: nil,
		},
		{
			name:      "gitCommit is required",
			version:   "1.0.0",
			buildDate: "2021-01-01",
			gitCommit: "",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "gitCommit is required")
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, err := New(
				WithVersion(tt.version),
				WithBuildDate(tt.buildDate),
				WithGitCommit(tt.gitCommit),
			)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, handler)
		})
	}
}

func TestHandler_Version(t *testing.T) {
	t.Parallel()

	handler, err := New(
		WithVersion("1.0.0"),
		WithBuildDate("2021-01-01"),
		WithGitCommit("1234567890"),
	)

	require.NoError(t, err)
	assert.Equal(t, Version0, handler.Version())
}

func testRequest(t *testing.T, ts *httptest.Server, method,
	path string, token string, body io.Reader) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, ts.URL+path, body)
	require.NoError(t, err)

	req.Close = true
	req.Header.Add("Connection", "keep-alive")
	req.Header.Add("User-Agent", "PostmanRuntime/7.32.3")

	if token != "" {
		req.Header.Set("Authorization", token)
	}

	ts.Client().CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)

	return resp
}

func runTestServer(t *testing.T, h *Handler) *echo.Echo {
	t.Helper()

	e := echo.New()

	api := e.Group("api/")

	// v0
	apiv0 := api.Group("v0/")

	apiv0.GET("health", h.Health)

	return e
}
