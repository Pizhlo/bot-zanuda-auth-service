package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

// CheckToken проверяет токен на валидность: поле exp и наличие payload.
func (s *Service) CheckToken(authHeader string) (*jwt.Token, error) {
	logrus.Debug("checking token")

	scheme, tokenString, ok := strings.Cut(authHeader, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return nil, errors.New("invalid token: no prefix Bearer")
	}

	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, errors.New("invalid token: empty bearer token")
	}

	claims := jwt.MapClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		return s.secretKey, nil
	},
		jwt.WithValidMethods([]string{"HS256"}), // Защита от alg confusion
		jwt.WithIssuer(s.issuer),                // Проверка iss
		jwt.WithAudience(internalAPIAudience),   // Проверка aud
		jwt.WithExpirationRequired(),            // Обязательный exp
		// WithIssuedAt: если iat есть — проверить, что токен не «из будущего» (сам claim по RFC опционален).
		jwt.WithIssuedAt(),
	)
	if err != nil {
		logrus.WithError(err).Warn("token validation failed")
		return nil, errors.New("invalid token")
	}

	iat, err := claims.GetIssuedAt()
	if err != nil {
		logrus.WithError(err).Warn("token validation failed")
		return nil, errors.New("invalid token")
	}

	if iat == nil {
		errMissing := fmt.Errorf("%w: %s", jwt.ErrTokenRequiredClaimMissing, "iat claim is required")
		logrus.WithError(errMissing).Warn("token validation failed")

		return nil, errors.New("invalid token")
	}

	if !token.Valid {
		return nil, errors.New("token invalid")
	}

	return token, nil
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
