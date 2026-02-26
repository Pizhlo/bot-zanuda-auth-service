package permissions

import (
	"auth-service/internal/model"
	"auth-service/internal/service/politics/permissions/mocks"
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // это тест
func TestNewNotePermissionResolver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		createOptions func(storage *mocks.Mockstorage) []option
		createWant    func(storage *mocks.Mockstorage) *NotePermissionResolver
		check         func(t *testing.T, want, actual *NotePermissionResolver)
		wantErr       require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			createOptions: func(storage *mocks.Mockstorage) []option {
				t.Helper()

				return []option{
					WithStorage(storage),
				}
			},
			createWant: func(storage *mocks.Mockstorage) *NotePermissionResolver {
				t.Helper()

				return &NotePermissionResolver{
					storage: storage,
				}
			},
			check: func(t *testing.T, want, actual *NotePermissionResolver) {
				t.Helper()

				assert.Equal(t, want, actual)
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: storage is required",
			createOptions: func(storage *mocks.Mockstorage) []option {
				t.Helper()

				return []option{}
			},
			createWant: func(storage *mocks.Mockstorage) *NotePermissionResolver {
				t.Helper()

				return nil
			},
			check: func(t *testing.T, want, actual *NotePermissionResolver) {
				t.Helper()

				assert.Nil(t, actual)
			},
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				t.Helper()

				require.ErrorContains(tt, err, "storage is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			storage := mocks.NewMockstorage(ctrl)

			opts := tt.createOptions(storage)

			actual, err := NewNotePermissionResolver(opts...)
			tt.wantErr(t, err)

			tt.check(t, tt.createWant(storage), actual)
		})
	}
}

//nolint:funlen // длинный тест
func TestNotePermissionResolver_ResolveNotePermissions(t *testing.T) {
	t.Parallel()

	type args struct {
		access  model.SpaceMember
		noteIDs []int
	}

	tests := []struct {
		name       string
		args       args
		want       map[int]model.NoteAccessInfo
		setupMocks func(ctrl *gomock.Controller) *mocks.Mockstorage
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case: full access (user is owner)",
			args: args{
				access: model.SpaceMember{
					RoleCode: model.OwnerRoleCode,
				},
				noteIDs: []int{1, 2, 3},
			},
			setupMocks: mocks.NewMockstorage,
			want: map[int]model.NoteAccessInfo{
				1: {CanRead: true, CanEdit: true},
				2: {CanRead: true, CanEdit: true},
				3: {CanRead: true, CanEdit: true},
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: non owner, full access",
			args: args{
				access: model.SpaceMember{
					RoleCode: model.EditorRoleCode,
				},
				noteIDs: []int{1, 2, 3},
			},
			setupMocks: func(ctrl *gomock.Controller) *mocks.Mockstorage {
				storage := mocks.NewMockstorage(ctrl)

				storage.EXPECT().GetNotesVisibility(gomock.Any(), gomock.Any()).Return(
					[]model.NoteVisibility{
						{
							ID:         1,
							Visibility: model.VisibilityTypeSpace,
						},
						{
							ID:         2,
							Visibility: model.VisibilityTypeSpace,
						},
						{
							ID:         3,
							Visibility: model.VisibilityTypeSpace,
						},
					}, nil,
				)

				return storage
			},
			want: map[int]model.NoteAccessInfo{
				1: {
					CanRead: true,
					CanEdit: true,
				},
				2: {
					CanRead: true,
					CanEdit: true,
				},
				3: {
					CanRead: true,
					CanEdit: true,
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: editor without access",
			args: args{
				access: model.SpaceMember{
					RoleCode: model.EditorRoleCode,
				},
				noteIDs: []int{1, 2, 3},
			},
			setupMocks: func(ctrl *gomock.Controller) *mocks.Mockstorage {
				storage := mocks.NewMockstorage(ctrl)

				storage.EXPECT().GetNotesVisibility(gomock.Any(), gomock.Any()).Return(
					[]model.NoteVisibility{
						{
							ID:         1,
							Visibility: model.VisibilityTypePrivateToAuthor,
						},
						{
							ID:         2,
							Visibility: model.VisibilityTypePrivateToAuthor,
						},
						{
							ID:         3,
							Visibility: model.VisibilityTypePrivateToAuthor,
						},
					}, nil,
				)

				return storage
			},
			want:    map[int]model.NoteAccessInfo{},
			wantErr: require.NoError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			resolver, err := NewNotePermissionResolver(WithStorage(tt.setupMocks(ctrl)))
			require.NoError(t, err)

			got, err := resolver.ResolveNotePermissions(context.Background(), tt.args.access, tt.args.noteIDs)
			tt.wantErr(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGrantSameAccessToAllNotes(t *testing.T) {
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

			got := grantSameAccessToAllNotes(tt.noteIDs, tt.canRead, tt.canEdit)

			assert.Equal(t, tt.want, got)
		})
	}
}

//nolint:funlen // это тест
func TestDecideNoteAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		member     model.SpaceMember
		visibility model.NoteVisibility
		want       model.NoteAccessInfo
	}{
		{
			name: "positive case: role OWNER, visibility SPACE",
			member: model.SpaceMember{
				UserID:   1,
				IsMember: true,
				RoleCode: model.OwnerRoleCode,
			},
			visibility: model.NoteVisibility{
				ID:         1,
				Visibility: model.VisibilityTypeSpace,
			},
			want: model.NoteAccessInfo{
				CanRead: true,
				CanEdit: true,
			},
		},
		{
			name: "positive case: role ADMIN, visibility SPACE",
			member: model.SpaceMember{
				UserID:   1,
				IsMember: true,
				RoleCode: model.AdminRoleCode,
			},
			visibility: model.NoteVisibility{
				ID:         1,
				Visibility: model.VisibilityTypeSpace,
			},
			want: model.NoteAccessInfo{
				CanRead: true,
				CanEdit: true,
			},
		},
		{
			name: "positive case: role EDITOR, visibility SPACE",
			member: model.SpaceMember{
				UserID:   1,
				IsMember: true,
				RoleCode: model.EditorRoleCode,
			},
			visibility: model.NoteVisibility{
				ID:         1,
				Visibility: model.VisibilityTypeSpace,
			},
			want: model.NoteAccessInfo{
				CanRead: true,
				CanEdit: true,
			},
		},
		{
			name: "positive case: role VIEWER, visibility SPACE",
			member: model.SpaceMember{
				UserID:   1,
				IsMember: true,
				RoleCode: model.ViewerRoleCode,
			},
			visibility: model.NoteVisibility{
				ID:         1,
				Visibility: model.VisibilityTypeSpace,
			},
			want: model.NoteAccessInfo{
				CanRead: true,
				CanEdit: false,
			},
		},
		{
			name: "positive case: role EDITOR, visibility CUSTOM: denyAll",
			member: model.SpaceMember{
				UserID:   1,
				IsMember: true,
				RoleCode: model.EditorRoleCode,
			},
			visibility: model.NoteVisibility{
				ID:         1,
				Visibility: model.VisibilityTypeCustom,
			},
			want: model.NoteAccessInfo{
				CanRead: false,
				CanEdit: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			actual := decideNoteAccess(tt.member, tt.visibility)
			require.Equal(t, tt.want, actual)
		})
	}
}

//nolint:funlen // это тест
func TestAccessForSpaceVisibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		member model.SpaceMember
		want   model.NoteAccessInfo
	}{
		{
			name: "positive case: OWNER",
			member: model.SpaceMember{
				UserID:   1,
				IsMember: true,
				RoleCode: model.OwnerRoleCode,
			},
			want: model.NoteAccessInfo{
				CanRead: true,
				CanEdit: true,
			},
		},
		{
			name: "positive case: ADMIN",
			member: model.SpaceMember{
				UserID:   1,
				IsMember: true,
				RoleCode: model.AdminRoleCode,
			},
			want: model.NoteAccessInfo{
				CanRead: true,
				CanEdit: true,
			},
		},
		{
			name: "positive case: EDITOR",
			member: model.SpaceMember{
				UserID:   1,
				IsMember: true,
				RoleCode: model.EditorRoleCode,
			},
			want: model.NoteAccessInfo{
				CanRead: true,
				CanEdit: true,
			},
		},
		{
			name: "positive case: VIEWER",
			member: model.SpaceMember{
				UserID:   1,
				IsMember: true,
				RoleCode: model.ViewerRoleCode,
			},
			want: model.NoteAccessInfo{
				CanRead: true,
				CanEdit: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			actual := accessForSpaceVisibility(tt.member)
			require.Equal(t, tt.want, actual)
		})
	}
}
