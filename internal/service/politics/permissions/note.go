package permissions

import (
	"auth-service/internal/model"
	"context"
	"errors"

	"github.com/sirupsen/logrus"
)

// NotePermissionResolver - сервис, разрешающий политики доступа к заметкам.
type NotePermissionResolver struct {
	storage storage
}

//go:generate mockgen -source=note.go -destination=mocks/mocks.go -package=mocks
type storage interface {
	// GetNotesVisibility возвращает информацию об уровнях видимости заметок.
	GetNotesVisibility(ctx context.Context, ids []int) ([]model.NoteVisibility, error)
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
func (s *NotePermissionResolver) ResolveNotePermissions(ctx context.Context, member model.SpaceMember, noteIDs []int) (map[int]model.NoteAccessInfo, error) {
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
	res := make(map[int]model.NoteAccessInfo, len(visibilities))
	for _, v := range visibilities {
		info := decideNoteAccess(member, v)
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
func decideNoteAccess(member model.SpaceMember, v model.NoteVisibility) model.NoteAccessInfo {
	// В будущем сюда легко добавить проверку:
	// - member.HasPermission("NOTE_VIEW_ALL")
	// - member.HasPermission("NOTE_EDIT_ANY")
	// - note ACL, и т.д.
	switch v.Visibility {
	case model.VisibilityTypeSpace:
		return accessForSpaceVisibility(member)
	// case model.VisibilityTypePrivateToAuthor:
	// case model.VisibilityTypeCustom:
	//   сюда добавишь, когда дойдёшь
	default:
		return denyAll
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

// grantSameAccessToAllNotes возвращает мапу с переданными айди, указывая для всех переданные флаги canEdit и canRead.
func grantSameAccessToAllNotes(noteIDs []int, canRead, canEdit bool) map[int]model.NoteAccessInfo {
	res := make(map[int]model.NoteAccessInfo, len(noteIDs))
	info := model.NoteAccessInfo{CanRead: canRead, CanEdit: canEdit}

	for _, id := range noteIDs {
		res[id] = info
	}

	return res
}
