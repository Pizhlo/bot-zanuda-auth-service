package politics

import (
	"auth-service/internal/model"
	"auth-service/internal/service"
	"auth-service/internal/service/politics/mocks"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type filterNotesMocks struct {
	storage      *mocks.Mockstorage
	spaceChecker *mocks.MockspaceAccessChecker
	noteResolver *mocks.MocknotePermissionResolver
}

//nolint:funlen // длинный тест
func TestFilterNotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		req          model.FilterNotesRequest
		prepareMocks func(ctrl *gomock.Controller) *filterNotesMocks
		want         map[int]model.NoteAccessInfo
		wantErr      require.ErrorAssertionFunc
	}{
		{
			name: "positive case: user is member",
			req: model.FilterNotesRequest{
				UserID:  1,
				SpaceID: 1,
				NoteIDs: []int{1, 2, 3},
			},
			prepareMocks: func(ctrl *gomock.Controller) *filterNotesMocks {
				t.Helper()

				m := &filterNotesMocks{
					storage:      mocks.NewMockstorage(ctrl),
					spaceChecker: mocks.NewMockspaceAccessChecker(ctrl),
					noteResolver: mocks.NewMocknotePermissionResolver(ctrl),
				}

				spaceAccess := model.SpaceMember{
					UserID:   1,
					IsMember: true,
				}

				m.spaceChecker.EXPECT().
					CheckSpaceAccess(gomock.Any(), 1, 1).
					Return(spaceAccess, nil)

				m.storage.EXPECT().
					FilterNoteIDs(gomock.Any(), 1, []int{1, 2, 3}).
					Return([]int{1, 2, 3}, nil)

				m.noteResolver.EXPECT().
					ResolveNotePermissions(gomock.Any(), spaceAccess, []int{1, 2, 3}).
					Return(map[int]model.NoteAccessInfo{
						1: {CanRead: true, CanEdit: true},
						2: {CanRead: true, CanEdit: true},
						3: {CanRead: true, CanEdit: true},
					}, nil)

				return m
			},
			want: map[int]model.NoteAccessInfo{
				1: {CanRead: true, CanEdit: true},
				2: {CanRead: true, CanEdit: true},
				3: {CanRead: true, CanEdit: true},
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: user is not member",
			req: model.FilterNotesRequest{
				UserID:  1,
				SpaceID: 1,
				NoteIDs: []int{1, 2, 3},
			},
			prepareMocks: func(ctrl *gomock.Controller) *filterNotesMocks {
				t.Helper()

				m := &filterNotesMocks{
					storage:      mocks.NewMockstorage(ctrl),
					spaceChecker: mocks.NewMockspaceAccessChecker(ctrl),
					noteResolver: mocks.NewMocknotePermissionResolver(ctrl),
				}

				m.spaceChecker.EXPECT().
					CheckSpaceAccess(gomock.Any(), 1, 1).
					Return(model.SpaceMember{
						UserID:   1,
						IsMember: false,
					}, nil)

				// storage.FilterNoteIDs and noteResolver.ResolveNotePermissions
				// should not be called when user is not a member.

				return m
			},
			want: map[int]model.NoteAccessInfo{},
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(tt, err, service.ErrUserNotMember.Error())
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			m := tt.prepareMocks(ctrl)

			svc := &Service{
				storage:                m.storage,
				spaceAccessChecker:     m.spaceChecker,
				notePermissionResolver: m.noteResolver,
			}

			res, err := svc.FilterNotes(t.Context(), tt.req)
			tt.wantErr(t, err)

			assert.Equal(t, tt.want, res)
		})
	}
}
