package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(t *testing.T) {
	t.Parallel()

	const child = "BZ167_TEST_MAIN_CONFIG"
	if path := os.Getenv(child); path != "" {
		os.Args = []string{"auth-service", "-config", path}

		main()

		return
	}

	binary, err := os.Executable()
	require.NoError(t, err)

	//nolint:gosec // Запускается собственный тестовый бинарник из os.Executable() с фиксированным аргументом.
	cmd := exec.CommandContext(t.Context(), binary, "-test.run=^TestMain$")
	cmd.Env = append(os.Environ(), child+"="+filepath.Join(t.TempDir(), "missing.yaml"))

	var exit *exec.ExitError

	out, err := cmd.CombinedOutput()
	require.ErrorAs(t, err, &exit)

	require.Equal(t, 1, exit.ExitCode())
	require.Contains(t, string(out), "failed to load config")
}
