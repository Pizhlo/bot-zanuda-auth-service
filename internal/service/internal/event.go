package internal

import (
	"auth-service/pkg/audit"
	"context"
	"fmt"
)

// MessageContextKey - ключ для контекста сообщения.
// Может хранить в себе любые данные: url, по которому пришел запрос, метод, uuid пользователя, etc.
// При обработке события аудита, эти данные добавляются в stash хуками по этому ключу.
type MessageContextKey struct{}

// WithMessageCtx добавляет контекст сообщения в контекст по ключу MessageContextKey.
// При обработке события аудита, эти данные добавляются в stash хуками по этому ключу.
func WithMessageCtx(ctx context.Context, messageCtx audit.EventContext) context.Context {
	return context.WithValue(ctx, MessageContextKey{}, messageCtx)
}

// UserIDKey - ключ для контекста ID пользователя.
// Хранит ID пользователя, который выполняет операцию.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
type UserIDKey struct{}

// WithUserID добавляет ID пользователя в контекст по ключу UserIDKey.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey{}, userID)
}

// ServiceNameKey - ключ для контекста имени сервиса.
// Хранит имя сервиса, который выполняет операцию.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
type ServiceNameKey struct{}

// WithServiceName добавляет имя сервиса в контекст по ключу ServiceNameKey.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
func WithServiceName(ctx context.Context) context.Context {
	return context.WithValue(ctx, ServiceNameKey{}, "auth-service")
}

// MessageKey - ключ для контекста сообщения.
// Хранит сообщение, которое будет добавлено в stash события.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
type MessageKey struct{}

// WithMessage добавляет сообщение в контекст по ключу MessageKey.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
func WithMessage(ctx context.Context, message string) context.Context {
	return context.WithValue(ctx, MessageKey{}, message)
}

// LevelKey - ключ для контекста уровня ошибки.
// Хранит уровень ошибки, который будет добавлен в stash события.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
type LevelKey struct{}

// WithLevel добавляет уровень ошибки в контекст по ключу LevelKey.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
func WithLevel(ctx context.Context, level audit.ErrorLevel) context.Context {
	return context.WithValue(ctx, LevelKey{}, level)
}

// ErrorCodeKey - ключ для контекста кода ошибки.
// Хранит код ошибки, который будет добавлен в stash события.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
type ErrorCodeKey struct{}

// WithErrorCode добавляет код ошибки в контекст по ключу ErrorCodeKey.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
func WithErrorCode(ctx context.Context, errorCode audit.ErrorCode) context.Context {
	return context.WithValue(ctx, ErrorCodeKey{}, errorCode)
}

// KindKey - ключ для контекста вида ошибки.
// Хранит вид ошибки, который будет добавлен в stash события.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
type KindKey struct{}

// WithKind добавляет вид ошибки в контекст по ключу KindKey.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
func WithKind(ctx context.Context, kind audit.Kind) context.Context {
	return context.WithValue(ctx, KindKey{}, kind)
}

// OperationKey - ключ для контекста операции.
// Хранит операцию, которая будет добавлена в stash события.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
type OperationKey struct{}

// WithOperation добавляет операцию в контекст по ключу OperationKey.
// При обработке события аудита, это поле добавляется в stash хуками по этому ключу.
func WithOperation(ctx context.Context, operation string) context.Context {
	return context.WithValue(ctx, OperationKey{}, operation)
}

// WithPanicRecovery добавляет обработку паник для события аудита.
// Используется в качестве хука для аудитора.
func WithPanicRecovery(ctx context.Context, event audit.Event) func() {
	// метода End() нет в основном интерфейсе, но мы можем получить к нему доступ через тип assertion.
	type ender interface {
		End(ctx context.Context)
	}

	return func() {
		if r := recover(); r != nil {
			event.Append(audit.Message("PANIC"))
			event.WithError(audit.ErrCodePanic, audit.KindInternal, fmt.Errorf("%v", r))
			event.Append(audit.Level(audit.ErrLevelPanic))

			if ender, ok := event.(ender); ok {
				ender.End(ctx)
			}

			panic(r)
		}

		if ender, ok := event.(ender); ok {
			ender.End(ctx)
		}
	}
}
