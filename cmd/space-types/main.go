// Command space-types проверяет и дополняет типы по доверенной выгрузке; по умолчанию работает без записи.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	return run(ctx, args, output)
}

func run(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("space-types", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	endpoint := flags.String("api-url", "", "OpenFGA URL")
	store := flags.String("store-id", "", "existing store ID")
	model := flags.String("model-id", "", "pinned transition or strict model ID")
	input := flags.String("input", "", "trusted complete JSON export")

	apply := flags.Bool("apply", false, "write verified missing markers (requires paused writers)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 || *endpoint == "" || *store == "" || *model == "" || *input == "" {
		return fmt.Errorf("api-url, store-id, model-id and input are required; positional arguments are unsupported")
	}

	records, err := readExport(*input)
	if err != nil {
		return err
	}

	api, err := sdk.NewSdkClient(&sdk.ClientConfiguration{ApiUrl: *endpoint, StoreId: *store, AuthorizationModelId: *model})
	if err != nil {
		return err
	}

	plan, err := spacemigration.Run(ctx, api, records, *apply)
	if err != nil {
		return err
	}

	return json.NewEncoder(output).Encode(plan)
}

func readExport(path string) ([]spacemigration.Record, error) {
	data, err := os.Open(path) //nolint:gosec // Путь явно передаёт оператор утилиты.
	if err != nil {
		return nil, err
	}
	defer func() { _ = data.Close() }()

	return spacemigration.Decode(data)
}
