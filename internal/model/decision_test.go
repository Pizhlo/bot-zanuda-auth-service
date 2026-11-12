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
		Action:   DecisionAction{Name: ActionNameUpdate, Attributes: DecisionActionAttributes{FieldMask: []string{"text", "title"}}}}
}

func TestDecodeDecision(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(decisionFixture())
	require.NoError(t, err)

	base := string(data)

	tests := []struct {
		name  string
		body  string
		valid bool
	}{
		{"valid", base, true}, {"missing object", "{}", false}, {"not object", "[]", false},
		{"syntax", "{", false}, {"multiple", base + base, false}, {"trailing", base + "!", false},
		{"null object", "null", false}, {"null mask", strings.Replace(base, `["text","title"]`, `null`, 1), false},
		{"empty mask", strings.Replace(base, `["text","title"]`, `[]`, 1), false},
		{"blank field", strings.Replace(base, `["text","title"]`, `[" "]`, 1), false},
		{"field type", strings.Replace(base, `["text","title"]`, `[1]`, 1), false},
		{"mask type", strings.Replace(base, `["text","title"]`, `"text"`, 1), false},
		{"unknown", strings.Replace(base, `"name":"update"`, `"name":"update","unexpected":true`, 1), false},
		{"untrusted role", strings.Replace(base, `"type":"user"`, `"type":"user","relation":"owner"`, 1), false},
		{"untrusted admin", strings.Replace(base, `"type":"user"`, `"type":"user","is_admin":true`, 1), false},
		{"uuid", strings.Replace(base, "00000000-0000-4000-8000-000000000003", "n_1001", 1), false},
		{"unknown valid field", strings.Replace(base, `["text","title"]`, `["timestamp"]`, 1), true},
		{"duplicates", strings.Replace(base, `["text","title"]`, `["text","text"]`, 1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := DecodeDecision([]byte(tt.body))
			if !tt.valid {
				require.ErrorIs(t, err, ErrInvalidDecisionRequest)
				return
			}

			require.NoError(t, err)
			require.Equal(t, HigherConsistency, req.ConsistencyPreference())
		})
	}
}

//nolint:funlen // Таблица граничных значений единого контракта запроса.
func TestDecisionValidation(t *testing.T) {
	t.Parallel()

	invalidConsistency := Consistency("")
	latency := MinimizeLatency
	nilID := uuid.Nil
	space := uuid.New()
	role := "viewer"
	owner := "owner"
	badIP := "invalid"
	goodIP := "::1"
	badTime := "today"
	goodTime := "2026-06-13T14:00:00Z"
	badID := "-1"
	goodID := "123"

	tests := []struct {
		name   string
		change func(*CheckDecisionRequest)
		valid  bool
	}{
		{"missing request", func(r *CheckDecisionRequest) { r.RequestID = uuid.Nil }, false},
		{"missing subject", func(r *CheckDecisionRequest) { r.Subject.ID = uuid.Nil }, false},
		{"wrong subject", func(r *CheckDecisionRequest) { r.Subject.Type = ResourceTypeSpace }, false},
		{"missing resource", func(r *CheckDecisionRequest) { r.Resource.ID = uuid.Nil }, false},
		{"unsupported action", func(r *CheckDecisionRequest) { r.Action.Name = ActionNameStop }, false},
		{"unsupported resource", func(r *CheckDecisionRequest) { r.Resource.Type = ResourceTypeUser }, false},
		{"mask missing", func(r *CheckDecisionRequest) { r.Action.Attributes.FieldMask = nil }, false},
		{"mask on read", func(r *CheckDecisionRequest) { r.Action.Name = ActionNameRead }, false},
		{"role on update", func(r *CheckDecisionRequest) { r.Action.Attributes.Role = &role }, false},
		{"empty space", func(r *CheckDecisionRequest) { r.Resource.Attributes.SpaceID = &nilID }, false},
		{"space context", func(r *CheckDecisionRequest) { r.Resource.Attributes.SpaceID = &space }, true},
		{"space with attribute", func(r *CheckDecisionRequest) {
			r.Resource.Type = ResourceTypeSpace
			r.Resource.Attributes.SpaceID = &space
		}, false},
		{"create missing parent", func(r *CheckDecisionRequest) { r.Action.Name = ActionNameCreate; r.Action.Attributes.FieldMask = nil }, false},
		{"member missing role", func(r *CheckDecisionRequest) {
			r.Resource.Type = ResourceTypeMember
			r.Resource.Attributes.SpaceID = &space
			r.Action = DecisionAction{Name: ActionNameAdd}
		}, false},
		{"member owner role", func(r *CheckDecisionRequest) {
			r.Resource.Type = ResourceTypeMember
			r.Resource.Attributes.SpaceID = &space
			r.Action = DecisionAction{Name: ActionNameAdd, Attributes: DecisionActionAttributes{Role: &owner}}
		}, false},
		{"member valid", func(r *CheckDecisionRequest) {
			r.Resource.Type = ResourceTypeMember
			r.Resource.Attributes.SpaceID = &space
			r.Action = DecisionAction{Name: ActionNameEdit, Attributes: DecisionActionAttributes{Role: &role}}
		}, true},
		{"bad consistency", func(r *CheckDecisionRequest) { r.Options.Consistency = &invalidConsistency }, false},
		{"latency", func(r *CheckDecisionRequest) { r.Options.Consistency = &latency }, true},
		{"bad IP", func(r *CheckDecisionRequest) { r.Context.IP = &badIP }, false},
		{"bad time", func(r *CheckDecisionRequest) { r.Context.Time = &badTime }, false},
		{"context", func(r *CheckDecisionRequest) { r.Context.IP = &goodIP; r.Context.Time = &goodTime }, true},
		{"bad telegram", func(r *CheckDecisionRequest) { r.Subject.Attributes.TelegramUserID = &badID }, false},
		{"telegram", func(r *CheckDecisionRequest) { r.Subject.Attributes.TelegramUserID = &goodID }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := decisionFixture()
			tt.change(&r)

			err := r.Validate()
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrInvalidDecisionRequest)
			}
		})
	}

	r := decisionFixture()
	r.Options.Consistency = &latency
	require.Equal(t, latency, r.ConsistencyPreference())
}

func TestDecisionTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind     ResourceType
		action   ActionName
		relation string
		parent   bool
	}{
		{ResourceTypeNote, ActionNameCreate, "can_create_note", true}, {ResourceTypeNote, ActionNameRead, "can_view", false}, {ResourceTypeNote, ActionNameUpdate, "can_update", false}, {ResourceTypeNote, ActionNameDelete, "can_delete", false},
		{ResourceTypeReminder, ActionNameCreate, "can_create_reminder", true}, {ResourceTypeReminder, ActionNameRead, "can_view", false}, {ResourceTypeReminder, ActionNameUpdate, "can_update", false}, {ResourceTypeReminder, ActionNameDelete, "can_delete", false}, {ResourceTypeReminder, ActionNameStop, "can_stop", false},
		{ResourceTypeSpace, ActionNameCreate, "", false}, {ResourceTypeSpace, ActionNameRead, "can_view", false}, {ResourceTypeSpace, ActionNameUpdate, "can_manage_space", false}, {ResourceTypeSpace, ActionNameDelete, "can_delete", false},
		{ResourceTypeMember, ActionNameAdd, "can_manage_members", true}, {ResourceTypeMember, ActionNameEdit, "can_manage_members", true}, {ResourceTypeMember, ActionNameRemove, "can_manage_members", true},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind)+":"+string(tt.action), func(t *testing.T) {
			t.Parallel()

			r := decisionFixture()
			r.Resource.Type = tt.kind
			r.Action.Name = tt.action
			space := uuid.New()
			r.Resource.Attributes.SpaceID = &space
			object, relation, err := r.Target()
			require.NoError(t, err)
			require.Equal(t, tt.relation, relation)

			want := string(tt.kind) + ":" + r.Resource.ID.String()
			if tt.parent {
				want = "space:" + space.String()
			}

			if relation == "" {
				want = ""
			}

			require.Equal(t, want, object)
		})
	}
}

func TestDecisionResponseJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(CheckDecisionResponse{Result: "deny", Reasons: []DecisionReason{}})
	require.NoError(t, err)
	require.Contains(t, string(data), `"result":"deny"`)
	require.Contains(t, string(data), `"allow":false`)
	require.Contains(t, string(data), `"reasons":[]`)

	legacy, err := json.Marshal(UpdateResourceResponse{})
	require.NoError(t, err)
	require.Contains(t, string(legacy), `"auth_model_id"`)
	require.NotContains(t, string(legacy), `"authorization_model_id"`)
}

func TestValidTelegramID(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"", "0", "-1", "+1", "01", "1.1", " 1", "9223372036854775808"} {
		require.False(t, ValidTelegramID(v), v)
	}

	require.True(t, ValidTelegramID("123456789"))
}
