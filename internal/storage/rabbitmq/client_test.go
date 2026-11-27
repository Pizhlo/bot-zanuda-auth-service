package rabbitmq

import (
	"context"
	"io"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	valid := []Option{
		WithURL("amqp://localhost:5672"), WithTopics([]string{"test"}),
		WithConnectTimeout(time.Second), WithPublishTimeout(time.Second),
		WithMaxRetries(3), WithRetryBackoff(100 * time.Millisecond),
	}
	ready := &Client{url: "amqp://localhost:5672", topics: []string{"test"}, connectTimeout: time.Second, publishTimeout: time.Second, maxRetries: 3, retryBackoff: 100 * time.Millisecond}

	wantError := func(message string) require.ErrorAssertionFunc {
		return func(t require.TestingT, err error, _ ...any) {
			require.EqualError(t, err, "rabbitmq "+message)
		}
	}
	for _, tc := range []struct {
		name    string
		options []Option
		want    *Client
		wantErr require.ErrorAssertionFunc
	}{
		{"configured", nil, ready, require.NoError},
		{"missing url", []Option{WithURL("")}, nil, wantError("url is required")},
		{"missing topics", []Option{WithTopics(nil)}, nil, wantError("topics are required")},
		{"empty topics", []Option{WithTopics([]string{})}, nil, wantError("topics are required")},
		{"zero connect timeout", []Option{WithConnectTimeout(0)}, nil, wantError("connect timeout is required")},
		{"negative connect timeout", []Option{WithConnectTimeout(-time.Second)}, nil, wantError("connect timeout is required")},
		{"zero publish timeout", []Option{WithPublishTimeout(0)}, nil, wantError("publish timeout is required")},
		{"negative publish timeout", []Option{WithPublishTimeout(-time.Second)}, nil, wantError("publish timeout is required")},
		{"zero retries", []Option{WithMaxRetries(0)}, nil, wantError("max retries is required")},
		{"negative retries", []Option{WithMaxRetries(-1)}, nil, wantError("max retries is required")},
		{"zero backoff", []Option{WithRetryBackoff(0)}, nil, wantError("retry backoff is required")},
		{"negative backoff", []Option{WithRetryBackoff(-time.Second)}, nil, wantError("retry backoff is required")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := append(slices.Clone(valid), tc.options...)

			got, err := NewClient(opts...)
			tc.wantErr(t, err)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestStop(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		setup   func(*testing.T, *Client) context.Context
		wantErr error
	}{
		{"no connection", func(t *testing.T, _ *Client) context.Context { t.Helper(); return t.Context() }, nil},
		{"canceled while another call holds lifecycle lock", func(t *testing.T, c *Client) context.Context {
			t.Helper()
			release, err := c.lockLifecycle(t.Context())
			require.NoError(t, err)
			t.Cleanup(release)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			return ctx
		}, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := &Client{connectTimeout: time.Second}
			ctx := tc.setup(t, c)

			err := c.Stop(ctx)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestPublish(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		client  func(*testing.T) *Client
		wantErr string
	}{
		{"disconnected", func(*testing.T) *Client { return &Client{} }, "connection is not connected"},
		{"missing channel", func(*testing.T) *Client { return &Client{conn: &amqp091.Connection{}} }, "channel is not connected"},
		{"missing transport", func(*testing.T) *Client {
			return &Client{conn: &amqp091.Connection{}, channel: &amqp091.Channel{}, publishTimeout: time.Second}
		}, "transport is not connected"},
		{"incomplete AMQP handshake does not block publish", clientWhileConnecting, "connection is not connected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tc.client(t)

			published := make(chan error, 1)
			go func() { published <- c.Publish(t.Context(), "audit", []byte("{}")) }()

			select {
			case err := <-published:
				require.ErrorContains(t, err, tc.wantErr)
			case <-time.After(3 * time.Second):
				t.Fatal("publish did not return")
			}
		})
	}
}

func TestPublishWithCancellation(t *testing.T) {
	t.Parallel()

	normal := func(t *testing.T, conn net.Conn) (context.Context, net.Conn) { t.Helper(); return t.Context(), conn }
	unexpected := func(t *testing.T, _ net.Conn) error { t.Helper(); t.Error("publish unexpectedly called"); return nil }

	for _, tc := range []struct {
		name          string
		setup         func(*testing.T, net.Conn) (context.Context, net.Conn)
		publish       func(*testing.T, net.Conn) error
		wantErr       require.ErrorAssertionFunc
		wantTransport func(*testing.T, net.Conn, net.Conn)
	}{
		{"completed write leaves transport open", normal, func(*testing.T, net.Conn) error { return nil }, require.NoError, requireTransportOpen},
		{"publisher error is preserved", normal, func(*testing.T, net.Conn) error { return io.ErrShortWrite }, func(t require.TestingT, err error, _ ...any) { require.ErrorIs(t, err, io.ErrShortWrite) }, requireTransportOpen},
		{"missing transport", func(t *testing.T, _ net.Conn) (context.Context, net.Conn) { t.Helper(); return t.Context(), nil }, unexpected, func(t require.TestingT, err error, _ ...any) { require.ErrorContains(t, err, "transport") }, requireTransportOpen},
		{"already canceled", func(t *testing.T, conn net.Conn) (context.Context, net.Conn) {
			t.Helper()
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			return ctx, conn
		}, unexpected, func(t require.TestingT, err error, _ ...any) { require.ErrorIs(t, err, context.Canceled) }, requireTransportOpen},
		{"cancellation interrupts a blocked write", func(t *testing.T, conn net.Conn) (context.Context, net.Conn) {
			t.Helper()
			ctx, cancel := context.WithCancel(t.Context())

			return ctx, &cancelOnWrite{Conn: conn, cancel: cancel}
		}, func(t *testing.T, conn net.Conn) error { t.Helper(); _, err := conn.Write([]byte("audit")); return err }, func(t require.TestingT, err error, _ ...any) { require.ErrorIs(t, err, context.Canceled) }, func(t *testing.T, _, reader net.Conn) {
			t.Helper()

			_, err := reader.Read(make([]byte, 1))
			require.ErrorIs(t, err, io.EOF)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			writer, reader := net.Pipe()

			t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })
			// Ограничиваем время даже при регрессии, оставляющей заблокированный Write.
			require.NoError(t, writer.SetWriteDeadline(time.Now().Add(3*time.Second)))
			require.NoError(t, reader.SetReadDeadline(time.Now().Add(3*time.Second)))
			ctx, transport := tc.setup(t, writer)

			err := publishWithCancellation(ctx, transport, func() error { return tc.publish(t, transport) })
			tc.wantErr(t, err)

			tc.wantTransport(t, writer, reader)
		})
	}
}

type cancelOnWrite struct {
	net.Conn
	cancel context.CancelFunc
}

func (conn *cancelOnWrite) Write(frame []byte) (int, error) {
	conn.cancel()
	return conn.Conn.Write(frame)
}

// clientWhileConnecting удерживает AMQP handshake на локальной TCP-заглушке.
func clientWhileConnecting(t *testing.T) *Client {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	c, err := NewClient(WithURL("amqp://"+listener.Addr().String()), WithTopics([]string{"audit"}), WithConnectTimeout(5*time.Second), WithPublishTimeout(time.Second), WithMaxRetries(1), WithRetryBackoff(time.Millisecond))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())

	connected := make(chan error, 1)
	go func() { connected <- c.Run(ctx) }()

	t.Cleanup(func() {
		cancel()

		select {
		case err := <-connected:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(3 * time.Second):
			t.Error("connection attempt did not finish")
		}
	})
	require.NoError(t, listener.(*net.TCPListener).SetDeadline(time.Now().Add(3*time.Second)))
	conn, err := listener.Accept()
	require.NoError(t, err)
	// Отменяем Run до закрытия второй стороны его TCP-соединения.
	t.Cleanup(func() { cancel(); _ = conn.Close() })

	return c
}

func TestLockLifecycle(t *testing.T) {
	t.Parallel()

	var c Client

	release, err := c.lockLifecycle(t.Context())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = c.lockLifecycle(ctx)

	release()
	require.ErrorIs(t, err, context.Canceled)

	release, err = c.lockLifecycle(t.Context())
	require.NoError(t, err)
	release()
}

func TestRun(t *testing.T) {
	t.Parallel()

	c := Client{connectTimeout: time.Second}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, c.Run(ctx), context.Canceled)
	require.Nil(t, c.conn)
}

func TestConnect(t *testing.T) {
	t.Parallel()

	c := Client{url: "://invalid", connectTimeout: time.Second}

	conn, err := c.connect(t.Context())
	require.Error(t, err)

	require.Nil(t, conn)
}

func requireTransportOpen(t *testing.T, writer, reader net.Conn) {
	t.Helper()
	require.NoError(t, reader.SetReadDeadline(time.Now().Add(time.Second)))

	sent := make(chan error, 1)

	go func() { _, err := writer.Write([]byte{1}); sent <- err }()

	data := make([]byte, 1)
	_, err := io.ReadFull(reader, data)
	require.NoError(t, err)

	require.NoError(t, <-sent)
	require.Equal(t, byte(1), data[0])
}
