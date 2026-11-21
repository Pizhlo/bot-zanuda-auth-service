package v0

import (
	"auth-service/internal/model"
	"auth-service/internal/service"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

const stageFilterNotes = "filter_notes"

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
//	@Router		/api/v0/auth/notes/filter [post]
func (s *NotesHandler) FilterNotes(c echo.Context) error {
	ctx := c.Request().Context()

	attempt := audit.Current(ctx, audit.OperationFilterNotes)
	if attempt == nil {
		return errResponse(c, http.StatusInternalServerError, audit.ErrMissingAttempt)
	}

	attempt.At(audit.StageValidateRequest.String())

	req, err := bindFilterNotes(c)
	if err != nil {
		attempt.Fail(audit.ErrCodeValidationFailed, audit.KindValidation, audit.ErrLevelDebug, err.Error(), err)
		return errResponse(c, http.StatusBadRequest, err)
	}

	attempt.At(audit.StageVerifySubject.String())

	userID, ok := userIDFromContext(ctx)
	if !ok || userID == "" {
		attempt.Fail(audit.ErrCodeValidationFailed, audit.KindValidation, audit.ErrLevelWarn, "no user in context", nil)
		return errResponse(c, http.StatusUnauthorized, errors.New("no user in context"))
	}

	userIDUUID, err := s.userSvc.GetUserIDByTelegramID(ctx, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			attempt.Fail(audit.ErrCodeUserNotFound, audit.KindDomain, audit.ErrLevelError, "user not found", storage.ErrNotFound)
			return errResponse(c, http.StatusUnauthorized, errors.New("user not found"))
		}

		logrus.WithError(err).Error("error getting user id by telegram id")

		attempt.Fail(audit.ErrCodeServiceUnavailable, audit.KindInfra, audit.ErrLevelError, "internal server error", nil)

		return errResponse(c, http.StatusInternalServerError, errors.New("internal server error"))
	}

	req.UserID = userIDUUID

	attempt.Context(audit.EventContext{audit.KeyUserID: userIDUUID.String(), audit.KeySpaceID: req.SpaceID.String(), audit.KeyNoteIDs: req.NoteIDs})
	attempt.At(stageFilterNotes)

	notes, err := s.politicsService.FilterNotes(ctx, req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotMember) {
			attempt.Fail(audit.ErrCodePermDeniedSpace, audit.KindDomain, audit.ErrLevelWarn, "User is not a member of the space", service.ErrUserNotMember)
			return errResponse(c, http.StatusForbidden, err)
		}

		logrus.WithError(err).Error("error filtering notes")

		attempt.Fail(audit.ErrCodeServiceUnavailable, audit.KindInfra, audit.ErrLevelWarn, "internal server error", nil)

		return errResponse(c, http.StatusInternalServerError, errors.New("internal server error"))
	}

	attempt.At(audit.StageWriteResponse.String())

	return c.JSON(http.StatusOK, model.FilterNotesResponse{Notes: notes})
}

func bindFilterNotes(c echo.Context) (model.FilterNotesRequest, error) {
	var req model.FilterNotesRequest
	if err := c.Bind(&req); err != nil {
		logrus.WithError(err).Error("error binding request")
		return req, errors.New("cannot bind request")
	}

	if len(req.NoteIDs) == 0 {
		return req, errors.New("empty note id list")
	}

	if req.SpaceID == uuid.Nil {
		return req, errors.New("empty space id")
	}

	return req, nil
}

func userIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(userIDKey{})
	id, ok := v.(string)

	return id, ok
}
