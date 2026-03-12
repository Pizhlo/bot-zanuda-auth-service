package politics

import (
	"auth-service/internal/model"
	"auth-service/internal/service"
	"auth-service/internal/service/politics/mocks"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
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

	note1 := uuid.New()
	note2 := uuid.New()
	note3 := uuid.New()

	userID := uuid.New()
	spaceID := uuid.New()

	noteIDs := []uuid.UUID{note1, note2, note3}

	tests := []struct {
		name         string
		req          model.FilterNotesRequest
		prepareMocks func(ctrl *gomock.Controller) *filterNotesMocks
		want         map[uuid.UUID]model.NoteAccessInfo
		wantErr      require.ErrorAssertionFunc
	}{
		{
			name: "positive case: user is member",
			req: model.FilterNotesRequest{
				UserID:  userID,
				SpaceID: spaceID,
				NoteIDs: noteIDs,
			},
			prepareMocks: func(ctrl *gomock.Controller) *filterNotesMocks {
				t.Helper()

				m := &filterNotesMocks{
					storage:      mocks.NewMockstorage(ctrl),
					spaceChecker: mocks.NewMockspaceAccessChecker(ctrl),
					noteResolver: mocks.NewMocknotePermissionResolver(ctrl),
				}

				spaceAccess := model.SpaceMember{
					UserID:   userID,
					IsMember: true,
				}

				m.spaceChecker.EXPECT().
					CheckSpaceAccess(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(spaceAccess, nil)

				m.storage.EXPECT().
					FilterNoteIDs(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(noteIDs, nil)

				m.noteResolver.EXPECT().
					ResolveNotePermissions(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[uuid.UUID]model.NoteAccessInfo{
						note1: {CanRead: true, CanEdit: true},
						note2: {CanRead: true, CanEdit: true},
						note3: {CanRead: true, CanEdit: true},
					}, nil)

				return m
			},
			want: map[uuid.UUID]model.NoteAccessInfo{
				note1: {CanRead: true, CanEdit: true},
				note2: {CanRead: true, CanEdit: true},
				note3: {CanRead: true, CanEdit: true},
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: user is not member",
			req: model.FilterNotesRequest{
				UserID:  userID,
				SpaceID: spaceID,
				NoteIDs: noteIDs,
			},
			prepareMocks: func(ctrl *gomock.Controller) *filterNotesMocks {
				t.Helper()

				m := &filterNotesMocks{
					storage:      mocks.NewMockstorage(ctrl),
					spaceChecker: mocks.NewMockspaceAccessChecker(ctrl),
					noteResolver: mocks.NewMocknotePermissionResolver(ctrl),
				}

				m.spaceChecker.EXPECT().
					CheckSpaceAccess(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(model.SpaceMember{
						UserID:   userID,
						IsMember: false,
					}, nil)

				// storage.FilterNoteIDs and noteResolver.ResolveNotePermissions
				// should not be called when user is not a member.

				return m
			},
			want: map[uuid.UUID]model.NoteAccessInfo{},
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
