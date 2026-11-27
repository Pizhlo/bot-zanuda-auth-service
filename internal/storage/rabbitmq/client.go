package rabbitmq

import (
	"context"
	"fmt"
	"net"
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

	lifecycle chan struct{} // Сериализует Run/Stop с отменой ожидания; Publish не ждёт.
	mu        sync.RWMutex
	transport net.Conn
	conn      *amqp091.Connection
	channel   *amqp091.Channel
	topics    []string
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

// WithMaxRetries задаёт число повторных попыток подключения после первой неудачи.
// Publish не повторяет отправку: при потере ответа повтор мог бы дублировать событие.
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

	if s.connectTimeout <= 0 {
		return nil, fmt.Errorf("rabbitmq connect timeout is required")
	}

	if s.publishTimeout <= 0 {
		return nil, fmt.Errorf("rabbitmq publish timeout is required")
	}

	if s.maxRetries <= 0 {
		return nil, fmt.Errorf("rabbitmq max retries is required")
	}

	if s.retryBackoff <= 0 {
		return nil, fmt.Errorf("rabbitmq retry backoff is required")
	}

	return s, nil
}

// lockLifecycle ожидает право менять соединение, соблюдая общий бюджет Run/Stop.
// Под mu только инициализируется семафор; ожидание выполняется вне блокировки состояния.
func (s *Client) lockLifecycle(ctx context.Context) (func(), error) {
	s.mu.Lock()
	if s.lifecycle == nil {
		s.lifecycle = make(chan struct{}, 1)
	}

	lifecycle := s.lifecycle
	s.mu.Unlock()

	select {
	case lifecycle <- struct{}{}:
		release := func() { <-lifecycle }
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}

		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Run подключается и объявляет очереди в пределах ConnectTimeout и контекста.
// Повторы касаются только подключения; готовое состояние заменяется атомарно,
// поэтому Publish не ждёт сетевого I/O или задержки между попытками.
func (s *Client) Run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.connectTimeout)
	defer cancel()

	release, err := s.lockLifecycle(ctx)
	if err != nil {
		return err
	}
	defer release()

	s.mu.RLock()
	connected := s.conn != nil && s.channel != nil && !s.channel.IsClosed() && !s.conn.IsClosed()
	s.mu.RUnlock()

	if connected {
		return nil
	}

	bo := backoff.WithMaxRetries(
		backoff.NewExponentialBackOff(
			backoff.WithMaxElapsedTime(s.connectTimeout),
			backoff.WithMaxInterval(s.retryBackoff*10),
		),
		uint64(s.maxRetries),
	)

	var connection *brokerConnection

	operation := func() error {
		var err error

		connection, err = s.connect(ctx)

		return err
	}

	if err := backoff.Retry(operation, backoff.WithContext(bo, ctx)); err != nil {
		return fmt.Errorf("connect retries exhausted (%d retries): %w", s.maxRetries, err)
	}

	s.mu.Lock()
	previous := s.transport
	s.conn, s.channel, s.transport = connection.conn, connection.channel, connection.transport
	s.mu.Unlock()

	if previous != nil {
		_ = previous.Close()
	}

	logrus.WithFields(logrus.Fields{
		"topics": len(s.topics),
	}).Info("rabbitmq connected")

	return nil
}

// brokerConnection публикуется в состоянии Client только после объявления очередей.
// До этого транспорт принадлежит connect и закрывается при любой ошибке или отмене.
type brokerConnection struct {
	conn      *amqp091.Connection
	channel   *amqp091.Channel
	transport net.Conn
}

func (s *Client) connect(ctx context.Context) (*brokerConnection, error) {
	connection := &brokerConnection{}
	stopCancellation := func() bool { return true }
	succeeded := false

	defer func() {
		stopCancellation()

		if !succeeded && connection.transport != nil {
			_ = connection.transport.Close()
		}
	}()

	conn, err := amqp091.DialConfig(s.url, amqp091.Config{
		Dial: func(network, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: s.connectTimeout}

			raw, err := dialer.DialContext(ctx, network, address)
			if err == nil {
				connection.transport = raw
				stopCancellation = context.AfterFunc(ctx, func() { _ = raw.Close() })
			}

			return raw, err
		},
	})
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	connection.conn = conn

	channel, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("channel: %w", err)
	}

	connection.channel = channel

	if err := declareQueues(channel, s.topics); err != nil {
		return nil, fmt.Errorf("declare RabbitMQ queues: %w", err)
	}
	// Отмену надо отключить до передачи владения: запущенный callback мог уже
	// закрыть транспорт, даже если предыдущая проверка ctx.Err ещё была успешной.
	if !stopCancellation() {
		return nil, ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	succeeded = true

	return connection, nil
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

// Stop сначала убирает соединение из доступного состояния, затем закрывает его.
// Параллельный Publish получает ошибку вместо ожидания сетевого Close.
func (s *Client) Stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.connectTimeout)
	defer cancel()

	release, err := s.lockLifecycle(ctx)
	if err != nil {
		return err
	}
	defer release()

	s.mu.Lock()
	conn, transport := s.conn, s.transport
	s.conn, s.channel, s.transport = nil, nil, nil
	s.mu.Unlock()

	if conn == nil {
		return nil
	}

	// Сетевой Close не удерживает mutex состояния: новые Publish сразу получают ошибку.
	stop := context.AfterFunc(ctx, func() {
		if transport != nil {
			_ = transport.Close()
		}
	})
	defer stop()

	if err := conn.Close(); err != nil {
		return fmt.Errorf("rabbitmq close connection: %w", err)
	}

	logrus.Info("rabbitmq client stopped")

	return nil
}

// Publish отправляет одно JSON-сообщение без повторов, в пределах PublishTimeout.
// При отмене закрывается общий транспорт: другие текущие публикации тоже могут
// завершиться ошибкой, а для восстановления соединения требуется Run.
func (s *Client) Publish(ctx context.Context, topic string, data []byte) error {
	s.mu.RLock()
	conn, ch, transport := s.conn, s.channel, s.transport
	s.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("rabbitmq connection is not connected")
	}

	if ch == nil {
		return fmt.Errorf("rabbitmq channel is not connected")
	}

	ctx, cancel := context.WithTimeout(ctx, s.publishTimeout)
	defer cancel()

	return publishWithCancellation(ctx, transport, func() error {
		return ch.PublishWithContext(ctx, "", topic, false, false, amqp091.Publishing{
			ContentType: "application/json", Body: data,
		})
	})
}

// SDK не прерывает начатый Publish по context. Закрытие транспорта освобождает
// сетевую запись и ожидающие её публикации, не создавая фоновых зависших workers.
func publishWithCancellation(ctx context.Context, transport net.Conn, publish func() error) error {
	if transport == nil {
		return fmt.Errorf("rabbitmq transport is not connected")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	stop := context.AfterFunc(ctx, func() { _ = transport.Close() })
	defer stop()

	err := publish()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return err
}
