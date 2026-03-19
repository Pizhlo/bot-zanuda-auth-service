package postgres

import (
	"auth-service/internal/model"
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

// FilterNoteIDs фильтрует входящий список айди: возвращает только те, что существуют в пространстве.
func (db *Repo) FilterNoteIDs(ctx context.Context, spaceID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, db.readTimeout)
	defer cancel()

	q := `SELECT id
FROM notes.notes
WHERE space_id = $1
  AND id = ANY($2);`

	res := make([]uuid.UUID, 0, len(ids))

	rows, err := db.db.QueryContext(ctxTimeout, q, spaceID, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("error filtering IDs: %w", err)
	}

	defer func() {
		err := rows.Close()
		if err != nil {
			logrus.Errorf("FilterIDs: error closing rows: %v", err)
		}
	}()

	for rows.Next() {
		var id uuid.UUID

		err := rows.Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}

		res = append(res, id)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("rows error: %w", rows.Err())
	}

	return res, nil
}

// GetNotesVisibility возвращает информацию об уровнях видимости заметок.
func (db *Repo) GetNotesVisibility(ctx context.Context, ids []uuid.UUID) ([]model.NoteVisibility, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, db.readTimeout)
	defer cancel()

	q := `SELECT id, visibility_type
FROM notes.notes
WHERE id = ANY($1);`

	res := make([]model.NoteVisibility, 0, len(ids))

	rows, err := db.db.QueryContext(ctxTimeout, q, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("error getting notes visibility: %w", err)
	}

	defer func() {
		if err := rows.Close(); err != nil {
			logrus.Errorf("GetNotesVisibility: error closing rows: %v", err)
		}
	}()

	for rows.Next() {
		var note model.NoteVisibility

		err := rows.Scan(&note.ID, &note.Visibility)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}

		res = append(res, note)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("rows error: %w", rows.Err())
	}

	return res, nil
}
