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

//nolint:funlen // длинный тест
func TestHandler_FilterNotes(t *testing.T) {
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
		prepareMocks func(ps *mocks.MockPoliticsService)
		checkWant    func(actual wantResponse)
	}{
		{
			name: "positive case: notes filtered successfully",
			body: model.FilterNotesRequest{
				SpaceID: spaceID,
				NoteIDs: noteIDs,
			},
			userID: userID.String(),
			prepareMocks: func(ps *mocks.MockPoliticsService) {
				ps.EXPECT().
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
			prepareMocks: func(ps *mocks.MockPoliticsService) {
				ps.EXPECT().
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
			prepareMocks: func(ps *mocks.MockPoliticsService) {
				ps.EXPECT().
					FilterNotes(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("db error"))
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

			politicsSvc := mocks.NewMockPoliticsService(ctrl)

			if tt.prepareMocks != nil {
				tt.prepareMocks(politicsSvc)
			}

			h, err := New(
				WithVersion("1.0.0"),
				WithBuildDate("2021-01-01"),
				WithGitCommit("1234567890"),
				WithPoliticsService(politicsSvc),
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

			err = h.FilterNotes(c)
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
