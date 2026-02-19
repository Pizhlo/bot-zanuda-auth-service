package auth

import (
	"context"
	"errors"
	"time"

	"github.com/sirupsen/logrus"
)

// Service - сервис для работы с авторизацией.
// используется для получения ключа авторизации из vault и его обновления, а также для генерации jwt токенов.
type Service struct {
	updateKeyInterval time.Duration // периодичность, с которой нужно обновлять ключ
	vaultClient       vaultClient   // клиент для доступа к vault
	redisClient       redisClient   // клиент для доступа к redis
	isMaster          bool
}

// vaultClient - интерфейс для доступа к vault.
//
//go:generate mockgen -source=service.go -destination=mocks/mocks.go -package=mocks
type vaultClient interface {
	// здесь методы для доступа к vault
}

// redisClient - интерфейс для доступа к redis.
//
//go:generate mockgen -source=service.go -destination=mocks/mocks.go -package=mocks
type redisClient interface {
	Get(ctx context.Context, key string) (string, error)
	// SetWithLocking устанавливает значение в Redis с блокировкой.
	// Если ttl > 0, блокировка будет автоматически обновляться пока приложение работает.
	// Если ttl = 0, блокировка будет существовать бесконечно (не рекомендуется).
	SetWithLocking(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	Del(ctx context.Context, key string) error
}

type option func(*Service)

// WithUpdateKeyInterval устанавливает периодичность обновления ключа авторизации.
func WithUpdateKeyInterval(interval time.Duration) option {
	return func(s *Service) {
		s.updateKeyInterval = interval
	}
}

// WithVaultClient устанавливает клиент для доступа к vault.
func WithVaultClient(client vaultClient) option {
	return func(s *Service) {
		s.vaultClient = client
	}
}

// WithRedisClient устанавливает клиент для доступа к redis.
func WithRedisClient(client redisClient) option {
	return func(s *Service) {
		s.redisClient = client
	}
}

// New создает новый сервис для работы с авторизацией.
func New(opts ...option) (*Service, error) {
	s := &Service{}

	for _, opt := range opts {
		opt(s)
	}

	if s.updateKeyInterval == 0 {
		return nil, errors.New("update key interval is required")
	}

	if s.vaultClient == nil {
		return nil, errors.New("vault client is required")
	}

	if s.redisClient == nil {
		return nil, errors.New("redis client is required")
	}

	return s, nil
}

func (s *Service) Run(ctx context.Context) error {
	if err := s.checkMasterKey(ctx); err == nil {
		// ключ найден - мастер уже есть
		logrus.WithFields(logrus.Fields{
			"service": "auth",
		}).Info("master key found, skipping master key setup")

		return nil
	}

	if !s.isMaster {
		return s.setMasterKey(ctx)
	}

	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"service": "auth",
	}).Info("stopping auth service")

	if !s.isMaster {
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"service": "auth",
		"key":     masterKey,
	}).Info("deleting master key")

	return s.redisClient.Del(ctx, masterKey)
}
