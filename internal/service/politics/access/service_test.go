package access

import (
	"auth-service/internal/service/politics/access/mocks"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		createOpts func(t *testing.T, ctrl *gomock.Controller) []option
		wantErr    require.ErrorAssertionFunc
		check      func(t *testing.T, svc *SpaceAccessChecker)
	}{
		{
			name: "positive case",
			createOpts: func(t *testing.T, ctrl *gomock.Controller) []option {
				t.Helper()

				mockRepo := mocks.NewMockspacesRepo(ctrl)

				return []option{
					WithStorage(mockRepo),
				}
			},
			wantErr: require.NoError,
			check: func(t *testing.T, svc *SpaceAccessChecker) {
				t.Helper()
				require.NotNil(t, svc)
				require.NotNil(t, svc.spacesRepo)
			},
		},
		{
			name: "error case: storage is required",
			createOpts: func(t *testing.T, _ *gomock.Controller) []option {
				t.Helper()

				return nil
			},
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(tt, err, "storage is required")
			},
			check: func(t *testing.T, svc *SpaceAccessChecker) {
				t.Helper()
				require.Nil(t, svc)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc, err := New(tt.createOpts(t, ctrl)...)
			tt.wantErr(t, err)

			tt.check(t, svc)
		})
	}
}
