package audit

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitLogger(t *testing.T) {
	t.Parallel()

	logger := InitLogger()
	require.NotNil(t, logger)

	require.NotNil(t, logger.Handler())
}

func TestNewLogger(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		level slog.Level
		attrs []any
		want  string
	}{
		{"audit", LevelAudit, nil, "level=AUDIT"},
		{"known error", slog.LevelError, nil, "level=ERROR"},
		{"custom level", slog.Level(9), nil, "level=ERROR+1"},
		{"level attribute supplied as a string", LevelAudit, []any{"level", "custom"}, "level=custom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer

			logger := newLogger(&output)
			logger.Log(t.Context(), tc.level, "audit message", tc.attrs...)

			require.Contains(t, output.String(), "audit message")
			require.Contains(t, output.String(), tc.want)
			require.NotContains(t, output.String(), "time=")
		})
	}
}
