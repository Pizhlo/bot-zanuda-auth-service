package server

import (
	handlerV0 "auth-service/internal/api/v0"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sirupsen/logrus"
	echoSwagger "github.com/swaggo/echo-swagger"
)

// Server - сервер.
// Содержит порт, эхо сервер, логгер и хендлеры.
// Связующее звено между эхо сервером и хендлерами.
type Server struct {
	port            int
	shutdownTimeout time.Duration

	e *echo.Echo

	api struct {
		h0 handler
	}
}

//go:generate mockgen -source=server.go -destination=mocks/handler_mock.go -package=mocks handler
type handler interface {
	healthHandler
	versionHandler
}

type versionHandler interface {
	// Version возвращает версию апи хендлера, чтобы нельзя было использовать хендлер не той версии.
	Version() string
}

type healthHandler interface {
	Health(c echo.Context) error
}

// Option - опция для настройки сервера.
type Option func(*Server)

// WithPort - устанавливает порт сервера.
func WithPort(port int) Option {
	return func(s *Server) {
		s.port = port
	}
}

// WithShutdownTimeout - устанавливает таймаут graceful shutdown.
func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(s *Server) {
		s.shutdownTimeout = shutdownTimeout
	}
}

// WithHandlerV0 - устанавливает хендлер версии 0.
func WithHandlerV0(handler handler) Option {
	return func(s *Server) {
		s.api.h0 = handler
	}
}

// New - создает новый сервер. Принимает опции для настройки сервера.
// Доступные опции:
//
//   - WithPort - устанавливает порт сервера.
//   - WithHandlerV0 - устанавливает хендлер версии 0.
//   - WithShutdownTimeout - устанавливает таймаут graceful shutdown.
func New(opts ...Option) (*Server, error) {
	s := &Server{}
	for _, opt := range opts {
		opt(s)
	}

	if s.port == 0 {
		return nil, fmt.Errorf("port is required")
	}

	if s.api.h0 == nil {
		return nil, fmt.Errorf("handler is required")
	}

	if s.shutdownTimeout == 0 {
		return nil, fmt.Errorf("shutdown timeout is required")
	}

	if !checkHandlerVersion(s.api.h0, handlerV0.Version0) {
		return nil, fmt.Errorf("expected handler version is %s, got %s", handlerV0.Version0, s.api.h0.Version())
	}

	return s, nil
}

func checkHandlerVersion(h versionHandler, expectedVersion string) bool {
	return h.Version() == expectedVersion
}

// Start - запускает сервер. Создает маршруты и запускает сервер.
// Принимает контекст для graceful shutdown.
func (s *Server) Start(ctx context.Context) error {
	if err := s.createRoutes(); err != nil {
		return err
	}

	// запускаем сервер в отдельной горутине
	errChan := make(chan error, 1)

	go func() {
		errChan <- s.e.Start(fmt.Sprintf(":%d", s.port))
	}()

	// ждем либо ошибку запуска, либо отмену контекста
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		// контекст отменен - делаем graceful shutdown
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdownTimeout)
		defer cancel()

		logrus.WithFields(logrus.Fields{
			"port":            s.port,
			"shutdownTimeout": s.shutdownTimeout,
		}).Info("shutting down server")

		return s.e.Shutdown(shutdownCtx)
	}
}

func (s *Server) createRoutes() error {
	e := echo.New()

	// Swagger UI route - must be registered before other middleware
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	skipper := func(c echo.Context) bool {
		return strings.Contains(c.Request().URL.Path, "swagger")
	}

	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{Skipper: skipper}))
	e.Use(middleware.Logger())

	e.Use(echoprometheus.NewMiddleware("webserver")) // adds middleware to gather metrics
	e.GET("/metrics", echoprometheus.NewHandler())   // adds route to serve gathered metrics

	api := e.Group("api/")

	// v0
	apiv0 := api.Group("v0/")

	apiv0.GET("health", s.api.h0.Health)

	s.e = e

	if len(s.e.Routes()) == 0 {
		return errors.New("no routes initialized")
	}

	return nil
}
