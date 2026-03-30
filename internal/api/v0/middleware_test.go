package v0

import (
	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

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
