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
		createOpts func(t *testing.T, mockVaultClient *mocks.MockvaultClient) []option
		createWant func(t *testing.T, mockVaultClient *mocks.MockvaultClient) *service
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			createOpts: func(t *testing.T, mockVaultClient *mocks.MockvaultClient) []option {
				t.Helper()

				return []option{
					WithUpdateKeyInterval(1 * time.Second),
					WithVaultClient(mockVaultClient),
				}
			},
			createWant: func(t *testing.T, mockVaultClient *mocks.MockvaultClient) *service {
				t.Helper()

				return &service{
					updateKeyInterval: 1 * time.Second,
					vaultClient:       mockVaultClient,
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: update key interval is required",
			createOpts: func(t *testing.T, mockVaultClient *mocks.MockvaultClient) []option {
				t.Helper()

				return []option{
					WithVaultClient(mockVaultClient),
				}
			},
			createWant: func(t *testing.T, mockVaultClient *mocks.MockvaultClient) *service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "update key interval is required")
			},
		},
		{
			name: "error case: vault client is required",
			createOpts: func(t *testing.T, mockVaultClient *mocks.MockvaultClient) []option {
				t.Helper()

				return []option{
					WithUpdateKeyInterval(1 * time.Second),
				}
			},
			createWant: func(t *testing.T, mockVaultClient *mocks.MockvaultClient) *service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "vault client is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockVaultClient := mocks.NewMockvaultClient(ctrl)

			got, err := New(tt.createOpts(t, mockVaultClient)...)
			tt.wantErr(t, err)

			assert.Equal(t, tt.createWant(t, mockVaultClient), got)
		})
	}
}
