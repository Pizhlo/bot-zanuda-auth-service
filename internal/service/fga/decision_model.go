package fga

import (
	"context"
	"encoding/json"
	"fmt"

	fgamodel "auth-service/fga"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/openfga/language/pkg/go/transformer"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// WithModelID закрепляет существующую версию для чтений и записей процесса.
// Её нельзя сочетать с apply_model_on_start: публикация новой версии и выбор существующей
// являются разными режимами запуска.
func WithModelID(id string) option { return func(c *Client) { c.modelID = id } }

// EnsureDecisionModel сверяет закреплённую модель со встроенной строгой моделью.
// Ошибка чтения или структурное отличие блокируют запуск check: старые и переходные
// модели ещё не обеспечивают изоляцию personal. Метод ничего не публикует и не мигрирует.
// Эквивалентная по правам, но иначе структурированная модель также будет отклонена.
func (c *Client) EnsureDecisionModel(ctx context.Context) error {
	expected, err := transformer.TransformDSLToProto(fgamodel.Strict)
	if err != nil {
		return fmt.Errorf("parse embedded decision model: %w", err)
	}

	stored, err := c.readPinnedModel(ctx)
	if err != nil {
		return fmt.Errorf("verify decision model: %w", err)
	}

	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("encode pinned decision model: %w", err)
	}

	actual := &openfgav1.AuthorizationModel{}
	if err := protojson.Unmarshal(data, actual); err != nil {
		return fmt.Errorf("decode pinned decision model: %w", err)
	}

	actual.Id = ""
	// Проверяем структурное равенство protobuf, нормализуя добавленные сервером defaults.
	// Это не доказательство эквивалентности прав: порядок повторяющихся узлов тоже важен.
	if !proto.Equal(actual, expected) {
		return fmt.Errorf("pinned model does not match the strict decision model; complete the space-type migration first")
	}

	return nil
}

// CheckModelAvailability подтверждает доступность закреплённой модели для правил без проверки отношения.
func (c *Client) CheckModelAvailability(ctx context.Context) error {
	_, err := c.readPinnedModel(ctx)
	return err
}

// readPinnedModel читает именно версию, выбранную при запуске сервиса.
func (c *Client) readPinnedModel(ctx context.Context) (*openfga.AuthorizationModel, error) {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	resp, err := c.fgaClient.ReadAuthorizationModel(ctx).Options(sdk.ClientReadAuthorizationModelOptions{AuthorizationModelId: &c.modelID}).Execute()
	if err != nil {
		return nil, fmt.Errorf("read pinned decision model: %w", err)
	}

	if resp == nil || resp.AuthorizationModel == nil {
		return nil, fmt.Errorf("pinned decision model is empty")
	}

	return resp.AuthorizationModel, nil
}
