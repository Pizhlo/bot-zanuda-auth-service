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
	sender Sender

	// какие уровни отправлять в rabbitmq. если не заданы, то отправляются все уровни.
	// если установлены и levels, и kinds, то событие будет отправлено только если оно соответствует обоим условиям.
	// если правило попало и в include, и в exclude, то событие не будет отправлено.
	levels struct {
		include []string
		exclude []string
	}

	// какие виды ошибок отправлять в rabbitmq. если не заданы, то отправляются все виды.
	// если установлены и levels, и kinds, то событие будет отправлено только если оно соответствует обоим условиям.
	// если правило попало и в include, и в exclude, то событие не будет отправлено.
	kinds struct {
		include []string
		exclude []string
	}
}

// auditorOption - опция для создания аудитора.
type auditorOption func(*Auditor)

// WithHook добавляет новый хук в аудитор.
func WithHook(hook EventHook) auditorOption {
	return func(b *Auditor) {
		b.hooks = append(b.hooks, hook)
	}
}

// WithSender устанавливает sender для отправки событий аудита.
// Позволяет использовать разные системы для отправки событий аудита.
// Например, rabbitmq, kafka, etc.
func WithSender(sender Sender) auditorOption {
	return func(b *Auditor) {
		b.sender = sender
	}
}

// WithIncludeLevels устанавливает включаемые уровни ошибок.
func WithIncludeLevels(include []string) auditorOption {
	return func(b *Auditor) {
		b.levels.include = include
	}
}

// WithExcludeLevels устанавливает исключаемые уровни ошибок.
func WithExcludeLevels(exclude []string) auditorOption {
	return func(b *Auditor) {
		b.levels.exclude = exclude
	}
}

// WithIncludeKinds устанавливает включаемые виды ошибок.
func WithIncludeKinds(include []string) auditorOption {
	return func(b *Auditor) {
		b.kinds.include = include
	}
}

// WithExcludeKinds устанавливает исключаемые виды ошибок.
func WithExcludeKinds(exclude []string) auditorOption {
	return func(b *Auditor) {
		b.kinds.exclude = exclude
	}
}

// Sender - интерфейс для отправки событий аудита.
type Sender interface {
	Send(ctx context.Context, fields map[fieldID]any) error
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
		sender: a.sender,
		levels: a.levels,
		kinds:  a.kinds,
	}

	for _, hook := range a.hooks {
		e.stash = hook(ctx, e.stash)
	}

	e.Append(currentTime())
	e.Append(StatusField(StatusStarted))

	e.log(ctx)

	return e
}
