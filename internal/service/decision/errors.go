package decision

import (
	"errors"

	"auth-service/internal/model"
)

// Публичные коды ошибок проверки решения используются в HTTP и аудите.
const (
	CodeUnauthorized             = "unauthorized"
	CodeForbidden                = "forbidden"
	CodeSubjectMismatch          = "subject_mismatch"
	CodeUserNotFound             = "user_not_found"
	CodeIdentityUnavailable      = "identity_store_unavailable"
	CodeOpenFGAUnavailable       = "openfga_unavailable"
	CodeDecisionStoreUnavailable = "decision_store_unavailable"
	CodePolicyUnavailable        = "policy_unavailable"
	CodeInvalidRequest           = "invalid_request"
	CodeInternalError            = "internal_error"
)

// DescribeError классифицирует причину для публичного ответа и аудита, скрывая детали зависимостей.
func DescribeError(err error) (string, string) {
	if errors.Is(err, model.ErrInvalidDecisionRequest) {
		message := "Invalid decision request"
		if validation, ok := errors.AsType[*model.DecisionValidationError](err); ok {
			message = validation.Error()
		}

		return CodeInvalidRequest, message
	}

	// При нескольких обёрнутых причинах первая совпавшая категория определяет ответ.
	descriptions := [...]struct {
		cause   error
		message string
	}{
		{ErrUnauthorized, "Valid service token is required"},
		{ErrForbidden, "Client requires the bot scope"},
		{ErrSubjectMismatch, "Subject does not match the trusted identity"},
		{ErrUserNotFound, "User is not registered"},
		{ErrDecisionStoreUnavailable, "Decision store is unavailable"},
		{ErrIdentityUnavailable, "Identity store is unavailable"},
		{ErrOpenFGAUnavailable, "Relationship check is unavailable"},
		{ErrPolicyUnavailable, "Field policy is unavailable"},
	}
	for _, description := range descriptions {
		if errors.Is(err, description.cause) {
			return description.cause.Error(), description.message
		}
	}

	return CodeInternalError, "Internal server error"
}
