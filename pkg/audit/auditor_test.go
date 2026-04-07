package audit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewAuditor(t *testing.T) {
	t.Parallel()

	hook := func(ctx context.Context, stash Stash) Stash {
		return stash
	}

	auditor := NewAuditor(WithHook(hook))

	require.NotNil(t, auditor)
	require.NotNil(t, auditor.logger)
	require.Len(t, auditor.hooks, 1)
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
