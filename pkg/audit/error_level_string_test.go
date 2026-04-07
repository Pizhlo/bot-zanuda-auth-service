package audit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorLevelString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level ErrorLevel
		want  string
	}{
		{name: "ErrLevelDebug", level: ErrLevelDebug, want: "LevelDebug"},
		{name: "ErrorLevelInfo", level: ErrLevelInfo, want: "LevelInfo"},
		{name: "ErrorLevelWarn", level: ErrLevelWarn, want: "LevelWarn"},
		{name: "ErrorLevelError", level: ErrLevelError, want: "LevelError"},
		{name: "ErrorLevelFatal", level: ErrLevelFatal, want: "LevelFatal"},
		{name: "ErrorLevelPanic", level: ErrLevelPanic, want: "LevelPanic"},
		{name: "ErrorLevelUnknown", level: ErrorLevel(100), want: "ErrorLevel(100)"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.level.String())
		})
	}
}
