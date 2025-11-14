package redis

import (
	"auth-service/internal/config"
	"auth-service/internal/service/redis/mocks"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    []Option
		want    *Service
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			opts: []Option{
				WithCfg(&config.Redis{
					Type: config.RedisTypeSingle,
				}),
			},
			want: &Service{
				cfg: &config.Redis{
					Type: config.RedisTypeSingle,
				},
			},
			wantErr: require.NoError,
		},
		{
			name:    "negative case: cfg is nil",
			opts:    []Option{},
			want:    nil,
			wantErr: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := New(tt.opts...)
			tt.wantErr(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}

//nolint:funlen // длинный тест - это ок
func TestStop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      []Option
		createSvc func(t *testing.T, mockRedisClient *mocks.MockredisClient) *Service
		wantErr   require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			createSvc: func(t *testing.T, mockRedisClient *mocks.MockredisClient) *Service {
				t.Helper()

				mockRedisClient.EXPECT().Close(t.Context()).Return(nil)

				return &Service{
					cfg: &config.Redis{
						Type: config.RedisTypeSingle,
					},
					client: mockRedisClient,
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "negative case: close error",
			createSvc: func(t *testing.T, mockRedisClient *mocks.MockredisClient) *Service {
				t.Helper()

				mockRedisClient.EXPECT().Close(t.Context()).Return(errors.New("close error"))

				return &Service{
					cfg: &config.Redis{
						Type: config.RedisTypeSingle,
					},
					client: mockRedisClient,
				}
			},
			wantErr: require.Error,
		},
		{
			name: "positive case: no connection",
			createSvc: func(t *testing.T, mockRedisClient *mocks.MockredisClient) *Service {
				t.Helper()

				return &Service{
					cfg: &config.Redis{
						Type: config.RedisTypeSingle,
					},
				}
			},
			wantErr: require.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRedisClient := mocks.NewMockredisClient(ctrl)

			svc := tt.createSvc(t, mockRedisClient)

			err := svc.Stop(t.Context())
			tt.wantErr(t, err)
		})
	}
}
