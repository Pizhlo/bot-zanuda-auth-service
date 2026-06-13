package v0

import (
	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"auth-service/pkg/audit"
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест - это ок
func TestVerifyPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		payload    jwt.MapClaims
		setupMocks func(t *testing.T, authSvc *mocks.MockAuthService)
		wantCode   int
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "valid payload",
			payload: jwt.MapClaims{
				"sub":   "123",
				"iss":   "test",
				"scope": "bot",
				"exp":   1875018533,
				"iat":   1717252133,
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockAuthService) {
				t.Helper()

				authSvc.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "bot",
					ClientName: "bot",
					IsActive:   true,
					Scopes:     []string{string(model.BotScope)},
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}, nil)
			},
			wantCode: 0,
			wantErr:  require.NoError,
		},
		{
			name: "internal error",
			payload: jwt.MapClaims{
				"sub":   "123",
				"iss":   "test",
				"scope": "bot",
				"exp":   1875018533,
				"iat":   1717252133,
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockAuthService) {
				t.Helper()

				authSvc.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{}, errors.New("internal server error"))
			},
			wantCode: http.StatusInternalServerError,
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(tt, err, "internal server error")
			},
		},
		{
			name: "inactive client",
			payload: jwt.MapClaims{
				"sub":   "123",
				"iss":   "test",
				"scope": "bot",
				"exp":   1875018533,
				"iat":   1717252133,
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockAuthService) {
				t.Helper()

				authSvc.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "bot",
					ClientName: "bot",
				}, nil)
			},
			wantCode: http.StatusUnauthorized,
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(tt, err, "client is inactive")
			},
		},
		{
			name: "invalid scope",
			payload: jwt.MapClaims{
				"sub":   "123",
				"iss":   "test",
				"scope": "invalid",
				"exp":   1875018533,
				"iat":   1717252133,
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockAuthService) {
				t.Helper()

				authSvc.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "bot",
					ClientName: "bot",
					IsActive:   true,
					Scopes:     []string{"another_scope"},
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}, nil)
			},
			wantCode: http.StatusUnauthorized,
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(tt, err, "client does not have required scope")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			authSvc := mocks.NewMockAuthService(ctrl)

			tt.setupMocks(t, authSvc)

			middlewareHandler, err := NewMiddlewareHandler(WithMiddlewareAuthService(authSvc))
			require.NoError(t, err)

			code, err := middlewareHandler.verifyPayload(context.Background(), tt.payload)
			require.Equal(t, tt.wantCode, code)
			tt.wantErr(t, err)
		})
	}
}

func TestConnectionHook(t *testing.T) {
	t.Parallel()

	t.Run("adds all connection fields to stash", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		ctx = withTraceID(ctx, "trace-1")
		ctx = withRequestID(ctx, "req-1")
		ctx = withIPAddress(ctx, "127.0.0.1")
		ctx = withUserAgent(ctx, "curl/8.0")
		ctx = withUserID(ctx, "42")

		stash := ConnectionHook(ctx, audit.Stash{})
		values := stashFieldsByName(stash)

		require.Equal(t, "trace-1", values["trace_id"])
		require.Equal(t, "req-1", values["request_id"])
		require.Equal(t, audit.EventContext{
			"ip_address": "127.0.0.1",
			"user_agent": "curl/8.0",
			"user_id":    "42",
		}, values["context"])
	})

	t.Run("does not add context when values are missing", func(t *testing.T) {
		t.Parallel()

		stash := ConnectionHook(context.Background(), audit.Stash{})
		values := stashFieldsByName(stash)

		require.NotContains(t, values, "context")
	})
}

func stashFieldsByName(stash audit.Stash) map[string]any {
	v := reflect.ValueOf(&stash).Elem()

	fields := v.FieldByName("fields")
	if !fields.IsValid() || fields.IsNil() {
		return map[string]any{}
	}

	fields = reflect.NewAt(fields.Type(), unsafe.Pointer(fields.UnsafeAddr())).Elem()

	result := make(map[string]any, fields.Len())

	iter := fields.MapRange()
	for iter.Next() {
		key := iter.Key().Interface()
		keyName := fmt.Sprint(key)

		fieldIface := iter.Value().Interface()

		valueField := reflect.ValueOf(fieldIface).FieldByName("Value")
		if valueField.IsValid() && valueField.CanInterface() {
			result[keyName] = valueField.Interface()
		}
	}

	return result
}
