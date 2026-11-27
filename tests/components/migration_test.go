//go:build integration

package components_test

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	fgamodel "auth-service/fga"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func migrationInput(t *testing.T, s store, n int) []map[string]string {
	t.Helper()

	owner := uuid.NewString()
	var keys []key
	rows := make([]map[string]string, n)
	for i := range rows {
		id := uuid.NewString()
		rows[i] = map[string]string{"space_id": id, "owner_id": owner, "space_type": "shared"}
		keys = append(keys, key{"user:" + owner, "owner", "space:" + id})
	}
	rows[0]["space_type"] = "personal"
	keys = append(keys, key{"user:*", "personal", "space:" + rows[0]["space_id"]})
	s.write(t, keys)

	return rows
}

func dumpFile(t *testing.T, rows []map[string]string) string {
	t.Helper()

	data, err := json.Marshal(rows)
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "spaces.json")
	require.NoError(t, os.WriteFile(file, data, 0600))

	return file
}

func cli(t *testing.T, s store, endpoint, file string, args ...string) (string, string, int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	args = append([]string{"-api-url", endpoint, "-store-id", s.id, "-model-id", s.model, "-input", file}, args...)
	cmd := exec.CommandContext(ctx, "/components/space-types", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	require.NoError(t, ctx.Err(), "CLI exceeded the outer test deadline")
	if err != nil {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		return stdout.String(), stderr.String(), exit.ExitCode()
	}
	return stdout.String(), stderr.String(), 0
}

// TestMigrationRejectsInvalidExport проверяет отказ от миграции при неполной выгрузке,
// неверном владельце или противоречивой классификации пространств. CLI должна сообщить
// причину отказа до первой попытки записи в OpenFGA и сохранить весь исходный граф.
func TestMigrationRejectsInvalidExport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, diagnostic string
		corrupt          func([]map[string]string) []map[string]string
	}{
		{
			name:       "missing type",
			diagnostic: "invalid export record",
			corrupt: func(rows []map[string]string) []map[string]string {
				delete(rows[0], "space_type")
				return rows
			},
		},
		{
			name:       "incomplete",
			diagnostic: "incomplete export",
			corrupt: func(rows []map[string]string) []map[string]string {
				return rows[:1]
			},
		},
		{
			name:       "wrong type",
			diagnostic: "space type conflict",
			corrupt: func(rows []map[string]string) []map[string]string {
				rows[0]["space_type"] = "shared"
				return rows
			},
		},
		{
			name:       "wrong owner",
			diagnostic: "space or owner mismatch",
			corrupt: func(rows []map[string]string) []map[string]string {
				rows[0]["owner_id"] = uuid.NewString()
				return rows
			},
		},
		{
			name:       "two personal",
			diagnostic: "multiple personal spaces",
			corrupt: func(rows []map[string]string) []map[string]string {
				rows[1]["space_type"] = "personal"
				return rows
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newStore(t, fgamodel.Transition)
			rows := migrationInput(t, s, 2)
			graph := s.graph(t)
			trace := observe(t)
			file := dumpFile(t, tc.corrupt(rows))

			out, stderr, code := cli(t, s, trace.url, file, "-apply")

			require.Equal(t, 1, code)
			require.Empty(t, out)
			require.Contains(t, stderr, tc.diagnostic)
			require.Empty(t, trace.take("/write"))
			require.Equal(t, graph, s.graph(t))
		})
	}
}

// TestMigrationPagesAndRepeatedApply проверяет миграцию выгрузки, для которой нужно
// прочитать несколько страниц графа и записать отметки несколькими запросами.
// Предварительный просмотр и применение должны дать одинаковый план. После применения
// все исходные связи сохранены, а повтор в любом режиме не пытается ничего записать.
func TestMigrationPagesAndRepeatedApply(t *testing.T) {
	t.Parallel()

	s := newStore(t, fgamodel.Transition)
	rows := migrationInput(t, s, 111)
	file := dumpFile(t, rows)
	initial := s.graph(t)
	trace := observe(t)

	dry, stderr, code := cli(t, s, trace.url, file)

	require.Zerof(t, code, "%s", stderr)
	require.Empty(t, trace.take("/write"))
	require.Equal(t, initial, s.graph(t))

	trace.reset()

	applied, stderr, code := cli(t, s, trace.url, file, "-apply")

	require.Zerof(t, code, "%s", stderr)
	require.JSONEq(t, dry, applied)
	require.GreaterOrEqual(t, len(trace.take("/write")), 2)

	continued := false
	for _, read := range trace.take("/read") {
		if token, _ := read["continuation_token"].(string); token != "" {
			continued = true
		}
	}
	require.True(t, continued, "expected a real continuation token")

	expected := maps.Clone(initial)
	for _, row := range rows {
		obj := "space:" + row["space_id"]
		expected[key{"user:*", "created", obj}] = true
		expected[key{"user:*", row["space_type"], obj}] = true
	}
	user := "user:" + rows[0]["owner_id"]
	expected[key{user, "has_personal_space", user}] = true
	require.Equal(t, expected, s.graph(t))

	type migrationPlan struct {
		Spaces int   `json:"spaces"`
		Writes []key `json:"writes"`
	}
	var planned migrationPlan
	require.NoError(t, json.Unmarshal([]byte(dry), &planned))
	require.Equal(t, len(rows), planned.Spaces)
	var missing []key
	for marker := range expected {
		if !initial[marker] {
			missing = append(missing, marker)
		}
	}
	require.ElementsMatch(t, missing, planned.Writes)

	for _, args := range [][]string{{"-apply"}, nil} {
		trace.reset()

		output, stderr, code := cli(t, s, trace.url, file, args...)

		require.Zerof(t, code, "%s", stderr)
		var plan migrationPlan
		require.NoError(t, json.Unmarshal([]byte(output), &plan))
		require.Equal(t, migrationPlan{Spaces: len(rows), Writes: []key{}}, plan)
		require.Empty(t, trace.take("/write"))
		require.Equal(t, expected, s.graph(t))
	}
}

// TestMigrationInputDeadline проверяет, что общий таймаут миграции действует уже при
// чтении выгрузки. Если источник не отдаёт данные, CLI должна вовремя вернуть ошибку,
// не вывести план и сохранить граф без изменений.
func TestMigrationInputDeadline(t *testing.T) {
	t.Parallel()

	s := newStore(t, fgamodel.Transition)
	migrationInput(t, s, 2)
	before := s.graph(t)
	path := filepath.Join(t.TempDir(), "input")
	require.NoError(t, syscall.Mkfifo(path, 0600))

	out, stderr, code := cli(t, s, fgaURL, path, "-apply", "-timeout", "200ms")

	require.Equal(t, 1, code)
	require.Empty(t, out)
	require.Contains(t, stderr, "context deadline exceeded")
	require.Equal(t, before, s.graph(t))
}
