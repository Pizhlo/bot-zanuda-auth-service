// Command space-types проверяет и дополняет типы по доверенной выгрузке; по умолчанию работает без записи.
package main

import (
	"auth-service/internal/config"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"auth-service/internal/service/spacemigration"

	sdk "github.com/openfga/go-sdk/client"
)

func main() {
	if err := execute(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute(args []string, output io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, args, output); err != nil {
		return fmt.Errorf("run space-type migration: %w", err)
	}

	return nil
}

func run(ctx context.Context, args []string, output io.Writer) error {
	options, err := parseArguments(args)
	if err != nil {
		return fmt.Errorf("parse migration arguments: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, options.timeout)
	defer cancel()

	var records []spacemigration.Record

	err = waitForIO(ctx, func() error {
		var readErr error

		records, readErr = readExport(options.input)

		return readErr
	})
	if err != nil {
		return fmt.Errorf("read migration export: %w", err)
	}

	api, err := sdk.NewSdkClient(&sdk.ClientConfiguration{ApiUrl: options.apiURL, StoreId: options.storeID, AuthorizationModelId: options.modelID, HTTPClient: &http.Client{Timeout: options.requestTimeout}})
	if err != nil {
		return fmt.Errorf("create migration FGA client: %w", err)
	}

	plan, err := spacemigration.Run(ctx, api, records, spacemigration.Options{Apply: options.apply, RequestTimeout: options.requestTimeout})
	if err != nil {
		return fmt.Errorf("execute migration plan: %w", err)
	}

	if err := waitForIO(ctx, func() error { return json.NewEncoder(output).Encode(plan) }); err != nil {
		return fmt.Errorf("encode migration plan: %w", err)
	}

	return nil
}

// waitForIO даёт процессу CLI завершиться по сигналу или дедлайну даже при
// заблокированном файловом вводе/выводе. Сетевые операции сами учитывают контекст
// и возвращают оператору диагностику частично применённой миграции.
func waitForIO(ctx context.Context, operation func() error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("migration interrupted: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- operation() }()

	select {
	case <-ctx.Done():
		return fmt.Errorf("migration interrupted: %w", ctx.Err())
	case err := <-done:
		return err
	}
}

type migrationArguments struct {
	apiURL, storeID, modelID, input string
	apply                           bool
	requestTimeout, timeout         time.Duration
}

func parseArguments(args []string) (migrationArguments, error) {
	var options migrationArguments

	flags := flag.NewFlagSet("space-types", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.apiURL, "api-url", "", "OpenFGA URL")
	flags.StringVar(&options.storeID, "store-id", "", "existing store ID")
	flags.StringVar(&options.modelID, "model-id", "", "pinned transition or strict model ID")
	flags.StringVar(&options.input, "input", "", "trusted complete JSON export")
	flags.DurationVar(&options.requestTimeout, "request-timeout", config.DefaultOpenFGARequestTimeout, "OpenFGA request/read timeout")
	flags.DurationVar(&options.timeout, "timeout", config.DefaultMigrationTimeout, "total migration timeout")
	flags.BoolVar(&options.apply, "apply", false, "write verified missing markers (requires paused writers)")

	if err := flags.Parse(args); err != nil {
		return migrationArguments{}, fmt.Errorf("parse flags: %w", err)
	}

	if flags.NArg() != 0 || options.apiURL == "" || options.storeID == "" || options.modelID == "" || options.input == "" {
		return migrationArguments{}, fmt.Errorf("api-url, store-id, model-id and input are required; positional arguments are unsupported")
	}

	if options.requestTimeout <= 0 || options.timeout <= 0 {
		return migrationArguments{}, fmt.Errorf("migration timeouts must be positive")
	}

	return options, nil
}

func readExport(path string) ([]spacemigration.Record, error) {
	data, err := os.Open(path) //nolint:gosec // Путь явно передаёт оператор утилиты.
	if err != nil {
		return nil, fmt.Errorf("open export: %w", err)
	}
	defer func() { _ = data.Close() }()

	records, err := spacemigration.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode export: %w", err)
	}

	return records, nil
}
