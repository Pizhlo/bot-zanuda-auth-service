// Package spacemigration классифицирует пространства только по доверенной полной выгрузке.
package spacemigration

import (
	"fmt"

	"auth-service/internal/model"
	"context"
	"strings"

	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
)

// Record — строка полной доверенной выгрузки бизнес-данных: UUID пространства,
// его единственный владелец и подтверждённый тип. Тип не выводится из наличия участников.
type Record struct {
	SpaceID   uuid.UUID       `json:"space_id"`
	OwnerID   uuid.UUID       `json:"owner_id"`
	SpaceType model.SpaceKind `json:"space_type"`
}

// Plan содержит число проверенных пространств, недостающие отметки UUID, типов и владельцев personal.
// Владельцы и участники не изменяются. Пустой Writes означает готовность данных
// к строгой модели, только если выгрузка полна и конкурирующие записи остановлены.
type Plan struct {
	Spaces int                  `json:"spaces"`
	Writes []sdk.ClientTupleKey `json:"writes"`
}

type existingSpace struct {
	created bool
	owners  []model.Resource
	types   []string
}

// BuildPlan сопоставляет полную выгрузку с прочитанными tuples без побочных записей.
// Проверяет владельцев, дубли, пропущенные пространства и конфликты типов; при ошибке
// не возвращает частичный план. Записи отсортированы по объекту для повторяемого dry-run.
func BuildPlan(ctx context.Context, records []Record, tuples []openfga.Tuple) (Plan, error) {
	existing, err := inventory(ctx, tuples)
	if err != nil {
		return Plan{}, fmt.Errorf("inventory existing spaces: %w", err)
	}

	seen := make(map[string]bool, len(records))
	plan := Plan{Spaces: len(existing), Writes: []sdk.ClientTupleKey{}}

	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return Plan{}, err
		}

		object, missing, err := validateRecord(record, existing)
		if err != nil {
			return Plan{}, fmt.Errorf("validate export record: %w", err)
		}

		if seen[object] {
			return Plan{}, fmt.Errorf("duplicate space: %s", object)
		}

		seen[object] = true

		if !existing[object].created {
			plan.Writes = append(plan.Writes, sdk.ClientTupleKey{Object: object, Relation: model.RelationSpaceCreated, User: "user:*"})
		}

		if missing {
			plan.Writes = append(plan.Writes, sdk.ClientTupleKey{Object: object, Relation: string(record.SpaceType), User: "user:*"})
		}
	}

	if len(seen) != len(existing) {
		return Plan{}, fmt.Errorf("incomplete export: %d of %d spaces", len(seen), len(existing))
	}

	markers, err := personalSpaceMarkers(ctx, records, tuples)
	if err != nil {
		return Plan{}, fmt.Errorf("validate personal space owners: %w", err)
	}

	plan.Writes = append(plan.Writes, markers...)

	plan.Writes, err = sortWrites(ctx, plan.Writes)
	if err != nil {
		return Plan{}, err
	}

	return plan, nil
}

func validateRecord(record Record, existing map[string]existingSpace) (string, bool, error) {
	if record.SpaceID == uuid.Nil || record.OwnerID == uuid.Nil || !record.SpaceType.Valid() {
		return "", false, fmt.Errorf("invalid export record")
	}

	object := "space:" + record.SpaceID.String()

	space, ok := existing[object]
	if !ok || len(space.owners) != 1 || !space.owners[0].Equal(model.Resource{Type: model.ResourceTypeUser, ID: record.OwnerID}) {
		return "", false, fmt.Errorf("space or owner mismatch: %s", object)
	}

	if len(space.types) > 1 || (len(space.types) == 1 && space.types[0] != string(record.SpaceType)) {
		return "", false, fmt.Errorf("space type conflict: %s", object)
	}

	return object, len(space.types) == 0, nil
}

func inventory(ctx context.Context, tuples []openfga.Tuple) (map[string]existingSpace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	spaces := make(map[string]existingSpace)

	for _, tuple := range tuples {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

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
		if err := space.add(key); err != nil {
			return nil, fmt.Errorf("collect space metadata: %w", err)
		}

		spaces[key.GetObject()] = space
	}

	return spaces, nil
}

// add извлекает только метаданные пространства; пользовательские роли сохраняются вне плана.
func (s *existingSpace) add(key openfga.TupleKey) error {
	switch key.GetRelation() {
	case string(model.RelationTypeOwner):
		owner, err := model.ParseResourceIdentity(key.GetUser())
		if err != nil || owner.Type != model.ResourceTypeUser {
			return fmt.Errorf("invalid space owner: %s", key.GetObject())
		}

		s.owners = append(s.owners, owner)
	case model.RelationSpaceCreated:
		if key.GetUser() != "user:*" {
			return fmt.Errorf("invalid creation marker: %s", key.GetObject())
		}

		s.created = true
	case string(model.SpaceKindPersonal), string(model.SpaceKindShared):
		if key.GetUser() != "user:*" {
			return fmt.Errorf("invalid marker: %s", key.GetObject())
		}

		s.types = append(s.types, key.GetRelation())
	}

	return nil
}

// personalSpaceMarkers дополняет индекс владельцев из проверенной полной выгрузки.
// Два личных пространства у владельца или отметка без пространства требуют сверки данных.
func personalSpaceMarkers(ctx context.Context, records []Record, tuples []openfga.Tuple) ([]sdk.ClientTupleKey, error) {
	missing, err := missingPersonalOwners(ctx, records)
	if err != nil {
		return nil, err
	}

	for _, row := range tuples {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		key := row.GetKey()
		if key.Relation != model.RelationHasPersonalSpace {
			continue
		}

		if key.Object != key.User || !missing[key.Object] {
			return nil, fmt.Errorf("unexpected personal space marker on %s", key.Object)
		}

		delete(missing, key.Object)
	}

	writes := make([]sdk.ClientTupleKey, 0, len(missing))
	for owner := range missing {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		writes = append(writes, sdk.ClientTupleKey{Object: owner, Relation: model.RelationHasPersonalSpace, User: owner})
	}

	return writes, nil
}

// sortWrites устойчиво сортирует отметки по объекту. При слиянии соседних
// серий проверяет отмену, чтобы большая выгрузка укладывалась в общий бюджет CLI.
func sortWrites(ctx context.Context, writes []sdk.ClientTupleKey) ([]sdk.ClientTupleKey, error) {
	if len(writes) < 2 {
		return writes, ctx.Err()
	}

	buffer := make([]sdk.ClientTupleKey, len(writes))
	for width := 1; width < len(writes); width *= 2 {
		for start := 0; start < len(writes); start += 2 * width {
			mid := min(start+width, len(writes))
			end := min(mid+width, len(writes))

			if err := mergeWrites(ctx, buffer[start:end], writes[start:mid], writes[mid:end]); err != nil {
				return nil, err
			}
		}

		writes, buffer = buffer, writes
	}

	return writes, ctx.Err()
}

// missingPersonalOwners собирает ожидаемые отметки владельцев и отвергает дубли личных пространств.
func missingPersonalOwners(ctx context.Context, records []Record) (map[string]bool, error) {
	missing := make(map[string]bool)

	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if record.SpaceType != model.SpaceKindPersonal {
			continue
		}

		owner := "user:" + record.OwnerID.String()
		if missing[owner] {
			return nil, fmt.Errorf("multiple personal spaces for %s", owner)
		}

		missing[owner] = true
	}

	return missing, nil
}

// mergeWrites сливает две отсортированные серии, сохраняя исходный порядок равных объектов.
func mergeWrites(ctx context.Context, out, left, right []sdk.ClientTupleKey) error {
	for i := range out {
		if err := ctx.Err(); err != nil {
			return err
		}

		if len(right) == 0 || (len(left) > 0 && left[0].Object <= right[0].Object) {
			out[i], left = left[0], left[1:]
		} else {
			out[i], right = right[0], right[1:]
		}
	}

	return nil
}
