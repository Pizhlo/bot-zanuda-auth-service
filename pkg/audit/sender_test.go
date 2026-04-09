package audit

import (
	"context"
	"testing"

	"auth-service/pkg/audit/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест - это ок
func TestNewSender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      func(client *mocks.Mockclient) []option
		checkWant func(client *mocks.Mockclient, actual Sender)
		wantErr   require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			opts: func(client *mocks.Mockclient) []option {
				t.Helper()

				return []option{
					WithClient(client),
					WithTopic("test"),
				}
			},
			checkWant: func(client *mocks.Mockclient, actual Sender) {
				t.Helper()

				s := sender{
					client: client,
					topic:  "test",
				}

				require.Equal(t, &s, actual)
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: client is required",
			opts: func(client *mocks.Mockclient) []option {
				t.Helper()

				return []option{
					WithClient(nil),
					WithTopic("test"),
				}
			},
			checkWant: func(client *mocks.Mockclient, actual Sender) {
				t.Helper()

				require.Nil(t, actual)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "client is required")
			},
		},
		{
			name: "error case: topic is required",
			opts: func(client *mocks.Mockclient) []option {
				t.Helper()

				return []option{
					WithClient(client),
				}
			},
			checkWant: func(client *mocks.Mockclient, actual Sender) {
				t.Helper()

				require.Nil(t, actual)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "topic is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockclient(ctrl)
			actual, err := NewSender(tt.opts(mockClient)...)
			tt.wantErr(t, err)

			tt.checkWant(mockClient, actual)
		})
	}
}

func TestSender_Send(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fields     map[fieldID]any
		setupMocks func(t *testing.T, client *mocks.Mockclient)
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			fields: map[fieldID]any{
				fieldServiceName: "test",
			},
			setupMocks: func(t *testing.T, client *mocks.Mockclient) {
				t.Helper()

				client.EXPECT().Publish(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			wantErr: require.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockclient(ctrl)

			sender, err := NewSender(WithClient(mockClient), WithTopic("test"))
			tt.wantErr(t, err)

			tt.setupMocks(t, mockClient)

			err = sender.Send(context.Background(), tt.fields)
			tt.wantErr(t, err)
		})
	}
}
