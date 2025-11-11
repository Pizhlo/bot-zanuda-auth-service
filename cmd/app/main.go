package main

import (
	"auth-service/docs" // swagger docs
	handlerV0 "auth-service/internal/api/v0"
	"auth-service/internal/config"
	"auth-service/internal/server"
	"auth-service/internal/storage/vault"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
)

// @title           Auth Service API
// @version         1.0
// @description     API для работы с авторизацией
// @host            localhost:8080 // Дефолтное значение, будет перезаписано динамически из конфига
// @basePath        /api/v0 //nolint:godot // swagger комментарии не должны заканчиваться точкой.
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

	handlerV0 := initHandlerV0(butler.BuildInfo)
	server := initServer(handlerV0, config.Server)

	go butler.start(func() error {
		return server.Start(notifyCtx)
	})

	vaultClient := initVaultClient(config.Vault)

	if err := vaultClient.Connect(); err != nil {
		logrus.WithError(err).Fatal("failed to connect to vault")
	}

	defer butler.stop(ctx, vaultClient)

	logrus.Info("all services started")

	// Ждем сигнал завершения
	<-notifyCtx.Done()
	logrus.Info("received shutdown signal, stopping services...")

	// Ждем завершения всех горутин
	butler.waitForAll()
	logrus.Info("all services stopped")
}

func initHandlerV0(buildInfo *BuildInfo) *handlerV0.Handler {
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
		),
	)
}

func initServer(handlerV0 *handlerV0.Handler, cfg config.Server) *server.Server {
	logrus.WithFields(logrus.Fields{
		"port":            cfg.Port,
		"shutdownTimeout": cfg.ShutdownTimeout,
	}).Info("initializing server")

	return start(
		server.New(
			server.WithHandlerV0(handlerV0),
			server.WithPort(cfg.Port),
			server.WithShutdownTimeout(cfg.ShutdownTimeout),
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
