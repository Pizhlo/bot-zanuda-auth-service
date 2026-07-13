package fga

import (
	"errors"
	"fmt"

	"auth-service/internal/model"

	"github.com/google/uuid"
	openFGAClient "github.com/openfga/go-sdk/client"
)

// ErrNoTuplesToWriteOrDelete ошибка о том, что нет tuples для записи или удаления.
var ErrNoTuplesToWriteOrDelete = errors.New("no tuples to write or delete")

// toOpenFGAWriteRequest преобразует запрос на обновление ресурса в запрос на запись tuples в OpenFGA.
func toOpenFGAWriteRequest(req model.UpdateResourceRequest) (openFGAClient.ClientWriteRequest, error) {
	writes, deletes, err := tuplesFromRequest(req)
	if err != nil {
		return openFGAClient.ClientWriteRequest{}, err
	}

	if len(writes) == 0 && len(deletes) == 0 {
		return openFGAClient.ClientWriteRequest{}, ErrNoTuplesToWriteOrDelete
	}

	return openFGAClient.ClientWriteRequest{
		Writes:  writes,
		Deletes: deletes,
	}, nil
}

// tuplesFromRequest формирует tuples для записи и удаления на основе типа операции в запросе.
func tuplesFromRequest(req model.UpdateResourceRequest) (
	[]openFGAClient.ClientTupleKey,
	[]openFGAClient.ClientTupleKeyWithoutCondition,
	error,
) {
	switch req.Operation {
	case model.OperationCreate:
		return tuplesForCreate(req.Resource, req.Relations)
	default:
		return nil, nil, fmt.Errorf("unknown operation: %s", req.Operation)
	}
}

// tuplesForCreate формирует tuples для создания ресурса: owner и связь с родителем.
func tuplesForCreate(resource model.Resource, rel model.Relation) ([]openFGAClient.ClientTupleKey, []openFGAClient.ClientTupleKeyWithoutCondition, error) {
	object := formatObject(resource)
	writes := make([]openFGAClient.ClientTupleKey, 0, 2)

	if rel.Owner.ID != uuid.Nil {
		writes = append(writes, openFGAClient.ClientTupleKey{
			User:     formatObject(rel.Owner),
			Relation: "owner",
			Object:   object,
		})
	}

	if rel.Parent.ID != uuid.Nil {
		tuple, err := parentWrites(resource, rel)
		if err != nil {
			return nil, nil, err
		}

		writes = append(writes, tuple)
	}

	return writes, nil, nil
}

// parentWrites формирует tuple, связывающий ресурс с родителем (например, note -> space).
func parentWrites(resource model.Resource, rel model.Relation) (openFGAClient.ClientTupleKey, error) {
	if rel.Parent.ID == uuid.Nil {
		return openFGAClient.ClientTupleKey{}, fmt.Errorf("parent is required")
	}

	parentRelation, err := resource.ParentRelationName()
	if err != nil {
		return openFGAClient.ClientTupleKey{}, err
	}

	object := formatObject(resource)

	return openFGAClient.ClientTupleKey{
		User:     formatObject(rel.Parent),
		Relation: parentRelation,
		Object:   object,
	}, nil
}

// formatObject форматирует ресурс в строковое представление объекта OpenFGA: "type:id".
func formatObject(resource model.Resource) string {
	return fmt.Sprintf("%s:%s", resource.Type, resource.ID)
}

// toModelTuples преобразует tuples OpenFGA в модель ответа сервиса.
func toModelTuples(writes []openFGAClient.ClientTupleKey, deletes []openFGAClient.ClientTupleKeyWithoutCondition) ([]model.Tuple, []model.Tuple) {
	written := make([]model.Tuple, 0, len(writes))
	for _, tuple := range writes {
		written = append(written, model.Tuple{
			Subject:  tuple.User,
			Relation: tuple.Relation,
			Resource: tuple.Object,
		})
	}

	deleted := make([]model.Tuple, 0, len(deletes))
	for _, tuple := range deletes {
		deleted = append(deleted, model.Tuple{
			Subject:  tuple.User,
			Relation: tuple.Relation,
			Resource: tuple.Object,
		})
	}

	return written, deleted
}
