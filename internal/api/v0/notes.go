package v0

import (
	"auth-service/internal/model"
	"auth-service/internal/service"
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

// NotesHandler хендлер для работы с заметками.
type NotesHandler struct {
	politicsService PoliticsService
}

// PoliticsService - интерфейс для доступа к сервису политик. Отвечает за доступ пользователей к данным.
//
//go:generate mockgen -source=notes.go -destination=mocks/politics_svc_mock.go -package=mocks PoliticsService
type PoliticsService interface {
	// FilterNotes фильтрует входящие заметки согласно политикам.
	// Возвращает только заметки, доступные пользователю, с флагом canEdit -
	// может ли пользователь редактировать заметку.
	FilterNotes(ctx context.Context, req model.FilterNotesRequest) (map[uuid.UUID]model.NoteAccessInfo, error)
}

type notesHandlerOption func(*NotesHandler)

// WithPoliticsService устанавливает сервис политик.
func WithPoliticsService(svc PoliticsService) notesHandlerOption {
	return func(h *NotesHandler) {
		h.politicsService = svc
	}
}

// NewNotesHandler создает новый хендлер для работы с заметками.
func NewNotesHandler(opts ...notesHandlerOption) (*NotesHandler, error) {
	h := &NotesHandler{}

	for _, opt := range opts {
		opt(h)
	}

	if h.politicsService == nil {
		return nil, errors.New("politics service is required")
	}

	logrus.Info("created notes handler")

	return h, nil
}

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
func (s *NotesHandler) FilterNotes(c echo.Context) error {
	var req model.FilterNotesRequest

	if err := c.Bind(&req); err != nil {
		logrus.WithError(err).Error("error binding request")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cannot bind request"})
	}

	if len(req.NoteIDs) == 0 {
		logrus.Debug("empty note id list")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "empty note id list"})
	}

	if req.SpaceID == uuid.Nil {
		logrus.Debug("invalid space id")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "empty space id"})
	}

	userID, ok := userIDFromContext(c.Request().Context())
	if !ok || userID == "" {
		logrus.Debug("no user in context")
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "no user in context"})
	}

	userIDUUID, err := uuid.Parse(userID)
	if err != nil {
		logrus.WithError(err).Error("error parsing user id")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	req.UserID = userIDUUID

	notes, err := s.politicsService.FilterNotes(c.Request().Context(), req)
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

func userIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(withUserIDCtxKey{})
	id, ok := v.(string)

	return id, ok
}
