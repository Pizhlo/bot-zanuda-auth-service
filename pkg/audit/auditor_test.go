package audit

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

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

func TestCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		output    io.Writer
		wantErr   require.ErrorAssertionFunc
		wantEvent func(*testing.T, Event)
	}{
		{
			name: "started event", output: io.Discard, wantErr: require.NoError,
			wantEvent: func(t *testing.T, e Event) {
				t.Helper()
				require.NotNil(t, e)
				stash := e.(*event).stash
				require.NotNil(t, stash)
				require.Len(t, stash.fields, 2)
				require.Equal(t, StatusStarted, stash.fields[fieldStatus].Value)
			},
		},
		{
			name: "local output fails", output: brokenOutput{},
			wantErr: func(t require.TestingT, err error, _ ...any) {
				require.ErrorIs(t, err, ErrLocalWrite)
				require.ErrorIs(t, err, io.ErrClosedPipe)
			},
			wantEvent: func(t *testing.T, e Event) { t.Helper(); require.Nil(t, e) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hookCalled := false
			auditor := NewAuditor(WithOutput(tt.output), WithHook(func(_ context.Context, stash Stash) Stash {
				hookCalled = true
				return stash
			}))
			require.NotNil(t, auditor)
			require.NotNil(t, auditor.logger)
			require.Len(t, auditor.hooks, 1)

			e, err := auditor.Create(t.Context())
			tt.wantErr(t, err)

			tt.wantEvent(t, e)
			require.True(t, hookCalled)
		})
	}
}

func TestWithDeliveryTimeout(t *testing.T) {
	t.Parallel()

	var a Auditor

	WithDeliveryTimeout(3 * time.Second)(&a)

	require.Equal(t, 3*time.Second, a.deliveryTimeout)
}

func TestWithOutput(t *testing.T) {
	t.Parallel()

	var (
		a      Auditor
		output bytes.Buffer
	)

	WithOutput(&output)(&a)
	a.logger.Log(t.Context(), LevelAudit, "captured")
	require.Contains(t, output.String(), "captured")
}
