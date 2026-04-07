package politics

import (
	"auth-service/internal/model"
	"auth-service/pkg/audit"
	"context"
	"errors"

	"github.com/google/uuid"
)

// Service - сервис политик. К нему обращаются, когда нужно выяснить, может ли пользователь
// выполнить какое-либо действие или получить к чему-то доступ.
type Service struct {
	storage                storage
	spaceAccessChecker     spaceAccessChecker
	notePermissionResolver notePermissionResolver
	auditor                auditor
}

type auditor interface {
	Create(ctx context.Context) audit.Event
}

// storage - интерфейс для доступа к хранилищу.
//
//go:generate mockgen -source=service.go -destination=mocks/storage_mock.go -package=mocks Storage
type storage interface {
	// FilterNoteIDs фильтрует входящие айди, чтобы они все принадлежали пространствам.
	// Пример: список [1, 2, 3, 4, 5, 6], в пространстве только: [1, 2, 3] -> вернется только [1, 2, 3].
	FilterNoteIDs(ctx context.Context, spaceID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error)
}

type spaceAccessChecker interface {
	// CheckSpaceAccess проверяет, есть ли у пользователя доступ к пространству.
	CheckSpaceAccess(ctx context.Context, userID, spaceID uuid.UUID) (model.SpaceMember, error)
}

type notePermissionResolver interface {
	// ResolveNotePermissions определяет, к каким заметкам у пользователя есть доступ. Возвращает мапу по айди с указанием, какие
	// заметки можно читать, какие - редактировать.
	ResolveNotePermissions(ctx context.Context, access model.SpaceMember, noteIDs []uuid.UUID) (map[uuid.UUID]model.NoteAccessInfo, error)
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

// WithAuditor устанавливает сервис для логирования.
func WithAuditor(auditor auditor) option {
	return func(s *Service) {
		s.auditor = auditor
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

	if s.auditor == nil {
		return nil, errors.New("auditor is required")
	}

	return s, nil
}
