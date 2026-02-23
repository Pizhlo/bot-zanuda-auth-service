package auth

import (
	"errors"
	"time"
)

// Service - сервис для работы с авторизацией.
// используется для получения ключа авторизации из vault и его обновления, а также для генерации jwt токенов.
type Service struct {
	updateKeyInterval time.Duration // периодичность, с которой нужно обновлять ключ
	vaultClient       vaultClient   // клиент для доступа к vault
	secretKey         []byte
}

// vaultClient - интерфейс для доступа к vault.
//
//go:generate mockgen -source=service.go -destination=mocks/mocks.go -package=mocks
type vaultClient interface {
	// здесь методы для доступа к vault
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

// New создает новый сервис для работы с авторизацией.
func New(opts ...option) (*Service, error) {
	s := &Service{}

	for _, opt := range opts {
		opt(s)
	}

	if s.updateKeyInterval == 0 {
		return nil, errors.New("update key interval is required")
	}

	if s.vaultClient == nil {
		return nil, errors.New("vault client is required")
	}

	if len(s.secretKey) == 0 {
		return nil, errors.New("secret key is required")
	}

	return s, nil
}
