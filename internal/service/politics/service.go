package politics

import (
	"auth-service/internal/model"
	"context"
	"errors"
)

// Service - сервис политик. К нему обращаются, когда нужно выяснить, может ли пользователь
// выполнить какое-либо действие или получить к чему-то доступ.
type Service struct {
	storage                storage
	spaceAccessChecker     spaceAccessChecker
	notePermissionResolver notePermissionResolver
}

// storage - интерфейс для доступа к хранилищу.
//
//go:generate mockgen -source=service.go -destination=mocks/storage_mock.go -package=mocks Storage
type storage interface {
	// FilterNoteIDs фильтрует входящие айди, чтобы они все принадлежали пространствам.
	// Пример: список [1, 2, 3, 4, 5, 6], в пространстве только: [1, 2, 3] -> вернется только [1, 2, 3].
	FilterNoteIDs(ctx context.Context, spaceID int, ids []int) ([]int, error)
}

type spaceAccessChecker interface {
	CheckSpaceAccess(ctx context.Context, userID, spaceID int) (model.SpaceMember, error)
}

type notePermissionResolver interface {
	ResolveNotePermissions(ctx context.Context, access model.SpaceMember, noteIDs []int) (map[int]model.NoteAccessInfo, error)
}

type option func(*Service)

// WithStorage устанавливает хранилище.
func WithStorage(storage storage) option {
	return func(s *Service) {
		s.storage = storage
	}
}

// WithSpaceAccessChecker устанавливает сервис, проверяющий доступ к пространству.
func WithSpaceAccessChecker(checker spaceAccessChecker) option {
	return func(s *Service) {
		s.spaceAccessChecker = checker
	}
}

// WithNotePermissionResolver устанавливает сервис, разрешающий политики доступ к заметкам.
func WithNotePermissionResolver(resolver notePermissionResolver) option {
	return func(s *Service) {
		s.notePermissionResolver = resolver
	}
}

// New создает новый сервис политик.
func New(opts ...option) (*Service, error) {
	s := &Service{}

	for _, opt := range opts {
		opt(s)
	}

	if s.storage == nil {
		return nil, errors.New("storage is required")
	}

	if s.spaceAccessChecker == nil {
		return nil, errors.New("space access checker is required")
	}

	if s.notePermissionResolver == nil {
		return nil, errors.New("note permission resolver is required")
	}

	return s, nil
}
