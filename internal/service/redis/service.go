package redis

import (
	"auth-service/internal/config"
	"auth-service/internal/storage/redis"
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

// Service - сервис для работы с Redis.
type Service struct {
	cfg    *config.Redis
	client redisClient

	once sync.Once
	err  error
	mu   sync.Mutex
}

// redisClient - интерфейс для работы с Redis.
// Его реализуют клиент и кластерный клиент.
//
//go:generate mockgen -source=service.go -destination=mocks/mocks.go -package=mocks redisClient
type redisClient interface {
	Connect(ctx context.Context) error
	Close(ctx context.Context) error
}

// Option определяет опции для Service.
type Option func(*Service)

// WithCfg сохраняет конфигурацию Redis.
func WithCfg(cfg *config.Redis) Option {
	return func(s *Service) {
		s.cfg = cfg
	}
}

// New создает новый экземпляр Service для работы с Redis.
func New(opts ...Option) (*Service, error) {
	s := &Service{}

	for _, opt := range opts {
		opt(s)
	}

	if s.cfg == nil {
		return nil, fmt.Errorf("cfg is required")
	}

	return s, nil
}

// Connect соединяется с Redis в зависимости от типа конфигурации: single - один узел, cluster - кластер.
func (s *Service) Connect(ctx context.Context) error {
	s.once.Do(func() {
		var client redisClient

		s.mu.Lock()
		defer s.mu.Unlock()

		switch s.cfg.Type {
		case config.RedisTypeSingle:
			client, s.err = redis.NewSingleClient(s.cfg)
			if s.err != nil {
				s.err = fmt.Errorf("error creating redis client (single): %w", s.err)
				return
			}
		case config.RedisTypeCluster:
			client, s.err = redis.NewClusterClient(s.cfg)
			if s.err != nil {
				s.err = fmt.Errorf("error creating redis client (cluster): %w", s.err)
				return
			}
		default:
			s.err = fmt.Errorf("unknown redis type: %s", s.cfg.Type)
			return
		}

		if s.err = client.Connect(ctx); s.err != nil {
			s.err = fmt.Errorf("error connecting to redis: %w", s.err)
			return
		}

		s.client = client
		logrus.WithFields(logrus.Fields{
			"type":  s.cfg.Type,
			"host":  s.cfg.Host,
			"port":  s.cfg.Port,
			"addrs": s.cfg.Addrs,
		}).Info("successfully connected redis")
	})

	return s.err
}

// Stop закрывает соединение с Redis.
func (s *Service) Stop(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"type":  s.cfg.Type,
		"host":  s.cfg.Host,
		"port":  s.cfg.Port,
		"addrs": s.cfg.Addrs,
	}).Info("stopping redis")

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client == nil { // нет соединения, значит не нужно закрывать
		return nil
	}

	return s.client.Close(ctx)
}
