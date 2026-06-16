package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест
func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configFile string
		want       *Config
		wantErr    require.ErrorAssertionFunc
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
				Audit: AuditConfig{
					BrokerEnabled: true,
					Topic:         "errors.auth-service",
					Kinds: struct {
						Include []string `yaml:"include"`
						Exclude []string `yaml:"exclude"`
					}{
						Include: []string{"infra", "internal", "external"},
						Exclude: []string{},
					},
					Levels: struct {
						Include []string `yaml:"include"`
						Exclude []string `yaml:"exclude"`
					}{
						Include: []string{"error", "fatal"},
						Exclude: []string{},
					},
				},
				RabbitMQ: RabbitMQ{
					URL:            "amqp://localhost:5672/",
					ConnectTimeout: 5 * time.Second,
					PublishTimeout: 2 * time.Second,
					MaxRetries:     3,
					RetryBackoff:   100 * time.Millisecond,
				},
				Vault: Vault{
					Address:     "https://localhost:8200",
					Token:       "vault-token",
					SecretsPath: "secret/data",
				},
				Redis: Redis{
					Type: RedisTypeSingle,
					Host: "localhost",
					Port: 6379,
				},
				Postgres: Postgres{
					Host:          "localhost",
					Port:          5432,
					User:          "user",
					Password:      "pass",
					DBName:        "db",
					InsertTimeout: 5 * time.Second,
					ReadTimeout:   5 * time.Second,
				},
				OpenFGA: OpenFGA{
					APIURL:             "http://localhost:1234",
					AuthorizationModel: "testpath/testfile.fga",
					StoreName:          "notes-bot",
					ApplyModelOnStart:  false,
				},
				Auth: Auth{
					SecretKey:         "your-key",
					UpdateKeyInterval: 1 * time.Hour,
					Issuer:            "test",
					TokenDuration:     1 * time.Hour,
					UserCacheTTL:      1 * time.Hour,
				},
				Policy: Policy{
					Config: "casbin_model.conf",
				},
			},
			wantErr: require.NoError,
		},
		{
			name:       "valid config: audit disabled",
			configFile: "testdata/valid_audit_disabled.yaml",
			want: &Config{
				LogLevel: "debug",
				Server: Server{
					Port:            8080,
					ShutdownTimeout: 100 * time.Millisecond,
				},
				OpenFGA: OpenFGA{
					APIURL:             "http://localhost:1234",
					AuthorizationModel: "testpath/testfile.fga",
					StoreName:          "notes-bot",
					ApplyModelOnStart:  false,
				},
				Audit: AuditConfig{
					BrokerEnabled: false,
					Topic:         "errors.auth-service",
					Kinds: struct {
						Include []string `yaml:"include"`
						Exclude []string `yaml:"exclude"`
					}{
						Include: []string{"infra", "internal", "external"},
						Exclude: []string{},
					},
					Levels: struct {
						Include []string `yaml:"include"`
						Exclude []string `yaml:"exclude"`
					}{
						Include: []string{"error", "fatal"},
						Exclude: []string{},
					},
				},
				RabbitMQ: RabbitMQ{
					URL:            "amqp://localhost:5672/",
					ConnectTimeout: 5 * time.Second,
					PublishTimeout: 2 * time.Second,
					MaxRetries:     3,
					RetryBackoff:   100 * time.Millisecond,
				},
				Vault: Vault{
					Address:     "https://localhost:8200",
					Token:       "vault-token",
					SecretsPath: "secret/data",
				},
				Redis: Redis{
					Type: RedisTypeSingle,
					Host: "localhost",
					Port: 6379,
				},
				Postgres: Postgres{
					Host:          "localhost",
					Port:          5432,
					User:          "user",
					Password:      "pass",
					DBName:        "db",
					InsertTimeout: 5 * time.Second,
					ReadTimeout:   5 * time.Second,
				},
				Auth: Auth{
					SecretKey:         "your-key",
					UpdateKeyInterval: 1 * time.Hour,
					Issuer:            "test",
					TokenDuration:     1 * time.Hour,
					UserCacheTTL:      1 * time.Hour,
				},
				Policy: Policy{
					Config: "casbin_model.conf",
				},
			},
			wantErr: require.NoError,
		},
		{
			name:       "valid config: redis cluster",
			configFile: "testdata/valid_redis_cluster.yaml",
			want: &Config{
				LogLevel: "debug",
				Server: Server{
					Port:            8080,
					ShutdownTimeout: 100 * time.Millisecond,
				},
				OpenFGA: OpenFGA{
					APIURL:             "http://localhost:1234",
					AuthorizationModel: "testpath/testfile.fga",
					StoreName:          "notes-bot",
					ApplyModelOnStart:  false,
				},
				Audit: AuditConfig{
					BrokerEnabled: true,
					Topic:         "errors.auth-service",
					Kinds: struct {
						Include []string `yaml:"include"`
						Exclude []string `yaml:"exclude"`
					}{
						Include: []string{"infra", "internal", "external"},
						Exclude: []string{},
					},
					Levels: struct {
						Include []string `yaml:"include"`
						Exclude []string `yaml:"exclude"`
					}{
						Include: []string{"error", "fatal"},
						Exclude: []string{},
					},
				},
				RabbitMQ: RabbitMQ{
					URL:            "amqp://localhost:5672/",
					ConnectTimeout: 5 * time.Second,
					PublishTimeout: 2 * time.Second,
					MaxRetries:     3,
					RetryBackoff:   100 * time.Millisecond,
				},
				Vault: Vault{
					Address:     "https://localhost:8200",
					Token:       "vault-token",
					SecretsPath: "secret/data",
				},
				Redis: Redis{
					Type:  RedisTypeCluster,
					Addrs: []string{"localhost:6379"},
				},
				Postgres: Postgres{
					Host:          "localhost",
					Port:          5432,
					User:          "user",
					Password:      "pass",
					DBName:        "db",
					InsertTimeout: 5 * time.Second,
					ReadTimeout:   5 * time.Second,
				},
				Auth: Auth{
					SecretKey:         "your-key",
					UpdateKeyInterval: 1 * time.Hour,
					Issuer:            "test",
					TokenDuration:     1 * time.Hour,
					UserCacheTTL:      1 * time.Hour,
				},
				Policy: Policy{
					Config: "casbin_model.conf",
				},
			},
			wantErr: require.NoError,
		},
		{
			name:       "error: config file not found",
			configFile: "testdata/__does_not_exist__.yaml",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error read file")
			},
		},
		{
			name:       "error: invalid yaml",
			configFile: "testdata/invalid_yaml.yaml",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error unmarshal")
			},
		},
		{
			name:       "error: redis single without host and port",
			configFile: "testdata/redis_single_invalid.yaml",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error validate redis")
			},
		},
		{
			name:       "error: audit enabled but topic empty",
			configFile: "testdata/audit_empty_topic.yaml",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "audit is enabled but errors topic is not set")
			},
		},
		{
			name:       "error: audit enabled but rabbitmq url empty",
			configFile: "testdata/rabbit_empty_url.yaml",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error validate rabbitmq")
			},
		},
		{
			name:       "error: openfga config invalid",
			configFile: "testdata/invalid_openfga.yaml",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error validate openfga")
			},
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

//nolint:funlen // длинный тест
func TestValidateRabbitMQConfig(t *testing.T) {
	t.Parallel()

	valid := RabbitMQ{
		URL:            "amqp://localhost:5672/",
		ConnectTimeout: 5 * time.Second,
		PublishTimeout: 2 * time.Second,
		MaxRetries:     3,
		RetryBackoff:   100 * time.Millisecond,
	}

	tests := []struct {
		name    string
		cfg     RabbitMQ
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:    "valid config",
			cfg:     valid,
			wantErr: require.NoError,
		},
		{
			name: "valid config: amqps url",
			cfg: func() RabbitMQ {
				c := valid
				c.URL = "amqps://guest:guest@localhost:5672/"

				return c
			}(),
			wantErr: require.NoError,
		},
		{
			name: "missing url",
			cfg: func() RabbitMQ {
				c := valid
				c.URL = ""

				return c
			}(),
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq url is not set")
			},
		},
		{
			name: "missing connect timeout",
			cfg: func() RabbitMQ {
				c := valid
				c.ConnectTimeout = 0

				return c
			}(),
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq connect timeout is not set")
			},
		},
		{
			name: "missing publish timeout",
			cfg: func() RabbitMQ {
				c := valid
				c.PublishTimeout = 0

				return c
			}(),
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq publish timeout is not set")
			},
		},
		{
			name: "missing max retries",
			cfg: func() RabbitMQ {
				c := valid
				c.MaxRetries = 0

				return c
			}(),
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq max retries is not set")
			},
		},
		{
			name: "missing retry backoff",
			cfg: func() RabbitMQ {
				c := valid
				c.RetryBackoff = 0

				return c
			}(),
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq retry backoff is not set")
			},
		},
		{
			name: "invalid config: rabbitmq url must start with amqp://",
			cfg: func() RabbitMQ {
				c := valid
				c.URL = "http://localhost:5672/"

				return c
			}(),
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq url must start with amqp:// or amqps://")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateRabbitMQConfig(&tt.cfg)
			tt.wantErr(t, err)
		})
	}
}

//nolint:funlen // длинный тест
func TestValidateOpenFGAConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *Config
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "valid config",
			cfg: &Config{
				OpenFGA: OpenFGA{
					StoreName: "notes-bot",
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "valid config: store name is empty",
			cfg: &Config{
				OpenFGA: OpenFGA{
					StoreName: "",
					StoreID:   "123",
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "valid config: store id is empty",
			cfg: &Config{
				OpenFGA: OpenFGA{
					StoreName: "notes-bot",
					StoreID:   "",
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "invalid config: store name and store id are empty",
			cfg: &Config{
				OpenFGA: OpenFGA{
					StoreName: "",
					StoreID:   "",
				},
			},
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				t.Helper()

				require.Error(tt, err)
				require.ErrorContains(tt, err, "store_id or store_name is required")
			},
		},
		{
			name: "invalid config: store id and store name are spaces only",
			cfg: &Config{
				OpenFGA: OpenFGA{
					StoreName: "   ",
					StoreID:   "   ",
				},
			},
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				t.Helper()

				require.Error(tt, err)
				require.ErrorContains(tt, err, "store_id or store_name is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.validateOpenFGAConfig()
			tt.wantErr(t, err)
		})
	}
}
