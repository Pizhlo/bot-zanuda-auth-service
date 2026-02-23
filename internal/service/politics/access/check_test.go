package access

import (
	"auth-service/internal/model"
	"auth-service/internal/service/politics/access/mocks"
	"auth-service/internal/storage"
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест
func TestSpaceAccessChecker_CheckSpaceAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		userID       int
		spaceID      int
		prepareMocks func(ctrl *gomock.Controller) *mocks.MockspacesRepo
		want         model.SpaceMember
		wantErr      require.ErrorAssertionFunc
	}{
		{
			name:    "user is member",
			userID:  1,
			spaceID: 10,
			prepareMocks: func(ctrl *gomock.Controller) *mocks.MockspacesRepo {
				t.Helper()

				mockRepo := mocks.NewMockspacesRepo(ctrl)
				mockRepo.EXPECT().
					GetSpaceMember(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(model.SpaceMember{
						UserID:   1,
						RoleCode: string(model.EditorRoleCode),
						Status:   model.ActiveMemberStatus,
					}, nil)

				return mockRepo
			},
			want: model.SpaceMember{
				UserID:   1,
				IsMember: true,
				RoleCode: string(model.EditorRoleCode),
				Status:   model.ActiveMemberStatus,
			},
			wantErr: require.NoError,
		},
		{
			name:    "user not found in space",
			userID:  2,
			spaceID: 10,
			prepareMocks: func(ctrl *gomock.Controller) *mocks.MockspacesRepo {
				t.Helper()

				mockRepo := mocks.NewMockspacesRepo(ctrl)
				mockRepo.EXPECT().
					GetSpaceMember(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(model.SpaceMember{}, storage.ErrNotFound)

				return mockRepo
			},
			want: model.SpaceMember{
				IsMember: false,
			},
			wantErr: require.NoError,
		},
		{
			name:    "repo returns error",
			userID:  1,
			spaceID: 10,
			prepareMocks: func(ctrl *gomock.Controller) *mocks.MockspacesRepo {
				t.Helper()

				mockRepo := mocks.NewMockspacesRepo(ctrl)
				mockRepo.EXPECT().
					GetSpaceMember(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(model.SpaceMember{}, errors.New("db connection failed"))

				return mockRepo
			},
			want: model.SpaceMember{},
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(tt, err, "error getting space member")
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := tt.prepareMocks(ctrl)
			svc := &SpaceAccessChecker{spacesRepo: mockRepo}

			got, err := svc.CheckSpaceAccess(context.Background(), tt.userID, tt.spaceID)
			tt.wantErr(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}
