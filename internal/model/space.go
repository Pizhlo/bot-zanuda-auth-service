package model

import "time"

// SpaceType определяет тип пространства: личное или совместное.
type SpaceType string

const (
	// PersonalType - личное пространство, доступное только пользователю.
	PersonalType = "PERSONAL"
	// SharedType - совместное пространство, в которое можно приглашать других пользователей.
	SharedType = "SHARED"
)

// Space - пространство. Может быть личным и совместным.
// В совместное можно приглашать других пользователей, личное доступно только пользователю-создателю.
// Используется для хранения записей пользователей: заметок, напоминаний.
type Space struct {
	ID                     int
	Type                   SpaceType
	OwnerID                int
	DefaultParticipantRole string
	CreatedAt              time.Time
}

// RoleCode - роль участников пространства.
type RoleCode string

const (
	// OwnerRoleCode - владелец пространства. Имеет самые широкие разрешения, ему доступно все.
	OwnerRoleCode = "OWNER"
	// AdminRoleCode - админ пространства. Чуть меньше, чем владелец: ему недоступно только управление ролями.
	AdminRoleCode = "ADMIN"
	// EditorRoleCode - стандартная роль: можно просматривать и редактировать контент, но нельзя управлять пространством.
	EditorRoleCode = "EDITOR"
	// ViewerRoleCode - самая минимальная роль. Доступен только просмотр контента, ничего редактировать и удалять нельзя.
	ViewerRoleCode = "VIEWER"
)

// MemberStatus - статус участника пространства.
type MemberStatus string

const (
	// ActiveMemberStatus - активный участник пространства (принял приглашение, не забанен и не удален).
	ActiveMemberStatus = "ACTIVE"
	// InvitedMemberStatus - приглашенный участник. Ожидание принятия приглашения. Пока не имеет доступ ни к чему.
	InvitedMemberStatus = "INVITED"
	// BlockedMemberStatus - заблокированный (удаленный) участник пространства. Уже не имеет доступ ни к чему.
	BlockedMemberStatus = "BLOCKED"
)

// SpaceMember - участник пространства.
// Описывает информацию об участнике пространства: статус, может ли приглашать других, роль и т.д.
type SpaceMember struct {
	UserID    int
	IsMember  bool
	RoleCode  string // OWNER / ADMIN / EDITOR / VIEWER / ...
	InvitedBy int
	Status    MemberStatus
	CanInvite bool
	CreatedAt time.Time
}
