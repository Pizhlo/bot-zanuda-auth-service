package rabbitmq

import (
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест
func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options []Option
		want    *Client
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			options: []Option{
				WithURL("amqp://localhost:5672"),
				WithTopics([]string{"test"}),
				WithConnectTimeout(1 * time.Second),
				WithPublishTimeout(1 * time.Second),
				WithMaxRetries(3),
				WithRetryBackoff(100 * time.Millisecond),
			},
			want: &Client{
				url:            "amqp://localhost:5672",
				topics:         []string{"test"},
				connectTimeout: 1 * time.Second,
				publishTimeout: 1 * time.Second,
				maxRetries:     3,
				retryBackoff:   100 * time.Millisecond,
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: url is required",
			options: []Option{
				WithTopics([]string{"test"}),
				WithConnectTimeout(1 * time.Second),
				WithPublishTimeout(1 * time.Second),
				WithMaxRetries(3),
				WithRetryBackoff(100 * time.Millisecond),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq url is required")
			},
		},
		{
			name: "error case: topics are required",
			options: []Option{
				WithURL("amqp://localhost:5672"),
				WithConnectTimeout(1 * time.Second),
				WithPublishTimeout(1 * time.Second),
				WithMaxRetries(3),
				WithRetryBackoff(100 * time.Millisecond),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq topics are required")
			},
		},
		{
			name: "error case: connect timeout is required",
			options: []Option{
				WithURL("amqp://localhost:5672"),
				WithTopics([]string{"test"}),
				WithPublishTimeout(1 * time.Second),
				WithMaxRetries(3),
				WithRetryBackoff(100 * time.Millisecond),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq connect timeout is required")
			},
		},
		{
			name: "error case: publish timeout is required",
			options: []Option{
				WithURL("amqp://localhost:5672"),
				WithTopics([]string{"test"}),
				WithConnectTimeout(1 * time.Second),
				WithMaxRetries(3),
				WithRetryBackoff(100 * time.Millisecond),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq publish timeout is required")
			},
		},
		{
			name: "error case: max retries is required",
			options: []Option{
				WithURL("amqp://localhost:5672"),
				WithTopics([]string{"test"}),
				WithConnectTimeout(1 * time.Second),
				WithPublishTimeout(1 * time.Second),
				WithRetryBackoff(100 * time.Millisecond),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq max retries is required")
			},
		},
		{
			name: "error case: retry backoff is required",
			options: []Option{
				WithURL("amqp://localhost:5672"),
				WithTopics([]string{"test"}),
				WithConnectTimeout(1 * time.Second),
				WithPublishTimeout(1 * time.Second),
				WithMaxRetries(3),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "rabbitmq retry backoff is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewClient(tt.options...)
			tt.wantErr(t, err)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestStop(t *testing.T) {
	t.Parallel()

	// можем протестировать только случай conn == nil
	client := &Client{}

	require.NoError(t, client.Stop(t.Context()))
}

func TestPublish(t *testing.T) {
	t.Parallel()

	// можем протестировать только случай conn == nil
	client := &Client{
		url: "amqp://localhost:5672",
	}

	err := client.Publish(t.Context(), "test", []byte("test"))
	require.ErrorContains(t, err, "rabbitmq connection is not connected")

	client.conn = &amqp091.Connection{}
	require.ErrorContains(t, client.Publish(t.Context(), "test", []byte("test")), "rabbitmq channel is not connected")
}
