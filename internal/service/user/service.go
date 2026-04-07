package user

import (
	"auth-service/internal/service/internal"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"
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
	auditor  auditor
}

//go:generate mockgen -source=service.go -destination=mocks/mocks.go -package=mocks userStorage
type userStorage interface {
	GetUserIDByTelegramID(ctx context.Context, telegramID string) (uuid.UUID, error)
}

type cache interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

type auditor interface {
	Create(ctx context.Context) audit.Event
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

// WithAuditor устанавливает сервис для логирования.
func WithAuditor(auditor auditor) option {
	return func(s *Service) {
		s.auditor = auditor
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

	if s.auditor == nil {
		return nil, errors.New("auditor is required")
	}

	return s, nil
}

const (
	serviceName         = "user"
	messageUserNotFound = "user not found"
)

// GetUserIDByTelegramID возвращает ID пользователя по telegram ID.
// Сначала пытается прочитать из кэша. Если не получается - читает из БД и обновляет кэш.
// Если значение в кэше не является строкой - читает из БД и обновляет кэш.
// Если значение в кэше не является UUID - читает из БД и обновляет кэш.
func (s *Service) GetUserIDByTelegramID(ctx context.Context, telegramID string) (uuid.UUID, error) {
	operationGetUserIDByTelegramID := fmt.Sprintf("%s.%s", serviceName, "get_user_id_by_telegram_id")

	ctx = internal.WithOperation(ctx, operationGetUserIDByTelegramID)
	ctx = internal.WithServiceName(ctx)

	event := s.auditor.Create(ctx)
	defer internal.WithPanicRecovery(ctx, event)()

	logrus.Debug("user service: getting user id by telegram id")

	userID, err := s.cache.Get(ctx, telegramID)
	if err != nil && !errors.Is(err, redis.Nil) {
		logrus.WithError(err).Warn("user service: cache read failed")

		userID = nil
	}

	getUserIDFromDB := func() (uuid.UUID, error) {
		userIDUUID, err := s.getUserIDByTelegramIDAndUpdateCache(ctx, telegramID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				event.WithError(audit.ErrCodeUserNotFound, audit.KindDomain, err)
				event.Append(audit.Message(messageUserNotFound))
				event.Append(audit.Level(audit.ErrLevelError))

				return uuid.Nil, err
			}

			event.WithError(audit.ErrCodeServiceUnavailable, audit.KindInfra, err)
			event.Append(audit.Level(audit.ErrLevelError))

			return uuid.Nil, err
		}

		return userIDUUID, nil
	}

	if err != nil || userID == nil {
		// Cache miss - получаем из storage
		return getUserIDFromDB()
	}

	// Cache hit - парсим значение
	userIDString, ok := userID.(string)
	if !ok {
		// любое невалидное значение в кэше обновляем из БД
		return getUserIDFromDB()
	}

	userIDUUID, err := uuid.Parse(userIDString)
	if err != nil {
		// любое невалидное значение в кэше обновляем из БД
		return getUserIDFromDB()
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

// GetUserIDByTelegramIDHook добавляет поля в stash события.
// Используется в качестве хука для аудитора.
func GetUserIDByTelegramIDHook(ctx context.Context, stash audit.Stash) audit.Stash {
	if serviceName, ok := ctx.Value(internal.ServiceNameKey{}).(string); ok {
		stash = stash.Append(audit.ServiceName(serviceName))
	}

	if operation, ok := ctx.Value(internal.OperationKey{}).(string); ok {
		stash = stash.Append(audit.Operation(operation))
	}

	if level, ok := ctx.Value(internal.LevelKey{}).(audit.ErrorLevel); ok {
		stash = stash.Append(audit.Level(level))
	}

	if errorCode, ok := ctx.Value(internal.ErrorCodeKey{}).(audit.ErrorCode); ok {
		stash = stash.Append(audit.ErrorCodeField(errorCode))
	}

	if messageCtx, ok := ctx.Value(internal.MessageContextKey{}).(audit.EventContext); ok {
		stash = stash.Append(audit.ContextField(messageCtx, stash))
	}

	if userID, ok := ctx.Value(internal.UserIDKey{}).(string); ok {
		stash = stash.Append(audit.UserID(userID))
	}

	if kind, ok := ctx.Value(internal.KindKey{}).(audit.Kind); ok {
		stash = stash.Append(audit.KindField(kind))
	}

	return stash
}
