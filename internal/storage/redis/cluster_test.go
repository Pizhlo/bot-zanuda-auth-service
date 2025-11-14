package redis

import (
	"auth-service/internal/config"
	"reflect"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        *config.Redis
		want       *cluster
		checkCache func(t *testing.T, want *cluster, got *cluster)
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			cfg: &config.Redis{
				Type:  config.RedisTypeCluster,
				Addrs: []string{"localhost:6379"},
			},
			want: &cluster{
				cfg: &config.Redis{
					Type:  config.RedisTypeCluster,
					Addrs: []string{"localhost:6379"},
				},
			},
			checkCache: func(t *testing.T, want *cluster, got *cluster) {
				t.Helper()

				assert.NotNil(t, got.cache)

				c := reflect.TypeOf(got.cache)
				assert.Equal(t, reflect.TypeOf(&redis.ClusterClient{}), c)

				// Compare only the cfg field, not the cache field
				require.Equal(t, want.cfg, got.cfg)
			},
			wantErr: require.NoError,
		},
		{
			name: "negative case: cfg is nil",
			cfg:  nil,
			want: nil,
			checkCache: func(t *testing.T, want *cluster, got *cluster) {
				t.Helper()

				assert.Nil(t, got)
			},
			wantErr: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewClusterClient(tt.cfg)
			tt.wantErr(t, err)

			tt.checkCache(t, tt.want, got)
		})
	}
}
