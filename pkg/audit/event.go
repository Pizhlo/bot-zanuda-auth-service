package audit

import (
	"auth-service/pkg/audit/internal"
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strings"
)

// event - событие ошибки.
// Хранит внутри поля, необходимые для логирования ошибки.
type event struct {
	stash  Stash
	err    error
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

func (e *event) Append(field internal.Field) {
	e.stash = e.stash.Append(field)
}

func (e *event) AppendContext(context EventContext) {
	e.Append(ContextField(context, e.stash))
}

func (e *event) AppendError(err error) {
	e.err = err

	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	stack := string(buf[:n])

	e.Append(stackTrace(stack))

	e.Append(StatusField(StatusFailed))
}

func (e *event) WithError(code ErrorCode, kind Kind, err error) {
	e.Append(ErrorCodeField(code))
	e.Append(KindField(kind))
	e.Append(Level(ErrLevelError))
	e.AppendError(err)
}

func (e *event) Error() string {
	if e.err == nil {
		return ""
	}

	return e.err.Error()
}

func (e *event) End(ctx context.Context) error {
	if val, ok := e.stash.fields[fieldStatus]; ok && val.Value == StatusStarted { // уже может быть установлен статус completed, canceled или failed
		e.Append(StatusField(StatusCompleted))
	}

	e.log(ctx)

	val, ok := e.stash.fields[fieldKind]
	if !ok {
		return nil // если нет kind -> не ошибка -> не отправляем
	}

	kind := val.Value.(Kind)

	val, ok = e.stash.fields[fieldLevel]
	if !ok {
		return fmt.Errorf("fieldLevel not found")
	}

	level := val.Value.(ErrorLevel)

	if e.shouldSend(kind, level) && e.sender != nil {
		fields := make(map[fieldID]any)
		for k, v := range e.stash.fields {
			fields[fieldID(k)] = v.Value
		}

		if e.err != nil {
			fields[fieldCause] = e.err.Error()
		}

		err := e.sender.Send(ctx, fields)
		if err != nil {
			return fmt.Errorf("send event: %w", err)
		}
	}

	return nil
}

func (e *event) shouldSend(kind Kind, level ErrorLevel) bool {
	kindStr := strings.ToLower(kind.String())
	levelStr := strings.ToLower(level.String())

	// исключения всегда блокируют (kind или level)
	if slices.Contains(e.kinds.exclude, kindStr) {
		return false
	}

	if slices.Contains(e.levels.exclude, levelStr) {
		return false
	}

	// eсли include пустой → все разрешены  (после проверки exclude)
	if len(e.kinds.include) == 0 && len(e.levels.include) == 0 {
		return true
	}

	// kind whitelist (если задан)
	kindOK := false
	if len(e.kinds.include) == 0 {
		kindOK = true // все kinds
	} else {
		kindOK = slices.Contains(e.kinds.include, kindStr)
	}

	// level whitelist (если задан)
	levelOK := false
	if len(e.levels.include) == 0 {
		levelOK = true // все levels
	} else {
		levelOK = slices.Contains(e.levels.include, levelStr)
	}

	return kindOK && levelOK
}

func (e *event) log(ctx context.Context) {
	attrs := make([]slog.Attr, 0, len(e.stash.fields))

	for k, v := range e.stash.fields {
		attrs = append(attrs, slog.Any(fieldID(k).String(), v.Value))
	}

	if e.err != nil {
		attrs = append(attrs, slog.Any("error", e.err.Error()))
	}

	e.logger.LogAttrs(ctx, LevelAudit, "audit event", attrs...)
}
