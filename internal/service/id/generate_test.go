package id

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		length     int
		assertWant func(t *testing.T, got string)
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name:   "positive",
			length: 10,
			assertWant: func(t *testing.T, got string) {
				t.Helper()

				require.NotEmpty(t, got)
				require.Len(t, got, 10)
				require.Regexp(t, `^[a-zA-Z0-9]+$`, got)
			},
			wantErr: require.NoError,
		},
		{
			name:   "error case: length is less than 0",
			length: -1,
			assertWant: func(t *testing.T, got string) {
				t.Helper()

				require.Empty(t, got)
			},
			wantErr: require.Error,
		},
		{
			name:   "error case: length is 0",
			length: 0,
			assertWant: func(t *testing.T, got string) {
				t.Helper()

				require.Empty(t, got)
			},
			wantErr: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Generate(tt.length)
			tt.wantErr(t, err)

			tt.assertWant(t, got)
		})
	}
}
