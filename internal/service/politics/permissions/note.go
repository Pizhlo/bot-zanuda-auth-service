package permissions

import (
	"auth-service/internal/model"
	"context"

	"github.com/sirupsen/logrus"
)

// NotePermissionResolver - сервис, разрешающий политики доступа к заметкам.
type NotePermissionResolver struct {
}

// NewNotePermissionResolver создает новый сервис, разрешающий политики доступа к заметкам - NotePermissionResolver.
func NewNotePermissionResolver() *NotePermissionResolver {
	return &NotePermissionResolver{}
}

// ResolveNotePermissions определяет, к каким заметкам у пользователя есть доступ. Возвращает мапу по айди с указанием, какие
// заметки можно читать, какие - редактировать.
func (s *NotePermissionResolver) ResolveNotePermissions(ctx context.Context, access model.SpaceMember, noteIDs []int) (map[int]model.NoteAccessInfo, error) {
	logrus.WithFields(logrus.Fields{
		"note_ids":  noteIDs,
		"role_code": access.RoleCode,
	}).Debugf("resolve note permissions")

	if access.RoleCode == model.OwnerRoleCode { // пользователь - владелец пространства, сразу разрешаем все
		logrus.WithFields(logrus.Fields{
			"note_ids":  noteIDs,
			"role_code": access.RoleCode,
		}).Debugf("user is owner: access all notes")

		return returnAllNotesWithFlag(noteIDs, true, true), nil
	}

	logrus.WithFields(logrus.Fields{
		"note_ids":  noteIDs,
		"role_code": access.RoleCode,
	}).Debugf("access denied")

	return map[int]model.NoteAccessInfo{}, nil
}

// returnAllNotesWithFlag возвращает мапу с переданными айди, указывая для всех переданные флаги canEdit и canRead.
func returnAllNotesWithFlag(noteIDs []int, canEdit, canRead bool) map[int]model.NoteAccessInfo {
	res := make(map[int]model.NoteAccessInfo, len(noteIDs))

	for _, id := range noteIDs {
		res[id] = model.NoteAccessInfo{
			CanRead: canRead,
			CanEdit: canEdit,
		}
	}

	return res
}
