package politics

import (
	"auth-service/internal/model"
	"auth-service/internal/service"
	serviceinternal "auth-service/internal/service/internal"
	"auth-service/internal/service/politics/mocks"
	"auth-service/pkg/audit"
	"auth-service/pkg/audit/testaudit"
	"context"
	"fmt"
	"reflect"
	"testing"
	"unsafe"

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
				auditor:                testaudit.NewAuditor(t),
			}

			res, err := svc.FilterNotes(t.Context(), tt.req)
			tt.wantErr(t, err)

			assert.Equal(t, tt.want, res)
		})
	}
}

func TestFilterNotesFailedHook(t *testing.T) {
	t.Parallel()

	t.Run("adds all fields from context", func(t *testing.T) {
		t.Parallel()

		spaceID := uuid.New()
		noteIDs := []uuid.UUID{uuid.New(), uuid.New()}

		ctx := context.Background()
		ctx = serviceinternal.WithServiceName(ctx)
		ctx = serviceinternal.WithMessage(ctx, "user is not member")
		ctx = serviceinternal.WithLevel(ctx, audit.ErrLevelWarn)
		ctx = serviceinternal.WithErrorCode(ctx, audit.ErrCodePermDeniedSpace)
		ctx = serviceinternal.WithMessageCtx(ctx, audit.EventContext{"space_id": "space-1"})
		ctx = serviceinternal.WithUserID(ctx, "user-1")
		ctx = serviceinternal.WithOperation(ctx, "politics.filter_notes")
		ctx = serviceinternal.WithKind(ctx, audit.KindDomain)
		ctx = withSpaceID(ctx, spaceID)
		ctx = withNoteIDs(ctx, noteIDs)

		stash := FilterNotesFailedHook(ctx, audit.Stash{})
		values := stashFieldsByName(stash)

		require.Equal(t, "auth-service", values["service_name"])
		require.Equal(t, "user is not member", values["message"])
		require.Equal(t, audit.ErrLevelWarn, values["level"])
		require.Equal(t, audit.ErrCodePermDeniedSpace, values["error_code"])
		require.Equal(t, "user-1", values["user_id"])
		require.Equal(t, "politics.filter_notes", values["operation"])
		require.Equal(t, audit.KindDomain, values["kind"])
		require.Equal(t, audit.EventContext{"space_id": spaceID.String(), "note_ids": noteIDs}, values["context"])
	})

	t.Run("returns unchanged stash when context is empty", func(t *testing.T) {
		t.Parallel()

		stash := FilterNotesFailedHook(context.Background(), audit.Stash{})
		values := stashFieldsByName(stash)

		require.Empty(t, values)
	})
}

func stashFieldsByName(stash audit.Stash) map[string]any {
	v := reflect.ValueOf(&stash).Elem()

	fields := v.FieldByName("fields")
	if !fields.IsValid() || fields.IsNil() {
		return map[string]any{}
	}

	fields = reflect.NewAt(fields.Type(), unsafe.Pointer(fields.UnsafeAddr())).Elem()

	result := make(map[string]any, fields.Len())

	iter := fields.MapRange()
	for iter.Next() {
		key := iter.Key().Interface()
		keyName := fmt.Sprint(key)

		fieldIface := iter.Value().Interface()

		valueField := reflect.ValueOf(fieldIface).FieldByName("Value")
		if valueField.IsValid() && valueField.CanInterface() {
			result[keyName] = valueField.Interface()
		}
	}

	return result
}
