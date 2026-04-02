package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// Service - сервис пользователей.
type Service struct {
	storage  userStorage
	cache    cache
	cacheTTL time.Duration
}

//go:generate mockgen -source=service.go -destination=mocks/mocks.go -package=mocks userStorage
type userStorage interface {
	GetUserIDByTelegramID(ctx context.Context, telegramID string) (uuid.UUID, error)
}

type cache interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

type option func(*Service)

// WithStorage устанавливает хранилище пользователей.
func WithStorage(storage userStorage) option {
	return func(s *Service) {
		s.storage = storage
	}
}

// WithCache устанавливает кэш пользователей.
func WithCache(cache cache) option {
	return func(s *Service) {
		s.cache = cache
	}
}

// WithCacheTTL устанавливает TTL кэша.
func WithCacheTTL(ttl time.Duration) option {
	return func(s *Service) {
		s.cacheTTL = ttl
	}
}

// New создает новый сервис пользователей.
func New(opts ...option) (*Service, error) {
	s := &Service{}

	for _, opt := range opts {
		opt(s)
	}

	if s.storage == nil {
		return nil, errors.New("storage is required")
	}

	if s.cache == nil {
		return nil, errors.New("cache is required")
	}

	if s.cacheTTL == 0 {
		return nil, errors.New("cache ttl is required")
	}

	return s, nil
}

// GetUserIDByTelegramID возвращает ID пользователя по telegram ID.
// Сначала пытается прочитать из кэша. Если не получается - читает из БД и обновляет кэш.
// Если значение в кэше не является строкой - читает из БД и обновляет кэш.
// Если значение в кэше не является UUID - читает из БД и обновляет кэш.
func (s *Service) GetUserIDByTelegramID(ctx context.Context, telegramID string) (uuid.UUID, error) {
	logrus.Debug("user service: getting user id by telegram id")

	userID, err := s.cache.Get(ctx, telegramID)
	if err != nil && !errors.Is(err, redis.Nil) {
		logrus.WithError(err).Warn("user service: cache read failed")

		userID = nil
	}

	if err != nil || userID == nil {
		// Cache miss - получаем из storage
		return s.getUserIDByTelegramIDAndUpdateCache(ctx, telegramID)
	}

	// Cache hit - парсим значение
	userIDString, ok := userID.(string)
	if !ok {
		// любое невалидное значение в кэше обновляем из БД
		return s.getUserIDByTelegramIDAndUpdateCache(ctx, telegramID)
	}

	userIDUUID, err := uuid.Parse(userIDString)
	if err != nil {
		// любое невалидное значение в кэше обновляем из БД
		return s.getUserIDByTelegramIDAndUpdateCache(ctx, telegramID)
	}

	return userIDUUID, nil
}

// getUserIDByTelegramIDAndUpdateCache получает ID пользователя по telegram ID и обновляет кэш.
// Если не получится обновить кэш, вернет валидный UUID и nil-ошибку.
func (s *Service) getUserIDByTelegramIDAndUpdateCache(ctx context.Context, telegramID string) (uuid.UUID, error) {
	logrus.Debug("user service: getting user id by telegram id and updating cache")

	userIDUUID, err := s.storage.GetUserIDByTelegramID(ctx, telegramID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("user service: failed to get user id by telegram id: %w", err)
	}

	if err = s.cache.Set(ctx, telegramID, userIDUUID.String(), s.cacheTTL); err != nil {
		logrus.WithField("user_uuid", userIDUUID).WithError(err).Warn("user service: cache write failed")

		return userIDUUID, nil
	}

	return userIDUUID, nil
}
