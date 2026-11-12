package decision

import (
	"slices"
	"strings"

	"auth-service/internal/model"
)

const (
	messageRegisteredUser             = "Registered user:%s may create shared space:%s"
	messageFirstPersonalSpace         = "OpenFGA relation %q is absent on %s; the user may create their first personal space"
	messagePersonalSpaceAlreadyExists = "OpenFGA relation %q is present on %s; the user already has a personal space"
	messageRelationPresent            = "OpenFGA allowed relation %q on %s for user:%s"
	messageRelationAbsent             = "OpenFGA denied relation %q on %s for user:%s"
	messageAllowedFields              = "Allowed fields for %s:%s: %s"
	messageForbiddenFields            = "Forbidden fields for %s:%s: %s"
	messageProtectedOwner             = "Membership of user:%s cannot be changed because they own %s"
)

func reason(code, message string) []model.DecisionReason {
	return []model.DecisionReason{{Code: code, Message: message}}
}

// reasonFields перечисляет только проверенные имена, не изменяя маску запроса.
func reasonFields(fields []string) string {
	unique := slices.Clone(fields)
	slices.Sort(unique)

	return strings.Join(slices.Compact(unique), ", ")
}
