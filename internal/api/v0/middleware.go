package v0

import (
	"auth-service/internal/model"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"
	"context"
	"errors"
	"net/http"
	"slices"

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
	CheckToken(ctx context.Context, authHeader string) (*jwt.Token, error)
	GetPayload(token *jwt.Token) (jwt.MapClaims, bool)
	GetServiceClient(ctx context.Context, clientID string) (model.ServiceClient, error)
	GetIssuer() string
}

type option func(*MiddlewareHandler)

// WithMiddlewareAuthService устанавливает сервис авторизации.
func WithMiddlewareAuthService(svc AuthService) option {
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

const userIDHeader = "X-Telegram-User-Id"

// CheckToken проверяет токен из запроса. Также проверяет хедер X-Telegram-User-Id.
// Токен должен быть валидным, не просроченным и иметь scope bot.
func (s *MiddlewareHandler) CheckToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		tokenHeader := c.Request().Header.Get("Authorization")

		ctx := c.Request().Context()

		token, err := s.authService.CheckToken(ctx, tokenHeader)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
		}

		payload, ok := s.authService.GetPayload(token)
		if !ok {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token: no payload"})
		}

		if code, err := s.verifyPayload(c.Request().Context(), payload); err != nil {
			return c.JSON(code, map[string]string{"error": err.Error()})
		}

		logrus.WithFields(logrus.Fields{
			"client_id": payload["sub"],
			"issuer":    payload["iss"],
			"scope":     payload["scope"],
			"exp":       payload["exp"],
			"iat":       payload["iat"],
			"aud":       payload["aud"],
		}).Debug("check token: payload verified")

		userID := c.Request().Header.Get(userIDHeader)
		if userID == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid request: header 'X-Telegram-User-Id' not found"})
		}

		ctx = withUserID(c.Request().Context(), userID)

		req := c.Request().WithContext(ctx)

		c.SetRequest(req)

		return next(c)
	}
}

// FillCtx заполняет контекст запроса данными из заголовков.
func FillCtx(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		ctx = withTraceID(ctx, c.Request().Header.Get("X-Trace-ID"))
		ctx = withRequestID(ctx, c.Request().Header.Get("X-Request-ID"))

		ipAddress := c.Request().Header.Get("X-Forwarded-For")

		if ipAddress == "" {
			ipAddress = c.Request().RemoteAddr
		}

		ctx = withIPAddress(ctx, ipAddress)

		userAgent := c.Request().Header.Get("User-Agent")

		if userAgent == "" {
			userAgent = "unknown"
		}

		ctx = withUserAgent(ctx, userAgent)

		c.SetRequest(c.Request().WithContext(ctx))

		return next(c)
	}
}

type ipAddressKey struct{}

func withIPAddress(ctx context.Context, ipAddress string) context.Context {
	return context.WithValue(ctx, ipAddressKey{}, ipAddress)
}

type userAgentKey struct{}

func withUserAgent(ctx context.Context, userAgent string) context.Context {
	return context.WithValue(ctx, userAgentKey{}, userAgent)
}

type traceIDKey struct{}

func withTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

type requestIDKey struct{}

func withRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// ConnectionHook добавляет данные из соединения (IP-адрес, User-Agent, User-ID) в событие.
func ConnectionHook(ctx context.Context, stash audit.Stash) audit.Stash {
	msgCtx := audit.EventContext{}

	if traceID, ok := ctx.Value(traceIDKey{}).(string); ok {
		stash = stash.Append(audit.TraceID(traceID))
	}

	if requestID, ok := ctx.Value(requestIDKey{}).(string); ok {
		stash = stash.Append(audit.RequestID(requestID))
	}

	if ipAddress, ok := ctx.Value(ipAddressKey{}).(string); ok {
		msgCtx["ip_address"] = ipAddress
	}

	if userAgent, ok := ctx.Value(userAgentKey{}).(string); ok {
		msgCtx["user_agent"] = userAgent
	}

	if userID, ok := ctx.Value(userIDKey{}).(string); ok {
		msgCtx["user_id"] = userID
	}

	if len(msgCtx) > 0 {
		stash = stash.Append(audit.ContextField(msgCtx, stash))
	}

	return stash
}

// verifyPayload проверяет payload токена на валидность.
// Возвращает код статуса и ошибку.
// 0 - если payload валидный.
func (s *MiddlewareHandler) verifyPayload(ctx context.Context, payload jwt.MapClaims) (int, error) {
	clientID, ok := payload["sub"].(string)
	if !ok || clientID == "" {
		return http.StatusUnauthorized, errors.New("invalid token: invalid client id")
	}

	scope, ok := payload["scope"].(string)
	if !ok || scope == "" {
		return http.StatusUnauthorized, errors.New("invalid token: invalid scope")
	}

	client, err := s.authService.GetServiceClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return http.StatusUnauthorized, errors.New("invalid client")
		}

		return http.StatusInternalServerError, errors.New("internal server error")
	}

	if !client.IsActive {
		return http.StatusUnauthorized, errors.New("client is inactive")
	}

	if !slices.Contains(client.Scopes, scope) {
		return http.StatusUnauthorized, errors.New("client does not have required scope")
	}

	return 0, nil
}

type userIDKey struct{}

func withUserID(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, userIDKey{}, value)
}

func parseUserID(value any) string {
	switch t := value.(type) {
	case string:
		return t
	default:
		return ""
	}
}
