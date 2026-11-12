package rabbitmq

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestRealAuditTransport(t *testing.T) {
	t.Parallel()

	address := os.Getenv("AUTH_TEST_RABBITMQ_URL")
	if address == "" {
		t.Skip("set AUTH_TEST_RABBITMQ_URL to an isolated local RabbitMQ")
	}

	parsed, err := url.Parse(address)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, parsed.Hostname())

	topic := "bz167-" + uuid.NewString()
	client, err := NewClient(WithURL(address), WithTopics([]string{topic}), WithConnectTimeout(5*time.Second), WithPublishTimeout(time.Second), WithMaxRetries(3), WithRetryBackoff(time.Millisecond*50))
	require.NoError(t, err)
	require.NoError(t, client.Run(t.Context()))
	t.Cleanup(func() { require.NoError(t, client.Stop(t.Context())) })
	require.NoError(t, client.Run(t.Context()))

	conn, err := amqp.Dial(address)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })

	channel, err := conn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := channel.QueueDelete(topic, false, false, false)
		require.NoError(t, err)
		require.NoError(t, channel.Close())
	})

	deliveries, err := channel.Consume(topic, "", true, false, false, false, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	require.NoError(t, client.Publish(ctx, topic, []byte(`{"decision":"allow"}`)))

	select {
	case msg := <-deliveries:
		require.JSONEq(t, `{"decision":"allow"}`, string(msg.Body))
	case <-ctx.Done():
		t.Fatal("audit message was not delivered", ctx.Err())
	}

	require.NoError(t, client.Stop(t.Context()))
	require.ErrorContains(t, client.Publish(t.Context(), topic, []byte("test")), "not connected")
}
