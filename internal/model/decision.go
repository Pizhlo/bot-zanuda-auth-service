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

// ResourceTypeMember обозначает целевого участника пространства.
const ResourceTypeMember ResourceType = "member"

// Consistency задаёт режим чтения OpenFGA для всех проверок одного решения.
// Он не создаёт транзакцию между check и последующей синхронизацией.
type Consistency string

const (
	// HigherConsistency запрашивает повышенную согласованность.
	HigherConsistency Consistency = "higher_consistency"
	// MinimizeLatency допускает кеш OpenFGA ради меньшей задержки.
	MinimizeLatency Consistency = "minimize_latency"
)

// DecisionSubject — заявленный в теле пользователь. Type должен быть user,
// а ID — внутренним UUID из хранилища по доверенному Telegram-заголовку.
// До совпадения идентичностей проверки прав не выполняются.
type DecisionSubject struct {
	Type       ResourceType              `json:"type" validate:"required" swaggertype:"string" enums:"user"`
	ID         uuid.UUID                 `json:"id" validate:"required"`
	Attributes DecisionSubjectAttributes `json:"attributes,omitempty"`
}

// DecisionSubjectAttributes содержит диагностическую копию Telegram ID.
// Если она передана, её значение обязано совпасть с заголовком; она не выбирает пользователя.
type DecisionSubjectAttributes struct {
	TelegramUserID *string `json:"telegram_user_id,omitempty"`
}

// DecisionAction описывает ровно одно действие над одним ресурсом.
// Допустимые сочетания действия, типа ресурса и атрибутов проверяет сервис решений.
type DecisionAction struct {
	Name       ActionName               `json:"name" validate:"required"`
	Attributes DecisionActionAttributes `json:"attributes,omitempty"`
}

// DecisionActionAttributes содержит параметры, допустимые только для своего действия.
// FieldMask обязателен для update: одно запрещённое поле запрещает всю операцию.
// Role обязателен для member:add/edit и не может назначать owner.
type DecisionActionAttributes struct {
	FieldMask []string `json:"field_mask,omitempty"`
	Role      *string  `json:"role,omitempty" enums:"viewer,editor,admin"`
}

// DecisionResource задаёт запрошенный объект, в том числе будущий при create.
// Для member ID обозначает пользователя, чьё участие меняют, а SpaceID — пространство.
// Проверяемый объект FGA может отличаться от этого ресурса; его выбирает сервис решений.
type DecisionResource struct {
	Type       ResourceType               `json:"type" validate:"required" swaggertype:"string" enums:"note,reminder,space,member"`
	ID         uuid.UUID                  `json:"id" validate:"required"`
	Attributes DecisionResourceAttributes `json:"attributes,omitempty"`
}

// DecisionResourceAttributes содержит SpaceID, обязательный для create записей и member.
// Для существующей записи SpaceID только диагностический: он не заменяет parent в FGA.
type DecisionResourceAttributes struct {
	SpaceID *uuid.UUID `json:"space_id,omitempty"`
	// SpaceType обязателен только для space:create и задаёт тип будущего пространства.
	SpaceType *SpaceKind `json:"space_type,omitempty" enums:"shared,personal"`
}

// DecisionContext содержит недоверенные диагностические сведения клиента.
// Они не влияют на права и не заменяют caller или серверное время аудита.
// Переданные IP и Time всё равно должны иметь корректный формат.
type DecisionContext struct {
	Service string  `json:"service,omitempty"`
	Client  string  `json:"client,omitempty"`
	IP      *string `json:"ip,omitempty"`
	TraceID string  `json:"trace_id,omitempty"`
	Time    *string `json:"time,omitempty"`
}

// DecisionOptions управляет объяснениями и режимом чтения отношений.
// Без IncludeReasons ответ содержит reasons: []; без Consistency используется higher_consistency.
type DecisionOptions struct {
	IncludeReasons bool         `json:"include_reasons" default:"false"`
	Consistency    *Consistency `json:"consistency,omitempty" enums:"higher_consistency,minimize_latency" default:"higher_consistency"`
}

// CheckDecisionRequest — отдельный от синхронизации контракт проверки одной операции.
// RequestID служит корреляции: повтор запроса вычисляет новое решение и новый DecisionID.
type CheckDecisionRequest struct {
	RequestID uuid.UUID        `json:"request_id" validate:"required"`
	Subject   DecisionSubject  `json:"subject" validate:"required"`
	Action    DecisionAction   `json:"action" validate:"required"`
	Resource  DecisionResource `json:"resource" validate:"required"`
	Context   DecisionContext  `json:"context,omitempty"`
	Options   DecisionOptions  `json:"options,omitempty"`
}

// DecisionReason объясняет подтверждённый этап решения: отношение, поля или защитный запрет.
// Code — стабильный код для клиента, Message — проверенные relation/object/subject
// либо поля запрошенной маски, отсортированные и без дублей; не вся политика.
// Boolean-ответ OpenFGA не позволяет утверждать конкретный путь наследования роли.
type DecisionReason struct {
	Code    string `json:"code" validate:"required"`
	Message string `json:"message" validate:"required"`
}

// DecisionMeta связывает результат с доверенным пользователем, запрошенным объектом
// и фактически использованными версиями правил. Resource не подменяется parent-объектом.
// LatencyMS измеряет вычисление решения до доставки аудита.
type DecisionMeta struct {
	Subject              string     `json:"subject" validate:"required"`
	Action               ActionName `json:"action" validate:"required"`
	Resource             string     `json:"resource" validate:"required"`
	PolicyVersion        string     `json:"policy_version" validate:"required"`
	AuthorizationModelID string     `json:"authorization_model_id" validate:"required"`
	LatencyMS            int64      `json:"latency_ms" validate:"required"`
}

// CheckDecisionResponse возвращается с HTTP 200 как для allow, так и для deny.
// Result и Allow согласованы, Reasons всегда массив. Ошибки проверки имеют отдельное тело
// без решения; DecisionID не является токеном разрешения на последующую запись.
type CheckDecisionResponse struct {
	RequestID  uuid.UUID        `json:"request_id" validate:"required"`
	DecisionID uuid.UUID        `json:"decision_id" validate:"required"`
	Result     string           `json:"result" validate:"required"`
	Allow      bool             `json:"allow" validate:"required"`
	Reasons    []DecisionReason `json:"reasons" validate:"required"`
	Meta       DecisionMeta     `json:"meta" validate:"required"`
}

// DecodeDecision декодирует один JSON-объект, отклоняя неизвестные поля, null
// и несколько JSON-значений. Затем Validate проверяет условные поля и идентификаторы.
// Имена полей в каждом объекте должны быть каноническими и уникальными.
func DecodeDecision(data []byte) (CheckDecisionRequest, error) {
	var req CheckDecisionRequest

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	decodeErr := decoder.Decode(&req)
	if err := validateDecisionJSON(data); err != nil {
		if req.RequestID != uuid.Nil && !hasUniqueDecisionRequestID(data) {
			req.RequestID = uuid.Nil
		}

		return req, fmt.Errorf("validate JSON structure: %w", errors.Join(err, decodeErr))
	}

	if decodeErr != nil {
		return req, fmt.Errorf("decode decision body: %w: %w", InvalidDecisionRequest("body contains invalid JSON"), decodeErr)
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return req, InvalidDecisionRequest("body must contain exactly one JSON object")
	}

	if err := req.Validate(); err != nil {
		return req, fmt.Errorf("validate decision input: %w", err)
	}

	return req, nil
}

// Validate выполняет первичную проверку DTO: обязательные базовые поля и форматы.
// Матрица действий, условные атрибуты и допустимые назначения ролей проверяются сервисом.
func (r CheckDecisionRequest) Validate() error {
	if err := r.validateIdentifiers(); err != nil {
		return fmt.Errorf("validate decision identifiers: %w", err)
	}

	if slices.ContainsFunc(r.Action.Attributes.FieldMask, func(field string) bool { return strings.TrimSpace(field) == "" }) {
		return InvalidDecisionRequest("action.attributes.field_mask must contain nonempty field names")
	}

	if r.Options.Consistency != nil && *r.Options.Consistency != HigherConsistency && *r.Options.Consistency != MinimizeLatency {
		return InvalidDecisionRequest("options.consistency must be higher_consistency or minimize_latency")
	}

	if r.Subject.Attributes.TelegramUserID != nil && !ValidTelegramID(*r.Subject.Attributes.TelegramUserID) {
		return InvalidDecisionRequest("subject.attributes.telegram_user_id must be a positive decimal int64 without spaces, signs or leading zeros")
	}

	if err := r.Context.validate(); err != nil {
		return fmt.Errorf("validate decision context: %w", err)
	}

	return nil
}

func (r CheckDecisionRequest) validateIdentifiers() error {
	if r.RequestID == uuid.Nil {
		return InvalidDecisionRequest("request_id must be a nonzero UUID")
	}

	if r.Subject.ID == uuid.Nil {
		return InvalidDecisionRequest("subject.id must be a nonzero UUID")
	}

	if r.Resource.ID == uuid.Nil {
		return InvalidDecisionRequest("resource.id must be a nonzero UUID")
	}

	if r.Subject.Type != ResourceTypeUser {
		return InvalidDecisionRequest("subject.type must be user")
	}

	if strings.TrimSpace(string(r.Action.Name)) == "" {
		return InvalidDecisionRequest("action.name is required")
	}

	if strings.TrimSpace(string(r.Resource.Type)) == "" {
		return InvalidDecisionRequest("resource.type is required")
	}

	if space := r.Resource.Attributes.SpaceID; space != nil && *space == uuid.Nil {
		return InvalidDecisionRequest("resource.attributes.space_id must be a nonzero UUID")
	}

	return nil
}

func (c DecisionContext) validate() error {
	if c.IP != nil {
		if _, err := netip.ParseAddr(*c.IP); err != nil {
			return fmt.Errorf("parse context IP: %w: %w", InvalidDecisionRequest("context.ip must be a valid IPv4 or IPv6 address"), err)
		}
	}

	if c.Time != nil {
		if _, err := time.Parse(time.RFC3339, *c.Time); err != nil {
			return fmt.Errorf("parse context time: %w: %w", InvalidDecisionRequest("context.time must be a valid RFC3339 timestamp"), err)
		}
	}

	return nil
}

// ValidTelegramID принимает каноническую положительную десятичную запись Telegram ID.
func ValidTelegramID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}

// ConsistencyPreference возвращает указанный режим либо higher_consistency.
// Вызывать после Validate: неизвестный явный режим здесь не исправляется автоматически.
func (r CheckDecisionRequest) ConsistencyPreference() Consistency {
	if r.Options.Consistency == nil {
		return HigherConsistency
	}

	return *r.Options.Consistency
}
