package audit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestFinish(t *testing.T) {
	t.Parallel()

	complete := func(a *Attempt, ctx context.Context, err *error) {
		defer a.Finish(ctx, err)
	}
	for _, tc := range []struct {
		name, stage         string
		wantPanic           any
		result, deliveryErr error
		finish              func(*Attempt, context.Context, *error)
		want                func(*testing.T, string)
	}{
		{"completed", "read", nil, nil, nil, complete, func(t *testing.T, log string) { t.Helper(); require.NotContains(t, log, "status=failed") }},
		{"early return", StageReceiveRequest.String(), nil, nil, nil, complete, func(t *testing.T, log string) {
			t.Helper()
			require.Contains(t, log, string(ErrCodeRequestInterrupted))
		}},
		{"caller error", "read", nil, context.Canceled, nil, complete, func(t *testing.T, log string) {
			t.Helper()
			require.Contains(t, log, "read_failed")
			require.Contains(t, log, "context_error:canceled")
		}},
		{"delivery failed", "read", nil, nil, io.ErrClosedPipe, complete, func(t *testing.T, log string) { t.Helper(); require.Contains(t, log, "stage:read") }},
		{"panic after earlier rejection", "read", "private panic", nil, nil, func(a *Attempt, ctx context.Context, err *error) {
			defer a.Finish(ctx, err)

			a.Fail(ErrCodeValidationFailed, KindValidation, ErrLevelDebug, "Invalid resource", nil)
			a.At("write")
			panic("private panic")
		}, func(t *testing.T, log string) {
			t.Helper()
			require.Equal(t, 1, strings.Count(log, "status=failed"))
			require.Contains(t, log, "stage:read")
			require.Contains(t, log, "panic:true")
			require.Contains(t, log, "Invalid resource")
			require.NotContains(t, log, "private panic")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var log bytes.Buffer

			ctx, a, err := Begin(t.Context(), NewAuditor(WithOutput(&log)), "update", tc.stage)
			require.NoError(t, err)
			event := &finishEvent{Event: a.event, err: tc.deliveryErr}
			a.event = event

			var caught any

			func() { defer func() { caught = recover() }(); tc.finish(a, ctx, &tc.result) }()
			require.Equal(t, tc.wantPanic, caught)
			require.Equal(t, 1, event.ended)
			tc.want(t, log.String())
		})
	}
}

type finishEvent struct {
	Event
	err   error
	ended int
}

func (e *finishEvent) End(ctx context.Context) error {
	e.ended++
	return errors.Join(e.Event.End(ctx), e.err)
}

func TestBegin(t *testing.T) {
	t.Parallel()

	wantMissing := func(t *testing.T, parent, got context.Context, a *Attempt) {
		t.Helper()
		require.Same(t, parent, got)
		require.Nil(t, a)
	}
	for _, tc := range []struct {
		name    string
		source  Source
		wantErr error
		want    func(*testing.T, context.Context, context.Context, *Attempt)
	}{
		{"no source", nil, ErrMissingAttempt, wantMissing},
		{"no event", emptyEventSource{}, ErrMissingAttempt, wantMissing},
		{"active event", NewAuditor(WithOutput(io.Discard)), nil, func(t *testing.T, _ context.Context, ctx context.Context, a *Attempt) {
			t.Helper()
			require.Same(t, a, Current(ctx, "update"))
			require.Equal(t, "decode", a.stage)
			require.NotEqual(t, uuid.Nil, a.id)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parent := t.Context()
			ctx, a, err := Begin(parent, tc.source, "update", "decode")
			require.ErrorIs(t, err, tc.wantErr)

			tc.want(t, parent, ctx, a)
		})
	}
}

func TestCurrent(t *testing.T) {
	t.Parallel()

	a := &Attempt{operation: "read"}
	ctx := context.WithValue(t.Context(), attemptKey{}, a)
	require.Same(t, a, Current(ctx, "read"))
	require.Nil(t, Current(ctx, "write"))

	a.finished = true

	require.Nil(t, Current(ctx, "read"))
}

func TestFromContext(t *testing.T) {
	t.Parallel()

	require.Nil(t, FromContext(t.Context()))

	a := &Attempt{}
	require.Same(t, a, FromContext(context.WithValue(t.Context(), attemptKey{}, a)))
}

func TestAt(t *testing.T) {
	t.Parallel()

	a := &Attempt{}

	a.At("decode")

	require.Equal(t, "decode", a.stage)
	a.failed = true

	a.At("write")

	require.Equal(t, "decode", a.stage)
}

func TestCorrelate(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	a := &Attempt{}

	a.Correlate(id)

	a.Correlate(uuid.Nil)

	require.Equal(t, id, a.id)
}

func TestContext(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer

	ctx, a, startErr := Begin(t.Context(), NewAuditor(WithOutput(&output)), "update", "decode")
	require.NoError(t, startErr)

	a.Context(EventContext{"space_id": "s"})

	a.Context(EventContext{"subject": "u"})

	var err error
	a.Finish(ctx, &err)
	require.Contains(t, output.String(), "space_id:s")
	require.Contains(t, output.String(), "subject:u")
}

func TestFail(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		level     ErrorLevel
		reason    error
		wantCause string
	}{
		{ErrLevelDebug, errors.New("owner is required"), "owner is required"},
		{ErrLevelWarn, errors.New("user is not member"), "user is not member"},
		{ErrLevelError, nil, "Operation failed"},
	} {
		t.Run(tc.level.String(), func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer

			sender := &mockSender{}
			source := NewAuditor(WithOutput(&output), WithSender(sender))
			ctx, a, err := Begin(t.Context(), source, "update", "validate")
			require.NoError(t, err)

			a.Fail(ErrCodeValidationFailed, KindValidation, tc.level, "Operation failed", tc.reason)
			a.At("write")
			a.Fail(ErrCodePanic, KindInternal, ErrLevelError, "Second failure", nil)
			a.Finish(ctx, &err)

			require.Equal(t, 1, sender.sendCalls)
			require.Equal(t, tc.level, sender.sent[fieldLevel])
			require.Equal(t, tc.wantCause, sender.sent[fieldCause])
			require.Equal(t, "validate", a.stage)
			require.Contains(t, output.String(), "cause=")
			require.Contains(t, output.String(), tc.wantCause)
			require.NotContains(t, output.String(), "Second failure")
		})
	}
}

func TestNoteCancellation(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		err  error
		want string
	}{{"canceled", context.Canceled, "canceled"}, {"deadline", context.DeadlineExceeded, "deadline_exceeded"}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer

			ctx, a, startErr := Begin(t.Context(), NewAuditor(WithOutput(&output)), "update", "decode")
			require.NoError(t, startErr)

			a.NoteCancellation(tt.err)

			var err error
			a.Finish(ctx, &err)
			require.Contains(t, output.String(), "context_error:"+tt.want)
		})
	}
}

type emptyEventSource struct{}

func (emptyEventSource) Create(context.Context) (Event, error) { return nil, nil }

func TestOpenEvent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		source Source
		want   error
	}{
		{"no source", nil, ErrMissingAttempt}, {"no event", emptyEventSource{}, ErrMissingAttempt},
		{"active source", NewAuditor(WithOutput(io.Discard)), nil},
		{"unwritable log", NewAuditor(WithOutput(brokenOutput{})), ErrLocalWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			event, err := OpenEvent(t.Context(), tc.source)
			require.ErrorIs(t, err, tc.want)

			require.Equal(t, tc.want == nil, event != nil)
		})
	}
}
