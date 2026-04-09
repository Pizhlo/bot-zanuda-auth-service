package audit

import (
	"context"
	"encoding/json"
	"fmt"
)

// sender - сервис для отправки событий аудита в систему (rabbitmq, kafka, etc).
// Не зависит от конкретной системы, поэтому можно переиспользовать для разных систем.
type sender struct {
	client client
	topic  string // топик для отправки событий аудита
}

// client - интерфейс для отправки событий аудита в систему (rabbitmq, kafka, etc).
//
//go:generate mockgen -source=sender.go -destination=mocks/mocks.go -package=mocks client
type client interface {
	Publish(ctx context.Context, topic string, data []byte) error
}

type option func(*sender)

// WithClient устанавливает клиент для отправки событий аудита.
func WithClient(client client) option {
	return func(s *sender) {
		s.client = client
	}
}

// WithTopic устанавливает топик для отправки событий аудита.
func WithTopic(topic string) option {
	return func(s *sender) {
		s.topic = topic
	}
}

// NewSender создает новый sender.
func NewSender(opts ...option) (Sender, error) {
	s := &sender{}

	for _, opt := range opts {
		opt(s)
	}

	if s.client == nil {
		return nil, fmt.Errorf("client is required")
	}

	if s.topic == "" {
		return nil, fmt.Errorf("topic is required")
	}

	return s, nil
}

func (s *sender) Send(ctx context.Context, fields map[fieldID]any) error {
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}

	return s.client.Publish(ctx, s.topic, data)
}
