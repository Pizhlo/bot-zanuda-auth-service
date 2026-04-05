package testaudit

import (
	"auth-service/pkg/audit"
	"auth-service/pkg/audit/internal"
	"context"
	"testing"
)

// testAudit - заглушка аудита для использования в тестах.
type testAudit struct{}

// NewAuditor создает новый тестовый auditor.
func NewAuditor(t *testing.T) *testAudit {
	t.Helper()

	return &testAudit{}
}

type testAuditEvent struct {
}

func (ta *testAudit) Create(ctx context.Context) audit.Event {
	return &testAuditEvent{}
}

func (ta *testAuditEvent) Append(field internal.Field) {
}

func (ta *testAuditEvent) AppendContext(context audit.EventContext) {
}

func (ta *testAuditEvent) AppendError(err error) {
}

func (ta *testAuditEvent) WithError(code audit.ErrorCode, kind audit.Kind, err error) {
}

func (ta *testAuditEvent) Error() string {
	return ""
}
