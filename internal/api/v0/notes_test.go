package v0

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"auth-service/internal/service"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

//nolint:dupl,funlen // дублирование, т.к. схожи тест-кейсы для разных хендлеров; длинный тест - это ок
func TestNewNotesHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      func(t *testing.T, ctrl *gomock.Controller) []notesHandlerOption
		checkWant func(t *testing.T, notesHandler *NotesHandler)
		wantErr   require.ErrorAssertionFunc
	}{
		{
			name: "success",
			opts: func(t *testing.T, ctrl *gomock.Controller) []notesHandlerOption {
				t.Helper()

				return []notesHandlerOption{
					WithPoliticsService(mocks.NewMockPoliticsService(ctrl)),
					WithUserService(mocks.NewMockuserService(ctrl)),
				}
			},
			checkWant: func(t *testing.T, notesHandler *NotesHandler) {
				t.Helper()

				require.NotNil(t, notesHandler)
				require.NotNil(t, notesHandler.politicsService)
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: politics service is required",
			opts: func(t *testing.T, ctrl *gomock.Controller) []notesHandlerOption {
				t.Helper()

				return []notesHandlerOption{
					WithUserService(mocks.NewMockuserService(ctrl)),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "politics service is required")
			},
			checkWant: func(t *testing.T, notesHandler *NotesHandler) {
				t.Helper()

				require.Nil(t, notesHandler)
			},
		},
		{
			name: "error case: user service is required",
			opts: func(t *testing.T, ctrl *gomock.Controller) []notesHandlerOption {
				t.Helper()

				return []notesHandlerOption{
					WithPoliticsService(mocks.NewMockPoliticsService(ctrl)),
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "user service is required")
			},
			checkWant: func(t *testing.T, notesHandler *NotesHandler) {
				t.Helper()

				require.Nil(t, notesHandler)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			opts := tt.opts(t, ctrl)
			notesHandler, err := NewNotesHandler(opts...)
			tt.wantErr(t, err)

			tt.checkWant(t, notesHandler)
		})
	}
}

func TestFilterNotes(t *testing.T) {
	t.Parallel()

	space, user := uuid.New(), uuid.New()
	noteIDs := []uuid.UUID{uuid.New(), uuid.New()}
	request := model.FilterNotesRequest{SpaceID: space, NoteIDs: noteIDs}
	verified := model.FilterNotesRequest{SpaceID: space, NoteIDs: noteIDs, UserID: user}
	notes := map[uuid.UUID]model.NoteAccessInfo{
		noteIDs[0]: {CanRead: true, CanEdit: true},
		noteIDs[1]: {CanRead: true, CanEdit: false},
	}

	const telegramID = "1234567890"

	withAudit := (*MiddlewareHandler).AuditNotesFilter

	noMocks := func(*mocks.MockPoliticsService, *mocks.MockuserService) {}
	for _, tc := range []struct {
		name         string
		audit        func(*MiddlewareHandler, echo.HandlerFunc) echo.HandlerFunc
		body         any
		telegramID   string
		prepareMocks func(*mocks.MockPoliticsService, *mocks.MockuserService)
		status       int
		wantLog      string
		response     any
	}{
		{"missing audit", withoutRequestAudit, request, telegramID, noMocks, http.StatusInternalServerError, "", map[string]string{"error": audit.ErrMissingAttempt.Error()}},
		{"filtered notes", withAudit, request, telegramID, expectNotesFilter(telegramID, user, verified, notes, nil), http.StatusOK, "level=info", model.FilterNotesResponse{Notes: notes}},
		{"invalid JSON field type", withAudit, "invalid object", telegramID, noMocks, http.StatusBadRequest, "level=debug", map[string]string{"error": "cannot bind request"}},
		{"empty note IDs", withAudit, model.FilterNotesRequest{SpaceID: space}, telegramID, noMocks, http.StatusBadRequest, "level=debug", map[string]string{"error": "empty note id list"}},
		{"missing space ID", withAudit, model.FilterNotesRequest{NoteIDs: noteIDs}, telegramID, noMocks, http.StatusBadRequest, "level=debug", map[string]string{"error": "empty space id"}},
		{"missing Telegram identity", withAudit, request, "", noMocks, http.StatusUnauthorized, "level=warn", map[string]string{"error": "no user in context"}},
		{"unknown Telegram user", withAudit, request, telegramID, failedNotesLookup(telegramID, fmt.Errorf("lookup: %w", storage.ErrNotFound)), http.StatusUnauthorized, "level=error", map[string]string{"error": "user not found"}},
		{"identity store failure", withAudit, request, telegramID, failedNotesLookup(telegramID, errors.New("private database error")), http.StatusInternalServerError, "level=error", map[string]string{"error": "internal server error"}},
		{"not a member", withAudit, request, telegramID, expectNotesFilter(telegramID, user, verified, nil, service.ErrUserNotMember), http.StatusForbidden, "level=warn", map[string]string{"error": service.ErrUserNotMember.Error()}},
		{"filter failure", withAudit, request, telegramID, expectNotesFilter(telegramID, user, verified, nil, errors.New("private policy error")), http.StatusInternalServerError, "level=warn", map[string]string{"error": "internal server error"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			politics, users := mocks.NewMockPoliticsService(ctrl), mocks.NewMockuserService(ctrl)
			tc.prepareMocks(politics, users)
			h, err := NewNotesHandler(WithPoliticsService(politics), WithUserService(users))
			require.NoError(t, err)

			rec := httptest.NewRecorder()
			c := notesRequestContext(t, tc.telegramID, tc.body, rec)

			var output bytes.Buffer

			mw := &MiddlewareHandler{requestAuditor: audit.NewAuditor(audit.WithOutput(&output))}
			err = tc.audit(mw, h.FilterNotes)(c)
			require.NoError(t, err)
			requireNotesResponse(t, rec, tc.status, tc.response)
			require.Contains(t, output.String(), tc.wantLog)
			require.NotContains(t, output.String(), "private")
		})
	}
}

func requireNotesResponse(t *testing.T, rec *httptest.ResponseRecorder, status int, response any) {
	t.Helper()

	require.Equal(t, status, rec.Code)

	expected, err := json.Marshal(response)
	require.NoError(t, err)

	require.JSONEq(t, string(expected), rec.Body.String())
}

func expectNotesFilter(telegramID string, user uuid.UUID, verified model.FilterNotesRequest, result map[uuid.UUID]model.NoteAccessInfo, err error) func(*mocks.MockPoliticsService, *mocks.MockuserService) {
	return func(politics *mocks.MockPoliticsService, users *mocks.MockuserService) {
		gomock.InOrder(
			users.EXPECT().GetUserIDByTelegramID(gomock.Any(), telegramID).Return(user, nil),
			politics.EXPECT().FilterNotes(gomock.Any(), verified).Return(result, err),
		)
	}
}

func failedNotesLookup(telegramID string, err error) func(*mocks.MockPoliticsService, *mocks.MockuserService) {
	return func(politics *mocks.MockPoliticsService, users *mocks.MockuserService) {
		users.EXPECT().GetUserIDByTelegramID(gomock.Any(), telegramID).Return(uuid.Nil, err)
		politics.EXPECT().FilterNotes(gomock.Any(), gomock.Any()).Times(0)
	}
}

func notesRequestContext(t *testing.T, telegramID string, body any, rec *httptest.ResponseRecorder) echo.Context {
	t.Helper()

	data, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(withUserID(t.Context(), telegramID), http.MethodPost, "/api/v0/auth/notes/filter", bytes.NewReader(data))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	return echo.New().NewContext(req, rec)
}

func TestUserIDFromContext(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		ctx  context.Context
		want string
		ok   bool
	}{
		{"stored identity", withUserID(t.Context(), "123"), "123", true},
		{"stored empty string", withUserID(t.Context(), ""), "", true},
		{"missing identity", t.Context(), "", false},
		{"unexpected identity type", context.WithValue(t.Context(), userIDKey{}, 123), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			id, ok := userIDFromContext(tc.ctx)

			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, id)
		})
	}
}

func TestBindFilterNotes(t *testing.T) {
	t.Parallel()

	id, space := uuid.New(), uuid.New()

	wantError := func(message string) require.ErrorAssertionFunc {
		return func(t require.TestingT, err error, _ ...any) {
			require.ErrorContains(t, err, message)
		}
	}

	for _, tt := range []struct {
		name, body string
		want       model.FilterNotesRequest
		wantErr    require.ErrorAssertionFunc
	}{
		{"invalid JSON", "{", model.FilterNotesRequest{}, wantError("cannot bind request")},
		{"missing notes", "{}", model.FilterNotesRequest{}, wantError("empty note id list")},
		{"missing space", fmt.Sprintf(`{"note_ids":[%q]}`, id), model.FilterNotesRequest{NoteIDs: []uuid.UUID{id}}, wantError("empty space id")},
		{"valid", fmt.Sprintf(`{"note_ids":[%q],"space_id":%q}`, id, space), model.FilterNotesRequest{NoteIDs: []uuid.UUID{id}, SpaceID: space}, require.NoError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), "POST", "/", strings.NewReader(tt.body))
			r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

			req, err := bindFilterNotes(echo.New().NewContext(r, httptest.NewRecorder()))
			tt.wantErr(t, err)

			require.Equal(t, tt.want, req)
		})
	}
}
