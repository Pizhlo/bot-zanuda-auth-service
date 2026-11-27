package model

// Результаты завершённого решения. Ошибки зависимостей возвращаются отдельно
// и не должны подменяться DecisionResultDeny.
const (
	DecisionResultAllow = "allow"
	DecisionResultDeny  = "deny"
)

// Коды причин — стабильная машинная часть ответа. Клиенты должны использовать
// их, а не разбирать человекочитаемый Message; перечисление не раскрывает путь FGA.
const (
	ReasonFirstPersonalSpace         = "first_personal_space"
	ReasonPersonalSpaceAlreadyExists = "personal_space_already_exists"
	ReasonAllowedForRegisteredUser   = "allowed_for_registered_user"
	ReasonAllowedViaFGARelation      = "allowed_via_fga_relation"
	ReasonDeniedByFGARelation        = "denied_by_fga_relation"
	ReasonAllowedFields              = "allowed_fields"
	ReasonForbiddenFields            = "forbidden_fields"
	ReasonProtectedSpaceOwner        = "protected_space_owner"
)
