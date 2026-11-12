package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValid(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		kind SpaceKind
		want bool
	}{
		{"shared", SpaceKindShared, true},
		{"personal", SpaceKindPersonal, true},
		{"wrong case", SpaceKind("SHARED"), false},
		{"unknown", SpaceKind("group"), false},
		{"empty", SpaceKind(""), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.kind.Valid()

			require.Equal(t, tc.want, got)
		})
	}
}
