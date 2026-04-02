package postgres

import (
	"auth-service/internal/storage"
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// GetUserIDByTelegramID возвращает ID пользователя по telegram ID.
func (db *Repo) GetUserIDByTelegramID(ctx context.Context, telegramID string) (uuid.UUID, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, db.readTimeout)
	defer cancel()

	q := `SELECT id FROM users.users WHERE id = (SELECT user_id FROM users.telegram WHERE tg_id = $1);`

	row := db.db.QueryRowContext(ctxTimeout, q, telegramID)

	var userID uuid.UUID

	err := row.Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, storage.ErrNotFound
		}

		return uuid.Nil, err
	}

	return userID, nil
}
