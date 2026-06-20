package model

import (
	"fmt"

	"github.com/google/uuid"
)

// ResourceType определяет тип ресурса.
type ResourceType string

const (
	// ResourceTypeSpace - пространство.
	ResourceTypeSpace = "space"
	// ResourceTypeNote - заметка.
	ResourceTypeNote = "note"
	// ResourceTypeReminder - напоминание.
	ResourceTypeReminder = "reminder"
	// ResourceTypeUser - пользователь.
	ResourceTypeUser = "user"
)

// Resource - ресурс.
type Resource struct {
	ID   uuid.UUID    `json:"id"`
	Type ResourceType `json:"type"`
}

// ParentRelationName возвращает имя relation для связи ресурса с родителем в auth-модели OpenFGA.
func (r Resource) ParentRelationName() (string, error) {
	switch r.Type {
	case ResourceTypeNote, ResourceTypeReminder:
		return "space", nil
	default:
		return "", fmt.Errorf("resource type %s does not support parent relation", r.Type)
	}
}

// Relation - связь ресурса с родителем и участниками.
type Relation struct {
	Owner     Resource `json:"owner"`      // создатель ресурса
	Parent    Resource `json:"parent"`     // к чему принадлежит ресурс (пространство)
	OldParent Resource `json:"old_parent"` // прежний родитель (для resource_moved)
	Members   []Member `json:"members"`    // изменение прав участников ресурса
}

// Member - участник ресурса.
type Member struct {
	UserID  uuid.UUID `json:"user_id"`
	OldRole RoleCode  `json:"old_role"`
	NewRole RoleCode  `json:"new_role"`
}

// Operation - тип операции обновления ресурса.
type Operation string

const (
	// OperationCreate - создание ресурса.
	OperationCreate Operation = "create" // создание ресурса
	// OperationUpdate - обновление ресурса.
	OperationUpdate Operation = "update" // обновление ресурса
	// OperationDelete - удаление ресурса.
	OperationDelete Operation = "delete" // удаление ресурса
)

// ChangeType - тип изменения ресурса.
type ChangeType string

const (
	// ChangeTypeMembershipChanged - изменение прав участников ресурса.
	ChangeTypeMembershipChanged ChangeType = "membership_changed" // изменение прав участников ресурса
	// ChangeTypeMembershipAdded - добавление участника в ресурс.
	ChangeTypeMembershipAdded ChangeType = "membership_added" // добавление участника в ресурс
	// ChangeTypeMembershipRemoved - удаление участника из ресурса.
	ChangeTypeMembershipRemoved ChangeType = "membership_removed" // удаление участника из ресурса
	// ChangeTypeResourceAdded - создание ресурса.
	ChangeTypeResourceAdded ChangeType = "resource_added" // создание ресурса
	// ChangeTypeResourceRemoved - удаление ресурса из ресурса.
	ChangeTypeResourceRemoved ChangeType = "resource_removed" // удаление ресурса из ресурса
	// ChangeTypeResourceMoved - перемещение ресурса в ресурс, изменение родителя.
	ChangeTypeResourceMoved ChangeType = "resource_moved" // перемещение ресурса в ресурс, изменение родителя
)

// Context - контекст обновления ресурса.
type Context struct {
	SourceService string `json:"source_service"` // сервис, который вызвал обновление ресурса
	EventType     string `json:"event_type"`     // тип события, которое вызвало обновление ресурса
	TraceID       string `json:"trace_id"`       // trace ID для отслеживания обновления ресурса
}

// UpdateResourceRequest - запрос на обновление ресурса.
// Используется для любого вида обновления: создание, удаление, изменение, перемещение.
type UpdateResourceRequest struct {
	RequestID      uuid.UUID  `json:"request_id"`
	IdempotencyKey string     `json:"idempotency_key"`
	DecisionID     uuid.UUID  `json:"decision_id"`
	Resource       Resource   `json:"resource"`
	Relations      Relation   `json:"relations"`
	Operation      Operation  `json:"operation"`
	ChangeType     ChangeType `json:"change_type"`
	Context        Context    `json:"context"`
	TelegramID     int        `json:"-"` // оригинальный айди пользователя из хедера X-Telegram-User-Id
}

// Status - статус обновления ресурса.
type Status string

const (
	// StatusPending - статус обновления ресурса в процессе.
	StatusPending Status = "pending"
	// StatusCompleted - статус обновления ресурса успешно завершен.
	StatusCompleted Status = "completed"
	// StatusFailed - статус обновления ресурса неуспешно завершен.
	StatusFailed Status = "failed"
	// StatusError - статус обновления ресурса с ошибкой.
	StatusError Status = "error"
)

// Result - результат обновления ресурса.
type Result string

const (
	// ResultApplied - результат обновления ресурса успешно применен.
	ResultApplied Result = "applied"
	// ResultRejected - результат обновления ресурса неуспешно применен.
	ResultRejected Result = "rejected"
	ResultFailed   Result = "failed"
)

// Tuple - tuple для запроса на обновление ресурса.
type Tuple struct {
	Subject  string `json:"subject"`
	Relation string `json:"relation"`
	Resource string `json:"resource"`
}

// Meta - метаданные ответа на запрос на обновление ресурса.
type Meta struct {
	AuthModelID string `json:"auth_model_id"`
}

// UpdateResourceResponse - ответ на запрос на обновление ресурса.
type UpdateResourceResponse struct {
	RequestID      uuid.UUID `json:"request_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Status         Status    `json:"status"`
	Result         Result    `json:"result"`
	Resource       Resource  `json:"resource"`
	WrittenTuples  []Tuple   `json:"written_tuples"`
	DeletedTuples  []Tuple   `json:"deleted_tuples"`
	Meta           Meta      `json:"meta"`
}
