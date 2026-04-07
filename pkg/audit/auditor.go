package audit

import (
	"auth-service/pkg/audit/internal"
	"context"
	"log/slog"
)

// Stash - структура для хранения полей события.
type Stash struct {
	fields map[fieldID]internal.Field
}

// Append добавляет новое поле в stash.
func (s Stash) Append(field internal.Field) Stash {
	if s.fields == nil {
		s.fields = make(map[fieldID]internal.Field)
	}

	s.fields[fieldID(field.FieldID)] = field

	return s
}

// EventHook - функция, которая обрабатывает событие.
// С помощью нее другие сервисы могут добавлять свои поля в событие.
type EventHook func(ctx context.Context, stash Stash) Stash

// Auditor - аудитор событий.
// Следит за всеми событиями и логирует их.
// Сервисы интегрируют его себе через хуки и контекст.
type Auditor struct {
	hooks  []EventHook
	logger *slog.Logger
}

// auditorOption - опция для создания аудитора.
type auditorOption func(*Auditor)

// WithHook добавляет новый хук в аудитор.
func WithHook(hook EventHook) auditorOption {
	return func(b *Auditor) {
		b.hooks = append(b.hooks, hook)
	}
}

// Event - интерфейс события аудита.
type Event interface {
	Append(field internal.Field)
	AppendContext(context EventContext)
	AppendError(err error)
	WithError(code ErrorCode, kind Kind, err error)
	Error() string
}

// NewAuditor создает новый аудитор.
func NewAuditor(opts ...auditorOption) *Auditor {
	b := &Auditor{
		logger: InitLogger(),
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// Create создает новое событие аудита.
// Автоматически добавляет поле текущего времени и статус операции: started.
func (a *Auditor) Create(ctx context.Context) Event {
	e := &event{
		stash:  Stash{fields: make(map[fieldID]internal.Field)},
		logger: a.logger,
	}

	for _, hook := range a.hooks {
		e.stash = hook(ctx, e.stash)
	}

	e.Append(currentTime())
	e.Append(StatusField(StatusStarted))

	e.log(ctx)

	return e
}
