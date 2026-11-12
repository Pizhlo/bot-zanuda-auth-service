package fga

import (
	"context"
	"errors"
	"fmt"
	"time"

	"auth-service/internal/model"
	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	openFGAClient "github.com/openfga/go-sdk/client"
)

// ErrSpacePrivacy означает нарушение типа, владельца или защиты участников пространства.
// Возвращается до Write; неизвестный или противоречивый сохранённый тип тоже запрещает запись.
var ErrSpacePrivacy = errors.New("invalid space privacy operation")

type tupleReader interface {
	Read(context.Context) openFGAClient.SdkClientReadRequestInterface
}

// ReadTuples собирает все страницы с higher_consistency; пустой object читает весь store.
// Весь проход ограничен переданным бюджетом и контекстом вызывающего кода. Цикл continuation
// tokens или ошибка страницы возвращает ошибку без частичного результата.
func ReadTuples(ctx context.Context, api tupleReader, object string, timeout time.Duration) ([]openfga.Tuple, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("read tuples: timeout must be positive")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var tuples []openfga.Tuple

	seen := make(map[string]struct{})

	token := ""
	preference := openfga.CONSISTENCYPREFERENCE_HIGHER_CONSISTENCY

	body := openFGAClient.ClientReadRequest{}
	if object != "" {
		body.Object = &object
	}

	for {
		resp, err := api.Read(ctx).Body(body).Options(openFGAClient.ClientReadOptions{ContinuationToken: &token, Consistency: &preference}).Execute()
		if err != nil {
			return nil, fmt.Errorf("read space relations: %w", err)
		}

		if resp == nil {
			return nil, errors.New("read space relations: empty response")
		}

		tuples = append(tuples, resp.GetTuples()...)

		next := resp.GetContinuationToken()
		if next == "" {
			return tuples, nil
		}

		if _, repeated := seen[next]; repeated {
			return nil, errors.New("read space relations: repeated continuation token")
		}

		seen[next] = struct{}{}
		token = next
	}
}

// guardSpace проверяет неизменяемый тип и владельца по сохранённым tuples перед записью.
// Check не является пропуском: вызывающий синхронизацию мог не выполнить его или выполнить раньше.
// Это защита инвариантов пространства, а не повторная авторизация всего UpdateResource.
func (c *Client) guardSpace(ctx context.Context, req model.UpdateResourceRequest, actor uuid.UUID) error {
	if req.Resource.Type != model.ResourceTypeSpace {
		return nil
	}

	if err := validateSpaceIdentity(req, actor); err != nil {
		return fmt.Errorf("validate space identity: %w", err)
	}

	tuples, err := ReadTuples(ctx, c.fgaClient, formatObject(req.Resource), c.requestTimeout)
	if err != nil {
		return fmt.Errorf("read stored space: %w", err)
	}

	if len(tuples) == 0 && req.ChangeType == model.ChangeTypeResourceAdded {
		return nil
	}

	storedKind, owner, err := spaceIdentity(tuples)
	if err != nil {
		return fmt.Errorf("decode stored space identity: %w", err)
	}

	if err := validateStoredSpace(req, storedKind, owner); err != nil {
		return fmt.Errorf("validate stored space operation: %w", err)
	}

	return nil
}

func validateStoredSpace(req model.UpdateResourceRequest, storedKind model.SpaceKind, owner model.Resource) error {
	if req.ChangeType == model.ChangeTypeResourceAdded {
		return ErrResourceAlreadyExistsOrNotFound
	}

	if kind, hasKind := req.Resource.Attributes[model.AttributeSpaceType]; hasKind && kind != string(storedKind) {
		return ErrSpacePrivacy
	}

	if !req.Relations.Owner.Equal(owner) {
		return ErrSpacePrivacy
	}

	if storedKind == model.SpaceKindPersonal {
		return ErrSpacePrivacy
	}

	if err := guardMembers(req, owner); err != nil {
		return fmt.Errorf("guard space membership: %w", err)
	}

	return nil
}

func validateSpaceIdentity(req model.UpdateResourceRequest, actor uuid.UUID) error {
	if req.Relations.Owner.Type != model.ResourceTypeUser {
		return ErrOwnerTypeInvalid
	}

	kind, hasKind := req.Resource.Attributes[model.AttributeSpaceType]

	kindName, validKind := kind.(string)
	if hasKind && (!validKind || !model.SpaceKind(kindName).Valid()) {
		return ErrSpacePrivacy
	}

	if req.ChangeType != model.ChangeTypeResourceAdded {
		return nil
	}

	if !hasKind || !req.Relations.Owner.Equal(model.Resource{Type: model.ResourceTypeUser, ID: actor}) || actor == uuid.Nil || len(req.Relations.Members) > 0 {
		return ErrSpacePrivacy
	}

	return nil
}

func guardMembers(req model.UpdateResourceRequest, owner model.Resource) error {
	for _, member := range req.Relations.Members {
		if member.UserID == uuid.Nil || owner.Equal(model.Resource{Type: model.ResourceTypeUser, ID: member.UserID}) {
			return ErrSpacePrivacy
		}

		if member.NewRole.IsOwner() || member.OldRole.IsOwner() {
			return ErrSpacePrivacy
		}
	}

	return nil
}

func spaceIdentity(tuples []openfga.Tuple) (model.SpaceKind, model.Resource, error) {
	var (
		kind  model.SpaceKind
		owner model.Resource
	)

	for _, tuple := range tuples {
		key := tuple.GetKey()
		switch key.GetRelation() {
		case string(model.SpaceKindPersonal), string(model.SpaceKindShared):
			if key.GetUser() != "user:*" || kind != "" {
				return "", model.Resource{}, ErrSpacePrivacy
			}

			kind = model.SpaceKind(key.GetRelation())
		case string(model.RelationTypeOwner):
			if !owner.IsEmpty() {
				return "", model.Resource{}, ErrSpacePrivacy
			}

			var err error

			owner, err = ownerResource(key.GetUser())
			if err != nil {
				return "", model.Resource{}, fmt.Errorf("parse stored space owner: %w", err)
			}
		}
	}

	if kind == "" || owner.IsEmpty() {
		return "", model.Resource{}, ErrSpacePrivacy
	}

	return kind, owner, nil
}

func ownerResource(user string) (model.Resource, error) {
	owner, err := model.ParseResourceIdentity(user)
	if err != nil || owner.Type != model.ResourceTypeUser {
		return model.Resource{}, ErrSpacePrivacy
	}

	return owner, nil
}
