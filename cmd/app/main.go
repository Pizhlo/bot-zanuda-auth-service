package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()

	butler := NewButler()

	logrus.WithFields(logrus.Fields{
		"version": butler.BuildInfo.Version,
		"commit":  butler.BuildInfo.GitCommit,
		"date":    butler.BuildInfo.BuildDate,
	}).Info("starting service")
	defer logrus.Info("shutdown")

	notifyCtx, notify := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer notify()

	logrus.Info("all services started")

	// Ждем сигнал завершения
	<-notifyCtx.Done()
	logrus.Info("received shutdown signal, stopping services...")

	// Ждем завершения всех горутин
	butler.waitForAll()
	logrus.Info("all services stopped")
}
