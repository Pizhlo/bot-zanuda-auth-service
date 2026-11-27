package decision

import (
	"errors"
	"fmt"
	"testing"

	"auth-service/internal/model"
	"github.com/stretchr/testify/require"
)

func TestDescribeError(t *testing.T) {
	t.Parallel()

	const fieldMessage = "resource.attributes.space_type is required for space:create"

	validation := model.InvalidDecisionRequest(fieldMessage)

	tests := []struct {
		name        string
		err         error
		wantCode    string
		wantMessage string
	}{
		{"unauthorized", ErrUnauthorized, "unauthorized", "Valid service token is required"},
		{"forbidden", ErrForbidden, "forbidden", "Client requires the bot scope"},
		{"subject mismatch", ErrSubjectMismatch, "subject_mismatch", "Subject does not match the trusted identity"},
		{"user not found", ErrUserNotFound, "user_not_found", "User is not registered"},
		{"decision store", ErrDecisionStoreUnavailable, "decision_store_unavailable", "Decision store is unavailable"},
		{"identity store", ErrIdentityUnavailable, "identity_store_unavailable", "Identity store is unavailable"},
		{"OpenFGA", ErrOpenFGAUnavailable, "openfga_unavailable", "Relationship check is unavailable"},
		{"field policy", ErrPolicyUnavailable, "policy_unavailable", "Field policy is unavailable"},
		{"wrapped dependency", fmt.Errorf("private database detail: %w", ErrIdentityUnavailable), "identity_store_unavailable", "Identity store is unavailable"},
		{"validation without field", fmt.Errorf("private input: %w", model.ErrInvalidDecisionRequest), "invalid_request", "Invalid decision request"},
		{"wrapped field validation", fmt.Errorf("private input: %w", validation), "invalid_request", fieldMessage},
		{"validation takes priority", errors.Join(ErrUnauthorized, validation), "invalid_request", fieldMessage},
		{"authorization takes priority", errors.Join(ErrOpenFGAUnavailable, ErrUnauthorized), "unauthorized", "Valid service token is required"},
		{"unrelated error with same text", errors.New("unauthorized"), "internal_error", "Internal server error"},
		{"nil", nil, "internal_error", "Internal server error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			code, message := DescribeError(tt.err)

			require.Equal(t, tt.wantCode, code)
			require.Equal(t, tt.wantMessage, message)
		})
	}
}
