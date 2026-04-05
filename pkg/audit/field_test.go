package audit

import (
	"auth-service/pkg/audit/internal"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCurrentTime(t *testing.T) {
	t.Parallel()

	field := currentTime()

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldCurrentTime), field.FieldID)
	require.NotNil(t, field.Value)
}

func TestServiceName(t *testing.T) {
	t.Parallel()

	field := ServiceName("test")

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldServiceName), field.FieldID)
	require.Equal(t, "test", field.Value)
}

func TestLevel(t *testing.T) {
	t.Parallel()

	field := Level(ErrLevelDebug)

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldLevel), field.FieldID)
	require.Equal(t, ErrLevelDebug, field.Value)
}

func TestMessage(t *testing.T) {
	t.Parallel()

	field := Message("test")

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldMessage), field.FieldID)
	require.Equal(t, "test", field.Value)
}

func TestErrorCodeField(t *testing.T) {
	t.Parallel()

	field := ErrorCodeField(ErrCodeServiceNotFound)

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldErrorCode), field.FieldID)
	require.Equal(t, ErrCodeServiceNotFound, field.Value)
}

func TestTraceID(t *testing.T) {
	t.Parallel()

	field := TraceID("test")

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldTraceID), field.FieldID)
	require.Equal(t, "test", field.Value)
}

func TestRequestID(t *testing.T) {
	t.Parallel()

	field := RequestID("test")

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldRequestID), field.FieldID)
	require.Equal(t, "test", field.Value)
}

func TestUserID(t *testing.T) {
	t.Parallel()

	field := UserID("test")

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldUserID), field.FieldID)
	require.Equal(t, "test", field.Value)
}

func TestOperation(t *testing.T) {
	t.Parallel()

	field := Operation("test")

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldOperation), field.FieldID)
	require.Equal(t, "test", field.Value)
}

func TestStackTrace(t *testing.T) {
	t.Parallel()

	field := stackTrace("test")

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldStackTrace), field.FieldID)
	require.Equal(t, "test", field.Value)
}

func TestIPAddress(t *testing.T) {
	t.Parallel()

	field := IPAddress("test")

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldIPAddress), field.FieldID)
	require.Equal(t, "test", field.Value)
}

func TestContextField(t *testing.T) {
	t.Parallel()

	stash := Stash{
		fields: map[fieldID]internal.Field{},
	}

	field := ContextField(EventContext{
		"ctx1": "test",
	}, stash)

	stash = stash.Append(field)

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldContext), field.FieldID)
	require.Equal(t, EventContext{
		"ctx1": "test",
	}, field.Value)

	field = ContextField(EventContext{
		"ctx2": "test2",
	}, stash)

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldContext), field.FieldID)
	require.Equal(t, EventContext{
		"ctx1": "test",
		"ctx2": "test2",
	}, field.Value)
}

func TestVersion(t *testing.T) {
	t.Parallel()

	field := version("test")

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldVersion), field.FieldID)
	require.Equal(t, "test", field.Value)
}

func TestKindField(t *testing.T) {
	t.Parallel()

	field := KindField(KindValidation)

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldKind), field.FieldID)
	require.Equal(t, KindValidation, field.Value)
}

func TestCause(t *testing.T) {
	t.Parallel()

	field := cause(errors.New("test"))

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldCause), field.FieldID)
	require.Equal(t, errors.New("test"), field.Value)
}

func TestStatusField(t *testing.T) {
	t.Parallel()

	field := StatusField(StatusStarted)

	require.NotNil(t, field)
	require.Equal(t, internal.FieldID(fieldStatus), field.FieldID)
	require.Equal(t, StatusStarted, field.Value)
}
