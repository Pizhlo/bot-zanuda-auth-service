package redis

import (
	"auth-service/internal/config"
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

type cluster struct {
	cfg   *config.Redis
	cache *redis.ClusterClient
}

// NewClusterClient создает новый экземпляр клиента для работы с Redis в режиме cluster.
func NewClusterClient(cfg *config.Redis) (*cluster, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cfg is required")
	}

	logrus.WithFields(logrus.Fields{
		"addrs": cfg.Addrs,
		"type":  "cluster",
	}).Info("creating cluster client for redis")

	return &cluster{
		cfg: cfg,
		cache: redis.NewClusterClient(&redis.ClusterOptions{
			Addrs: cfg.Addrs,
		}),
	}, nil
}

// Connect соединяется с Redis в режиме cluster.
func (c *cluster) Connect(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"addrs": c.cfg.Addrs,
		"type":  "cluster",
	}).Info("connecting to redis cluster")

	return c.cache.Ping(ctx).Err()
}

// Close закрывает соединение с Redis в режиме cluster.
func (c *cluster) Close(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"addrs": c.cfg.Addrs,
		"type":  "cluster",
	}).Info("closing cluster client for redis cluster")

	return c.cache.Close()
}
