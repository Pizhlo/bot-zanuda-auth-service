package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidDecisionRequest обозначает некорректный контракт проверки.
var ErrInvalidDecisionRequest = errors.New("invalid decision request")

const (
	// ResourceTypeMember обозначает целевого участника пространства.
	ResourceTypeMember ResourceType = "member"
	// ActionNameStop останавливает напоминание без удаления.
	ActionNameStop ActionName = "stop"
	// ActionNameAdd добавляет участника.
	ActionNameAdd ActionName = "add"
	// ActionNameEdit изменяет роль участника.
	ActionNameEdit ActionName = "edit"
	// ActionNameRemove удаляет участие пользователя.
	ActionNameRemove ActionName = "remove"
)

// Consistency задаёт предпочтение согласованности отношений.
type Consistency string

const (
	// HigherConsistency запрашивает повышенную согласованность.
	HigherConsistency Consistency = "higher_consistency"
	// MinimizeLatency допускает кеш OpenFGA ради меньшей задержки.
	MinimizeLatency Consistency = "minimize_latency"
)

// DecisionSubject содержит личность для сверки с доверенным заголовком.
type DecisionSubject struct {
	Type       ResourceType              `json:"type" validate:"required"`
	ID         uuid.UUID                 `json:"id" validate:"required"`
	Attributes DecisionSubjectAttributes `json:"attributes,omitempty"`
}

// DecisionSubjectAttributes содержит необязательное подтверждение Telegram ID.
type DecisionSubjectAttributes struct {
	TelegramUserID *string `json:"telegram_user_id,omitempty"`
}

// DecisionAction описывает одно действие.
type DecisionAction struct {
	Name       ActionName               `json:"name" validate:"required"`
	Attributes DecisionActionAttributes `json:"attributes,omitempty"`
}

// DecisionActionAttributes содержит типизированные параметры действия.
type DecisionActionAttributes struct {
	FieldMask []string `json:"field_mask,omitempty"`
	Role      *string  `json:"role,omitempty" enums:"viewer,editor,admin"`
}

// DecisionResource задаёт ресурс; для member ID является целевым пользователем.
type DecisionResource struct {
	Type       ResourceType               `json:"type" validate:"required"`
	ID         uuid.UUID                  `json:"id" validate:"required"`
	Attributes DecisionResourceAttributes `json:"attributes,omitempty"`
}

// DecisionResourceAttributes содержит родителя create/member или контекст существующей записи.
type DecisionResourceAttributes struct {
	SpaceID *uuid.UUID `json:"space_id,omitempty"`
}

// DecisionContext содержит недоверенные диагностические сведения клиента.
type DecisionContext struct {
	Service string  `json:"service,omitempty"`
	Client  string  `json:"client,omitempty"`
	IP      *string `json:"ip,omitempty"`
	TraceID string  `json:"trace_id,omitempty"`
	Time    *string `json:"time,omitempty"`
}

// DecisionOptions задаёт объяснения и согласованность.
type DecisionOptions struct {
	IncludeReasons bool         `json:"include_reasons" default:"false"`
	Consistency    *Consistency `json:"consistency,omitempty" enums:"higher_consistency,minimize_latency" default:"higher_consistency"`
}

// CheckDecisionRequest является независимым от UpdateResource контрактом проверки.
type CheckDecisionRequest struct {
	RequestID uuid.UUID        `json:"request_id" validate:"required"`
	Subject   DecisionSubject  `json:"subject" validate:"required"`
	Action    DecisionAction   `json:"action" validate:"required"`
	Resource  DecisionResource `json:"resource" validate:"required"`
	Context   DecisionContext  `json:"context,omitempty"`
	Options   DecisionOptions  `json:"options,omitempty"`
}

// DecisionReason объясняет подтверждённый этап решения.
type DecisionReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DecisionMeta содержит версии и канонические идентификаторы запроса.
type DecisionMeta struct {
	Subject              string     `json:"subject" validate:"required"`
	Action               ActionName `json:"action" validate:"required"`
	Resource             string     `json:"resource" validate:"required"`
	PolicyVersion        string     `json:"policy_version"`
	AuthorizationModelID string     `json:"authorization_model_id"`
	LatencyMS            int64      `json:"latency_ms"`
}

// CheckDecisionResponse содержит завершённое решение; ошибки имеют отдельное тело.
type CheckDecisionResponse struct {
	RequestID  uuid.UUID        `json:"request_id" validate:"required"`
	DecisionID uuid.UUID        `json:"decision_id"`
	Result     string           `json:"result"`
	Allow      bool             `json:"allow"`
	Reasons    []DecisionReason `json:"reasons"`
	Meta       DecisionMeta     `json:"meta"`
}

// DecodeDecision строго декодирует единственный JSON-объект без неизвестных полей и null.
func DecodeDecision(data []byte) (CheckDecisionRequest, error) {
	var req CheckDecisionRequest

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		return req, fmt.Errorf("%w: invalid JSON schema", ErrInvalidDecisionRequest)
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return req, fmt.Errorf("%w: expected one object", ErrInvalidDecisionRequest)
	}

	if err := rejectNull(data); err != nil {
		return req, err
	}

	return req, req.Validate()
}

func rejectNull(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil || token == nil {
			return fmt.Errorf("%w: null is not supported", ErrInvalidDecisionRequest)
		}
	}
}

// Validate проверяет смысл запроса до внешних операций.
func (r CheckDecisionRequest) Validate() error {
	if r.RequestID == uuid.Nil || r.Subject.ID == uuid.Nil || r.Resource.ID == uuid.Nil || r.Subject.Type != ResourceTypeUser {
		return fmt.Errorf("%w: invalid identifiers or subject type", ErrInvalidDecisionRequest)
	}

	if _, _, err := r.Target(); err != nil {
		return err
	}

	if err := r.validateAttributes(); err != nil {
		return err
	}

	if r.Options.Consistency != nil && *r.Options.Consistency != HigherConsistency && *r.Options.Consistency != MinimizeLatency {
		return fmt.Errorf("%w: invalid consistency", ErrInvalidDecisionRequest)
	}

	if r.Subject.Attributes.TelegramUserID != nil && !ValidTelegramID(*r.Subject.Attributes.TelegramUserID) {
		return fmt.Errorf("%w: invalid telegram_user_id", ErrInvalidDecisionRequest)
	}

	return r.Context.validate()
}

func (r CheckDecisionRequest) validateAttributes() error {
	if err := r.validateActionAttributes(); err != nil {
		return err
	}

	space := r.Resource.Attributes.SpaceID

	requiresSpace := r.Resource.Type == ResourceTypeMember || (r.Action.Name == ActionNameCreate && r.Resource.Type != ResourceTypeSpace)
	if (requiresSpace && space == nil) || (space != nil && (*space == uuid.Nil || r.Resource.Type == ResourceTypeSpace)) {
		return fmt.Errorf("%w: invalid space_id", ErrInvalidDecisionRequest)
	}

	return nil
}

func (r CheckDecisionRequest) validateActionAttributes() error {
	fields := r.Action.Attributes.FieldMask
	if r.Action.Name == ActionNameUpdate {
		if len(fields) == 0 || slices.ContainsFunc(fields, func(s string) bool { return strings.TrimSpace(s) == "" }) {
			return fmt.Errorf("%w: nonempty field_mask is required", ErrInvalidDecisionRequest)
		}
	} else if fields != nil {
		return fmt.Errorf("%w: field_mask is only valid for update", ErrInvalidDecisionRequest)
	}

	memberRole := r.Resource.Type == ResourceTypeMember && (r.Action.Name == ActionNameAdd || r.Action.Name == ActionNameEdit)

	role := r.Action.Attributes.Role
	if memberRole {
		if role == nil || !slices.Contains([]string{"viewer", "editor", "admin"}, *role) {
			return fmt.Errorf("%w: invalid member role", ErrInvalidDecisionRequest)
		}
	} else if role != nil {
		return fmt.Errorf("%w: role is not supported for this action", ErrInvalidDecisionRequest)
	}

	return nil
}

func (c DecisionContext) validate() error {
	if c.IP != nil {
		if _, err := netip.ParseAddr(*c.IP); err != nil {
			return fmt.Errorf("%w: invalid context IP", ErrInvalidDecisionRequest)
		}
	}

	if c.Time != nil {
		if _, err := time.Parse(time.RFC3339, *c.Time); err != nil {
			return fmt.Errorf("%w: invalid context time", ErrInvalidDecisionRequest)
		}
	}

	return nil
}

// ValidTelegramID принимает каноническую положительную десятичную запись Telegram ID.
func ValidTelegramID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}

// ConsistencyPreference возвращает явное предпочтение либо безопасный default.
func (r CheckDecisionRequest) ConsistencyPreference() Consistency {
	if r.Options.Consistency == nil {
		return HigherConsistency
	}

	return *r.Options.Consistency
}

// Target определяет объект и relation; пустой target означает исключение space:create.
func (r CheckDecisionRequest) Target() (string, string, error) {
	relation := ""

	object := string(r.Resource.Type) + ":" + r.Resource.ID.String()
	switch r.Resource.Type {
	case ResourceTypeNote, ResourceTypeReminder:
		relation = r.recordRelation()
		if r.Action.Name == ActionNameCreate && r.Resource.Attributes.SpaceID != nil {
			object = "space:" + r.Resource.Attributes.SpaceID.String()
		}
	case ResourceTypeSpace:
		if r.Action.Name == ActionNameCreate {
			return "", "", nil
		}

		relation = spaceRelation(r.Action.Name)
	case ResourceTypeMember:
		if slices.Contains([]ActionName{ActionNameAdd, ActionNameEdit, ActionNameRemove}, r.Action.Name) {
			relation = "can_manage_members"

			if r.Resource.Attributes.SpaceID != nil {
				object = "space:" + r.Resource.Attributes.SpaceID.String()
			}
		}
	}

	if relation == "" {
		return "", "", fmt.Errorf("%w: unsupported resource/action", ErrInvalidDecisionRequest)
	}

	return object, relation, nil
}

func spaceRelation(action ActionName) string {
	switch action {
	case ActionNameRead:
		return "can_view"
	case ActionNameUpdate:
		return "can_manage_space"
	case ActionNameDelete:
		return "can_delete"
	default:
		return ""
	}
}

func (r CheckDecisionRequest) recordRelation() string {
	switch r.Action.Name {
	case ActionNameCreate:
		return "can_create_" + string(r.Resource.Type)
	case ActionNameRead:
		return "can_view"
	case ActionNameUpdate:
		return "can_update"
	case ActionNameDelete:
		return "can_delete"
	case ActionNameStop:
		if r.Resource.Type == ResourceTypeReminder {
			return "can_stop"
		}
	}

	return ""
}
