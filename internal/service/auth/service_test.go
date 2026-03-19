package auth

import (
	"auth-service/internal/service/auth/mocks"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест - это ок
func TestNewService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		createOpts func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) []option
		createWant func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) *Service
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			createOpts: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) []option {
				t.Helper()

				return []option{
					WithUpdateKeyInterval(1 * time.Second),
					WithVaultClient(mockVaultClient),
					WithSecretKey([]byte("abc")),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
					WithStorage(mockStorage),
				}
			},
			createWant: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) *Service {
				t.Helper()

				return &Service{
					updateKeyInterval: 1 * time.Second,
					vaultClient:       mockVaultClient,
					secretKey:         []byte("abc"),
					tokenDuration:     1 * time.Second,
					storage:           mockStorage,
					issuer:            "test",
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: update key interval must be greater than 0",
			createOpts: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) []option {
				t.Helper()

				return []option{
					WithSecretKey([]byte("abc")),
					WithVaultClient(mockVaultClient),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
					WithStorage(mockStorage),
				}
			},
			createWant: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) *Service {
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
			createOpts: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) []option {
				t.Helper()

				return []option{
					WithSecretKey([]byte("abc")),
					WithUpdateKeyInterval(1 * time.Second),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
					WithStorage(mockStorage),
				}
			},
			createWant: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) *Service {
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
			createOpts: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) []option {
				t.Helper()

				return []option{
					WithVaultClient(mockVaultClient),
					WithUpdateKeyInterval(1 * time.Second),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
					WithStorage(mockStorage),
				}
			},
			createWant: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) *Service {
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
			createOpts: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) []option {
				t.Helper()

				return []option{
					WithSecretKey([]byte("abc")),
					WithUpdateKeyInterval(1 * time.Second),
					WithVaultClient(mockVaultClient),
					WithTokenDuration(1 * time.Second),
					WithStorage(mockStorage),
				}
			},
			createWant: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) *Service {
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
			createOpts: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) []option {
				t.Helper()

				return []option{
					WithSecretKey([]byte("abc")),
					WithUpdateKeyInterval(1 * time.Second),
					WithVaultClient(mockVaultClient),
					WithIssuer("test"),
					WithStorage(mockStorage),
				}
			},
			createWant: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) *Service {
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
			createOpts: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) []option {
				t.Helper()

				return []option{
					WithSecretKey([]byte("abc")),
					WithUpdateKeyInterval(1 * time.Second),
					WithVaultClient(mockVaultClient),
					WithIssuer("test"),
					WithTokenDuration(1 * time.Second),
				}
			},
			createWant: func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "storage is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockVaultClient := mocks.NewMockvaultClient(ctrl)

			mockStorage := mocks.NewMockstorage(ctrl)

			got, err := New(tt.createOpts(t, mockVaultClient, mockStorage)...)
			tt.wantErr(t, err)

			assert.Equal(t, tt.createWant(t, mockVaultClient, mockStorage), got)
		})
	}
}
