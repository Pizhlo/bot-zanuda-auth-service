package server

import (
	handlerV0 "auth-service/internal/api/v0"
	"auth-service/internal/server/mocks"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест
func TestNewServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		createOpts func(t *testing.T, mockHandler *mocks.Mockhandler) []Option
		createWant func(t *testing.T, mockHandler *mocks.Mockhandler) *Server
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler) []Option {
				t.Helper()

				mockHandler.EXPECT().Version().Return("v0")

				return []Option{
					WithPort(8080),
					WithShutdownTimeout(100 * time.Millisecond),
					WithHandlerV0(mockHandler),
				}
			},
			createWant: func(t *testing.T, mockHandler *mocks.Mockhandler) *Server {
				t.Helper()

				return &Server{
					port:            8080,
					shutdownTimeout: 100 * time.Millisecond,
					api: struct {
						h0 handler
					}{h0: mockHandler},
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: handler is required",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler) []Option {
				t.Helper()

				return []Option{
					WithPort(8080),
					WithShutdownTimeout(100 * time.Millisecond),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "handler is required")
			},
		},
		{
			name: "error case: handler version is not v0",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler) []Option {
				t.Helper()

				mockHandler.EXPECT().Version().Return("v1").Times(2)

				return []Option{
					WithPort(8080),
					WithShutdownTimeout(100 * time.Millisecond),
					WithHandlerV0(mockHandler),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "expected handler version is v0, got v1")
			},
		},
		{
			name: "error case: port is required",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler) []Option {
				t.Helper()

				return []Option{
					WithShutdownTimeout(100 * time.Millisecond),
					WithHandlerV0(mockHandler),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "port is required")
			},
		},
		{
			name: "error case: shutdown timeout is required",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler) []Option {
				t.Helper()

				return []Option{
					WithPort(8080),
					WithHandlerV0(mockHandler),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "shutdown timeout is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler := mocks.NewMockhandler(ctrl)

			server, err := New(tt.createOpts(t, handler)...)
			tt.wantErr(t, err)

			if tt.createWant != nil {
				require.NotNil(t, server)
				assert.Equal(t, tt.createWant(t, handler), server)
			}
		})
	}
}

func TestCreateRoutes(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h := mocks.NewMockhandler(ctrl)
	h.EXPECT().Version().Return("v0").Times(1)

	server, err := New(
		WithPort(8080),
		WithShutdownTimeout(100*time.Millisecond),
		WithHandlerV0(h),
	)
	require.NoError(t, err)

	err = server.createRoutes()
	require.NoError(t, err)

	routes := server.e.Routes()

	expectedRoutes := []*echo.Route{
		{
			Method: http.MethodGet,
			Path:   "/api/v0/health",
			Name:   "webserver/internal/server.handler.Health-fm",
		},
		{
			Method: http.MethodGet,
			Path:   "/metrics",
			Name:   "github.com/labstack/echo-contrib/echoprometheus.NewHandler",
		},
		{
			Method: http.MethodGet,
			Path:   "/swagger/*",
			Name:   "github.com/swaggo/echo-swagger.EchoWrapHandler.func1",
		},
	}

	assert.Equal(t, len(expectedRoutes), len(routes))

	actualRoutesMap := routesMap(t, routes)

	// not found routes
	notFound := []string{}

	// path: method
	expRoutesMap := routesMap(t, expectedRoutes)

	for expectedPath, expectedMethod := range expRoutesMap {
		if actualMethod, found := actualRoutesMap[expectedPath]; !found {
			notFound = append(notFound, expectedPath)
		} else {
			assert.Equal(t, expectedMethod, actualMethod, fmt.Sprintf("methods not equal for path '%s'", expectedPath))
		}
	}

	if len(notFound) > 0 {
		t.Errorf("not found paths: %+v", notFound)
	}
}

func TestCheckHandlerVersion(t *testing.T) {
	t.Parallel()

	type test struct {
		name            string
		version         string
		expectedVersion string
		want            bool
	}

	tests := []test{
		{name: "positive case", version: "v0", expectedVersion: handlerV0.Version0, want: true},
		{name: "negative case", version: "v1", expectedVersion: handlerV0.Version0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler := mocks.NewMockhandler(ctrl)
			handler.EXPECT().Version().Return(tt.version)

			assert.Equal(t, tt.want, checkHandlerVersion(handler, tt.expectedVersion))
		})
	}
}

func routesMap(t *testing.T, routes []*echo.Route) map[string]string {
	t.Helper()

	res := map[string]string{}

	for _, r := range routes {
		res[r.Path] = r.Method
	}

	return res
}
