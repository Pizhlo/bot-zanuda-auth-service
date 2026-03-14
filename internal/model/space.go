package model

import (
	"time"

	"github.com/google/uuid"
)

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
	ID                     uuid.UUID
	Type                   SpaceType
	OwnerID                uuid.UUID
	DefaultParticipantRole string
	CreatedAt              time.Time
}

// RoleCode - роль участников пространства.
type RoleCode string

const (
	// OwnerRoleCode - владелец пространства. Имеет самые широкие разрешения, ему доступно все.
	OwnerRoleCode RoleCode = "OWNER"
	// AdminRoleCode - админ пространства. Чуть меньше, чем владелец: ему недоступно только управление ролями.
	AdminRoleCode RoleCode = "ADMIN"
	// EditorRoleCode - стандартная роль: можно просматривать и редактировать контент, но нельзя управлять пространством.
	EditorRoleCode RoleCode = "EDITOR"
	// ViewerRoleCode - самая минимальная роль. Доступен только просмотр контента, ничего редактировать и удалять нельзя.
	ViewerRoleCode RoleCode = "VIEWER"
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
	UserID    uuid.UUID
	IsMember  bool
	RoleCode  RoleCode // OWNER / ADMIN / EDITOR / VIEWER / ...
	InvitedBy uuid.UUID
	Status    MemberStatus
	CanInvite bool
	CreatedAt time.Time
}

// CanEditSpaceNote отвечает, может ли участник редактировать заметки с visibility=SPACE.
//
// Сейчас: все, кроме VIEWER. В будущем можно завязать на permissions (NOTE_EDIT_ANY / NOTE_EDIT_OWN).
func (m SpaceMember) CanEditSpaceNote() bool {
	return m.RoleCode != ViewerRoleCode
}
