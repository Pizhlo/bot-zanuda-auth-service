package audit

import (
	"auth-service/pkg/audit/internal"
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

func TestEvent_Error(t *testing.T) {
	t.Parallel()

	event := &event{
		err: errors.New("test"),
	}

	require.Equal(t, "test", event.Error())

	event.err = nil
	require.Empty(t, event.Error())
}

func TestEvent_End(t *testing.T) {
	t.Parallel()

	event := &event{
		stash: Stash{
			fields: make(map[fieldID]internal.Field),
		},
		logger: InitLogger(),
	}

	event.Append(KindField(KindValidation))
	event.Append(StatusField(StatusStarted))
	event.End(t.Context())

	require.Len(t, event.stash.fields, 2)
	require.Equal(t, StatusCompleted, event.stash.fields[fieldStatus].Value)

	event.stash.fields = map[fieldID]internal.Field{
		fieldStatus: {
			FieldID: internal.FieldID(fieldStatus),
			Value:   StatusFailed,
		},
	}
	event.End(t.Context())

	require.Len(t, event.stash.fields, 1)
	require.Equal(t, StatusFailed, event.stash.fields[fieldStatus].Value)
}
