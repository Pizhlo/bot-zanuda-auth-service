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

//nolint:funlen,dupl // длинный тест - ничего страшного; похожие тест-кейсы
func TestNewHandler(t *testing.T) {
	t.Parallel()

	type test struct {
		name     string
		opts     func(t *testing.T, ctrl *gomock.Controller, politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) []handlerOption
		wantErr  require.ErrorAssertionFunc
		makeWant func(politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) *Handler
	}

	tests := []test{
		{
			name: "success",
			opts: func(t *testing.T, ctrl *gomock.Controller, politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(authHandler),
					WithPoliticsService(politicsSvc),
					WithVersion("1.0.0"),
					WithBuildDate("2021-01-01"),
					WithGitCommit("1234567890"),
				}
			},
			wantErr: require.NoError,
			makeWant: func(politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) *Handler {
				return &Handler{
					version:         "1.0.0",
					buildDate:       "2021-01-01",
					gitCommit:       "1234567890",
					apiVersion:      Version0,
					PoliticsService: politicsSvc,
					auth:            authHandler,
				}
			},
		},
		{
			name: "version is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(authHandler),
					WithPoliticsService(politicsSvc),
					WithBuildDate("2021-01-01"),
					WithGitCommit("1234567890"),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "version is required")
			},
			makeWant: func(politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) *Handler {
				t.Helper()

				return nil
			},
		},
		{
			name: "buildDate is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(authHandler),
					WithPoliticsService(politicsSvc),
					WithVersion("1.0.0"),
					WithGitCommit("1234567890"),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "buildDate is required")
			},
			makeWant: func(politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) *Handler {
				t.Helper()

				return nil
			},
		},
		{
			name: "gitCommit is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(authHandler),
					WithPoliticsService(politicsSvc),
					WithVersion("1.0.0"),
					WithBuildDate("2021-01-01"),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "gitCommit is required")
			},
			makeWant: func(politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) *Handler {
				t.Helper()

				return nil
			},
		},
		{
			name: "politics service is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(authHandler),
					WithVersion("1.0.0"),
					WithBuildDate("2021-01-01"),
					WithGitCommit("1234567890"),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "politics service is required")
			},
			makeWant: func(politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) *Handler {
				t.Helper()

				return nil
			},
		},
		{
			name: "auth handler is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithPoliticsService(politicsSvc),
					WithVersion("1.0.0"),
					WithBuildDate("2021-01-01"),
					WithGitCommit("1234567890"),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "auth handler is required")
			},
			makeWant: func(politicsSvc *mocks.MockPoliticsService, authHandler *mocks.MockauthProcessorHandler) *Handler {
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

			politicsSvc := mocks.NewMockPoliticsService(ctrl)
			authHandler := mocks.NewMockauthProcessorHandler(ctrl)

			handler, err := NewHandler(tt.opts(t, ctrl, politicsSvc, authHandler)...)

			tt.wantErr(t, err)
			assert.Equal(t, tt.makeWant(politicsSvc, authHandler), handler)
		})
	}
}

func TestHandler_Version(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	politicsSvc := mocks.NewMockPoliticsService(ctrl)

	authSvc := mocks.NewMockauthSerivce(ctrl)

	authHandler, err := NewAuthHandler(
		WithAuthService(authSvc),
	)
	require.NoError(t, err)

	handler, err := NewHandler(
		WithVersion("1.0.0"),
		WithBuildDate("2021-01-01"),
		WithGitCommit("1234567890"),
		WithPoliticsService(politicsSvc),
		WithAuthHandler(authHandler),
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
