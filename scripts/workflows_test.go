package scripts_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type workflow struct {
	On   map[string]workflowTrigger `yaml:"on"`
	Jobs map[string]workflowJob     `yaml:"jobs"`
}

type workflowTrigger struct {
	Branches []string                 `yaml:"branches"`
	Inputs   map[string]workflowInput `yaml:"inputs"`
}

type workflowInput struct {
	Type    string `yaml:"type"`
	Default string `yaml:"default"`
}

type workflowJob struct {
	Uses           string            `yaml:"uses"`
	Secrets        string            `yaml:"secrets"`
	Needs          []string          `yaml:"needs"`
	TimeoutMinutes int               `yaml:"timeout-minutes"`
	Permissions    map[string]string `yaml:"permissions"`
	Env            map[string]string `yaml:"env"`
	Steps          []workflowStep    `yaml:"steps"`
}

type workflowStep struct {
	ID   string            `yaml:"id"`
	Uses string            `yaml:"uses"`
	If   string            `yaml:"if"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]string `yaml:"with"`
}

func TestIntegrationWorkflowContract(t *testing.T) {
	t.Parallel()

	wf := readWorkflow(t, "pytests.yaml")
	assert.Equal(t, []string{"main"}, wf.On["pull_request"].Branches)
	for _, trigger := range []string{"workflow_call", "workflow_dispatch"} {
		input, ok := wf.On[trigger].Inputs["autotests_tag"]
		require.True(t, ok, "%s must accept the published test image tag", trigger)
		assert.Equal(t, workflowInput{Type: "string", Default: "latest"}, input)
	}

	job, ok := wf.Jobs["integration"]
	require.True(t, ok)
	assert.Equal(t, 30, job.TimeoutMinutes)
	assert.Equal(t, map[string]string{"contents": "read"}, job.Permissions)
	assert.Equal(t, map[string]string{
		"CI":              "true",
		"AUTH_IMAGE":      "${{ secrets.DOCKER_USERNAME }}/${{ secrets.DOCKER_REPO_NAME }}:test-${{ github.sha }}",
		"AUTOTESTS_IMAGE": "${{ secrets.PYTESTS_IMAGE_REPOSITORY }}:${{ inputs.autotests_tag || 'latest' }}",
		"AUTOTESTS_TAG":   "${{ inputs.autotests_tag || 'latest' }}",
		"RUN_ID":          "auth-${{ github.run_id }}-${{ github.run_attempt }}",
	}, job.Env)

	checkout := stepUsing(t, job, "actions/checkout@")
	assert.Equal(t, "recursive", checkout.With["submodules"])
	assert.Equal(t, "false", checkout.With["persist-credentials"])
	assert.Equal(t, "${{ secrets.CI_GITHUB_TOKEN || github.token }}", checkout.With["token"])

	_, buildIndex := stepRunning(t, job, "docker build")
	run, runIndex := stepRunning(t, job, "make integration-run")
	assert.Less(t, buildIndex, runIndex, "the runner must receive the image built from this revision")
	assert.Empty(t, run.If, "run the tests after a successful build")

	main := readWorkflow(t, "main.yaml")
	assert.Equal(t, "./.github/workflows/pytests.yaml", main.Jobs["pytests"].Uses)
	assert.Equal(t, "inherit", main.Jobs["pytests"].Secrets)
	assert.Contains(t, main.Jobs["build"].Needs, "pytests", "publishing must wait for integration tests")
}

func TestWorkflowImageSettingsValidation(t *testing.T) {
	t.Parallel()

	job := readWorkflow(t, "pytests.yaml").Jobs["integration"]
	validation, validationIndex := stepRunning(t, job, "DOCKER_USERNAME:?")
	_, buildIndex := stepRunning(t, job, "docker build")
	assert.Less(t, validationIndex, buildIndex)
	require.Equal(t, map[string]string{
		"DOCKER_USERNAME":          "${{ secrets.DOCKER_USERNAME }}",
		"DOCKER_REPO_NAME":         "${{ secrets.DOCKER_REPO_NAME }}",
		"PYTESTS_IMAGE_REPOSITORY": "${{ secrets.PYTESTS_IMAGE_REPOSITORY }}",
	}, validation.Env)

	for _, variable := range []string{"", "DOCKER_USERNAME", "DOCKER_REPO_NAME", "PYTESTS_IMAGE_REPOSITORY"} {
		for _, empty := range []bool{false, true} {
			t.Run(fmt.Sprintf("missing=%s/empty=%t", variable, empty), func(t *testing.T) {
				t.Parallel()

				fixture := newCommandFixture(t)
				for key, value := range map[string]string{
					"DOCKER_USERNAME": "synthetic-user", "DOCKER_REPO_NAME": "auth",
					"PYTESTS_IMAGE_REPOSITORY": "registry.example/tests",
				} {
					if key != variable {
						fixture.env = append(fixture.env, key+"="+value)
					} else if empty {
						fixture.env = append(fixture.env, key+"=")
					}
				}

				output, err := fixture.runShell(t, validation.Run)
				if variable == "" {
					require.NoError(t, err, output)
					assert.Empty(t, output)
				} else {
					require.Error(t, err)
					assert.Contains(t, output, variable)
				}
			})
		}
	}
}

func TestWorkflowDockerBuildCommands(t *testing.T) {
	t.Parallel()

	const commit = "0123456789abcdef0123456789abcdef01234567"
	const timestamp = "2026-10-14T12:34:56Z"
	const image = "registry.example/team/auth:test-revision"

	for _, tc := range []struct {
		file     string
		job      string
		imageEnv string
		metadata bool
	}{
		{file: "build.yaml", job: "build", imageEnv: "BUILD_IMAGE"},
		{file: "main.yaml", job: "build", imageEnv: "BUILD_IMAGE", metadata: true},
		{file: "pytests.yaml", job: "integration", imageEnv: "AUTH_IMAGE", metadata: true},
	} {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()

			job := readWorkflow(t, tc.file).Jobs[tc.job]
			step, _ := stepRunning(t, job, "docker build")
			if tc.imageEnv == "BUILD_IMAGE" {
				assert.Equal(t, "${{ secrets.DOCKER_USERNAME }}/${{ secrets.DOCKER_REPO_NAME }}:build-${{ github.sha }}", step.Env[tc.imageEnv])
			}
			fixture := newCommandFixture(t)
			fixture.recordTool(t, "docker")
			fixture.stubTool(t, "date", `test "$*" = '-u +%Y-%m-%dT%H:%M:%SZ'
printf '%s\n' '`+timestamp+`'
`)
			fixture.env = append(fixture.env, tc.imageEnv+"="+image, "GITHUB_SHA="+commit)

			output, err := fixture.runShell(t, step.Run)
			require.NoError(t, err, output)
			calls := fixture.lines(t, "commands.log")
			require.Len(t, calls, 1)
			args := strings.Split(calls[0], "\t")
			require.Equal(t, "build", args[0])
			assert.Equal(t, image, flagValue(t, args, "-t"))
			assert.Contains(t, args, ".", "build from the checked-out repository context")
			if tc.metadata {
				assert.Equal(t, []string{
					"VERSION=" + commit, "BUILD_DATE=" + timestamp, "GIT_COMMIT=" + commit,
				}, flagValues(t, args, "--build-arg"))
			}
		})
	}
}

func TestPublishedImageBuildMetadata(t *testing.T) {
	t.Parallel()

	job := readWorkflow(t, "main.yaml").Jobs["push-image"]
	timestampStep, timestampIndex := stepRunning(t, job, "GITHUB_OUTPUT")
	build := stepUsing(t, job, "docker/build-push-action@")
	assert.NotEmpty(t, timestampStep.ID)
	fixture := newCommandFixture(t)
	fixture.stubTool(t, "date", `test "$*" = '-u +%Y-%m-%dT%H:%M:%SZ'
printf '%s\n' '2026-10-14T12:34:56Z'
`)
	outputPath := filepath.Join(fixture.dir, "action output")
	writeTestFile(t, outputPath, "existing=preserved\n", 0o600)
	fixture.env = append(fixture.env, "GITHUB_OUTPUT="+outputPath)

	output, err := fixture.runShell(t, timestampStep.Run)
	require.NoError(t, err, output)
	assert.Equal(t, "existing=preserved\nvalue=2026-10-14T12:34:56Z\n", readTestFile(t, outputPath))
	assert.Equal(t, []string{
		"VERSION=${{ github.sha }}",
		"BUILD_DATE=${{ steps." + timestampStep.ID + ".outputs.value }}",
		"GIT_COMMIT=${{ github.sha }}",
	}, strings.Split(strings.TrimSpace(build.With["build-args"]), "\n"))
	assert.Equal(t, "true", build.With["push"])
	for i, step := range job.Steps {
		if step.Uses == build.Uses {
			assert.Less(t, timestampIndex, i, "generate the timestamp before publishing")
		}
	}
}

func TestWorkflowDiagnosticsAndCleanup(t *testing.T) {
	t.Parallel()

	job := readWorkflow(t, "pytests.yaml").Jobs["integration"]
	_, runIndex := stepRunning(t, job, "make integration-run")
	logs, logsIndex := stepRunning(t, job, "make integration-logs")
	down, downIndex := stepRunning(t, job, "make integration-down")
	assert.Less(t, runIndex, logsIndex)
	assert.Less(t, logsIndex, downIndex, "cleanup must be a separate step after diagnostics")
	assert.Equal(t, "always()", logs.If)
	assert.Equal(t, "always()", down.If)

	for _, tc := range []struct {
		name   string
		hasKit bool
		failAt string
	}{
		{name: "checkout failed"},
		{name: "runner finished", hasKit: true},
		{name: "diagnostics fail", hasKit: true, failAt: "integration-logs"},
		{name: "cleanup fails", hasKit: true, failAt: "integration-down"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := newCommandFixture(t)
			fixture.recordTool(t, "make")
			fixture.env = append(fixture.env, "FAIL_AT="+tc.failAt)
			if tc.hasKit {
				writeTestFile(t, filepath.Join(fixture.dir, "testkit", "Makefile"), "", 0o600)
			}
			for _, step := range []workflowStep{logs, down} {
				output, err := fixture.runShell(t, step.Run)
				if tc.failAt != "" && strings.Contains(step.Run, "make "+tc.failAt) {
					require.Error(t, err, output)
				} else {
					require.NoError(t, err, output)
				}
			}
			if tc.hasKit {
				assert.Equal(t, []string{"integration-logs", "integration-down"}, fixture.lines(t, "commands.log"))
			} else {
				_, err := os.Stat(filepath.Join(fixture.dir, "commands.log"))
				assert.ErrorIs(t, err, os.ErrNotExist, "no runner commands after a failed checkout")
			}
		})
	}
}

func readWorkflow(t *testing.T, name string) workflow {
	t.Helper()

	var wf workflow
	require.NoError(t, yaml.Unmarshal([]byte(readTestFile(t, filepath.Join("..", ".github", "workflows", name))), &wf))
	require.NotEmpty(t, wf.Jobs)

	return wf
}

func stepRunning(t *testing.T, job workflowJob, command string) (workflowStep, int) {
	t.Helper()

	for i, step := range job.Steps {
		if strings.Contains(step.Run, command) {
			return step, i
		}
	}
	t.Fatalf("missing workflow command %q", command)

	return workflowStep{}, -1
}

func stepUsing(t *testing.T, job workflowJob, action string) workflowStep {
	t.Helper()

	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, action) {
			return step
		}
	}
	t.Fatalf("missing action %q", action)

	return workflowStep{}
}

func (f *commandFixture) runShell(t *testing.T, script string) (string, error) {
	t.Helper()

	// Match Actions' non-interactive bash error and pipeline behavior.
	return f.run(t, "bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", script)
}

func flagValue(t *testing.T, args []string, flag string) string {
	t.Helper()

	values := flagValues(t, args, flag)
	require.Len(t, values, 1)

	return values[0]
}

func flagValues(t *testing.T, args []string, flag string) []string {
	t.Helper()

	var values []string
	for i, arg := range args {
		if arg == flag {
			require.Less(t, i+1, len(args), "%s needs a value", flag)
			values = append(values, args[i+1])
		}
	}

	return values
}
