package enforcer

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFieldPolicy(t *testing.T) {
	t.Parallel()

	policy, err := NewFieldPolicy()
	require.NoError(t, err)

	tests := []struct {
		name, resource, action string
		fields, forbidden      []string
	}{
		{"note", "note", "update", []string{"title", "text", "title"}, []string{}},
		{"reminder date and title", "reminder", "update", []string{"remind_at", "title"}, []string{}},
		{"space name", "space", "update", []string{"name"}, []string{}},
		{"technical", "note", "update", []string{"text", "timestamp", "created", "last_edit", "id", "space_id", "timestamp"}, []string{"created", "id", "last_edit", "space_id", "timestamp"}},
		{"membership", "space", "update", []string{"name", "participants"}, []string{"participants"}},
		{"unknown", "unknown", "update", []string{"text"}, []string{"text"}},
		{"other action", "note", "create", []string{"text"}, []string{"text"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			before := append([]string{}, tt.fields...)
			got, err := policy.CheckFields(tt.resource, tt.action, tt.fields)
			require.NoError(t, err)
			require.Equal(t, tt.forbidden, got)
			require.Equal(t, before, tt.fields)
		})
	}

	require.Contains(t, policy.Version(), "casbin:sha256:")
	_, err = policy.CheckFields("note", "update", nil)
	require.Error(t, err)
}

func TestFieldPolicyVersion(t *testing.T) {
	t.Parallel()

	first, err := NewFieldPolicy()
	require.NoError(t, err)
	again, err := NewFieldPolicy()
	require.NoError(t, err)
	require.Equal(t, first.Version(), again.Version())

	changed, err := loadFieldPolicy(fieldModel, strings.Replace(fieldPolicy, "remind_at", "scheduled_at", 1))
	require.NoError(t, err)
	require.NotEqual(t, first.Version(), changed.Version())

	modelChanged, err := loadFieldPolicy(fieldModel+"\n", fieldPolicy)
	require.NoError(t, err)
	require.NotEqual(t, first.Version(), modelChanged.Version())
}

func TestInvalidFieldPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, model, policy string }{
		{"missing model", "", fieldPolicy}, {"missing policy", fieldModel, ""}, {"invalid model", "not a model", fieldPolicy},
		{"invalid rule", fieldModel, "p, note, update"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := loadFieldPolicy(tt.model, tt.policy)
			require.Error(t, err)
		})
	}
}

type failingFieldEvaluator struct{}

func (failingFieldEvaluator) Enforce(...interface{}) (bool, error) {
	return false, errors.New("evaluator failed")
}
func TestFieldPolicyEvaluationFailure(t *testing.T) {
	t.Parallel()

	p := &FieldPolicy{evaluator: failingFieldEvaluator{}}
	fields, err := p.CheckFields("note", "update", []string{"text"})
	require.ErrorContains(t, err, "evaluate field policy")
	require.Nil(t, fields)
}

func TestConcurrentFieldPolicyReads(t *testing.T) {
	t.Parallel()

	policy, err := NewFieldPolicy()
	require.NoError(t, err)

	for range 32 {
		t.Run("shared immutable snapshot", func(t *testing.T) {
			t.Parallel()

			forbidden, err := policy.CheckFields("reminder", "update", []string{"title", "remind_at", "updated_at"})
			require.NoError(t, err)
			require.Equal(t, []string{"updated_at"}, forbidden)
		})
	}
}
