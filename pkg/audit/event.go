package audit

import (
	"auth-service/pkg/audit/internal"
	"context"
	"log/slog"
	"runtime"
)

// event - событие ошибки.
// Хранит внутри поля, необходимые для логирования ошибки.
type event struct {
	stash  Stash
	stack  string
	err    error
	logger *slog.Logger
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
	e.stack = string(buf[:n])

	e.Append(stackTrace(e.stack))

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

func (e *event) End(ctx context.Context) {
	if val, ok := e.stash.fields[fieldStatus]; ok && val.Value == StatusStarted { // уже может быть установлен статус completed, canceled или failed
		e.Append(StatusField(StatusCompleted))
	}

	e.log(ctx)
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
