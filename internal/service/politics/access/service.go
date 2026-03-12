package access

import (
	"auth-service/internal/model"
	"context"
	"errors"

	"github.com/google/uuid"
)

// SpaceAccessChecker - сервис для определения доступа к пространству.
type SpaceAccessChecker struct {
	spacesRepo spacesRepo
}

//go:generate mockgen -source=service.go -destination=mocks/storage_mock.go -package=mocks
type spacesRepo interface {
	GetSpaceMember(ctx context.Context, userID, spaceID uuid.UUID) (model.SpaceMember, error)
}

type option func(*SpaceAccessChecker)

// WithStorage устанавливает хранилище.
func WithStorage(spacesRepo spacesRepo) option {
	return func(s *SpaceAccessChecker) {
		s.spacesRepo = spacesRepo
	}
}

// New создает новый сервис для определения доступа к пространству - SpaceAccessChecker.
func New(opts ...option) (*SpaceAccessChecker, error) {
	s := &SpaceAccessChecker{}

	for _, opt := range opts {
		opt(s)
	}

	if s.spacesRepo == nil {
		return nil, errors.New("storage is required")
	}

	return s, nil
}
