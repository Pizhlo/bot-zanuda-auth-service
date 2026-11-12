package decision

import (
	"slices"
	"testing"

	"auth-service/internal/model"
	"github.com/stretchr/testify/require"
)

func TestReason(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, code, message string
	}{
		{"registered user", model.ReasonAllowedForRegisteredUser, "Shared space creation is allowed"},
		{"first personal space", model.ReasonFirstPersonalSpace, "No personal space found"},
		{"existing personal space", model.ReasonPersonalSpaceAlreadyExists, "Personal space exists"},
		{"relation present", model.ReasonAllowedViaFGARelation, "can_view on note"},
		{"relation absent", model.ReasonDeniedByFGARelation, "can_view is absent"},
		{"allowed fields", model.ReasonAllowedFields, "Allowed fields for note:update: text, title"},
		{"forbidden fields", model.ReasonForbiddenFields, "Forbidden fields for note:update: created_at"},
		{"protected owner", model.ReasonProtectedSpaceOwner, "Membership cannot be changed"},
		{"empty code", "", "reason text"},
		{"empty message", model.ReasonDeniedByFGARelation, ""},
		{"empty code and message", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := reason(tc.code, tc.message)

			require.Equal(t, []model.DecisionReason{{Code: tc.code, Message: tc.message}}, got)
		})
	}
}

func TestReasonFields(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		fields []string
		want   string
	}{
		{"nil", nil, ""},
		{"empty", []string{}, ""},
		{"one field", []string{"title"}, "title"},
		{"sorted unique", []string{"text", "title"}, "text, title"},
		{"unsorted unique", []string{"title", "text"}, "text, title"},
		{"all identical", []string{"title", "title", "title"}, "title"},
		{"adjacent duplicates", []string{"text", "text", "title", "title"}, "text, title"},
		{"unsorted with duplicates", []string{"title", "text", "created_at", "title", "text"}, "created_at, text, title"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := slices.Clone(tt.fields)

			got := reasonFields(tt.fields)

			require.Equal(t, tt.want, got)
			require.Equal(t, original, tt.fields)
		})
	}
}
