package redis

import (
	"auth-service/internal/config"
	"context"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

type cluster struct {
	cfg   *config.Redis
	cache *redis.ClusterClient

	// Для управления блокировками с автоматическим обновлением TTL
	locksMu sync.Mutex
	locks   map[string]*lockInfo
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

	// Останавливаем все обновления блокировок
	// Ключи останутся в Redis и истекут естественным образом через TTL
	c.locksMu.Lock()

	for _, lock := range c.locks {
		lock.cancel()
		lock.stopOnce.Do(func() {
			close(lock.stopped)
		})
	}

	c.locks = make(map[string]*lockInfo)
	c.locksMu.Unlock()

	return c.cache.Close()
}

func (c *cluster) Get(ctx context.Context, key string) (string, error) {
	return c.cache.Get(ctx, key).Result()
}

func (c *cluster) Del(ctx context.Context, key string) error {
	return c.cache.Del(ctx, key).Err()
}
