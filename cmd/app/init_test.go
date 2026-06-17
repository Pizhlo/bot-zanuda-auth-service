package main

import (
	"auth-service/internal/config"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatPostgresAddr(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Postgres: config.Postgres{
			Host:     "localhost",
			Port:     5432,
			User:     "user",
			Password: "password",
			DBName:   "db",
		},
	}

	addr := formatPostgresAddr(cfg.Postgres)

	assert.Equal(t, "postgresql://user:password@localhost:5432/db?sslmode=disable", addr)
}

func TestStartService(t *testing.T) {
	t.Parallel()

	err := errors.New("test error")

	exitCode := -1
	oldExit := logrus.StandardLogger().ExitFunc
	logrus.StandardLogger().ExitFunc = func(code int) {
		exitCode = code
		panic(code)
	}

	defer func() { logrus.StandardLogger().ExitFunc = oldExit }()

	require.Panics(t, func() {
		startService(err, "test service")
	})

	require.Equal(t, 1, exitCode)

	require.NotPanics(t, func() {
		startService(nil, "test service")
	})
}

func TestStart(t *testing.T) {
	t.Parallel()

	type testSvc struct{}

	svc := start(testSvc{}, nil)
	require.NotNil(t, svc)
	require.IsType(t, testSvc{}, svc)
}
