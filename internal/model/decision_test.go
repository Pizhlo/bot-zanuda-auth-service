package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func decisionFixture() CheckDecisionRequest {
	return CheckDecisionRequest{RequestID: uuid.MustParse("00000000-0000-4000-8000-000000000001"),
		Subject:  DecisionSubject{Type: ResourceTypeUser, ID: uuid.MustParse("00000000-0000-4000-8000-000000000002")},
		Resource: DecisionResource{Type: ResourceTypeNote, ID: uuid.MustParse("00000000-0000-4000-8000-000000000003")},
		Action: DecisionAction{
			Name:       ActionNameUpdate,
			Attributes: DecisionActionAttributes{FieldMask: []string{"text", "title"}},
		}}
}

func wantDecisionError(message string) require.ErrorAssertionFunc {
	return func(t require.TestingT, err error, _ ...any) {
		require.ErrorIs(t, err, ErrInvalidDecisionRequest)
		require.ErrorContains(t, err, message)
	}
}

func wantSyntaxError(message string) require.ErrorAssertionFunc {
	return func(t require.TestingT, err error, _ ...any) {
		require.ErrorIs(t, err, ErrInvalidDecisionRequest)

		var validation *DecisionValidationError
		require.ErrorAs(t, err, &validation)
		require.Contains(t, validation.Error(), message)
		require.NotContains(t, validation.Error(), "private-input")
	}
}

// При ошибке декодирования обработчик использует только идентификатор запроса.
func wantDecisionRequestID(id uuid.UUID) func(*testing.T, CheckDecisionRequest) {
	return func(t *testing.T, req CheckDecisionRequest) {
		t.Helper()
		require.Equal(t, id, req.RequestID)
	}
}

func wantDecisionMask(want CheckDecisionRequest, fields []string) func(*testing.T, CheckDecisionRequest) {
	want.Action.Attributes.FieldMask = fields

	return func(t *testing.T, got CheckDecisionRequest) {
		t.Helper()
		require.Equal(t, want, got)
		require.Equal(t, HigherConsistency, got.ConsistencyPreference())
	}
}

func TestDecodeDecision(t *testing.T) {
	t.Parallel()

	fixture := decisionFixture()
	data, err := json.Marshal(fixture)
	require.NoError(t, err)

	base := string(data)

	wantUnchanged := wantDecisionMask(fixture, fixture.Action.Attributes.FieldMask)
	wantInvalid := func(t require.TestingT, err error, _ ...any) {
		require.ErrorIs(t, err, ErrInvalidDecisionRequest)
	}
	noID := wantDecisionRequestID(uuid.Nil)
	parsedID := wantDecisionRequestID(fixture.RequestID)

	tests := []struct {
		name    string
		body    string
		want    func(*testing.T, CheckDecisionRequest)
		wantErr require.ErrorAssertionFunc
	}{
		{"valid", base, wantUnchanged, require.NoError}, {"missing object", "{}", noID, wantInvalid},
		{"multiple", base + base, parsedID, wantInvalid}, {"trailing", base + "!", parsedID, wantInvalid},
		{"null object", "null", noID, wantInvalid}, {"null mask", strings.Replace(base, `["text","title"]`, `null`, 1), parsedID, wantInvalid},
		{"empty mask requires operation validation", strings.Replace(base, `["text","title"]`, `[]`, 1), wantDecisionMask(fixture, []string{}), require.NoError},
		{"blank field", strings.Replace(base, `["text","title"]`, `[" "]`, 1), parsedID, wantInvalid},
		{"field type", strings.Replace(base, `["text","title"]`, `[1]`, 1), parsedID, wantInvalid},
		{"mask type", strings.Replace(base, `["text","title"]`, `"text"`, 1), parsedID, wantInvalid},
		{"unknown", strings.Replace(base, `"name":"update"`, `"name":"update","unexpected":true`, 1), parsedID, wantInvalid},
		{"untrusted role", strings.Replace(base, `"type":"user"`, `"type":"user","relation":"owner"`, 1), parsedID, wantInvalid},
		{"untrusted admin", strings.Replace(base, `"type":"user"`, `"type":"user","is_admin":true`, 1), parsedID, wantInvalid},
		{"uuid", strings.Replace(base, "00000000-0000-4000-8000-000000000003", "n_1001", 1), parsedID, wantInvalid},
		{"unknown valid field", strings.Replace(base, `["text","title"]`, `["timestamp"]`, 1), wantDecisionMask(fixture, []string{"timestamp"}), require.NoError},
		{"duplicates", strings.Replace(base, `["text","title"]`, `["text","text"]`, 1), wantDecisionMask(fixture, []string{"text", "text"}), require.NoError},
		{"noncanonical field", strings.ReplaceAll(strings.ReplaceAll(base, `"subject"`, `"SUBJECT"`), `"id"`, `"ID"`), parsedID, wantInvalid},
		{"duplicate identifier", strings.Replace(base, `"request_id":`, `"request_id":"00000000-0000-0000-0000-000000000000","request_id":`, 1), noID, wantInvalid},
		{"duplicate object", strings.Replace(base, `"subject":`, `"subject":{"type":"ignored"},"subject":`, 1), parsedID, wantInvalid},
		{"duplicate request ID after another invalid field", strings.Replace(base, `"request_id":`, `"unexpected":0,"REQUEST_ID":"00000000-0000-4000-8000-000000000004","request_id":`, 1), noID, wantInvalid},
		{"noncanonical request ID only", strings.Replace(base, `"request_id"`, `"Request_ID"`, 1), noID, wantInvalid},
		{"overridden null rejected", strings.Replace(base, `"subject":`, `"subject":null,"subject":`, 1), parsedID, wantInvalid},
		{"overridden invalid type rejected", strings.Replace(base, `"subject":`, `"subject":123,"subject":`, 1), parsedID, wantInvalid},
		{"overridden invalid UUID rejected", strings.Replace(base, `"request_id":`, `"request_id":"private-input","request_id":`, 1), noID, wantInvalid},
		{"empty", "", noID, wantSyntaxError("body is required and must be a JSON object")},
		{"incomplete object", `{"subject":{`, noID, wantSyntaxError("body contains incomplete JSON")},
		{"incomplete value", `{"subject":{"id":`, noID, wantSyntaxError("body contains incomplete JSON")},
		{"incomplete array", `{"action":{"attributes":{"field_mask":["text"`, noID, wantSyntaxError("body contains incomplete JSON")},
		{"invalid syntax", `{"subject":{"id":!private-input}}`, noID, wantSyntaxError("body contains invalid JSON at byte")},
		{"invalid separator", `{"subject":{"type":"user" !private-input}}`, noID, wantSyntaxError("body contains invalid JSON at byte")},
		{"wrong root", `[]`, noID, wantSyntaxError("body must be a JSON object")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := DecodeDecision([]byte(tt.body))
			tt.wantErr(t, err)

			tt.want(t, req)
		})
	}
}

func TestCheckDecisionResponse(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(CheckDecisionResponse{Result: "deny", Reasons: []DecisionReason{}})
	require.NoError(t, err)

	require.Contains(t, string(data), `"result":"deny"`)
	require.Contains(t, string(data), `"allow":false`)
	require.Contains(t, string(data), `"reasons":[]`)
}

func TestValidTelegramID(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, id string
		want     bool
	}{
		{"empty", "", false}, {"zero", "0", false}, {"negative", "-1", false},
		{"leading plus", "+1", false}, {"leading zero", "01", false}, {"decimal", "1.1", false},
		{"whitespace", " 1", false}, {"above int64", "9223372036854775808", false},
		{"smallest positive", "1", true}, {"ordinary id", "123456789", true}, {"largest int64", "9223372036854775807", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ValidTelegramID(tc.id)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestCheckDecisionRequestValidate(t *testing.T) {
	t.Parallel()

	base := decisionFixture()
	withAttrs := func(fields []string, consistency *Consistency, telegram *string, ctx DecisionContext) CheckDecisionRequest {
		return CheckDecisionRequest{
			RequestID: base.RequestID,
			Subject:   DecisionSubject{Type: base.Subject.Type, ID: base.Subject.ID, Attributes: DecisionSubjectAttributes{TelegramUserID: telegram}},
			Action:    DecisionAction{Name: base.Action.Name, Attributes: DecisionActionAttributes{FieldMask: fields}},
			Resource:  base.Resource,
			Options:   DecisionOptions{Consistency: consistency},
			Context:   ctx,
		}
	}

	for _, tc := range []struct {
		name    string
		req     CheckDecisionRequest
		wantErr require.ErrorAssertionFunc
	}{
		{"valid", base, require.NoError},
		{"invalid identifiers", CheckDecisionRequest{}, wantDecisionError("request_id")},
		{"valid optional attributes", withAttrs([]string{"text"}, new(MinimizeLatency), new("42"), DecisionContext{IP: new("::1")}), require.NoError},
		{"blank field", withAttrs([]string{"title", " "}, nil, nil, DecisionContext{}), wantDecisionError("nonempty field names")},
		{"invalid consistency", withAttrs(nil, new(Consistency("stale")), nil, DecisionContext{}), wantDecisionError("options.consistency")},
		{"invalid telegram ID", withAttrs(nil, nil, new("-1"), DecisionContext{}), wantDecisionError("subject.attributes.telegram_user_id")},
		{"invalid context", withAttrs(nil, nil, nil, DecisionContext{Time: new("today")}), wantDecisionError("context.time")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.req.Validate()
			tc.wantErr(t, err)
		})
	}
}

func TestValidateIdentifiers(t *testing.T) {
	t.Parallel()

	id, subject, resource := uuid.New(), DecisionSubject{Type: ResourceTypeUser, ID: uuid.New()}, DecisionResource{Type: ResourceTypeNote, ID: uuid.New()}
	action := DecisionAction{Name: ActionNameRead}

	for _, tc := range []struct {
		name    string
		req     CheckDecisionRequest
		wantErr require.ErrorAssertionFunc
	}{
		{"valid", CheckDecisionRequest{RequestID: id, Subject: subject, Action: action, Resource: resource}, require.NoError},
		{"missing request", CheckDecisionRequest{}, wantDecisionError("request_id")},
		{"missing subject", CheckDecisionRequest{RequestID: id}, wantDecisionError("subject.id")},
		{"missing resource", CheckDecisionRequest{RequestID: id, Subject: subject}, wantDecisionError("resource.id")},
		{"wrong subject type", CheckDecisionRequest{RequestID: id, Subject: DecisionSubject{ID: subject.ID, Type: ResourceTypeSpace}, Action: action, Resource: resource}, wantDecisionError("subject.type")},
		{"blank action", CheckDecisionRequest{RequestID: id, Subject: subject, Action: DecisionAction{Name: "  "}, Resource: resource}, wantDecisionError("action.name")},
		{"blank resource type", CheckDecisionRequest{RequestID: id, Subject: subject, Action: action, Resource: DecisionResource{ID: resource.ID, Type: "  "}}, wantDecisionError("resource.type")},
		{"zero parent", CheckDecisionRequest{RequestID: id, Subject: subject, Action: action, Resource: DecisionResource{ID: resource.ID, Type: resource.Type, Attributes: DecisionResourceAttributes{SpaceID: &uuid.Nil}}}, wantDecisionError("resource.attributes.space_id")},
		{"specified parent", CheckDecisionRequest{RequestID: id, Subject: subject, Action: action, Resource: DecisionResource{ID: resource.ID, Type: resource.Type, Attributes: DecisionResourceAttributes{SpaceID: new(uuid.New())}}}, require.NoError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.req.validateIdentifiers()
			tc.wantErr(t, err)
		})
	}
}

func TestDecisionContextValidate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		ctx     DecisionContext
		wantErr require.ErrorAssertionFunc
	}{
		{"empty", DecisionContext{}, require.NoError},
		{"IPv4", DecisionContext{IP: new("127.0.0.1")}, require.NoError},
		{"IPv6 and time", DecisionContext{IP: new("::1"), Time: new("2026-11-19T11:00:00Z")}, require.NoError},
		{"invalid time", DecisionContext{Time: new("today")}, wantDecisionError("context.time")},
		{"invalid IP", DecisionContext{IP: new("private")}, wantDecisionError("context.ip")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.ctx.validate()
			tc.wantErr(t, err)
		})
	}
}

func TestConsistencyPreference(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		choice *Consistency
		want   Consistency
	}{
		{"default", nil, HigherConsistency},
		{"higher consistency", new(HigherConsistency), HigherConsistency},
		{"minimize latency", new(MinimizeLatency), MinimizeLatency},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := CheckDecisionRequest{Options: DecisionOptions{Consistency: tc.choice}}

			got := req.ConsistencyPreference()

			require.Equal(t, tc.want, got)
		})
	}
}
