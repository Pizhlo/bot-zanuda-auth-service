package audit

import (
	"auth-service/pkg/audit/internal"
	"context"
	"errors"
	"testing"

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

//nolint:funlen // длинный тест - это ок
func TestEvent_End(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		event      *event
		fields     map[fieldID]internal.Field
		wantFields map[fieldID]any
		expected   int
	}{
		{
			name: "success",
			event: &event{
				stash: Stash{
					fields: map[fieldID]internal.Field{
						fieldStatus: {
							FieldID: internal.FieldID(fieldStatus),
							Value:   StatusCompleted,
						},
						fieldContext: {
							FieldID: internal.FieldID(fieldContext),
							Value:   EventContext{"ctx1": "test"},
						},
						fieldOperation: {
							FieldID: internal.FieldID(fieldOperation),
							Value:   "test",
						},
						fieldCause: {
							FieldID: internal.FieldID(fieldCause),
							Value:   errors.New("test"),
						},
					},
				},
				logger: InitLogger(),
			},
			wantFields: nil,
			expected:   0,
		},
		{
			name: "failed",
			event: &event{
				stash: Stash{
					fields: map[fieldID]internal.Field{
						fieldStatus: {
							FieldID: internal.FieldID(fieldStatus),
							Value:   StatusFailed,
						},
						fieldContext: {
							FieldID: internal.FieldID(fieldContext),
							Value:   EventContext{"ctx1": "test"},
						},
						fieldOperation: {
							FieldID: internal.FieldID(fieldOperation),
							Value:   "test",
						},
						fieldKind: {
							FieldID: internal.FieldID(fieldKind),
							Value:   KindValidation,
						},
						fieldLevel: {
							FieldID: internal.FieldID(fieldLevel),
							Value:   ErrLevelError,
						},
					},
				},
				logger: InitLogger(),
				sender: &mockSender{},
			},
			wantFields: map[fieldID]any{
				fieldLevel:     ErrLevelError,
				fieldKind:      KindValidation,
				fieldOperation: "test",
				fieldStatus:    StatusFailed,
				fieldContext:   EventContext{"ctx1": "test"},
			},
			expected: 1,
		},
		{
			name: "failed with error",
			event: &event{
				err: errors.New("test"),
				stash: Stash{
					fields: map[fieldID]internal.Field{
						fieldStatus: {
							FieldID: internal.FieldID(fieldStatus),
							Value:   StatusFailed,
						},
						fieldKind: {
							FieldID: internal.FieldID(fieldKind),
							Value:   KindValidation,
						},
						fieldLevel: {
							FieldID: internal.FieldID(fieldLevel),
							Value:   ErrLevelError,
						},
					},
				},
				logger: InitLogger(),
				sender: &mockSender{},
			},
			wantFields: map[fieldID]any{
				fieldStatus: StatusFailed,
				fieldKind:   KindValidation,
				fieldLevel:  ErrLevelError,
				fieldCause:  "test",
			},
			expected: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			sender := &mockSender{
				t:          t,
				wantFields: test.wantFields,
			}

			test.event.sender = sender

			err := test.event.End(t.Context())
			require.NoError(t, err)

			require.Equal(t, test.expected, sender.sendCalls)
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

type mockSender struct {
	t          *testing.T
	sendCalls  int
	wantFields map[fieldID]any
}

func (ms *mockSender) Send(ctx context.Context, fields map[fieldID]any) error {
	ms.sendCalls++

	if ms.wantFields != nil {
		require.Equal(ms.t, ms.wantFields, fields)
	}

	return nil
}
