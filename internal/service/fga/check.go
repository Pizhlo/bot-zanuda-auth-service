package fga

import (
	"context"
	"fmt"

	"auth-service/internal/model"

	openfga "github.com/openfga/go-sdk"
	openFGAClient "github.com/openfga/go-sdk/client"
)

// CheckRelation проверяет одно отношение на закреплённой версии и передаёт режим согласованности SDK.
// Вызов ограничен настроенным request_timeout; ошибка транспорта, отмена или пустой ответ не превращаются в deny.
// Метод возвращает только boolean и не выводит из него путь наследования роли.
func (c *Client) CheckRelation(ctx context.Context, user, relation, object string, consistency model.Consistency) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	preference := openfga.CONSISTENCYPREFERENCE_HIGHER_CONSISTENCY

	switch consistency {
	case model.HigherConsistency:
	case model.MinimizeLatency:
		preference = openfga.CONSISTENCYPREFERENCE_MINIMIZE_LATENCY
	default:
		return false, fmt.Errorf("unsupported consistency: %s", consistency)
	}

	resp, err := c.fgaClient.Check(ctx).Body(openFGAClient.ClientCheckRequest{User: user, Relation: relation, Object: object}).Options(openFGAClient.ClientCheckOptions{AuthorizationModelId: &c.modelID, Consistency: &preference}).Execute()
	if err != nil {
		return false, fmt.Errorf("check relation: %w", err)
	}

	if resp == nil {
		return false, fmt.Errorf("check relation: empty response")
	}

	allowed, ok := resp.GetAllowedOk()
	if !ok {
		return false, fmt.Errorf("check relation: missing allowed result")
	}

	return *allowed, nil
}
