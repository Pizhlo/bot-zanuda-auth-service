package politics

import (
	"auth-service/internal/model"
	"auth-service/internal/service"
	"auth-service/internal/service/politics/mocks"
	"auth-service/pkg/audit"
	"auth-service/pkg/audit/testaudit"
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
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

	filtered := []uuid.UUID{note2}
	member := model.SpaceMember{UserID: userID, IsMember: true}
	req := model.FilterNotesRequest{UserID: userID, SpaceID: spaceID, NoteIDs: noteIDs}
	failure := errors.New("dependency unavailable")

	audited := func(t *testing.T) context.Context {
		t.Helper()
		ctx, _, err := audit.Begin(t.Context(), testaudit.NewAuditor(t), audit.OperationFilterNotes, audit.StageReceiveRequest.String())
		require.NoError(t, err)

		return ctx
	}

	tests := []struct {
		name         string
		req          model.FilterNotesRequest
		ctx          func(*testing.T) context.Context
		prepareMocks func(*filterNotesMocks)
		want         map[uuid.UUID]model.NoteAccessInfo
		wantErr      error
	}{
		{
			name: "audit must be open before fetching access", ctx: (*testing.T).Context, req: req,
			prepareMocks: func(*filterNotesMocks) {},
			wantErr:      audit.ErrMissingAttempt,
		},
		{
			name: "only existing notes are resolved", ctx: audited, req: req,
			prepareMocks: func(m *filterNotesMocks) {
				gomock.InOrder(
					m.spaceChecker.EXPECT().CheckSpaceAccess(gomock.Any(), userID, spaceID).Return(member, nil),
					m.storage.EXPECT().FilterNoteIDs(gomock.Any(), spaceID, noteIDs).Return(filtered, nil),
					m.noteResolver.EXPECT().ResolveNotePermissions(gomock.Any(), member, filtered).Return(map[uuid.UUID]model.NoteAccessInfo{note2: {CanRead: true}}, nil),
				)
			},
			want: map[uuid.UUID]model.NoteAccessInfo{note2: {CanRead: true}},
		},
		{
			name: "not a member", ctx: audited, req: req,
			prepareMocks: func(m *filterNotesMocks) {
				m.spaceChecker.EXPECT().CheckSpaceAccess(gomock.Any(), userID, spaceID).Return(model.SpaceMember{}, nil)
			},
			want: map[uuid.UUID]model.NoteAccessInfo{}, wantErr: service.ErrUserNotMember,
		},
		{
			name: "space access failure", ctx: audited, req: req,
			prepareMocks: func(m *filterNotesMocks) {
				m.spaceChecker.EXPECT().CheckSpaceAccess(gomock.Any(), userID, spaceID).Return(model.SpaceMember{}, failure)
			},
			wantErr: failure,
		},
		{
			name: "filter failure", ctx: audited, req: req,
			prepareMocks: func(m *filterNotesMocks) {
				gomock.InOrder(
					m.spaceChecker.EXPECT().CheckSpaceAccess(gomock.Any(), userID, spaceID).Return(member, nil),
					m.storage.EXPECT().FilterNoteIDs(gomock.Any(), spaceID, noteIDs).Return(nil, failure),
				)
			},
			wantErr: failure,
		},
		{
			name: "permission resolver failure", ctx: audited, req: req,
			prepareMocks: func(m *filterNotesMocks) {
				gomock.InOrder(
					m.spaceChecker.EXPECT().CheckSpaceAccess(gomock.Any(), userID, spaceID).Return(member, nil),
					m.storage.EXPECT().FilterNoteIDs(gomock.Any(), spaceID, noteIDs).Return(filtered, nil),
					m.noteResolver.EXPECT().ResolveNotePermissions(gomock.Any(), member, filtered).Return(nil, failure),
				)
			},
			wantErr: failure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			m := &filterNotesMocks{
				spaceChecker: mocks.NewMockspaceAccessChecker(ctrl),
				storage:      mocks.NewMockstorage(ctrl),
				noteResolver: mocks.NewMocknotePermissionResolver(ctrl),
			}
			tt.prepareMocks(m)

			svc := &Service{
				storage:                m.storage,
				spaceAccessChecker:     m.spaceChecker,
				notePermissionResolver: m.noteResolver,
			}

			ctx := tt.ctx(t)

			res, err := svc.FilterNotes(ctx, tt.req)
			require.ErrorIs(t, err, tt.wantErr)

			require.Equal(t, tt.want, res)
		})
	}
}
