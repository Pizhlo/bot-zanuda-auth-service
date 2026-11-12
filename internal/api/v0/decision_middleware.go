package v0

import (
	"context"
	"errors"
	"slices"
	"strings"

	"auth-service/internal/service/decision"
	"auth-service/internal/storage"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type decisionClientKey struct{}

func decisionClient(ctx context.Context) string {
	id, _ := ctx.Value(decisionClientKey{}).(string)
	return id
}

// CheckDecisionToken проверяет клиента и отдельные scopes, сохраняя только доверенный client ID.
func (s *MiddlewareHandler) CheckDecisionToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		token, err := s.authService.CheckToken(ctx, c.Request().Header.Get("Authorization"))
		if err != nil {
			return decisionError(c, decision.ErrUnauthorized, uuid.Nil)
		}

		claims, ok := s.authService.GetPayload(token)
		if !ok {
			return decisionError(c, decision.ErrUnauthorized, uuid.Nil)
		}

		clientID, err := s.verifyDecisionClient(ctx, claims)
		if err != nil {
			return decisionError(c, err, uuid.Nil)
		}

		ctx = context.WithValue(ctx, decisionClientKey{}, clientID)
		c.SetRequest(c.Request().WithContext(ctx))

		return next(c)
	}
}

func (s *MiddlewareHandler) verifyDecisionClient(ctx context.Context, claims jwt.MapClaims) (string, error) {
	clientID, ok := claims["sub"].(string)
	if !ok || clientID == "" {
		return "", decision.ErrUnauthorized
	}

	scope, ok := claims["scope"].(string)
	if !ok || len(strings.Fields(scope)) == 0 {
		return "", decision.ErrUnauthorized
	}

	client, err := s.authService.GetServiceClient(ctx, clientID)
	if errors.Is(err, storage.ErrNotFound) {
		return "", decision.ErrForbidden
	}

	if err != nil {
		return "", decision.ErrIdentityUnavailable
	}

	if !client.IsActive {
		return "", decision.ErrForbidden
	}

	scopes := strings.Fields(scope)
	if !slices.Contains(scopes, "bot") {
		return "", decision.ErrForbidden
	}

	for _, tokenScope := range scopes {
		if !slices.Contains(client.Scopes, tokenScope) {
			return "", decision.ErrForbidden
		}
	}

	return clientID, nil
}
