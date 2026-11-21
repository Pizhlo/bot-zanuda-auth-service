package auth

import (
	"auth-service/internal/model"
	db "auth-service/internal/storage"
	"auth-service/internal/storage/vault"
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
	// ErrClientSecretNotFound возвращается, если секрет клиента не найден в Vault.
	ErrClientSecretNotFound = errors.New("client secret not found in vault")
	// ErrInvalidScope возвращается, если scope не соответствует разрешенным scopes из БД.
	ErrInvalidScope = errors.New("invalid scope")
	// ErrInvalidGrantType возвращается, если grant type неверный.
	ErrInvalidGrantType = errors.New("invalid grant type")
	// ErrEmptyLoginRequest возвращается, если запрос на авторизацию пуст.
	ErrEmptyLoginRequest = errors.New("empty login request")
)

const internalAPIAudience = "zanuda-internal-api"

// Login проверяет grant type и вызывает соответствующую функцию.
// Допустимые grant type:
//
//   - ClientCredentialsGrantType
func (s *Service) Login(ctx context.Context, req model.LoginRequest) (model.LoginResponse, error) {
	switch req.GrantType {
	case model.ClientCredentialsGrantType:
		return s.loginWithClientCredentials(ctx, req)
	default:
		if req.IsEmpty() {
			return model.LoginResponse{}, ErrEmptyLoginRequest
		}

		return model.LoginResponse{}, ErrInvalidGrantType
	}
}

func (s *Service) loginWithClientCredentials(ctx context.Context, req model.LoginRequest) (model.LoginResponse, error) {
	client, err := s.validateClient(ctx, req.ClientID)
	if err != nil {
		return model.LoginResponse{}, err
	}

	if err := s.validateSecret(req.ClientID, req.ClientSecret); err != nil {
		if errors.Is(err, ErrClientSecretNotFound) {
			return model.LoginResponse{}, ErrInvalidSecret
		}

		return model.LoginResponse{}, err
	}

	scope, err := s.validateAndGetScopes(client.Scopes, string(req.Scope))
	if err != nil {
		return model.LoginResponse{}, ErrInvalidScope
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
		return model.LoginResponse{}, err
	}

	return model.LoginResponse{
		AccessToken: token,
		TokenType:   model.BearerTokenType,
		ExpiresIn:   int(s.tokenDuration.Seconds()),
		Scope:       model.Scope(strings.Join(scope, " ")),
	}, nil
}

// validateClient проверяет существование и активность клиента.
func (s *Service) validateClient(ctx context.Context, clientID string) (*model.ServiceClient, error) {
	client, err := s.storage.GetServiceClient(ctx, clientID)
	if err != nil {
		return nil, err
	}

	if !client.IsActive {
		return nil, ErrInactiveClient
	}

	return &client, nil
}

// validateSecret проверяет секрет из Vault.
func (s *Service) validateSecret(clientID string, clientSecret string) error {
	vaultSecret, err := s.vaultClient.GetClientSecret(clientID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrClientSecretNotFound
		}

		if errors.Is(err, vault.ErrClientSecretNotFound) {
			return ErrClientSecretNotFound
		}

		return err
	}

	if vaultSecret == "" {
		return ErrClientSecretNotFound
	}

	if subtle.ConstantTimeCompare([]byte(clientSecret), []byte(vaultSecret)) != 1 {
		return ErrInvalidSecret
	}

	return nil
}

// validateAndGetScopes возвращает валидные scopes.
func (s *Service) validateAndGetScopes(clientScopes []string, reqScope string) ([]string, error) {
	var scope []string

	if reqScope == "" {
		scope = clientScopes
	} else {
		var err error

		scope, err = validateScopes(clientScopes, reqScope)
		if err != nil {
			return nil, ErrInvalidScope
		}
	}

	return scope, nil
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
