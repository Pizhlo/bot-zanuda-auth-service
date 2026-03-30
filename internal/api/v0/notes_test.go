package v0

import (
	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"auth-service/internal/service"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:dupl // дублирование, т.к. схожи тест-кейсы для разных хендлеров
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

				return []notesHandlerOption{}
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

//nolint:funlen // длинный тест
func TestNotesHandler_FilterNotes(t *testing.T) {
	t.Parallel()

	type wantResponse struct {
		status int
		errMsg string
		resp   *model.FilterNotesResponse
	}

	spaceID := uuid.New()
	userID := uuid.New()

	note1 := uuid.New()
	note2 := uuid.New()

	noteIDs := []uuid.UUID{note1, note2}

	tests := []struct {
		name         string
		body         any
		userID       string
		prepareMocks func(m *mocks.MockPoliticsService)
		checkWant    func(actual wantResponse)
	}{
		{
			name: "positive case: notes filtered successfully",
			body: model.FilterNotesRequest{
				SpaceID: spaceID,
				NoteIDs: noteIDs,
			},
			userID: userID.String(),
			prepareMocks: func(m *mocks.MockPoliticsService) {
				t.Helper()

				m.EXPECT().
					FilterNotes(gomock.Any(), gomock.Any()).
					Return(map[uuid.UUID]model.NoteAccessInfo{
						note1: {CanRead: true, CanEdit: true},
						note2: {CanRead: true, CanEdit: false},
					}, nil)
			},
			checkWant: func(actual wantResponse) {
				assert.Equal(t, http.StatusOK, actual.status)
				assert.Equal(t, "", actual.errMsg)
				assert.Equal(t, &model.FilterNotesResponse{
					Notes: map[uuid.UUID]model.NoteAccessInfo{
						note1: {CanRead: true, CanEdit: true},
						note2: {CanRead: true, CanEdit: false},
					},
				}, actual.resp)
			},
		},
		{
			name: "bad request: empty note id list",
			body: model.FilterNotesRequest{
				SpaceID: spaceID,
				NoteIDs: []uuid.UUID{},
			},
			userID: userID.String(),
			prepareMocks: func(m *mocks.MockPoliticsService) {
				t.Helper()
			},
			checkWant: func(actual wantResponse) {
				assert.Equal(t, http.StatusBadRequest, actual.status)
				assert.Equal(t, "empty note id list", actual.errMsg)
			},
		},
		{
			name: "bad request: empty space id",
			body: model.FilterNotesRequest{
				SpaceID: uuid.UUID{},
				NoteIDs: noteIDs,
			},
			prepareMocks: func(m *mocks.MockPoliticsService) {
				t.Helper()
			},
			userID: userID.String(),
			checkWant: func(actual wantResponse) {
				assert.Equal(t, http.StatusBadRequest, actual.status)
				assert.Equal(t, "empty space id", actual.errMsg)
			},
		},
		{
			name: "unauthorized: no user in context",
			body: model.FilterNotesRequest{
				SpaceID: spaceID,
				NoteIDs: noteIDs,
			},
			userID: "",
			prepareMocks: func(m *mocks.MockPoliticsService) {
				t.Helper()
			},
			checkWant: func(actual wantResponse) {
				assert.Equal(t, http.StatusUnauthorized, actual.status)
				assert.Equal(t, "no user in context", actual.errMsg)
			},
		},
		{
			name: "forbidden: user is not member",
			body: model.FilterNotesRequest{
				SpaceID: spaceID,
				NoteIDs: noteIDs,
			},
			userID: userID.String(),
			prepareMocks: func(m *mocks.MockPoliticsService) {
				t.Helper()

				m.EXPECT().
					FilterNotes(gomock.Any(), gomock.Any()).
					Return(nil, service.ErrUserNotMember)
			},
			checkWant: func(actual wantResponse) {
				assert.Equal(t, http.StatusForbidden, actual.status)
				assert.Equal(t, service.ErrUserNotMember.Error(), actual.errMsg)
			},
		},
		{
			name: "internal error: service error",
			body: model.FilterNotesRequest{
				SpaceID: spaceID,
				NoteIDs: noteIDs,
			},
			userID: userID.String(),
			prepareMocks: func(m *mocks.MockPoliticsService) {
				t.Helper()

				m.EXPECT().
					FilterNotes(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("service error"))
			},
			checkWant: func(actual wantResponse) {
				assert.Equal(t, http.StatusInternalServerError, actual.status)
				assert.Equal(t, "internal server error", actual.errMsg)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			politicsService := mocks.NewMockPoliticsService(ctrl)

			tt.prepareMocks(politicsService)

			notesHandler, err := NewNotesHandler(
				WithPoliticsService(politicsService),
			)
			require.NoError(t, err)

			e := echo.New()

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/notes/filter", bytes.NewReader(bodyBytes))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

			ctx := withUserID(req.Context(), tt.userID)

			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err = notesHandler.FilterNotes(c)
			require.NoError(t, err)

			body := rec.Body.Bytes()

			// Пытаемся распарсить ошибку (если она есть).
			var errResp map[string]string

			_ = json.Unmarshal(body, &errResp)

			resp := &model.FilterNotesResponse{
				Notes: nil,
			}

			err = json.Unmarshal(body, &resp)
			require.NoError(t, err)

			actual := wantResponse{
				status: rec.Code,
				errMsg: errResp["error"],
				resp:   resp,
			}

			tt.checkWant(actual)
		})
	}
}

func TestUserIDFromContext(t *testing.T) {
	t.Parallel()

	ctx := withUserID(t.Context(), "123")
	id, ok := userIDFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "123", id)

	ctx = t.Context()
	id, ok = userIDFromContext(ctx)
	assert.False(t, ok)
	assert.Equal(t, "", id)
}

func TestParseUserID(t *testing.T) {
	t.Parallel()

	id := parseUserID("123")
	assert.Equal(t, "123", id)

	id = parseUserID(123)
	assert.Equal(t, "", id)
}

func TestWithUserID(t *testing.T) {
	t.Parallel()

	ctx := withUserID(t.Context(), "123")
	id, ok := userIDFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "123", id)
}
