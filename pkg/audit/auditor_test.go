package audit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест - это ок
func TestNewAuditor(t *testing.T) {
	t.Parallel()

	hook := func(ctx context.Context, stash Stash) Stash {
		return stash
	}

	tests := []struct {
		name      string
		opts      []auditorOption
		checkWant func(t *testing.T, auditor *Auditor)
	}{
		{
			name: "only hooks are provided",
			opts: []auditorOption{
				WithHook(hook),
			},
			checkWant: func(t *testing.T, auditor *Auditor) {
				t.Helper()

				require.NotNil(t, auditor)
				require.NotNil(t, auditor.logger)
				require.Len(t, auditor.hooks, 1)
			},
		},
		{
			name: "only sender is provided",
			opts: []auditorOption{
				WithSender(&mockSender{}),
			},
			checkWant: func(t *testing.T, auditor *Auditor) {
				t.Helper()

				require.NotNil(t, auditor)
				require.NotNil(t, auditor.logger)
				require.Len(t, auditor.hooks, 0)
				require.NotNil(t, auditor.sender)
			},
		},
		{
			name: "both hooks and sender are provided",
			opts: []auditorOption{
				WithHook(hook),
				WithSender(&mockSender{}),
			},
			checkWant: func(t *testing.T, auditor *Auditor) {
				t.Helper()

				require.NotNil(t, auditor)
				require.NotNil(t, auditor.logger)
				require.Len(t, auditor.hooks, 1)
				require.NotNil(t, auditor.sender)
			},
		},
		{
			name: "with include levels",
			opts: []auditorOption{
				WithIncludeLevels([]string{"error"}),
			},
			checkWant: func(t *testing.T, auditor *Auditor) {
				t.Helper()

				require.NotNil(t, auditor)
				require.NotNil(t, auditor.logger)
				require.Equal(t, []string{"error"}, auditor.levels.include)
			},
		},
		{
			name: "with exclude levels",
			opts: []auditorOption{
				WithExcludeLevels([]string{"error"}),
			},
			checkWant: func(t *testing.T, auditor *Auditor) {
				t.Helper()

				require.NotNil(t, auditor)
				require.NotNil(t, auditor.logger)
				require.Equal(t, []string{"error"}, auditor.levels.exclude)
			},
		},
		{
			name: "with include kinds",
			opts: []auditorOption{
				WithIncludeKinds([]string{"error"}),
			},
			checkWant: func(t *testing.T, auditor *Auditor) {
				t.Helper()

				require.NotNil(t, auditor)
				require.NotNil(t, auditor.logger)
				require.Equal(t, []string{"error"}, auditor.kinds.include)
			},
		},
		{
			name: "with exclude kinds",
			opts: []auditorOption{
				WithExcludeKinds([]string{"error"}),
			},
			checkWant: func(t *testing.T, auditor *Auditor) {
				t.Helper()

				require.NotNil(t, auditor)
				require.NotNil(t, auditor.logger)
				require.Equal(t, []string{"error"}, auditor.kinds.exclude)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			auditor := NewAuditor(test.opts...)
			test.checkWant(t, auditor)
		})
	}
}

func TestAuditor_Create(t *testing.T) {
	t.Parallel()

	hookCalled := false
	hook := func(ctx context.Context, stash Stash) Stash {
		hookCalled = true
		return stash
	}

	auditor := NewAuditor(WithHook(hook))

	require.NotNil(t, auditor)
	require.NotNil(t, auditor.logger)
	require.Len(t, auditor.hooks, 1)

	e := auditor.Create(context.Background())

	require.NotNil(t, e)
	require.NotNil(t, e.(*event).stash)
	require.Len(t, e.(*event).stash.fields, 2)
	require.Equal(t, StatusStarted, e.(*event).stash.fields[fieldStatus].Value)
	require.True(t, hookCalled)
}
