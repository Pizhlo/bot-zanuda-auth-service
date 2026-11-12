// Package decision вычисляет одно решение без изменения ресурсов и отношений.
package decision

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"auth-service/internal/model"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"
	"github.com/google/uuid"
)

var (
	// ErrUnauthorized обозначает отсутствие доверенного клиента.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden обозначает недопустимого клиента или scope.
	ErrForbidden = errors.New("forbidden")
	// ErrSubjectMismatch обозначает несовпадение доверенной личности и тела.
	ErrSubjectMismatch = errors.New("subject_mismatch")
	// ErrUserNotFound обозначает незарегистрированного пользователя.
	ErrUserNotFound = errors.New("user_not_found")
	// ErrIdentityUnavailable обозначает недоступность хранилища личности.
	ErrIdentityUnavailable = errors.New("identity_store_unavailable")
	// ErrOpenFGAUnavailable обозначает ошибку проверки отношений.
	ErrOpenFGAUnavailable = errors.New("openfga_unavailable")
	// ErrPolicyUnavailable обозначает ошибку вычисления политики полей.
	ErrPolicyUnavailable = errors.New("policy_unavailable")
)

// Caller содержит идентификатор проверенного сервисного клиента и доверенный Telegram ID.
type Caller struct{ ClientID, TelegramID string }

//go:generate mockgen -source=decision.go -destination=mocks/decision.go -package=mocks

type identityStore interface {
	GetUserIDByTelegramID(context.Context, string) (uuid.UUID, error)
}
type relationChecker interface {
	CheckRelation(context.Context, string, string, string, model.Consistency) (bool, error)
	ModelID() string
}
type fieldPolicy interface {
	CheckFields(string, string, []string) ([]string, error)
	Version() string
}
type auditor interface {
	Create(context.Context) audit.Event
}

// Service объединяет доверенную личность, отношения OpenFGA и фиксированную политику полей.
type Service struct {
	users     identityStore
	relations relationChecker
	fields    fieldPolicy
	audit     auditor
}

// New создаёт сервис с уже загруженной политикой и закреплённой моделью OpenFGA.
func New(users identityStore, relations relationChecker, fields fieldPolicy, events auditor) (*Service, error) {
	if users == nil || relations == nil || fields == nil || events == nil {
		return nil, errors.New("decision dependencies are required")
	}

	if relations.ModelID() == "" || fields.Version() == "" {
		return nil, errors.New("authorization model and field policy must be loaded")
	}

	return &Service{users: users, relations: relations, fields: fields, audit: events}, nil
}

// Check проверяет одно действие; повтор request_id всегда вычисляет новое решение.
func (s *Service) Check(ctx context.Context, caller Caller, req model.CheckDecisionRequest) (model.CheckDecisionResponse, error) {
	started := time.Now()

	if err := req.Validate(); err != nil {
		return model.CheckDecisionResponse{}, err
	}

	if err := s.verifySubject(ctx, caller, req.Subject); err != nil {
		return model.CheckDecisionResponse{}, err
	}

	object, relation, err := req.Target()
	if err != nil {
		return model.CheckDecisionResponse{}, err
	}

	allowed, reasons, err := s.evaluate(ctx, req, object, relation)
	if err != nil {
		return model.CheckDecisionResponse{}, err
	}

	result := "deny"
	if allowed {
		result = "allow"
	}

	if !req.Options.IncludeReasons {
		reasons = []model.DecisionReason{}
	}

	resp := model.CheckDecisionResponse{
		RequestID: req.RequestID, DecisionID: uuid.New(), Result: result, Allow: allowed, Reasons: reasons,
		Meta: model.DecisionMeta{Subject: "user:" + req.Subject.ID.String(), Action: req.Action.Name,
			Resource: fmt.Sprintf("%s:%s", req.Resource.Type, req.Resource.ID), PolicyVersion: s.fields.Version(),
			AuthorizationModelID: s.relations.ModelID(), LatencyMS: time.Since(started).Milliseconds()},
	}
	s.record(ctx, caller, req, resp, object, relation)

	return resp, nil
}

func (s *Service) verifySubject(ctx context.Context, caller Caller, subject model.DecisionSubject) error {
	if caller.ClientID == "" {
		return ErrUnauthorized
	}

	if !model.ValidTelegramID(caller.TelegramID) {
		return model.ErrInvalidDecisionRequest
	}

	if subject.Attributes.TelegramUserID != nil && *subject.Attributes.TelegramUserID != caller.TelegramID {
		return ErrSubjectMismatch
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

func (s *Service) evaluate(ctx context.Context, req model.CheckDecisionRequest, object, relation string) (bool, []model.DecisionReason, error) {
	if req.Resource.Type == model.ResourceTypeSpace && req.Action.Name == model.ActionNameCreate {
		return true, reason("allowed_for_registered_user", "Registered user may create a space"), nil
	}

	allowed, err := s.relations.CheckRelation(ctx, "user:"+req.Subject.ID.String(), relation, object, req.ConsistencyPreference())
	if err != nil {
		return false, nil, fmt.Errorf("%w: %w", ErrOpenFGAUnavailable, err)
	}

	if !allowed {
		return false, reason("denied_by_fga_relation", "Required relation is absent"), nil
	}

	if req.Resource.Type == model.ResourceTypeMember {
		return s.checkMember(ctx, req, object)
	}

	reasons := reason("allowed_via_fga_relation", "Required relation is present")
	if req.Action.Name != model.ActionNameUpdate {
		return true, reasons, nil
	}

	forbidden, err := s.fields.CheckFields(string(req.Resource.Type), string(req.Action.Name), req.Action.Attributes.FieldMask)
	if err != nil {
		return false, nil, fmt.Errorf("%w: %w", ErrPolicyUnavailable, err)
	}

	if len(forbidden) > 0 {
		return false, reason("forbidden_fields", "User cannot modify fields: "+strings.Join(forbidden, ", ")), nil
	}

	return true, append(reasons, model.DecisionReason{Code: "allowed_fields", Message: "Requested fields are allowed"}), nil
}

func (s *Service) checkMember(ctx context.Context, req model.CheckDecisionRequest, object string) (bool, []model.DecisionReason, error) {
	owner, err := s.relations.CheckRelation(ctx, "user:"+req.Resource.ID.String(), "owner", object, req.ConsistencyPreference())
	if err != nil {
		return false, nil, fmt.Errorf("%w: %w", ErrOpenFGAUnavailable, err)
	}

	if owner {
		return false, reason("protected_space_owner", "Space owner membership cannot be changed"), nil
	}

	return true, reason("allowed_via_fga_relation", "Required relation is present"), nil
}

func reason(code, message string) []model.DecisionReason {
	return []model.DecisionReason{{Code: code, Message: message}}
}
