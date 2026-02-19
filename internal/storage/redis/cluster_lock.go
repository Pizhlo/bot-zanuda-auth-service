package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// SetWithLocking устанавливает значение в Redis с блокировкой.
// Если ttl > 0, блокировка будет автоматически обновляться пока приложение работает.
// Если ttl = 0, блокировка будет существовать бесконечно (не рекомендуется).
func (c *cluster) SetWithLocking(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	ok, err := c.cache.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, err
	}

	if !ok {
		return false, nil // ключ уже существует
	}

	// Если TTL установлен, запускаем автоматическое обновление
	if ttl > 0 {
		if err := c.startLockRefresh(ctx, key, value, ttl); err != nil {
			// Если не удалось запустить обновление, удаляем ключ
			_ = c.cache.Del(ctx, key)
			return false, fmt.Errorf("failed to start lock refresh: %w", err)
		}
	}

	return true, nil
}

// RefreshLock обновляет TTL существующей блокировки.
func (c *cluster) RefreshLock(ctx context.Context, key string, ttl time.Duration) error {
	return c.cache.Expire(ctx, key, ttl).Err()
}

// ReleaseLock освобождает блокировку, останавливая автоматическое обновление TTL.
// Ключ останется в Redis и истечет естественным образом через установленный TTL.
// Если нужно немедленно удалить ключ, используйте Del напрямую.
func (c *cluster) ReleaseLock(ctx context.Context, key string) error {
	c.locksMu.Lock()
	lock, exists := c.locks[key]
	if exists {
		delete(c.locks, key)
		lock.cancel()
		lock.stopOnce.Do(func() {
			close(lock.stopped)
		})
	}
	c.locksMu.Unlock()

	// Не удаляем ключ - он истечет естественным образом через TTL
	// Это безопаснее: если процесс упал, ключ все равно истечет
	return nil
}

// startLockRefresh запускает фоновую горутину для автоматического обновления TTL блокировки.
func (c *cluster) startLockRefresh(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	refreshCtx, cancel := context.WithCancel(ctx)
	refreshInterval := ttl / 3
	if refreshInterval < time.Second {
		refreshInterval = time.Second
	}

	lock := &lockInfo{
		key:     key,
		value:   value,
		ttl:     ttl,
		ctx:     refreshCtx,
		cancel:  cancel,
		refresh: refreshInterval,
		stopped: make(chan struct{}),
	}

	c.locksMu.Lock()
	c.locks[key] = lock
	c.locksMu.Unlock()

	go c.refreshLockLoop(lock)

	return nil
}

// refreshLockLoop периодически обновляет TTL блокировки.
func (c *cluster) refreshLockLoop(lock *lockInfo) {
	ticker := time.NewTicker(lock.refresh)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Обновляем TTL блокировки
			if err := c.cache.Expire(lock.ctx, lock.key, lock.ttl).Err(); err != nil {
				logrus.WithError(err).WithField("key", lock.key).Error("failed to refresh lock TTL")
				// Если не удалось обновить, останавливаем обновление
				c.locksMu.Lock()
				delete(c.locks, lock.key)
				c.locksMu.Unlock()
				lock.stopOnce.Do(func() {
					close(lock.stopped)
				})
				return
			}
			logrus.WithFields(logrus.Fields{
				"key": lock.key,
				"ttl": lock.ttl,
			}).Debug("lock TTL refreshed")

		case <-lock.ctx.Done():
			// Контекст отменен - останавливаем обновление
			c.locksMu.Lock()
			delete(c.locks, lock.key)
			c.locksMu.Unlock()
			lock.stopOnce.Do(func() {
				close(lock.stopped)
			})
			return

		case <-lock.stopped:
			// Явная остановка
			return
		}
	}
}
