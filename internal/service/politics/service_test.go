package politics

import (
	"auth-service/internal/service/politics/mocks"
	"auth-service/pkg/audit/testaudit"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testMocks struct {
	storage      *mocks.Mockstorage
	spaceChecker *mocks.MockspaceAccessChecker
	noteResolver *mocks.MocknotePermissionResolver
	auditor      auditor
}

//nolint:funlen, dupl // длинный тест - это ок, похожие тест-кейсы
func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		createOpts func(t *testing.T, m *testMocks) []option
		createWant func(t *testing.T, m *testMocks) *Service
		check      func(svc *Service, want *Service)
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			createOpts: func(t *testing.T, m *testMocks) []option {
				t.Helper()

				return []option{
					WithStorage(m.storage),
					WithSpaceAccessChecker(m.spaceChecker),
					WithNotePermissionResolver(m.noteResolver),
					WithAuditor(m.auditor),
				}
			},
			createWant: func(t *testing.T, m *testMocks) *Service {
				t.Helper()

				return &Service{
					storage:                m.storage,
					spaceAccessChecker:     m.spaceChecker,
					notePermissionResolver: m.noteResolver,
					auditor:                m.auditor,
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
			createOpts: func(t *testing.T, m *testMocks) []option {
				t.Helper()

				return []option{}
			},
			createWant: func(t *testing.T, m *testMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, _ ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "storage is required")
			},
			check: func(svc *Service, want *Service) {
				require.Nil(t, svc)
			},
		},
		{
			name: "error case: space access checker is required",
			createOpts: func(t *testing.T, m *testMocks) []option {
				t.Helper()

				return []option{
					WithStorage(m.storage),
				}
			},
			createWant: func(t *testing.T, m *testMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, _ ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "space access checker is required")
			},
			check: func(svc *Service, want *Service) {
				require.Nil(t, svc)
			},
		},
		{
			name: "error case: note permission resolver is required",
			createOpts: func(t *testing.T, m *testMocks) []option {
				t.Helper()

				return []option{
					WithStorage(m.storage),
					WithSpaceAccessChecker(m.spaceChecker),
					WithAuditor(m.auditor),
				}
			},
			createWant: func(t *testing.T, m *testMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, _ ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "note permission resolver is required")
			},
			check: func(svc *Service, want *Service) {
				require.Nil(t, svc)
			},
		},
		{
			name: "error case: auditor is required",
			createOpts: func(t *testing.T, m *testMocks) []option {
				t.Helper()

				return []option{
					WithStorage(m.storage),
					WithSpaceAccessChecker(m.spaceChecker),
					WithNotePermissionResolver(m.noteResolver),
				}
			},
			createWant: func(t *testing.T, m *testMocks) *Service {
				t.Helper()

				return nil
			},
			wantErr: func(t require.TestingT, err error, _ ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "auditor is required")
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

			m := &testMocks{
				storage:      mocks.NewMockstorage(ctrl),
				spaceChecker: mocks.NewMockspaceAccessChecker(ctrl),
				noteResolver: mocks.NewMocknotePermissionResolver(ctrl),
				auditor:      testaudit.NewAuditor(t),
			}

			svc, err := New(tt.createOpts(t, m)...)
			tt.wantErr(t, err)

			tt.check(svc, tt.createWant(t, m))
		})
	}
}
