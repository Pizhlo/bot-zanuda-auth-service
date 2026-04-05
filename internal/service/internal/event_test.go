package internal

import (
	"auth-service/pkg/audit"
	"context"
	"testing"

	"auth-service/pkg/audit/testaudit"

	"github.com/stretchr/testify/require"
)

type panicRecoveryEventStub struct {
	audit.Event
	endCalls int
}

func (s *panicRecoveryEventStub) End(ctx context.Context) {
	s.endCalls++
}

func TestWithMessageCtx(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ctx = WithMessageCtx(ctx, audit.EventContext{"message": "test"})

	val, ok := ctx.Value(MessageContextKey{}).(audit.EventContext)
	require.True(t, ok)
	require.Equal(t, audit.EventContext{"message": "test"}, val)
}

func TestWithUserID(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ctx = WithUserID(ctx, "123")

	val, ok := ctx.Value(UserIDKey{}).(string)
	require.True(t, ok)
	require.Equal(t, "123", val)
}

func TestWithServiceName(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ctx = WithServiceName(ctx)

	val, ok := ctx.Value(ServiceNameKey{}).(string)
	require.True(t, ok)
	require.Equal(t, "auth-service", val)
}

func TestWithMessage(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ctx = WithMessage(ctx, "test")

	val, ok := ctx.Value(MessageKey{}).(string)
	require.True(t, ok)
	require.Equal(t, "test", val)
}

func TestWithLevel(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ctx = WithLevel(ctx, audit.ErrLevelError)

	val, ok := ctx.Value(LevelKey{}).(audit.ErrorLevel)
	require.True(t, ok)
	require.Equal(t, audit.ErrLevelError, val)
}

func TestWithErrorCode(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ctx = WithErrorCode(ctx, audit.ErrCodeServiceNotFound)

	val, ok := ctx.Value(ErrorCodeKey{}).(audit.ErrorCode)
	require.True(t, ok)
	require.Equal(t, audit.ErrCodeServiceNotFound, val)
}

func TestWithKind(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ctx = WithKind(ctx, audit.KindValidation)

	val, ok := ctx.Value(KindKey{}).(audit.Kind)
	require.True(t, ok)
	require.Equal(t, audit.KindValidation, val)
}

func TestWithOperation(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ctx = WithOperation(ctx, "test")

	val, ok := ctx.Value(OperationKey{}).(string)
	require.True(t, ok)
	require.Equal(t, "test", val)
}

func TestWithPanicRecovery_NoPanic(t *testing.T) {
	t.Parallel()

	baseEvent := testaudit.NewAuditor(t).Create(t.Context())
	event := &panicRecoveryEventStub{Event: baseEvent}

	require.NotPanics(t, func() {
		defer WithPanicRecovery(t.Context(), event)()
	})

	require.Equal(t, 1, event.endCalls)
}

func TestWithPanicRecovery_Panic(t *testing.T) {
	t.Parallel()

	baseEvent := testaudit.NewAuditor(t).Create(t.Context())
	event := &panicRecoveryEventStub{Event: baseEvent}

	require.Panics(t, func() {
		defer WithPanicRecovery(t.Context(), event)()

		panic("boom")
	})

	require.Equal(t, 1, event.endCalls)
}
