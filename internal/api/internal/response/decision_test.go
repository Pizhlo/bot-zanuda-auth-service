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

func TestDecisionErrorClassification(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{model.ErrInvalidDecisionRequest, 400, "invalid_request"}, {decision.ErrUnauthorized, 401, "unauthorized"},
		{decision.ErrForbidden, 403, "forbidden"}, {decision.ErrSubjectMismatch, 403, "subject_mismatch"},
		{decision.ErrUserNotFound, 404, "user_not_found"}, {decision.ErrIdentityUnavailable, 503, "identity_store_unavailable"},
		{decision.ErrOpenFGAUnavailable, 503, "openfga_unavailable"}, {decision.ErrPolicyUnavailable, 503, "policy_unavailable"},
		{errors.New("secret"), 500, "internal_error"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			t.Parallel()

			id := uuid.New()
			result := DecisionError(fmt.Errorf("private details: %w", tt.err), id)
			require.Equal(t, tt.status, result.StatusCode)
			require.Equal(t, tt.code, result.Body.ErrorCode)
			require.NotContains(t, result.Body.Message, "private")
			require.NotContains(t, result.Body.Message, "secret")
			require.Equal(t, &id, result.Body.RequestID)
			require.Nil(t, DecisionError(tt.err, uuid.Nil).Body.RequestID)
		})
	}
}
