//nolint:funlen,dupl // тесты
package fga

import (
	"testing"

	"auth-service/internal/model"

	"github.com/google/uuid"
	openFGAClient "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestToOpenFGAWriteRequest_CreateNote(t *testing.T) {
	t.Parallel()

	noteID := uuid.New()
	spaceID := uuid.New()
	ownerID := uuid.New()

	tests := []struct {
		name    string
		req     model.UpdateResourceRequest
		want    openFGAClient.ClientWriteRequest
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "create note",
			req: model.UpdateResourceRequest{
				Resource:  model.Resource{ID: noteID, Type: model.ResourceTypeNote},
				Operation: model.OperationCreate,
				Relations: model.Relation{
					Owner:  model.Resource{ID: ownerID, Type: model.ResourceTypeUser},
					Parent: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
				},
			},
			want: openFGAClient.ClientWriteRequest{
				Writes: []openFGAClient.ClientTupleKey{
					{
						User:     formatObject(model.Resource{ID: ownerID, Type: model.ResourceTypeUser}),
						Relation: "owner",
						Object:   formatObject(model.Resource{ID: noteID, Type: model.ResourceTypeNote}),
					},
					{
						User:     formatObject(model.Resource{ID: spaceID, Type: model.ResourceTypeSpace}),
						Relation: "space",
						Object:   formatObject(model.Resource{ID: noteID, Type: model.ResourceTypeNote}),
					},
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "no tuples to write or delete",
			req: model.UpdateResourceRequest{
				Resource:  model.Resource{ID: noteID, Type: model.ResourceTypeNote},
				Operation: model.OperationCreate,
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "no tuples to write or delete")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fgaReq, err := toOpenFGAWriteRequest(tt.req)
			tt.wantErr(t, err)
			require.Equal(t, tt.want, fgaReq)
		})
	}
}

func TestTuplesFromRequest(t *testing.T) {
	t.Parallel()

	noteID := uuid.New()
	spaceID := uuid.New()
	ownerID := uuid.New()

	tests := []struct {
		name        string
		req         model.UpdateResourceRequest
		wantWrites  []openFGAClient.ClientTupleKey
		wantDeletes []openFGAClient.ClientTupleKeyWithoutCondition
		wantErr     require.ErrorAssertionFunc
	}{
		{
			name: "create note",
			req: model.UpdateResourceRequest{
				Resource:  model.Resource{ID: noteID, Type: model.ResourceTypeNote},
				Operation: model.OperationCreate,
				Relations: model.Relation{
					Owner:  model.Resource{ID: ownerID, Type: model.ResourceTypeUser},
					Parent: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
				},
			},
			wantWrites: []openFGAClient.ClientTupleKey{
				{
					User:     formatObject(model.Resource{ID: ownerID, Type: model.ResourceTypeUser}),
					Relation: "owner",
					Object:   formatObject(model.Resource{ID: noteID, Type: model.ResourceTypeNote}),
				},
				{
					User:     formatObject(model.Resource{ID: spaceID, Type: model.ResourceTypeSpace}),
					Relation: "space",
					Object:   formatObject(model.Resource{ID: noteID, Type: model.ResourceTypeNote}),
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "unknown operation",
			req: model.UpdateResourceRequest{
				Resource:  model.Resource{ID: noteID, Type: model.ResourceTypeNote},
				Operation: model.Operation("unknown"),
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "unknown operation")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			writes, deletes, err := tuplesFromRequest(tt.req)
			tt.wantErr(t, err)
			require.Equal(t, tt.wantWrites, writes)
			require.Equal(t, tt.wantDeletes, deletes)
		})
	}
}

func TestTuplesForCreate(t *testing.T) {
	t.Parallel()

	noteID := uuid.New()
	reminderID := uuid.New()
	spaceID := uuid.New()
	ownerID := uuid.New()

	tests := []struct {
		name        string
		resource    model.Resource
		relations   model.Relation
		wantWrites  []openFGAClient.ClientTupleKey
		wantDeletes []openFGAClient.ClientTupleKeyWithoutCondition
		wantErr     require.ErrorAssertionFunc
	}{
		{
			name:     "create note with owner and parent",
			resource: model.Resource{ID: noteID, Type: model.ResourceTypeNote},
			relations: model.Relation{
				Owner:  model.Resource{ID: ownerID, Type: model.ResourceTypeUser},
				Parent: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
			},
			wantWrites: []openFGAClient.ClientTupleKey{
				{
					User:     formatObject(model.Resource{ID: ownerID, Type: model.ResourceTypeUser}),
					Relation: "owner",
					Object:   formatObject(model.Resource{ID: noteID, Type: model.ResourceTypeNote}),
				},
				{
					User:     formatObject(model.Resource{ID: spaceID, Type: model.ResourceTypeSpace}),
					Relation: "space",
					Object:   formatObject(model.Resource{ID: noteID, Type: model.ResourceTypeNote}),
				},
			},
			wantErr: require.NoError,
		},
		{
			name:     "create note with owner only",
			resource: model.Resource{ID: noteID, Type: model.ResourceTypeNote},
			relations: model.Relation{
				Owner: model.Resource{ID: ownerID, Type: model.ResourceTypeUser},
			},
			wantWrites: []openFGAClient.ClientTupleKey{
				{
					User:     formatObject(model.Resource{ID: ownerID, Type: model.ResourceTypeUser}),
					Relation: "owner",
					Object:   formatObject(model.Resource{ID: noteID, Type: model.ResourceTypeNote}),
				},
			},
			wantErr: require.NoError,
		},
		{
			name:     "create note with parent only",
			resource: model.Resource{ID: noteID, Type: model.ResourceTypeNote},
			relations: model.Relation{
				Parent: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
			},
			wantWrites: []openFGAClient.ClientTupleKey{
				{
					User:     formatObject(model.Resource{ID: spaceID, Type: model.ResourceTypeSpace}),
					Relation: "space",
					Object:   formatObject(model.Resource{ID: noteID, Type: model.ResourceTypeNote}),
				},
			},
			wantErr: require.NoError,
		},
		{
			name:     "create reminder with owner and parent",
			resource: model.Resource{ID: reminderID, Type: model.ResourceTypeReminder},
			relations: model.Relation{
				Owner:  model.Resource{ID: ownerID, Type: model.ResourceTypeUser},
				Parent: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
			},
			wantWrites: []openFGAClient.ClientTupleKey{
				{
					User:     formatObject(model.Resource{ID: ownerID, Type: model.ResourceTypeUser}),
					Relation: "owner",
					Object:   formatObject(model.Resource{ID: reminderID, Type: model.ResourceTypeReminder}),
				},
				{
					User:     formatObject(model.Resource{ID: spaceID, Type: model.ResourceTypeSpace}),
					Relation: "space",
					Object:   formatObject(model.Resource{ID: reminderID, Type: model.ResourceTypeReminder}),
				},
			},
			wantErr: require.NoError,
		},
		{
			name:       "no relations",
			resource:   model.Resource{ID: noteID, Type: model.ResourceTypeNote},
			relations:  model.Relation{},
			wantWrites: []openFGAClient.ClientTupleKey{},
			wantErr:    require.NoError,
		},
		{
			name:     "unsupported resource type with parent",
			resource: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
			relations: model.Relation{
				Parent: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "resource type space does not support parent relation")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			writes, deletes, err := tuplesForCreate(tt.resource, tt.relations)
			tt.wantErr(t, err)
			require.Equal(t, tt.wantWrites, writes)
			require.Equal(t, tt.wantDeletes, deletes)
		})
	}
}

func TestParentWrites(t *testing.T) {
	t.Parallel()

	noteID := uuid.New()
	reminderID := uuid.New()
	spaceID := uuid.New()

	tests := []struct {
		name      string
		resource  model.Resource
		relations model.Relation
		want      openFGAClient.ClientTupleKey
		wantErr   require.ErrorAssertionFunc
	}{
		{
			name:     "note with parent",
			resource: model.Resource{ID: noteID, Type: model.ResourceTypeNote},
			relations: model.Relation{
				Parent: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
			},
			want: openFGAClient.ClientTupleKey{
				User:     formatObject(model.Resource{ID: spaceID, Type: model.ResourceTypeSpace}),
				Relation: "space",
				Object:   formatObject(model.Resource{ID: noteID, Type: model.ResourceTypeNote}),
			},
			wantErr: require.NoError,
		},
		{
			name:     "reminder with parent",
			resource: model.Resource{ID: reminderID, Type: model.ResourceTypeReminder},
			relations: model.Relation{
				Parent: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
			},
			want: openFGAClient.ClientTupleKey{
				User:     formatObject(model.Resource{ID: spaceID, Type: model.ResourceTypeSpace}),
				Relation: "space",
				Object:   formatObject(model.Resource{ID: reminderID, Type: model.ResourceTypeReminder}),
			},
			wantErr: require.NoError,
		},
		{
			name:      "parent is required",
			resource:  model.Resource{ID: noteID, Type: model.ResourceTypeNote},
			relations: model.Relation{},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "parent is required")
			},
		},
		{
			name:     "unsupported resource type",
			resource: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
			relations: model.Relation{
				Parent: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "resource type space does not support parent relation")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parentWrites(tt.resource, tt.relations)
			tt.wantErr(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFormatObject(t *testing.T) {
	t.Parallel()

	noteID := uuid.New()
	reminderID := uuid.New()

	tests := []struct {
		name     string
		resource model.Resource
		want     string
	}{
		{
			name:     "note",
			resource: model.Resource{ID: noteID, Type: model.ResourceTypeNote},
			want:     "note:" + noteID.String(),
		},
		{
			name:     "reminder",
			resource: model.Resource{ID: reminderID, Type: model.ResourceTypeReminder},
			want:     "reminder:" + reminderID.String(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatObject(tt.resource)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestToModelTuples(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	noteID := uuid.New()
	spaceID := uuid.New()
	reminderID := uuid.New()

	ownerObject := formatObject(model.Resource{ID: ownerID, Type: model.ResourceTypeUser})
	noteObject := formatObject(model.Resource{ID: noteID, Type: model.ResourceTypeNote})
	spaceObject := formatObject(model.Resource{ID: spaceID, Type: model.ResourceTypeSpace})
	reminderObject := formatObject(model.Resource{ID: reminderID, Type: model.ResourceTypeReminder})

	tests := []struct {
		name        string
		writes      []openFGAClient.ClientTupleKey
		deletes     []openFGAClient.ClientTupleKeyWithoutCondition
		wantWritten []model.Tuple
		wantDeleted []model.Tuple
	}{
		{
			name:        "empty",
			writes:      []openFGAClient.ClientTupleKey{},
			deletes:     []openFGAClient.ClientTupleKeyWithoutCondition{},
			wantWritten: []model.Tuple{},
			wantDeleted: []model.Tuple{},
		},
		{
			name: "writes only",
			writes: []openFGAClient.ClientTupleKey{
				{
					User:     ownerObject,
					Relation: "owner",
					Object:   noteObject,
				},
				{
					User:     spaceObject,
					Relation: "space",
					Object:   noteObject,
				},
			},
			wantWritten: []model.Tuple{
				{Subject: ownerObject, Relation: "owner", Resource: noteObject},
				{Subject: spaceObject, Relation: "space", Resource: noteObject},
			},
			wantDeleted: []model.Tuple{},
		},
		{
			name:   "deletes only",
			writes: []openFGAClient.ClientTupleKey{},
			deletes: []openFGAClient.ClientTupleKeyWithoutCondition{
				{
					User:     ownerObject,
					Relation: "owner",
					Object:   reminderObject,
				},
			},
			wantWritten: []model.Tuple{},
			wantDeleted: []model.Tuple{
				{Subject: ownerObject, Relation: "owner", Resource: reminderObject},
			},
		},
		{
			name: "writes and deletes",
			writes: []openFGAClient.ClientTupleKey{
				{
					User:     ownerObject,
					Relation: "owner",
					Object:   noteObject,
				},
			},
			deletes: []openFGAClient.ClientTupleKeyWithoutCondition{
				{
					User:     spaceObject,
					Relation: "space",
					Object:   reminderObject,
				},
			},
			wantWritten: []model.Tuple{
				{Subject: ownerObject, Relation: "owner", Resource: noteObject},
			},
			wantDeleted: []model.Tuple{
				{Subject: spaceObject, Relation: "space", Resource: reminderObject},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotWritten, gotDeleted := toModelTuples(tt.writes, tt.deletes)
			require.Equal(t, tt.wantWritten, gotWritten)
			require.Equal(t, tt.wantDeleted, gotDeleted)
		})
	}
}
