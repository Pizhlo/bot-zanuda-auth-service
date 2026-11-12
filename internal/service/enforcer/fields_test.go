package enforcer

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckFields(t *testing.T) {
	t.Parallel()

	policy, err := NewFieldPolicy()
	require.NoError(t, err)

	for _, tc := range []struct {
		name              string
		policy            *FieldPolicy
		resource, action  string
		fields, forbidden []string
		wantErr           require.ErrorAssertionFunc
	}{
		{"note fields and duplicate", policy, "note", "update", []string{"title", "text", "title"}, []string{}, require.NoError},
		{"reminder fields", policy, "reminder", "update", []string{"remind_at", "title"}, []string{}, require.NoError},
		{"space name", policy, "space", "update", []string{"name"}, []string{}, require.NoError},
		{"technical fields", policy, "note", "update", []string{"text", "timestamp", "created", "last_edit", "id", "space_id", "timestamp"}, []string{"created", "id", "last_edit", "space_id", "timestamp"}, require.NoError},
		{"membership", policy, "space", "update", []string{"name", "participants"}, []string{"participants"}, require.NoError},
		{"unknown resource", policy, "unknown", "update", []string{"text"}, []string{"text"}, require.NoError},
		{"other action", policy, "note", "create", []string{"text"}, []string{"text"}, require.NoError},
		{"empty mask", policy, "note", "update", nil, nil, func(t require.TestingT, err error, _ ...any) {
			require.ErrorContains(t, err, "field mask must not be empty")
		}},
		{"evaluator failure", &FieldPolicy{evaluator: failingFieldEvaluator{}}, "note", "update", []string{"text"}, nil, func(t require.TestingT, err error, _ ...any) { require.ErrorContains(t, err, "evaluate field policy") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Все независимые проверки обращаются к одной неизменяемой политике параллельно.
			t.Parallel()

			before := slices.Clone(tc.fields)

			got, err := tc.policy.CheckFields(tc.resource, tc.action, tc.fields)
			tc.wantErr(t, err)

			require.Equal(t, tc.forbidden, got)
			require.Equal(t, before, tc.fields)
		})
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	original, err := NewFieldPolicy()
	require.NoError(t, err)

	require.Regexp(t, "^casbin:sha256:[0-9a-f]{64}$", original.Version())

	for _, tc := range []struct {
		name, model, policy string
		want                require.ComparisonAssertionFunc
	}{
		{"same configuration", fieldModel, fieldPolicy, require.Equal},
		{"comments excluded", fieldModel, "# additional comment\n" + fieldPolicy, require.Equal},
		{"different rule", fieldModel, strings.Replace(fieldPolicy, "remind_at", "scheduled_at", 1), require.NotEqual},
		{"different model", fieldModel + "\n", fieldPolicy, require.NotEqual},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			policy, err := loadFieldPolicy(tc.model, tc.policy)
			require.NoError(t, err)

			tc.want(t, original.Version(), policy.Version())
		})
	}
}

func TestLoadFieldPolicy(t *testing.T) {
	t.Parallel()

	failure := func(msg string) require.ErrorAssertionFunc {
		return func(t require.TestingT, err error, _ ...any) { require.ErrorContains(t, err, msg) }
	}
	wantNil := func(t *testing.T, p *FieldPolicy) { t.Helper(); require.Nil(t, p) }

	for _, tc := range []struct {
		name, model, policy string
		wantErr             require.ErrorAssertionFunc
		want                func(*testing.T, *FieldPolicy)
	}{
		{"valid", fieldModel, fieldPolicy, require.NoError, func(t *testing.T, p *FieldPolicy) {
			t.Helper()
			require.NotNil(t, p.evaluator)
			require.NotEmpty(t, p.Version())
		}},
		{"missing model", "", fieldPolicy, failure("model and policy are required"), wantNil},
		{"missing policy", fieldModel, "", failure("model and policy are required"), wantNil},
		{"invalid model", "not a model", fieldPolicy, failure("parse field model"), wantNil},
		{"invalid rule", fieldModel, "p, note, update", failure("normalize field rules: field policy requires resource, action and field"), wantNil},
		{"only comments", fieldModel, "# documentation only", failure("normalize field rules: field policy contains no rules"), wantNil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			policy, err := loadFieldPolicy(tc.model, tc.policy)
			tc.wantErr(t, err)

			tc.want(t, policy)
		})
	}
}

type failingFieldEvaluator struct{}

func (failingFieldEvaluator) Enforce(...interface{}) (bool, error) {
	return false, errors.New("evaluator failed")
}

func TestNewFieldPolicy(t *testing.T) {
	t.Parallel()

	p, err := NewFieldPolicy()
	require.NoError(t, err)

	require.NotEmpty(t, p.Version())
}

func TestNormalizeFieldRules(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, input, want string
		wantErr           require.ErrorAssertionFunc
	}{
		{"rules and comments", "# policy\n p, z, update, b\n\n  # comment\np, a, update, c", "p, a, update, c\np, z, update, b", require.NoError},
		{"empty list", "# comment", "", require.Error},
		{"missing field", "p, note, update", "", require.Error},
		{"empty field", "p, note, update, ", "", require.Error},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeFieldRules(tc.input)
			tc.wantErr(t, err)

			require.Equal(t, tc.want, got)
		})
	}
}
