package fga

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"auth-service/internal/model"
	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	openFGAClient "github.com/openfga/go-sdk/client"
)

// ErrSpacePrivacy обозначает нарушение неизменяемого типа или защиты пространства.
var ErrSpacePrivacy = errors.New("invalid space privacy operation")

// ReadTuples читает все страницы отношений объекта с higher_consistency; пустой object читает весь store.
func ReadTuples(ctx context.Context, api openFGAClient.SdkClient, object string) ([]openfga.Tuple, error) {
	var tuples []openfga.Tuple

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

		if next == token {
			return nil, errors.New("read space relations: repeated continuation token")
		}

		token = next
	}
}

func (c *Client) guardSpace(ctx context.Context, req model.UpdateResourceRequest, actor uuid.UUID) error {
	if req.Resource.Type != model.ResourceTypeSpace {
		return nil
	}

	if err := validateSpaceIdentity(req, actor); err != nil {
		return err
	}

	tuples, err := ReadTuples(ctx, c.fgaClient, formatObject(req.Resource))
	if err != nil {
		return err
	}

	if len(tuples) == 0 && req.ChangeType == model.ChangeTypeResourceAdded {
		return nil
	}

	storedKind, owner, err := spaceIdentity(tuples)
	if err != nil {
		return err
	}

	if kind, hasKind := req.Resource.Attributes["space_type"]; hasKind && kind != storedKind {
		return ErrSpacePrivacy
	}

	if req.Relations.Owner.ID.String() != owner {
		return ErrSpacePrivacy
	}

	if req.ChangeType == model.ChangeTypeResourceAdded {
		return ErrResourceAlreadyExistsOrNotFound
	}

	if storedKind == "personal" {
		return ErrSpacePrivacy
	}

	return guardMembers(req, owner)
}

func validateSpaceIdentity(req model.UpdateResourceRequest, actor uuid.UUID) error {
	if req.Relations.Owner.Type != model.ResourceTypeUser {
		return ErrOwnerTypeInvalid
	}

	kind, hasKind := req.Resource.Attributes["space_type"]
	if hasKind && kind != "personal" && kind != "shared" {
		return ErrSpacePrivacy
	}

	if req.ChangeType != model.ChangeTypeResourceAdded {
		return nil
	}

	if !hasKind || req.Relations.Owner.ID != actor || actor == uuid.Nil || len(req.Relations.Members) > 0 {
		return ErrSpacePrivacy
	}

	return nil
}

func guardMembers(req model.UpdateResourceRequest, owner string) error {
	for _, member := range req.Relations.Members {
		if member.UserID == uuid.Nil || member.UserID.String() == owner {
			return ErrSpacePrivacy
		}

		if member.NewRole == "owner" || member.OldRole == "owner" {
			return ErrSpacePrivacy
		}
	}

	return nil
}

func spaceIdentity(tuples []openfga.Tuple) (string, string, error) {
	kind, owner := "", ""

	for _, tuple := range tuples {
		key := tuple.GetKey()
		switch key.GetRelation() {
		case "personal", "shared":
			if key.GetUser() != "user:*" || kind != "" {
				return "", "", ErrSpacePrivacy
			}

			kind = key.GetRelation()
		case "owner":
			if owner != "" {
				return "", "", ErrSpacePrivacy
			}

			var err error

			owner, err = ownerID(key.GetUser())
			if err != nil {
				return "", "", err
			}
		}
	}

	if kind == "" || owner == "" {
		return "", "", ErrSpacePrivacy
	}

	return kind, owner, nil
}

func ownerID(user string) (string, error) {
	id, err := uuid.Parse(strings.TrimPrefix(user, "user:"))
	if err != nil || id == uuid.Nil || user != "user:"+id.String() {
		return "", ErrSpacePrivacy
	}

	return id.String(), nil
}
