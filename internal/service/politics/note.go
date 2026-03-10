package politics

import (
	"auth-service/internal/model"
	"auth-service/internal/service"
	"context"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// FilterNotes фильтрует входящий список айди заметок, возвращая только доступные пользователю и существующие в пространстве.
//
// Пример: список [1, 2, 3, 4, 5, 6], в пространстве только: [1, 2, 3], из них доступны пользователю: [2, 3] -> вернется только [2, 3].
func (s *Service) FilterNotes(ctx context.Context, req model.FilterNotesRequest) (map[uuid.UUID]model.NoteAccessInfo, error) {
	logrus.WithFields(logrus.Fields{
		"user_id":  req.UserID,
		"space_id": req.SpaceID,
		"note_ids": req.NoteIDs,
	}).Debug("filtering notes for space")

	spaceAccess, err := s.spaceAccessChecker.CheckSpaceAccess(ctx, req.UserID, req.SpaceID)
	if err != nil {
		return nil, err
	}

	if !spaceAccess.IsMember {
		logrus.WithFields(logrus.Fields{
			"user_id":  req.UserID,
			"space_id": req.SpaceID,
			"note_ids": req.NoteIDs,
		}).Debugf("user is not member: access denied")

		return nil, service.ErrUserNotMember
	}

	existingIDs, err := s.storage.FilterNoteIDs(ctx, req.SpaceID, req.NoteIDs)
	if err != nil {
		return nil, err
	}

	return s.notePermissionResolver.ResolveNotePermissions(ctx, spaceAccess, existingIDs)
}
