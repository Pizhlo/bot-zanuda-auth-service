package fga

import (
	"context"
	"encoding/json"
	"fmt"

	fgamodel "auth-service/fga"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/openfga/language/pkg/go/transformer"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// WithModelID закрепляет существующую модель вместо выбора latest при запуске.
func WithModelID(id string) option { return func(c *Client) { c.modelID = id } }

// EnsureDecisionModel не допускает запуск check с прежней моделью без защиты personal.
func (c *Client) EnsureDecisionModel(ctx context.Context) error {
	expected, err := transformer.TransformDSLToProto(fgamodel.Strict)
	if err != nil {
		return err
	}

	resp, err := c.fgaClient.ReadAuthorizationModel(ctx).Options(sdk.ClientReadAuthorizationModelOptions{AuthorizationModelId: &c.modelID}).Execute()
	if err != nil {
		return fmt.Errorf("read pinned decision model: %w", err)
	}

	if resp == nil {
		return fmt.Errorf("pinned decision model is empty")
	}

	data, err := json.Marshal(resp.GetAuthorizationModel())
	if err != nil {
		return err
	}

	actual := &openfgav1.AuthorizationModel{}
	if err := protojson.Unmarshal(data, actual); err != nil {
		return err
	}

	actual.Id = ""
	// Protobuf сравнивает семантику, включая defaults, которые OpenFGA добавляет в JSON.
	if !proto.Equal(actual, expected) {
		return fmt.Errorf("pinned model does not match the strict decision model; complete the space-type migration first")
	}

	return nil
}
