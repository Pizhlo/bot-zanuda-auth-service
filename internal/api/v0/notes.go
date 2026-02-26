package v0

import (
	"auth-service/internal/model"
	"auth-service/internal/service"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

// FilterNotes фильтрует входящий список айди заметок, возвращая только те, которые доступны пользователю (в соответствии с политиками).
//
// FilterNotes godoc
//
//	@Summary		Фильтрация записей, доступных пользователю
//	@Description	Фильтрует входящий список айди заметок, возвращая только те, которые доступны пользователю (в соответствии с политиками).
//	@Tags			notes
//	@Accept			json
//	@Produce		json
//	@Param			request	body		model.FilterNotesRequest	true	"Запрос на фильтрацию заметок"
//	@Success		200		{object}	model.FilterNotesResponse
//	@Failure		400		{object}	map[string]string	"Некорректный запрос"
//	@Failure		401		{object}	map[string]string	"Пользователь не авторизован"
//	@Failure		403		{object}	map[string]string	"Нет доступа к запрошенным заметкам"
//	@Failure		500		{object}	map[string]string	"Внутренняя ошибка сервера"
//	@Security	BearerAuth
//	@Router		/auth/notes/filter [post]
func (s *Handler) FilterNotes(c echo.Context) error {
	var req model.FilterNotesRequest

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		logrus.WithError(err).Error("error reading body")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cannot read body"})
	}

	err = json.Unmarshal(body, &req)
	if err != nil {
		logrus.WithError(err).Error("error unmarshaling body")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cannot unmarshal body"})
	}

	if len(req.NoteIDs) == 0 {
		logrus.Debug("empty note id list")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "empty note id list"})
	}

	if req.SpaceID < 1 {
		logrus.Debug("invalid space id")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "empty space id"})
	}

	userID, ok := userIDFromContext(c.Request().Context())
	if !ok {
		logrus.Debug("no user in context")
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "no user in context"})
	}

	req.UserID = userID

	notes, err := s.PoliticsService.FilterNotes(c.Request().Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotMember) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": service.ErrUserNotMember.Error()})
		}

		logrus.WithError(err).Error("error filtering notes")

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	resp := model.FilterNotesResponse{
		Notes: notes,
	}

	return c.JSON(http.StatusOK, resp)
}

func userIDFromContext(ctx context.Context) (int, bool) {
	v := ctx.Value(withUserIDCtxKey{})
	id, ok := v.(int)

	return id, ok
}
