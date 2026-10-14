package scripts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the auth-service adapter, not the external testkit runner.
// All commands run in temporary directories with synthetic inputs and stub tools.
func TestIntegrationTargetsDelegate(t *testing.T) {
	t.Parallel()

	for _, target := range []string{
		"integration-up", "integration-run", "integration-seed", "integration-test",
		"integration-down", "integration-logs", "integration-rabbitmq-setup",
	} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			fixture := newCommandFixture(t)
			fixture.copyMakefile(t)
			fixture.addTestkit(t, filepath.Join(fixture.dir, "testkit"))
			// A file named after the target must not suppress a phony recipe.
			writeTestFile(t, filepath.Join(fixture.dir, target), "", 0o600)
			writeTestFile(t, filepath.Join(fixture.dir, ".env"), "LOCAL_SETTING=keep\n", 0o600)
			writeTestFile(t, filepath.Join(fixture.dir, "config.yaml"), "local: keep\n", 0o600)

			output, err := fixture.run(t, "make", target)
			require.NoError(t, err, output)
			assert.Equal(t, []string{
				"target=" + target,
				"working_dir=" + filepath.Join(fixture.dir, "testkit"),
				"root=" + fixture.dir,
				"context=auth-service",
				"pytests=" + filepath.Join(filepath.Dir(fixture.dir), "pytests"),
				"auth_mode=", "test_mode=", "pytest_config=", "seed_data=", "run_id=",
			}, fixture.lines(t, "testkit.log"))
			assert.Equal(t, "LOCAL_SETTING=keep\n", readTestFile(t, filepath.Join(fixture.dir, ".env")))
			assert.Equal(t, "local: keep\n", readTestFile(t, filepath.Join(fixture.dir, "config.yaml")))
		})
	}
}

func TestIntegrationOverridesReachChildMake(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"environment", "command line"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()

			fixture := newCommandFixture(t)
			fixture.copyMakefile(t)
			kitDir := filepath.Join(fixture.dir, "custom testkit")
			pytestsDir := filepath.Join(fixture.dir, "local pytests")
			config := filepath.Join(pytestsDir, "custom config.yaml")
			fixture.addTestkit(t, kitDir)
			writeTestFile(t, config, "local: unchanged\n", 0o600)
			overrides := []string{
				"TESTKIT_DIR=" + kitDir, "PYTESTS_DIR=" + pytestsDir,
				"AUTH_MODE=host", "TEST_MODE=host", "PYTEST_CONFIG=" + config, "SEED_DATA=false",
			}
			args := []string{"integration-run"}
			if source == "environment" {
				fixture.env = append(fixture.env, overrides...)
			} else {
				// Command-line settings must take precedence over the environment.
				fixture.env = append(fixture.env, "PYTESTS_DIR=/unused", "AUTH_MODE=container")
				args = append(args, overrides...)
			}

			output, err := fixture.run(t, "make", args...)
			require.NoError(t, err, output)
			assert.Equal(t, []string{
				"target=integration-run", "working_dir=" + kitDir,
				"root=" + fixture.dir, "context=auth-service", "pytests=" + pytestsDir,
				"auth_mode=host", "test_mode=host", "pytest_config=" + config,
				"seed_data=false", "run_id=",
			}, fixture.lines(t, "testkit.log"))
			assert.Equal(t, "local: unchanged\n", readTestFile(t, config))
		})
	}
}

func TestIntegrationUsesCallingDirectory(t *testing.T) {
	t.Parallel()

	fixture := newCommandFixture(t)
	fixture.copyMakefile(t)
	caller := filepath.Join(fixture.dir, "caller")
	fixture.addTestkit(t, filepath.Join(caller, "testkit"))
	makefile := filepath.Join(fixture.dir, "Makefile")
	fixture.dir = caller

	output, err := fixture.run(t, "make", "-f", makefile, "integration-seed")
	require.NoError(t, err, output)
	assert.Contains(t, fixture.lines(t, "testkit.log"), "root="+caller)
	assert.Contains(t, fixture.lines(t, "testkit.log"), "pytests="+filepath.Join(filepath.Dir(caller), "pytests"))
}

func TestIntegrationFailuresPropagate(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"missing testkit", "failed child target"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			fixture := newCommandFixture(t)
			fixture.copyMakefile(t)
			if scenario == "failed child target" {
				fixture.addTestkit(t, filepath.Join(fixture.dir, "testkit"))
				fixture.env = append(fixture.env, "CHILD_EXIT=23")
			}

			output, err := fixture.run(t, "make", "integration-test")
			require.Error(t, err, output)
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			assert.NotZero(t, exitErr.ExitCode())
			if scenario == "failed child target" {
				assert.Contains(t, output, "Error 23")
				assert.Contains(t, fixture.lines(t, "testkit.log"), "target=integration-test")
			} else {
				_, statErr := os.Stat(filepath.Join(fixture.dir, "testkit.log"))
				assert.ErrorIs(t, statErr, os.ErrNotExist)
			}
		})
	}
}

func TestContainersUpUsesLocalFGAMigrationAndStopsOnFailure(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		rabbitmq string
		failAt   string
	}{
		{name: "default includes rabbitmq"},
		{name: "rabbitmq disabled", rabbitmq: "false"},
		{name: "vault setup fails", failAt: "setup-vault"},
		{name: "fga migration fails", failAt: "fga-migrate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := newCommandFixture(t)
			fixture.copyMakefile(t)
			fixture.recordTool(t, "docker")
			fixture.env = append(fixture.env, "FAIL_AT="+tc.failAt)
			args := []string{"containers-up"}
			if tc.rabbitmq != "" {
				args = append(args, "RABBITMQ="+tc.rabbitmq)
			}

			output, err := fixture.run(t, "make", args...)
			want := []string{
				"compose\t-f\tdocker-compose.yaml\t--profile\tinit\trun\t--rm\tsetup-vault",
				"compose\t-f\tdocker-compose.yaml\t--profile\tinit\trun\t--rm\tfga-migrate",
			}
			if tc.failAt != "" {
				require.Error(t, err, output)
				if tc.failAt == "setup-vault" {
					want = want[:1]
				}
			} else {
				require.NoError(t, err, output)
				want = append(want, "compose\t-f\tdocker-compose.yaml\tup\t-d\tpostgres-fga\topen-fga")
				services := "postgres\tvault\tredis\topen-fga"
				if tc.rabbitmq != "false" {
					want = append(want, "compose\t-f\tdocker-compose.yaml\t--profile\tinit\trun\t--rm\trabbitmq-setup")
					services += "\trabbitmq"
				}
				want = append(want, "compose\tup\t-d\t"+services)
			}
			assert.Equal(t, want, fixture.lines(t, "commands.log"))
		})
	}
}

type commandFixture struct {
	dir string
	bin string
	env []string
}

func newCommandFixture(t *testing.T) *commandFixture {
	t.Helper()

	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	require.NoError(t, os.Mkdir(bin, 0o700))

	// Do not inherit developer credentials, MAKEFLAGS, or integration settings.
	return &commandFixture{
		dir: dir, bin: bin,
		env: []string{"PATH=" + bin + ":/usr/bin:/bin", "LANG=C", "COMMAND_LOG=" + filepath.Join(dir, "commands.log")},
	}
}

func (f *commandFixture) run(t *testing.T, command string, args ...string) (string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = f.dir
	cmd.Env = f.env
	output, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err(), "command timed out: %s %v\n%s", command, args, output)

	return string(output), err
}

func (f *commandFixture) copyMakefile(t *testing.T) {
	t.Helper()

	writeTestFile(t, filepath.Join(f.dir, "Makefile"), readTestFile(t, "../Makefile"), 0o600)
}

func (f *commandFixture) addTestkit(t *testing.T, dir string) {
	t.Helper()

	// Read exported variables in the child shell, rather than expanding Make variables.
	writeTestFile(t, filepath.Join(dir, "Makefile"), `$(MAKECMDGOALS):
	@printf '%s\n' "target=$@" "working_dir=$(CURDIR)" \
	  "root=$$INTEGRATION_ROOT" "context=$$INTEGRATION_CONTEXT" "pytests=$$PYTESTS_DIR" \
	  "auth_mode=$$AUTH_MODE" "test_mode=$$TEST_MODE" "pytest_config=$$PYTEST_CONFIG" \
	  "seed_data=$$SEED_DATA" "run_id=$$RUN_ID" > ../testkit.log
	@exit $${CHILD_EXIT:-0}
`, 0o600)
}

func (f *commandFixture) recordTool(t *testing.T, name string) {
	t.Helper()

	f.stubTool(t, name, `printf '%s\t' "$@" >> "$COMMAND_LOG"
printf '\n' >> "$COMMAND_LOG"
for arg do
  if [ -n "${FAIL_AT:-}" ] && [ "$arg" = "$FAIL_AT" ]; then exit 17; fi
done
`)
}

func (f *commandFixture) stubTool(t *testing.T, name, body string) {
	t.Helper()

	writeTestFile(t, filepath.Join(f.bin, name), "#!/bin/sh\nset -eu\n"+body, 0o700)
}

func (f *commandFixture) lines(t *testing.T, name string) []string {
	t.Helper()

	content := strings.TrimSuffix(readTestFile(t, filepath.Join(f.dir, name)), "\n")
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\t")
	}

	return lines
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(content)
}
