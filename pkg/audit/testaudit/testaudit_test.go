package testaudit

import (
	"auth-service/pkg/audit"
	"auth-service/pkg/audit/internal"
	"errors"
	"testing"

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
	event := auditor.Create(t.Context())
	require.NotNil(t, event)
	require.Empty(t, event)
}

func TestTestAuditEvent_Append(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event := auditor.Create(t.Context())
	event.Append(internal.Field{
		FieldID: internal.FieldID(1),
		Value:   "test",
	})
	require.NotNil(t, event)
}

func TestTestAuditEvent_AppendContext(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event := auditor.Create(t.Context())
	event.AppendContext(audit.EventContext{
		"test": "test",
	})
	require.NotNil(t, event)
}

func TestTestAuditEvent_AppendError(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event := auditor.Create(t.Context())
	event.AppendError(errors.New("test"))
	require.NotNil(t, event)
}

func TestTestAuditEvent_WithError(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event := auditor.Create(t.Context())
	event.WithError(audit.ErrorCode("test"), audit.KindValidation, errors.New("test"))
	require.NotNil(t, event)
}

func TestTestAuditEvent_Error(t *testing.T) {
	t.Parallel()

	auditor := NewAuditor(t)
	event := auditor.Create(t.Context())
	require.Empty(t, event.Error())
}
