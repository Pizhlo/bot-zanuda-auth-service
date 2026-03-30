package model

import (
	"time"

	"github.com/google/uuid"
)

// GrantType тип grant type для авторизации.
type GrantType string

const (
	// ClientCredentialsGrantType тип grant type для авторизации с client credentials (для машинных клиентов).
	ClientCredentialsGrantType GrantType = "client_credentials"
)

// Scope тип scope для авторизации.
type Scope string

const (
	// BotScope scope для бота.
	BotScope Scope = "bot"
)

// LoginRequest запрос на авторизацию.
type LoginRequest struct {
	GrantType    GrantType `json:"grant_type"`    // обязательный, сейчас поддерживаем только "client_credentials"
	ClientID     string    `json:"client_id"`     // bot
	ClientSecret string    `json:"client_secret"` // secret from vault
	Scope        Scope     `json:"scope"`         // опциональный параметр. Если не передан - использовать все scopes из service_clients.scopes; если передан — разбить по пробелу, проверить, что каждый scope содержится в service_clients.scopes; если нет — ошибка.
}

// TokenType тип токена.
type TokenType string

const (
	// BearerTokenType тип токена в виде Bearer.
	BearerTokenType TokenType = "Bearer"
)

// LoginResponse ответ на запрос на авторизацию.
type LoginResponse struct {
	AccessToken string    `json:"access_token"` // подписанный JWT
	TokenType   TokenType `json:"token_type"`   // всегда "Bearer"
	ExpiresIn   int       `json:"expires_in"`   // TTL токена в секундах (например, 900 = 15 минут)
	Scope       Scope     `json:"scope"`        // фактический список scope’ов, разделённых пробелом.
}

// ServiceClient информация о сервисе-клиенте (машинном клиенте).
type ServiceClient struct {
	ID         uuid.UUID `json:"id"`
	ClientID   string    `json:"client_id"`
	ClientName string    `json:"client_name"`
	Scopes     []string  `json:"scopes"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
