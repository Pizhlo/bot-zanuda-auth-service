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

	type mocksStruct struct {
		notesHandler     *mocks.MocknotesProcessorHandler
		authHandler      *mocks.MockauthProcessorHandler
		resourcesHandler *mocks.MockresourcesProcessorHandler
	}

	type test struct {
		name     string
		opts     func(t *testing.T, ctrl *gomock.Controller, mocks mocksStruct) []handlerOption
		wantErr  require.ErrorAssertionFunc
		makeWant func(mocks mocksStruct) *Handler
	}

	tests := []test{
		{
			name: "success",
			opts: func(t *testing.T, ctrl *gomock.Controller, mocks mocksStruct) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(mocks.authHandler),
					WithNotesHandler(mocks.notesHandler),
					WithVersion("1.0.0"),
					WithBuildDate("2021-01-01"),
					WithGitCommit("1234567890"),
					WithResourcesHandler(mocks.resourcesHandler),
				}
			},
			wantErr: require.NoError,
			makeWant: func(mocks mocksStruct) *Handler {
				return &Handler{
					version:    "1.0.0",
					buildDate:  "2021-01-01",
					gitCommit:  "1234567890",
					apiVersion: Version0,
					notes:      mocks.notesHandler,
					auth:       mocks.authHandler,
					resources:  mocks.resourcesHandler,
				}
			},
		},
		{
			name: "version is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, mocks mocksStruct) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(mocks.authHandler),
					WithNotesHandler(mocks.notesHandler),
					WithBuildDate("2021-01-01"),
					WithGitCommit("1234567890"),
					WithResourcesHandler(mocks.resourcesHandler),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "version is required")
			},
			makeWant: func(mocks mocksStruct) *Handler {
				t.Helper()

				return nil
			},
		},
		{
			name: "buildDate is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, mocks mocksStruct) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(mocks.authHandler),
					WithNotesHandler(mocks.notesHandler),
					WithVersion("1.0.0"),
					WithGitCommit("1234567890"),
					WithResourcesHandler(mocks.resourcesHandler),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "buildDate is required")
			},
			makeWant: func(mocks mocksStruct) *Handler {
				t.Helper()

				return nil
			},
		},
		{
			name: "gitCommit is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, mocks mocksStruct) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(mocks.authHandler),
					WithNotesHandler(mocks.notesHandler),
					WithVersion("1.0.0"),
					WithBuildDate("2021-01-01"),
					WithResourcesHandler(mocks.resourcesHandler),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "gitCommit is required")
			},
			makeWant: func(mocks mocksStruct) *Handler {
				t.Helper()

				return nil
			},
		},
		{
			name: "notes handler is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, mocks mocksStruct) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(mocks.authHandler),
					WithVersion("1.0.0"),
					WithBuildDate("2021-01-01"),
					WithGitCommit("1234567890"),
					WithResourcesHandler(mocks.resourcesHandler),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "notes handler is required")
			},
			makeWant: func(mocks mocksStruct) *Handler {
				t.Helper()

				return nil
			},
		},
		{
			name: "auth handler is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, mocks mocksStruct) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithNotesHandler(mocks.notesHandler),
					WithVersion("1.0.0"),
					WithBuildDate("2021-01-01"),
					WithGitCommit("1234567890"),
					WithResourcesHandler(mocks.resourcesHandler),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "auth handler is required")
			},
			makeWant: func(mocks mocksStruct) *Handler {
				t.Helper()

				return nil
			},
		},
		{
			name: "resources handler is required",
			opts: func(t *testing.T, ctrl *gomock.Controller, mocks mocksStruct) []handlerOption {
				t.Helper()

				return []handlerOption{
					WithAuthHandler(mocks.authHandler),
					WithNotesHandler(mocks.notesHandler),
					WithVersion("1.0.0"),
					WithBuildDate("2021-01-01"),
					WithGitCommit("1234567890"),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "resources handler is required")
			},
			makeWant: func(mocks mocksStruct) *Handler {
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

			mocks := mocksStruct{
				notesHandler:     mocks.NewMocknotesProcessorHandler(ctrl),
				authHandler:      mocks.NewMockauthProcessorHandler(ctrl),
				resourcesHandler: mocks.NewMockresourcesProcessorHandler(ctrl),
			}

			handler, err := NewHandler(tt.opts(t, ctrl, mocks)...)

			tt.wantErr(t, err)
			assert.Equal(t, tt.makeWant(mocks), handler)
		})
	}
}

func TestHandler_Version(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	notesHandler := mocks.NewMocknotesProcessorHandler(ctrl)
	resourcesHandler := mocks.NewMockresourcesProcessorHandler(ctrl)
	authHandler := mocks.NewMockauthProcessorHandler(ctrl)

	handler, err := NewHandler(
		WithVersion("1.0.0"),
		WithBuildDate("2021-01-01"),
		WithGitCommit("1234567890"),
		WithNotesHandler(notesHandler),
		WithAuthHandler(authHandler),
		WithResourcesHandler(resourcesHandler),
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
