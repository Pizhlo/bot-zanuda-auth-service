package decision

import (
	"auth-service/internal/model"
	"fmt"
	"slices"

	"github.com/google/uuid"
)

// validateRequest проверяет форму DTO и правила выбранной операции до внешних вызовов.
// Назначаемая роль проверяется как часть операции member, а не как источник прав caller.
func validateRequest(req model.CheckDecisionRequest) (string, string, error) {
	if err := req.Validate(); err != nil {
		return "", "", fmt.Errorf("validate input: %w", err)
	}

	if err := validateActionAttributes(req); err != nil {
		return "", "", fmt.Errorf("validate action attributes: %w", err)
	}

	if err := validateResourceAttributes(req); err != nil {
		return "", "", fmt.Errorf("validate resource attributes: %w", err)
	}

	object, relation, err := resolveTarget(req)
	if err != nil {
		return "", "", fmt.Errorf("resolve authorization target: %w", err)
	}

	return object, relation, nil
}

func validateResourceAttributes(r model.CheckDecisionRequest) error {
	space := r.Resource.Attributes.SpaceID

	requiresSpace := r.Resource.Type == model.ResourceTypeMember || (r.Action.Name == model.ActionNameCreate && r.Resource.Type != model.ResourceTypeSpace)
	if requiresSpace && space == nil {
		return model.InvalidDecisionRequest("resource.attributes.space_id is required for note/reminder create and member actions")
	}

	if space != nil && (*space == uuid.Nil || r.Resource.Type == model.ResourceTypeSpace) {
		return model.InvalidDecisionRequest("resource.attributes.space_id must be a nonzero UUID and is not supported for space resources")
	}

	return validateRequestedSpaceType(r)
}

func validateRequestedSpaceType(r model.CheckDecisionRequest) error {
	kind := r.Resource.Attributes.SpaceType
	if r.Resource.Type == model.ResourceTypeSpace && r.Action.Name == model.ActionNameCreate {
		if kind == nil {
			return model.InvalidDecisionRequest("resource.attributes.space_type is required for space:create")
		}

		if !kind.Valid() {
			return model.InvalidDecisionRequest("resource.attributes.space_type must be shared or personal")
		}
	} else if kind != nil {
		return model.InvalidDecisionRequest("resource.attributes.space_type is only supported for space:create")
	}

	return nil
}

func validateActionAttributes(r model.CheckDecisionRequest) error {
	fields := r.Action.Attributes.FieldMask
	if r.Action.Name == model.ActionNameUpdate {
		if len(fields) == 0 {
			return model.InvalidDecisionRequest("action.attributes.field_mask must be nonempty for update")
		}
	} else if fields != nil {
		return model.InvalidDecisionRequest("action.attributes.field_mask is only supported for update")
	}

	memberRole := r.Resource.Type == model.ResourceTypeMember && (r.Action.Name == model.ActionNameAdd || r.Action.Name == model.ActionNameEdit)

	role := r.Action.Attributes.Role
	if memberRole {
		if role == nil || !slices.Contains([]string{string(model.RelationTypeViewer), string(model.RelationTypeEditor), string(model.RelationTypeAdmin)}, *role) {
			return model.InvalidDecisionRequest("action.attributes.role must be viewer, editor or admin for member add/edit")
		}
	} else if role != nil {
		return model.InvalidDecisionRequest("action.attributes.role is only supported for member add/edit")
	}

	return nil
}

// resolveTarget выбирает объект и отношение из фиксированной матрицы действий.
// Для create записи и member проверяется родительский space; для существующей записи — она сама.
// Для создания personal проверяется отметка владельца, для shared пара пуста. Тип уже проверен validateRequestedSpaceType.
func resolveTarget(r model.CheckDecisionRequest) (string, string, error) {
	if r.Resource.Type == model.ResourceTypeSpace && r.Action.Name == model.ActionNameCreate {
		if *r.Resource.Attributes.SpaceType == model.SpaceKindPersonal {
			return "user:" + r.Subject.ID.String(), model.RelationHasPersonalSpace, nil
		}

		return "", "", nil
	}

	relation := ""

	object := string(r.Resource.Type) + ":" + r.Resource.ID.String()
	switch r.Resource.Type {
	case model.ResourceTypeNote, model.ResourceTypeReminder:
		relation = recordRelation(r)
		if r.Action.Name == model.ActionNameCreate && r.Resource.Attributes.SpaceID != nil {
			object = "space:" + r.Resource.Attributes.SpaceID.String()
		}
	case model.ResourceTypeSpace:
		relation = spaceRelation(r.Action.Name)
	case model.ResourceTypeMember:
		if slices.Contains([]model.ActionName{model.ActionNameAdd, model.ActionNameEdit, model.ActionNameRemove}, r.Action.Name) {
			relation = string(model.ActionTypeManageMembers)

			if r.Resource.Attributes.SpaceID != nil {
				object = "space:" + r.Resource.Attributes.SpaceID.String()
			}
		}
	}

	if relation == "" {
		return "", "", model.InvalidDecisionRequest("action.name is not supported for resource.type")
	}

	return object, relation, nil
}

func spaceRelation(action model.ActionName) string {
	switch action {
	case model.ActionNameRead:
		return string(model.ActionTypeRead)
	case model.ActionNameUpdate:
		return string(model.ActionTypeManageSpace)
	case model.ActionNameDelete:
		return string(model.ActionTypeCanDelete)
	default:
		return ""
	}
}

func recordRelation(r model.CheckDecisionRequest) string {
	switch r.Action.Name {
	case model.ActionNameCreate:
		if r.Resource.Type == model.ResourceTypeNote {
			return string(model.ActionTypeCreateNote)
		}

		return string(model.ActionTypeCreateReminder)
	case model.ActionNameRead:
		return string(model.ActionTypeRead)
	case model.ActionNameUpdate:
		return string(model.ActionTypeCanUpdate)
	case model.ActionNameDelete:
		return string(model.ActionTypeCanDelete)
	case model.ActionNameStop:
		if r.Resource.Type == model.ResourceTypeReminder {
			return string(model.ActionTypeCanStopReminder)
		}
	}

	return ""
}
