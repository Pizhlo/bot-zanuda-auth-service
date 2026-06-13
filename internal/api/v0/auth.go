package v0

import (
	"auth-service/internal/model"
	"auth-service/internal/service/auth"
	"auth-service/internal/storage"
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

// AuthHandler хендлер для авторизации.
type AuthHandler struct {
	authSerivce authSerivce
}

//go:generate mockgen -source=auth.go -destination=mocks/auth_svc_mock.go -package=mocks authSerivce
type authSerivce interface {
	Login(context.Context, model.LoginRequest) (model.LoginResponse, error)
}

type authHandlerOption func(*AuthHandler)

// WithAuthService устанавливает сервис авторизации.
func WithAuthService(auth authSerivce) authHandlerOption {
	return func(h *AuthHandler) {
		h.authSerivce = auth
	}
}

// NewAuthHandler создает новый хендлер для авторизации.
func NewAuthHandler(opts ...authHandlerOption) (*AuthHandler, error) {
	h := &AuthHandler{}

	for _, opt := range opts {
		opt(h)
	}

	if h.authSerivce == nil {
		return nil, errors.New("auth service is required")
	}

	logrus.Info("created auth handler")

	return h, nil
}

// Login проверяет корректность полученных данных и отправляет в ответ JWT-токен.
func (s *AuthHandler) Login(c echo.Context) error {
	var req model.LoginRequest

	if err := c.Bind(&req); err != nil {
		logrus.WithError(err).Error("error binding request")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "cannot bind request"})
	}

	resp, err := s.authSerivce.Login(c.Request().Context(), req)
	if err != nil {
		logrus.WithError(err).Error("login failed, raw error")

		if errors.Is(err, auth.ErrInvalidGrantType) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid grant type"})
		}

		if errors.Is(err, storage.ErrNotFound) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid client"})
		}

		if errors.Is(err, auth.ErrInactiveClient) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "client is inactive"})
		}

		if errors.Is(err, auth.ErrInvalidSecret) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid client secret"})
		}

		if errors.Is(err, auth.ErrInvalidScope) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "invalid scope"})
		}

		if errors.Is(err, auth.ErrEmptyLoginRequest) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "empty login request"})
		}

		logrus.WithError(err).Error("error logging in")

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, resp)
}
