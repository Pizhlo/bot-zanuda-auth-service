package enforcer

import (
	"errors"
	"testing"

	"auth-service/internal/service/enforcer/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест
func TestNewEnforcer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    []option
		want    *Enforcer
		wantErr require.ErrorAssertionFunc
		check   func(t *testing.T, want, actual *Enforcer)
	}{
		{
			name: "positive case",
			opts: []option{
				WithDsn("postgresql://user:password@localhost:5432/db"),
				WithModelConf("test"),
			},
			want: &Enforcer{
				dsn:       "postgresql://user:password@localhost:5432/db",
				modelConf: "test",
			},
			wantErr: require.NoError,
			check: func(t *testing.T, want, actual *Enforcer) {
				t.Helper()

				assert.Equal(t, want, actual)
			},
		},
		{
			name: "error case: dsn is required",
			opts: []option{
				WithModelConf("test"),
			},
			want:    nil,
			wantErr: require.Error,
			check: func(t *testing.T, want, actual *Enforcer) {
				t.Helper()

				assert.Nil(t, actual)
			},
		},
		{
			name: "error case: model conf is required",
			opts: []option{
				WithDsn("postgresql://user:password@localhost:5432/db"),
			},
			want:    nil,
			wantErr: require.Error,
			check: func(t *testing.T, want, actual *Enforcer) {
				t.Helper()

				assert.Nil(t, actual)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewEnforcer(tt.opts...)
			tt.wantErr(t, err)

			tt.check(t, tt.want, got)
		})
	}
}

//nolint:funlen // длинный тест
func TestEnforcer_Stop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		createEnforcer func(t *testing.T, m *mocks.Mockadapter) *Enforcer
		setupMocks     func(t *testing.T, m *mocks.Mockadapter)
		wantErr        require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			createEnforcer: func(t *testing.T, m *mocks.Mockadapter) *Enforcer {
				t.Helper()

				return &Enforcer{
					adapter: m,
				}
			},
			setupMocks: func(t *testing.T, m *mocks.Mockadapter) {
				t.Helper()

				m.EXPECT().Close().Return(nil)
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: close error",
			createEnforcer: func(t *testing.T, m *mocks.Mockadapter) *Enforcer {
				t.Helper()

				return &Enforcer{
					adapter: m,
				}
			},
			setupMocks: func(t *testing.T, m *mocks.Mockadapter) {
				t.Helper()

				m.EXPECT().Close().Return(errors.New("close error"))
			},
			wantErr: require.Error,
		},
		{
			name: "positive case: no adapter",
			createEnforcer: func(t *testing.T, m *mocks.Mockadapter) *Enforcer {
				t.Helper()

				return &Enforcer{
					adapter: nil,
				}
			},
			setupMocks: func(t *testing.T, m *mocks.Mockadapter) {
				t.Helper()
			},
			wantErr: require.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAdapter := mocks.NewMockadapter(ctrl)

			tt.setupMocks(t, mockAdapter)

			enforcer := tt.createEnforcer(t, mockAdapter)

			err := enforcer.Stop(t.Context())
			tt.wantErr(t, err)
		})
	}
}
