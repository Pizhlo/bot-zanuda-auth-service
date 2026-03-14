package access

import (
	"auth-service/internal/model"
	"auth-service/internal/service/politics/access/mocks"
	"auth-service/internal/storage"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест
func TestSpaceAccessChecker_CheckSpaceAccess(t *testing.T) {
	t.Parallel()

	spaceID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name         string
		userID       uuid.UUID
		spaceID      uuid.UUID
		prepareMocks func(ctrl *gomock.Controller) *mocks.MockspacesRepo
		want         model.SpaceMember
		wantErr      require.ErrorAssertionFunc
	}{
		{
			name:    "user is member",
			userID:  userID,
			spaceID: spaceID,
			prepareMocks: func(ctrl *gomock.Controller) *mocks.MockspacesRepo {
				t.Helper()

				mockRepo := mocks.NewMockspacesRepo(ctrl)
				mockRepo.EXPECT().
					GetSpaceMember(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(model.SpaceMember{
						UserID:   userID,
						RoleCode: model.EditorRoleCode,
						Status:   model.ActiveMemberStatus,
					}, nil)

				return mockRepo
			},
			want: model.SpaceMember{
				UserID:   userID,
				IsMember: true,
				RoleCode: model.EditorRoleCode,
				Status:   model.ActiveMemberStatus,
			},
			wantErr: require.NoError,
		},
		{
			name:    "user not found in space",
			userID:  userID,
			spaceID: spaceID,
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
			userID:  userID,
			spaceID: spaceID,
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

			got, err := svc.CheckSpaceAccess(t.Context(), tt.userID, tt.spaceID)
			tt.wantErr(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}
