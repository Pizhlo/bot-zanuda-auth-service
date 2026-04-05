package auth

import (
	"auth-service/internal/model"
	"auth-service/internal/service/auth/mocks"
	"auth-service/pkg/audit/testaudit"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест - это ок
func TestNewService(t *testing.T) {
	t.Parallel()

	type mockMocks struct {
		mockVaultClient *mocks.MockvaultClient
		mockStorage     *mocks.Mockstorage
		mockAuditor     auditor
	}

	tests := []struct {
		name       string
		createOpts func(t *testing.T, m *mockMocks) []option
		createWant func(t *testing.T, m *mockMocks) *Service
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			createOpts: func(t *testing.T, m *mockMocks) []option {
				t.Helper()

				return []option{
					WithUpdateKeyInterval(1 * time.Second),
					WithVaultClient(m.mockVaultClient),
					WithSecretKey([]byte("abc")),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
					WithAuditor(m.mockAuditor),
					WithStorage(m.mockStorage),
				}
			},
			createWant: func(t *testing.T, m *mockMocks) *Service {
				t.Helper()

				return &Service{
					updateKeyInterval: 1 * time.Second,
					vaultClient:       m.mockVaultClient,
					secretKey:         []byte("abc"),
					tokenDuration:     1 * time.Second,
					storage:           m.mockStorage,
					issuer:            "test",
					auditor:           m.mockAuditor,
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: update key interval must be greater than 0",
			createOpts: func(t *testing.T, m *mockMocks) []option {
				t.Helper()

				return []option{
					WithSecretKey([]byte("abc")),
					WithVaultClient(m.mockVaultClient),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
					WithStorage(m.mockStorage),
					WithAuditor(m.mockAuditor),
				}
			},
			createWant: func(t *testing.T, m *mockMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "update key interval must be greater than 0")
			},
		},
		{
			name: "error case: vault client is required",
			createOpts: func(t *testing.T, m *mockMocks) []option {
				t.Helper()

				return []option{
					WithSecretKey([]byte("abc")),
					WithUpdateKeyInterval(1 * time.Second),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
					WithStorage(m.mockStorage),
					WithAuditor(m.mockAuditor),
				}
			},
			createWant: func(t *testing.T, m *mockMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "vault client is required")
			},
		},
		{
			name: "error case: secret key is required",
			createOpts: func(t *testing.T, m *mockMocks) []option {
				t.Helper()

				return []option{
					WithVaultClient(m.mockVaultClient),
					WithUpdateKeyInterval(1 * time.Second),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
					WithStorage(m.mockStorage),
					WithAuditor(m.mockAuditor),
				}
			},
			createWant: func(t *testing.T, m *mockMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "secret key is required")
			},
		},
		{
			name: "error case: issuer is required",
			createOpts: func(t *testing.T, m *mockMocks) []option {
				t.Helper()

				return []option{
					WithSecretKey([]byte("abc")),
					WithUpdateKeyInterval(1 * time.Second),
					WithVaultClient(m.mockVaultClient),
					WithTokenDuration(1 * time.Second),
					WithStorage(m.mockStorage),
					WithAuditor(m.mockAuditor),
				}
			},
			createWant: func(t *testing.T, m *mockMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "issuer is required")
			},
		},
		{
			name: "error case: token duration must be greater than 0",
			createOpts: func(t *testing.T, m *mockMocks) []option {
				t.Helper()

				return []option{
					WithSecretKey([]byte("abc")),
					WithUpdateKeyInterval(1 * time.Second),
					WithVaultClient(m.mockVaultClient),
					WithIssuer("test"),
					WithStorage(m.mockStorage),
					WithAuditor(m.mockAuditor),
				}
			},
			createWant: func(t *testing.T, m *mockMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "token duration must be greater than 0")
			},
		},
		{
			name: "error case: storage is required",
			createOpts: func(t *testing.T, m *mockMocks) []option {
				t.Helper()

				return []option{
					WithSecretKey([]byte("abc")),
					WithUpdateKeyInterval(1 * time.Second),
					WithVaultClient(m.mockVaultClient),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
					WithAuditor(m.mockAuditor),
				}
			},
			createWant: func(t *testing.T, m *mockMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "storage is required")
			},
		},
		{
			name: "error case: auditor is required",
			createOpts: func(t *testing.T, m *mockMocks) []option {
				t.Helper()

				return []option{
					WithUpdateKeyInterval(1 * time.Second),
					WithVaultClient(m.mockVaultClient),
					WithSecretKey([]byte("abc")),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
					WithStorage(m.mockStorage),
				}
			},
			createWant: func(t *testing.T, m *mockMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "auditor is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			m := &mockMocks{
				mockVaultClient: mocks.NewMockvaultClient(ctrl),
				mockStorage:     mocks.NewMockstorage(ctrl),
				mockAuditor:     testaudit.NewAuditor(t),
			}

			got, err := New(tt.createOpts(t, m)...)
			tt.wantErr(t, err)

			assert.Equal(t, tt.createWant(t, m), got)
		})
	}
}

func TestGetIssuer(t *testing.T) {
	t.Parallel()

	svc := createTestAuthService(t, []byte("abc"), nil)
	require.Equal(t, "test", svc.GetIssuer())
}

//nolint:funlen // длинный тест - это ок
func TestGetServiceClient(t *testing.T) {
	t.Parallel()

	clientID := "test"
	id := uuid.New()

	now := time.Now()

	tests := []struct {
		name       string
		setupMocks func(t *testing.T, m *mockMocks)
		want       model.ServiceClient
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{
					ID:         id,
					ClientID:   "test",
					ClientName: "test",
					Scopes:     []string{string(model.BotScope)},
					IsActive:   true,
					CreatedAt:  now,
					UpdatedAt:  now,
				}, nil)
			},
			want: model.ServiceClient{
				ID:         id,
				ClientID:   "test",
				ClientName: "test",
				Scopes:     []string{string(model.BotScope)},
				IsActive:   true,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
			wantErr: require.NoError,
		},
		{
			name: "error case",
			setupMocks: func(t *testing.T, m *mockMocks) {
				t.Helper()

				m.mockStorage.EXPECT().GetServiceClient(gomock.Any(), gomock.Any()).Return(model.ServiceClient{}, errors.New("error"))
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := createTestAuthService(t, []byte("abc"), tt.setupMocks)
			got, err := svc.GetServiceClient(t.Context(), clientID)
			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
