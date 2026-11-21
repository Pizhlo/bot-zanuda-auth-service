package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"auth-service/internal/config"
	"auth-service/internal/service/spacemigration"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // Таблица объединяет варианты одной операции с их входными данными и ожиданиями.
func TestRun(t *testing.T) {
	t.Parallel()

	emptyStore := newEmptyStoreServer(t)

	unavailableStore := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(unavailableStore.Close)
	path := filepath.Join(t.TempDir(), "export.json")
	require.NoError(t, os.WriteFile(path, []byte("[]"), 0600))

	args := func(url, store, input string) []string {
		return []string{"-api-url", url, "-store-id", store, "-model-id", "01ARZ3NDEKTSV4RRFFQ69G5FAV", "-input", input}
	}

	const store = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	brokenInput, brokenOutput := io.Pipe()
	require.NoError(t, brokenInput.CloseWithError(io.ErrClosedPipe))
	t.Cleanup(func() { _ = brokenOutput.Close() })

	emptyOutput := func(t *testing.T, output string) {
		t.Helper()
		require.Empty(t, output)
	}
	toBuffer := func(buf *bytes.Buffer) io.Writer { return buf }
	tests := []struct {
		name       string
		args       []string
		output     func(*bytes.Buffer) io.Writer
		wantErr    require.ErrorAssertionFunc
		wantOutput func(*testing.T, string)
	}{
		{
			name:       "invalid flags",
			output:     toBuffer,
			wantErr:    wantCLIError("parse migration arguments"),
			wantOutput: emptyOutput,
		},
		{
			name:   "missing export",
			args:   args(emptyStore.URL, store, path+".missing"),
			output: toBuffer,
			wantErr: func(t require.TestingT, err error, _ ...any) {
				require.ErrorIs(t, err, os.ErrNotExist)
			},
			wantOutput: emptyOutput,
		},
		{
			name:       "invalid API URL",
			args:       args("://", store, path),
			output:     toBuffer,
			wantErr:    wantCLIError("create migration FGA client"),
			wantOutput: emptyOutput,
		},
		{
			name:       "invalid store ID",
			args:       args(emptyStore.URL, "invalid", path),
			output:     toBuffer,
			wantErr:    wantCLIError("create migration FGA client"),
			wantOutput: emptyOutput,
		},
		{
			name:       "unavailable store",
			args:       args(unavailableStore.URL, store, path),
			output:     toBuffer,
			wantErr:    wantCLIError("execute migration plan"),
			wantOutput: emptyOutput,
		},
		{
			name:   "unwritable output",
			args:   args(emptyStore.URL, store, path),
			output: func(*bytes.Buffer) io.Writer { return brokenOutput },
			wantErr: func(t require.TestingT, err error, _ ...any) {
				require.ErrorIs(t, err, io.ErrClosedPipe)
				require.ErrorContains(t, err, "encode migration plan")
			},
			wantOutput: emptyOutput,
		},
		{
			name:    "dry run prints plan",
			args:    args(emptyStore.URL, store, path),
			output:  toBuffer,
			wantErr: require.NoError,
			wantOutput: func(t *testing.T, output string) {
				t.Helper()
				require.JSONEq(t, `{"spaces":0,"writes":[]}`, output)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			err := run(t.Context(), tc.args, tc.output(&buf))
			tc.wantErr(t, err)

			tc.wantOutput(t, buf.String())
		})
	}
}

func newEmptyStoreServer(t *testing.T) *httptest.Server {
	t.Helper()

	emptyStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"tuples":[],"continuation_token":""}`)
	}))
	t.Cleanup(emptyStore.Close)

	return emptyStore
}

func wantCLIError(message string) require.ErrorAssertionFunc {
	return func(t require.TestingT, err error, _ ...any) {
		require.ErrorContains(t, err, message)
	}
}

func TestReadExport(t *testing.T) {
	t.Parallel()

	rows := []spacemigration.Record{{SpaceID: uuid.New(), OwnerID: uuid.New(), SpaceType: "shared"}}
	data, err := json.Marshal(rows)
	require.NoError(t, err)

	tests := []struct {
		name, content string
		want          []spacemigration.Record
		wantErr       require.ErrorAssertionFunc
	}{
		{
			name:    "malformed JSON",
			content: "not json",
			wantErr: func(t require.TestingT, err error, _ ...any) { require.ErrorContains(t, err, "decode trusted export") },
		},
		{
			name:    "empty export",
			content: "[]",
			want:    []spacemigration.Record{},
			wantErr: require.NoError,
		},
		{
			name:    "space and owner from export",
			content: string(data),
			want:    rows,
			wantErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "export.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0600))

			records, err := readExport(path)
			tc.wantErr(t, err)

			require.Equal(t, tc.want, records)
		})
	}
}

//nolint:funlen // Таблица объединяет варианты одной операции с их входными данными и ожиданиями.
func TestParseArguments(t *testing.T) {
	t.Parallel()

	base := []string{"-api-url", "http://localhost", "-store-id", "store", "-model-id", "model", "-input", "export.json"}

	defaults := migrationArguments{apiURL: "http://localhost", storeID: "store", modelID: "model", input: "export.json",
		requestTimeout: config.DefaultOpenFGARequestTimeout, timeout: config.DefaultMigrationTimeout}
	explicit := defaults
	explicit.apply = true
	explicit.requestTimeout = 2 * time.Second
	explicit.timeout = 30 * time.Second

	with := func(flags ...string) []string { return append(slices.Clone(base), flags...) }

	tests := []struct {
		name    string
		flags   []string
		want    migrationArguments
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:    "missing required flags",
			wantErr: wantCLIError("are required"),
		},
		{
			name:    "unknown flag",
			flags:   []string{"-unknown"},
			wantErr: wantCLIError("parse flags"),
		},
		{
			name:    "positional argument",
			flags:   with("extra"),
			wantErr: wantCLIError("positional arguments"),
		},
		{
			name:    "defaults",
			flags:   base,
			want:    defaults,
			wantErr: require.NoError,
		},
		{
			name:    "explicit",
			flags:   with("-apply", "-request-timeout", "2s", "-timeout", "30s"),
			want:    explicit,
			wantErr: require.NoError,
		},
		{
			name:    "zero request",
			flags:   with("-request-timeout", "0s"),
			wantErr: wantCLIError("timeouts must be positive"),
		},
		{
			name:    "negative total",
			flags:   with("-timeout", "-1s"),
			wantErr: wantCLIError("timeouts must be positive"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			options, err := parseArguments(tt.flags)
			tt.wantErr(t, err)

			require.Equal(t, tt.want, options)
		})
	}
}

// TestExecute проверяет CLI на HTTP-заглушке пустого store, не запуская OpenFGA.
func TestExecute(t *testing.T) {
	t.Parallel()

	paths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.Method + " " + r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"tuples":[],"continuation_token":""}`)
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "export.json")
	require.NoError(t, os.WriteFile(path, []byte("[]"), 0600))

	const store = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	var output bytes.Buffer

	err := execute([]string{"-api-url", server.URL, "-store-id", store, "-model-id", store, "-input", path}, &output)
	require.NoError(t, err)

	require.JSONEq(t, `{"spaces":0,"writes":[]}`, output.String())
	require.Len(t, paths, 1)
	require.Equal(t, "POST /stores/"+store+"/read", <-paths)
}

func TestMain(t *testing.T) {
	t.Parallel()

	const child = "BZ167_TEST_CLI_MAIN"
	if os.Getenv(child) == "1" {
		os.Args = []string{"space-types"}

		main()

		return
	}

	binary, err := os.Executable()
	require.NoError(t, err)

	//nolint:gosec // Запускается собственный тестовый бинарник из os.Executable() с фиксированным аргументом.
	cmd := exec.CommandContext(t.Context(), binary, "-test.run=^TestMain$")

	cmd.Env = append(os.Environ(), child+"=1")

	var exit *exec.ExitError

	out, err := cmd.CombinedOutput()
	require.ErrorAs(t, err, &exit)

	require.Equal(t, 1, exit.ExitCode())
	require.Contains(t, string(out), "api-url, store-id, model-id and input are required")
}

func TestWaitForIO(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		operationErr error
		block        bool
		want         error
	}{
		{
			name: "finished",
		},
		{
			name:         "writer failed",
			operationErr: io.ErrClosedPipe,
			want:         io.ErrClosedPipe,
		},
		{
			name:  "blocked writer",
			block: true,
			want:  context.Canceled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			release := make(chan struct{})
			finished := make(chan struct{})
			err := waitForIO(ctx, func() error {
				defer close(finished)

				if tc.block {
					cancel()
					<-release
				}

				return tc.operationErr
			})

			close(release)
			<-finished

			require.ErrorIs(t, err, tc.want)
		})
	}
}
