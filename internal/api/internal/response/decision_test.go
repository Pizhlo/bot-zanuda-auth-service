package response

import (
	"errors"
	"fmt"
	"testing"

	"auth-service/internal/model"
	"auth-service/internal/service/decision"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDecisionError(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	for _, tc := range []struct {
		name          string
		err           error
		id            uuid.UUID
		status        int
		code, message string
		wantID        *uuid.UUID
	}{
		{"validation details without trusted ID", model.InvalidDecisionRequest("X-Telegram-User-Id header is required"), uuid.Nil, 400, "invalid_request", "X-Telegram-User-Id header is required", nil},
		{"validation sentinel", model.ErrInvalidDecisionRequest, id, 400, "invalid_request", "Invalid decision request", &id},
		{"dependency failure preserves ID", decision.ErrIdentityUnavailable, id, 503, "identity_store_unavailable", "Identity store is unavailable", &id},
		{"unknown error without correlation", errors.New("secret"), uuid.Nil, 500, "internal_error", "Internal server error", nil},
		{"precedence of joined errors", errors.Join(decision.ErrOpenFGAUnavailable, decision.ErrUnauthorized), id, 401, "unauthorized", "Valid service token is required", &id},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := DecisionError(fmt.Errorf("private details: %w", tc.err), tc.id)

			require.Equal(t, tc.status, result.StatusCode)
			require.Equal(t, DecisionErrorBody{ErrorCode: tc.code, Message: tc.message, RequestID: tc.wantID}, result.Body)
		})
	}
}

func TestDecisionStatus(t *testing.T) {
	t.Parallel()

	for code, want := range map[string]int{"invalid_request": 400, "unauthorized": 401, "forbidden": 403, "subject_mismatch": 403, "user_not_found": 404, "decision_store_unavailable": 503, "identity_store_unavailable": 503, "openfga_unavailable": 503, "policy_unavailable": 503, "unknown": 500} {
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, want, decisionStatus(code))
		})
	}
}
