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
	logrus.Debug("check token")

	if !strings.HasPrefix(authHeader, "Bearer") {
		return nil, errors.New("invalid token: no prefix Bearer")
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	token, err := s.ParseToken(tokenString)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %v", err)
	}

	_, ok := s.GetPayload(token)
	if !ok {
		return nil, errors.New("no payload found")
	}

	return token, nil
}

// ParseToken парсит токен в виде строки и возвращает *jwt.Token.
func (s *Service) ParseToken(tokenString string) (*jwt.Token, error) {
	logrus.Debug("parsing token")

	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		return s.secretKey, nil
	})
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
