package model

import "github.com/google/uuid"

// DecisionRecord хранит проверенную операцию и результат для последующего поиска по DecisionID.
type DecisionRecord struct {
	DecisionID           uuid.UUID
	RequestID            uuid.UUID
	ClientID             string
	SubjectID            uuid.UUID
	Action               DecisionAction
	Resource             DecisionResource
	CheckedObject        *string // Оба поля равны nil, если операция не требует проверки в OpenFGA.
	CheckedRelation      *string
	Result               string
	Reasons              []DecisionReason
	PolicyVersion        string
	AuthorizationModelID string
}
