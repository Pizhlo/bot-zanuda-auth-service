// Package decision вычисляет одно решение без изменения ресурсов и отношений.
package decision

import (
	"context"
	"errors"
	"fmt"
	"time"

	"auth-service/internal/model"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"

	"github.com/google/uuid"
)

var (
	// ErrUnauthorized обозначает отсутствие доверенного клиента.
	ErrUnauthorized = errors.New(CodeUnauthorized)
	// ErrForbidden обозначает недопустимого клиента или scope.
	ErrForbidden = errors.New(CodeForbidden)
	// ErrSubjectMismatch обозначает несовпадение доверенной личности и тела.
	ErrSubjectMismatch = errors.New(CodeSubjectMismatch)
	// ErrUserNotFound обозначает незарегистрированного пользователя.
	ErrUserNotFound = errors.New(CodeUserNotFound)
	// ErrIdentityUnavailable обозначает недоступность хранилища личности.
	ErrIdentityUnavailable = errors.New(CodeIdentityUnavailable)
	// ErrOpenFGAUnavailable обозначает ошибку проверки отношений.
	ErrOpenFGAUnavailable = errors.New(CodeOpenFGAUnavailable)
	// ErrDecisionStoreUnavailable означает, что сервис не подтвердил запись решения.
	ErrDecisionStoreUnavailable = errors.New(CodeDecisionStoreUnavailable)
	// ErrPolicyUnavailable обозначает ошибку вычисления политики полей.
	ErrPolicyUnavailable = errors.New(CodePolicyUnavailable)
)

// Caller передаёт личность из доверенной HTTP-границы: проверенный сервисный ClientID
// и TelegramID из заголовка. Значения из тела или JWT пользователя сюда не копируются.
type Caller struct{ ClientID, TelegramID string }

//go:generate mockgen -source=decision.go -destination=mocks/decision.go -package=mocks

type decisionStore interface {
	SaveDecision(context.Context, model.DecisionRecord) error
}

type identityStore interface {
	GetUserIDByTelegramID(context.Context, string) (uuid.UUID, error)
}
type relationChecker interface {
	CheckModelAvailability(context.Context) error
	CheckRelation(context.Context, string, string, string, model.Consistency) (bool, error)
	ModelID() string
}
type fieldPolicy interface {
	CheckFields(string, string, []string) ([]string, error)
	Version() string
}

// Service последовательно сверяет пользователя, проверяет отношения OpenFGA
// и, только после разрешения на update, фиксированную политику полей Casbin.
// Он не регистрирует пользователей, не меняет tuples и не кеширует решения.
type Service struct {
	decisions decisionStore
	users     identityStore
	relations relationChecker
	fields    fieldPolicy
}

// serviceOption задаёт одну зависимость или параметр сервиса перед валидацией New.
type serviceOption func(*Service)

// WithDecisionStore подключает обязательное хранилище завершённых решений.
func WithDecisionStore(store decisionStore) serviceOption {
	return func(s *Service) { s.decisions = store }
}

// WithIdentityStore подключает поиск внутреннего UUID по доверенному Telegram ID.
func WithIdentityStore(users identityStore) serviceOption {
	return func(s *Service) { s.users = users }
}

// WithRelationChecker подключает проверки на уже выбранной версии модели OpenFGA.
func WithRelationChecker(relations relationChecker) serviceOption {
	return func(s *Service) { s.relations = relations }
}

// WithFieldPolicy подключает загруженную неизменяемую политику полей.
func WithFieldPolicy(fields fieldPolicy) serviceOption {
	return func(s *Service) { s.fields = fields }
}

// New применяет опции и проверяет зависимости, версии правил.
// Модель OpenFGA и политика полей должны быть готовы до запуска HTTP.
func New(opts ...serviceOption) (*Service, error) {
	s := &Service{}
	for _, opt := range opts {
		opt(s)
	}

	if s.users == nil {
		return nil, errors.New("identity store is required")
	}

	if s.relations == nil {
		return nil, errors.New("relation checker is required")
	}

	if s.fields == nil {
		return nil, errors.New("field policy is required")
	}

	if s.decisions == nil {
		return nil, errors.New("decision store is required")
	}

	if s.relations.ModelID() == "" || s.fields.Version() == "" {
		return nil, errors.New("authorization model and field policy must be loaded")
	}

	return s, nil
}

// Check вычисляет одно решение после валидации запроса и доверенной личности.
// Ошибка зависимости возвращается отдельно от deny; дальнейшие этапы при отказе не запускаются.
// Завершённый результат получает новый DecisionID, даже при повторном RequestID.
// Сбой доставки аудита не меняет уже вычисленное решение.
func (s *Service) Check(ctx context.Context, caller Caller, req model.CheckDecisionRequest) (model.CheckDecisionResponse, error) {
	a := currentAttempt(ctx)
	if a == nil {
		return model.CheckDecisionResponse{}, audit.ErrMissingAttempt
	}

	a.id = req.RequestID
	if caller.ClientID != "" {
		a.fields[audit.KeyCaller] = caller.ClientID
	}

	resp, err := s.evaluateAndSave(ctx, caller, req, a)
	a.failure = err

	return resp, err
}

// evaluateAndSave выполняет одну попытку от валидации до записи результата. Сначала определяет
// объект и отношение по типу операции, затем сверяет subject с доверенной личностью.
// Только после этого обращается к правилам доступа. Ошибка любого этапа остаётся
// ошибкой запроса, а вычисленные allow и deny возвращаются после подтверждения БД.
func (s *Service) evaluateAndSave(ctx context.Context, caller Caller, req model.CheckDecisionRequest, a *attempt) (model.CheckDecisionResponse, error) {
	started := time.Now()

	a.at(audit.StageValidateRequest.String())

	object, relation, err := validateRequest(req)
	if err != nil {
		return model.CheckDecisionResponse{}, fmt.Errorf("validate decision request: %w", err)
	}

	a.operation(req)
	a.at(audit.StageVerifySubject.String())

	if err := s.verifySubject(ctx, caller, req.Subject); err != nil {
		return model.CheckDecisionResponse{}, fmt.Errorf("verify decision subject: %w", err)
	}

	a.verifiedSubject(req.Subject.ID, object, relation)

	allowed, reasons, err := s.evaluate(ctx, req, object, relation, a)
	if err != nil {
		return model.CheckDecisionResponse{}, fmt.Errorf("evaluate decision: %w", err)
	}

	resp := s.decisionResponse(req, allowed, reasons, started)
	a.evaluated(resp)

	a.at(stageSaveDecision)

	if err := s.saveDecision(ctx, req, caller, object, relation, resp); err != nil {
		return model.CheckDecisionResponse{}, fmt.Errorf("persist evaluated decision: %w", err)
	}

	a.at(stageCompleted)

	if !req.Options.IncludeReasons {
		resp.Reasons = []model.DecisionReason{}
	}

	return resp, nil
}

func (s *Service) decisionResponse(req model.CheckDecisionRequest, allowed bool, reasons []model.DecisionReason, started time.Time) model.CheckDecisionResponse {
	result := model.DecisionResultDeny
	if allowed {
		result = model.DecisionResultAllow
	}

	return model.CheckDecisionResponse{
		RequestID: req.RequestID, DecisionID: uuid.New(), Result: result, Allow: allowed, Reasons: reasons,
		Meta: model.DecisionMeta{
			Subject: "user:" + req.Subject.ID.String(), Action: req.Action.Name,
			Resource:      fmt.Sprintf("%s:%s", req.Resource.Type, req.Resource.ID),
			PolicyVersion: s.fields.Version(), AuthorizationModelID: s.relations.ModelID(),
			LatencyMS: time.Since(started).Milliseconds(),
		},
	}
}

// saveDecision сохраняет полный набор причин независимо от include_reasons в HTTP-запросе.
func (s *Service) saveDecision(ctx context.Context, req model.CheckDecisionRequest, caller Caller, object, relation string, resp model.CheckDecisionResponse) error {
	record := model.DecisionRecord{
		DecisionID: resp.DecisionID, RequestID: req.RequestID, ClientID: caller.ClientID, SubjectID: req.Subject.ID,
		Action: req.Action, Resource: req.Resource,
		Result: resp.Result, Reasons: resp.Reasons, PolicyVersion: resp.Meta.PolicyVersion, AuthorizationModelID: resp.Meta.AuthorizationModelID,
	}
	if object != "" {
		record.CheckedObject = &object
		record.CheckedRelation = &relation
	}

	if err := s.decisions.SaveDecision(ctx, record); err != nil {
		return fmt.Errorf("save completed decision: %w: %w", ErrDecisionStoreUnavailable, err)
	}

	return nil
}

func (s *Service) verifySubject(ctx context.Context, caller Caller, subject model.DecisionSubject) error {
	if caller.ClientID == "" {
		return ErrUnauthorized
	}

	if caller.TelegramID == "" {
		return model.InvalidDecisionRequest("X-Telegram-User-Id header is required")
	}

	if !model.ValidTelegramID(caller.TelegramID) {
		return model.InvalidDecisionRequest("X-Telegram-User-Id header must be a positive decimal int64 without spaces, signs or leading zeros")
	}

	if subject.Attributes.TelegramUserID != nil && *subject.Attributes.TelegramUserID != caller.TelegramID {
		return model.InvalidDecisionRequest("subject.attributes.telegram_user_id must match the X-Telegram-User-Id header")
	}

	id, err := s.users.GetUserIDByTelegramID(ctx, caller.TelegramID)
	if errors.Is(err, storage.ErrNotFound) {
		return ErrUserNotFound
	}

	if err != nil {
		return fmt.Errorf("%w: %w", ErrIdentityUnavailable, err)
	}

	if id == uuid.Nil {
		return ErrUserNotFound
	}

	if id != subject.ID {
		return ErrSubjectMismatch
	}

	return nil
}

// evaluate применяет правила к подтверждённому пользователю. Создание shared разрешено зарегистрированному
// пользователю. Для personal проверяется наличие личного пространства у владельца. Для остальных
// операций отсутствие отношения сразу даёт deny. После разрешения FGA защищаем
// владельца при изменении участников или проверяем всю маску полей при update.
// Ошибка зависимости прерывает вычисление и сохраняет этап для аудита.
func (s *Service) evaluate(ctx context.Context, req model.CheckDecisionRequest, object, relation string, a *attempt) (bool, []model.DecisionReason, error) {
	if req.Resource.Type == model.ResourceTypeSpace && req.Action.Name == model.ActionNameCreate {
		if *req.Resource.Attributes.SpaceType == model.SpaceKindShared {
			return s.checkSharedSpace(ctx, req)
		}

		return s.checkPersonalSpace(ctx, object, relation)
	}

	allowed, err := s.relations.CheckRelation(ctx, "user:"+req.Subject.ID.String(), relation, object, req.ConsistencyPreference())
	if err != nil {
		return false, nil, fmt.Errorf("%w: %w", ErrOpenFGAUnavailable, err)
	}

	if !allowed {
		return false, reason(model.ReasonDeniedByFGARelation, fmt.Sprintf(messageRelationAbsent, relation, object, req.Subject.ID)), nil
	}

	if req.Resource.Type == model.ResourceTypeMember {
		a.at(stageCheckTargetOwner)
		return s.checkMembershipChangeTarget(ctx, req, object, relation, a)
	}

	reasons := reason(model.ReasonAllowedViaFGARelation, fmt.Sprintf(messageRelationPresent, relation, object, req.Subject.ID))
	if req.Action.Name == model.ActionNameUpdate {
		a.at(stageCheckFields)
		return s.checkFields(req, reasons)
	}

	return true, reasons, nil
}

// checkSharedSpace подтверждает доступность закреплённой модели перед разрешением
// создать общее пространство уже зарегистрированному пользователю.
func (s *Service) checkSharedSpace(ctx context.Context, req model.CheckDecisionRequest) (bool, []model.DecisionReason, error) {
	if err := s.relations.CheckModelAvailability(ctx); err != nil {
		return false, nil, fmt.Errorf("check pinned model availability: %w: %w", ErrOpenFGAUnavailable, err)
	}

	return true, reason(model.ReasonAllowedForRegisteredUser, fmt.Sprintf(messageRegisteredUser, req.Subject.ID, req.Resource.ID)), nil
}

// checkPersonalSpace проверяет отметку подтверждённого владельца с повышенной согласованностью.
// Отсутствие отметки разрешает создание. Само место занимает запись tuples при синхронизации.
func (s *Service) checkPersonalSpace(ctx context.Context, owner, relation string) (bool, []model.DecisionReason, error) {
	exists, err := s.relations.CheckRelation(ctx, owner, relation, owner, model.HigherConsistency)
	if err != nil {
		return false, nil, fmt.Errorf("check personal space: %w: %w", ErrOpenFGAUnavailable, err)
	}

	if exists {
		return false, reason(model.ReasonPersonalSpaceAlreadyExists, fmt.Sprintf(messagePersonalSpaceAlreadyExists, relation, owner)), nil
	}

	return true, reason(model.ReasonFirstPersonalSpace, fmt.Sprintf(messageFirstPersonalSpace, relation, owner)), nil
}

// checkFields дополняет успешную проверку объекта; одно запрещённое имя отменяет весь update.
func (s *Service) checkFields(req model.CheckDecisionRequest, reasons []model.DecisionReason) (bool, []model.DecisionReason, error) {
	forbidden, err := s.fields.CheckFields(string(req.Resource.Type), string(req.Action.Name), req.Action.Attributes.FieldMask)
	if err != nil {
		return false, nil, fmt.Errorf("check editable fields: %w: %w", ErrPolicyUnavailable, err)
	}

	if len(forbidden) > 0 {
		return false, reason(model.ReasonForbiddenFields, fmt.Sprintf(messageForbiddenFields, req.Resource.Type, req.Action.Name, reasonFields(forbidden))), nil
	}

	return true, append(reasons, model.DecisionReason{Code: model.ReasonAllowedFields, Message: fmt.Sprintf(messageAllowedFields, req.Resource.Type, req.Action.Name, reasonFields(req.Action.Attributes.FieldMask))}), nil
}

// checkMembershipChangeTarget вызывается после проверки can_manage_members у инициатора.
// Проверяет, является ли целевой пользователь владельцем пространства. Если да, изменение
// его участия запрещено. Для остальных целей сохраняется уже проверенное право инициатора.
func (s *Service) checkMembershipChangeTarget(ctx context.Context, req model.CheckDecisionRequest, object, relation string, a *attempt) (bool, []model.DecisionReason, error) {
	isOwner, err := s.relations.CheckRelation(ctx, "user:"+req.Resource.ID.String(), string(model.RelationTypeOwner), object, req.ConsistencyPreference())
	if err != nil {
		return false, nil, fmt.Errorf("check protected membership: %w: %w", ErrOpenFGAUnavailable, err)
	}

	a.fields[fieldTargetIsOwner] = isOwner

	if isOwner {
		return false, reason(model.ReasonProtectedSpaceOwner, fmt.Sprintf(messageProtectedOwner, req.Resource.ID, object)), nil
	}

	return true, reason(model.ReasonAllowedViaFGARelation, fmt.Sprintf(messageRelationPresent, relation, object, req.Subject.ID)), nil
}
