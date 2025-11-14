package redis

import (
	"auth-service/internal/config"
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

type client struct {
	cfg   *config.Redis
	cache *redis.Client
}

// NewSingleClient создает новый экземпляр клиента для работы с Redis в режиме single.
func NewSingleClient(cfg *config.Redis) (*client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cfg is required")
	}

	logrus.WithFields(logrus.Fields{
		"host": cfg.Host,
		"port": cfg.Port,
		"type": "single",
	}).Info("creating client for redis")

	return &client{
		cfg: cfg,
		cache: redis.NewClient(&redis.Options{
			Addr: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		}),
	}, nil
}

// Connect соединяется с Redis в режиме single.
func (c *client) Connect(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"host": c.cfg.Host,
		"port": c.cfg.Port,
		"type": "single",
	}).Info("connecting to redis")

	return c.cache.Ping(ctx).Err()
}

// Close закрывает соединение с Redis в режиме single.
func (c *client) Close(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"host": c.cfg.Host,
		"port": c.cfg.Port,
		"type": "single",
	}).Info("closing single client for redis")

	return c.cache.Close()
}
