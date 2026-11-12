package decision

import (
	"testing"

	"auth-service/internal/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func decisionFixture() model.CheckDecisionRequest {
	return model.CheckDecisionRequest{
		RequestID: uuid.New(),
		Subject:   model.DecisionSubject{Type: model.ResourceTypeUser, ID: uuid.New()},
		Resource:  model.DecisionResource{Type: model.ResourceTypeNote, ID: uuid.New()},
		Action: model.DecisionAction{
			Name:       model.ActionNameUpdate,
			Attributes: model.DecisionActionAttributes{FieldMask: []string{"text", "title"}},
		},
	}
}

func wantInvalidRequest(message string) require.ErrorAssertionFunc {
	return func(t require.TestingT, err error, _ ...any) {
		require.ErrorIs(t, err, model.ErrInvalidDecisionRequest)
		require.ErrorContains(t, err, message)
	}
}

func TestValidateRequest(t *testing.T) {
	t.Parallel()

	note := model.DecisionResource{ID: uuid.New(), Type: model.ResourceTypeNote}
	subject := model.DecisionSubject{ID: uuid.New(), Type: model.ResourceTypeUser}
	request := func(action model.ActionName) model.CheckDecisionRequest {
		return model.CheckDecisionRequest{RequestID: uuid.New(), Subject: subject, Action: model.DecisionAction{Name: action}, Resource: note}
	}

	for _, tc := range []struct {
		name             string
		req              model.CheckDecisionRequest
		object, relation string
		wantErr          require.ErrorAssertionFunc
	}{
		{"valid request", request(model.ActionNameRead), "note:" + note.ID.String(), "can_view", require.NoError},
		{"invalid DTO", model.CheckDecisionRequest{}, "", "", wantInvalidRequest("validate input")},
		{"invalid action attributes", request(model.ActionNameUpdate), "", "", wantInvalidRequest("validate action attributes")},
		{"missing parent space", request(model.ActionNameCreate), "", "", wantInvalidRequest("validate resource attributes")},
		{"unsupported action", request(model.ActionNameStop), "", "", wantInvalidRequest("resolve authorization target")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			object, relation, err := validateRequest(tc.req)
			tc.wantErr(t, err)

			require.Equal(t, tc.object, object)
			require.Equal(t, tc.relation, relation)
		})
	}
}

func TestResolveTarget(t *testing.T) {
	t.Parallel()

	subject := model.DecisionSubject{ID: uuid.New(), Type: model.ResourceTypeUser}
	parent, resourceID := uuid.New(), uuid.New()
	note := model.DecisionResource{ID: resourceID, Type: model.ResourceTypeNote}
	reminder := model.DecisionResource{ID: resourceID, Type: model.ResourceTypeReminder}
	space := model.DecisionResource{ID: resourceID, Type: model.ResourceTypeSpace}
	member := model.DecisionResource{ID: resourceID, Type: model.ResourceTypeMember, Attributes: model.DecisionResourceAttributes{SpaceID: &parent}}
	withParent := func(resource model.DecisionResource) model.DecisionResource {
		resource.Attributes.SpaceID = &parent
		return resource
	}
	spaceOfType := func(kind model.SpaceKind) model.DecisionResource {
		return model.DecisionResource{ID: resourceID, Type: model.ResourceTypeSpace, Attributes: model.DecisionResourceAttributes{SpaceType: &kind}}
	}
	request := func(action model.ActionName, resource model.DecisionResource) model.CheckDecisionRequest {
		return model.CheckDecisionRequest{Subject: subject, Action: model.DecisionAction{Name: action}, Resource: resource}
	}
	noteObject, reminderObject := "note:"+resourceID.String(), "reminder:"+resourceID.String()
	spaceObject, parentObject := "space:"+resourceID.String(), "space:"+parent.String()
	unsupported := wantInvalidRequest("action.name is not supported for resource.type")

	for _, tc := range []struct {
		name             string
		req              model.CheckDecisionRequest
		object, relation string
		wantErr          require.ErrorAssertionFunc
	}{
		{"shared creation", request(model.ActionNameCreate, spaceOfType(model.SpaceKindShared)), "", "", require.NoError},
		{"personal creation", request(model.ActionNameCreate, spaceOfType(model.SpaceKindPersonal)), "user:" + subject.ID.String(), model.RelationHasPersonalSpace, require.NoError},
		{"note creation", request(model.ActionNameCreate, withParent(note)), parentObject, "can_create_note", require.NoError},
		{"reminder creation", request(model.ActionNameCreate, withParent(reminder)), parentObject, "can_create_reminder", require.NoError},
		{"read note with parent", request(model.ActionNameRead, withParent(note)), noteObject, "can_view", require.NoError},
		{"read reminder", request(model.ActionNameRead, reminder), reminderObject, "can_view", require.NoError},
		{"update note", request(model.ActionNameUpdate, note), noteObject, "can_update", require.NoError},
		{"update reminder", request(model.ActionNameUpdate, reminder), reminderObject, "can_update", require.NoError},
		{"delete note", request(model.ActionNameDelete, note), noteObject, "can_delete", require.NoError},
		{"delete reminder", request(model.ActionNameDelete, reminder), reminderObject, "can_delete", require.NoError},
		{"stop reminder", request(model.ActionNameStop, reminder), reminderObject, "can_stop", require.NoError},
		{"read space", request(model.ActionNameRead, space), spaceObject, "can_view", require.NoError},
		{"update space", request(model.ActionNameUpdate, space), spaceObject, "can_manage_space", require.NoError},
		{"delete space", request(model.ActionNameDelete, space), spaceObject, "can_delete", require.NoError},
		{"add member", request(model.ActionNameAdd, member), parentObject, "can_manage_members", require.NoError},
		{"edit member", request(model.ActionNameEdit, member), parentObject, "can_manage_members", require.NoError},
		{"remove member", request(model.ActionNameRemove, member), parentObject, "can_manage_members", require.NoError},
		{"stop note", request(model.ActionNameStop, note), "", "", unsupported},
		{"stop space", request(model.ActionNameStop, space), "", "", unsupported},
		{"read member", request(model.ActionNameRead, member), "", "", unsupported},
		{"unsupported resource", request(model.ActionNameRead, model.DecisionResource{ID: resourceID, Type: model.ResourceTypeUser}), "", "", unsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			object, relation, err := resolveTarget(tc.req)
			tc.wantErr(t, err)

			require.Equal(t, tc.object, object)
			require.Equal(t, tc.relation, relation)
		})
	}
}

func TestValidateRequestedSpaceType(t *testing.T) {
	t.Parallel()

	request := func(resource model.ResourceType, action model.ActionName, kind *model.SpaceKind) model.CheckDecisionRequest {
		return model.CheckDecisionRequest{Action: model.DecisionAction{Name: action}, Resource: model.DecisionResource{Type: resource, Attributes: model.DecisionResourceAttributes{SpaceType: kind}}}
	}
	restricted := wantInvalidRequest("space_type is only supported for space:create")

	invalid := wantInvalidRequest("space_type must be shared or personal")
	for _, tc := range []struct {
		name    string
		req     model.CheckDecisionRequest
		wantErr require.ErrorAssertionFunc
	}{
		{"shared", request(model.ResourceTypeSpace, model.ActionNameCreate, new(model.SpaceKindShared)), require.NoError},
		{"personal", request(model.ResourceTypeSpace, model.ActionNameCreate, new(model.SpaceKindPersonal)), require.NoError},
		{"missing", request(model.ResourceTypeSpace, model.ActionNameCreate, nil), wantInvalidRequest("space_type is required for space:create")},
		{"unknown", request(model.ResourceTypeSpace, model.ActionNameCreate, new(model.SpaceKind("group"))), invalid},
		{"empty", request(model.ResourceTypeSpace, model.ActionNameCreate, new(model.SpaceKind(""))), invalid},
		{"existing space without type", request(model.ResourceTypeSpace, model.ActionNameDelete, nil), require.NoError},
		{"existing space with type", request(model.ResourceTypeSpace, model.ActionNameDelete, new(model.SpaceKindShared)), restricted},
		{"note creation with space type", request(model.ResourceTypeNote, model.ActionNameCreate, new(model.SpaceKindPersonal)), restricted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateRequestedSpaceType(tc.req)
			tc.wantErr(t, err)
		})
	}
}

func TestValidateResourceAttributes(t *testing.T) {
	t.Parallel()

	parent := uuid.New()
	request := func(resource model.ResourceType, action model.ActionName, id *uuid.UUID) model.CheckDecisionRequest {
		return model.CheckDecisionRequest{Action: model.DecisionAction{Name: action}, Resource: model.DecisionResource{Type: resource, Attributes: model.DecisionResourceAttributes{SpaceID: id}}}
	}
	missing := wantInvalidRequest("space_id is required")

	invalid := wantInvalidRequest("space_id must be a nonzero UUID")
	for _, tc := range []struct {
		name    string
		req     model.CheckDecisionRequest
		wantErr require.ErrorAssertionFunc
	}{
		{"read without optional parent", request(model.ResourceTypeNote, model.ActionNameRead, nil), require.NoError},
		{"read with optional parent", request(model.ResourceTypeNote, model.ActionNameRead, &parent), require.NoError},
		{"create note with parent", request(model.ResourceTypeNote, model.ActionNameCreate, &parent), require.NoError},
		{"create reminder with parent", request(model.ResourceTypeReminder, model.ActionNameCreate, &parent), require.NoError},
		{"member target with parent", request(model.ResourceTypeMember, model.ActionNameAdd, &parent), require.NoError},
		{"create note missing parent", request(model.ResourceTypeNote, model.ActionNameCreate, nil), missing},
		{"create reminder missing parent", request(model.ResourceTypeReminder, model.ActionNameCreate, nil), missing},
		{"member missing parent", request(model.ResourceTypeMember, model.ActionNameEdit, nil), missing},
		{"zero parent", request(model.ResourceTypeNote, model.ActionNameCreate, &uuid.Nil), invalid},
		{"parent on space", request(model.ResourceTypeSpace, model.ActionNameRead, &parent), invalid},
		{"space type validation failure", request(model.ResourceTypeSpace, model.ActionNameCreate, nil), wantInvalidRequest("space_type is required")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateResourceAttributes(tc.req)
			tc.wantErr(t, err)
		})
	}
}

func TestValidateActionAttributes(t *testing.T) {
	t.Parallel()

	request := func(resource model.ResourceType, action model.ActionName, fields []string, role *string) model.CheckDecisionRequest {
		return model.CheckDecisionRequest{Resource: model.DecisionResource{Type: resource}, Action: model.DecisionAction{Name: action, Attributes: model.DecisionActionAttributes{FieldMask: fields, Role: role}}}
	}
	mask := []string{"title"}
	member, note := model.ResourceTypeMember, model.ResourceTypeNote
	needsMask := wantInvalidRequest("field_mask must be nonempty for update")
	unexpectedMask := wantInvalidRequest("field_mask is only supported for update")
	invalidRole := wantInvalidRequest("role must be viewer, editor or admin")
	unexpectedRole := wantInvalidRequest("role is only supported for member add/edit")

	for _, tc := range []struct {
		name    string
		req     model.CheckDecisionRequest
		wantErr require.ErrorAssertionFunc
	}{
		{"update with mask", request(note, model.ActionNameUpdate, mask, nil), require.NoError},
		{"update without mask", request(note, model.ActionNameUpdate, nil, nil), needsMask},
		{"update with empty mask", request(note, model.ActionNameUpdate, []string{}, nil), needsMask},
		{"read without attributes", request(note, model.ActionNameRead, nil, nil), require.NoError},
		{"read with mask", request(note, model.ActionNameRead, mask, nil), unexpectedMask},
		{"create with empty mask", request(note, model.ActionNameCreate, []string{}, nil), unexpectedMask},
		{"update with role", request(note, model.ActionNameUpdate, mask, new("viewer")), unexpectedRole},
		{"role on note creation", request(note, model.ActionNameCreate, nil, new("viewer")), unexpectedRole},
		{"member add viewer", request(member, model.ActionNameAdd, nil, new("viewer")), require.NoError},
		{"member add editor", request(member, model.ActionNameAdd, nil, new("editor")), require.NoError},
		{"member add admin", request(member, model.ActionNameAdd, nil, new("admin")), require.NoError},
		{"member edit viewer", request(member, model.ActionNameEdit, nil, new("viewer")), require.NoError},
		{"member edit editor", request(member, model.ActionNameEdit, nil, new("editor")), require.NoError},
		{"member edit admin", request(member, model.ActionNameEdit, nil, new("admin")), require.NoError},
		{"member add without role", request(member, model.ActionNameAdd, nil, nil), invalidRole},
		{"member edit without role", request(member, model.ActionNameEdit, nil, nil), invalidRole},
		{"member add owner", request(member, model.ActionNameAdd, nil, new("owner")), invalidRole},
		{"member edit owner", request(member, model.ActionNameEdit, nil, new("owner")), invalidRole},
		{"empty role", request(member, model.ActionNameEdit, nil, new("")), invalidRole},
		{"unknown role", request(member, model.ActionNameAdd, nil, new("reader")), invalidRole},
		{"mask on member edit", request(member, model.ActionNameEdit, mask, new("viewer")), unexpectedMask},
		{"member remove without role", request(member, model.ActionNameRemove, nil, nil), require.NoError},
		{"member remove with role", request(member, model.ActionNameRemove, nil, new("viewer")), unexpectedRole},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateActionAttributes(tc.req)
			tc.wantErr(t, err)
		})
	}
}

func TestSpaceRelation(t *testing.T) {
	t.Parallel()

	for action, want := range map[model.ActionName]string{model.ActionNameRead: "can_view", model.ActionNameUpdate: "can_manage_space", model.ActionNameDelete: "can_delete", model.ActionNameStop: ""} {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			require.Equal(t, want, spaceRelation(action))
		})
	}
}

func TestRecordRelation(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		kind   model.ResourceType
		action model.ActionName
		want   string
	}{
		{model.ResourceTypeNote, model.ActionNameCreate, "can_create_note"},
		{model.ResourceTypeReminder, model.ActionNameCreate, "can_create_reminder"},
		{model.ResourceTypeNote, model.ActionNameRead, "can_view"},
		{model.ResourceTypeNote, model.ActionNameUpdate, "can_update"},
		{model.ResourceTypeNote, model.ActionNameDelete, "can_delete"},
		{model.ResourceTypeReminder, model.ActionNameStop, "can_stop"},
		{model.ResourceTypeNote, model.ActionNameStop, ""},
	} {
		t.Run(string(tt.kind)+"/"+string(tt.action), func(t *testing.T) {
			t.Parallel()

			r := model.CheckDecisionRequest{Resource: model.DecisionResource{Type: tt.kind}, Action: model.DecisionAction{Name: tt.action}}

			require.Equal(t, tt.want, recordRelation(r))
		})
	}
}
