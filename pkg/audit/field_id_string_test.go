package audit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFieldIDString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fieldID fieldID
		want    string
	}{
		{name: "fieldCurrentTime", fieldID: fieldCurrentTime, want: "CurrentTime"},
		{name: "fieldServiceName", fieldID: fieldServiceName, want: "ServiceName"},
		{name: "fieldLevel", fieldID: fieldLevel, want: "Level"},
		{name: "fieldMessage", fieldID: fieldMessage, want: "Message"},
		{name: "fieldErrorCode", fieldID: fieldErrorCode, want: "ErrorCode"},
		{name: "fieldTraceID", fieldID: fieldTraceID, want: "TraceID"},
		{name: "fieldRequestID", fieldID: fieldRequestID, want: "RequestID"},
		{name: "fieldUserID", fieldID: fieldUserID, want: "UserID"},
		{name: "fieldStackTrace", fieldID: fieldStackTrace, want: "StackTrace"},
		{name: "fieldContext", fieldID: fieldContext, want: "Context"},
		{name: "fieldVersion", fieldID: fieldVersion, want: "Version"},
		{name: "fieldKind", fieldID: fieldKind, want: "Kind"},
		{name: "fieldCause", fieldID: fieldCause, want: "Cause"},
		{name: "fieldIPAddress", fieldID: fieldIPAddress, want: "IPAddress"},
		{name: "fieldOperation", fieldID: fieldOperation, want: "Operation"},
		{name: "fieldStatus", fieldID: fieldStatus, want: "Status"},
		{name: "fieldUnknown", fieldID: fieldID(100), want: "fieldID(100)"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.fieldID.String())
		})
	}
}
