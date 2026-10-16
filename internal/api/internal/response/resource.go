package response

import (
	"errors"

	"auth-service/internal/model"
	"auth-service/internal/service/fga"
	"auth-service/pkg/audit"

	"github.com/google/uuid"
)

// ResourceContext содержит только метаданные ресурсного ответа, без модели запроса.
type ResourceContext struct {
	RequestID uuid.UUID
	Resource  model.Resource
	Operation model.Operation
	Meta      model.Meta
}

// ResourceBody сохраняет JSON-контракт ошибочного ответа resources.
type ResourceBody struct {
	RequestID    uuid.UUID    `json:"request_id"`
	Status       model.Status `json:"status"`
	Result       model.Result `json:"result"`
	Resource     Resource     `json:"resource"`
	ErrorMessage Message      `json:"error"`
	Meta         Meta         `json:"meta"`
}

// Message содержит публичное описание ошибки и подробности операции.
type Message struct {
	Code    audit.ErrorCode `json:"code"`
	Message string          `json:"message"`
	Details Details         `json:"details"`
}

// Details содержит подробности операции; отсутствующая ошибка сериализуется как null.
type Details struct {
	Operation     model.Operation `json:"operation"`
	DetailedError *DetailedError  `json:"detailed_error"`
}

// Resource содержит поля ресурса для ошибочного ответа.
type Resource struct {
	ID   uuid.UUID          `json:"id"`
	Type model.ResourceType `json:"type"`
}

// Meta содержит метаданные ошибочного ответа.
type Meta struct {
	AuthModelID string `json:"auth_model_id"`
}

// DetailedError содержит только публичные подробности ошибки.
type DetailedError struct {
	Message string `json:"message"`
	Value   any    `json:"value"`
}

// ResourceError формирует ресурсный ответ; неизвестные ошибки дают безопасный fallback 500.
func ResourceError(err error, ctx ResourceContext) Result[ResourceBody] {
	detailed, ok := errors.AsType[*fga.DetailedError](err)
	if ok {
		err = detailed.Err
	}

	classified := classify(err)

	var publicDetails *DetailedError
	if detailed != nil {
		publicDetails = &DetailedError{Message: detailed.Message, Value: detailed.Value}
	}

	return Result[ResourceBody]{
		StatusCode: classified.StatusCode,
		Body: ResourceBody{
			RequestID: ctx.RequestID, Status: model.StatusError, Result: model.ResultFailed,
			Resource: Resource{ID: ctx.Resource.ID, Type: ctx.Resource.Type}, Meta: Meta{AuthModelID: ctx.Meta.AuthModelID},
			ErrorMessage: Message{Code: classified.Code, Message: classified.Message,
				Details: Details{Operation: ctx.Operation, DetailedError: publicDetails}},
		},
	}
}
