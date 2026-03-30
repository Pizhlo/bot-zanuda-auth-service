package v0

import (
	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"auth-service/internal/service/auth"
	"auth-service/internal/storage"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

//nolint:dupl // дублирование, т.к. схожи тест-кейсы для разных хендлеров
func TestNewAuthHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      func(t *testing.T, ctrl *gomock.Controller) []authHandlerOption
		checkWant func(t *testing.T, authHandler *AuthHandler)
		wantErr   require.ErrorAssertionFunc
	}{
		{
			name: "success",
			opts: func(t *testing.T, ctrl *gomock.Controller) []authHandlerOption {
				t.Helper()

				return []authHandlerOption{
					WithAuthService(mocks.NewMockauthSerivce(ctrl)),
				}
			},
			checkWant: func(t *testing.T, authHandler *AuthHandler) {
				t.Helper()

				require.NotNil(t, authHandler)
				require.NotNil(t, authHandler.authSerivce)
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: auth service is required",
			opts: func(t *testing.T, ctrl *gomock.Controller) []authHandlerOption {
				t.Helper()

				return []authHandlerOption{}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "auth service is required")
			},
			checkWant: func(t *testing.T, authHandler *AuthHandler) {
				t.Helper()

				require.Nil(t, authHandler)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			opts := tt.opts(t, ctrl)
			authHandler, err := NewAuthHandler(opts...)
			tt.wantErr(t, err)

			tt.checkWant(t, authHandler)
		})
	}
}

//nolint:funlen,dupl // длинный тест - ничего страшного; похожие тест-кейсы
func TestAuthHandler_Login(t *testing.T) {
	t.Parallel()

	type wantResponse struct {
		status int
		errMsg string
		resp   *model.LoginResponse
	}

	tests := []struct {
		name       string
		body       model.LoginRequest
		checkWant  func(actual wantResponse)
		setupMocks func(t *testing.T, authSvc *mocks.MockauthSerivce)
	}{
		{
			name: "success",
			body: model.LoginRequest{
				GrantType:    model.ClientCredentialsGrantType,
				ClientID:     "client_id",
				ClientSecret: "client_secret",
				Scope:        model.BotScope,
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockauthSerivce) {
				t.Helper()

				authSvc.EXPECT().Login(gomock.Any(), gomock.Any()).Return(model.LoginResponse{
					AccessToken: "access_token",
					TokenType:   model.BearerTokenType,
					ExpiresIn:   1000,
					Scope:       model.BotScope,
				}, nil)
			},
			checkWant: func(actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusOK, actual.status)
				require.Equal(t, "access_token", actual.resp.AccessToken)
				require.Equal(t, model.BearerTokenType, actual.resp.TokenType)
				require.Equal(t, 1000, actual.resp.ExpiresIn)
			},
		},
		{
			name: "error case: invalid grant type",
			body: model.LoginRequest{
				GrantType: "invalid_grant_type",
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockauthSerivce) {
				t.Helper()

				authSvc.EXPECT().Login(gomock.Any(), gomock.Any()).Return(model.LoginResponse{}, auth.ErrInvalidGrantType)
			},
			checkWant: func(actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusBadRequest, actual.status)
				require.Equal(t, "invalid grant type", actual.errMsg)
			},
		},
		{
			name: "error case: invalid client",
			body: model.LoginRequest{
				GrantType:    model.ClientCredentialsGrantType,
				ClientID:     "invalid_client_id",
				ClientSecret: "client_secret",
				Scope:        model.BotScope,
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockauthSerivce) {
				t.Helper()

				authSvc.EXPECT().Login(gomock.Any(), gomock.Any()).Return(model.LoginResponse{}, storage.ErrNotFound)
			},
			checkWant: func(actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusUnauthorized, actual.status)
				require.Equal(t, "invalid client", actual.errMsg)
			},
		},
		{
			name: "error case: inactive client",
			body: model.LoginRequest{
				GrantType:    model.ClientCredentialsGrantType,
				ClientID:     "client_id",
				ClientSecret: "client_secret",
				Scope:        model.BotScope,
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockauthSerivce) {
				t.Helper()

				authSvc.EXPECT().Login(gomock.Any(), gomock.Any()).Return(model.LoginResponse{}, auth.ErrInactiveClient)
			},
			checkWant: func(actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusUnauthorized, actual.status)
				require.Equal(t, "client is inactive", actual.errMsg)
			},
		},
		{
			name: "error case: invalid client secret",
			body: model.LoginRequest{
				GrantType:    model.ClientCredentialsGrantType,
				ClientID:     "client_id",
				ClientSecret: "invalid_client_secret",
				Scope:        model.BotScope,
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockauthSerivce) {
				t.Helper()

				authSvc.EXPECT().Login(gomock.Any(), gomock.Any()).Return(model.LoginResponse{}, auth.ErrInvalidSecret)
			},
			checkWant: func(actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusUnauthorized, actual.status)
				require.Equal(t, "invalid client secret", actual.errMsg)
			},
		},
		{
			name: "error case: invalid scope",
			body: model.LoginRequest{
				GrantType:    model.ClientCredentialsGrantType,
				ClientID:     "client_id",
				ClientSecret: "client_secret",
				Scope:        "invalid_scope",
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockauthSerivce) {
				t.Helper()

				authSvc.EXPECT().Login(gomock.Any(), gomock.Any()).Return(model.LoginResponse{}, auth.ErrInvalidScope)
			},
			checkWant: func(actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusForbidden, actual.status)
				require.Equal(t, "invalid scope", actual.errMsg)
			},
		},
		{
			name: "error case: internal server error",
			body: model.LoginRequest{
				GrantType:    model.ClientCredentialsGrantType,
				ClientID:     "client_id",
				ClientSecret: "client_secret",
				Scope:        model.BotScope,
			},
			setupMocks: func(t *testing.T, authSvc *mocks.MockauthSerivce) {
				t.Helper()

				authSvc.EXPECT().Login(gomock.Any(), gomock.Any()).Return(model.LoginResponse{}, errors.New("internal server error"))
			},
			checkWant: func(actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusInternalServerError, actual.status)
				require.Equal(t, "internal server error", actual.errMsg)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			authSvc := mocks.NewMockauthSerivce(ctrl)
			tt.setupMocks(t, authSvc)

			authHandler, err := NewAuthHandler(WithAuthService(authSvc))
			require.NoError(t, err)

			e := echo.New()

			bodyBytes, err := json.Marshal(tt.body) //nolint:gosec // это тест
			require.NoError(t, err)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/login", bytes.NewReader(bodyBytes))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

			ctx := withUserID(req.Context(), uuid.New().String())

			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err = authHandler.Login(c)
			require.NoError(t, err)

			body := rec.Body.Bytes()

			// Пытаемся распарсить ошибку (если она есть).
			var errResp map[string]string

			_ = json.Unmarshal(body, &errResp)

			resp := &model.LoginResponse{}

			err = json.Unmarshal(body, &resp)
			require.NoError(t, err)

			actual := wantResponse{
				status: rec.Code,
				errMsg: errResp["error"],
				resp:   resp,
			}

			tt.checkWant(actual)
		})
	}
}
