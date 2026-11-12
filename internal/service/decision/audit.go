package decision

import (
	"context"
	"errors"
	"time"

	"auth-service/internal/model"
	"auth-service/pkg/audit"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

const (
	// OperationCheckDecision идентифицирует проверку решения в журнале аудита.
	OperationCheckDecision = "check_decision"
	stageCheckRelation     = "check_relation"
	stageCheckTargetOwner  = "check_target_owner"
	stageCheckFields       = "check_fields"
	stageSaveDecision      = "save_decision"
	stageCompleted         = "completed"
)

// Поля события проверки решения.
const (
	fieldResource        = "resource"
	fieldAction          = "action"
	fieldRole            = "role"
	fieldTargetUserID    = "target_user_id"
	fieldTargetIsOwner   = "target_is_owner"
	fieldCheckedObject   = "checked_object"
	fieldCheckedRelation = "checked_relation"
	fieldDecision        = "decision"
	fieldDecisionID      = "decision_id"
	fieldPolicyVersion   = "policy_version"
	fieldTimestamp       = "timestamp"
)

const (
	codeResponseWriteFailed    = "response_write_failed"
	messageResponseWriteFailed = "Decision response could not be written"
	messageDecisionPanic       = "Panic during decision request"
)

// Один синхронный запрос владеет попыткой до окончания записи HTTP-ответа.
type attemptKey struct{}

type attempt struct {
	operationName string
	transportID   uuid.UUID
	event         audit.Event
	id            uuid.UUID
	panicID       uuid.UUID
	stage         string
	fields        audit.EventContext
	failure       error
	writeFailed   bool
	finished      bool
}

// currentAttempt возвращает открытую попытку check_decision для обработки запроса.
func currentAttempt(ctx context.Context) *attempt {
	a, _ := ctx.Value(attemptKey{}).(*attempt)
	if a == nil || a.finished || a.operationName != OperationCheckDecision {
		return nil
	}

	return a
}

// PanicCorrelation возвращает ID из запроса и ID завершённого события при панике.
// Пустой ID события означает, что запрос обрабатывается штатно.
func PanicCorrelation(ctx context.Context) (uuid.UUID, uuid.UUID) {
	a, _ := ctx.Value(attemptKey{}).(*attempt)
	if a == nil || a.operationName != OperationCheckDecision || a.panicID == uuid.Nil {
		return uuid.Nil, uuid.Nil
	}

	return a.id, a.panicID
}

// AuditRequest ведёт аудит запроса сервиса решений до окончания записи ответа.
// Вызывающий маршрут передаёт имя операции, которое попадёт в журнал.
func AuditRequest(ctx context.Context, source audit.Source, operation string, transportID uuid.UUID, handle func(context.Context) error) error {
	event, err := audit.OpenEvent(ctx, source)
	if err != nil {
		return err
	}

	a := &attempt{operationName: operation, transportID: transportID, event: event, stage: audit.StageReceiveRequest.String(), fields: audit.EventContext{}}
	defer a.finish(ctx)

	a.event.Append(audit.Operation(operation))
	a.event.Append(audit.KindField(audit.KindDomain))

	err = handle(context.WithValue(ctx, attemptKey{}, a))
	a.writeFailed = err != nil && a.stage != audit.StageReceiveRequest.String()

	return err
}

// RequireAudit проверяет, что middleware открыло событие именно для проверки решения.
func RequireAudit(ctx context.Context) error {
	if currentAttempt(ctx) == nil {
		return audit.ErrMissingAttempt
	}

	return nil
}

// AuditAuthenticationStarted отмечает начало проверки токена после общих HTTP-middleware.
func AuditAuthenticationStarted(ctx context.Context) {
	currentAttempt(ctx).at(audit.StageAuthenticate.String())
}

// AuditAuthenticated отмечает, что middleware подтвердил клиента и передаёт тело на разбор.
func AuditAuthenticated(ctx context.Context, clientID string) {
	if a := currentAttempt(ctx); a != nil {
		a.fields[audit.KeyCaller] = clientID
		a.stage = audit.StageDecodeRequest.String()
	}
}

// AuditRejection связывает ответ об ошибке с общей попыткой, включая отказы до Service.Check.
func AuditRejection(ctx context.Context, id uuid.UUID, err error) {
	if a := currentAttempt(ctx); a != nil {
		a.id = id
		a.failure = err
	}
}

func (a *attempt) at(stage string) {
	if a != nil {
		a.stage = stage
	}
}

func (a *attempt) operation(req model.CheckDecisionRequest) {
	if a == nil {
		return
	}

	a.fields[fieldResource] = string(req.Resource.Type) + ":" + req.Resource.ID.String()

	a.fields[fieldAction] = req.Action.Name
	if kind := req.Resource.Attributes.SpaceType; kind != nil {
		a.fields[model.AttributeSpaceType] = *kind
	}

	if req.Resource.Type == model.ResourceTypeMember {
		a.fields[fieldTargetUserID] = req.Resource.ID.String()
	}

	if role := req.Action.Attributes.Role; role != nil {
		a.fields[fieldRole] = *role
	}
}

func (a *attempt) verifiedSubject(subjectID uuid.UUID, object, relation string) {
	if a == nil {
		return
	}

	a.fields[audit.KeySubject] = string(model.SubjectTypeUser) + ":" + subjectID.String()

	a.fields[fieldCheckedObject], a.fields[fieldCheckedRelation] = nil, nil
	if object != "" {
		a.fields[fieldCheckedObject], a.fields[fieldCheckedRelation] = object, relation
	}

	a.stage = stageCheckRelation
}

// evaluated сохраняет вычисленный исход до записи в БД. Успех записи определяет конечный этап.
func (a *attempt) evaluated(resp model.CheckDecisionResponse) {
	if a == nil {
		return
	}

	a.fields[fieldDecision], a.fields[fieldDecisionID] = resp.Result, resp.DecisionID.String()
	a.fields[fieldPolicyVersion], a.fields[audit.KeyAuthorizationModelID] = resp.Meta.PolicyVersion, resp.Meta.AuthorizationModelID
}

// finish вызывается напрямую через defer и сохраняет исходную панику.
func (a *attempt) finish(ctx context.Context) {
	p := recover()

	a.finished = true
	if p != nil {
		a.fields[audit.KeyPanic] = true
	}

	correlationID := a.id
	if correlationID == uuid.Nil {
		correlationID = a.transportID
		if correlationID == uuid.Nil {
			correlationID = uuid.New()
		}
	}

	if p != nil {
		a.panicID = correlationID
	}

	event := a.event
	event.Append(audit.RequestID(correlationID.String()))

	a.fields[fieldTimestamp] = time.Now().UTC().Format(time.RFC3339Nano)
	if (a.writeFailed || p != nil) && a.stage == stageCompleted {
		a.stage = audit.StageWriteResponse.String()
	}

	a.fields[audit.KeyStage] = a.stage
	if a.writeFailed {
		a.fields[audit.KeyResponseWriteFailed] = true
	}

	a.outcome(event, p != nil)
	event.AppendContext(a.fields)

	if err := event.End(ctx); err != nil {
		logrus.WithField(audit.KeyRequestID, correlationID.String()).WithField(audit.KeyLocalWriteFailed, errors.Is(err, audit.ErrLocalWrite)).Warn("decision audit delivery failed")
	}

	if p != nil {
		panic(p)
	}
}

func (a *attempt) outcome(event audit.Event, panicked bool) {
	var code, message string

	switch {
	case a.failure != nil:
		code, message = DescribeError(a.failure)
		switch {
		case errors.Is(a.failure, context.Canceled):
			a.fields[audit.KeyContextError] = audit.ContextErrorCanceled.String()
		case errors.Is(a.failure, context.DeadlineExceeded):
			a.fields[audit.KeyContextError] = audit.ContextErrorDeadlineExceeded.String()
		}
	case panicked:
		code, message = CodeInternalError, messageDecisionPanic
	case a.stage == audit.StageReceiveRequest.String():
		code, message = string(audit.ErrCodeRequestInterrupted), audit.MessageRequestInterrupted
	case a.writeFailed:
		code, message = codeResponseWriteFailed, messageResponseWriteFailed
	default:
		event.Append(audit.StatusField(audit.StatusCompleted))
		event.Append(audit.Level(audit.ErrLevelInfo))

		return
	}

	event.Append(audit.StatusField(audit.StatusFailed))
	event.Append(audit.Level(audit.ErrLevelError))
	event.Append(audit.ErrorCodeField(audit.ErrorCode(code)))
	event.Append(audit.Message(message))
}
