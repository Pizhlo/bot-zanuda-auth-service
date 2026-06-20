package main

import (
	"auth-service/internal/config"
	"auth-service/internal/storage/rabbitmq"
	"auth-service/pkg/audit"
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

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

	defer butler.stop(notifyCtx, vaultClient)

	repo := initPostgresStorage(notifyCtx, config.Postgres)
	if err := repo.Run(notifyCtx); err != nil {
		logrus.WithFields(logrus.Fields{
			"db_name": config.Postgres.DBName,
		}).WithError(err).Fatal("unable to connect postgres")
	}

	defer butler.stop(notifyCtx, repo)

	var (
		rabbitMQ *rabbitmq.Client
		sender   audit.Sender
	)

	if config.Audit.BrokerEnabled {
		rabbitMQ = initRabbitMQStorage(config.RabbitMQ, []string{config.Audit.Topic})

		if err := rabbitMQ.Run(notifyCtx); err != nil {
			logrus.WithError(err).Fatal("failed to connect to rabbitmq")
		}

		defer butler.stop(notifyCtx, rabbitMQ)

		sender = initSender(config.Audit, rabbitMQ)
	}

	auditor := initAuditor(config.Audit, sender)

	fga := initFGAClient(config.OpenFGA, auditor, repo)
	if err := fga.Connect(notifyCtx); err != nil {
		logrus.WithError(err).Fatal("failed to connect to fga")
	}

	defer butler.stop(notifyCtx, fga)

	authSvc := initAuthService(vaultClient, config.Auth, repo, auditor)

	spaceAccessChecker := initSpaceAccessChecker(repo)
	notePermissionResolver := initNotePermissionResolver(repo)

	politicsSvc := initPoliticsService(repo, notePermissionResolver, spaceAccessChecker, auditor)

	redis := initRedisStorage(notifyCtx, config.Redis)
	defer butler.stop(notifyCtx, redis)

	userSvc := initUserService(repo, redis, config.Auth, auditor)

	resourcesHandler := initResourcesHandler(fga)
	authHandler := initAuthHandler(authSvc)
	notesHandler := initNotesHandler(politicsSvc, userSvc)
	handlerV0 := initHandlerV0(butler.BuildInfo, notesHandler, authHandler, resourcesHandler)
	middlewareHandler := initMiddlewareHandler(authSvc)

	server := initServer(handlerV0, middlewareHandler, config.Server)

	go butler.start(func() error {
		return server.Start(notifyCtx)
	})

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
