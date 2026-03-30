package postgres

import (
	"auth-service/internal/model"
	"auth-service/internal/storage"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

// GetServiceClient получает информацию о сервисе-клиенте из БД по clientID.
func (db *Repo) GetServiceClient(ctx context.Context, clientID string) (model.ServiceClient, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, db.readTimeout)
	defer cancel()

	q := `SELECT id, client_id, client_name, scopes, is_active, created_at, updated_at
FROM auth.service_clients
WHERE client_id = $1;`

	row := db.db.QueryRowContext(ctxTimeout, q, clientID)

	var serviceClient model.ServiceClient

	err := row.Scan(&serviceClient.ID, &serviceClient.ClientID, &serviceClient.ClientName, (*pq.StringArray)(&serviceClient.Scopes), &serviceClient.IsActive, &serviceClient.CreatedAt, &serviceClient.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ServiceClient{}, storage.ErrNotFound
		}

		return model.ServiceClient{}, fmt.Errorf("scan error: %w", err)
	}

	return serviceClient, nil
}
