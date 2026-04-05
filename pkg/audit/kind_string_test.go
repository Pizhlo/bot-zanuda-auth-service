package audit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKindString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind Kind
		want string
	}{
		{name: "KindValidation", kind: KindValidation, want: "Validation"},
		{name: "KindDomain", kind: KindDomain, want: "Domain"},
		{name: "KindInfra", kind: KindInfra, want: "Infra"},
		{name: "KindExternal", kind: KindExternal, want: "External"},
		{name: "KindInternal", kind: KindInternal, want: "Internal"},
		{name: "KindUnknown", kind: Kind(100), want: "Kind(100)"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.kind.String())
		})
	}
}
