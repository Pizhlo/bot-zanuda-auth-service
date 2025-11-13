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
