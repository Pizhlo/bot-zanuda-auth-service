package main

import (
	"auth-service/docs" // swagger docs
	handlerV0 "auth-service/internal/api/v0"
	"auth-service/internal/config"
	"auth-service/internal/server"
	"auth-service/internal/service/auth"
	"auth-service/internal/service/enforcer"
	"auth-service/internal/service/politics"
	"auth-service/internal/service/politics/access"
	"auth-service/internal/service/politics/permissions"
	"auth-service/internal/service/redis"
	repo "auth-service/internal/storage/postgres"
	"auth-service/internal/storage/vault"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// @title           Auth Service API
// @version         1.0
// @description     API для работы с авторизацией
// @host            localhost:8080
// @basePath        /api/v0
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
//
//nolint:funlen // длинная функция запуска
func main() {
	ctx := context.Background()

	butler := NewButler()

	configPath := flag.String("config", "./config.yaml", "path to config file")

	flag.Parse()

	config, err := config.LoadConfig(*configPath)
	if err != nil {
		logrus.WithError(err).Fatal("failed to load config")
	}

	// Обновляем host в swagger документации из конфига
	updateSwaggerHost(config.Server)

	level, err := logrus.ParseLevel(config.LogLevel)
	if err != nil {
		logrus.WithError(err).Fatalf("error parsing log level")
	}

	logrus.SetLevel(level)

	logrus.WithField("level", logrus.GetLevel()).Info("set log level")

	logrus.WithFields(logrus.Fields{
		"version": butler.BuildInfo.Version,
		"commit":  butler.BuildInfo.GitCommit,
		"date":    butler.BuildInfo.BuildDate,
	}).Info("starting service")
	defer logrus.Info("shutdown")

	notifyCtx, notify := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer notify()

	vaultClient := initVaultClient(config.Vault)

	if err := vaultClient.Connect(); err != nil {
		logrus.WithError(err).Fatal("failed to connect to vault")
	}

	defer butler.stop(ctx, vaultClient)

	authSvc := initAuthService([]byte(config.Auth.SecretKey), vaultClient, config.Auth.UpdateKeyInterval)

	repo := initPostgresStorage(ctx, config.Postgres)

	if err := repo.Run(ctx); err != nil {
		logrus.WithFields(logrus.Fields{
			"db_name": config.Postgres.DBName,
		}).WithError(err).Fatal("unable to connect postgres")
	}

	defer butler.stop(ctx, repo)

	spaceAccessChecker := initSpaceAccessChecker(repo)
	notePermissionResolver := initNotePermissionResolver(repo)

	politicsSvc := initPoliticsService(repo, notePermissionResolver, spaceAccessChecker)

	handlerV0 := initHandlerV0(butler.BuildInfo, politicsSvc)
	middlewareHandler := initMiddlewareHandler(authSvc)
	server := initServer(handlerV0, middlewareHandler, config.Server)

	go butler.start(func() error {
		return server.Start(notifyCtx)
	})

	redis := initRedisStorage(ctx, config.Redis)
	defer butler.stop(ctx, redis)

	enf := initEnforcer(config)

	go butler.start(func() error {
		return enf.Run(notifyCtx)
	})

	defer butler.stop(ctx, enf)

	logrus.Info("all services started")

	// Ждем сигнал завершения
	<-notifyCtx.Done()
	logrus.Info("received shutdown signal, stopping services...")

	// Ждем завершения всех горутин
	butler.waitForAll()
	logrus.Info("all services stopped")
}

func initEnforcer(cfg *config.Config) *enforcer.Enforcer {
	logrus.Info("initializing enforcer")

	dsn := formatPostgresAddr(cfg.Postgres)

	return start(enforcer.NewEnforcer(enforcer.WithDsn(dsn), enforcer.WithModelConf(cfg.Policy.Config)))
}

func initPostgresStorage(ctx context.Context, cfg config.Postgres) *repo.Repo {
	addr := formatPostgresAddr(cfg)

	logrus.WithFields(logrus.Fields{
		"host":           cfg.Host,
		"port":           cfg.Port,
		"user":           cfg.User,
		"db_name":        cfg.DBName,
		"insert_timeout": cfg.InsertTimeout,
		"read_timeout":   cfg.ReadTimeout,
	}).Info("connecting postgres")

	return start(repo.New(ctx, repo.WithAddr(addr),
		repo.WithInsertTimeout(cfg.InsertTimeout),
		repo.WithReadTimeout(cfg.ReadTimeout),
	))
}

func initPoliticsService(storage *repo.Repo, resolver *permissions.NotePermissionResolver, spaceChecker *access.SpaceAccessChecker) *politics.Service {
	logrus.Info("initializing politics service")

	return start(
		politics.New(
			politics.WithStorage(storage),
			politics.WithNotePermissionResolver(resolver),
			politics.WithSpaceAccessChecker(spaceChecker),
		),
	)
}

func initAuthService(secretKey []byte, vaultClient *vault.Client, updateKeyInterval time.Duration) *auth.Service {
	logrus.WithFields(logrus.Fields{
		"update_key_interval": updateKeyInterval,
	}).Info("initializing auth service")

	return start(
		auth.New(
			auth.WithSecretKey(secretKey),
			auth.WithUpdateKeyInterval(updateKeyInterval),
			auth.WithVaultClient(vaultClient),
		),
	)
}

func initHandlerV0(buildInfo *BuildInfo, politics handlerV0.PoliticsService) *handlerV0.Handler {
	logrus.WithFields(logrus.Fields{
		"version":   buildInfo.Version,
		"buildDate": buildInfo.BuildDate,
		"gitCommit": buildInfo.GitCommit,
	}).Info("initializing handler v0")

	return start(
		handlerV0.New(
			handlerV0.WithVersion(buildInfo.Version),
			handlerV0.WithBuildDate(buildInfo.BuildDate),
			handlerV0.WithGitCommit(buildInfo.GitCommit),
			handlerV0.WithPoliticsService(politics),
		),
	)
}

func initMiddlewareHandler(authSvc handlerV0.AuthService) *handlerV0.MiddlewareHandler {
	logrus.Info("initializing middleware handler")

	return start(
		handlerV0.NewMiddlewareHandler(
			handlerV0.WithAuthService(authSvc),
		),
	)
}

func initSpaceAccessChecker(storage *repo.Repo) *access.SpaceAccessChecker {
	logrus.Info("initializing space access checker")

	return start(
		access.New(access.WithStorage(storage)),
	)
}

func initNotePermissionResolver(storage *repo.Repo) *permissions.NotePermissionResolver {
	logrus.Info("initializing note permission resolver")

	return start(
		permissions.NewNotePermissionResolver(
			permissions.WithStorage(storage),
		),
	)
}

func initServer(handlerV0 *handlerV0.Handler, mdHandler *handlerV0.MiddlewareHandler, cfg config.Server) *server.Server {
	logrus.WithFields(logrus.Fields{
		"port":            cfg.Port,
		"shutdownTimeout": cfg.ShutdownTimeout,
	}).Info("initializing server")

	return start(
		server.New(
			server.WithHandlerV0(handlerV0),
			server.WithPort(cfg.Port),
			server.WithShutdownTimeout(cfg.ShutdownTimeout),
			server.WithMiddlewareHandler(mdHandler),
		),
	)
}

func initVaultClient(cfg config.Vault) *vault.Client {
	logrus.WithFields(logrus.Fields{
		"address":           cfg.Address,
		"insecure_skip_tls": cfg.InsecureSkipTLS,
	}).Info("initializing vault client")

	opts := []vault.ClientOption{
		vault.WithAddress(cfg.Address),
		vault.WithToken(cfg.Token),
	}

	if cfg.InsecureSkipTLS {
		opts = append(opts, vault.WithInsecureSkipTLS(true))
	}

	if cfg.CAPath != "" || cfg.ClientCertPath != "" || cfg.ClientKeyPath != "" {
		opts = append(opts, vault.WithTLSConfig(cfg.CAPath, cfg.ClientCertPath, cfg.ClientKeyPath))
	}

	return start(
		vault.NewClient(opts...),
	)
}

func initRedisStorage(ctx context.Context, cfg config.Redis) *redis.Service {
	redis := start(redis.New(redis.WithCfg(&cfg)))

	startService(redis.Connect(ctx), "redis connect")

	return redis
}

func startService(err error, name string) {
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"service": name,
		}).Fatalf("error creating service: %+v", err)
	}
}

func start[T any](svc T, err error) T {
	startService(err, fmt.Sprintf("%T", svc))

	return svc
}

// updateSwaggerHost обновляет host в swagger документации на основе конфигурации сервера.
// Если Host указан в конфиге, используется он, иначе формируется из localhost и порта.
func updateSwaggerHost(cfg config.Server) {
	host := cfg.SwaggerHost
	if host == "" {
		host = fmt.Sprintf("localhost:%d", cfg.Port)
	}

	docs.SwaggerInfo.Host = host
	logrus.WithField("swagger_host", host).Debug("swagger host updated")
}

func formatPostgresAddr(cfg config.Postgres) string {
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.User, cfg.Password,
		cfg.Host, cfg.Port, cfg.DBName)
}
