package v0

import (
	"context"
	"errors"
	"fmt"

	"auth-service/internal/service/decision"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// AuditDecision открывает событие при входе в HTTP, до общего middleware и проверки JWT.
func (s *MiddlewareHandler) AuditDecision(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		r := c.Request()

		id := decisionRequestHeaderID(r.Header)
		if id == uuid.Nil {
			id = uuid.New()
		}

		c.Response().Header().Set(echo.HeaderXRequestID, id.String())

		ctx := withRequestID(connectionContext(r), id.String())

		err := decision.AuditRequest(ctx, s.requestAuditor, decision.OperationCheckDecision, id, func(ctx context.Context) error {
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		})
		if errors.Is(err, audit.ErrMissingAttempt) && !c.Response().Committed {
			return writeDecisionError(c, err, decisionRequestHeaderID(r.Header), id)
		}

		return err
	}
}

// decisionClientError переводит причину отказа в публичную категорию decisions/check.
func decisionClientError(err error) error {
	switch {
	case errors.Is(err, errInvalidClientID), errors.Is(err, errInvalidTokenScope):
		return decision.ErrUnauthorized
	case errors.Is(err, storage.ErrNotFound), errors.Is(err, errInactiveClient), errors.Is(err, errMissingScope):
		return decision.ErrForbidden
	default:
		return fmt.Errorf("%w: %w", decision.ErrIdentityUnavailable, err)
	}
}
