package rabbitmq

import (
	"context"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
)

func TestBlockedPublishIsInterrupted(t *testing.T) {
	t.Parallel()

	writer, reader := net.Pipe()

	t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	entered := make(chan struct{})

	done := make(chan error, 1)
	go func() {
		done <- publishWithCancellation(ctx, writer, func() error { close(entered); _, err := writer.Write([]byte("audit")); return err })
	}()

	<-entered
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestPublishCancellationDoesNotCloseHealthyTransport(t *testing.T) {
	t.Parallel()

	writer, reader := net.Pipe()

	t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })
	require.NoError(t, publishWithCancellation(t.Context(), writer, func() error { return nil }))

	sent := make(chan error, 1)

	go func() { _, err := writer.Write([]byte{1}); sent <- err }()

	data := make([]byte, 1)
	_, err := reader.Read(data)
	require.NoError(t, err)
	require.NoError(t, <-sent)
	require.Equal(t, byte(1), data[0])
	require.ErrorContains(t, publishWithCancellation(t.Context(), nil, func() error { return nil }), "transport")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, publishWithCancellation(ctx, writer, func() error { t.Fatal("canceled publish executed"); return nil }), context.Canceled)
}
