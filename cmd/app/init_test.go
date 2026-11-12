package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"auth-service/internal/config"
	"auth-service/internal/service/auth"
	"auth-service/internal/service/decision"
	"auth-service/internal/service/fga"
	"auth-service/internal/service/politics/access"
	"auth-service/internal/service/politics/permissions"
	"auth-service/internal/service/redis"
	"auth-service/internal/storage/postgres"
	"auth-service/internal/storage/vault"
	"auth-service/pkg/audit"

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

func TestInitAuthorization(t *testing.T) {
	t.Parallel()

	// Аннулированный контекст останавливает чтение модели до отправки HTTP-запроса.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	for _, tc := range []struct {
		name    string
		config  config.OpenFGA
		ctx     context.Context
		wantErr func(*testing.T, error)
	}{
		{"invalid configuration", config.OpenFGA{}, t.Context(), func(t *testing.T, err error) {
			t.Helper()
			require.ErrorContains(t, err, "create FGA client")
		}},
		{"model and apply on start conflict", config.OpenFGA{
			APIURL: "http://127.0.0.1", AuthorizationModel: "model.fga", RequestTimeout: time.Second,
			StoreID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", ModelID: "01ARZ3NDEKTSV4RRFFQ69G5FAW", ApplyModelOnStart: true,
		}, ctx, func(t *testing.T, err error) {
			t.Helper()
			require.ErrorContains(t, err, "connect to FGA")
			require.ErrorContains(t, err, "mutually exclusive")
		}},
		{"canceled model verification", config.OpenFGA{
			APIURL: "http://127.0.0.1", AuthorizationModel: "model.fga", RequestTimeout: time.Second,
			StoreID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", ModelID: "01ARZ3NDEKTSV4RRFFQ69G5FAW",
		}, ctx, func(t *testing.T, err error) {
			t.Helper()
			require.ErrorContains(t, err, "verify authorization model")
			require.ErrorIs(t, err, context.Canceled)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, svc, err := initAuthorization(tc.ctx, &config.Config{OpenFGA: tc.config}, &postgres.Repo{})
			tc.wantErr(t, err)

			require.Nil(t, client)
			require.Nil(t, svc)
		})
	}
}

func TestInitAuditor(t *testing.T) {
	t.Parallel()

	a := initAuditor(config.AuditConfig{DeliveryTimeout: time.Second}, nil)
	e, err := a.Create(t.Context())
	require.NoError(t, err)

	require.NoError(t, e.End(t.Context()))
}

func TestInitUserService(t *testing.T) {
	t.Parallel()

	s := initUserService(&postgres.Repo{}, &redis.Service{}, config.Auth{UserCacheTTL: time.Minute})

	require.NotNil(t, s)
}

func TestInitPostgresStorage(t *testing.T) {
	t.Parallel()

	db, err := initPostgresStorage(t.Context(), config.Postgres{})
	require.ErrorContains(t, err, "insert timeout is required")

	require.Nil(t, db)
}

func TestInitPoliticsService(t *testing.T) {
	t.Parallel()

	s := initPoliticsService(&postgres.Repo{}, &permissions.NotePermissionResolver{}, &access.SpaceAccessChecker{})

	require.NotNil(t, s)
}

func TestInitAuthService(t *testing.T) {
	t.Parallel()

	s := initAuthService(&vault.Client{}, config.Auth{
		SecretKey:         "local-test",
		UpdateKeyInterval: time.Minute,
		Issuer:            "fixture",
		TokenDuration:     time.Minute,
	}, &postgres.Repo{})

	require.Equal(t, "fixture", s.GetIssuer())
}

func TestInitResourcesHandler(t *testing.T) {
	t.Parallel()

	require.NotNil(t, initResourcesHandler(&fga.Client{}, &decision.Service{}))
}

func TestInitMiddlewareHandler(t *testing.T) {
	t.Parallel()

	h := initMiddlewareHandler(&auth.Service{}, audit.NewAuditor())

	require.NotNil(t, h)
}

func TestInitFGAClient(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		config  config.OpenFGA
		wantErr require.ErrorAssertionFunc
		want    func(*testing.T, *fga.Client)
	}{
		{"missing model", config.OpenFGA{}, func(t require.TestingT, err error, _ ...any) {
			require.ErrorContains(t, err, "authorization model is required")
		}, func(t *testing.T, client *fga.Client) {
			t.Helper()
			require.Nil(t, client)
		}},
		{"configured", config.OpenFGA{
			APIURL: "http://127.0.0.1", AuthorizationModel: "model.fga", RequestTimeout: time.Second,
			StoreID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", ModelID: "01ARZ3NDEKTSV4RRFFQ69G5FAW",
		}, require.NoError, func(t *testing.T, client *fga.Client) {
			t.Helper()
			require.NotNil(t, client)
			require.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FAW", client.ModelID())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, err := initFGAClient(tc.config, &postgres.Repo{})
			tc.wantErr(t, err)

			tc.want(t, client)
		})
	}
}
