package v0

import (
	"auth-service/internal/api/v0/mocks"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
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
		makeWant  func(authSvc mocks.MockAuthService, politicsSvc mocks.MockPoliticsService) *Handler
	}

	tests := []test{
		{
			name:      "success",
			version:   "1.0.0",
			buildDate: "2021-01-01",
			gitCommit: "1234567890",
			wantErr:   require.NoError,
			makeWant: func(authSvc mocks.MockAuthService, politicsSvc mocks.MockPoliticsService) *Handler {
				return &Handler{
					version:         "1.0.0",
					buildDate:       "2021-01-01",
					gitCommit:       "1234567890",
					apiVersion:      Version0,
					PoliticsService: &politicsSvc,
				}
			},
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
			makeWant: func(authSvc mocks.MockAuthService, politicsSvc mocks.MockPoliticsService) *Handler {
				t.Helper()

				return nil
			},
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
			makeWant: func(authSvc mocks.MockAuthService, politicsSvc mocks.MockPoliticsService) *Handler {
				t.Helper()

				return nil
			},
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
			makeWant: func(authSvc mocks.MockAuthService, politicsSvc mocks.MockPoliticsService) *Handler {
				t.Helper()

				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			authSvc := mocks.NewMockAuthService(ctrl)
			politicsSvc := mocks.NewMockPoliticsService(ctrl)

			handler, err := New(
				WithVersion(tt.version),
				WithBuildDate(tt.buildDate),
				WithGitCommit(tt.gitCommit),
				WithPoliticsService(politicsSvc),
			)

			tt.wantErr(t, err)
			assert.Equal(t, tt.makeWant(*authSvc, *politicsSvc), handler)
		})
	}
}

func TestHandler_Version(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	politicsSvc := mocks.NewMockPoliticsService(ctrl)

	handler, err := New(
		WithVersion("1.0.0"),
		WithBuildDate("2021-01-01"),
		WithGitCommit("1234567890"),
		WithPoliticsService(politicsSvc),
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

	resp, err := ts.Client().Do(req) //nolint:gosec // тестовая функция
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
