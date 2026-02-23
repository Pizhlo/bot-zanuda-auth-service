package politics

import (
	"auth-service/internal/model"
	"auth-service/internal/storage"
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetSpaceMember возвращает информацию об участнике пространства. Если не найдено пространство или участник - возвращает storage.ErrNotFound.
func (db *Repo) GetSpaceMember(ctx context.Context, userID, spaceID int) (model.SpaceMember, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, db.readTimeout)
	defer cancel()

	q := `SELECT sr.code, sm.invited_by, sm.status, sm.can_invite, sm.created_at
FROM spaces.space_member sm
JOIN spaces.space_role sr ON sm.role_id = sr.id
WHERE sm.space_id = $1 AND sm.user_id = $2;`

	row := db.db.QueryRowContext(ctxTimeout, q, spaceID, userID)

	var member model.SpaceMember

	var invitedBy sql.NullInt64

	err := row.Scan(&member.RoleCode, &invitedBy, &member.Status, &member.CanInvite, &member.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.SpaceMember{}, storage.ErrNotFound
		}

		return model.SpaceMember{}, fmt.Errorf("scan error: %w", err)
	}

	member.UserID = userID

	if invitedBy.Valid {
		member.InvitedBy = int(invitedBy.Int64)
	}

	return member, nil
}
