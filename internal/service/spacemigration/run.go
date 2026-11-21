package spacemigration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"auth-service/internal/service/fga"

	sdk "github.com/openfga/go-sdk/client"
)

type tupleStore interface {
	Read(context.Context) sdk.SdkClientReadRequestInterface
	Write(context.Context) sdk.SdkClientWriteRequestInterface
}

// Decode читает единственный JSON-массив с известными полями.
// Содержательные проверки UUID, типов и полноты выполняет BuildPlan после чтения store.
func Decode(reader io.Reader) ([]Record, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var records []Record
	if err := decoder.Decode(&records); err != nil {
		return nil, fmt.Errorf("decode trusted export: %w", err)
	}

	if records == nil {
		return nil, fmt.Errorf("trusted export must be a JSON array")
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trusted export must contain one JSON array")
	}

	return records, nil
}

// Options задаёт режим записи и бюджет обращения к FGA из конфигурации CLI.
type Options struct {
	Apply          bool
	RequestTimeout time.Duration
}

// Run читает и проверяет весь store до первой записи. Без apply возвращает dry-run.
// При apply пишет только недостающие маркеры на закреплённой модели и повторно проверяет полноту.
// Бatches не составляют транзакцию: после сбоя нужно заново выполнить dry-run.
// Оператор обязан остановить конкурирующие изменения на всё время миграции.
func Run(ctx context.Context, api tupleStore, records []Record, options Options) (Plan, error) {
	tuples, err := fga.ReadTuples(ctx, api, "", options.RequestTimeout)
	if err != nil {
		return Plan{}, fmt.Errorf("read migration tuples: %w", err)
	}

	plan, err := BuildPlan(ctx, records, tuples)
	if err != nil {
		return Plan{}, fmt.Errorf("build space migration plan: %w", err)
	}

	if !options.Apply {
		return plan, nil
	}

	const writeBatchSize = 100 // Максимальный размер одной записи tuples в OpenFGA.

	for start := 0; start < len(plan.Writes); start += writeBatchSize {
		end := min(start+writeBatchSize, len(plan.Writes))
		if err := writeMarkers(ctx, api, plan.Writes[start:end], options.RequestTimeout); err != nil {
			return plan, fmt.Errorf("write markers (%d of %d confirmed; rerun dry-run before retry): %w", start, len(plan.Writes), err)
		}
	}

	after, err := fga.ReadTuples(ctx, api, "", options.RequestTimeout)
	if err != nil {
		return plan, fmt.Errorf("verify space migration: %w", err)
	}

	remaining, err := BuildPlan(ctx, records, after)
	if err != nil {
		return plan, fmt.Errorf("verify space migration: %w", err)
	}

	if len(remaining.Writes) > 0 {
		return plan, fmt.Errorf("verification found %d missing markers", len(remaining.Writes))
	}

	return plan, nil
}

func writeMarkers(ctx context.Context, api tupleStore, writes []sdk.ClientTupleKey, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if _, err := api.Write(ctx).Body(sdk.ClientWriteRequest{Writes: writes}).Execute(); err != nil {
		return fmt.Errorf("write type markers: %w", err)
	}

	return nil
}
