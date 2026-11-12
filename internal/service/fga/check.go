package fga

import (
	"context"
	"fmt"

	"auth-service/internal/model"
	openfga "github.com/openfga/go-sdk"
	openFGAClient "github.com/openfga/go-sdk/client"
)

// ModelID возвращает закреплённую для checks и writes модель.
func (c *Client) ModelID() string { return c.modelID }

// CheckRelation проверяет одну связь на закреплённой модели с указанной согласованностью.
func (c *Client) CheckRelation(ctx context.Context, user, relation, object string, consistency model.Consistency) (bool, error) {
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

	return resp.GetAllowed(), nil
}
