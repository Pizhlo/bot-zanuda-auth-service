package spacemigration

import (
	"strings"
	"testing"

	fgamodel "auth-service/fga"
	"auth-service/internal/service/fga"
	"auth-service/internal/testfga"
	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
)

func tuple(object, relation, user string) openfga.Tuple {
	return openfga.Tuple{Key: openfga.TupleKey{Object: object, Relation: relation, User: user}}
}

func TestBuildPlan(t *testing.T) {
	t.Parallel()

	id, owner := uuid.New(), uuid.New()
	object := "space:" + id.String()
	record := Record{SpaceID: id, OwnerID: owner, SpaceType: "personal"}

	ownerTuple := tuple(object, "owner", "user:"+owner.String())
	for _, tt := range []struct {
		name    string
		records []Record
		tuples  []openfga.Tuple
		wantErr bool
		writes  int
	}{
		{"missing marker", []Record{record}, []openfga.Tuple{ownerTuple}, false, 1},
		{"replay", []Record{record}, []openfga.Tuple{ownerTuple, tuple(object, "personal", "user:*")}, false, 0},
		{"missing export", nil, []openfga.Tuple{ownerTuple}, true, 0},
		{"duplicate export", []Record{record, record}, []openfga.Tuple{ownerTuple}, true, 0},
		{"wrong owner", []Record{{SpaceID: id, OwnerID: uuid.New(), SpaceType: "shared"}}, []openfga.Tuple{ownerTuple}, true, 0},
		{"no owner", []Record{record}, []openfga.Tuple{tuple(object, "personal", "user:*")}, true, 0},
		{"multiple owners", []Record{record}, []openfga.Tuple{ownerTuple, tuple(object, "owner", "user:"+uuid.NewString())}, true, 0},
		{"type conflict", []Record{record}, []openfga.Tuple{ownerTuple, tuple(object, "shared", "user:*")}, true, 0},
		{"two types", []Record{record}, []openfga.Tuple{ownerTuple, tuple(object, "shared", "user:*"), tuple(object, "personal", "user:*")}, true, 0},
		{"invalid marker", []Record{record}, []openfga.Tuple{ownerTuple, tuple(object, "personal", "user:"+owner.String())}, true, 0},
		{"orphan parent", nil, []openfga.Tuple{tuple("note:"+uuid.NewString(), "space", object)}, true, 0},
		{"unknown space", []Record{record}, nil, true, 0},
		{"nil ids", []Record{{SpaceType: "personal"}}, nil, true, 0},
		{"unknown type", []Record{{SpaceID: id, OwnerID: owner, SpaceType: "unknown"}}, []openfga.Tuple{ownerTuple}, true, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan, err := BuildPlan(tt.records, tt.tuples)
			if tt.wantErr {
				require.Error(t, err)
				require.Empty(t, plan.Writes)

				return
			}

			require.NoError(t, err)
			require.Len(t, plan.Writes, tt.writes)

			if tt.writes > 0 {
				require.Equal(t, sdk.ClientTupleKey{Object: object, Relation: "personal", User: "user:*"}, plan.Writes[0])
			}
		})
	}
}

func TestDecode(t *testing.T) {
	t.Parallel()

	for _, text := range []string{`null`, `[] []`, `[{"unknown":true}]`, `{`, `[{"space_id":"invalid"}]`} {
		t.Run(text, func(t *testing.T) { t.Parallel(); _, err := Decode(strings.NewReader(text)); require.Error(t, err) })
	}

	records, err := Decode(strings.NewReader(`[]`))
	require.NoError(t, err)
	require.Empty(t, records)
}

func TestMigrationRoundTrip(t *testing.T) {
	t.Parallel()
	api := testfga.New(t, fgamodel.Transition)
	id, owner := uuid.New(), uuid.New()
	object := "space:" + id.String()
	tuples := []sdk.ClientTupleKey{{Object: object, Relation: "owner", User: "user:" + owner.String()}, {Object: object, Relation: "viewer", User: "user:" + uuid.NewString()}}
	testfga.Write(t, api, tuples...)

	records := []Record{{SpaceID: id, OwnerID: owner, SpaceType: "personal"}}
	dry, err := Run(t.Context(), api, records, false)
	require.NoError(t, err)
	require.Len(t, dry.Writes, 1)
	before, err := fga.ReadTuples(t.Context(), api, "")
	require.NoError(t, err)
	require.Len(t, before, 2)
	applied, err := Run(t.Context(), api, records, true)
	require.NoError(t, err)
	require.Equal(t, dry, applied)
	replay, err := Run(t.Context(), api, records, true)
	require.NoError(t, err)
	require.Empty(t, replay.Writes)
	after, err := fga.ReadTuples(t.Context(), api, "")
	require.NoError(t, err)
	require.Len(t, after, 3)

	keys := make([]openfga.TupleKey, 0, len(after))
	for _, item := range after {
		keys = append(keys, item.GetKey())
	}

	for _, item := range before {
		require.Contains(t, keys, item.GetKey())
	}

	testfga.SetModel(t, api, fgamodel.Strict)
	resp, err := api.Check(t.Context()).Body(sdk.ClientCheckRequest{User: "user:" + owner.String(), Object: object, Relation: "can_delete"}).Execute()
	require.NoError(t, err)
	require.False(t, resp.GetAllowed())
}

func TestMigrationReadsAllPagesAndWritesBatches(t *testing.T) {
	t.Parallel()
	api := testfga.New(t, fgamodel.Transition)
	records := make([]Record, 120)
	tuples := make([]sdk.ClientTupleKey, 120)

	for i := range records {
		id, owner := uuid.New(), uuid.New()
		records[i] = Record{SpaceID: id, OwnerID: owner, SpaceType: "shared"}
		tuples[i] = sdk.ClientTupleKey{Object: "space:" + id.String(), Relation: "owner", User: "user:" + owner.String()}
	}

	testfga.Write(t, api, tuples[:100]...)
	testfga.Write(t, api, tuples[100:]...)
	incomplete, err := Run(t.Context(), api, records[:119], true)
	require.ErrorContains(t, err, "incomplete")
	require.Empty(t, incomplete.Writes)
	plan, err := Run(t.Context(), api, records, true)
	require.NoError(t, err)
	require.Len(t, plan.Writes, 120)
	replay, err := Run(t.Context(), api, records, false)
	require.NoError(t, err)
	require.Empty(t, replay.Writes)
}
