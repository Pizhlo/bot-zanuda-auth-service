package config

import "time"

const (
	// DefaultOpenFGARequestTimeout сохраняет прежний бюджет при отсутствии настройки в YAML.
	DefaultOpenFGARequestTimeout = 5 * time.Second
	// DefaultAuditDeliveryTimeout ограничивает задержку ответа при недоступном брокере.
	DefaultAuditDeliveryTimeout = time.Second
	// DefaultMigrationTimeout — общий бюджет операторского запуска space-types.
	DefaultMigrationTimeout = 10 * time.Minute
)

func (c *Config) setDefaults() {
	if c.OpenFGA.RequestTimeout == 0 {
		c.OpenFGA.RequestTimeout = DefaultOpenFGARequestTimeout
	}

	if c.Audit.DeliveryTimeout == 0 {
		c.Audit.DeliveryTimeout = DefaultAuditDeliveryTimeout
	}
}
