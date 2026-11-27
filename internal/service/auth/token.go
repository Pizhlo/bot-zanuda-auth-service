package auth

import (
	"auth-service/pkg/audit"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

const (
	messageNoPrefixBearer   = "invalid token: no prefix Bearer"
	messageEmptyBearerToken = "invalid token: empty bearer token"
)

var (
	// ErrInvalidToken возвращается, если токен невалиден.
	ErrInvalidToken = errors.New("invalid token")
)

// CheckToken проверяет токен на валидность: поле exp и наличие payload.
func (s *Service) CheckToken(ctx context.Context, authHeader string) (*jwt.Token, error) {
	logrus.Debug("checking token")

	reject := func(public, reason error) (*jwt.Token, error) {
		audit.FromContext(ctx).Fail(audit.ErrCodeTokenInvalid, audit.KindValidation, audit.ErrLevelWarn, audit.MessageTokenVerificationFailed, reason)
		return nil, public
	}

	scheme, tokenString, ok := strings.Cut(authHeader, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		err := errors.New(messageNoPrefixBearer)
		return reject(err, err)
	}

	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		err := errors.New(messageEmptyBearerToken)
		return reject(err, err)
	}

	token, claims, err := s.parseToken(tokenString)
	if err != nil {
		return reject(ErrInvalidToken, tokenAuditCause(err))
	}

	err = validateIAT(claims)
	if err != nil {
		return reject(ErrInvalidToken, err)
	}

	if !token.Valid {
		err := errors.New("token invalid")
		return reject(err, err)
	}

	return token, nil
}

// tokenAuditCause сохраняет диагностику проверенных claims. Ошибки разбора
// неподписанных данных могут содержать введённые клиентом строки, например alg.
func tokenAuditCause(err error) error {
	if errors.Is(err, jwt.ErrTokenInvalidClaims) {
		return err
	}

	for _, reason := range []error{jwt.ErrTokenSignatureInvalid, jwt.ErrTokenMalformed, jwt.ErrTokenUnverifiable} {
		if errors.Is(err, reason) {
			return reason
		}
	}

	return ErrInvalidToken
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
func validateIAT(claims jwt.MapClaims) error {
	iat, err := claims.GetIssuedAt()
	if err != nil {
		return err
	}

	if iat == nil {
		return fmt.Errorf("%w: iat claim is required", jwt.ErrTokenRequiredClaimMissing)
	}

	return nil
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
