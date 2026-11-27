package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"auth-service/internal/model"
	"auth-service/pkg/audit"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAuditRequest(t *testing.T) {
	t.Parallel()

	id := uuid.New()

	for _, tc := range []struct {
		name, operation string
		requestContext  func(*testing.T) context.Context
		source          func(*testing.T) (audit.Source, func() requestAuditEvent)
		handle          func(context.Context) error
		want            func(*testing.T, func() error, func() requestAuditEvent)
	}{
		{"interrupted before authentication without error", OperationCheckDecision, (*testing.T).Context, captureRequestAudit, func(context.Context) error { return nil }, wantInterruptedAudit(nil)},
		{"interrupted before authentication with error", OperationCheckDecision, (*testing.T).Context, captureRequestAudit, func(context.Context) error { return io.ErrClosedPipe }, wantInterruptedAudit(io.ErrClosedPipe)},
		{"canceled request and failed delivery", OperationCheckDecision, canceledRequestContext, captureFailedDelivery, completeAuditWithDecision(id, nil), wantDeliveredDecisionAudit(id, "completed", "completed", "", nil)},
		{"decision delivered", OperationCheckDecision, (*testing.T).Context, captureRequestAudit, completeAuditWithDecision(id, nil), wantDeliveredDecisionAudit(id, "completed", "completed", "", nil)},
		{"response write failed", OperationCheckDecision, (*testing.T).Context, captureRequestAudit, completeAuditWithDecision(id, io.ErrClosedPipe), wantDeliveredDecisionAudit(id, "failed", "write_response", "response_write_failed", io.ErrClosedPipe)},
		{"identity rejected", OperationCheckDecision, (*testing.T).Context, captureRequestAudit, func(ctx context.Context) error {
			AuditAuthenticated(ctx, "bot")
			AuditRejection(ctx, id, errors.Join(ErrIdentityUnavailable, context.DeadlineExceeded))

			return io.ErrClosedPipe
		}, func(t *testing.T, run func() error, published func() requestAuditEvent) {
			t.Helper()
			require.ErrorIs(t, run(), io.ErrClosedPipe)

			got := published()
			require.Equal(t, id, got.RequestID)
			require.Equal(t, "identity_store_unavailable", got.ErrorCode)
			require.Equal(t, "bot", got.Context["caller"])
			require.Equal(t, "decode_request", got.Context["stage"])
			require.Equal(t, true, got.Context["response_write_failed"])
			require.NotContains(t, got.Context, "decision_id")
		}},
		{"panic at entry", OperationCheckDecision, (*testing.T).Context, captureRequestAudit, func(context.Context) error { panic("private failure") }, wantPanicAtEntry},
		{"operation supplied by caller", "test.operation", (*testing.T).Context, captureRequestAudit, RequireAudit, func(t *testing.T, run func() error, published func() requestAuditEvent) {
			t.Helper()
			require.ErrorIs(t, run(), audit.ErrMissingAttempt)
			require.Equal(t, "test.operation", published().Operation)
		}},
		{"event unavailable", OperationCheckDecision, (*testing.T).Context, func(t *testing.T) (audit.Source, func() requestAuditEvent) { t.Helper(); return nil, nil }, func(context.Context) error { panic("handler invoked without audit") }, func(t *testing.T, run func() error, _ func() requestAuditEvent) {
			t.Helper()
			require.ErrorIs(t, run(), audit.ErrMissingAttempt)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, published := tc.source(t)
			tc.want(t, func() error { return AuditRequest(tc.requestContext(t), source, tc.operation, id, tc.handle) }, published)
		})
	}
}

func wantPanicAtEntry(t *testing.T, run func() error, published func() requestAuditEvent) {
	t.Helper()
	require.PanicsWithValue(t, "private failure", func() { _ = run() })

	got := published()
	require.NotEqual(t, uuid.Nil, got.RequestID)
	require.Equal(t, "failed", got.Status)
	require.Equal(t, "internal_error", got.ErrorCode)
	require.Equal(t, "Panic during decision request", got.Message)
	require.Equal(t, "receive_request", got.Context["stage"])
	require.Equal(t, true, got.Context["panic"])
	require.NotContains(t, got.Context, "caller")
	require.NotContains(t, got.Context, "decision_id")
}

func canceledRequestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	return ctx
}

func wantInterruptedAudit(handlerErr error) func(*testing.T, func() error, func() requestAuditEvent) {
	return func(t *testing.T, run func() error, published func() requestAuditEvent) {
		t.Helper()
		require.ErrorIs(t, run(), handlerErr)

		got := published()
		require.NotEqual(t, uuid.Nil, got.RequestID)
		require.Equal(t, "failed", got.Status)
		require.Equal(t, "request_interrupted", got.ErrorCode)
		require.Equal(t, "receive_request", got.Context["stage"])
		require.NotContains(t, got.Context, "decision_id")
	}
}

func captureFailedDelivery(t *testing.T) (audit.Source, func() requestAuditEvent) {
	t.Helper()

	var local bytes.Buffer

	publisher := &auditPublisher{err: io.ErrClosedPipe}
	sender, err := audit.NewSender(audit.WithClient(publisher), audit.WithTopic("test"))
	require.NoError(t, err)

	source := audit.NewAuditor(audit.WithOutput(&local), audit.WithSender(sender), audit.WithDeliveryTimeout(time.Second))

	return source, func() requestAuditEvent {
		require.Len(t, publisher.data, 1)
		require.True(t, publisher.bounded)
		require.NoError(t, publisher.contextErr)

		var got requestAuditEvent
		require.NoError(t, json.Unmarshal([]byte(publisher.data[0]), &got))
		require.Equal(t, 1, strings.Count(local.String(), "status=completed"))
		require.NotContains(t, local.String(), "status=failed")
		require.Contains(t, local.String(), "request_id="+got.RequestID.String())
		require.Contains(t, local.String(), "decision:deny")

		return got
	}
}

type requestAuditEvent struct {
	RequestID uuid.UUID      `json:"request_id"`
	Operation string         `json:"operation"`
	Status    string         `json:"status"`
	ErrorCode string         `json:"error_code"`
	Message   string         `json:"message"`
	Context   map[string]any `json:"context"`
}

// captureRequestAudit возвращает аудитор и чтение единственного опубликованного события.
func captureRequestAudit(t *testing.T) (audit.Source, func() requestAuditEvent) {
	t.Helper()

	publisher := &auditPublisher{}
	sender, err := audit.NewSender(audit.WithClient(publisher), audit.WithTopic("test"))
	require.NoError(t, err)

	source := audit.NewAuditor(audit.WithOutput(io.Discard), audit.WithSender(sender))

	return source, func() requestAuditEvent {
		t.Helper()

		require.Len(t, publisher.data, 1)

		var event requestAuditEvent
		require.NoError(t, json.Unmarshal([]byte(publisher.data[0]), &event))

		return event
	}
}

func TestCurrentAttempt(t *testing.T) {
	t.Parallel()

	active := &attempt{operationName: OperationCheckDecision}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want *attempt
	}{
		{"absent", t.Context(), nil},
		{"finished", context.WithValue(t.Context(), attemptKey{}, &attempt{operationName: OperationCheckDecision, finished: true}), nil},
		{"another operation", context.WithValue(t.Context(), attemptKey{}, &attempt{operationName: "other"}), nil},
		{"check decision", context.WithValue(t.Context(), attemptKey{}, active), active},
	} {
		t.Run(tc.name, func(t *testing.T) { t.Parallel(); require.Same(t, tc.want, currentAttempt(tc.ctx)) })
	}
}

func TestAuditAuthenticated(t *testing.T) {
	t.Parallel()

	a := &attempt{operationName: OperationCheckDecision, fields: audit.EventContext{}}
	ctx := context.WithValue(t.Context(), attemptKey{}, a)

	AuditAuthenticated(ctx, "admin")

	require.Equal(t, "admin", a.fields["caller"])
	require.Equal(t, "decode_request", a.stage)
}

func TestAuditRejection(t *testing.T) {
	t.Parallel()

	a := &attempt{operationName: OperationCheckDecision}
	ctx := context.WithValue(t.Context(), attemptKey{}, a)
	id := uuid.New()

	AuditRejection(ctx, id, ErrForbidden)

	require.Equal(t, id, a.id)
	require.ErrorIs(t, a.failure, ErrForbidden)
}

func TestAt(t *testing.T) {
	t.Parallel()

	a := &attempt{}

	a.at("validate_request")

	require.Equal(t, "validate_request", a.stage)
}

func TestOperation(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	for _, tc := range []struct {
		name     string
		resource model.DecisionResource
		action   model.DecisionAction
		want     audit.EventContext
	}{
		{"note", model.DecisionResource{Type: model.ResourceTypeNote, ID: id}, model.DecisionAction{Name: model.ActionNameRead}, audit.EventContext{fieldResource: "note:" + id.String(), fieldAction: model.ActionNameRead}},
		{"space type", model.DecisionResource{Type: model.ResourceTypeSpace, ID: id, Attributes: model.DecisionResourceAttributes{SpaceType: new(model.SpaceKindShared)}}, model.DecisionAction{Name: model.ActionNameCreate}, audit.EventContext{fieldResource: "space:" + id.String(), fieldAction: model.ActionNameCreate, model.AttributeSpaceType: model.SpaceKindShared}},
		{"membership role", model.DecisionResource{Type: model.ResourceTypeMember, ID: id}, model.DecisionAction{Name: model.ActionNameEdit, Attributes: model.DecisionActionAttributes{Role: new("admin")}}, audit.EventContext{fieldResource: "member:" + id.String(), fieldAction: model.ActionNameEdit, fieldTargetUserID: id.String(), fieldRole: "admin"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := &attempt{fields: audit.EventContext{}}
			a.operation(model.CheckDecisionRequest{Resource: tc.resource, Action: tc.action})

			require.Equal(t, tc.want, a.fields)
		})
	}
}

func TestVerifiedSubject(t *testing.T) {
	t.Parallel()

	id := uuid.New()

	for _, tc := range []struct {
		name, object, relation   string
		wantObject, wantRelation any
	}{
		{"relation checked", "space:target", "can_view", "space:target", "can_view"},
		{"model availability only", "", "", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := &attempt{fields: audit.EventContext{}}
			a.verifiedSubject(id, tc.object, tc.relation)

			require.Equal(t, "user:"+id.String(), a.fields[audit.KeySubject])
			require.Equal(t, tc.wantObject, a.fields[fieldCheckedObject])
			require.Equal(t, tc.wantRelation, a.fields[fieldCheckedRelation])
			require.Equal(t, stageCheckRelation, a.stage)
		})
	}
}

func TestEvaluated(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	a := &attempt{fields: audit.EventContext{}}

	a.evaluated(model.CheckDecisionResponse{
		DecisionID: id,
		Result:     "deny",
		Meta: model.DecisionMeta{
			PolicyVersion:        "policy",
			AuthorizationModelID: "model",
		},
	})

	require.Empty(t, a.stage)
	require.Equal(t, audit.EventContext{
		"decision_id":            id.String(),
		"decision":               "deny",
		"policy_version":         "policy",
		"authorization_model_id": "model",
	}, a.fields)
}

func TestFinish(t *testing.T) {
	t.Parallel()

	source, published := captureRequestAudit(t)
	ctx := t.Context()
	opened, openErr := source.Create(ctx)
	require.NoError(t, openErr)

	a := &attempt{event: opened, fields: audit.EventContext{}, stage: stageCompleted}
	a.event.Append(audit.Operation(OperationCheckDecision))
	a.event.Append(audit.KindField(audit.KindDomain))
	func() { defer a.finish(ctx) }()

	event := published()
	require.NotEqual(t, uuid.Nil, event.RequestID)
	require.Equal(t, "completed", event.Status)

	_, err := time.Parse(time.RFC3339Nano, event.Context["timestamp"].(string))
	require.NoError(t, err)
}

func TestOutcome(t *testing.T) {
	t.Parallel()

	noContextError := func(t *testing.T, fields map[string]any) {
		t.Helper()
		require.NotContains(t, fields, "context_error")
	}
	contextError := func(value string) func(*testing.T, map[string]any) {
		return func(t *testing.T, fields map[string]any) {
			t.Helper()
			require.Equal(t, value, fields["context_error"])
		}
	}

	_, forbiddenMessage := DescribeError(ErrForbidden)
	for _, tt := range []struct {
		name, initialStage    string
		failure               error
		writeFailed, panicked bool
		status, code, message string
		wantContext           func(*testing.T, map[string]any)
	}{
		{"interrupted at entry", audit.StageReceiveRequest.String(), nil, false, false, "failed", string(audit.ErrCodeRequestInterrupted), audit.MessageRequestInterrupted, noContextError},
		{"domain error before panic", stageCheckRelation, ErrForbidden, false, true, "failed", CodeForbidden, forbiddenMessage, noContextError},
		{"completed", stageCompleted, nil, false, false, "completed", "", "", noContextError},
		{"write failed", stageCompleted, nil, true, false, "failed", "response_write_failed", "Decision response could not be written", noContextError},
		{"panic before write failure", stageCompleted, nil, true, true, "failed", "internal_error", "Panic during decision request", noContextError},
		{"canceled", stageCompleted, errors.Join(ErrIdentityUnavailable, context.Canceled), false, false, "failed", "identity_store_unavailable", "Identity store is unavailable", contextError("canceled")},
		{"deadline before write failure", stageCompleted, errors.Join(ErrIdentityUnavailable, context.DeadlineExceeded), true, false, "failed", "identity_store_unavailable", "Identity store is unavailable", contextError("deadline_exceeded")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source, published := captureRequestAudit(t)
			ctx := t.Context()
			event, openErr := source.Create(ctx)
			require.NoError(t, openErr)
			event.Append(audit.Operation(OperationCheckDecision))
			event.Append(audit.KindField(audit.KindDomain))

			a := attempt{stage: tt.initialStage, failure: tt.failure, writeFailed: tt.writeFailed, fields: audit.EventContext{}}

			a.outcome(event, tt.panicked)
			event.AppendContext(a.fields)
			require.NoError(t, event.End(ctx))

			got := published()
			require.Equal(t, tt.status, got.Status)
			require.Equal(t, tt.code, got.ErrorCode)
			require.Equal(t, tt.message, got.Message)

			tt.wantContext(t, got.Context)
		})
	}
}

func TestPanicCorrelation(t *testing.T) {
	t.Parallel()

	id, panicID := uuid.New(), uuid.New()

	for _, tc := range []struct {
		name                         string
		ctx                          context.Context
		wantRequest, wantCorrelation uuid.UUID
	}{
		{"outside decision", t.Context(), uuid.Nil, uuid.Nil},
		{"panicked", context.WithValue(t.Context(), attemptKey{}, &attempt{operationName: OperationCheckDecision, finished: true, id: id, panicID: panicID}), id, panicID},
		{"panic in another operation", context.WithValue(t.Context(), attemptKey{}, &attempt{operationName: "other", panicID: panicID}), uuid.Nil, uuid.Nil},
		{"ordinary request", context.WithValue(t.Context(), attemptKey{}, &attempt{operationName: OperationCheckDecision, id: uuid.New()}), uuid.Nil, uuid.Nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			id, correlation := PanicCorrelation(tc.ctx)

			require.Equal(t, tc.wantRequest, id)
			require.Equal(t, tc.wantCorrelation, correlation)
		})
	}
}

func TestAuditAuthenticationStarted(t *testing.T) {
	t.Parallel()

	AuditAuthenticationStarted(t.Context())

	a := &attempt{operationName: OperationCheckDecision, stage: audit.StageReceiveRequest.String()}

	AuditAuthenticationStarted(context.WithValue(t.Context(), attemptKey{}, a))

	require.Equal(t, audit.StageAuthenticate.String(), a.stage)
}

func TestRequireAudit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"absent", t.Context(), audit.ErrMissingAttempt},
		{"another operation", context.WithValue(t.Context(), attemptKey{}, &attempt{operationName: "other"}), audit.ErrMissingAttempt},
		{"finished", context.WithValue(t.Context(), attemptKey{}, &attempt{operationName: OperationCheckDecision, finished: true}), audit.ErrMissingAttempt},
		{"present", context.WithValue(t.Context(), attemptKey{}, &attempt{operationName: OperationCheckDecision}), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, RequireAudit(tc.ctx), tc.want)
		})
	}
}

func completeAuditWithDecision(id uuid.UUID, writeErr error) func(context.Context) error {
	return func(ctx context.Context) error {
		a := currentAttempt(ctx)
		a.evaluated(model.CheckDecisionResponse{DecisionID: id, Result: "deny"})
		a.at(stageCompleted)

		return writeErr
	}
}

func wantDeliveredDecisionAudit(id uuid.UUID, status, stage, code string, writeErr error) func(*testing.T, func() error, func() requestAuditEvent) {
	return func(t *testing.T, run func() error, published func() requestAuditEvent) {
		t.Helper()
		require.ErrorIs(t, run(), writeErr)

		got := published()
		require.Equal(t, "check_decision", got.Operation)
		require.Equal(t, status, got.Status)
		require.Equal(t, code, got.ErrorCode)
		require.Equal(t, stage, got.Context["stage"])
		require.Equal(t, id.String(), got.Context["decision_id"])
		require.Equal(t, "deny", got.Context["decision"])
	}
}
