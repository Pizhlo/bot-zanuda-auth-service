package permissions

import (
	"auth-service/internal/model"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест
func TestNotePermissionResolver_ResolveNotePermissions(t *testing.T) {
	t.Parallel()

	type args struct {
		access  model.SpaceMember
		noteIDs []int
	}

	tests := []struct {
		name    string
		args    args
		want    map[int]model.NoteAccessInfo
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "positive case: full access (user is owner)",
			args: args{
				access: model.SpaceMember{
					RoleCode: string(model.OwnerRoleCode),
				},
				noteIDs: []int{1, 2, 3},
			},
			want: map[int]model.NoteAccessInfo{
				1: {CanRead: true, CanEdit: true},
				2: {CanRead: true, CanEdit: true},
				3: {CanRead: true, CanEdit: true},
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: non owner, no access",
			args: args{
				access: model.SpaceMember{
					RoleCode: string(model.EditorRoleCode),
				},
				noteIDs: []int{1, 2, 3},
			},
			want:    map[int]model.NoteAccessInfo{},
			wantErr: require.NoError,
		},
		{
			name: "positive case: owner with empty note ids returns empty map",
			args: args{
				access: model.SpaceMember{
					RoleCode: string(model.OwnerRoleCode),
				},
				noteIDs: []int{},
			},
			want:    map[int]model.NoteAccessInfo{},
			wantErr: require.NoError,
		},
	}

	resolver := NewNotePermissionResolver()

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolver.ResolveNotePermissions(context.Background(), tt.args.access, tt.args.noteIDs)
			tt.wantErr(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReturnAllNotesWithFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		noteIDs []int
		canEdit bool
		canRead bool
		want    map[int]model.NoteAccessInfo
	}{
		{
			name:    "full access",
			noteIDs: []int{1, 2, 3},
			canEdit: true,
			canRead: true,
			want: map[int]model.NoteAccessInfo{
				1: {CanRead: true, CanEdit: true},
				2: {CanRead: true, CanEdit: true},
				3: {CanRead: true, CanEdit: true},
			},
		},
		{
			name:    "read only",
			noteIDs: []int{10, 20},
			canEdit: false,
			canRead: true,
			want: map[int]model.NoteAccessInfo{
				10: {CanRead: true, CanEdit: false},
				20: {CanRead: true, CanEdit: false},
			},
		},
		{
			name:    "empty ids",
			noteIDs: []int{},
			canEdit: true,
			canRead: true,
			want:    map[int]model.NoteAccessInfo{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := returnAllNotesWithFlag(tt.noteIDs, tt.canEdit, tt.canRead)

			assert.Equal(t, tt.want, got)
		})
	}
}
