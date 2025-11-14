package redis

import (
	"auth-service/internal/config"
	"reflect"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        *config.Redis
		want       *client
		checkCache func(t *testing.T, want *client, got *client)
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			cfg: &config.Redis{
				Type: config.RedisTypeSingle,
				Host: "localhost",
				Port: 6379,
			},
			want: &client{
				cfg: &config.Redis{
					Type: config.RedisTypeSingle,
					Host: "localhost",
					Port: 6379,
				},
			},
			checkCache: func(t *testing.T, want *client, got *client) {
				t.Helper()

				assert.NotNil(t, got.cache)

				c := reflect.TypeOf(got.cache)
				assert.Equal(t, reflect.TypeOf(&redis.Client{}), c)

				// Compare only the cfg field, not the cache field
				require.Equal(t, want.cfg, got.cfg)
			},
			wantErr: require.NoError,
		},
		{
			name: "negative case: cfg is nil",
			cfg:  nil,
			want: nil,
			checkCache: func(t *testing.T, want *client, got *client) {
				t.Helper()

				assert.Nil(t, got)
			},
			wantErr: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewSingleClient(tt.cfg)
			tt.wantErr(t, err)

			tt.checkCache(t, tt.want, got)
		})
	}
}
