package testaudit

import (
	"errors"
	"testing"

	"auth-service/pkg/audit"
	"auth-service/pkg/audit/internal"

	"github.com/stretchr/testify/require"
)

func TestNewAuditor(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	require.Empty(t, auditor)
}

func TestTestAudit_Create(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event, err := auditor.Create(t.Context())
	require.NoError(t, err)

	require.NotNil(t, event)
	require.Empty(t, event)
}

func TestTestAuditEvent_Append(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event, err := auditor.Create(t.Context())
	require.NoError(t, err)
	event.Append(internal.Field{
		FieldID: internal.FieldID(1),
		Value:   "test",
	})
	require.NotNil(t, event)
}

func TestTestAuditEvent_AppendContext(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event, err := auditor.Create(t.Context())
	require.NoError(t, err)
	event.AppendContext(audit.EventContext{
		"test": "test",
	})
	require.NotNil(t, event)
}

func TestTestAuditEvent_AppendError(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event, err := auditor.Create(t.Context())
	require.NoError(t, err)
	event.AppendError(errors.New("test"))
	require.NotNil(t, event)
}

func TestTestAuditEvent_WithError(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event, err := auditor.Create(t.Context())
	require.NoError(t, err)
	event.WithError(audit.ErrorCode("test"), audit.KindValidation, errors.New("test"))
	require.NotNil(t, event)
}

func TestTestAuditEvent_Error(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event, err := auditor.Create(t.Context())
	require.NoError(t, err)

	require.Empty(t, event.Error())
}

func TestEnd(t *testing.T) {
	t.Parallel()

	event := &testAuditEvent{}
	require.NoError(t, event.End(t.Context()))
}
