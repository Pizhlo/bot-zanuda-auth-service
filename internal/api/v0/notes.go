package v0

import (
	"auth-service/internal/model"
	"auth-service/internal/service"
	"auth-service/internal/storage"
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
	userSvc         userService
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

type userService interface {
	GetUserIDByTelegramID(ctx context.Context, telegramID string) (uuid.UUID, error)
}

type notesHandlerOption func(*NotesHandler)

// WithPoliticsService устанавливает сервис политик.
func WithPoliticsService(svc PoliticsService) notesHandlerOption {
	return func(h *NotesHandler) {
		h.politicsService = svc
	}
}

// WithUserService устанавливает сервис пользователей.
func WithUserService(svc userService) notesHandlerOption {
	return func(h *NotesHandler) {
		h.userSvc = svc
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

	if h.userSvc == nil {
		return nil, errors.New("user service is required")
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
		return errResponse(c, http.StatusBadRequest, errors.New("cannot bind request"))
	}

	if len(req.NoteIDs) == 0 {
		return errResponse(c, http.StatusBadRequest, errors.New("empty note id list"))
	}

	if req.SpaceID == uuid.Nil {
		return errResponse(c, http.StatusBadRequest, errors.New("empty space id"))
	}

	userID, ok := userIDFromContext(c.Request().Context())
	if !ok || userID == "" {
		return errResponse(c, http.StatusUnauthorized, errors.New("no user in context"))
	}

	userIDUUID, err := s.userSvc.GetUserIDByTelegramID(c.Request().Context(), userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return errResponse(c, http.StatusUnauthorized, errors.New("user not found"))
		}

		logrus.WithError(err).Error("error getting user id by telegram id")

		return errResponse(c, http.StatusInternalServerError, errors.New("internal server error"))
	}

	req.UserID = userIDUUID

	notes, err := s.politicsService.FilterNotes(c.Request().Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotMember) {
			return errResponse(c, http.StatusForbidden, err)
		}

		logrus.WithError(err).Error("error filtering notes")

		return errResponse(c, http.StatusInternalServerError, errors.New("internal server error"))
	}

	resp := model.FilterNotesResponse{
		Notes: notes,
	}

	return c.JSON(http.StatusOK, resp)
}

func userIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(userIDKey{})
	id, ok := v.(string)

	return id, ok
}
