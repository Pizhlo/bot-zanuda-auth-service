package audit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitLogger(t *testing.T) {
	t.Parallel()

	logger := InitLogger()
	require.NotNil(t, logger)

	require.NotNil(t, logger.Handler())
}
