package user

import (
	"errors"
	"testing"
	"time"

	"auth-service/internal/service/user/mocks"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест - это ок
func TestNew(t *testing.T) {
	t.Parallel()

	type testMocks struct {
		storage *mocks.MockuserStorage
		cache   *mocks.Mockcache
	}

	tests := []struct {
		name        string
		opts        func(t *testing.T, m *testMocks) []option
		createMocks func(t *testing.T, ctrl *gomock.Controller) *testMocks
		createWant  func(t *testing.T, m *testMocks) *Service
		wantErr     require.ErrorAssertionFunc
		check       func(svc *Service, want *Service)
	}{
		{
			name: "positive case",
			opts: func(t *testing.T, m *testMocks) []option {
				t.Helper()

				return []option{
					WithStorage(m.storage),
					WithCache(m.cache),
					WithCacheTTL(1 * time.Hour),
				}
			},
			createMocks: func(t *testing.T, ctrl *gomock.Controller) *testMocks {
				t.Helper()

				return &testMocks{
					storage: mocks.NewMockuserStorage(ctrl),
					cache:   mocks.NewMockcache(ctrl),
				}
			},
			createWant: func(t *testing.T, m *testMocks) *Service {
				t.Helper()

				return &Service{
					storage:  m.storage,
					cache:    m.cache,
					cacheTTL: 1 * time.Hour,
				}
			},
			wantErr: require.NoError,
			check: func(svc *Service, want *Service) {
				require.NotNil(t, svc)
				assert.Equal(t, want, svc)
			},
		},
		{
			name: "error case: storage is required",
			opts: func(t *testing.T, m *testMocks) []option {
				t.Helper()

				return []option{}
			},
			createMocks: func(t *testing.T, ctrl *gomock.Controller) *testMocks {
				t.Helper()

				return &testMocks{
					cache:   mocks.NewMockcache(ctrl),
					storage: mocks.NewMockuserStorage(ctrl),
				}
			},
			createWant: func(t *testing.T, m *testMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "storage is required")
			},
			check: func(svc *Service, want *Service) {
				require.Nil(t, svc)
			},
		},
		{
			name: "error case: cache is required",
			opts: func(t *testing.T, m *testMocks) []option {
				t.Helper()

				return []option{
					WithStorage(m.storage),
					WithCacheTTL(1 * time.Hour),
				}
			},
			createMocks: func(t *testing.T, ctrl *gomock.Controller) *testMocks {
				t.Helper()

				return &testMocks{
					storage: mocks.NewMockuserStorage(ctrl),
				}
			},
			createWant: func(t *testing.T, m *testMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "cache is required")
			},
			check: func(svc *Service, want *Service) {
				require.Nil(t, svc)
			},
		},
		{
			name: "error case: cache ttl is required",
			opts: func(t *testing.T, m *testMocks) []option {
				t.Helper()

				return []option{
					WithStorage(m.storage),
					WithCache(m.cache),
				}
			},
			createMocks: func(t *testing.T, ctrl *gomock.Controller) *testMocks {
				t.Helper()

				return &testMocks{
					storage: mocks.NewMockuserStorage(ctrl),
					cache:   mocks.NewMockcache(ctrl),
				}
			},
			createWant: func(t *testing.T, m *testMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "cache ttl is required")
			},
			check: func(svc *Service, want *Service) {
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

			m := tt.createMocks(t, ctrl)

			opts := tt.opts(t, m)
			svc, err := New(opts...)
			tt.wantErr(t, err)
			tt.check(svc, tt.createWant(t, m))
		})
	}
}

//nolint:funlen,dupl // длинный тест - это ок, дублирование тест-кейсов - это ок
func TestGetUserIDByTelegramID(t *testing.T) {
	t.Parallel()

	telegramID := "424242"
	wantUUID := uuid.New()

	tests := []struct {
		name       string
		setup      func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache)
		want       uuid.UUID
		wantErr    require.ErrorAssertionFunc
		wantErrMsg string
	}{
		{
			name: "cache hit",
			setup: func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache) {
				t.Helper()

				cache.EXPECT().Get(gomock.Any(), gomock.Any()).Return(wantUUID.String(), nil)
			},
			want: wantUUID,
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.NoError(tt, err)
			},
		},
		{
			name: "cache miss: load from storage and set cache",
			setup: func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache) {
				t.Helper()

				cache.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", redis.Nil)
				storage.EXPECT().GetUserIDByTelegramID(gomock.Any(), telegramID).Return(wantUUID, nil)
				cache.EXPECT().Set(gomock.Any(), telegramID, wantUUID.String(), gomock.Any()).Return(nil)
			},
			want: wantUUID,
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.NoError(tt, err)
			},
		},
		{
			name: "cache error",
			setup: func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache) {
				t.Helper()

				cacheErr := errors.New("cache unavailable")
				cache.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, cacheErr)

				storage.EXPECT().GetUserIDByTelegramID(gomock.Any(), gomock.Any()).Return(wantUUID, nil)
				cache.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(cacheErr)
			},
			want:    wantUUID,
			wantErr: require.NoError,
		},
		{
			name: "storage error on cache miss",
			setup: func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache) {
				t.Helper()

				storeErr := errors.New("user not found")

				cache.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", redis.Nil)
				storage.EXPECT().GetUserIDByTelegramID(gomock.Any(), gomock.Any()).Return(uuid.Nil, storeErr)
			},
			want: uuid.Nil,
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(tt, err, "user not found")
			},
		},
		{
			name: "cached value is not a string",
			setup: func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache) {
				t.Helper()

				cache.EXPECT().Get(gomock.Any(), gomock.Any()).Return(42, nil)

				storage.EXPECT().GetUserIDByTelegramID(gomock.Any(), gomock.Any()).Return(wantUUID, nil)
				cache.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			want: wantUUID,
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.NoError(tt, err)
			},
		},
		{
			name: "cached string is not a valid uuid",
			setup: func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache) {
				t.Helper()

				cache.EXPECT().Get(gomock.Any(), gomock.Any()).Return("not-a-uuid", nil)

				storage.EXPECT().GetUserIDByTelegramID(gomock.Any(), gomock.Any()).Return(wantUUID, nil)
				cache.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			want:    wantUUID,
			wantErr: require.NoError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			storage := mocks.NewMockuserStorage(ctrl)
			cache := mocks.NewMockcache(ctrl)
			tt.setup(t, ctrl, storage, cache)

			svc, err := New(WithStorage(storage), WithCache(cache), WithCacheTTL(1*time.Hour))
			require.NoError(t, err)

			got, err := svc.GetUserIDByTelegramID(t.Context(), telegramID)
			tt.wantErr(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}

//nolint:funlen,dupl // длинный тест - это ок; похожие тест-кейсы
func TestGetUserIDByTelegramIDAndUpdateCache(t *testing.T) {
	t.Parallel()

	telegramID := "424242"
	wantUUID := uuid.New()

	tests := []struct {
		name       string
		setup      func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache)
		wantUUID   uuid.UUID
		wantErr    require.ErrorAssertionFunc
		wantErrMsg string
	}{
		{
			name: "positive case",
			setup: func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache) {
				t.Helper()

				storage.EXPECT().GetUserIDByTelegramID(gomock.Any(), gomock.Any()).Return(wantUUID, nil)
				cache.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			wantUUID: wantUUID,
			wantErr:  require.NoError,
		},
		{
			name: "error case: storage error",
			setup: func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache) {
				t.Helper()

				storage.EXPECT().GetUserIDByTelegramID(gomock.Any(), gomock.Any()).Return(uuid.Nil, errors.New("storage error"))
			},
			wantUUID: uuid.Nil,
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				require.Error(tt, err)
				require.ErrorContains(tt, err, "storage error")
			},
		},
		{
			name: "error case: cache error",
			setup: func(t *testing.T, ctrl *gomock.Controller, storage *mocks.MockuserStorage, cache *mocks.Mockcache) {
				t.Helper()

				storage.EXPECT().GetUserIDByTelegramID(gomock.Any(), gomock.Any()).Return(wantUUID, nil)
				cache.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("cache error"))
			},
			wantUUID: wantUUID,
			wantErr:  require.NoError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			storage := mocks.NewMockuserStorage(ctrl)
			cache := mocks.NewMockcache(ctrl)
			tt.setup(t, ctrl, storage, cache)

			svc, err := New(WithStorage(storage), WithCache(cache), WithCacheTTL(1*time.Hour))
			require.NoError(t, err)

			got, err := svc.getUserIDByTelegramIDAndUpdateCache(t.Context(), telegramID)
			tt.wantErr(t, err)

			assert.Equal(t, tt.wantUUID, got)
		})
	}
}
