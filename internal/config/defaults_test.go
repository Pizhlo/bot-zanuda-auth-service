package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSetDefaults(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name               string
		fga, audit         time.Duration
		wantFGA, wantAudit time.Duration
	}{
		{"defaults", 0, 0, DefaultOpenFGARequestTimeout, DefaultAuditDeliveryTimeout},
		{"explicit", 2 * time.Second, 3 * time.Second, 2 * time.Second, 3 * time.Second},
		{"negative fga", -time.Second, time.Second, -time.Second, time.Second},
		{"negative audit", time.Second, -time.Second, time.Second, -time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg Config

			cfg.OpenFGA.RequestTimeout, cfg.Audit.DeliveryTimeout = tt.fga, tt.audit

			cfg.setDefaults()

			require.Equal(t, tt.wantFGA, cfg.OpenFGA.RequestTimeout)
			require.Equal(t, tt.wantAudit, cfg.Audit.DeliveryTimeout)
		})
	}
}
