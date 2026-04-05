package vault

import (
	"auth-service/internal/storage"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// GetClientSecret получает секрет клиента из Vault по clientID по пути, указанному в secretsPath.
func (c *Client) GetClientSecret(clientID string) (string, error) {
	logrus.WithField("client_id", clientID).Debug("getting client secret")

	// Для KV v2 путь вида: "<mount>/data/<path>"
	// пример: "secret/data/bots/my-bot"
	path := fmt.Sprintf("%s/%s", c.secretsPath, clientID)

	secret, err := c.client.Logical().Read(path)
	if err != nil {
		return "", fmt.Errorf("read secret %q: %w", path, err)
	}

	if secret == nil {
		return "", fmt.Errorf("secret %q not found", path)
	}

	// Для KV v2 реальные данные лежат в secret.Data["data"]
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		// Если KV v1, то ключ сразу в Data
		data = secret.Data
	}

	v, ok := data["api_key"].(string)
	if !ok {
		warnings := secret.Warnings
		if len(warnings) > 0 {
			logrus.WithField("warnings", strings.Join(warnings, ", ")).Warning("warnings while getting client secret")
		}

		return "", storage.ErrNotFound
	}

	return v, nil
}
