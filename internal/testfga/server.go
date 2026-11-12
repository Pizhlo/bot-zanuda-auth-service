// Package testfga создаёт отдельный store на явно указанном локальном тестовом OpenFGA.
package testfga

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"testing"
	"time"

	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/openfga/language/pkg/go/transformer"
	"github.com/stretchr/testify/require"
)

// New создаёт тестовый store и модель. Без AUTH_TEST_FGA_URL интеграционный тест пропускается.
func New(t *testing.T, dsl string) *sdk.OpenFgaClient {
	t.Helper()

	address := os.Getenv("AUTH_TEST_FGA_URL")
	if address == "" {
		t.Skip("set AUTH_TEST_FGA_URL to an isolated local OpenFGA")
	}

	parsed, err := url.Parse(address)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, parsed.Hostname(), "integration tests only accept loopback hosts")

	api, err := sdk.NewSdkClient(&sdk.ClientConfiguration{ApiUrl: address})
	require.NoError(t, err)
	store, err := api.CreateStore(t.Context()).Body(sdk.ClientCreateStoreRequest{Name: "bz167-test"}).Execute()
	require.NoError(t, err)
	require.NoError(t, api.SetStoreId(store.GetId()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()

		_, err := api.DeleteStore(ctx).Execute()
		require.NoError(t, err)
	})
	SetModel(t, api, dsl)

	return api
}

// SetModel устанавливает модель в тестовом store и закрепляет её в клиенте.
func SetModel(t *testing.T, api *sdk.OpenFgaClient, dsl string) string {
	t.Helper()

	text, err := transformer.TransformDSLToJSON(dsl)
	require.NoError(t, err)

	var body openfga.WriteAuthorizationModelRequest
	require.NoError(t, json.Unmarshal([]byte(text), &body))
	model, err := api.WriteAuthorizationModel(t.Context()).Body(body).Execute()
	require.NoError(t, err)
	require.NoError(t, api.SetAuthorizationModelId(model.GetAuthorizationModelId()))

	return model.GetAuthorizationModelId()
}

// Write записывает подготовленные tuples в изолированный store.
func Write(t *testing.T, api *sdk.OpenFgaClient, tuples ...sdk.ClientTupleKey) {
	t.Helper()
	_, err := api.Write(t.Context()).Body(sdk.ClientWriteRequest{Writes: tuples}).Execute()
	require.NoError(t, err)
}
