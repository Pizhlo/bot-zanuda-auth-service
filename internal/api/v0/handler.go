package v0

import (
	"errors"

	"github.com/sirupsen/logrus"
)

// Handler - хендлер версии 0.
type Handler struct {
	version   string
	buildDate string
	gitCommit string

	apiVersion string
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
