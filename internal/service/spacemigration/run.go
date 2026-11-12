package spacemigration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"auth-service/internal/service/fga"
	sdk "github.com/openfga/go-sdk/client"
)

// Decode читает единственную JSON-выгрузку с известными полями.
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

// Run сначала проверяет весь store; при apply записывает только markers на закреплённой модели.
// Оператор обязан остановить конкурирующие изменения до чтения и до завершения проверки.
func Run(ctx context.Context, api sdk.SdkClient, records []Record, apply bool) (Plan, error) {
	tuples, err := fga.ReadTuples(ctx, api, "")
	if err != nil {
		return Plan{}, err
	}

	plan, err := BuildPlan(records, tuples)
	if err != nil || !apply {
		return plan, err
	}

	for start := 0; start < len(plan.Writes); start += 100 {
		end := min(start+100, len(plan.Writes))
		if _, err := api.Write(ctx).Body(sdk.ClientWriteRequest{Writes: plan.Writes[start:end]}).Execute(); err != nil {
			return plan, fmt.Errorf("write markers (rerun dry-run before retry): %w", err)
		}
	}

	after, err := fga.ReadTuples(ctx, api, "")
	if err != nil {
		return plan, err
	}

	remaining, err := BuildPlan(records, after)
	if err != nil {
		return plan, err
	}

	if len(remaining.Writes) > 0 {
		return plan, fmt.Errorf("verification found %d missing markers", len(remaining.Writes))
	}

	return plan, nil
}
