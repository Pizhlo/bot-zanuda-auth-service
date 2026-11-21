package spacemigration

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"auth-service/internal/service/fga/mocks"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
)

type readStub struct {
	sdk.SdkClientReadRequestInterface
	tuples []openfga.Tuple
	err    error
}

func (s *readStub) Body(sdk.ClientReadRequest) sdk.SdkClientReadRequestInterface    { return s }
func (s *readStub) Options(sdk.ClientReadOptions) sdk.SdkClientReadRequestInterface { return s }
func (s *readStub) Execute() (*sdk.ClientReadResponse, error) {
	return &sdk.ClientReadResponse{Tuples: s.tuples}, s.err
}

type writeStub struct {
	sdk.SdkClientWriteRequestInterface
	body sdk.ClientWriteRequest
	err  error
}

func (s *writeStub) Body(body sdk.ClientWriteRequest) sdk.SdkClientWriteRequestInterface {
	s.body = body
	return s
}
func (s *writeStub) Execute() (*sdk.ClientWriteResponse, error) {
	return &sdk.ClientWriteResponse{}, s.err
}

//nolint:funlen // Таблица объединяет варианты одной операции с их входными данными и ожиданиями.
func TestRun(t *testing.T) {
	t.Parallel()

	records, before, after := personalMigrationFixture()
	object := "space:" + records[0].SpaceID.String()

	batchRecords, batchTuples, batchWrites := twoBatchInventory()

	expected := personalMigrationWrites(records[0])
	complete := Plan{Spaces: 1, Writes: expected}

	tests := []struct {
		name      string
		records   []Record
		apply     bool
		wantPlan  Plan
		wantWrite []sdk.ClientTupleKey
		wantErr   require.ErrorAssertionFunc
		expect    func(*mocks.MockSdkClient, *writeStub)
	}{
		{
			name:     "replay skips all writes",
			records:  records,
			apply:    true,
			wantPlan: Plan{Spaces: 1, Writes: []sdk.ClientTupleKey{}},
			wantErr:  require.NoError,
			expect: func(api *mocks.MockSdkClient, _ *writeStub) {
				api.EXPECT().Read(gomock.Any()).Return(&readStub{tuples: after}).Times(2)
			},
		},
		{
			name:      "second batch fails after first is acknowledged",
			records:   batchRecords,
			apply:     true,
			wantPlan:  Plan{Spaces: len(batchRecords), Writes: batchWrites},
			wantWrite: batchWrites[:100],
			wantErr: func(t require.TestingT, err error, _ ...any) {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.ErrorContains(t, err, "100 of 202 confirmed")
			},
			expect: func(api *mocks.MockSdkClient, writer *writeStub) {
				gomock.InOrder(
					api.EXPECT().Read(gomock.Any()).Return(&readStub{tuples: batchTuples}),
					api.EXPECT().Write(gomock.Any()).Return(writer),
					api.EXPECT().Write(gomock.Any()).Return(&writeStub{err: context.DeadlineExceeded}),
				)
			},
		},
		{
			name:     "dry run",
			records:  records,
			wantPlan: complete,
			wantErr:  require.NoError,
			expect: func(api *mocks.MockSdkClient, _ *writeStub) {
				api.EXPECT().Read(gomock.Any()).Return(&readStub{tuples: before})
			},
		},
		{
			name:      "apply",
			records:   records,
			apply:     true,
			wantPlan:  complete,
			wantWrite: expected,
			wantErr:   require.NoError,
			expect:    expectVerifiedWrite(before, &readStub{tuples: after}),
		},
		{
			name:    "read failure",
			records: records,
			apply:   true,
			wantErr: wantMigrationError("read migration tuples"),
			expect: func(api *mocks.MockSdkClient, _ *writeStub) {
				api.EXPECT().Read(gomock.Any()).Return(&readStub{err: context.DeadlineExceeded})
			},
		},
		{
			name:    "invalid inventory",
			records: records,
			apply:   true,
			wantErr: wantMigrationError("build space migration plan"),
			expect:  func(api *mocks.MockSdkClient, _ *writeStub) { api.EXPECT().Read(gomock.Any()).Return(&readStub{}) },
		},
		{
			name:      "write failure",
			records:   records,
			apply:     true,
			wantPlan:  complete,
			wantWrite: expected,
			wantErr:   wantMigrationError("write markers"),
			expect: func(api *mocks.MockSdkClient, write *writeStub) {
				write.err = context.DeadlineExceeded
				gomock.InOrder(api.EXPECT().Read(gomock.Any()).Return(&readStub{tuples: before}), api.EXPECT().Write(gomock.Any()).Return(write))
			},
		},
		{
			name:      "verification read failure",
			records:   records,
			apply:     true,
			wantPlan:  complete,
			wantWrite: expected,
			wantErr:   wantMigrationError("verify space migration"),
			expect:    expectVerifiedWrite(before, &readStub{err: context.DeadlineExceeded}),
		},
		{
			name:      "verification missing marker",
			records:   records,
			apply:     true,
			wantPlan:  complete,
			wantWrite: expected,
			wantErr:   wantMigrationError("missing markers"),
			expect:    expectVerifiedWrite(before, &readStub{tuples: before}),
		},
		{
			name:      "verification conflict",
			records:   records,
			apply:     true,
			wantPlan:  complete,
			wantWrite: expected,
			wantErr:   wantMigrationError("verify space migration"),
			expect:    expectVerifiedWrite(before, &readStub{tuples: []openfga.Tuple{tuple(object, "owner", "user:"+uuid.NewString())}}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := mocks.NewMockSdkClient(gomock.NewController(t))
			write := &writeStub{}
			tt.expect(api, write)

			plan, err := Run(t.Context(), api, tt.records, Options{Apply: tt.apply, RequestTimeout: time.Second})
			tt.wantErr(t, err)

			require.Equal(t, tt.wantPlan, plan)
			require.Equal(t, tt.wantWrite, write.body.Writes)
		})
	}
}

//nolint:funlen // Таблица объединяет варианты одной операции с их входными данными и ожиданиями.
func TestDecode(t *testing.T) {
	t.Parallel()

	row := Record{SpaceID: uuid.New(), OwnerID: uuid.New(), SpaceType: "shared"}
	valid, err := json.Marshal([]Record{row})
	require.NoError(t, err)

	tests := []struct {
		name, input string
		want        []Record
		wantErr     require.ErrorAssertionFunc
	}{
		{
			name:    "empty export",
			input:   "[]",
			want:    []Record{},
			wantErr: require.NoError,
		},
		{
			name:    "space",
			input:   string(valid),
			want:    []Record{row},
			wantErr: require.NoError,
		},
		{
			name:    "null",
			input:   "null",
			wantErr: wantMigrationError("must be a JSON array"),
		},
		{
			name:    "extra array",
			input:   "[] []",
			wantErr: wantMigrationError("one JSON array"),
		},
		{
			name:    "extra invalid input",
			input:   "[] trailing",
			wantErr: wantMigrationError("one JSON array"),
		},
		{
			name:    "unknown field",
			input:   `[{"unknown":true}]`,
			wantErr: wantMigrationError("unknown field"),
		},
		{
			name:    "malformed",
			input:   "{",
			wantErr: wantMigrationError("decode trusted export"),
		},
		{
			name:    "invalid UUID",
			input:   `[{"space_id":"invalid"}]`,
			wantErr: wantMigrationError("decode trusted export"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			records, err := Decode(strings.NewReader(tc.input))
			tc.wantErr(t, err)

			require.Equal(t, tc.want, records)
		})
	}
}

func TestWriteMarkers(t *testing.T) {
	t.Parallel()

	writes := []sdk.ClientTupleKey{{User: "user:*", Relation: "shared", Object: "space:s"}}

	tests := []struct {
		name    string
		prepare func(*mocks.MockSdkClient, *writeStub)
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:    "successful write",
			prepare: func(api *mocks.MockSdkClient, writer *writeStub) { api.EXPECT().Write(gomock.Any()).Return(writer) },
			wantErr: require.NoError,
		},
		{
			name: "write failed",
			prepare: func(api *mocks.MockSdkClient, writer *writeStub) {
				writer.err = context.DeadlineExceeded
				api.EXPECT().Write(gomock.Any()).Return(writer)
			},
			wantErr: func(t require.TestingT, err error, _ ...any) { require.ErrorIs(t, err, context.DeadlineExceeded) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := mocks.NewMockSdkClient(gomock.NewController(t))
			writer := &writeStub{}
			tc.prepare(api, writer)

			err := writeMarkers(t.Context(), api, writes, time.Second)
			tc.wantErr(t, err)

			require.Equal(t, writes, writer.body.Writes)
		})
	}
}

func personalMigrationFixture() ([]Record, []openfga.Tuple, []openfga.Tuple) {
	id, owner := uuid.New(), uuid.New()
	object := "space:" + id.String()
	records := []Record{{SpaceID: id, OwnerID: owner, SpaceType: "personal"}}
	before := []openfga.Tuple{tuple(object, "owner", "user:"+owner.String())}
	after := append(append([]openfga.Tuple{}, before...), tuple(object, "created", "user:*"), tuple(object, "personal", "user:*"), tuple("user:"+owner.String(), "has_personal_space", "user:"+owner.String()))

	return records, before, after
}

func personalMigrationWrites(record Record) []sdk.ClientTupleKey {
	owner := "user:" + record.OwnerID.String()

	return []sdk.ClientTupleKey{
		{Object: "space:" + record.SpaceID.String(), Relation: "created", User: "user:*"},
		{Object: "space:" + record.SpaceID.String(), Relation: "personal", User: "user:*"},
		{Object: owner, Relation: "has_personal_space", User: owner},
	}
}

func twoBatchInventory() ([]Record, []openfga.Tuple, []sdk.ClientTupleKey) {
	const size = 101

	records := make([]Record, 0, size)
	tuples := make([]openfga.Tuple, 0, size)
	objects := make([]string, 0, size)

	for range size {
		id, owner := uuid.New(), uuid.New()
		object := "space:" + id.String()
		records = append(records, Record{SpaceID: id, OwnerID: owner, SpaceType: "shared"})
		tuples = append(tuples, tuple(object, "owner", "user:"+owner.String()))
		objects = append(objects, object)
	}

	slices.Sort(objects)

	writes := make([]sdk.ClientTupleKey, 0, 2*len(objects))
	for _, object := range objects {
		writes = append(writes,
			sdk.ClientTupleKey{User: "user:*", Relation: "created", Object: object},
			sdk.ClientTupleKey{User: "user:*", Relation: "shared", Object: object},
		)
	}

	return records, tuples, writes
}

func expectVerifiedWrite(before []openfga.Tuple, after *readStub) func(*mocks.MockSdkClient, *writeStub) {
	return func(api *mocks.MockSdkClient, write *writeStub) {
		gomock.InOrder(
			api.EXPECT().Read(gomock.Any()).Return(&readStub{tuples: before}),
			api.EXPECT().Write(gomock.Any()).Return(write),
			api.EXPECT().Read(gomock.Any()).Return(after),
		)
	}
}
