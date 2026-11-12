package fgamodel

import (
	"slices"
	"testing"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"

	"github.com/openfga/language/pkg/go/transformer"
	"github.com/stretchr/testify/require"
)

func TestModelsParse(t *testing.T) {
	t.Parallel()

	for name, dsl := range map[string]string{"strict": Strict} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			model, err := transformer.TransformDSLToProto(dsl)
			require.NoError(t, err)

			for _, pair := range []struct {
				object, relation string
				wildcard         bool
			}{
				{"space", "created", true}, {"space", "shared", true}, {"space", "personal", true}, {"user", "has_personal_space", false},
			} {
				index := slices.IndexFunc(model.TypeDefinitions, func(d *openfgav1.TypeDefinition) bool { return d.Type == pair.object })
				require.NotEqual(t, -1, index, pair.object)
				typ := model.TypeDefinitions[index]
				require.Contains(t, typ.Relations, pair.relation)
				require.IsType(t, (*openfgav1.Userset_This)(nil), typ.Relations[pair.relation].Userset, pair.relation)
				refs := typ.GetMetadata().GetRelations()[pair.relation].GetDirectlyRelatedUserTypes()
				require.Len(t, refs, 1)
				require.Equal(t, "user", refs[0].GetType())
				_, wildcard := refs[0].RelationOrWildcard.(*openfgav1.RelationReference_Wildcard)
				require.Equal(t, pair.wildcard, wildcard)
			}
		})
	}
}
