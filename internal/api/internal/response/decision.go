package response

import (
	"errors"
	"net/http"

	"auth-service/internal/model"
	"auth-service/internal/service/decision"
	"github.com/google/uuid"
)

// DecisionErrorBody содержит безопасную ошибку check без решения allow/deny.
type DecisionErrorBody struct {
	ErrorCode string     `json:"error_code"`
	Message   string     `json:"message"`
	RequestID *uuid.UUID `json:"request_id,omitempty"`
}

// DecisionError классифицирует ошибки только нового check API.
func DecisionError(err error, requestID uuid.UUID) Result[DecisionErrorBody] {
	status, code, message := classifyDecision(err)

	body := DecisionErrorBody{ErrorCode: code, Message: message}
	if requestID != uuid.Nil {
		body.RequestID = &requestID
	}

	return Result[DecisionErrorBody]{StatusCode: status, Body: body}
}

func classifyDecision(err error) (int, string, string) {
	switch {
	case errors.Is(err, model.ErrInvalidDecisionRequest):
		return http.StatusBadRequest, "invalid_request", "Invalid decision request"
	case errors.Is(err, decision.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized", "Valid service token is required"
	case errors.Is(err, decision.ErrForbidden):
		return http.StatusForbidden, "forbidden", "Client requires the bot scope"
	case errors.Is(err, decision.ErrSubjectMismatch):
		return http.StatusForbidden, "subject_mismatch", "Subject does not match the trusted identity"
	case errors.Is(err, decision.ErrUserNotFound):
		return http.StatusNotFound, "user_not_found", "User is not registered"
	case errors.Is(err, decision.ErrIdentityUnavailable):
		return http.StatusServiceUnavailable, "identity_store_unavailable", "Identity store is unavailable"
	case errors.Is(err, decision.ErrOpenFGAUnavailable):
		return http.StatusServiceUnavailable, "openfga_unavailable", "Relationship check is unavailable"
	case errors.Is(err, decision.ErrPolicyUnavailable):
		return http.StatusServiceUnavailable, "policy_unavailable", "Field policy is unavailable"
	default:
		return http.StatusInternalServerError, "internal_error", "Internal server error"
	}
}
