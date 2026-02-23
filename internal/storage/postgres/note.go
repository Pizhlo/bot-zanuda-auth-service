package politics

import (
	"context"
	"fmt"

	"github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

// FilterNoteIDs фильтрует входящий список айди: возвращает только те, что существуют в пространстве.
func (db *Repo) FilterNoteIDs(ctx context.Context, spaceID int, ids []int) ([]int, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, db.readTimeout)
	defer cancel()

	q := `SELECT id
FROM notes.notes
WHERE space_id = $1
  AND id = ANY($2);`

	res := make([]int, 0, len(ids))

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
		var id int

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
