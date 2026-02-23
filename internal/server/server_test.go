package server

import (
	handlerV0 "auth-service/internal/api/v0"
	"auth-service/internal/server/mocks"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест
func TestNewServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		createOpts func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler) []Option
		createWant func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler) *Server
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
			createWant: func(t *testing.T, mockHandler *mocks.Mockhandler, mdHandler *mocks.MockmiddlewareHandler) *Server {
				t.Helper()

				return &Server{
					port:            8080,
					shutdownTimeout: 100 * time.Millisecond,
					api: struct {
						h0         handler
						middleware middlewareHandler
					}{
						h0:         mockHandler,
						middleware: mdHandler,
					},
				}
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
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
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
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
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
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
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
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
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
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
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

			if tt.createWant != nil {
				require.NotNil(t, server)
				assert.Equal(t, tt.createWant(t, handler, mdHandler), server)
			}
		})
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
