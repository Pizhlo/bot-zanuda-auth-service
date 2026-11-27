package decision

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"auth-service/internal/model"
	"auth-service/internal/service/decision/mocks"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	override := func(opt serviceOption) func(*fixture) []serviceOption {
		return func(f *fixture) []serviceOption { return append(allDecisionOptions(f), opt) }
	}
	noMocks := func(*fixture) {}
	versions := func(f *fixture) {
		f.relations.EXPECT().ModelID().Return("pinned-model")
		f.fields.EXPECT().Version().Return("policy-version")
	}
	tests := []struct {
		name         string
		options      func(*fixture) []serviceOption
		prepareMocks func(*fixture)
		wantErr      require.ErrorAssertionFunc
		want         func(*testing.T, *Service, *fixture)
	}{
		{"ready dependencies", allDecisionOptions, versions, require.NoError, wantReadyDecisionService},
		{"no options", func(*fixture) []serviceOption { return nil }, noMocks, wantConstructorError("identity store is required"), wantNilDecisionService},
		{"missing identity store", override(WithIdentityStore(nil)), noMocks, wantConstructorError("identity store is required"), wantNilDecisionService},
		{"missing relation checker", override(WithRelationChecker(nil)), noMocks, wantConstructorError("relation checker is required"), wantNilDecisionService},
		{"missing field policy", override(WithFieldPolicy(nil)), noMocks, wantConstructorError("field policy is required"), wantNilDecisionService},
		{"missing decision store", override(WithDecisionStore(nil)), noMocks, wantConstructorError("decision store is required"), wantNilDecisionService},
		{"model not loaded", allDecisionOptions, func(f *fixture) { f.relations.EXPECT().ModelID().Return("") }, wantConstructorError("authorization model and field policy must be loaded"), wantNilDecisionService},
		{"policy not loaded", allDecisionOptions, func(f *fixture) {
			f.relations.EXPECT().ModelID().Return("pinned-model")
			f.fields.EXPECT().Version().Return("")
		}, wantConstructorError("authorization model and field policy must be loaded"), wantNilDecisionService},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newDecisionDependencies(t)
			tc.prepareMocks(f)

			got, err := New(tc.options(f)...)
			tc.wantErr(t, err)

			tc.want(t, got, f)
		})
	}
}

func allDecisionOptions(f *fixture) []serviceOption {
	return []serviceOption{WithIdentityStore(f.users), WithRelationChecker(f.relations), WithFieldPolicy(f.fields), WithDecisionStore(f.store)}
}

func wantReadyDecisionService(t *testing.T, got *Service, f *fixture) {
	t.Helper()
	require.NotNil(t, got)
	require.Same(t, f.users, got.users)
	require.Same(t, f.relations, got.relations)
	require.Same(t, f.fields, got.fields)
	require.Same(t, f.store, got.decisions)
}

func wantConstructorError(message string) require.ErrorAssertionFunc {
	return func(t require.TestingT, err error, _ ...any) { require.EqualError(t, err, message) }
}

func wantNilDecisionService(t *testing.T, got *Service, _ *fixture) {
	t.Helper()
	require.Nil(t, got)
}

type fixture struct {
	t         *testing.T
	store     *mocks.MockdecisionStore
	users     *mocks.MockidentityStore
	relations *mocks.MockrelationChecker
	fields    *mocks.MockfieldPolicy

	req    model.CheckDecisionRequest
	caller Caller
}

func newDecisionDependencies(t *testing.T) *fixture {
	t.Helper()

	ctrl := gomock.NewController(t)

	return &fixture{
		t:         t,
		store:     mocks.NewMockdecisionStore(ctrl),
		users:     mocks.NewMockidentityStore(ctrl),
		relations: mocks.NewMockrelationChecker(ctrl),
		fields:    mocks.NewMockfieldPolicy(ctrl),
	}
}

func newFixture(t *testing.T) (*fixture, *Service) {
	t.Helper()

	f := newDecisionDependencies(t)
	f.caller = Caller{ClientID: "bot", TelegramID: "42"}
	f.req = model.CheckDecisionRequest{
		RequestID: uuid.New(),
		Subject:   model.DecisionSubject{ID: uuid.New(), Type: model.ResourceTypeUser},
		Action: model.DecisionAction{
			Name:       model.ActionNameUpdate,
			Attributes: model.DecisionActionAttributes{FieldMask: []string{"text", "title"}},
		},
		Resource: model.DecisionResource{ID: uuid.New(), Type: model.ResourceTypeNote},
		Options:  model.DecisionOptions{IncludeReasons: true},
	}
	f.relations.EXPECT().ModelID().Return("pinned-model").AnyTimes()
	f.fields.EXPECT().Version().Return("casbin:sha256:test").AnyTimes()
	svc, err := New(allDecisionOptions(f)...)
	require.NoError(t, err)

	return f, svc
}

func (f *fixture) completed() {
	f.store.EXPECT().SaveDecision(gomock.Any(), gomock.Any())
}

func (f *fixture) identity() {
	f.users.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(f.req.Subject.ID, nil)
}
func (f *fixture) primary(allowed bool, err error) *gomock.Call {
	return f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Subject.ID.String(), "can_update", "note:"+f.req.Resource.ID.String(), model.HigherConsistency).Return(allowed, err)
}

func TestEvaluateAndSave(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantAudit func(*testing.T, *attempt)
		expect    func(*fixture)
		wantErr   error
		want      func(*testing.T, model.CheckDecisionResponse)
	}{
		{name: "allow", wantAudit: wantDecisionStage(stageCompleted), want: func(t *testing.T, resp model.CheckDecisionResponse) {
			t.Helper()
			require.True(t, resp.Allow)
			require.NotEmpty(t, resp.Reasons)
			require.Equal(t, "allowed_via_fga_relation", resp.Reasons[0].Code)
		}, expect: func(f *fixture) {
			f.completed()
			f.identity()
			first := f.primary(true, nil)
			f.fields.EXPECT().CheckFields("note", "update", []string{"text", "title"}).After(first).Return([]string{}, nil)
		}},
		{name: "write fails after allow", expect: expectFailedDecisionSave(true), wantErr: ErrDecisionStoreUnavailable, want: wantEvaluationFailure, wantAudit: wantSaveFailureAudit(model.DecisionResultAllow)},
		{name: "write fails after deny", expect: expectFailedDecisionSave(false), wantErr: ErrDecisionStoreUnavailable, want: wantEvaluationFailure, wantAudit: wantSaveFailureAudit(model.DecisionResultDeny)},
		{name: "subject verification failed", expect: func(f *fixture) { f.caller.ClientID = "" }, wantErr: ErrUnauthorized, want: wantEvaluationFailure, wantAudit: wantDecisionStage(audit.StageVerifySubject.String())},
		{
			name: "invalid request", wantAudit: wantDecisionStage(audit.StageValidateRequest.String()),
			wantErr: model.ErrInvalidDecisionRequest, want: wantEvaluationFailure,
			expect: func(f *fixture) { f.req.RequestID = uuid.Nil },
		},
		{
			name: "fga unavailable", wantAudit: wantDecisionStage(stageCheckRelation),
			wantErr: ErrOpenFGAUnavailable, want: wantEvaluationFailure,
			expect: func(f *fixture) {
				f.identity()
				f.primary(false, context.DeadlineExceeded)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, svc := newFixture(t)
			tt.expect(f)

			a := &attempt{fields: audit.EventContext{}}
			resp, err := svc.evaluateAndSave(t.Context(), f.caller, f.req, a)
			require.ErrorIs(t, err, tt.wantErr)

			tt.want(t, resp)

			tt.wantAudit(t, a)
		})
	}
}

func TestVerifySubject(t *testing.T) {
	t.Parallel()

	const invalidHeader = "X-Telegram-User-Id header must be a positive decimal int64 without spaces, signs or leading zeros"

	noAdditionalChecks := func(*testing.T, error) {}
	tests := []struct {
		name    string
		expect  func(*fixture)
		wantErr error
		want    func(*testing.T, error)
	}{
		{name: "missing client", want: noAdditionalChecks, wantErr: ErrUnauthorized, expect: func(f *fixture) { f.caller.ClientID = "" }},
		{
			name: "negative ID",
			want: wantDecisionHeaderError(invalidHeader), wantErr: model.ErrInvalidDecisionRequest,
			expect: func(f *fixture) { f.caller.TelegramID = "-42" },
		},
		{
			name: "telegram mismatch",
			want: noAdditionalChecks, wantErr: model.ErrInvalidDecisionRequest,
			expect: func(f *fixture) {
				value := "43"
				f.req.Subject.Attributes.TelegramUserID = &value
			},
		},
		{
			name: "subject mismatch",
			want: noAdditionalChecks, wantErr: ErrSubjectMismatch,
			expect: func(f *fixture) { f.users.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(uuid.New(), nil) },
		},
		{name: "unregistered", want: noAdditionalChecks, wantErr: ErrUserNotFound, expect: func(f *fixture) {
			f.users.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(uuid.Nil, storage.ErrNotFound)
		}},
		{
			name: "nil identity",
			want: noAdditionalChecks, wantErr: ErrUserNotFound,
			expect: func(f *fixture) { f.users.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(uuid.Nil, nil) },
		},
		{name: "database unavailable", want: noAdditionalChecks, wantErr: ErrIdentityUnavailable, expect: func(f *fixture) {
			f.users.EXPECT().GetUserIDByTelegramID(gomock.Any(), "42").Return(uuid.Nil, errors.New("db secret must stay internal"))
		}},
		{name: "verified", want: noAdditionalChecks, expect: func(f *fixture) { f.identity() }},
		{name: "unexpanded variable", expect: func(f *fixture) { f.caller.TelegramID = "{{telegram_user_id}}" }, wantErr: model.ErrInvalidDecisionRequest, want: wantDecisionHeaderError(invalidHeader)},
		{name: "overflow", expect: func(f *fixture) { f.caller.TelegramID = "9223372036854775808" }, wantErr: model.ErrInvalidDecisionRequest, want: wantDecisionHeaderError(invalidHeader)},
		{name: "missing header", want: wantDecisionHeaderError("X-Telegram-User-Id header is required"), wantErr: model.ErrInvalidDecisionRequest, expect: func(f *fixture) { f.caller.TelegramID = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, svc := newFixture(t)
			tt.expect(f)

			err := svc.verifySubject(t.Context(), f.caller, f.req.Subject)
			require.ErrorIs(t, err, tt.wantErr)
			tt.want(t, err)
		})
	}
}

func TestEvaluate(t *testing.T) {
	t.Parallel()

	space := uuid.New()
	parent := "space:" + space.String()

	noAdditionalChecks := func(*testing.T, *fixture, []model.DecisionReason) {}
	tests := []struct {
		name   string
		want   func(*testing.T, *fixture, []model.DecisionReason)
		expect func(*fixture)
		allow  bool
		codes  []string
		cause  error
		stage  string
	}{
		{name: "parent relation present", allow: true, codes: []string{model.ReasonAllowedViaFGARelation}, expect: expectParentRelation(space, true), want: wantParentRelation(parent, "allowed")},
		{name: "parent relation absent", codes: []string{model.ReasonDeniedByFGARelation}, expect: expectParentRelation(space, false), want: wantParentRelation(parent, "denied")},
		{name: "relation deny", want: noAdditionalChecks, codes: []string{"denied_by_fga_relation"}, expect: func(f *fixture) { f.primary(false, nil) }},
		{name: "primary relation unavailable", codes: []string{}, cause: ErrOpenFGAUnavailable, want: noAdditionalChecks, expect: func(f *fixture) { f.primary(false, context.DeadlineExceeded) }},
		{name: "update delegates to field policy", allow: true, stage: stageCheckFields, codes: []string{model.ReasonAllowedViaFGARelation, model.ReasonAllowedFields}, want: noAdditionalChecks, expect: func(f *fixture) {
			first := f.primary(true, nil)
			f.fields.EXPECT().CheckFields("note", "update", f.req.Action.Attributes.FieldMask).After(first).Return(nil, nil)
		}},
		{name: "membership delegates to owner check", allow: true, stage: stageCheckTargetOwner, codes: []string{model.ReasonAllowedViaFGARelation}, want: noAdditionalChecks, expect: expectMembershipEvaluation},
		{name: "shared space create", want: noAdditionalChecks, allow: true, codes: []string{"allowed_for_registered_user"}, expect: func(f *fixture) {
			f.req.Action = model.DecisionAction{Name: model.ActionNameCreate}
			f.req.Resource.Type = model.ResourceTypeSpace
			f.req.Resource.Attributes.SpaceType = new(model.SpaceKindShared)
			f.relations.EXPECT().CheckModelAvailability(gomock.Any()).Return(nil)
		}},
		{name: "personal space create", want: noAdditionalChecks, codes: []string{model.ReasonPersonalSpaceAlreadyExists}, expect: expectExistingPersonalSpace},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, svc := newFixture(t)
			tt.expect(f)
			object, relation, err := resolveTarget(f.req)
			require.NoError(t, err)

			a := &attempt{fields: audit.EventContext{}}
			allowed, reasons, err := svc.evaluate(t.Context(), f.req, object, relation, a)
			require.ErrorIs(t, err, tt.cause)

			require.Equal(t, tt.allow, allowed)
			require.Equal(t, tt.stage, a.stage)

			codes := make([]string, len(reasons))
			for i, r := range reasons {
				codes[i] = r.Code
			}

			require.Equal(t, tt.codes, codes)
			tt.want(t, f, reasons)
		})
	}
}

func TestCheckSharedSpace(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		prepareMocks func(*fixture)
		wantErr      require.ErrorAssertionFunc
		want         func(*testing.T, model.CheckDecisionRequest, bool, []model.DecisionReason)
	}{
		{"pinned model available", func(f *fixture) { f.relations.EXPECT().CheckModelAvailability(gomock.Any()).Return(nil) }, require.NoError, func(t *testing.T, req model.CheckDecisionRequest, allowed bool, reasons []model.DecisionReason) {
			t.Helper()
			require.True(t, allowed)
			require.Equal(t, []model.DecisionReason{{Code: model.ReasonAllowedForRegisteredUser, Message: "Registered user:" + req.Subject.ID.String() + " may create shared space:" + req.Resource.ID.String()}}, reasons)
		}},
		{"pinned model unavailable", func(f *fixture) {
			f.relations.EXPECT().CheckModelAvailability(gomock.Any()).Return(context.DeadlineExceeded)
		}, wantUnavailableRelation, func(t *testing.T, _ model.CheckDecisionRequest, allowed bool, reasons []model.DecisionReason) {
			t.Helper()
			require.False(t, allowed)
			require.Empty(t, reasons)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, svc := newFixture(t)
			tc.prepareMocks(f)
			req := model.CheckDecisionRequest{
				Subject:  f.req.Subject,
				Action:   model.DecisionAction{Name: model.ActionNameCreate},
				Resource: model.DecisionResource{Type: model.ResourceTypeSpace, ID: uuid.New(), Attributes: model.DecisionResourceAttributes{SpaceType: new(model.SpaceKindShared)}},
			}

			allowed, reasons, err := svc.checkSharedSpace(t.Context(), req)
			tc.wantErr(t, err)

			tc.want(t, req, allowed, reasons)
		})
	}
}

func wantUnavailableRelation(t require.TestingT, err error, _ ...any) {
	require.ErrorIs(t, err, ErrOpenFGAUnavailable)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestCheckPersonalSpace проверяет оба состояния владельца и ошибку чтения OpenFGA.
func TestCheckPersonalSpace(t *testing.T) {
	t.Parallel()

	owner := "user:" + uuid.NewString()

	expectMarker := func(exists bool, err error) func(*fixture) {
		return func(f *fixture) {
			f.relations.EXPECT().CheckRelation(gomock.Any(), owner, model.RelationHasPersonalSpace, owner, model.HigherConsistency).Return(exists, err)
		}
	}

	wantDecision := func(allowed bool, code string) func(*testing.T, bool, []model.DecisionReason) {
		return func(t *testing.T, got bool, reasons []model.DecisionReason) {
			t.Helper()
			require.Equal(t, allowed, got)
			require.Len(t, reasons, 1)
			require.Equal(t, code, reasons[0].Code)
			require.Contains(t, reasons[0].Message, owner)
			require.Contains(t, reasons[0].Message, model.RelationHasPersonalSpace)
		}
	}

	for _, tc := range []struct {
		name         string
		prepareMocks func(*fixture)
		wantErr      require.ErrorAssertionFunc
		want         func(*testing.T, bool, []model.DecisionReason)
	}{
		{name: "first", prepareMocks: expectMarker(false, nil), wantErr: require.NoError, want: wantDecision(true, model.ReasonFirstPersonalSpace)},
		{name: "already exists", prepareMocks: expectMarker(true, nil), wantErr: require.NoError, want: wantDecision(false, model.ReasonPersonalSpaceAlreadyExists)},
		{
			name:         "unavailable",
			prepareMocks: expectMarker(false, context.DeadlineExceeded),
			wantErr:      wantUnavailableRelation,
			want: func(t *testing.T, allowed bool, reasons []model.DecisionReason) {
				t.Helper()
				require.False(t, allowed)
				require.Empty(t, reasons)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, svc := newFixture(t)
			tc.prepareMocks(f)

			allowed, reasons, err := svc.checkPersonalSpace(t.Context(), owner, model.RelationHasPersonalSpace)
			tc.wantErr(t, err)

			tc.want(t, allowed, reasons)
		})
	}
}

func TestCheckMembershipChangeTarget(t *testing.T) {
	t.Parallel()

	target := model.DecisionResource{ID: uuid.New(), Type: model.ResourceTypeMember}
	object := "space:" + uuid.NewString()

	expectOwner := func(consistency model.Consistency, isOwner bool, err error) func(*mocks.MockrelationChecker) {
		return func(checker *mocks.MockrelationChecker) {
			checker.EXPECT().CheckRelation(gomock.Any(), "user:"+target.ID.String(), "owner", object, consistency).Return(isOwner, err)
		}
	}

	wantReason := func(code string) func(*testing.T, []model.DecisionReason) {
		return func(t *testing.T, reasons []model.DecisionReason) {
			t.Helper()
			require.Len(t, reasons, 1)
			require.Equal(t, code, reasons[0].Code)
			require.Contains(t, reasons[0].Message, object)
		}
	}
	for _, tc := range []struct {
		name        string
		consistency model.Consistency
		expect      func(*mocks.MockrelationChecker)
		allowed     bool
		want        func(*testing.T, []model.DecisionReason)
		wantErr     error
		ownerResult any
	}{
		{"protected owner", model.HigherConsistency, expectOwner(model.HigherConsistency, true, nil), false, wantReason(model.ReasonProtectedSpaceOwner), nil, true},
		{"other member", model.MinimizeLatency, expectOwner(model.MinimizeLatency, false, nil), true, wantReason(model.ReasonAllowedViaFGARelation), nil, false},
		{"unavailable", model.HigherConsistency, expectOwner(model.HigherConsistency, false, context.DeadlineExceeded), false, func(t *testing.T, reasons []model.DecisionReason) {
			t.Helper()
			require.Empty(t, reasons)
		}, ErrOpenFGAUnavailable, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			checker := mocks.NewMockrelationChecker(gomock.NewController(t))
			tc.expect(checker)
			svc := Service{relations: checker}
			req := model.CheckDecisionRequest{Resource: target, Options: model.DecisionOptions{Consistency: &tc.consistency}}

			a := &attempt{fields: audit.EventContext{}}
			allowed, reasons, err := svc.checkMembershipChangeTarget(t.Context(), req, object, "can_manage_members", a)
			require.ErrorIs(t, err, tc.wantErr)

			result, present := a.fields[fieldTargetIsOwner]
			require.Equal(t, tc.ownerResult != nil, present)
			require.Equal(t, tc.ownerResult, result)
			require.Equal(t, tc.allowed, allowed)

			tc.want(t, reasons)
		})
	}
}

func newTestAuditor(t *testing.T, publisher *auditPublisher) audit.Source {
	t.Helper()

	sender, err := audit.NewSender(audit.WithClient(publisher), audit.WithTopic("test"))
	require.NoError(t, err)

	return audit.NewAuditor(audit.WithSender(sender), audit.WithDeliveryTimeout(time.Second))
}

func auditedCheck(t *testing.T, svc *Service, source audit.Source, f *fixture) (resp model.CheckDecisionResponse, err error) {
	t.Helper()

	err = AuditRequest(t.Context(), source, OperationCheckDecision, uuid.Nil, func(ctx context.Context) error {
		resp, err = svc.Check(ctx, f.caller, f.req)
		return err
	})

	return resp, err
}

type auditPublisher struct {
	err        error
	data       []string
	bounded    bool
	contextErr error
}

func (p *auditPublisher) Publish(ctx context.Context, _ string, data []byte) error {
	p.data = append(p.data, string(data))
	_, p.bounded = ctx.Deadline()
	p.contextErr = ctx.Err()

	return p.err
}

func TestWithDecisionStore(t *testing.T) {
	t.Parallel()

	dep := mocks.NewMockdecisionStore(gomock.NewController(t))

	var s Service

	WithDecisionStore(dep)(&s)

	require.Same(t, dep, s.decisions)
}

func TestWithIdentityStore(t *testing.T) {
	t.Parallel()

	dep := mocks.NewMockidentityStore(gomock.NewController(t))

	var s Service

	WithIdentityStore(dep)(&s)

	require.Same(t, dep, s.users)
}

func TestWithRelationChecker(t *testing.T) {
	t.Parallel()

	dep := mocks.NewMockrelationChecker(gomock.NewController(t))

	var s Service

	WithRelationChecker(dep)(&s)

	require.Same(t, dep, s.relations)
}

func TestWithFieldPolicy(t *testing.T) {
	t.Parallel()

	dep := mocks.NewMockfieldPolicy(gomock.NewController(t))

	var s Service

	WithFieldPolicy(dep)(&s)

	require.Same(t, dep, s.fields)
}

func TestDecisionResponse(t *testing.T) {
	t.Parallel()

	f, s := newFixture(t)
	reasons := reason("sample", "details")

	resp := s.decisionResponse(f.req, false, reasons, time.Now())

	require.Equal(t, "deny", resp.Result)
	require.False(t, resp.Allow)
	require.Equal(t, reasons, resp.Reasons)
	require.Equal(t, f.req.RequestID, resp.RequestID)
	require.NotEqual(t, uuid.Nil, resp.DecisionID)
	require.Equal(t, "user:"+f.req.Subject.ID.String(), resp.Meta.Subject)
	require.Equal(t, "pinned-model", resp.Meta.AuthorizationModelID)
	require.GreaterOrEqual(t, resp.Meta.LatencyMS, int64(0))
}

func TestSaveDecision(t *testing.T) {
	t.Parallel()

	f, s := newFixture(t)
	resp := model.CheckDecisionResponse{DecisionID: uuid.New(), Result: "deny", Reasons: reason("absent", "missing relation")}
	object := "note:" + f.req.Resource.ID.String()
	relation := "can_update"
	want := model.DecisionRecord{
		DecisionID:      resp.DecisionID,
		RequestID:       f.req.RequestID,
		ClientID:        f.caller.ClientID,
		SubjectID:       f.req.Subject.ID,
		Action:          f.req.Action,
		Resource:        f.req.Resource,
		Result:          "deny",
		Reasons:         resp.Reasons,
		CheckedObject:   &object,
		CheckedRelation: &relation,
	}
	f.store.EXPECT().SaveDecision(gomock.Any(), want).Return(context.DeadlineExceeded)

	err := s.saveDecision(t.Context(), f.req, f.caller, object, relation, resp)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, ErrDecisionStoreUnavailable)
}

func TestCheckFields(t *testing.T) {
	t.Parallel()

	req := decisionFixture()

	check := func(forbidden []string, err error) func(*mocks.MockfieldPolicy) {
		return func(fields *mocks.MockfieldPolicy) {
			fields.EXPECT().CheckFields("note", "update", req.Action.Attributes.FieldMask).Return(forbidden, err)
		}
	}

	beforeFields := model.DecisionReason{Code: "passed", Message: "allowed relation"}

	tests := []struct {
		name        string
		expect      func(*mocks.MockfieldPolicy)
		wantErr     require.ErrorAssertionFunc
		wantAllowed bool
		wantReasons []model.DecisionReason
	}{
		{
			name: "allowed", expect: check(nil, nil), wantErr: require.NoError, wantAllowed: true,
			wantReasons: []model.DecisionReason{beforeFields, {Code: model.ReasonAllowedFields, Message: "Allowed fields for note:update: text, title"}},
		},
		{
			name: "denied", expect: check([]string{"timestamp", "created_at"}, nil), wantErr: require.NoError,
			wantReasons: []model.DecisionReason{{Code: model.ReasonForbiddenFields, Message: "Forbidden fields for note:update: created_at, timestamp"}},
		},
		{
			name: "policy unavailable", expect: check(nil, context.DeadlineExceeded),
			wantErr: func(t require.TestingT, err error, _ ...any) {
				require.ErrorIs(t, err, ErrPolicyUnavailable)
				require.ErrorIs(t, err, context.DeadlineExceeded)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fields := mocks.NewMockfieldPolicy(gomock.NewController(t))
			tt.expect(fields)
			s := Service{fields: fields}

			allowed, reasons, err := s.checkFields(req, []model.DecisionReason{beforeFields})
			tt.wantErr(t, err)

			require.Equal(t, tt.wantAllowed, allowed)
			require.Equal(t, tt.wantReasons, reasons)
		})
	}
}

type checkResult func(*testing.T, *fixture, []checkCall, []model.DecisionRecord, *auditPublisher)

type checkCall struct {
	response model.CheckDecisionResponse
	err      error
}

func TestCheck(t *testing.T) {
	t.Parallel()

	const timestamp = "2030-01-01T00:00:00Z"

	expectNoCalls := func(*fixture, *[]model.DecisionRecord) {}

	for _, tc := range []struct {
		name        string
		calls       int
		deliveryErr error
		check       func(*testing.T, *Service, audit.Source, *fixture) (model.CheckDecisionResponse, error)
		expect      func(*fixture, *[]model.DecisionRecord)
		want        checkResult
	}{
		{name: "no audit", calls: 1, expect: expectNoCalls, check: func(t *testing.T, s *Service, _ audit.Source, f *fixture) (model.CheckDecisionResponse, error) {
			t.Helper()
			return s.Check(t.Context(), f.caller, f.req)
		}, want: wantCheckError(audit.ErrMissingAttempt)},
		{name: "context reused after audit finished", calls: 1, expect: expectNoCalls, check: checkAfterAuditFinished, want: wantCheckError(audit.ErrMissingAttempt)},
		{name: "event for a different operation", calls: 1, expect: expectNoCalls, check: func(t *testing.T, s *Service, _ audit.Source, f *fixture) (model.CheckDecisionResponse, error) {
			t.Helper()
			ctx := context.WithValue(t.Context(), attemptKey{}, &attempt{operationName: "other"})

			return s.Check(ctx, f.caller, f.req)
		}, want: wantCheckError(audit.ErrMissingAttempt)},
		{name: "validation failure is audited", calls: 1, check: auditedCheck, expect: func(f *fixture, _ *[]model.DecisionRecord) { f.req.Subject.ID = uuid.Nil }, want: wantAuditedValidationFailure},
		{name: "shared create with unavailable model", calls: 1, check: auditedCheck, expect: expectSharedModelUnavailable, want: wantCheckError(ErrOpenFGAUnavailable)},
		{name: "shared audit", calls: 1, check: auditedCheck, expect: expectSharedDecision, want: wantCheckMetadata(timestamp, true, wantSharedAudit)},
		{name: "personal audit", calls: 1, check: auditedCheck, expect: expectPersonalDecision, want: wantCheckMetadata(timestamp, false, wantPersonalAudit)},
		{name: "member audit", calls: 1, check: auditedCheck, expect: expectMemberDecision, want: wantCheckMetadata(timestamp, true, wantMemberAudit)},
		{name: "relation and allowed fields explained", calls: 1, check: auditedCheck, expect: expectAllowedUpdate, want: wantAllowedReasons},
		{name: "repeat after access change with failed audit delivery", calls: 2, deliveryErr: errors.New("rabbit unavailable"), check: auditedCheck,
			expect: expectAccessChange, want: wantRepeatedDecisions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, svc := newFixture(t)
			f.req.Context = model.DecisionContext{Client: "untrusted", Service: "untrusted", Time: new(timestamp)}

			var records []model.DecisionRecord
			tc.expect(f, &records)

			publisher := &auditPublisher{err: tc.deliveryErr}
			source := newTestAuditor(t, publisher)

			calls := make([]checkCall, tc.calls)
			for i := range calls {
				calls[i].response, calls[i].err = tc.check(t, svc, source, f)
			}

			tc.want(t, f, calls, records, publisher)
		})
	}
}

func checkAfterAuditFinished(t *testing.T, s *Service, source audit.Source, f *fixture) (model.CheckDecisionResponse, error) {
	t.Helper()

	var saved context.Context

	require.NoError(t, AuditRequest(t.Context(), source, OperationCheckDecision, uuid.Nil, func(ctx context.Context) error {
		saved = ctx
		return nil
	}))

	return s.Check(saved, f.caller, f.req)
}

func (f *fixture) captureRecords(records *[]model.DecisionRecord, count int) {
	f.store.EXPECT().SaveDecision(gomock.Any(), gomock.Any()).Do(func(_ context.Context, record model.DecisionRecord) { *records = append(*records, record) }).Times(count)
}

func checkTrustedDecisionFields(t *testing.T, data string, resp model.CheckDecisionResponse, timestamp string) map[string]any {
	t.Helper()

	var event map[string]any
	require.NoError(t, json.Unmarshal([]byte(data), &event))

	fields, ok := event["context"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "bot", fields["caller"])
	require.Equal(t, resp.Meta.Subject, fields["subject"])
	require.Equal(t, resp.Meta.Resource, fields["resource"])
	require.Equal(t, resp.Meta.PolicyVersion, fields["policy_version"])
	require.Equal(t, resp.Meta.AuthorizationModelID, fields["authorization_model_id"])
	require.Equal(t, resp.RequestID.String(), event["request_id"])
	require.NotEqual(t, timestamp, fields["timestamp"])
	require.NotContains(t, data, "untrusted")

	return fields
}

func wantSavedSpaceType(t *testing.T, req model.CheckDecisionRequest, saved model.DecisionRecord, fields map[string]any) {
	t.Helper()

	require.Equal(t, req.Resource.Attributes.SpaceType, saved.Resource.Attributes.SpaceType)
	require.Equal(t, string(*req.Resource.Attributes.SpaceType), fields["space_type"])
}

func wantSharedAudit(t *testing.T, req model.CheckDecisionRequest, saved model.DecisionRecord, fields map[string]any) {
	t.Helper()

	wantSavedSpaceType(t, req, saved, fields)
	require.Nil(t, saved.CheckedObject)
	require.Nil(t, saved.CheckedRelation)
	require.Nil(t, fields["checked_object"])
	require.Nil(t, fields["checked_relation"])
}

func wantPersonalAudit(t *testing.T, req model.CheckDecisionRequest, saved model.DecisionRecord, fields map[string]any) {
	t.Helper()

	wantSavedSpaceType(t, req, saved, fields)
	owner := "user:" + req.Subject.ID.String()
	require.Equal(t, &owner, saved.CheckedObject)
	require.Equal(t, new(model.RelationHasPersonalSpace), saved.CheckedRelation)
	require.Equal(t, owner, fields["checked_object"])
	require.Equal(t, model.RelationHasPersonalSpace, fields["checked_relation"])
}

func wantMemberAudit(t *testing.T, req model.CheckDecisionRequest, saved model.DecisionRecord, fields map[string]any) {
	t.Helper()

	object := "space:" + req.Resource.Attributes.SpaceID.String()
	relation := "can_manage_members"

	require.Equal(t, &object, saved.CheckedObject)
	require.Equal(t, &relation, saved.CheckedRelation)
	require.Equal(t, object, fields["checked_object"])
	require.Equal(t, relation, fields["checked_relation"])
	require.Equal(t, req.Resource.ID.String(), fields["target_user_id"])
	require.Equal(t, "admin", fields["role"])
}

func wantRepeatedDecisions(t *testing.T, f *fixture, calls []checkCall, saved []model.DecisionRecord, pub *auditPublisher) {
	t.Helper()

	first := calls[0].response
	require.NoError(t, calls[0].err)
	require.True(t, first.Allow)
	require.Empty(t, first.Reasons)
	require.NotNil(t, first.Reasons)

	second := calls[1].response
	require.NoError(t, calls[1].err)
	require.False(t, second.Allow)
	require.NotEqual(t, first.DecisionID, second.DecisionID)
	require.Len(t, saved, 2)

	for i, resp := range []model.CheckDecisionResponse{first, second} {
		require.Equal(t, resp.DecisionID, saved[i].DecisionID)
		require.Equal(t, f.req.RequestID, saved[i].RequestID)
		require.Equal(t, resp.Result, saved[i].Result)
		require.NotEmpty(t, saved[i].Reasons)
		require.Equal(t, f.req.Action, saved[i].Action)
		require.Equal(t, f.req.Resource, saved[i].Resource)
	}

	require.Len(t, pub.data, 2)
	require.True(t, pub.bounded)
	require.Contains(t, pub.data[0], first.DecisionID.String())
	require.Contains(t, pub.data[0], `"caller":"bot"`)
	require.Contains(t, pub.data[0], `"checked_relation":"can_update"`)
	require.Contains(t, pub.data[0], `"subject":"user:`+f.req.Subject.ID.String()+`"`)
}

func wantAllowedReasons(t *testing.T, f *fixture, calls []checkCall, _ []model.DecisionRecord, _ *auditPublisher) {
	t.Helper()
	require.NoError(t, calls[0].err)
	resp := calls[0].response
	require.True(t, resp.Allow)
	require.Len(t, resp.Reasons, 2)
	require.Contains(t, resp.Reasons[0].Message, "can_update")
	require.Contains(t, resp.Reasons[0].Message, "note:"+f.req.Resource.ID.String())
	require.Contains(t, resp.Reasons[0].Message, "user:"+f.req.Subject.ID.String())
	require.Equal(t, "Allowed fields for note:update: text, title", resp.Reasons[1].Message)
	require.Equal(t, []string{"title", "text", "title"}, f.req.Action.Attributes.FieldMask)
}

func wantEvaluationFailure(t *testing.T, resp model.CheckDecisionResponse) {
	t.Helper()
	require.Empty(t, resp)
}

func expectFailedDecisionSave(allowed bool) func(*fixture) {
	return func(f *fixture) {
		f.req.Action = model.DecisionAction{Name: model.ActionNameRead}
		f.identity()
		f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Subject.ID.String(), "can_view", "note:"+f.req.Resource.ID.String(), model.HigherConsistency).Return(allowed, nil)
		f.store.EXPECT().SaveDecision(gomock.Any(), gomock.Any()).Return(context.DeadlineExceeded)
	}
}

func wantSaveFailureAudit(result string) func(*testing.T, *attempt) {
	return func(t *testing.T, a *attempt) {
		t.Helper()
		require.Equal(t, stageSaveDecision, a.stage)
		require.Equal(t, result, a.fields[fieldDecision])
		require.NotEmpty(t, a.fields[fieldDecisionID])
		require.Equal(t, "pinned-model", a.fields[audit.KeyAuthorizationModelID])
	}
}

func wantDecisionStage(stage string) func(*testing.T, *attempt) {
	return func(t *testing.T, a *attempt) { t.Helper(); require.Equal(t, stage, a.stage) }
}

func wantDecisionHeaderError(message string) func(*testing.T, error) {
	return func(t *testing.T, err error) {
		t.Helper()

		var validation *model.DecisionValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, message, validation.Error())
	}
}

func wantParentRelation(parent string, outcome string) func(*testing.T, *fixture, []model.DecisionReason) {
	return func(t *testing.T, f *fixture, reasons []model.DecisionReason) {
		t.Helper()
		require.Len(t, reasons, 1)

		for _, text := range []string{outcome, "can_create_note", parent, "user:" + f.req.Subject.ID.String()} {
			require.Contains(t, reasons[0].Message, text)
		}

		require.NotContains(t, reasons[0].Message, f.req.Resource.ID.String())
	}
}

func expectParentRelation(space uuid.UUID, allow bool) func(*fixture) {
	parent := "space:" + space.String()

	return func(f *fixture) {
		f.req.Action = model.DecisionAction{Name: model.ActionNameCreate}
		f.req.Resource.Attributes.SpaceID = &space
		f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Subject.ID.String(), "can_create_note", parent, model.HigherConsistency).Return(allow, nil)
	}
}

func wantCheckError(cause error) checkResult {
	return func(t *testing.T, _ *fixture, calls []checkCall, _ []model.DecisionRecord, _ *auditPublisher) {
		t.Helper()
		require.ErrorIs(t, calls[0].err, cause)
		require.Empty(t, calls[0].response)
	}
}

func wantCheckMetadata(timestamp string, allow bool, want func(*testing.T, model.CheckDecisionRequest, model.DecisionRecord, map[string]any)) checkResult {
	return func(t *testing.T, f *fixture, calls []checkCall, records []model.DecisionRecord, pub *auditPublisher) {
		t.Helper()

		resp := calls[0].response
		require.NoError(t, calls[0].err)
		require.Equal(t, allow, resp.Allow)
		require.Len(t, records, 1)
		require.Equal(t, resp.Result, records[0].Result)
		require.Len(t, pub.data, 1)
		fields := checkTrustedDecisionFields(t, pub.data[0], resp, timestamp)
		want(t, f.req, records[0], fields)
	}
}

func wantAuditedValidationFailure(t *testing.T, f *fixture, calls []checkCall, records []model.DecisionRecord, pub *auditPublisher) {
	t.Helper()
	wantCheckError(model.ErrInvalidDecisionRequest)(t, f, calls, records, pub)
	require.Len(t, pub.data, 1)

	var event requestAuditEvent
	require.NoError(t, json.Unmarshal([]byte(pub.data[0]), &event))
	require.Equal(t, f.req.RequestID, event.RequestID)
	require.Equal(t, "invalid_request", event.ErrorCode)
	require.Equal(t, "bot", event.Context["caller"])
}

func expectSharedModelUnavailable(f *fixture, _ *[]model.DecisionRecord) {
	f.req.Action = model.DecisionAction{Name: model.ActionNameCreate}
	f.req.Resource = model.DecisionResource{Type: model.ResourceTypeSpace, ID: uuid.New(), Attributes: model.DecisionResourceAttributes{SpaceType: new(model.SpaceKindShared)}}
	f.identity()
	f.relations.EXPECT().CheckModelAvailability(gomock.Any()).Return(context.DeadlineExceeded)
}

func expectSharedDecision(f *fixture, records *[]model.DecisionRecord) {
	f.req.Action = model.DecisionAction{Name: model.ActionNameCreate}
	f.req.Resource = model.DecisionResource{Type: model.ResourceTypeSpace, ID: uuid.New(), Attributes: model.DecisionResourceAttributes{SpaceType: new(model.SpaceKindShared)}}
	f.identity()
	f.relations.EXPECT().CheckModelAvailability(gomock.Any()).Return(nil)
	f.captureRecords(records, 1)
}

func expectPersonalDecision(f *fixture, records *[]model.DecisionRecord) {
	f.req.Action = model.DecisionAction{Name: model.ActionNameCreate}
	f.req.Resource = model.DecisionResource{Type: model.ResourceTypeSpace, ID: uuid.New(), Attributes: model.DecisionResourceAttributes{SpaceType: new(model.SpaceKindPersonal)}}
	f.req.Options.Consistency = new(model.MinimizeLatency)
	owner := "user:" + f.req.Subject.ID.String()
	f.identity()
	f.relations.EXPECT().CheckRelation(gomock.Any(), owner, model.RelationHasPersonalSpace, owner, model.HigherConsistency).Return(true, nil)
	f.captureRecords(records, 1)
}

func expectMemberDecision(f *fixture, records *[]model.DecisionRecord) {
	space := uuid.New()
	f.req.Resource = model.DecisionResource{Type: model.ResourceTypeMember, ID: uuid.New(), Attributes: model.DecisionResourceAttributes{SpaceID: &space}}
	f.req.Action = model.DecisionAction{Name: model.ActionNameEdit, Attributes: model.DecisionActionAttributes{Role: new("admin")}}
	f.identity()
	actor := f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Subject.ID.String(), "can_manage_members", "space:"+space.String(), model.HigherConsistency).Return(true, nil)
	f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Resource.ID.String(), "owner", "space:"+space.String(), model.HigherConsistency).After(actor).Return(false, nil)
	f.captureRecords(records, 1)
}

func expectAllowedUpdate(f *fixture, _ *[]model.DecisionRecord) {
	f.req.Action.Attributes.FieldMask = []string{"title", "text", "title"}
	f.identity()
	first := f.primary(true, nil)
	f.fields.EXPECT().CheckFields("note", "update", []string{"title", "text", "title"}).After(first).Return([]string{}, nil)
	f.completed()
}

func expectAccessChange(f *fixture, records *[]model.DecisionRecord) {
	f.req.Options.IncludeReasons = false
	f.identity()
	f.identity()
	first := f.primary(true, nil)
	f.primary(false, nil).After(first)
	f.fields.EXPECT().CheckFields("note", "update", []string{"text", "title"}).After(first).Return([]string{}, nil)
	f.captureRecords(records, 2)
}

func expectMembershipEvaluation(f *fixture) {
	space := uuid.New()
	parent := "space:" + space.String()
	f.req.Resource = model.DecisionResource{Type: model.ResourceTypeMember, ID: uuid.New(), Attributes: model.DecisionResourceAttributes{SpaceID: &space}}
	f.req.Action = model.DecisionAction{Name: model.ActionNameEdit, Attributes: model.DecisionActionAttributes{Role: new("admin")}}
	first := f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Subject.ID.String(), "can_manage_members", parent, model.HigherConsistency).Return(true, nil)
	f.relations.EXPECT().CheckRelation(gomock.Any(), "user:"+f.req.Resource.ID.String(), string(model.RelationTypeOwner), parent, model.HigherConsistency).After(first).Return(false, nil)
}

func expectExistingPersonalSpace(f *fixture) {
	f.req.Action = model.DecisionAction{Name: model.ActionNameCreate}
	f.req.Resource.Type = model.ResourceTypeSpace
	f.req.Resource.Attributes.SpaceType = new(model.SpaceKindPersonal)
	owner := "user:" + f.req.Subject.ID.String()
	f.relations.EXPECT().CheckRelation(gomock.Any(), owner, model.RelationHasPersonalSpace, owner, model.HigherConsistency).Return(true, nil)
}
