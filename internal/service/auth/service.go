package auth

import (
	"errors"
	"time"
)

// service - сервис для работы с авторизацией.
// используется для получения ключа авторизации из vault и его обновления, а также для генерации jwt токенов.
type service struct {
	updateKeyInterval time.Duration // периодичность, с которой нужно обновлять ключ
	vaultClient       vaultClient   // клиент для доступа к vault
}

// vaultClient - интерфейс для доступа к vault.
//
//go:generate mockgen -source=service.go -destination=mocks/mocks.go -package=mocks
type vaultClient interface {
	// здесь методы для доступа к vault
}

type option func(*service)

// WithUpdateKeyInterval устанавливает периодичность обновления ключа авторизации.
func WithUpdateKeyInterval(interval time.Duration) option {
	return func(s *service) {
		s.updateKeyInterval = interval
	}
}

// WithVaultClient устанавливает клиент для доступа к vault.
func WithVaultClient(client vaultClient) option {
	return func(s *service) {
		s.vaultClient = client
	}
}

// New создает новый сервис для работы с авторизацией.
func New(opts ...option) (*service, error) {
	s := &service{}

	for _, opt := range opts {
		opt(s)
	}

	if s.updateKeyInterval == 0 {
		return nil, errors.New("update key interval is required")
	}

	if s.vaultClient == nil {
		return nil, errors.New("vault client is required")
	}

	return s, nil
}
