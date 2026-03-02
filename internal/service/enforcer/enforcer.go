package enforcer

import (
	"context"
	"fmt"

	pgadapter "github.com/casbin/casbin-pg-adapter" // пример, зависит от выбранного адаптера
	"github.com/casbin/casbin/v2"
	"github.com/sirupsen/logrus"
)

// Enforcer - сервис для работы с Casbin.
type Enforcer struct {
	dsn       string
	e         enforcer
	adapter   adapter
	modelConf string
}

//go:generate mockgen -source=enforcer.go -destination=mocks/adapter_mock.go -package=mocks
type adapter interface {
	Close() error
}

type enforcer interface {
	LoadPolicy() error
}

type option func(*Enforcer)

// WithDsn устанавливает DSN для подключения к базе данных.
func WithDsn(dsn string) option {
	return func(e *Enforcer) {
		e.dsn = dsn
	}
}

// WithModelConf устанавливает конфигурацию модели для Casbin.
func WithModelConf(modelConf string) option {
	return func(e *Enforcer) {
		e.modelConf = modelConf
	}
}

// NewEnforcer создает новый экземпляр Enforcer.
func NewEnforcer(opts ...option) (*Enforcer, error) {
	enforcer := &Enforcer{}

	for _, opt := range opts {
		opt(enforcer)
	}

	if len(enforcer.dsn) == 0 {
		return nil, fmt.Errorf("dsn is required")
	}

	if len(enforcer.modelConf) == 0 {
		return nil, fmt.Errorf("model conf is required")
	}

	logrus.WithFields(
		logrus.Fields{
			"model": enforcer.modelConf,
		},
	).Info("enforcer created")

	return enforcer, nil
}

// Run запускает работу enforcer'а.
// Соединяется с базой данных и загружает политику.
func (e *Enforcer) Run(_ context.Context) error {
	adapter, err := pgadapter.NewAdapter(e.dsn)
	if err != nil {
		return fmt.Errorf("error creating adapter: %w", err)
	}

	e.adapter = adapter

	enf, err := casbin.NewEnforcer(e.modelConf, adapter)
	if err != nil {
		return fmt.Errorf("error creating enforcer: %w", err)
	}

	e.e = enf

	logrus.WithFields(
		logrus.Fields{
			"model": e.modelConf,
		},
	).Info("enforcer started")

	return nil
}

// Stop разрывает соединение с адаптером.
func (e *Enforcer) Stop(_ context.Context) error {
	logrus.WithFields(
		logrus.Fields{
			"model": e.modelConf,
		},
	).Info("enforcer stopping")

	if e.adapter != nil {
		return e.adapter.Close()
	}

	return nil
}
