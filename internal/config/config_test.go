package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configFile    string
		want          *Config
		wantErr       require.ErrorAssertionFunc
		operationsErr require.ErrorAssertionFunc
	}{
		{
			name:       "valid config",
			configFile: "testdata/valid.yaml",
			want: &Config{
				LogLevel: "debug",
				Server: Server{
					Port:            8080,
					ShutdownTimeout: 100 * time.Millisecond,
				},
				Vault: Vault{
					Address: "https://localhost:8200",
					Token:   "vault-token",
				},
				Redis: Redis{
					Type: RedisTypeSingle,
					Host: "localhost",
					Port: 6379,
				},
			},
			wantErr: require.NoError,
		},
		{
			name:       "invalid config",
			configFile: "testdata/invalid.yaml",
			wantErr:    require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := LoadConfig(tt.configFile)
			tt.wantErr(t, err)

			if tt.want != nil {
				require.Equal(t, tt.want, cfg)
			}
		})
	}
}

//nolint:funlen // это тест
func TestValidateRedisConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *Config
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "valid config: single node",
			cfg: &Config{
				Redis: Redis{
					Type: RedisTypeSingle,
					Host: "localhost",
					Port: 6379,
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "valid config: cluster node",
			cfg: &Config{
				Redis: Redis{
					Type:  RedisTypeCluster,
					Addrs: []string{"localhost:6379"},
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "invalid config: single node with addrs",
			cfg: &Config{
				Redis: Redis{
					Type:  RedisTypeSingle,
					Host:  "localhost",
					Port:  6379,
					Addrs: []string{"localhost:6379"},
				},
			},
			wantErr: require.Error,
		},
		{
			name: "invalid config: single node without host and port",
			cfg: &Config{
				Redis: Redis{
					Type: RedisTypeSingle,
				},
			},
			wantErr: require.Error,
		},
		{
			name: "invalid config: cluster node with host and port",
			cfg: &Config{
				Redis: Redis{
					Type:  RedisTypeCluster,
					Host:  "localhost",
					Port:  6379,
					Addrs: []string{"localhost:6379"},
				},
			},
			wantErr: require.Error,
		},
		{
			name: "invalid config: cluster node without addrs",
			cfg: &Config{
				Redis: Redis{
					Type: RedisTypeCluster,
				},
			},
			wantErr: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.validateRedisConfig()
			tt.wantErr(t, err)
		})
	}
}

func TestValidateRedisSingleConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *Config
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "valid config",
			cfg: &Config{
				Redis: Redis{
					Type: RedisTypeSingle,
					Host: "localhost",
					Port: 6379,
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "invalid config: single node with addrs",
			cfg: &Config{
				Redis: Redis{
					Type:  RedisTypeSingle,
					Host:  "localhost",
					Port:  6379,
					Addrs: []string{"localhost:6379"},
				},
			},
			wantErr: require.Error,
		},
		{
			name: "invalid config: single node without host and port",
			cfg: &Config{
				Redis: Redis{
					Type: RedisTypeSingle,
				},
			},
			wantErr: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateRedisSingleConfig(&tt.cfg.Redis)
			tt.wantErr(t, err)
		})
	}
}

func TestValidateRedisClusterConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *Config
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "valid config",
			cfg: &Config{
				Redis: Redis{
					Type:  RedisTypeCluster,
					Addrs: []string{"localhost:6379"},
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "invalid config: cluster node with host and port",
			cfg: &Config{
				Redis: Redis{
					Type:  RedisTypeCluster,
					Host:  "localhost",
					Port:  6379,
					Addrs: []string{"localhost:6379"},
				},
			},
			wantErr: require.Error,
		},
		{
			name: "invalid config: cluster node without addrs",
			cfg: &Config{
				Redis: Redis{
					Type: RedisTypeCluster,
				},
			},
			wantErr: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateRedisClusterConfig(&tt.cfg.Redis)
			tt.wantErr(t, err)
		})
	}
}
