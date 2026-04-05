package auth

import (
	"auth-service/internal/model"
	"auth-service/pkg/audit"
	"context"
	"errors"
	"time"
)

// Service - сервис для работы с авторизацией.
// используется для получения ключа авторизации из vault и его обновления, а также для генерации jwt токенов.
type Service struct {
	updateKeyInterval time.Duration // периодичность, с которой нужно обновлять ключ
	vaultClient       vaultClient   // клиент для доступа к vault
	secretKey         []byte
	tokenDuration     time.Duration
	storage           storage
	issuer            string
	auditor           auditor
}

// auditor - интерфейс для доступа к auditor.
//
//go:generate mockgen -source=service.go -destination=mocks/mocks.go -package=mocks auditor
type auditor interface {
	Create(ctx context.Context) audit.Event
}

// vaultClient - интерфейс для доступа к vault.
//
//go:generate mockgen -source=service.go -destination=mocks/mocks.go -package=mocks
type vaultClient interface {
	GetClientSecret(clientID string) (string, error)
}

type storage interface {
	GetServiceClient(ctx context.Context, clientID string) (model.ServiceClient, error)
}

type option func(*Service)

// WithSecretKey устанавливает секретный ключ для генерации jwt-токена.
func WithSecretKey(secretKey []byte) option {
	return func(a *Service) {
		a.secretKey = secretKey
	}
}

// WithUpdateKeyInterval устанавливает периодичность обновления ключа авторизации.
func WithUpdateKeyInterval(interval time.Duration) option {
	return func(s *Service) {
		s.updateKeyInterval = interval
	}
}

// WithVaultClient устанавливает клиент для доступа к vault.
func WithVaultClient(client vaultClient) option {
	return func(s *Service) {
		s.vaultClient = client
	}
}

// WithIssuer устанавливает issuer для генерации токена.
func WithIssuer(issuer string) option {
	return func(s *Service) {
		s.issuer = issuer
	}
}

// WithTokenDuration устанавливает duration для генерации токена.
func WithTokenDuration(duration time.Duration) option {
	return func(s *Service) {
		s.tokenDuration = duration
	}
}

// WithStorage устанавливает хранилище для сервиса.
func WithStorage(storage storage) option {
	return func(s *Service) {
		s.storage = storage
	}
}

// WithAuditor устанавливает auditor для событий ошибок.
func WithAuditor(builder auditor) option {
	return func(s *Service) {
		s.auditor = builder
	}
}

// New создает новый сервис для работы с авторизацией.
func New(opts ...option) (*Service, error) {
	s := &Service{}

	for _, opt := range opts {
		opt(s)
	}

	if s.updateKeyInterval <= 0 {
		return nil, errors.New("update key interval must be greater than 0")
	}

	if s.vaultClient == nil {
		return nil, errors.New("vault client is required")
	}

	if len(s.secretKey) == 0 {
		return nil, errors.New("secret key is required")
	}

	if s.storage == nil {
		return nil, errors.New("storage is required")
	}

	if s.issuer == "" {
		return nil, errors.New("issuer is required")
	}

	if s.tokenDuration <= 0 {
		return nil, errors.New("token duration must be greater than 0")
	}

	if s.auditor == nil {
		return nil, errors.New("auditor is required")
	}

	return s, nil
}

// GetIssuer возвращает issuer для генерации токена.
func (s *Service) GetIssuer() string {
	return s.issuer
}

// GetServiceClient возвращает клиента по clientID.
func (s *Service) GetServiceClient(ctx context.Context, clientID string) (model.ServiceClient, error) {
	return s.storage.GetServiceClient(ctx, clientID)
}
