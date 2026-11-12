package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"auth-service/internal/model"
)

// SaveDecision записывает вычисленное решение одной вставкой до отправки результата клиенту.
func (db *Repo) SaveDecision(ctx context.Context, d model.DecisionRecord) error {
	ctx, cancel := context.WithTimeout(ctx, db.insertTimeout)
	defer cancel()

	attributes, err := json.Marshal(d.Action.Attributes)
	if err != nil {
		return fmt.Errorf("marshal decision action attributes: %w", err)
	}

	reasons, err := json.Marshal(d.Reasons)
	if err != nil {
		return fmt.Errorf("marshal decision reasons: %w", err)
	}

	const insert = `INSERT INTO auth.decisions
		(decision_id, request_id, client_id, subject_id, action, action_attributes, resource_type,
		 resource_id, space_id, checked_object, checked_relation, result, reasons, policy_version, authorization_model_id, space_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

	_, err = db.db.ExecContext(ctx, insert, d.DecisionID, d.RequestID, d.ClientID, d.SubjectID,
		d.Action.Name, string(attributes), d.Resource.Type, d.Resource.ID, d.Resource.Attributes.SpaceID,
		d.CheckedObject, d.CheckedRelation, d.Result, string(reasons), d.PolicyVersion, d.AuthorizationModelID, d.Resource.Attributes.SpaceType)
	if err != nil {
		return fmt.Errorf("insert decision: %w", err)
	}

	return nil
}
