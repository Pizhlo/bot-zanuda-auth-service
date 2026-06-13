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

//nolint:dupl // похожие функции - похожие тесты
func TestFieldID_MarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fieldID fieldID
		want    string
	}{
		{name: "fieldCurrentTime", fieldID: fieldCurrentTime, want: "current_time"},
		{name: "fieldServiceName", fieldID: fieldServiceName, want: "service_name"},
		{name: "fieldLevel", fieldID: fieldLevel, want: "level"},
		{name: "fieldMessage", fieldID: fieldMessage, want: "message"},
		{name: "fieldErrorCode", fieldID: fieldErrorCode, want: "error_code"},
		{name: "fieldTraceID", fieldID: fieldTraceID, want: "trace_id"},
		{name: "fieldRequestID", fieldID: fieldRequestID, want: "request_id"},
		{name: "fieldUserID", fieldID: fieldUserID, want: "user_id"},
		{name: "fieldStackTrace", fieldID: fieldStackTrace, want: "stack_trace"},
		{name: "fieldContext", fieldID: fieldContext, want: "context"},
		{name: "fieldVersion", fieldID: fieldVersion, want: "version"},
		{name: "fieldKind", fieldID: fieldKind, want: "kind"},
		{name: "fieldCause", fieldID: fieldCause, want: "cause"},
		{name: "fieldIPAddress", fieldID: fieldIPAddress, want: "ip_address"},
		{name: "fieldOperation", fieldID: fieldOperation, want: "operation"},
		{name: "fieldStatus", fieldID: fieldStatus, want: "status"},
		{name: "fieldUnknown", fieldID: fieldID(100), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			txt, err := tt.fieldID.MarshalText()
			require.NoError(t, err)
			require.Equal(t, tt.want, string(txt))
		})
	}
}

func TestFieldID_UnmarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fieldID fieldID
		want    fieldID
		wantErr require.ErrorAssertionFunc
	}{
		{name: "current_time", fieldID: fieldCurrentTime, want: fieldCurrentTime, wantErr: require.NoError},
		{name: "service_name", fieldID: fieldServiceName, want: fieldServiceName, wantErr: require.NoError},
		{name: "level", fieldID: fieldLevel, want: fieldLevel, wantErr: require.NoError},
		{name: "message", fieldID: fieldMessage, want: fieldMessage, wantErr: require.NoError},
		{name: "error_code", fieldID: fieldErrorCode, want: fieldErrorCode, wantErr: require.NoError},
		{name: "trace_id", fieldID: fieldTraceID, want: fieldTraceID, wantErr: require.NoError},
		{name: "request_id", fieldID: fieldRequestID, want: fieldRequestID, wantErr: require.NoError},
		{name: "user_id", fieldID: fieldUserID, want: fieldUserID, wantErr: require.NoError},
		{name: "stack_trace", fieldID: fieldStackTrace, want: fieldStackTrace, wantErr: require.NoError},
		{name: "context", fieldID: fieldContext, want: fieldContext, wantErr: require.NoError},
		{name: "version", fieldID: fieldVersion, want: fieldVersion, wantErr: require.NoError},
		{name: "kind", fieldID: fieldKind, want: fieldKind, wantErr: require.NoError},
		{name: "cause", fieldID: fieldCause, want: fieldCause, wantErr: require.NoError},
		{name: "ip_address", fieldID: fieldIPAddress, want: fieldIPAddress, wantErr: require.NoError},
		{name: "operation", fieldID: fieldOperation, want: fieldOperation, wantErr: require.NoError},
		{name: "status", fieldID: fieldStatus, want: fieldStatus, wantErr: require.NoError},
		{name: "unknown", fieldID: fieldID(100), want: fieldID(0), wantErr: func(t require.TestingT, err error, i ...interface{}) {
			require.Error(t, err)
			require.ErrorContains(t, err, "unknown fieldID: unknown")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var actual fieldID

			err := actual.UnmarshalText([]byte(tt.name))
			tt.wantErr(t, err)

			require.Equal(t, tt.want, actual)
		})
	}
}

func TestErrorLevel_MarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level ErrorLevel
		want  string
	}{
		{name: "debug", level: ErrLevelDebug, want: "debug"},
		{name: "info", level: ErrLevelInfo, want: "info"},
		{name: "warn", level: ErrLevelWarn, want: "warn"},
		{name: "error", level: ErrLevelError, want: "error"},
		{name: "fatal", level: ErrLevelFatal, want: "fatal"},
		{name: "panic", level: ErrLevelPanic, want: "panic"},
		{name: "unknown", level: ErrorLevel(100), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			txt, err := tt.level.MarshalText()
			require.NoError(t, err)
			require.Equal(t, tt.want, string(txt))
		})
	}
}

func TestErrorLevel_UnmarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		level   ErrorLevel
		want    ErrorLevel
		wantErr require.ErrorAssertionFunc
	}{
		{name: "debug", level: ErrLevelDebug, want: ErrLevelDebug, wantErr: require.NoError},
		{name: "info", level: ErrLevelInfo, want: ErrLevelInfo, wantErr: require.NoError},
		{name: "warn", level: ErrLevelWarn, want: ErrLevelWarn, wantErr: require.NoError},
		{name: "error", level: ErrLevelError, want: ErrLevelError, wantErr: require.NoError},
		{name: "fatal", level: ErrLevelFatal, want: ErrLevelFatal, wantErr: require.NoError},
		{name: "panic", level: ErrLevelPanic, want: ErrLevelPanic, wantErr: require.NoError},
		{name: "unknown", level: ErrorLevel(100), want: 0, wantErr: func(t require.TestingT, err error, i ...interface{}) {
			require.Error(t, err)
			require.ErrorContains(t, err, "unknown ErrorLevel: unknown")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var actual ErrorLevel

			err := actual.UnmarshalText([]byte(tt.name))
			tt.wantErr(t, err)

			require.Equal(t, tt.want, actual)
		})
	}
}

func TestKind_MarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind Kind
		want string
	}{
		{name: "validation", kind: KindValidation, want: "validation"},
		{name: "domain", kind: KindDomain, want: "domain"},
		{name: "infra", kind: KindInfra, want: "infra"},
		{name: "external", kind: KindExternal, want: "external"},
		{name: "internal", kind: KindInternal, want: "internal"},
		{name: "unknown", kind: Kind(100), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			txt, err := tt.kind.MarshalText()
			require.NoError(t, err)
			require.Equal(t, tt.want, string(txt))
		})
	}
}

func TestKind_UnmarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    Kind
		want    Kind
		wantErr require.ErrorAssertionFunc
	}{
		{name: "validation", kind: KindValidation, want: KindValidation, wantErr: require.NoError},
		{name: "domain", kind: KindDomain, want: KindDomain, wantErr: require.NoError},
		{name: "infra", kind: KindInfra, want: KindInfra, wantErr: require.NoError},
		{name: "external", kind: KindExternal, want: KindExternal, wantErr: require.NoError},
		{name: "internal", kind: KindInternal, want: KindInternal, wantErr: require.NoError},
		{name: "unknown", kind: Kind(100), want: 0, wantErr: func(t require.TestingT, err error, i ...interface{}) {
			require.Error(t, err)
			require.ErrorContains(t, err, "unknown Kind: unknown")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var actual Kind

			err := actual.UnmarshalText([]byte(tt.name))
			tt.wantErr(t, err)

			require.Equal(t, tt.want, actual)
		})
	}
}

func TestStatus_MarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{name: "started", status: StatusStarted, want: "started"},
		{name: "completed", status: StatusCompleted, want: "completed"},
		{name: "canceled", status: StatusCanceled, want: "canceled"},
		{name: "failed", status: StatusFailed, want: "failed"},
		{name: "unknown", status: Status(100), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			txt, err := tt.status.MarshalText()
			require.NoError(t, err)
			require.Equal(t, tt.want, string(txt))
		})
	}
}

func TestStatus_UnmarshalText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  Status
		want    Status
		wantErr require.ErrorAssertionFunc
	}{
		{name: "started", status: StatusStarted, want: StatusStarted, wantErr: require.NoError},
		{name: "completed", status: StatusCompleted, want: StatusCompleted, wantErr: require.NoError},
		{name: "canceled", status: StatusCanceled, want: StatusCanceled, wantErr: require.NoError},
		{name: "failed", status: StatusFailed, want: StatusFailed, wantErr: require.NoError},
		{name: "unknown", status: Status(100), want: Status(0), wantErr: func(t require.TestingT, err error, i ...interface{}) {
			require.Error(t, err)
			require.ErrorContains(t, err, "unknown Status: unknown")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var actual Status

			err := actual.UnmarshalText([]byte(tt.name))
			tt.wantErr(t, err)

			require.Equal(t, tt.want, actual)
		})
	}
}

//nolint:dupl // похожие функции - похожие тесты
func TestFieldIDString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fieldID fieldID
		want    string
	}{
		{name: "fieldCurrentTime", fieldID: fieldCurrentTime, want: "current_time"},
		{name: "fieldServiceName", fieldID: fieldServiceName, want: "service_name"},
		{name: "fieldLevel", fieldID: fieldLevel, want: "level"},
		{name: "fieldMessage", fieldID: fieldMessage, want: "message"},
		{name: "fieldErrorCode", fieldID: fieldErrorCode, want: "error_code"},
		{name: "fieldTraceID", fieldID: fieldTraceID, want: "trace_id"},
		{name: "fieldRequestID", fieldID: fieldRequestID, want: "request_id"},
		{name: "fieldUserID", fieldID: fieldUserID, want: "user_id"},
		{name: "fieldStackTrace", fieldID: fieldStackTrace, want: "stack_trace"},
		{name: "fieldContext", fieldID: fieldContext, want: "context"},
		{name: "fieldVersion", fieldID: fieldVersion, want: "version"},
		{name: "fieldKind", fieldID: fieldKind, want: "kind"},
		{name: "fieldCause", fieldID: fieldCause, want: "cause"},
		{name: "fieldIPAddress", fieldID: fieldIPAddress, want: "ip_address"},
		{name: "fieldOperation", fieldID: fieldOperation, want: "operation"},
		{name: "fieldStatus", fieldID: fieldStatus, want: "status"},
		{name: "fieldUnknown", fieldID: fieldID(100), want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.fieldID.String())
		})
	}
}

func TestKindString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind Kind
		want string
	}{
		{name: "validation", kind: KindValidation, want: "validation"},
		{name: "domain", kind: KindDomain, want: "domain"},
		{name: "infra", kind: KindInfra, want: "infra"},
		{name: "external", kind: KindExternal, want: "external"},
		{name: "internal", kind: KindInternal, want: "internal"},
		{name: "unknown", kind: Kind(100), want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.kind.String())
		})
	}
}

func TestErrorLevelString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level ErrorLevel
		want  string
	}{
		{name: "debug", level: ErrLevelDebug, want: "debug"},
		{name: "info", level: ErrLevelInfo, want: "info"},
		{name: "warn", level: ErrLevelWarn, want: "warn"},
		{name: "error", level: ErrLevelError, want: "error"},
		{name: "fatal", level: ErrLevelFatal, want: "fatal"},
		{name: "panic", level: ErrLevelPanic, want: "panic"},
		{name: "unknown", level: ErrorLevel(100), want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.level.String())
		})
	}
}

func TestStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{name: "started", status: StatusStarted, want: "started"},
		{name: "completed", status: StatusCompleted, want: "completed"},
		{name: "canceled", status: StatusCanceled, want: "canceled"},
		{name: "failed", status: StatusFailed, want: "failed"},
		{name: "unknown", status: Status(100), want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.status.String())
		})
	}
}
