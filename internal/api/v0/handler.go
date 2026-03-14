package v0

import (
	"auth-service/internal/model"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Handler - хендлер версии 0.
type Handler struct {
	version   string
	buildDate string
	gitCommit string

	apiVersion string

	// services

	PoliticsService PoliticsService
}

// PoliticsService - интерфейс для доступа к сервису политик. Отвечает за доступ пользователей к данным.
//
//go:generate mockgen -source=handler.go -destination=mocks/politics_svc_mock.go -package=mocks PoliticsService
type PoliticsService interface {
	// FilterNotes фильтрует входящие заметки согласно политикам.
	// Возвращает только заметки, доступные пользователю, с флагом canEdit -
	// может ли пользователь редактировать заметку.
	FilterNotes(ctx context.Context, req model.FilterNotesRequest) (map[uuid.UUID]model.NoteAccessInfo, error)
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

// WithPoliticsService устанавливает сервис политик.
func WithPoliticsService(svc PoliticsService) handlerOption {
	return func(h *Handler) {
		h.PoliticsService = svc
	}
}

// New создает новый хендлер. Автоматически устанавливает версию хендлера на Version0.
func New(opts ...handlerOption) (*Handler, error) {
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

	if h.PoliticsService == nil {
		return nil, errors.New("politics service is required")
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
