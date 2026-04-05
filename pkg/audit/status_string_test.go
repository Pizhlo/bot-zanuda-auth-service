package audit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{name: "StatusStarted", status: StatusStarted, want: "Started"},
		{name: "StatusCompleted", status: StatusCompleted, want: "Completed"},
		{name: "StatusCanceled", status: StatusCanceled, want: "Canceled"},
		{name: "StatusFailed", status: StatusFailed, want: "Failed"},
		{name: "StatusUnknown", status: Status(100), want: "Status(100)"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.status.String())
		})
	}
}
