package v0

import (
	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"auth-service/internal/service"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
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

	tests := []struct {
		name         string
		body         any
		userID       *int
		prepareMocks func(ps *mocks.MockPoliticsService)
		checkWant    func(actual wantResponse)
	}{
		{
			name: "positive case: notes filtered successfully",
			body: model.FilterNotesRequest{
				SpaceID: 1,
				NoteIDs: []int{1, 2, 3},
			},
			userID: func() *int { v := 42; return &v }(),
			prepareMocks: func(ps *mocks.MockPoliticsService) {
				ps.EXPECT().
					FilterNotes(gomock.Any(), model.FilterNotesRequest{
						UserID:  42,
						SpaceID: 1,
						NoteIDs: []int{1, 2, 3},
					}).
					Return(map[int]model.NoteAccessInfo{
						1: {CanRead: true, CanEdit: true},
						2: {CanRead: true, CanEdit: false},
					}, nil)
			},
			checkWant: func(actual wantResponse) {
				assert.Equal(t, http.StatusOK, actual.status)
				assert.Equal(t, "", actual.errMsg)
				assert.Equal(t, &model.FilterNotesResponse{
					Notes: map[int]model.NoteAccessInfo{
						1: {CanRead: true, CanEdit: true},
						2: {CanRead: true, CanEdit: false},
					},
				}, actual.resp)
			},
		},
		{
			name: "bad request: empty note id list",
			body: model.FilterNotesRequest{
				SpaceID: 1,
				NoteIDs: []int{},
			},
			userID: func() *int { v := 1; return &v }(),
			checkWant: func(actual wantResponse) {
				assert.Equal(t, http.StatusBadRequest, actual.status)
				assert.Equal(t, "empty note id list", actual.errMsg)
			},
		},
		{
			name: "bad request: empty space id",
			body: model.FilterNotesRequest{
				SpaceID: -11,
				NoteIDs: []int{1, 2, 3},
			},
			userID: func() *int { v := 1; return &v }(),
			checkWant: func(actual wantResponse) {
				assert.Equal(t, http.StatusBadRequest, actual.status)
				assert.Equal(t, "empty space id", actual.errMsg)
			},
		},
		{
			name: "unauthorized: no user in context",
			body: model.FilterNotesRequest{
				SpaceID: 1,
				NoteIDs: []int{1, 2, 3},
			},
			userID: nil,
			checkWant: func(actual wantResponse) {
				assert.Equal(t, http.StatusUnauthorized, actual.status)
				assert.Equal(t, "no user in context", actual.errMsg)
			},
		},
		{
			name: "forbidden: user is not member",
			body: model.FilterNotesRequest{
				SpaceID: 1,
				NoteIDs: []int{1, 2, 3},
			},
			userID: func() *int { v := 1; return &v }(),
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
				SpaceID: 1,
				NoteIDs: []int{1, 2, 3},
			},
			userID: func() *int { v := 1; return &v }(),
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

			req := httptest.NewRequest(http.MethodPost, "/auth/notes/filter", bytes.NewReader(bodyBytes))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

			ctx := req.Context()
			if tt.userID != nil {
				ctx = withUserID(ctx, *tt.userID)
			}

			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err = h.FilterNotes(c)
			require.NoError(t, err)

			body := rec.Body.Bytes()

			// Пытаемся распарсить ошибку (если она есть).
			var errResp map[string]string

			_ = json.Unmarshal(body, &errResp)

			// Пытаемся распарсить успешный ответ с заметками.
			type rawResponse struct {
				Notes map[string]model.NoteAccessInfo `json:"notes"`
			}

			var raw rawResponse

			_ = json.Unmarshal(body, &raw)

			resp := &model.FilterNotesResponse{
				Notes: nil,
			}

			if len(raw.Notes) > 0 {
				resp.Notes = make(map[int]model.NoteAccessInfo, len(raw.Notes))
				for k, v := range raw.Notes {
					// ключи приходят строками, конвертируем в int
					var id int

					_, convErr := fmt.Sscanf(k, "%d", &id)
					require.NoError(t, convErr)

					resp.Notes[id] = v
				}
			}

			actual := wantResponse{
				status: rec.Code,
				errMsg: errResp["error"],
				resp:   resp,
			}

			tt.checkWant(actual)
		})
	}
}
