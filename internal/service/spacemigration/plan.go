// Package spacemigration классифицирует пространства только по доверенной полной выгрузке.
package spacemigration

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
)

// Record содержит подтверждённые владельца и тип существующего пространства.
type Record struct {
	SpaceID   uuid.UUID `json:"space_id"`
	OwnerID   uuid.UUID `json:"owner_id"`
	SpaceType string    `json:"space_type"`
}

// Plan содержит только недостающие markers, без изменения владельцев и членства.
type Plan struct {
	Spaces int                  `json:"spaces"`
	Writes []sdk.ClientTupleKey `json:"writes"`
}

type existingSpace struct {
	owners []string
	types  []string
}

// BuildPlan проверяет полное покрытие, уникальность и совместимость выгрузки с сохранёнными отношениями.
func BuildPlan(records []Record, tuples []openfga.Tuple) (Plan, error) {
	existing, err := inventory(tuples)
	if err != nil {
		return Plan{}, err
	}

	seen := make(map[string]bool, len(records))
	plan := Plan{Spaces: len(existing), Writes: []sdk.ClientTupleKey{}}

	for _, record := range records {
		object, missing, err := validateRecord(record, existing)
		if err != nil {
			return Plan{}, err
		}

		if seen[object] {
			return Plan{}, fmt.Errorf("duplicate space: %s", object)
		}

		seen[object] = true

		if missing {
			plan.Writes = append(plan.Writes, sdk.ClientTupleKey{Object: object, Relation: record.SpaceType, User: "user:*"})
		}
	}

	if len(seen) != len(existing) {
		return Plan{}, fmt.Errorf("incomplete export: %d of %d spaces", len(seen), len(existing))
	}

	sort.Slice(plan.Writes, func(i, j int) bool { return plan.Writes[i].Object < plan.Writes[j].Object })

	return plan, nil
}

func validateRecord(record Record, existing map[string]existingSpace) (string, bool, error) {
	if record.SpaceID == uuid.Nil || record.OwnerID == uuid.Nil || (record.SpaceType != "personal" && record.SpaceType != "shared") {
		return "", false, fmt.Errorf("invalid export record")
	}

	object := "space:" + record.SpaceID.String()

	space, ok := existing[object]
	if !ok || len(space.owners) != 1 || space.owners[0] != "user:"+record.OwnerID.String() {
		return "", false, fmt.Errorf("space or owner mismatch: %s", object)
	}

	if len(space.types) > 1 || (len(space.types) == 1 && space.types[0] != record.SpaceType) {
		return "", false, fmt.Errorf("space type conflict: %s", object)
	}

	return object, len(space.types) == 0, nil
}

func inventory(tuples []openfga.Tuple) (map[string]existingSpace, error) {
	spaces := make(map[string]existingSpace)

	for _, tuple := range tuples {
		key := tuple.GetKey()
		// Ссылка дочернего объекта тоже выявляет пространство, даже если у него нет owner.
		if key.GetRelation() == "space" && strings.HasPrefix(key.GetUser(), "space:") {
			if _, ok := spaces[key.GetUser()]; !ok {
				spaces[key.GetUser()] = existingSpace{}
			}
		}

		if !strings.HasPrefix(key.GetObject(), "space:") {
			continue
		}

		space := spaces[key.GetObject()]
		switch key.GetRelation() {
		case "owner":
			space.owners = append(space.owners, key.GetUser())
		case "personal", "shared":
			if key.GetUser() != "user:*" {
				return nil, fmt.Errorf("invalid marker: %s", key.GetObject())
			}

			space.types = append(space.types, key.GetRelation())
		}

		spaces[key.GetObject()] = space
	}

	return spaces, nil
}
