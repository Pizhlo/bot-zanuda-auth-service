package permissions

import (
	"auth-service/internal/model"
	db "auth-service/internal/storage"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// NotePermissionResolver - сервис, разрешающий политики доступа к заметкам.
type NotePermissionResolver struct {
	storage storage
}

//go:generate mockgen -source=note.go -destination=mocks/mocks.go -package=mocks
type storage interface {
	// GetNotesVisibility возвращает информацию об уровнях видимости заметок.
	GetNotesVisibility(ctx context.Context, ids []uuid.UUID) ([]model.NoteVisibility, error)
	// GetNoteACL возвращает информацию о доступе к заметке.
	// Проверяет, есть ли в ACL запись для пользователя и заметки.
	// Если заметки нет в ACL - пользователь может читать и редактировать заметку.
	GetNoteACL(ctx context.Context, userID int, noteID uuid.UUID) (model.NoteACL, error)
}

type option func(*NotePermissionResolver)

// WithStorage устанавливает хранилище.
func WithStorage(storage storage) option {
	return func(s *NotePermissionResolver) {
		s.storage = storage
	}
}

// NewNotePermissionResolver создает новый сервис, разрешающий политики доступа к заметкам - NotePermissionResolver.
func NewNotePermissionResolver(opts ...option) (*NotePermissionResolver, error) {
	s := &NotePermissionResolver{}

	for _, opt := range opts {
		opt(s)
	}

	if s.storage == nil {
		return nil, errors.New("storage is required")
	}

	return s, nil
}

// ResolveNotePermissions определяет, к каким заметкам у пользователя есть доступ. Возвращает мапу по айди с указанием, какие
// заметки можно читать, какие - редактировать.
func (s *NotePermissionResolver) ResolveNotePermissions(ctx context.Context, member model.SpaceMember, noteIDs []uuid.UUID) (map[uuid.UUID]model.NoteAccessInfo, error) {
	logger := logrus.WithFields(logrus.Fields{
		"user_id":   member.UserID,
		"note_ids":  noteIDs,
		"role_code": member.RoleCode,
	})

	logger.Debugf("resolve note permissions")

	if member.RoleCode == model.OwnerRoleCode { // пользователь - владелец пространства, сразу разрешаем все
		logger.Debugf("user is owner: access all notes")

		return grantSameAccessToAllNotes(noteIDs, true, true), nil
	}

	visibilities, err := s.storage.GetNotesVisibility(ctx, noteIDs)
	if err != nil {
		logger.WithError(err).Errorf("error getting notes visibility")

		return nil, err
	}

	// применяем “движок” правил
	res := make(map[uuid.UUID]model.NoteAccessInfo, len(visibilities))
	for _, v := range visibilities {
		info, err := s.decideNoteAccess(ctx, member, v)
		if err != nil {
			return nil, fmt.Errorf("error deciding note access: %v", err)
		}

		if info.CanRead || info.CanEdit {
			res[v.ID] = info
		}
	}

	logger.WithFields(
		logrus.Fields{
			"note_count": len(res),
		},
	).Debugf("user accessed to space notes")

	return res, nil
}

//nolint:gochecknoglobals // добавлено для большей ясности о том, что запрещено все
var denyAll = model.NoteAccessInfo{}

// decideNoteAccess применяет правила доступа к одной заметке.
func (s *NotePermissionResolver) decideNoteAccess(ctx context.Context, member model.SpaceMember, v model.NoteVisibility) (model.NoteAccessInfo, error) {
	// В будущем сюда легко добавить проверку:
	// - member.HasPermission("NOTE_VIEW_ALL")
	// - member.HasPermission("NOTE_EDIT_ANY")
	// - note ACL, и т.д.
	switch v.Visibility {
	case model.VisibilityTypeSpace:
		return accessForSpaceVisibility(member), nil

	case model.VisibilityTypeCustom:
		return s.accessForCustomVisibility(ctx, member, v)
	default:
		return denyAll, nil
	}
}

// accessForSpaceVisibility работает для совместных пространств для заметок с уровнем видимости SPACE.
//
// Все, кроме VIEWER, могут редактировать и читать заметки (can_read=true, can_edit=true).
//
// VIEWER: can_read=true, can_edit=false.
func accessForSpaceVisibility(member model.SpaceMember) model.NoteAccessInfo {
	return model.NoteAccessInfo{
		CanRead: true,
		CanEdit: member.CanEditSpaceNote(),
	}
}

func (s *NotePermissionResolver) accessForCustomVisibility(ctx context.Context, member model.SpaceMember, v model.NoteVisibility) (model.NoteAccessInfo, error) {
	access, err := s.storage.GetNoteACL(ctx, member.UserID, v.ID)
	if err != nil {
		// политики для заметки не найдены - пользователю разрешено ее смотреть
		if errors.Is(err, db.ErrNotFound) {
			return model.NoteAccessInfo{
				CanRead: true,
				CanEdit: true,
			}, nil
		}
		return model.NoteAccessInfo{}, fmt.Errorf("error getting note ACL: %v", err)
	}

	// если заметка запрещена - сразу выходим
	if !access.Allowed() {
		return model.NoteAccessInfo{
			CanRead: false,
			CanEdit: false,
		}, nil
	}

	return model.NoteAccessInfo{
		CanRead: access.CanRead,
		CanEdit: access.CanEdit,
	}, nil
}

// grantSameAccessToAllNotes возвращает мапу с переданными айди, указывая для всех переданные флаги canEdit и canRead.
func grantSameAccessToAllNotes(noteIDs []uuid.UUID, canRead, canEdit bool) map[uuid.UUID]model.NoteAccessInfo {
	res := make(map[uuid.UUID]model.NoteAccessInfo, len(noteIDs))
	info := model.NoteAccessInfo{CanRead: canRead, CanEdit: canEdit}

	for _, id := range noteIDs {
		res[id] = info
	}

	return res
}
