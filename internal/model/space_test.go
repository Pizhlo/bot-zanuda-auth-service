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

func TestIsOwner(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		role RoleCode
		want bool
	}{
		{"stored owner code", OwnerRoleCode, true},
		{"FGA owner relation", RoleCode("owner"), true},
		{"admin", RoleCode("admin"), false},
		{"empty", RoleCode(""), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.role.IsOwner()

			require.Equal(t, tc.want, got)
		})
	}
}
