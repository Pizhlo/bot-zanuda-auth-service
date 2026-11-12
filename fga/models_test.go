package fgamodel

import (
	"encoding/json"
	"testing"

	"github.com/openfga/language/pkg/go/transformer"
	"github.com/stretchr/testify/require"
)

func TestModelsParse(t *testing.T) {
	t.Parallel()

	for name, model := range map[string]string{"strict": Strict, "transition": Transition} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			text, err := transformer.TransformDSLToJSON(model)
			require.NoError(t, err)
			require.True(t, json.Valid([]byte(text)))
		})
	}
}
