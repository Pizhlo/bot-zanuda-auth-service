package audit

import (
	"auth-service/pkg/audit/internal"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvent_Append(t *testing.T) {
	t.Parallel()

	event := &event{
		stash: Stash{
			fields: make(map[fieldID]internal.Field),
		},
		logger: InitLogger(),
	}

	field := ServiceName("test")
	event.Append(field)

	require.Len(t, event.stash.fields, 1)
	require.Equal(t, field.Value, event.stash.fields[fieldServiceName].Value)
}

func TestEvent_AppendContext(t *testing.T) {
	t.Parallel()

	event := &event{
		stash: Stash{
			fields: make(map[fieldID]internal.Field),
		},
		logger: InitLogger(),
	}

	context := EventContext{
		"ctx1": "test",
	}
	event.AppendContext(context)

	require.Len(t, event.stash.fields, 1)
	require.Equal(t, context, event.stash.fields[fieldContext].Value)

	context = EventContext{
		"ctx2": "test2",
	}
	event.AppendContext(context)

	require.Len(t, event.stash.fields[fieldContext].Value, 2)
	require.Equal(t, EventContext{
		"ctx1": "test",
		"ctx2": "test2",
	}, event.stash.fields[fieldContext].Value)
}

func TestEvent_AppendError(t *testing.T) {
	t.Parallel()

	event := &event{
		stash: Stash{
			fields: make(map[fieldID]internal.Field),
		},
		logger: InitLogger(),
	}

	err := errors.New("test")
	event.AppendError(err)

	require.NotNil(t, event.err)
	require.Equal(t, err, event.err)
	require.Equal(t, StatusFailed, event.stash.fields[fieldStatus].Value)
}

func TestEvent_WithError(t *testing.T) {
	t.Parallel()

	event := &event{
		stash: Stash{
			fields: make(map[fieldID]internal.Field),
		},
		logger: InitLogger(),
		sender: &mockSender{},
	}

	testErr := errors.New("test")

	event.WithError(ErrCodeServiceNotFound, KindValidation, testErr)

	require.NotNil(t, event.err)
	require.Equal(t, ErrCodeServiceNotFound, event.stash.fields[fieldErrorCode].Value)
	require.Equal(t, KindValidation, event.stash.fields[fieldKind].Value)
	require.Equal(t, ErrLevelError, event.stash.fields[fieldLevel].Value)
	require.Equal(t, testErr, event.err)
	require.Equal(t, StatusFailed, event.stash.fields[fieldStatus].Value)
	require.Equal(t, "test", event.Error())
}

func TestEvent_Error(t *testing.T) {
	t.Parallel()

	event := &event{
		err: errors.New("test"),
	}

	require.Equal(t, "test", event.Error())

	event.err = nil
	require.Empty(t, event.Error())
}

func TestEnd(t *testing.T) {
	t.Parallel()

	cause := errors.New("test")
	fields := []internal.Field{StatusField(StatusFailed), KindField(KindValidation), Level(ErrLevelError)}
	ctxAndOperation := append([]internal.Field{Operation("test"), ContextField(EventContext{"ctx1": "test"}, Stash{})}, fields...)
	wantFields := map[fieldID]any{fieldStatus: StatusFailed, fieldKind: KindValidation, fieldLevel: ErrLevelError}
	expectFields := func(want map[fieldID]any) func(*mockSender) { return func(s *mockSender) { s.wantFields = want } }
	noSetup := func(*mockSender) {}

	success := func(t *testing.T, err error) { t.Helper(); require.NoError(t, err) }
	for _, tc := range []struct {
		name    string
		fields  []internal.Field
		cause   error
		output  io.Writer
		timeout time.Duration
		setup   func(*mockSender)
		calls   int
		want    func(*testing.T, error)
	}{
		{"no kind", []internal.Field{StatusField(StatusCompleted)}, nil, io.Discard, 0, noSetup, 0, success},
		{"missing level", []internal.Field{KindField(KindDomain)}, nil, io.Discard, 0, noSetup, 0, func(t *testing.T, err error) { t.Helper(); require.ErrorContains(t, err, "fieldLevel not found") }},
		{"failed with fields", ctxAndOperation, nil, io.Discard, 0, expectFields(map[fieldID]any{fieldStatus: StatusFailed, fieldKind: KindValidation, fieldLevel: ErrLevelError, fieldOperation: "test", fieldContext: EventContext{"ctx1": "test"}}), 1, success},
		{"failed with cause", fields, cause, io.Discard, 0, expectFields(map[fieldID]any{fieldStatus: StatusFailed, fieldKind: KindValidation, fieldLevel: ErrLevelError, fieldCause: "test"}), 1, success},
		{"started becomes completed", []internal.Field{StatusField(StatusStarted), KindField(KindDomain), Level(ErrLevelInfo)}, nil, io.Discard, 0, expectFields(map[fieldID]any{fieldStatus: StatusCompleted, fieldKind: KindDomain, fieldLevel: ErrLevelInfo}), 1, success},
		{"delivery after request cancellation", fields, nil, io.Discard, time.Second, func(s *mockSender) {
			s.wantFields = wantFields
			s.checkContext = func(ctx context.Context) {
				require.NoError(s.t, ctx.Err())
				_, ok := ctx.Deadline()
				require.True(s.t, ok)
			}
		}, 1, success},
		{"local log failure, broker available", fields, nil, brokenOutput{}, 0, expectFields(wantFields), 1, wantEndErrors(ErrLocalWrite, io.ErrClosedPipe)},
		{"local and broker failures", fields, nil, brokenOutput{}, 0, func(s *mockSender) { s.err = cause }, 1, wantEndErrors(ErrLocalWrite, cause)},
		{"sender failure", fields, nil, io.Discard, 0, func(s *mockSender) { s.err = cause }, 1, func(t *testing.T, err error) { t.Helper(); require.ErrorIs(t, err, cause) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sender := &mockSender{t: t}
			tc.setup(sender)

			e := &event{logger: newLogger(tc.output), sender: sender, deliveryTimeout: tc.timeout, err: tc.cause, stash: Stash{fields: make(map[fieldID]internal.Field)}}
			for _, f := range tc.fields {
				e.Append(f)
			}

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := e.End(ctx)
			tc.want(t, err)

			require.Equal(t, tc.calls, sender.sendCalls)
		})
	}
}

//nolint:funlen // длинный тест - это ок
func TestEvent_shouldSend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event *event
		want  bool
	}{
		{
			name: "include is empty -> allow all",
			event: &event{
				levels: struct {
					include []string
					exclude []string
				}{},
				kinds: struct {
					include []string
					exclude []string
				}{},
			},
			want: true,
		},
		{
			name: "kind in exclude -> block",
			event: &event{
				kinds: struct {
					include []string
					exclude []string
				}{
					include: []string{"validation"},
					exclude: []string{"validation"},
				},
				levels: struct {
					include []string
					exclude []string
				}{
					include: []string{"error"},
				},
			},
			want: false,
		},
		{
			name: "level in exclude -> block",
			event: &event{
				levels: struct {
					include []string
					exclude []string
				}{
					include: []string{"error"},
					exclude: []string{"error"},
				},
				kinds: struct {
					include []string
					exclude []string
				}{
					include: []string{"validation"},
				},
			},
			want: false,
		},
		{
			name: "kind include only and kind is allowed",
			event: &event{
				kinds: struct {
					include []string
					exclude []string
				}{
					include: []string{"validation"},
				},
			},
			want: true,
		},
		{
			name: "kind include only and kind is not allowed",
			event: &event{
				kinds: struct {
					include []string
					exclude []string
				}{
					include: []string{"technical"},
				},
			},
			want: false,
		},
		{
			name: "level include only and level is allowed",
			event: &event{
				levels: struct {
					include []string
					exclude []string
				}{
					include: []string{"error"},
				},
			},
			want: true,
		},
		{
			name: "kind and level includes are set and both match",
			event: &event{
				kinds: struct {
					include []string
					exclude []string
				}{
					include: []string{"validation"},
				},
				levels: struct {
					include []string
					exclude []string
				}{
					include: []string{"error"},
				},
			},
			want: true,
		},
		{
			name: "kind and level includes are set and level does not match",
			event: &event{
				kinds: struct {
					include []string
					exclude []string
				}{
					include: []string{"validation"},
				},
				levels: struct {
					include []string
					exclude []string
				}{
					include: []string{"warn"},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.event.shouldSend(KindValidation, ErrLevelError)
			require.Equal(t, tt.want, got)
		})
	}
}

func wantEndErrors(expected ...error) func(*testing.T, error) {
	return func(t *testing.T, err error) {
		t.Helper()

		for _, cause := range expected {
			require.ErrorIs(t, err, cause)
		}
	}
}

type mockSender struct {
	sent         map[fieldID]any
	err          error
	checkContext func(context.Context)
	t            *testing.T
	sendCalls    int
	wantFields   map[fieldID]any
}

func (ms *mockSender) Send(ctx context.Context, fields map[fieldID]any) error {
	ms.sent = fields
	ms.sendCalls++

	if ms.checkContext != nil {
		ms.checkContext(ctx)
	}

	if ms.wantFields != nil {
		require.Equal(ms.t, ms.wantFields, fields)
	}

	return ms.err
}

// brokenOutput позволяет проверить ошибку самой локальной записи.
type brokenOutput struct{}

func (brokenOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestLog(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{"written", io.Discard, nil}, {"writer unavailable", brokenOutput{}, ErrLocalWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := event{logger: newLogger(tc.writer)}
			require.ErrorIs(t, e.log(t.Context()), tc.want)
		})
	}
}
