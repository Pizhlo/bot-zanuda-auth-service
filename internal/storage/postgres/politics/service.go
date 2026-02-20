package politics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	sqldblogger "github.com/simukti/sqldb-logger"
	"github.com/simukti/sqldb-logger/logadapter/logrusadapter"
	"github.com/sirupsen/logrus"

	_ "github.com/lib/pq" // postgres driver
)

// Repo сохраняет сообщения и результаты их обработки в базу данных.
type Repo struct {
	addr string
	db   *sql.DB

	insertTimeout time.Duration
	readTimeout   time.Duration

	transaction struct {
		mu sync.Mutex
		tx map[string]*sql.Tx
	}
}

// RepoOption определяет опции для репозитория.
type RepoOption func(*Repo)

// WithAddr устанавливает адрес базы данных.
func WithAddr(addr string) RepoOption {
	return func(r *Repo) {
		r.addr = addr
	}
}

// WithInsertTimeout устанавливает время ожидания вставки.
func WithInsertTimeout(insertTimeout time.Duration) RepoOption {
	return func(c *Repo) {
		c.insertTimeout = insertTimeout
	}
}

// WithReadTimeout устанавливает время ожидания чтения.
func WithReadTimeout(readTimeout time.Duration) RepoOption {
	return func(c *Repo) {
		c.readTimeout = readTimeout
	}
}

// New создает новый репозиторий.
func New(ctx context.Context, opts ...RepoOption) (*Repo, error) {
	r := &Repo{}

	for _, opt := range opts {
		opt(r)
	}

	if r.insertTimeout == 0 {
		return nil, fmt.Errorf("insert timeout is required")
	}

	if r.readTimeout == 0 {
		return nil, fmt.Errorf("read timeout is required")
	}

	if r.addr == "" {
		return nil, errors.New("addr is required")
	}

	db, err := sql.Open("postgres", r.addr)
	if err != nil {
		return nil, fmt.Errorf("connect open a db driver: %w", err)
	}

	logger := logrus.New()
	logger.Level = logrus.DebugLevel           // miminum level
	logger.Formatter = &logrus.TextFormatter{} // logrus automatically add time field

	drv := db.Driver()

	if err := db.Close(); err != nil {
		return nil, fmt.Errorf("close raw db: %w", err)
	}

	db = sqldblogger.OpenDriver(r.addr, drv, logrusadapter.New(logger) /*, using_default_options*/) // db is STILL *sql.DB

	r.transaction = struct {
		mu sync.Mutex
		tx map[string]*sql.Tx
	}{mu: sync.Mutex{}, tx: make(map[string]*sql.Tx)}

	r.db = db

	return r, nil
}

// Stop закрывает репозиторий.
func (db *Repo) Stop(_ context.Context) error {
	return db.db.Close()
}

// Run запускает репозиторий.
func (db *Repo) Run(ctx context.Context) error {
	if err := db.db.PingContext(ctx); err != nil {
		return fmt.Errorf("error pinging db: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"addr": db.addr,
	}).Info("successfully connected postgres")

	return nil
}
