package audit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStageString(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		value Stage
		want  string
	}{
		{StageReceiveRequest, "receive_request"},
		{StageAuthenticate, "authenticate"},
		{StageDecodeRequest, "decode_request"},
		{StageValidateRequest, "validate_request"},
		{StageVerifySubject, "verify_subject"},
		{StageWriteResponse, "write_response"},
		{0, "Stage(0)"},
		{7, "Stage(7)"},
		{255, "Stage(255)"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.value.String())
		})
	}
}

func TestContextErrorString(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		value ContextError
		want  string
	}{
		{ContextErrorCanceled, "canceled"},
		{ContextErrorDeadlineExceeded, "deadline_exceeded"},
		{0, "ContextError(0)"},
		{3, "ContextError(3)"},
		{255, "ContextError(255)"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.value.String())
		})
	}
}
