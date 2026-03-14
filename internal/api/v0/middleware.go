package v0

import (
	"context"
	"errors"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

// MiddlewareHandler - хендлер для использования middleware.
type MiddlewareHandler struct {
	authService AuthService
}

// AuthService - интерфейс для доступа к сервису авторизации для работы с токеном.
//
//go:generate mockgen -source=middleware.go -destination=mocks/mocks.go -package=mocks AuthService
type AuthService interface {
	CheckToken(authHeader string) (*jwt.Token, error)
	GetPayload(token *jwt.Token) (jwt.MapClaims, bool)
}

type option func(*MiddlewareHandler)

// WithAuthService устанавливает сервис авторизации.
func WithAuthService(svc AuthService) option {
	return func(h *MiddlewareHandler) {
		h.authService = svc
	}
}

// NewMiddlewareHandler создает новый хендлер для middleware.
func NewMiddlewareHandler(opts ...option) (*MiddlewareHandler, error) {
	h := &MiddlewareHandler{}

	for _, opt := range opts {
		opt(h)
	}

	if h.authService == nil {
		return nil, errors.New("auth service is required")
	}

	logrus.Info("created handler")

	return h, nil
}

// CheckToken проверяет токен из запроса. Проверяет на expired, а также наличие user_id.
func (s *MiddlewareHandler) CheckToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		tokenHeader := c.Request().Header.Get("Authorization")

		token, err := s.authService.CheckToken(tokenHeader)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
		}

		payload, ok := s.authService.GetPayload(token)
		if !ok {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token: no payload"})
		}

		userIDAny, ok := payload["user_id"]
		if !ok {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token: field 'user_id' not found"})
		}

		userID := parseUserID(userIDAny)
		if userID == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token: invalid type of field 'user_id'"})
		}

		ctx := withUserID(c.Request().Context(), userID)

		req := c.Request().WithContext(ctx)

		c.SetRequest(req)

		return next(c)
	}
}

type withUserIDCtxKey struct{}

func withUserID(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, withUserIDCtxKey{}, value)
}

func parseUserID(value any) string {
	switch t := value.(type) {
	case string:
		return t
	default:
		return ""
	}
}
