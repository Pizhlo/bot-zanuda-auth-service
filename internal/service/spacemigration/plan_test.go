package spacemigration

import (
	"context"
	"testing"

	"auth-service/internal/model"

	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
)

func tuple(object, relation, user string) openfga.Tuple {
	return openfga.Tuple{Key: openfga.TupleKey{Object: object, Relation: relation, User: user}}
}

//nolint:funlen // Таблица объединяет варианты одной операции с их входными данными и ожиданиями.
func TestBuildPlan(t *testing.T) {
	t.Parallel()

	id, owner := uuid.New(), uuid.New()
	object := "space:" + id.String()
	record := Record{SpaceID: id, OwnerID: owner, SpaceType: "personal"}

	ownerTuple := tuple(object, "owner", "user:"+owner.String())
	created := tuple(object, "created", "user:*")
	personal := tuple(object, "personal", "user:*")
	ownerMarker := tuple("user:"+owner.String(), "has_personal_space", "user:"+owner.String())
	createdWrite := sdk.ClientTupleKey{Object: object, Relation: "created", User: "user:*"}
	wantCanceled := func(t require.TestingT, err error, _ ...any) {
		require.ErrorIs(t, err, context.Canceled)
	}

	tests := []struct {
		name     string
		records  []Record
		tuples   []openfga.Tuple
		want     Plan
		wantErr  require.ErrorAssertionFunc
		cancelAt int
	}{
		{
			name:    "missing marker",
			records: []Record{record},
			tuples:  []openfga.Tuple{ownerTuple},
			want:    Plan{Spaces: 1, Writes: personalMigrationWrites(record)},
			wantErr: require.NoError,
		},
		{
			name:    "previous strict model",
			records: []Record{record},
			tuples:  []openfga.Tuple{ownerTuple, personal, ownerMarker},
			want:    Plan{Spaces: 1, Writes: []sdk.ClientTupleKey{createdWrite}},
			wantErr: require.NoError,
		},
		{
			name:    "shared already classified",
			records: []Record{{SpaceID: id, OwnerID: owner, SpaceType: "shared"}},
			tuples:  []openfga.Tuple{ownerTuple, tuple(object, "shared", "user:*")},
			want:    Plan{Spaces: 1, Writes: []sdk.ClientTupleKey{createdWrite}},
			wantErr: require.NoError,
		},
		{
			name:    "only UUID marker exists",
			records: []Record{record},
			tuples:  []openfga.Tuple{ownerTuple, created},
			want:    Plan{Spaces: 1, Writes: personalMigrationWrites(record)[1:]},
			wantErr: require.NoError,
		},
		{
			name:    "replay",
			records: []Record{record},
			tuples:  []openfga.Tuple{ownerTuple, created, personal, ownerMarker},
			want:    Plan{Spaces: 1, Writes: []sdk.ClientTupleKey{}},
			wantErr: require.NoError,
		},
		{
			name:    "inventory failure",
			records: []Record{record},
			tuples:  []openfga.Tuple{tuple(object, "owner", "invalid")},
			wantErr: wantMigrationError("invalid space owner"),
		},
		{
			name:    "record failure after valid row",
			records: []Record{record, {SpaceID: uuid.New(), OwnerID: owner, SpaceType: "shared"}},
			tuples:  []openfga.Tuple{ownerTuple},
			wantErr: wantMigrationError("space or owner mismatch"),
		},
		{
			name:    "missing export",
			tuples:  []openfga.Tuple{ownerTuple},
			wantErr: wantMigrationError("incomplete export"),
		},
		{
			name:    "orphaned personal-owner marker",
			records: []Record{record},
			tuples:  []openfga.Tuple{ownerTuple, tuple("user:other", "has_personal_space", "user:other")},
			wantErr: wantMigrationError("unexpected personal space marker"),
		},
		{
			name:    "duplicate export",
			records: []Record{record, record},
			tuples:  []openfga.Tuple{ownerTuple},
			wantErr: wantMigrationError("duplicate space"),
		},
		{
			name:     "cancel while scanning inventory",
			records:  []Record{record},
			tuples:   []openfga.Tuple{ownerTuple},
			wantErr:  wantCanceled,
			cancelAt: 2,
		},
		{
			name:     "cancel while reading export rows",
			records:  []Record{record},
			tuples:   []openfga.Tuple{ownerTuple},
			wantErr:  wantCanceled,
			cancelAt: 3,
		},
		{
			name:     "cancel while indexing personal records",
			records:  []Record{record},
			tuples:   []openfga.Tuple{ownerTuple},
			wantErr:  wantCanceled,
			cancelAt: 4,
		},
		{
			name:     "cancel while verifying owner tuples",
			records:  []Record{record},
			tuples:   []openfga.Tuple{ownerTuple},
			wantErr:  wantCanceled,
			cancelAt: 5,
		},
		{
			name:     "cancel while collecting missing markers",
			records:  []Record{record},
			tuples:   []openfga.Tuple{ownerTuple},
			wantErr:  wantCanceled,
			cancelAt: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan, err := BuildPlan(planBudget(t, tt.cancelAt), tt.records, tt.tuples)
			tt.wantErr(t, err)

			require.Equal(t, tt.want, plan)
		})
	}
}

//nolint:funlen // Таблица объединяет варианты одной операции с их входными данными и ожиданиями.
func TestValidateRecord(t *testing.T) {
	t.Parallel()

	record := Record{SpaceID: uuid.New(), OwnerID: uuid.New(), SpaceType: "shared"}
	object := "space:" + record.SpaceID.String()
	owner := model.Resource{Type: model.ResourceTypeUser, ID: record.OwnerID}
	other := model.Resource{Type: model.ResourceTypeUser, ID: uuid.New()}

	owned := []model.Resource{owner}
	tests := []struct {
		name       string
		row        Record
		space      existingSpace
		missing    bool
		wantObject string
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name:       "missing marker",
			row:        record,
			space:      existingSpace{owners: owned},
			missing:    true,
			wantObject: object,
			wantErr:    require.NoError,
		},
		{
			name:       "existing marker",
			row:        record,
			space:      existingSpace{owners: owned, types: []string{"shared"}},
			wantObject: object,
			wantErr:    require.NoError,
		},
		{
			name:    "missing owner ID",
			row:     Record{SpaceID: record.SpaceID, SpaceType: "shared"},
			space:   existingSpace{},
			wantErr: wantMigrationError("invalid export record"),
		},
		{
			name:    "missing space ID",
			row:     Record{OwnerID: record.OwnerID, SpaceType: "shared"},
			space:   existingSpace{},
			wantErr: wantMigrationError("invalid export record"),
		},
		{
			name:    "unknown type",
			row:     Record{SpaceID: record.SpaceID, OwnerID: record.OwnerID, SpaceType: "unknown"},
			space:   existingSpace{},
			wantErr: wantMigrationError("invalid export record"),
		},
		{
			name:    "unknown space",
			row:     Record{SpaceID: uuid.New(), OwnerID: record.OwnerID, SpaceType: "shared"},
			space:   existingSpace{},
			wantErr: wantMigrationError("space or owner mismatch"),
		},
		{
			name:    "no owner",
			row:     record,
			space:   existingSpace{},
			wantErr: wantMigrationError("space or owner mismatch"),
		},
		{
			name:    "wrong owner",
			row:     record,
			space:   existingSpace{owners: []model.Resource{other}},
			wantErr: wantMigrationError("space or owner mismatch"),
		},
		{
			name:    "multiple owners",
			row:     record,
			space:   existingSpace{owners: []model.Resource{owner, other}},
			wantErr: wantMigrationError("space or owner mismatch"),
		},
		{
			name:    "conflicting marker",
			row:     record,
			space:   existingSpace{owners: owned, types: []string{"personal"}},
			wantErr: wantMigrationError("space type conflict"),
		},
		{
			name:    "multiple markers",
			row:     record,
			space:   existingSpace{owners: owned, types: []string{"personal", "shared"}},
			wantErr: wantMigrationError("space type conflict"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, missing, err := validateRecord(tt.row, map[string]existingSpace{object: tt.space})
			tt.wantErr(t, err)

			require.Equal(t, tt.wantObject, got)
			require.Equal(t, tt.missing, missing)
		})
	}
}

func TestInventory(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	parent := "space:" + uuid.NewString()
	orphan := "space:" + uuid.NewString()
	rows := []openfga.Tuple{
		tuple("note:"+uuid.NewString(), "space", parent),
		tuple(parent, "owner", "user:"+id.String()),
		tuple("note:"+uuid.NewString(), "space", orphan),
	}

	got, err := inventory(t.Context(), rows)
	require.NoError(t, err)

	require.Len(t, got, 2)
	require.Contains(t, got, orphan)
	require.Equal(t, []model.Resource{{Type: model.ResourceTypeUser, ID: id}}, got[parent].owners)
}

//nolint:funlen // Таблица объединяет варианты одной операции с их входными данными и ожиданиями.
func TestAdd(t *testing.T) {
	t.Parallel()

	owner := model.Resource{ID: uuid.New(), Type: model.ResourceTypeUser}

	tests := []struct {
		name, relation, user string
		want                 existingSpace
		wantErr              require.ErrorAssertionFunc
	}{
		{
			name:     "UUID marker",
			relation: "created",
			user:     "user:*",
			want:     existingSpace{created: true},
			wantErr:  require.NoError,
		},
		{
			name:     "invalid UUID marker",
			relation: "created",
			user:     "user:invalid",
			want:     existingSpace{},
			wantErr:  wantMigrationError("invalid creation marker"),
		},
		{
			name:     "personal type",
			relation: "personal",
			user:     "user:*",
			want:     existingSpace{types: []string{"personal"}},
			wantErr:  require.NoError,
		},
		{
			name:     "shared type",
			relation: "shared",
			user:     "user:*",
			want:     existingSpace{types: []string{"shared"}},
			wantErr:  require.NoError,
		},
		{
			name:     "invalid type marker",
			relation: "shared",
			user:     "user:invalid",
			want:     existingSpace{},
			wantErr:  wantMigrationError("invalid marker"),
		},
		{
			name:     "owner",
			relation: "owner",
			user:     "user:" + owner.ID.String(),
			want:     existingSpace{owners: []model.Resource{owner}},
			wantErr:  require.NoError,
		},
		{
			name:     "invalid owner ID",
			relation: "owner",
			user:     "user:broken",
			want:     existingSpace{},
			wantErr:  wantMigrationError("invalid space owner"),
		},
		{
			name:     "invalid owner type",
			relation: "owner",
			user:     "space:" + owner.ID.String(),
			want:     existingSpace{},
			wantErr:  wantMigrationError("invalid space owner"),
		},
		{
			name:     "unrelated relation",
			relation: "member",
			user:     "user:" + owner.ID.String(),
			want:     existingSpace{},
			wantErr:  require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var space existingSpace

			err := space.add(openfga.TupleKey{Relation: tc.relation, User: tc.user})
			tc.wantErr(t, err)

			require.Equal(t, tc.want, space)
		})
	}
}

func TestPersonalSpaceMarkers(t *testing.T) {
	t.Parallel()

	owner := uuid.New()
	user := "user:" + owner.String()
	row := Record{SpaceID: uuid.New(), OwnerID: owner, SpaceType: "personal"}
	existing := tuple(user, "has_personal_space", user)

	marker := sdk.ClientTupleKey{Object: user, Relation: "has_personal_space", User: user}
	tests := []struct {
		name    string
		records []Record
		tuples  []openfga.Tuple
		want    []sdk.ClientTupleKey
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:    "missing",
			records: []Record{row},
			want:    []sdk.ClientTupleKey{marker},
			wantErr: require.NoError,
		},
		{
			name:    "backfilled",
			records: []Record{row},
			tuples:  []openfga.Tuple{existing},
			wantErr: require.NoError,
		},
		{
			name:    "duplicate owner",
			records: []Record{row, {SpaceID: uuid.New(), OwnerID: owner, SpaceType: "personal"}},
			wantErr: wantMigrationError("multiple personal spaces"),
		},
		{
			name:    "shared only",
			records: []Record{{SpaceID: uuid.New(), OwnerID: owner, SpaceType: "shared"}},
			wantErr: require.NoError,
		},
		{
			name:    "orphan",
			tuples:  []openfga.Tuple{existing},
			wantErr: wantMigrationError("unexpected"),
		},
		{
			name:    "wrong user",
			records: []Record{row},
			tuples:  []openfga.Tuple{tuple(user, "has_personal_space", "user:"+uuid.NewString())},
			wantErr: wantMigrationError("unexpected"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := personalSpaceMarkers(t.Context(), tt.records, tt.tuples)
			tt.wantErr(t, err)

			require.ElementsMatch(t, tt.want, got)
		})
	}
}

func wantMigrationError(message string) require.ErrorAssertionFunc {
	return func(t require.TestingT, err error, _ ...any) {
		require.ErrorContains(t, err, message)
	}
}

// cancelDuringPlan моделирует истечение бюджета внутри вычислений без таймеров и sleep.
type cancelDuringPlan struct {
	context.Context
	remaining int
	cancel    context.CancelFunc
}

func (c *cancelDuringPlan) Err() error {
	c.remaining--
	if c.remaining == 0 {
		c.cancel()
	}

	return c.Context.Err()
}

func TestSortWrites(t *testing.T) {
	t.Parallel()

	first := sdk.ClientTupleKey{Object: "space:a", Relation: "created"}
	second := sdk.ClientTupleKey{Object: "space:a", Relation: "shared"}
	last := sdk.ClientTupleKey{Object: "user:b"}

	tests := []struct {
		name         string
		writes, want []sdk.ClientTupleKey
		cancelAt     int
		wantErr      error
	}{
		{
			name: "empty",
		},
		{
			name:   "single",
			writes: []sdk.ClientTupleKey{last},
			want:   []sdk.ClientTupleKey{last},
		},
		{
			name:   "stable order across runs",
			writes: []sdk.ClientTupleKey{last, first, second},
			want:   []sdk.ClientTupleKey{first, second, last},
		},
		{
			name:     "canceled during merge",
			writes:   []sdk.ClientTupleKey{last, first, second},
			cancelAt: 2,
			wantErr:  context.Canceled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := sortWrites(planBudget(t, tc.cancelAt), tc.writes)
			require.ErrorIs(t, err, tc.wantErr)

			require.Equal(t, tc.want, got)
		})
	}
}

func planBudget(t *testing.T, cancelAt int) context.Context {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	return &cancelDuringPlan{Context: ctx, remaining: cancelAt, cancel: cancel}
}

func TestMergeWrites(t *testing.T) {
	t.Parallel()

	a := sdk.ClientTupleKey{Object: "space:a"}
	b := sdk.ClientTupleKey{Object: "space:b"}

	tests := []struct {
		name              string
		left, right, want []sdk.ClientTupleKey
		cancelAt          int
		wantErr           error
	}{
		{
			name:  "interleave",
			left:  []sdk.ClientTupleKey{b},
			right: []sdk.ClientTupleKey{a},
			want:  []sdk.ClientTupleKey{a, b},
		},
		{
			name: "empty inputs",
			want: []sdk.ClientTupleKey{},
		},
		{
			name:     "canceled",
			left:     []sdk.ClientTupleKey{a},
			want:     []sdk.ClientTupleKey{{}},
			cancelAt: 1,
			wantErr:  context.Canceled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out := make([]sdk.ClientTupleKey, len(tc.left)+len(tc.right))
			err := mergeWrites(planBudget(t, tc.cancelAt), out, tc.left, tc.right)
			require.ErrorIs(t, err, tc.wantErr)

			require.Equal(t, tc.want, out)
		})
	}
}

func TestMissingPersonalOwners(t *testing.T) {
	t.Parallel()

	owner := uuid.New()
	personal := Record{OwnerID: owner, SpaceType: "personal"}

	tests := []struct {
		name     string
		records  []Record
		cancelAt int
		want     map[string]bool
		wantErr  require.ErrorAssertionFunc
	}{
		{
			name:    "shared only",
			records: []Record{{SpaceType: "shared"}},
			want:    map[string]bool{},
			wantErr: require.NoError,
		},
		{
			name:    "personal owner",
			records: []Record{personal},
			want:    map[string]bool{"user:" + owner.String(): true},
			wantErr: require.NoError,
		},
		{
			name:    "duplicate owner",
			records: []Record{personal, personal},
			wantErr: wantMigrationError("multiple personal spaces"),
		},
		{
			name:     "canceled",
			records:  []Record{personal},
			cancelAt: 1,
			wantErr:  wantMigrationError("context canceled"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := missingPersonalOwners(planBudget(t, tc.cancelAt), tc.records)
			tc.wantErr(t, err)

			require.Equal(t, tc.want, got)
		})
	}
}
