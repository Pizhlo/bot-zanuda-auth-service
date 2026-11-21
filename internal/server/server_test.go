package server

import (
	"bytes"
	"io"
	"strings"

	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	handlerV0 "auth-service/internal/api/v0"
	apimocks "auth-service/internal/api/v0/mocks"
	"auth-service/internal/server/mocks"
	"auth-service/pkg/audit"

	"github.com/labstack/echo/v4"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест
func TestNew(t *testing.T) {
	t.Parallel()

	wantNoServer := func(t *testing.T, _ *mocks.Mockhandler, _ *mocks.MockmiddlewareHandler, server *Server) {
		t.Helper()
		require.Nil(t, server)
	}

	tests := []struct {
		name       string
		createOpts func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler) []Option
		want       func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler, server *Server)
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler) []Option {
				t.Helper()

				mockHandler.EXPECT().Version().Return("v0")

				return []Option{
					WithPort(8080),
					WithShutdownTimeout(100 * time.Millisecond),
					WithHandlerV0(mockHandler),
					WithMiddlewareHandler(mdHandler),
				}
			},
			want: func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler, server *Server) {
				t.Helper()

				require.Equal(t, &Server{
					port:            8080,
					shutdownTimeout: 100 * time.Millisecond,
					api: struct {
						h0         handler
						middleware middlewareHandler
					}{
						h0:         mockHandler,
						middleware: mdHandler,
					},
				}, server)
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: handler is required",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler) []Option {
				t.Helper()

				return []Option{
					WithPort(8080),
					WithShutdownTimeout(100 * time.Millisecond),
					WithMiddlewareHandler(mdHandler),
				}
			},
			want: wantNoServer,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "handler is required")
			},
		},
		{
			name: "error case: handler version is not v0",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler) []Option {
				t.Helper()

				mockHandler.EXPECT().Version().Return("v1").Times(2)

				return []Option{
					WithPort(8080),
					WithShutdownTimeout(100 * time.Millisecond),
					WithHandlerV0(mockHandler),
					WithMiddlewareHandler(mdHandler),
				}
			},
			want: wantNoServer,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "expected handler version is v0, got v1")
			},
		},
		{
			name: "error case: port is required",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler) []Option {
				t.Helper()

				return []Option{
					WithShutdownTimeout(100 * time.Millisecond),
					WithHandlerV0(mockHandler),
					WithMiddlewareHandler(mdHandler),
				}
			},
			want: wantNoServer,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "port is required")
			},
		},
		{
			name: "error case: shutdown timeout is required",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler) []Option {
				t.Helper()

				return []Option{
					WithPort(8080),
					WithHandlerV0(mockHandler),
					WithMiddlewareHandler(mdHandler),
				}
			},
			want: wantNoServer,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "shutdown timeout is required")
			},
		},
		{
			name: "error case: middleware handler is required",
			createOpts: func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler) []Option {
				t.Helper()

				return []Option{
					WithShutdownTimeout(100 * time.Millisecond),
					WithPort(8080),
					WithHandlerV0(mockHandler),
				}
			},
			want: wantNoServer,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "middleware handler is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler := mocks.NewMockhandler(ctrl)
			mdHandler := mocks.NewMockmiddlewareHandler(ctrl)

			server, err := New(tt.createOpts(t, handler, mdHandler)...)
			tt.wantErr(t, err)

			tt.want(t, handler, mdHandler, server)
		})
	}
}

func TestCheckHandlerVersion(t *testing.T) {
	t.Parallel()

	expectVersion := func(version string) func(*mocks.Mockhandler) {
		return func(h *mocks.Mockhandler) { h.EXPECT().Version().Return(version) }
	}

	type test struct {
		name            string
		expect          func(*mocks.Mockhandler)
		expectedVersion string
		want            bool
	}

	tests := []test{
		{name: "positive case", expect: expectVersion("v0"), expectedVersion: handlerV0.Version0, want: true},
		{name: "negative case", expect: expectVersion("v1"), expectedVersion: handlerV0.Version0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler := mocks.NewMockhandler(ctrl)
			tt.expect(handler)

			require.Equal(t, tt.want, checkHandlerVersion(handler, tt.expectedVersion))
		})
	}
}

func TestCreateRoutes(t *testing.T) {
	t.Parallel()

	type routeCase struct {
		name    string
		prepare func(*apimocks.MockAuthService)
		check   func(*testing.T, *Server, *bytes.Buffer)
	}

	routes := []struct{ path, operation string }{
		{"/api/v0/resources/update", "fga.update_resource"},
		{"/api/v0/auth/notes/filter", "politics.filter_notes"},
	}
	outcomes := []struct {
		name    string
		stop    echo.HandlerFunc
		status  int
		failure string
	}{
		{"HTTP error", func(echo.Context) error { return echo.ErrForbidden }, http.StatusForbidden, "error_code=request_interrupted"},
		{"response without error", func(c echo.Context) error { return c.NoContent(http.StatusServiceUnavailable) }, http.StatusServiceUnavailable, "error_code=request_interrupted"},
		{"panic", func(echo.Context) error { panic("private panic") }, http.StatusInternalServerError, "panic:true"},
	}
	noAuth := func(*apimocks.MockAuthService) {}
	cases := make([]routeCase, 1, 1+len(routes)*(1+len(outcomes)))
	cases[0] = routeCase{"metrics", noAuth, checkMetricRoute}

	for _, route := range routes {
		cases = append(cases, routeCase{route.path + "/requires JWT", expectMissingToken, checkRouteRequiresJWT(route.path, route.operation)})
		for _, tc := range outcomes {
			cases = append(cases, routeCase{route.path + "/" + tc.name, noAuth, func(t *testing.T, s *Server, output *bytes.Buffer) {
				t.Helper()
				checkAuditAtEntry(t, s, output, route.path, route.operation, tc.stop, tc.status, tc.failure)
			}})
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer

			source := audit.NewAuditor(audit.WithOutput(&output), audit.WithHook(handlerV0.ConnectionHook))
			ctrl := gomock.NewController(t)
			auth := apimocks.NewMockAuthService(ctrl)
			tc.prepare(auth)
			mw, err := handlerV0.NewMiddlewareHandler(handlerV0.WithRequestAuditor(source), handlerV0.WithMiddlewareAuthService(auth))
			require.NoError(t, err)

			var s Server

			s.api.h0, s.api.middleware = mocks.NewMockhandler(ctrl), mw

			require.NoError(t, s.createRoutes())
			require.NotNil(t, s.e)
			tc.check(t, &s, &output)
		})
	}
}

func expectMissingToken(auth *apimocks.MockAuthService) {
	auth.EXPECT().CheckToken(gomock.Any(), "").Return(nil, echo.ErrUnauthorized)
}

func checkRouteRequiresJWT(path, operation string) func(*testing.T, *Server, *bytes.Buffer) {
	return func(t *testing.T, s *Server, output *bytes.Buffer) {
		t.Helper()

		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		s.e.ServeHTTP(rec, r)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Contains(t, output.String(), "operation="+operation)
		require.Contains(t, output.String(), "stage:authenticate")
	}
}

func checkMetricRoute(t *testing.T, s *Server, _ *bytes.Buffer) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
	s.e.ServeHTTP(httptest.NewRecorder(), req)
	rec := httptest.NewRecorder()
	s.e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "go_goroutines")
	require.Contains(t, rec.Body.String(), "promhttp_metric_handler_requests_total")
	require.Contains(t, rec.Body.String(), "webserver_requests_total")
}

func checkAuditAtEntry(t *testing.T, s *Server, output *bytes.Buffer, path, operation string, stop echo.HandlerFunc, status int, failure string) {
	t.Helper()

	s.e.Logger.SetOutput(io.Discard)
	s.e.Use(func(echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			require.Equal(t, 1, strings.Count(output.String(), "status=started"))
			require.Contains(t, output.String(), "trace_id=test-trace")
			require.NotContains(t, output.String(), "status=failed")

			return stop(c)
		}
	})

	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil)
	r.Header.Set("X-Trace-ID", "test-trace")

	rec := httptest.NewRecorder()
	s.e.ServeHTTP(rec, r)

	require.Equal(t, status, rec.Code)

	logs := output.String()
	require.Equal(t, 1, strings.Count(logs, "status=started"))
	require.Equal(t, 1, strings.Count(logs, "status=failed"))
	require.Contains(t, logs, "operation="+operation)
	require.Contains(t, logs, "stage:receive_request")
	require.Contains(t, logs, failure)
	require.NotContains(t, logs, "private panic")
	require.NotContains(t, logs, "status=completed")
	require.NotContains(t, logs, "response_write_failed")
}

func TestAuditMatchedRequests(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		method, path string
		wantAudit    bool
	}{
		{http.MethodPost, "/operation", true},
		{http.MethodGet, "/operation", false},
		{http.MethodPost, "/unrelated", false},
		{http.MethodDelete, "/group/*", true},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()

			started := false
			start := func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error {
					started = true
					return next(c)
				}
			}

			mw := auditMatchedRequests(map[*echo.Route]echo.MiddlewareFunc{
				{Method: http.MethodPost, Path: "/operation"}:  start,
				{Method: echo.RouteNotFound, Path: "/group/*"}: start,
			})

			c := echo.New().NewContext(httptest.NewRequestWithContext(t.Context(), tc.method, "/", nil), httptest.NewRecorder())
			c.SetPath(tc.path)
			err := mw(func(echo.Context) error { return io.ErrClosedPipe })(c)
			require.ErrorIs(t, err, io.ErrClosedPipe)

			require.Equal(t, tc.wantAudit, started)
		})
	}
}
