package auth

import (
	"auth-service/internal/model"
	db "auth-service/internal/storage"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

//nolint:funlen,dupl // длинный тест - ничего страшного; похожие тест-кейсы
func TestLogin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		req        model.LoginRequest
		setupMocks func(t *testing.T, m *mockMocks)
		checkWant  func(t *testing.T, resp model.LoginResponse)
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			req: model.LoginRequest{
				GrantType:    model.ClientCredentialsGrantType,
				ClientID:     "client_id",
				ClientSecret: "client_secret",
				Scope:        model.BotScope,
			},

			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockVaultClient.EXPECT().GetClientSecret(gomock.Any()).Return("client_secret", nil)

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "client_id",
					ClientName: "client_name",
					Scopes:     []string{string(model.BotScope)},
					IsActive:   true,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}, nil)
			},
			checkWant: func(t *testing.T, resp model.LoginResponse) {
				t.Helper()

				want := model.LoginResponse{
					TokenType: model.BearerTokenType,
					ExpiresIn: 1,
					Scope:     model.BotScope,
				}

				require.NotNil(t, resp.AccessToken)
				require.Equal(t, want.TokenType, resp.TokenType)
				require.Equal(t, want.ExpiresIn, resp.ExpiresIn)
				require.Equal(t, want.Scope, resp.Scope)
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: invalid grant type",
			req: model.LoginRequest{
				GrantType:    "invalid",
				ClientID:     "client_id",
				ClientSecret: "client_secret",
				Scope:        model.BotScope,
			},
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()
			},
			checkWant: func(t *testing.T, resp model.LoginResponse) {
				t.Helper()

				want := model.LoginResponse{}
				require.Equal(t, want, resp)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "invalid grant type")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := createTestAuthService(t, []byte("secret"), tt.setupMocks)
			resp, err := svc.Login(t.Context(), tt.req)

			tt.wantErr(t, err)
			tt.checkWant(t, resp)
		})
	}
}

//nolint:funlen,dupl // длинный тест - ничего страшного; похожие тест-кейсы
func TestLoginWithClientCredentials(t *testing.T) {
	t.Parallel()

	validReqWithScope := model.LoginRequest{
		GrantType:    model.ClientCredentialsGrantType,
		ClientID:     "client_id",
		ClientSecret: "client_secret",
		Scope:        model.BotScope,
	}

	validReqWithoutScope := model.LoginRequest{
		GrantType:    model.ClientCredentialsGrantType,
		ClientID:     "client_id",
		ClientSecret: "client_secret",
		Scope:        "",
	}

	invalidSecretReq := model.LoginRequest{
		GrantType:    model.ClientCredentialsGrantType,
		ClientID:     "client_id",
		ClientSecret: "invalid_client_secret",
		Scope:        model.BotScope,
	}

	invalidScopeReq := model.LoginRequest{
		GrantType:    model.ClientCredentialsGrantType,
		ClientID:     "client_id",
		ClientSecret: "client_secret",
		Scope:        "invalid_scope",
	}

	tests := []struct {
		name       string
		req        model.LoginRequest
		setupMocks func(t *testing.T, m *mockMocks)
		want       model.LoginResponse
		checkWant  func(t *testing.T, resp model.LoginResponse)
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case: with scope",
			req:  validReqWithScope,
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockVaultClient.EXPECT().GetClientSecret(gomock.Any()).Return("client_secret", nil)

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "client_id",
					ClientName: "client_name",
					Scopes:     []string{string(model.BotScope)},
					IsActive:   true,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}, nil)
			},
			want: model.LoginResponse{
				TokenType: model.BearerTokenType,
				ExpiresIn: 1,
				Scope:     model.BotScope,
			},
			checkWant: func(t *testing.T, resp model.LoginResponse) {
				t.Helper()

				require.Equal(t, model.BearerTokenType, resp.TokenType)
				require.Equal(t, 1, resp.ExpiresIn)
				require.Equal(t, model.BotScope, resp.Scope)
				require.NotNil(t, resp.AccessToken)
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: without scope",
			req:  validReqWithoutScope,
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockVaultClient.EXPECT().GetClientSecret(gomock.Any()).Return("client_secret", nil)

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "client_id",
					ClientName: "client_name",
					Scopes:     []string{string(model.BotScope)},
					IsActive:   true,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}, nil)
			},
			want: model.LoginResponse{
				TokenType: model.BearerTokenType,
				ExpiresIn: 1,
				Scope:     model.BotScope,
			},
			checkWant: func(t *testing.T, resp model.LoginResponse) {
				t.Helper()

				require.Equal(t, model.BearerTokenType, resp.TokenType)
				require.Equal(t, 1, resp.ExpiresIn)
				require.Equal(t, model.BotScope, resp.Scope)
				require.NotNil(t, resp.AccessToken)
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: error getting client secret",
			req:  invalidSecretReq,
			want: model.LoginResponse{},
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "client_id",
					ClientName: "client_name",
					Scopes:     []string{string(model.BotScope)},
					IsActive:   true,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}, nil)

				m.mockVaultClient.EXPECT().GetClientSecret(gomock.Any()).Return("", errors.New("invalid client secret"))
			},
			checkWant: func(t *testing.T, resp model.LoginResponse) {
				t.Helper()

				require.Equal(t, model.LoginResponse{}, resp)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "invalid client secret")
			},
		},
		{
			name: "error case: empty client secret",
			req:  invalidSecretReq,
			want: model.LoginResponse{},
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "client_id",
					ClientName: "client_name",
					Scopes:     []string{string(model.BotScope)},
					IsActive:   true,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}, nil)

				m.mockVaultClient.EXPECT().GetClientSecret(gomock.Any()).Return("", nil)
			},
			checkWant: func(t *testing.T, resp model.LoginResponse) {
				t.Helper()

				require.Equal(t, model.LoginResponse{}, resp)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidSecret)
			},
		},
		{
			name: "error case: inactive client",
			req:  validReqWithScope,
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "client_id",
					ClientName: "client_name",
					Scopes:     []string{string(model.BotScope)},
					IsActive:   false,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}, nil)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInactiveClient)
			},
			checkWant: func(t *testing.T, resp model.LoginResponse) {
				t.Helper()

				require.Equal(t, model.LoginResponse{}, resp)
			},
		},
		{
			name: "error case: invalid secret",
			req:  invalidSecretReq,
			want: model.LoginResponse{},
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "client_id",
					ClientName: "client_name",
					Scopes:     []string{string(model.BotScope)},
					IsActive:   true,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}, nil)

				m.mockVaultClient.EXPECT().GetClientSecret(gomock.Any()).Return("client_secret", nil)
			},
			checkWant: func(t *testing.T, resp model.LoginResponse) {
				t.Helper()

				require.Equal(t, model.LoginResponse{}, resp)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidSecret)
			},
		},
		{
			name: "error case: invalid scope",
			req:  invalidScopeReq,
			want: model.LoginResponse{},
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockVaultClient.EXPECT().GetClientSecret(gomock.Any()).Return("client_secret", nil)

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         uuid.New(),
					ClientID:   "client_id",
					ClientName: "client_name",
					Scopes:     []string{string(model.BotScope), "another_scope"},
					IsActive:   true,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}, nil)
			},
			checkWant: func(t *testing.T, resp model.LoginResponse) {
				t.Helper()

				require.Equal(t, model.LoginResponse{}, resp)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidScope)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := createTestAuthService(t, []byte("secret"), tt.setupMocks)

			resp, err := svc.loginWithClientCredentials(t.Context(), tt.req)
			tt.wantErr(t, err)
			tt.checkWant(t, resp)
		})
	}
}

//nolint:funlen // длинный тест - ничего страшного
func TestValidateClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		clientID   string
		setupMocks func(t *testing.T, m *mockMocks)
		check      func(t *testing.T, got *model.ServiceClient, err error)
	}{
		{
			name:     "returns client when active",
			clientID: "client_id",
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), "client_id").Return(model.ServiceClient{
					ClientID: "client_id",
					IsActive: true,
				}, nil)
			},
			check: func(t *testing.T, got *model.ServiceClient, err error) {
				t.Helper()

				require.NoError(t, err)
				require.NotNil(t, got)
				require.Equal(t, "client_id", got.ClientID)
				require.True(t, got.IsActive)
			},
		},
		{
			name:     "returns inactive client error",
			clientID: "client_id",
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), "client_id").Return(model.ServiceClient{
					ClientID: "client_id",
					IsActive: false,
				}, nil)
			},
			check: func(t *testing.T, got *model.ServiceClient, err error) {
				t.Helper()

				require.Nil(t, got)
				require.ErrorIs(t, err, ErrInactiveClient)
			},
		},
		{
			name:     "returns storage error",
			clientID: "client_id",
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), "client_id").Return(model.ServiceClient{}, db.ErrNotFound)
			},
			check: func(t *testing.T, got *model.ServiceClient, err error) {
				t.Helper()

				require.Nil(t, got)
				require.ErrorIs(t, err, db.ErrNotFound)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := createTestAuthService(t, []byte("secret"), tt.setupMocks)

			got, err := svc.validateClient(t.Context(), tt.clientID)
			tt.check(t, got, err)
		})
	}
}

//nolint:funlen // длинный тест - ничего страшного
func TestValidateSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		clientID     string
		clientSecret string
		setupMocks   func(t *testing.T, m *mockMocks)
		wantErr      require.ErrorAssertionFunc
	}{
		{
			name:         "returns nil when secret is valid",
			clientID:     "client_id",
			clientSecret: "client_secret",
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockVaultClient.EXPECT().GetClientSecret("client_id").Return("client_secret", nil)
			},
			wantErr: require.NoError,
		},
		{
			name:         "returns not found when vault returns not found",
			clientID:     "client_id",
			clientSecret: "client_secret",
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockVaultClient.EXPECT().GetClientSecret("client_id").Return("", db.ErrNotFound)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrClientSecretNotFound)
			},
		},
		{
			name:         "returns not found when vault secret is empty",
			clientID:     "client_id",
			clientSecret: "client_secret",
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockVaultClient.EXPECT().GetClientSecret("client_id").Return("", nil)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrClientSecretNotFound)
			},
		},
		{
			name:         "returns invalid secret when secret mismatch",
			clientID:     "client_id",
			clientSecret: "invalid_secret",
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockVaultClient.EXPECT().GetClientSecret("client_id").Return("client_secret", nil)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidSecret)
			},
		},
		{
			name:         "returns vault error for unexpected error",
			clientID:     "client_id",
			clientSecret: "client_secret",
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockVaultClient.EXPECT().GetClientSecret("client_id").Return("", errors.New("vault unavailable"))
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "vault unavailable")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := createTestAuthService(t, []byte("secret"), tt.setupMocks)

			err := svc.validateSecret(tt.clientID, tt.clientSecret)
			tt.wantErr(t, err)
		})
	}
}

//nolint:funlen // длинный тест - ничего страшного; похожие тест-кейсы
func TestValidateScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		allowed      []string
		requestedStr string
		wantScopes   []string
		wantErr      require.ErrorAssertionFunc
	}{
		{
			name:         "returns requested scopes when all allowed",
			allowed:      []string{"bot", "notes:read"},
			requestedStr: "bot notes:read",
			wantScopes:   []string{"bot", "notes:read"},
			wantErr:      require.NoError,
		},
		{
			name:         "returns all allowed when requested is empty",
			allowed:      []string{"bot", "notes:read"},
			requestedStr: "",
			wantScopes:   []string{"bot", "notes:read"},
			wantErr:      require.NoError,
		},
		{
			name:         "returns all allowed when requested has only spaces",
			allowed:      []string{"bot", "notes:read"},
			requestedStr: "   \t  ",
			wantScopes:   []string{"bot", "notes:read"},
			wantErr:      require.NoError,
		},
		{
			name:         "accepts scopes case-insensitively",
			allowed:      []string{"BoT", "Notes:Read"},
			requestedStr: "bot notes:read",
			wantScopes:   []string{"bot", "notes:read"},
			wantErr:      require.NoError,
		},
		{
			name:         "returns error when scope is not allowed",
			allowed:      []string{"bot"},
			requestedStr: "bot admin",
			wantScopes:   nil,
			wantErr:      require.Error,
		},
		{
			name:         "returns requested scopes preserving order",
			allowed:      []string{"bot", "notes:read", "profile"},
			requestedStr: "profile bot",
			wantScopes:   []string{"profile", "bot"},
			wantErr:      require.NoError,
		},
		{
			name:         "returns duplicated scopes as requested",
			allowed:      []string{"bot"},
			requestedStr: "bot bot", //nolint:dupword // тест-кейс
			wantScopes:   []string{"bot", "bot"},
			wantErr:      require.NoError,
		},
		{
			name:         "returns error for unknown scope with mixed case",
			allowed:      []string{"BoT"},
			requestedStr: "bot Admin",
			wantScopes:   nil,
			wantErr:      require.Error,
		},
		{
			name:         "returns empty allowed list when both are empty",
			allowed:      []string{},
			requestedStr: "",
			wantScopes:   []string{},
			wantErr:      require.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateScopes(tt.allowed, tt.requestedStr)

			tt.wantErr(t, err)
			require.Equal(t, tt.wantScopes, got)
		})
	}
}

func TestValidateAndGetScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		clientScopes []string
		reqScope     string
		wantScopes   []string
		wantErr      require.ErrorAssertionFunc
	}{
		{
			name:         "returns all client scopes when reqScope is empty",
			clientScopes: []string{"bot", "notes:read"},
			reqScope:     "",
			wantScopes:   []string{"bot", "notes:read"},
			wantErr:      require.NoError,
		},
		{
			name:         "returns requested scopes when all are allowed",
			clientScopes: []string{"bot", "notes:read", "profile"},
			reqScope:     "profile bot",
			wantScopes:   []string{"profile", "bot"},
			wantErr:      require.NoError,
		},
		{
			name:         "returns invalid scope error when requested scope is not allowed",
			clientScopes: []string{"bot"},
			reqScope:     "admin",
			wantScopes:   nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidScope)
			},
		},
		{
			name:         "returns invalid scope error when one of requested scopes is not allowed",
			clientScopes: []string{"bot", "notes:read"},
			reqScope:     "bot admin",
			wantScopes:   nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidScope)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := createTestAuthService(t, []byte("secret"), nil)

			got, err := svc.validateAndGetScopes(tt.clientScopes, tt.reqScope)

			tt.wantErr(t, err)
			require.Equal(t, tt.wantScopes, got)
		})
	}
}

func TestHasScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		allowed []string
		scope   string
		want    bool
	}{
		{
			name:    "returns true for exact match",
			allowed: []string{"bot", "notes:read"},
			scope:   "bot",
			want:    true,
		},
		{
			name:    "returns true for case-insensitive match",
			allowed: []string{"BoT"},
			scope:   "bot",
			want:    true,
		},
		{
			name:    "returns false for absent scope",
			allowed: []string{"bot"},
			scope:   "admin",
			want:    false,
		},
		{
			name:    "returns false for empty allowed list",
			allowed: nil,
			scope:   "bot",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := hasScope(tt.allowed, tt.scope)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []string
		wantNil bool
	}{
		{
			name:    "returns nil for empty input",
			input:   "",
			want:    nil,
			wantNil: true,
		},
		{
			name:    "splits by single spaces",
			input:   "bot notes:read",
			want:    []string{"bot", "notes:read"},
			wantNil: false,
		},
		{
			name:    "splits by multiple spaces and tabs",
			input:   "  bot\t\tnotes:read   profile ",
			want:    []string{"bot", "notes:read", "profile"},
			wantNil: false,
		},
		{
			name:    "returns empty slice for whitespace-only input",
			input:   "   \t  ",
			want:    []string{},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseScopes(tt.input)

			if tt.wantNil {
				require.Nil(t, got)
				return
			}

			require.Equal(t, tt.want, got)
		})
	}
}
