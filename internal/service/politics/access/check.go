package access

import (
	"auth-service/internal/model"
	"auth-service/internal/storage"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// CheckSpaceAccess проверяет, является ли пользователь участником пространства.
func (s *SpaceAccessChecker) CheckSpaceAccess(ctx context.Context, userID, spaceID uuid.UUID) (model.SpaceMember, error) {
	logrus.WithFields(logrus.Fields{
		"user_id":  userID,
		"space_id": spaceID,
	}).Debugf("space access checker: checking space access")

	spaceAccess, err := s.spacesRepo.GetSpaceMember(ctx, userID, spaceID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return model.SpaceMember{
				IsMember: false,
			}, nil
		}

		return model.SpaceMember{}, fmt.Errorf("error getting space member: %w", err)
	}

	spaceAccess.IsMember = true // если было найдено и ошибок нет - значит участник существует

	logrus.WithFields(logrus.Fields{
		"user_id":  userID,
		"space_id": spaceID,
	}).Debugf("space access checker: user is member")

	return spaceAccess, nil
}
