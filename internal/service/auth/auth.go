package auth

import (
	"auth-service/internal/model"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrInactiveClient возвращается, если клиент не активен.
	ErrInactiveClient = errors.New("client is inactive")
	// ErrInvalidSecret возвращается, если секрет клиента не совпадает с хранимым в Vault.
	ErrInvalidSecret = errors.New("invalid client secret")
	// ErrInvalidScope возвращается, если scope не соответствует разрешенным scopes из БД.
	ErrInvalidScope = errors.New("invalid scope")
	// ErrInvalidGrantType возвращается, если grant type неверный.
	ErrInvalidGrantType = errors.New("invalid grant type")
)

const (
	internalAPIAudience = "zanuda-internal-api"
)

// Login проверяет grant type и вызывает соответствующую функцию.
// Допустимые grant type:
//
//   - ClientCredentialsGrantType
func (s *Service) Login(ctx context.Context, req model.LoginRequest) (model.LoginResponse, error) {
	switch req.GrantType {
	case model.ClientCredentialsGrantType:
		return s.loginWithClientCredentials(ctx, req)
	default:
		return model.LoginResponse{}, ErrInvalidGrantType
	}
}

func (s *Service) loginWithClientCredentials(ctx context.Context, req model.LoginRequest) (model.LoginResponse, error) {
	client, err := s.storage.GetServiceClient(ctx, req.ClientID)
	if err != nil {
		return model.LoginResponse{}, err
	}

	if !client.IsActive {
		return model.LoginResponse{}, ErrInactiveClient
	}

	clientSecret, err := s.vaultClient.GetClientSecret(req.ClientID)
	if err != nil {
		return model.LoginResponse{}, fmt.Errorf("error getting client secret: %w", err)
	}

	if clientSecret == "" {
		return model.LoginResponse{}, ErrInvalidSecret
	}

	if subtle.ConstantTimeCompare([]byte(req.ClientSecret), []byte(clientSecret)) != 1 {
		return model.LoginResponse{}, ErrInvalidSecret
	}

	var scope []string
	if req.Scope == "" { // если scope не передан, используем все scopes из БД
		scope = client.Scopes
	} else {
		// если scope передан, проверяем, что каждый scope содержится в БД (client.Scopes)
		scope, err = validateScopes(client.Scopes, string(req.Scope))
		if err != nil {
			return model.LoginResponse{}, ErrInvalidScope
		}
	}

	now := time.Now()

	token, err := s.generateToken(tokenClaims{
		Scope: strings.Join(scope, " "),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   req.ClientID,
			Audience:  []string{internalAPIAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenDuration)),
		},
	})
	if err != nil {
		return model.LoginResponse{}, fmt.Errorf("error generating token: %w", err)
	}

	return model.LoginResponse{
		AccessToken: token,
		TokenType:   model.BearerTokenType,
		ExpiresIn:   int(s.tokenDuration.Seconds()),
		Scope:       model.Scope(strings.Join(scope, " ")),
	}, nil
}

func validateScopes(allowed []string, requestedStr string) ([]string, error) {
	requested := parseScopes(requestedStr)

	// если клиент не просил scope вообще – выдать все разрешённые
	if len(requested) == 0 {
		return allowed, nil
	}

	for _, s := range requested {
		if !hasScope(allowed, s) {
			return nil, fmt.Errorf("invalid_scope: '%s' is not allowed", s)
		}
	}

	return requested, nil
}

func hasScope(allowed []string, s string) bool {
	for _, a := range allowed {
		if strings.EqualFold(a, s) {
			return true
		}
	}

	return false
}

func parseScopes(scopeStr string) []string {
	if scopeStr == "" {
		return nil
	}

	parts := strings.Fields(scopeStr) // режет по пробелам и табам

	return parts
}
