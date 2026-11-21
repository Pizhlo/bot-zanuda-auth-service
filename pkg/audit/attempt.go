package audit

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

//go:generate go run golang.org/x/tools/cmd/stringer -type=Stage,ContextError -linecomment -output=attempt_string.go

// OperationFilterNotes связывает HTTP-запрос и фильтрацию заметок с одним событием.
const OperationFilterNotes = "politics.filter_notes"

// MessageTokenVerificationFailed описывает отказ при проверке JWT сервисного клиента.
const MessageTokenVerificationFailed = "Token verification failed"

// Stage обозначает общий этап обработки пользовательского запроса.
type Stage uint8

const (
	StageReceiveRequest  Stage = iota + 1 // receive_request
	StageAuthenticate                     // authenticate
	StageDecodeRequest                    // decode_request
	StageValidateRequest                  // validate_request
	StageVerifySubject                    // verify_subject
	StageWriteResponse                    // write_response
)

// ContextError обозначает причину завершения контекста для поля context_error.
type ContextError uint8

const (
	ContextErrorCanceled         ContextError = iota + 1 // canceled
	ContextErrorDeadlineExceeded                         // deadline_exceeded
)

// ErrMissingAttempt означает, что операция вызвана без обязательного события аудита.
var ErrMissingAttempt = errors.New("request audit is required")

// Source создаёт события для одной операции.
type Source interface {
	Create(context.Context) (Event, error)
}

type attemptKey struct{}

// Attempt принадлежит одной синхронной операции. Её инициатор вызывает Finish через defer.
// Вложенные этапы уточняют стадию, идентификатор запроса и безопасную причину отказа.
type Attempt struct {
	operation string
	event     Event
	id        uuid.UUID
	stage     string
	failed    bool
	finished  bool
}

// OpenEvent получает обязательное событие до выполнения пользовательской операции.
func OpenEvent(ctx context.Context, source Source) (Event, error) {
	if source != nil {
		event, err := source.Create(ctx)
		if err != nil {
			return nil, errors.Join(ErrMissingAttempt, err)
		}

		if event != nil {
			return event, nil
		}
	}

	return nil, ErrMissingAttempt
}

// Begin создаёт событие до первой проверки и передаёт его вложенным этапам через контекст.
func Begin(ctx context.Context, source Source, operation, stage string) (context.Context, *Attempt, error) {
	event, err := OpenEvent(ctx, source)
	if err != nil {
		return ctx, nil, err
	}

	a := &Attempt{operation: operation, event: event, id: uuid.New(), stage: stage}
	a.event.Append(Operation(operation))
	a.event.Append(KindField(KindDomain))
	a.event.Append(Level(ErrLevelInfo))

	return context.WithValue(ctx, attemptKey{}, a), a, nil
}

// Current возвращает попытку указанной операции, если её уже открыл вызывающий код.
func Current(ctx context.Context, operation string) *Attempt {
	a := FromContext(ctx)
	if a != nil && a.operation == operation {
		return a
	}

	return nil
}

// FromContext возвращает попытку текущего HTTP-запроса для общего middleware.
func FromContext(ctx context.Context) *Attempt {
	a, _ := ctx.Value(attemptKey{}).(*Attempt)
	if a != nil && a.finished {
		return nil
	}

	return a
}

// At отмечает начало этапа. После отказа сохраняется этап первой ошибки.
// Методы аннотации допускают nil для вызовов за пределами аудируемой операции.
func (a *Attempt) At(stage string) {
	if a != nil && !a.failed {
		a.stage = stage
	}
}

// Correlate связывает событие с идентификатором, прочитанным из запроса.
func (a *Attempt) Correlate(id uuid.UUID) {
	if a != nil && id != uuid.Nil {
		a.id = id
	}
}

// Context добавляет к событию только известные на текущем этапе безопасные данные.
func (a *Attempt) Context(fields EventContext) {
	if a != nil {
		a.event.AppendContext(fields)
	}
}

// Fail сохраняет уровень, причину и этап первого отказа. Передавай только безопасную
// причину. Если зависимость может вернуть секреты, в cause попадёт описание отказа.
func (a *Attempt) Fail(code ErrorCode, kind Kind, level ErrorLevel, message string, reason error) {
	if a == nil || a.failed {
		return
	}

	a.failed = true
	a.event.Append(ErrorCodeField(code))
	a.event.Append(KindField(kind))
	a.event.Append(Level(level))
	a.event.Append(StatusField(StatusFailed))
	a.event.Append(Message(message))

	detail := message
	if reason != nil {
		detail = reason.Error()
	}

	a.event.Append(cause(detail))
}

// NoteCancellation отмечает отмену или истечение таймаута вложенного вызова,
// в том числе когда обработчик уже превратил его ошибку в HTTP-ответ.
func (a *Attempt) NoteCancellation(err error) {
	switch {
	case errors.Is(err, context.Canceled):
		a.Context(EventContext{KeyContextError: ContextErrorCanceled.String()})
	case errors.Is(err, context.DeadlineExceeded):
		a.Context(EventContext{KeyContextError: ContextErrorDeadlineExceeded.String()})
	}
}

// Finish фиксирует результат, включая ранний возврат и панику. При панике
// сначала завершает аудит, затем передаёт исходное значение штатному обработчику.
func (a *Attempt) Finish(ctx context.Context, result *error) {
	p := recover()
	if a.finished {
		if p != nil {
			panic(p)
		}

		return
	}

	a.finished = true
	switch {
	case p != nil:
		a.Context(EventContext{KeyPanic: true})
		a.Fail(ErrCodePanic, KindInternal, ErrLevelError, "Panic during operation", nil)
	case a.stage == StageReceiveRequest.String():
		a.Fail(ErrCodeRequestInterrupted, KindInfra, ErrLevelError, MessageRequestInterrupted, nil)
	case *result != nil:
		a.Fail(ErrorCode(a.stage+"_failed"), KindInfra, ErrLevelError, "Operation failed at "+a.stage, nil)
	}

	a.NoteCancellation(*result)
	a.NoteCancellation(ctx.Err())

	a.event.Append(RequestID(a.id.String()))
	a.Context(EventContext{KeyStage: a.stage})

	if err := a.event.End(ctx); err != nil {
		logrus.WithField(KeyRequestID, a.id.String()).WithField(KeyLocalWriteFailed, errors.Is(err, ErrLocalWrite)).Warn("audit event delivery failed")
	}

	if p != nil {
		panic(p)
	}
}
