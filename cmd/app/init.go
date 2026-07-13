package main

import (
	"auth-service/internal/config"
	"auth-service/internal/server"
	"auth-service/internal/service/auth"
	"auth-service/internal/service/enforcer"
	"auth-service/internal/service/fga"
	"auth-service/internal/service/politics"
	"auth-service/internal/service/politics/access"
	"auth-service/internal/service/politics/permissions"
	"auth-service/internal/service/redis"
	"auth-service/internal/service/user"
	"auth-service/internal/storage/rabbitmq"
	"auth-service/internal/storage/vault"
	"auth-service/pkg/audit"
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"auth-service/docs" // swagger docs
	handlerV0 "auth-service/internal/api/v0"
	repo "auth-service/internal/storage/postgres"
)

func initEnforcer(cfg *config.Config) *enforcer.Enforcer {
	logrus.Info("initializing enforcer")

	dsn := formatPostgresAddr(cfg.Postgres)

	return start(enforcer.NewEnforcer(enforcer.WithDsn(dsn), enforcer.WithModelConf(cfg.Policy.Config)))
}

func initAuditor(cfg config.AuditConfig, sender audit.Sender) *audit.Auditor {
	logrus.Info("initializing error event builder")

	return audit.NewAuditor(
		audit.WithHook(auth.TokenValidationFailedHook),
		audit.WithHook(handlerV0.ConnectionHook),
		audit.WithHook(politics.FilterNotesFailedHook),
		audit.WithHook(user.GetUserIDByTelegramIDHook),
		audit.WithIncludeLevels(cfg.Levels.Include),
		audit.WithExcludeLevels(cfg.Levels.Exclude),
		audit.WithIncludeKinds(cfg.Kinds.Include),
		audit.WithExcludeKinds(cfg.Kinds.Exclude),
		audit.WithSender(sender),
	)
}

func initUserService(storage *repo.Repo, cache *redis.Service, cfg config.Auth, auditor *audit.Auditor) *user.Service {
	logrus.Info("initializing user service")

	return start(user.New(user.WithStorage(storage), user.WithCache(cache), user.WithCacheTTL(cfg.UserCacheTTL), user.WithAuditor(auditor)))
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

func initPoliticsService(storage *repo.Repo, resolver *permissions.NotePermissionResolver, spaceChecker *access.SpaceAccessChecker, auditor *audit.Auditor) *politics.Service {
	logrus.Info("initializing politics service")

	return start(
		politics.New(
			politics.WithStorage(storage),
			politics.WithNotePermissionResolver(resolver),
			politics.WithSpaceAccessChecker(spaceChecker),
			politics.WithAuditor(auditor),
		),
	)
}

func initAuthService(vaultClient *vault.Client, cfg config.Auth, storage *repo.Repo, auditor *audit.Auditor) *auth.Service {
	logrus.WithFields(logrus.Fields{
		"update_key_interval": cfg.UpdateKeyInterval,
		"issuer":              cfg.Issuer,
		"token_duration":      cfg.TokenDuration,
	}).Info("initializing auth service")

	return start(
		auth.New(
			auth.WithSecretKey([]byte(cfg.SecretKey)),
			auth.WithUpdateKeyInterval(cfg.UpdateKeyInterval),
			auth.WithIssuer(cfg.Issuer),
			auth.WithTokenDuration(cfg.TokenDuration),
			auth.WithVaultClient(vaultClient),
			auth.WithStorage(storage),
			auth.WithAuditor(auditor),
		),
	)
}

func initAuthHandler(authSrv *auth.Service) *handlerV0.AuthHandler {
	logrus.WithFields(logrus.Fields{}).Info("initializing auth handler v0")

	return start(
		handlerV0.NewAuthHandler(
			handlerV0.WithAuthService(authSrv),
		),
	)
}

func initNotesHandler(politicsSvc *politics.Service, userSvc *user.Service) *handlerV0.NotesHandler {
	logrus.Info("initializing notes handler")

	return start(
		handlerV0.NewNotesHandler(
			handlerV0.WithPoliticsService(politicsSvc),
			handlerV0.WithUserService(userSvc),
		),
	)
}

func initResourcesHandler(resourceSvc *fga.Client) *handlerV0.ResourceHandler {
	logrus.Info("initializing resources handler")

	return start(
		handlerV0.NewResourceHandler(
			handlerV0.WithResourceService(resourceSvc),
		),
	)
}

func initHandlerV0(buildInfo *BuildInfo, notes *handlerV0.NotesHandler, auth *handlerV0.AuthHandler, resources *handlerV0.ResourceHandler) *handlerV0.Handler {
	logrus.WithFields(logrus.Fields{
		"version":   buildInfo.Version,
		"buildDate": buildInfo.BuildDate,
		"gitCommit": buildInfo.GitCommit,
	}).Info("initializing handler v0")

	return start(
		handlerV0.NewHandler(
			handlerV0.WithVersion(buildInfo.Version),
			handlerV0.WithBuildDate(buildInfo.BuildDate),
			handlerV0.WithGitCommit(buildInfo.GitCommit),
			handlerV0.WithNotesHandler(notes),
			handlerV0.WithAuthHandler(auth),
			handlerV0.WithResourcesHandler(resources),
		),
	)
}

func initMiddlewareHandler(authSvc handlerV0.AuthService) *handlerV0.MiddlewareHandler {
	logrus.Info("initializing middleware handler")

	return start(
		handlerV0.NewMiddlewareHandler(
			handlerV0.WithMiddlewareAuthService(authSvc),
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
		vault.WithSecretsPath(cfg.SecretsPath),
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

func initRabbitMQStorage(cfg config.RabbitMQ, errorsTopics []string) *rabbitmq.Client {
	logrus.WithFields(logrus.Fields{
		"url":            cfg.URL,
		"connectTimeout": cfg.ConnectTimeout,
		"publishTimeout": cfg.PublishTimeout,
		"maxRetries":     cfg.MaxRetries,
		"retryBackoff":   cfg.RetryBackoff,
	}).Info("initializing rabbitmq")

	client := start(
		rabbitmq.NewClient(
			rabbitmq.WithURL(cfg.URL),
			rabbitmq.WithTopics(errorsTopics),
			rabbitmq.WithConnectTimeout(cfg.ConnectTimeout),
			rabbitmq.WithPublishTimeout(cfg.PublishTimeout),
			rabbitmq.WithMaxRetries(cfg.MaxRetries),
			rabbitmq.WithRetryBackoff(cfg.RetryBackoff),
		),
	)

	return client
}

func initSender(cfg config.AuditConfig, rabbitMQ *rabbitmq.Client) audit.Sender {
	logrus.WithFields(logrus.Fields{
		"topic": cfg.Topic,
	}).Info("initializing sender")

	return start(audit.NewSender(
		audit.WithClient(rabbitMQ),
		audit.WithTopic(cfg.Topic),
	))
}

func initFGAClient(cfg config.OpenFGA, auditor *audit.Auditor, userRepo *repo.Repo) *fga.Client {
	logrus.WithFields(logrus.Fields{
		"api_url":              cfg.APIURL,
		"store_id":             cfg.StoreID,
		"store_name":           cfg.StoreName,
		"apply_model_on_start": cfg.ApplyModelOnStart,
	}).Info("initializing openFGA client")

	return start(fga.NewClient(
		fga.WithAPIURL(cfg.APIURL),
		fga.WithAuthorizationModel(cfg.AuthorizationModel),
		fga.WithStoreID(cfg.StoreID),
		fga.WithStoreName(cfg.StoreName),
		fga.WithApplyModelOnStart(cfg.ApplyModelOnStart),
		fga.WithAuditor(auditor),
		fga.WithUserRepo(userRepo),
	))
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
