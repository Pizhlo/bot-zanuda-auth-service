package rabbitmq

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// Client - клиент RabbitMQ.
// Читает и пишет сообщения в topics.
type Client struct {
	url string

	connectTimeout time.Duration
	publishTimeout time.Duration
	maxRetries     int
	retryBackoff   time.Duration

	mu      sync.Mutex
	conn    *amqp091.Connection
	channel *amqp091.Channel
	topics  []string
}

// Option - опция для настройки клиента RabbitMQ.
type Option func(*Client)

// WithURL устанавливает URL для соединения с RabbitMQ.
func WithURL(url string) Option {
	return func(s *Client) {
		s.url = url
	}
}

// WithConnectTimeout устанавливает время ожидания соединения с RabbitMQ.
func WithConnectTimeout(connectTimeout time.Duration) Option {
	return func(s *Client) {
		s.connectTimeout = connectTimeout
	}
}

// WithPublishTimeout устанавливает время ожидания отправки сообщения в RabbitMQ.
func WithPublishTimeout(publishTimeout time.Duration) Option {
	return func(s *Client) {
		s.publishTimeout = publishTimeout
	}
}

// WithTopics устанавливает очереди для чтения / отправки сообщений.
func WithTopics(topics []string) Option {
	return func(s *Client) {
		s.topics = topics
	}
}

// WithMaxRetries устанавливает максимальное количество попыток отправки сообщения в RabbitMQ.
func WithMaxRetries(maxRetries int) Option {
	return func(s *Client) {
		s.maxRetries = maxRetries
	}
}

// WithRetryBackoff устанавливает время ожидания между попытками отправки сообщения в RabbitMQ.
func WithRetryBackoff(retryBackoff time.Duration) Option {
	return func(s *Client) {
		s.retryBackoff = retryBackoff
	}
}

// NewClient создает новый клиент RabbitMQ.
func NewClient(opts ...Option) (*Client, error) {
	s := &Client{}

	for _, opt := range opts {
		opt(s)
	}

	if s.url == "" {
		return nil, fmt.Errorf("rabbitmq url is required")
	}

	if len(s.topics) == 0 {
		return nil, fmt.Errorf("rabbitmq topics are required")
	}

	if s.connectTimeout == 0 {
		return nil, fmt.Errorf("rabbitmq connect timeout is required")
	}

	if s.publishTimeout == 0 {
		return nil, fmt.Errorf("rabbitmq publish timeout is required")
	}

	if s.maxRetries == 0 {
		return nil, fmt.Errorf("rabbitmq max retries is required")
	}

	if s.retryBackoff == 0 {
		return nil, fmt.Errorf("rabbitmq retry backoff is required")
	}

	return s, nil
}

// Run создает соединение с RabbitMQ и декларирует очереди.
func (s *Client) Run(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil && s.channel != nil && !s.channel.IsClosed() && !s.conn.IsClosed() {
		return nil
	}

	bo := backoff.WithMaxRetries(
		backoff.NewExponentialBackOff(
			backoff.WithMaxElapsedTime(s.connectTimeout),
			backoff.WithMaxInterval(s.retryBackoff*10),
		),
		uint64(s.maxRetries),
	)

	var (
		conn *amqp091.Connection
		ch   *amqp091.Channel
	)

	operation := func() error {
		// Dial
		c, err := amqp091.DialConfig(s.url, amqp091.Config{
			Dial: amqp091.DefaultDial(s.connectTimeout),
		})
		if err != nil {
			return fmt.Errorf("dial: %w", err)
		}

		// Channel
		channel, err := c.Channel()
		if err != nil {
			_ = c.Close()
			return fmt.Errorf("channel: %w", err)
		}

		// Declare queues (active, idempotent)
		if err := declareQueues(channel, s.topics); err != nil {
			_ = channel.Close()
			_ = c.Close()

			return fmt.Errorf("declare queues: %w", err)
		}

		// Success
		conn, ch = c, channel

		return nil
	}

	err := backoff.Retry(operation, bo)
	if err != nil {
		return fmt.Errorf("connect retries exhausted (%d attempts): %w", s.maxRetries, err)
	}

	s.conn = conn
	s.channel = ch

	logrus.WithFields(logrus.Fields{
		"topics": len(s.topics),
	}).Info("rabbitmq connected")

	return nil
}

func declareQueues(channel *amqp091.Channel, topics []string) error {
	for _, topic := range topics {
		_, err := channel.QueueDeclare(
			topic, true, false, false, false, amqp091.Table{
				"x-queue-type": "quorum",
			},
		)
		if err != nil {
			return fmt.Errorf("queue %s: %w", topic, err)
		}
	}

	return nil
}

// Stop закрывает соединение с RabbitMQ.
func (s *Client) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil && s.channel == nil {
		return nil
	}

	if err := s.conn.Close(); err != nil {
		return fmt.Errorf("rabbitmq close connection: %w", err)
	}

	logrus.Info("rabbitmq client stopped")

	s.conn = nil
	s.channel = nil

	return nil
}

// Publish отправляет сообщение в RabbitMQ.
func (s *Client) Publish(ctx context.Context, topic string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil {
		return fmt.Errorf("rabbitmq connection is not connected")
	}

	if s.channel == nil {
		return fmt.Errorf("rabbitmq channel is not connected")
	}

	ch := s.channel

	ctx, cancel := context.WithTimeout(ctx, s.publishTimeout)
	defer cancel()

	return ch.PublishWithContext(ctx, "", topic, false, false, amqp091.Publishing{
		ContentType: "application/json",
		Body:        data,
	})
}
