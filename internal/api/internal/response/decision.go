package response

import (
	"net/http"

	"auth-service/internal/service/decision"

	"github.com/google/uuid"
)

// DecisionErrorBody содержит безопасную ошибку check без решения allow/deny.
// Внутренние ошибки зависимостей не раскрываются; RequestID есть только при известном UUID.
type DecisionErrorBody struct {
	ErrorCode string     `json:"error_code" validate:"required"`
	Message   string     `json:"message" validate:"required"`
	RequestID *uuid.UUID `json:"request_id,omitempty"`
}

// DecisionError сопоставляет обёрнутые sentinel-ошибки статусу и стабильному коду check API.
// Неизвестная ошибка становится безопасным 500; старый формат ответа синхронизации не меняется.
func DecisionError(err error, requestID uuid.UUID) Result[DecisionErrorBody] {
	code, message := decision.DescribeError(err)

	body := DecisionErrorBody{ErrorCode: code, Message: message}
	if requestID != uuid.Nil {
		body.RequestID = &requestID
	}

	return Result[DecisionErrorBody]{StatusCode: decisionStatus(code), Body: body}
}

func decisionStatus(code string) int {
	switch code {
	case decision.CodeInvalidRequest:
		return http.StatusBadRequest
	case decision.CodeUnauthorized:
		return http.StatusUnauthorized
	case decision.CodeForbidden, decision.CodeSubjectMismatch:
		return http.StatusForbidden
	case decision.CodeUserNotFound:
		return http.StatusNotFound
	case decision.CodeIdentityUnavailable, decision.CodeDecisionStoreUnavailable, decision.CodeOpenFGAUnavailable, decision.CodePolicyUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
