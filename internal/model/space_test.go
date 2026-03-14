package model

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSpaceMember_CanEditNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		member SpaceMember
		want   require.BoolAssertionFunc
	}{
		{
			name: "role: OWNER",
			member: SpaceMember{
				UserID:   uuid.New(),
				RoleCode: OwnerRoleCode,
			},
			want: require.True,
		},
		{
			name: "role: ADMIN",
			member: SpaceMember{
				UserID:   uuid.New(),
				RoleCode: AdminRoleCode,
			},
			want: require.True,
		},
		{
			name: "role: EDITOR",
			member: SpaceMember{
				UserID:   uuid.New(),
				RoleCode: EditorRoleCode,
			},
			want: require.True,
		},
		{
			name: "role: VIEWER",
			member: SpaceMember{
				UserID:   uuid.New(),
				RoleCode: ViewerRoleCode,
			},
			want: require.False,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.want(t, tt.member.CanEditSpaceNote())
		})
	}
}
