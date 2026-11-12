package fga

import (
	fgamodel "auth-service/fga"
	"auth-service/internal/model"
	"auth-service/internal/service/fga/mocks"
	"auth-service/internal/testfga"
	"auth-service/pkg/audit/testaudit"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSpaceSyncGuards(t *testing.T) {
	t.Parallel()
	api := testfga.New(t, fgamodel.Strict)
	owner, space := uuid.New(), uuid.New()
	object := "space:" + space.String()
	testfga.Write(t, api, sdk.ClientTupleKey{Object: object, User: "user:" + owner.String(), Relation: "owner"}, sdk.ClientTupleKey{Object: object, User: "user:*", Relation: "personal"})

	for _, tt := range []struct {
		name      string
		change    model.ChangeType
		operation model.Operation
		kind      any
		members   []model.Member
		owner     uuid.UUID
		want      error
	}{
		{"personal delete", model.ChangeTypeResourceRemoved, model.OperationDelete, nil, nil, owner, ErrSpacePrivacy},
		{"type flip", model.ChangeTypeResourceAdded, model.OperationCreate, "shared", nil, owner, ErrSpacePrivacy},
		{"same create", model.ChangeTypeResourceAdded, model.OperationCreate, "personal", nil, owner, ErrResourceAlreadyExistsOrNotFound},
		{"personal add", model.ChangeTypeMembershipAdded, model.OperationCreate, nil, []model.Member{{UserID: uuid.New(), NewRole: "viewer"}}, owner, ErrSpacePrivacy},
		{"personal edit", model.ChangeTypeMembershipChanged, model.OperationUpdate, nil, []model.Member{{UserID: uuid.New(), NewRole: "admin"}}, owner, ErrSpacePrivacy},
		{"personal remove", model.ChangeTypeMembershipRemoved, model.OperationDelete, nil, []model.Member{{UserID: uuid.New()}}, owner, ErrSpacePrivacy},
		{"unknown type", model.ChangeTypeResourceAdded, model.OperationCreate, "unknown", nil, owner, ErrSpacePrivacy},
		{"missing type", model.ChangeTypeResourceAdded, model.OperationCreate, nil, nil, owner, ErrSpacePrivacy},
		{"wrong owner", model.ChangeTypeResourceAdded, model.OperationCreate, "personal", nil, uuid.New(), ErrSpacePrivacy},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := mocks.NewMockuserRepo(gomock.NewController(t))
			repo.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(owner, nil)
			c := &Client{fgaClient: api, userRepo: repo, auditor: testaudit.NewAuditor(t)}
			postfix := map[model.Operation]string{model.OperationCreate: "CREATED", model.OperationUpdate: "UPDATED", model.OperationDelete: "DELETED"}[tt.operation]

			attributes := map[string]any{}
			if tt.kind != nil {
				attributes["space_type"] = tt.kind
			}

			req := model.UpdateResourceRequest{RequestID: uuid.New(), DecisionID: uuid.New(), IdempotencyKey: uuid.NewString(), TelegramID: 42, Resource: model.Resource{ID: space, Type: model.ResourceTypeSpace, Attributes: attributes}, Relations: model.Relation{Owner: model.Resource{ID: tt.owner, Type: model.ResourceTypeUser}, Members: tt.members}, Operation: tt.operation, ChangeType: tt.change, Context: model.Context{EventType: model.FormatEventType("SPACE", postfix)}}
			_, err := c.UpdateResource(t.Context(), req)
			require.ErrorIs(t, err, tt.want)
			tuples, err := ReadTuples(t.Context(), api, object)
			require.NoError(t, err)
			require.Len(t, tuples, 2)
		})
	}
}

func TestSpaceCreateMapper(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"personal", "shared", "bad", ""} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			owner, space := uuid.New(), uuid.New()
			req := model.UpdateResourceRequest{Resource: model.Resource{ID: space, Type: model.ResourceTypeSpace, Attributes: map[string]any{"space_type": kind}}, Relations: model.Relation{Owner: model.Resource{ID: owner, Type: model.ResourceTypeUser}}, Operation: model.OperationCreate, ChangeType: model.ChangeTypeResourceAdded}

			body, err := toOpenFGAWriteRequest(req)
			if kind == "bad" || kind == "" {
				require.ErrorIs(t, err, ErrSpacePrivacy)
				return
			}

			require.NoError(t, err)
			require.ElementsMatch(t, []sdk.ClientTupleKey{{Object: "space:" + space.String(), Relation: "owner", User: "user:" + owner.String()}, {Object: "space:" + space.String(), Relation: kind, User: "user:*"}}, body.Writes)
			require.Empty(t, body.Deletes)
		})
	}
}

func TestStoredSpaceIdentity(t *testing.T) {
	t.Parallel()

	owner := uuid.New()
	makeTuple := func(relation, user string) openfga.Tuple {
		return openfga.Tuple{Key: openfga.TupleKey{Relation: relation, User: user, Object: "space:s"}}
	}
	base := makeTuple("owner", "user:"+owner.String())

	marker := makeTuple("shared", "user:*")
	for _, tt := range []struct {
		name   string
		tuples []openfga.Tuple
		valid  bool
	}{
		{"shared", []openfga.Tuple{base, marker, makeTuple("viewer", "user:viewer")}, true},
		{"missing", nil, false}, {"no marker", []openfga.Tuple{base}, false},
		{"two markers", []openfga.Tuple{base, marker, makeTuple("personal", "user:*")}, false},
		{"invalid wildcard", []openfga.Tuple{base, makeTuple("personal", "user:other")}, false},
		{"two owners", []openfga.Tuple{base, base, marker}, false},
		{"invalid owner", []openfga.Tuple{makeTuple("owner", "user:bad"), marker}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			kind, id, err := spaceIdentity(tt.tuples)
			if !tt.valid {
				require.ErrorIs(t, err, ErrSpacePrivacy)
				return
			}

			require.NoError(t, err)
			require.Equal(t, "shared", kind)
			require.Equal(t, owner.String(), id)
		})
	}
}

func TestSharedOwnerMembershipGuard(t *testing.T) {
	t.Parallel()

	owner := uuid.New()
	for _, tt := range []struct {
		name      string
		member    model.Member
		forbidden bool
	}{
		{"owner target", model.Member{UserID: owner, NewRole: "viewer"}, true},
		{"assign owner", model.Member{UserID: uuid.New(), NewRole: "owner"}, true},
		{"remove owner role", model.Member{UserID: uuid.New(), OldRole: "owner"}, true},
		{"empty target", model.Member{}, true},
		{"admin can demote self", model.Member{UserID: uuid.New(), OldRole: "admin", NewRole: "viewer"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := guardMembers(model.UpdateResourceRequest{Relations: model.Relation{Members: []model.Member{tt.member}}}, owner.String())
			if tt.forbidden {
				require.ErrorIs(t, err, ErrSpacePrivacy)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
