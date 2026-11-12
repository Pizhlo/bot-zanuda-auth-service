package fga

import (
	"context"
	"testing"
	"time"

	"auth-service/internal/model"
	"auth-service/internal/service/fga/mocks"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestSpaceIdentity(t *testing.T) {
	t.Parallel()

	owner := uuid.New()
	makeTuple := func(relation, user string) openfga.Tuple {
		return openfga.Tuple{Key: openfga.TupleKey{Relation: relation, User: user, Object: "space:s"}}
	}
	base := makeTuple("owner", "user:"+owner.String())

	marker := makeTuple("shared", "user:*")
	wantOwner := model.Resource{Type: model.ResourceTypeUser, ID: owner}

	for _, tt := range []struct {
		name      string
		tuples    []openfga.Tuple
		wantErr   error
		wantKind  model.SpaceKind
		wantOwner model.Resource
	}{
		{"shared", []openfga.Tuple{base, marker, makeTuple("viewer", "user:viewer")}, nil, model.SpaceKindShared, wantOwner},
		{"missing", nil, ErrSpacePrivacy, "", model.Resource{}}, {"no marker", []openfga.Tuple{base}, ErrSpacePrivacy, "", model.Resource{}},
		{"two markers", []openfga.Tuple{base, marker, makeTuple("personal", "user:*")}, ErrSpacePrivacy, "", model.Resource{}},
		{"invalid wildcard", []openfga.Tuple{base, makeTuple("personal", "user:other")}, ErrSpacePrivacy, "", model.Resource{}},
		{"two owners", []openfga.Tuple{base, base, marker}, ErrSpacePrivacy, "", model.Resource{}},
		{"invalid owner", []openfga.Tuple{makeTuple("owner", "user:bad"), marker}, ErrSpacePrivacy, "", model.Resource{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			kind, id, err := spaceIdentity(tt.tuples)
			require.ErrorIs(t, err, tt.wantErr)

			require.Equal(t, tt.wantKind, kind)
			require.Equal(t, tt.wantOwner, id)
		})
	}
}

func TestGuardMembers(t *testing.T) {
	t.Parallel()

	owner := uuid.New()
	for _, tt := range []struct {
		name    string
		member  model.Member
		wantErr error
	}{
		{"owner target", model.Member{UserID: owner, NewRole: "viewer"}, ErrSpacePrivacy},
		{"assign legacy owner", model.Member{UserID: uuid.New(), NewRole: model.OwnerRoleCode}, ErrSpacePrivacy},
		{"remove legacy owner", model.Member{UserID: uuid.New(), OldRole: model.OwnerRoleCode}, ErrSpacePrivacy},
		{"assign owner", model.Member{UserID: uuid.New(), NewRole: "owner"}, ErrSpacePrivacy},
		{"remove owner role", model.Member{UserID: uuid.New(), OldRole: "owner"}, ErrSpacePrivacy},
		{"empty target", model.Member{}, ErrSpacePrivacy},
		{"admin can demote self", model.Member{UserID: uuid.New(), OldRole: "admin", NewRole: "viewer"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := guardMembers(model.UpdateResourceRequest{Relations: model.Relation{Members: []model.Member{tt.member}}}, model.Resource{
				Type: model.ResourceTypeUser,
				ID:   owner,
			})
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestValidateStoredSpace(t *testing.T) {
	t.Parallel()

	owner := model.Resource{Type: model.ResourceTypeUser, ID: uuid.New()}
	other := model.Resource{Type: model.ResourceTypeUser, ID: uuid.New()}
	request := func(change model.ChangeType, requestedOwner model.Resource, attrs map[string]any, members ...model.Member) model.UpdateResourceRequest {
		return model.UpdateResourceRequest{Resource: model.Resource{Type: model.ResourceTypeSpace, Attributes: attrs}, Relations: model.Relation{Owner: requestedOwner, Members: members}, ChangeType: change}
	}

	for _, tc := range []struct {
		name    string
		kind    model.SpaceKind
		req     model.UpdateResourceRequest
		wantErr error
	}{
		{"shared delete without type", model.SpaceKindShared, request(model.ChangeTypeResourceRemoved, owner, nil), nil},
		{"personal delete", model.SpaceKindPersonal, request(model.ChangeTypeResourceRemoved, owner, nil), ErrSpacePrivacy},
		{"owner substitution", model.SpaceKindShared, request(model.ChangeTypeResourceRemoved, other, nil), ErrSpacePrivacy},
		{"type substitution", model.SpaceKindShared, request(model.ChangeTypeResourceRemoved, owner, map[string]any{model.AttributeSpaceType: "personal"}), ErrSpacePrivacy},
		{"same type", model.SpaceKindShared, request(model.ChangeTypeResourceRemoved, owner, map[string]any{model.AttributeSpaceType: "shared"}), nil},
		{"duplicate create", model.SpaceKindShared, request(model.ChangeTypeResourceAdded, owner, nil), ErrResourceAlreadyExistsOrNotFound},
		{"duplicate UUID with another owner and type", model.SpaceKindShared, request(model.ChangeTypeResourceAdded, other, map[string]any{model.AttributeSpaceType: "personal"}), ErrResourceAlreadyExistsOrNotFound},
		{"owner is protected", model.SpaceKindShared, request(model.ChangeTypeMembershipAdded, owner, nil, model.Member{UserID: owner.ID}), ErrSpacePrivacy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateStoredSpace(tc.req, tc.kind, owner)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestReadTuples(t *testing.T) {
	t.Parallel()

	first := openfga.Tuple{Key: openfga.TupleKey{Object: "space:s", Relation: "owner", User: "user:u"}}
	second := openfga.Tuple{Key: openfga.TupleKey{Object: "space:s", Relation: "created", User: "user:*"}}
	page := func(next string, tuples ...openfga.Tuple) tupleReadStub {
		return tupleReadStub{response: &sdk.ClientReadResponse{Tuples: tuples, ContinuationToken: next}}
	}
	wantError := func(text string) require.ErrorAssertionFunc {
		return func(t require.TestingT, err error, _ ...any) { require.ErrorContains(t, err, text) }
	}
	noReads := func(*mocks.MockSdkClient) []*tupleReadStub { return nil }
	objectFilters := map[string]sdk.ClientReadRequest{"": {}, "space:s": {Object: new("space:s")}}

	for _, tc := range []struct {
		name, object string
		timeout      time.Duration
		expect       func(*mocks.MockSdkClient) []*tupleReadStub
		tokens       []string
		want         []openfga.Tuple
		wantErr      require.ErrorAssertionFunc
	}{
		{"zero timeout", "", 0, noReads, nil, nil, wantError("timeout must be positive")},
		{"negative timeout", "", -time.Second, noReads, nil, nil, wantError("timeout must be positive")},
		{"empty store", "", time.Second, expectTuplePages(page("")), []string{""}, nil, require.NoError},
		{"all pages", "space:s", time.Second, expectTuplePages(page("p2", first), page("", second)), []string{"", "p2"}, []openfga.Tuple{first, second}, require.NoError},
		{"absent response", "", time.Second, expectTuplePages(tupleReadStub{}), []string{""}, nil, wantError("empty response")},
		{"second page fails", "space:s", time.Second, expectTuplePages(page("p2", first), tupleReadStub{err: context.DeadlineExceeded}), []string{"", "p2"}, nil, func(t require.TestingT, err error, _ ...any) {
			require.ErrorIs(t, err, context.DeadlineExceeded)
		}},
		{"repeated token", "", time.Second, expectTuplePages(page("p2", first), page("p3"), page("p2")), []string{"", "p2", "p3"}, nil, wantError("repeated continuation token")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := mocks.NewMockSdkClient(gomock.NewController(t))
			requests := tc.expect(api)

			tuples, err := ReadTuples(t.Context(), api, tc.object, tc.timeout)
			tc.wantErr(t, err)

			require.Equal(t, tc.want, tuples)
			require.Len(t, requests, len(tc.tokens))

			for i, request := range requests {
				require.Equal(t, objectFilters[tc.object], request.body)
				require.Equal(t, tc.tokens[i], request.token)
				require.Equal(t, openfga.CONSISTENCYPREFERENCE_HIGHER_CONSISTENCY, request.preference)
			}
		})
	}
}

func expectTuplePages(pages ...tupleReadStub) func(*mocks.MockSdkClient) []*tupleReadStub {
	return func(api *mocks.MockSdkClient) []*tupleReadStub {
		requests := make([]*tupleReadStub, 0, len(pages))
		calls := make([]*gomock.Call, 0, len(pages))

		for i := range pages {
			request := &pages[i]
			requests = append(requests, request)
			calls = append(calls, api.EXPECT().Read(gomock.Any()).Return(request))
		}

		gomock.InOrder(calls...)

		return requests
	}
}

type tupleReadStub struct {
	sdk.SdkClientReadRequestInterface
	response   *sdk.ClientReadResponse
	err        error
	body       sdk.ClientReadRequest
	token      string
	preference openfga.ConsistencyPreference
}

func (s *tupleReadStub) Body(body sdk.ClientReadRequest) sdk.SdkClientReadRequestInterface {
	s.body = body
	return s
}

func (s *tupleReadStub) Options(opts sdk.ClientReadOptions) sdk.SdkClientReadRequestInterface {
	s.token = *opts.ContinuationToken
	s.preference = *opts.Consistency

	return s
}

func (s *tupleReadStub) Execute() (*sdk.ClientReadResponse, error) { return s.response, s.err }

func TestGuardSpace(t *testing.T) {
	t.Parallel()

	owner := model.Resource{Type: model.ResourceTypeUser, ID: uuid.New()}
	shared := map[string]any{model.AttributeSpaceType: "shared"}
	saved := []openfga.Tuple{
		{Key: openfga.TupleKey{Relation: "owner", User: "user:" + owner.ID.String()}},
		{Key: openfga.TupleKey{Relation: "shared", User: "user:*"}},
	}
	request := func(change model.ChangeType, requestedOwner model.Resource, attrs map[string]any) model.UpdateResourceRequest {
		return model.UpdateResourceRequest{Resource: model.Resource{ID: uuid.New(), Type: model.ResourceTypeSpace, Attributes: attrs}, Relations: model.Relation{Owner: requestedOwner}, ChangeType: change}
	}
	read := func(tuples []openfga.Tuple, err error) func(*mocks.MockSdkClient) {
		return func(api *mocks.MockSdkClient) {
			api.EXPECT().Read(gomock.Any()).Return(&tupleReadStub{response: &sdk.ClientReadResponse{Tuples: tuples}, err: err})
		}
	}

	noRead := func(*mocks.MockSdkClient) {}
	for _, tc := range []struct {
		name    string
		req     model.UpdateResourceRequest
		prepare func(*mocks.MockSdkClient)
		want    error
	}{
		{"new space", request(model.ChangeTypeResourceAdded, owner, shared), read(nil, nil), nil},
		{"duplicate UUID", request(model.ChangeTypeResourceAdded, owner, shared), read(saved, nil), ErrResourceAlreadyExistsOrNotFound},
		{"read fails", request(model.ChangeTypeResourceAdded, owner, shared), read(nil, context.DeadlineExceeded), context.DeadlineExceeded},
		{"shared operation", request(model.ChangeTypeResourceRemoved, owner, nil), read(saved, nil), nil},
		{"missing stored metadata", request(model.ChangeTypeResourceRemoved, owner, nil), read(nil, nil), ErrSpacePrivacy},
		{"cannot add a member to a missing space", request(model.ChangeTypeMembershipAdded, owner, nil), read(nil, nil), ErrSpacePrivacy},
		{"invalid owner type", request(model.ChangeTypeResourceAdded, model.Resource{Type: model.ResourceTypeSpace, ID: owner.ID}, shared), noRead, ErrOwnerTypeInvalid},
		{"invalid kind", request(model.ChangeTypeResourceAdded, owner, map[string]any{model.AttributeSpaceType: "other"}), noRead, ErrSpacePrivacy},
		{"different owner", request(model.ChangeTypeResourceAdded, model.Resource{Type: model.ResourceTypeUser, ID: uuid.New()}, shared), noRead, ErrSpacePrivacy},
		{"record bypasses space guard", model.UpdateResourceRequest{Resource: model.Resource{Type: model.ResourceTypeNote}}, noRead, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := mocks.NewMockSdkClient(gomock.NewController(t))
			tc.prepare(api)
			client := &Client{fgaClient: api, requestTimeout: time.Second}

			err := client.guardSpace(t.Context(), tc.req, owner.ID)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestOwnerResource(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	for _, tc := range []struct {
		name, user string
		want       model.Resource
		wantErr    error
	}{
		{"user owner", "user:" + id.String(), model.Resource{Type: model.ResourceTypeUser, ID: id}, nil},
		{"non-user owner", "space:" + id.String(), model.Resource{}, ErrSpacePrivacy},
		{"malformed ID", "user:bad", model.Resource{}, ErrSpacePrivacy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ownerResource(tc.user)
			require.ErrorIs(t, err, tc.wantErr)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestValidateSpaceIdentity(t *testing.T) {
	t.Parallel()

	actor := uuid.New()
	owner := model.Resource{Type: model.ResourceTypeUser, ID: actor}
	personal := map[string]any{model.AttributeSpaceType: "personal"}

	request := func(change model.ChangeType, requestedOwner model.Resource, attrs map[string]any, members ...model.Member) model.UpdateResourceRequest {
		return model.UpdateResourceRequest{ChangeType: change, Resource: model.Resource{Type: model.ResourceTypeSpace, Attributes: attrs}, Relations: model.Relation{Owner: requestedOwner, Members: members}}
	}
	for _, tc := range []struct {
		name  string
		req   model.UpdateResourceRequest
		actor uuid.UUID
		want  error
	}{
		{"personal by owner", request(model.ChangeTypeResourceAdded, owner, personal), actor, nil},
		{"different actor", request(model.ChangeTypeResourceAdded, owner, personal), uuid.New(), ErrSpacePrivacy},
		{"missing actor", request(model.ChangeTypeResourceAdded, model.Resource{Type: model.ResourceTypeUser}, personal), uuid.Nil, ErrSpacePrivacy},
		{"create requires type", request(model.ChangeTypeResourceAdded, owner, nil), actor, ErrSpacePrivacy},
		{"wrong owner type", request(model.ChangeTypeResourceAdded, model.Resource{Type: model.ResourceTypeSpace, ID: actor}, personal), actor, ErrOwnerTypeInvalid},
		{"invalid type name", request(model.ChangeTypeResourceAdded, owner, map[string]any{model.AttributeSpaceType: "other"}), actor, ErrSpacePrivacy},
		{"invalid type encoding", request(model.ChangeTypeResourceAdded, owner, map[string]any{model.AttributeSpaceType: 12}), actor, ErrSpacePrivacy},
		{"create rejects members", request(model.ChangeTypeResourceAdded, owner, personal, model.Member{UserID: uuid.New()}), actor, ErrSpacePrivacy},
		{"non-create may omit type", request(model.ChangeTypeMembershipAdded, owner, nil), actor, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateSpaceIdentity(tc.req, tc.actor)
			require.ErrorIs(t, err, tc.want)
		})
	}
}
