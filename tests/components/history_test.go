//go:build integration

package components_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestDecisionHistory проверяет, как PostgreSQL сохраняет решения при изменении пространства
// или роли участника, создании заметки и общего пространства. Клиент определяется по токену,
// а действие, атрибуты, проверенное право, причины и версии должны соответствовать запросу
// и ответу. Повтор запроса после отзыва права сохраняет и исходное разрешение, и новый отказ.
func TestDecisionHistory(t *testing.T) {
	f := newFixture(t)
	f.launch(t, "/app/main", fgaURL).ready(t)
	token := login(t)
	target, _ := f.actor(t)
	f.store.write(t, []key{{"user:" + target, "viewer", "space:" + f.space}})

	parent := object{"space_id": f.space}
	for _, tc := range []struct {
		action, kind, id     string
		resource, attributes object
		relation             string
	}{
		{"update", "space", f.space, object{}, object{"field_mask": []any{"name"}}, "can_manage_space"},
		{"create", "note", uuid.NewString(), parent, object{}, "can_create_note"},
		{"edit", "member", target, parent, object{"role": "editor"}, "can_manage_members"},
		{"create", "space", uuid.NewString(), object{"space_type": "shared"}, object{}, ""},
	} {
		t.Run(tc.kind+"-"+tc.action, func(t *testing.T) {
			body := f.body(tc.action, tc.kind, tc.id, tc.resource, tc.attributes)
			before := time.Now()
			result := f.check(t, token, body, http.StatusOK)
			require.Equal(t, "allow", result["result"])
			row := f.storedDecision(t, before, body, result)
			want := object{

				"subject_id": f.owner, "action": tc.action,
				"action_attributes": tc.attributes, "resource_type": tc.kind, "resource_id": tc.id,
				"space_id": tc.resource["space_id"], "space_type": tc.resource["space_type"],
				"checked_object": nil, "checked_relation": nil,
			}
			if tc.relation != "" {
				want["checked_relation"] = tc.relation
				want["checked_object"] = "space:" + f.space
			}
			require.Subset(t, row, want)
		})
	}
	t.Run("allow-then-deny", func(t *testing.T) {
		before := time.Now()
		body := f.body("read", "space", f.space, object{}, object{})
		allow := f.check(t, token, body, http.StatusOK)
		require.Equal(t, "allow", allow["result"])
		request(t, http.MethodPost, f.store.url()+"/write", object{
			"authorization_model_id": f.store.model,
			"deletes":                object{"tuple_keys": []key{{"user:" + f.owner, "owner", "space:" + f.space}}},
		}, nil, http.StatusOK)
		deny := f.check(t, token, body, http.StatusOK)
		require.Equal(t, "deny", deny["result"])
		require.NotEqual(t, allow["decision_id"], deny["decision_id"])
		f.storedDecision(t, before, body, allow)
		f.storedDecision(t, before, body, deny)
	})
}

func (f *fixture) storedDecision(t *testing.T, before time.Time, body, result object) object {
	t.Helper()
	meta := result["meta"].(object)
	require.NotEmpty(t, meta["policy_version"])
	require.Equal(t, f.store.model, meta["authorization_model_id"])
	require.NotEmpty(t, result["reasons"])

	var data []byte
	var created time.Time
	err := f.db.QueryRowContext(t.Context(), `SELECT to_jsonb(d), created_at
		FROM auth.decisions d WHERE decision_id = $1 AND request_id = $2`,
		result["decision_id"], body["request_id"]).Scan(&data, &created)
	require.NoError(t, err)

	require.WithinRange(t, created, before.Truncate(time.Microsecond), time.Now())
	var row object
	require.NoError(t, json.Unmarshal(data, &row))
	require.Subset(t, row, object{
		"client_id": "bot", "decision_id": result["decision_id"], "request_id": body["request_id"],
		"result": result["result"], "reasons": result["reasons"],
		"policy_version": meta["policy_version"], "authorization_model_id": f.store.model,
	})
	return row
}
