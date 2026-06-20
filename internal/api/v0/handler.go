package v0

import (
	"errors"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

// Handler - хендлер версии 0.
type Handler struct {
	version   string
	buildDate string
	gitCommit string

	apiVersion string

	// handlers
	auth      authProcessorHandler
	notes     notesProcessorHandler
	resources resourcesProcessorHandler
}

//go:generate mockgen -source=handler.go -destination=mocks/handler_mocks.go -package=mocks authProcessorHandler
type authProcessorHandler interface {
	// Login проверяет корректность полученных данных и отправляет в ответ JWT-токен.
	Login(c echo.Context) error
}

//go:generate mockgen -source=handler.go -destination=mocks/handler_mocks.go -package=mocks notesProcessorHandler
type notesProcessorHandler interface {
	// FilterNotes фильтрует входящие заметки, возвращая только те, которые доступны пользователю
	// согласно политикам.
	FilterNotes(c echo.Context) error
}

//go:generate mockgen -source=handler.go -destination=mocks/handler_mocks.go -package=mocks resourcesProcessorHandler
type resourcesProcessorHandler interface {
	// UpdateResource обновляет ресурс.
	UpdateResource(c echo.Context) error
}

type handlerOption func(*Handler)

// WithVersion устанавливает version.
func WithVersion(version string) handlerOption {
	return func(h *Handler) {
		h.version = version
	}
}

// WithBuildDate устанавливает build date.
func WithBuildDate(buildDate string) handlerOption {
	return func(h *Handler) {
		h.buildDate = buildDate
	}
}

// WithGitCommit устанавливает git commit.
func WithGitCommit(gitCommit string) handlerOption {
	return func(h *Handler) {
		h.gitCommit = gitCommit
	}
}

// WithAuthHandler устанавливает хендлер авторизации.
func WithAuthHandler(auth authProcessorHandler) handlerOption {
	return func(h *Handler) {
		h.auth = auth
	}
}

// WithNotesHandler устанавливает хендлер работы с заметками.
func WithNotesHandler(notes notesProcessorHandler) handlerOption {
	return func(h *Handler) {
		h.notes = notes
	}
}

// WithResourcesHandler устанавливает хендлер работы с ресурсами.
func WithResourcesHandler(resources resourcesProcessorHandler) handlerOption {
	return func(h *Handler) {
		h.resources = resources
	}
}

// NewHandler создает новый хендлер. Автоматически устанавливает версию хендлера на Version0.
func NewHandler(opts ...handlerOption) (*Handler, error) {
	h := &Handler{}

	for _, opt := range opts {
		opt(h)
	}

	if h.version == "" {
		return nil, errors.New("version is required")
	}

	if h.buildDate == "" {
		return nil, errors.New("buildDate is required")
	}

	if h.gitCommit == "" {
		return nil, errors.New("gitCommit is required")
	}

	if h.notes == nil {
		return nil, errors.New("notes handler is required")
	}

	if h.auth == nil {
		return nil, errors.New("auth handler is required")
	}

	if h.resources == nil {
		return nil, errors.New("resources handler is required")
	}

	h.apiVersion = Version0

	logrus.WithFields(logrus.Fields{
		"version":    h.version,
		"buildDate":  h.buildDate,
		"gitCommit":  h.gitCommit,
		"apiVersion": h.apiVersion,
	}).Info("created handler")

	return h, nil
}

const (
	// Version0 - константа версии апи хендлера. Версия: 0.
	Version0 = "v0"
)

// Version возвращает версию апи хендлера, чтобы нельзя было использовать хендлер не той версии.
// Нужен для соответствия интерфейсу server.versionHandler.
func (h *Handler) Version() string {
	return h.apiVersion
}

// Login проверяет корректность полученных данных и отправляет в ответ JWT-токен.
func (h *Handler) Login(c echo.Context) error {
	return h.auth.Login(c)
}

// FilterNotes фильтрует входящие заметки, возвращая только те, которые доступны пользователю
// согласно политикам.
func (h *Handler) FilterNotes(c echo.Context) error {
	return h.notes.FilterNotes(c)
}

// UpdateResource обновляет ресурс.
func (h *Handler) UpdateResource(c echo.Context) error {
	return h.resources.UpdateResource(c)
}
