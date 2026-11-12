package decision

import (
	"context"
	"time"

	"auth-service/internal/model"
	"auth-service/pkg/audit"
	"github.com/sirupsen/logrus"
)

func (s *Service) record(ctx context.Context, caller Caller, req model.CheckDecisionRequest, resp model.CheckDecisionResponse, object, relation string) {
	event := s.audit.Create(ctx)
	event.Append(audit.KindField(audit.KindDomain))
	event.Append(audit.Level(audit.ErrLevelInfo))
	event.Append(audit.RequestID(req.RequestID.String()))
	event.Append(audit.StatusField(audit.StatusCompleted))

	fields := audit.EventContext{
		"caller": caller.ClientID, "subject": resp.Meta.Subject, "action": req.Action.Name,
		"resource": resp.Meta.Resource, "decision": resp.Result, "decision_id": resp.DecisionID.String(),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "policy_version": resp.Meta.PolicyVersion,
		"authorization_model_id": resp.Meta.AuthorizationModelID, "checked_object": nil, "checked_relation": nil,
	}
	if object != "" {
		fields["checked_object"] = object
		fields["checked_relation"] = relation
	}

	if req.Resource.Type == model.ResourceTypeMember {
		fields["target_user_id"] = req.Resource.ID.String()
	}

	if req.Action.Attributes.Role != nil {
		fields["role"] = *req.Action.Attributes.Role
	}

	event.AppendContext(fields)

	ender, ok := event.(interface{ End(context.Context) error })
	if !ok {
		return
	}

	deliveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()

	if err := ender.End(deliveryCtx); err != nil {
		logrus.Warn("decision audit delivery failed; local event was recorded")
	}
}
