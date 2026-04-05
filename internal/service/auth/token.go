package auth

import (
	"auth-service/internal/service/internal"
	"auth-service/pkg/audit"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

const (
	messageTokenValidationFailed = "failed to validate token"
	messageNoPrefixBearer        = "invalid token: no prefix Bearer"
	messageEmptyBearerToken      = "invalid token: empty bearer token"
)

var (
	// ErrInvalidToken возвращается, если токен невалиден.
	ErrInvalidToken = errors.New("invalid token")
)

// CheckToken проверяет токен на валидность: поле exp и наличие payload.
func (s *Service) CheckToken(ctx context.Context, authHeader string) (*jwt.Token, error) {
	operationCheckToken := fmt.Sprintf("%s.%s", serviceName, "check_token")

	ctx = internal.WithOperation(ctx, operationCheckToken)

	event := s.auditor.Create(fillCtx(ctx))
	defer internal.WithPanicRecovery(ctx, event)()

	logrus.Debug("checking token")

	scheme, tokenString, ok := strings.Cut(authHeader, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		event.WithError(audit.ErrCodeTokenInvalid, audit.KindValidation, errors.New(messageNoPrefixBearer))
		event.Append(audit.Message(messageNoPrefixBearer))
		event.Append(audit.Level(audit.ErrLevelWarn))

		return nil, errors.New(messageNoPrefixBearer)
	}

	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		event.WithError(audit.ErrCodeTokenInvalid, audit.KindValidation, errors.New(messageEmptyBearerToken))
		event.Append(audit.Message(messageEmptyBearerToken))
		event.Append(audit.Level(audit.ErrLevelWarn))

		return nil, errors.New(messageEmptyBearerToken)
	}

	token, claims, err := s.parseToken(tokenString)
	if err != nil {
		var errCode audit.ErrorCode

		if errors.Is(err, jwt.ErrTokenExpired) {
			errCode = audit.ErrCodeTokenExpired
		} else {
			errCode = audit.ErrCodeTokenInvalid
		}

		event.WithError(errCode, audit.KindValidation, err)
		event.Append(audit.Message(messageTokenValidationFailed))
		event.Append(audit.Level(audit.ErrLevelWarn))

		return nil, errors.New("invalid token")
	}

	err = validateIAT(claims, event)
	if err != nil {
		return nil, err
	}

	if !token.Valid {
		err := errors.New("token invalid")

		event.Append(audit.Message(messageTokenValidationFailed))
		event.WithError(audit.ErrCodeTokenInvalid, audit.KindValidation, err)
		event.Append(audit.Level(audit.ErrLevelWarn))

		return nil, err
	}

	return token, nil
}

// parseToken парсит и валидирует JWT с claims.
func (s *Service) parseToken(tokenString string) (*jwt.Token, jwt.MapClaims, error) {
	claims := jwt.MapClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		return s.secretKey, nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(s.issuer),
		jwt.WithAudience(internalAPIAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil {
		return nil, jwt.MapClaims{}, err
	}

	return token, claims, nil
}

// validateIAT проверяет наличие и значение iat.
func validateIAT(claims jwt.MapClaims, event audit.Event) error {
	iat, err := claims.GetIssuedAt()
	if err != nil {
		event.WithError(audit.ErrCodeTokenInvalid, audit.KindValidation, err)
		event.Append(audit.Message(messageTokenValidationFailed))
		event.Append(audit.Level(audit.ErrLevelWarn))

		return ErrInvalidToken
	}

	if iat == nil {
		errMissing := fmt.Errorf("%w: %s", jwt.ErrTokenRequiredClaimMissing, "iat claim is required")

		event.WithError(audit.ErrCodeTokenInvalid, audit.KindValidation, errMissing)
		event.AppendContext(audit.EventContext{"iat": "iat claim is required"})
		event.Append(audit.Message(messageTokenValidationFailed))
		event.Append(audit.Level(audit.ErrLevelWarn))

		return ErrInvalidToken
	}

	return nil
}

func fillCtx(ctx context.Context) context.Context {
	ctx = internal.WithServiceName(ctx)

	return ctx
}

// TokenValidationFailedHook создает событие о невалидности токена.
func TokenValidationFailedHook(ctx context.Context, stash audit.Stash) audit.Stash {
	if serviceName, ok := ctx.Value(internal.ServiceNameKey{}).(string); ok {
		stash = stash.Append(audit.ServiceName(serviceName))
	}

	if message, ok := ctx.Value(internal.MessageKey{}).(string); ok {
		stash = stash.Append(audit.Message(message))
	}

	if level, ok := ctx.Value(internal.LevelKey{}).(audit.ErrorLevel); ok {
		stash = stash.Append(audit.Level(level))
	}

	if errorCode, ok := ctx.Value(internal.ErrorCodeKey{}).(audit.ErrorCode); ok {
		stash = stash.Append(audit.ErrorCodeField(errorCode))
	}

	if messageCtx, ok := ctx.Value(internal.MessageContextKey{}).(audit.EventContext); ok {
		stash = stash.Append(audit.ContextField(messageCtx, stash))
	}

	if userID, ok := ctx.Value(internal.UserIDKey{}).(string); ok {
		stash = stash.Append(audit.UserID(userID))
	}

	if operation, ok := ctx.Value(internal.OperationKey{}).(string); ok {
		stash = stash.Append(audit.Operation(operation))
	}

	if kind, ok := ctx.Value(internal.KindKey{}).(audit.Kind); ok {
		stash = stash.Append(audit.KindField(kind))
	}

	return stash
}

// GetPayload возвращает информацию токена в виде map[string]any.
func (s *Service) GetPayload(token *jwt.Token) (jwt.MapClaims, bool) {
	if token == nil {
		return nil, false
	}

	payload, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, false
	}

	return payload, true
}

type tokenClaims struct {
	Scope string `json:"scope"`
	jwt.RegisteredClaims
}

func (s *Service) generateToken(claims tokenClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(s.secretKey)
	if err != nil {
		return "", err
	}

	return signed, nil
}
