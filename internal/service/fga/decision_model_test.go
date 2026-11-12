package fga

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	fgamodel "auth-service/fga"
	"auth-service/internal/service/fga/mocks"

	"github.com/golang/mock/gomock"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/openfga/language/pkg/go/transformer"
	"github.com/stretchr/testify/require"
)

type modelReadStub struct {
	sdk.SdkClientReadAuthorizationModelRequestInterface
	response *sdk.ClientReadAuthorizationModelResponse
	err      error
	options  sdk.ClientReadAuthorizationModelOptions
}

func (s *modelReadStub) Options(options sdk.ClientReadAuthorizationModelOptions) sdk.SdkClientReadAuthorizationModelRequestInterface {
	s.options = options
	return s
}
func (s *modelReadStub) Execute() (*sdk.ClientReadAuthorizationModelResponse, error) {
	return s.response, s.err
}

func modelResponse(t *testing.T, dsl string) *sdk.ClientReadAuthorizationModelResponse {
	t.Helper()

	data, err := transformer.TransformDSLToJSON(dsl)
	require.NoError(t, err)

	var model openfga.AuthorizationModel
	require.NoError(t, json.Unmarshal([]byte(data), &model))
	model.Id = "pinned"

	return &sdk.ClientReadAuthorizationModelResponse{AuthorizationModel: &model}
}

func requireModelError(message string) require.ErrorAssertionFunc {
	return func(t require.TestingT, err error, _ ...any) {
		require.ErrorContains(t, err, message)
	}
}

func TestEnsureDecisionModel(t *testing.T) {
	t.Parallel()

	expectRead := func(api *mocks.MockSdkClient, stub *modelReadStub) {
		api.EXPECT().ReadAuthorizationModel(gomock.Any()).Return(stub)
	}

	for _, tt := range []struct {
		name     string
		response *sdk.ClientReadAuthorizationModelResponse
		err      error
		wantErr  require.ErrorAssertionFunc
		expect   func(*mocks.MockSdkClient, *modelReadStub)
	}{
		{"strict", modelResponse(t, fgamodel.Strict), nil, require.NoError, expectRead},
		{"different model", modelResponse(t, "model\n  schema 1.1\ntype user"), nil, requireModelError("does not match"), expectRead},
		{"unavailable", nil, context.DeadlineExceeded, func(t require.TestingT, err error, _ ...any) {
			require.ErrorContains(t, err, "verify decision model: read pinned decision model")
			require.ErrorIs(t, err, context.DeadlineExceeded)
		}, expectRead},
		{"empty", nil, nil, requireModelError("is empty"), expectRead},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := mocks.NewMockSdkClient(gomock.NewController(t))
			stub := &modelReadStub{response: tt.response, err: tt.err}
			tt.expect(api, stub)
			client := &Client{fgaClient: api, modelID: "pinned", requestTimeout: time.Second}

			err := client.EnsureDecisionModel(t.Context())
			tt.wantErr(t, err)

			require.Equal(t, "pinned", *stub.options.AuthorizationModelId)
		})
	}
}

func TestWithModelID(t *testing.T) {
	t.Parallel()

	var c Client

	WithModelID("pinned")(&c)

	require.Equal(t, "pinned", c.modelID)
}

func TestReadPinnedModel(t *testing.T) {
	t.Parallel()

	expectRead := func(api *mocks.MockSdkClient, stub *modelReadStub) {
		api.EXPECT().ReadAuthorizationModel(gomock.Any()).Return(stub)
	}

	found := modelResponse(t, fgamodel.Strict)
	for _, tc := range []struct {
		name     string
		response *sdk.ClientReadAuthorizationModelResponse
		err      error
		want     *openfga.AuthorizationModel
		wantErr  require.ErrorAssertionFunc
		expect   func(*mocks.MockSdkClient, *modelReadStub)
	}{
		{"found", found, nil, found.AuthorizationModel, require.NoError, expectRead},
		{"unavailable", nil, context.DeadlineExceeded, nil, func(t require.TestingT, err error, _ ...any) { require.ErrorIs(t, err, context.DeadlineExceeded) }, expectRead},
		{"empty model", &sdk.ClientReadAuthorizationModelResponse{}, nil, nil, requireModelError("is empty"), expectRead},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := mocks.NewMockSdkClient(gomock.NewController(t))
			stub := &modelReadStub{response: tc.response, err: tc.err}
			tc.expect(api, stub)
			c := &Client{fgaClient: api, modelID: "pinned", requestTimeout: time.Second}

			got, err := c.readPinnedModel(t.Context())
			tc.wantErr(t, err)

			require.Equal(t, "pinned", *stub.options.AuthorizationModelId)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestCheckModelAvailability(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		prepare func(*mocks.MockSdkClient)
		wantErr require.ErrorAssertionFunc
	}{
		{"available", func(api *mocks.MockSdkClient) {
			api.EXPECT().ReadAuthorizationModel(gomock.Any()).Return(&modelReadStub{response: &sdk.ClientReadAuthorizationModelResponse{AuthorizationModel: &openfga.AuthorizationModel{Id: "pinned"}}})
		}, require.NoError},
		{"unavailable", func(api *mocks.MockSdkClient) {
			api.EXPECT().ReadAuthorizationModel(gomock.Any()).Return(&modelReadStub{err: context.DeadlineExceeded})
		}, func(t require.TestingT, err error, _ ...any) { require.ErrorIs(t, err, context.DeadlineExceeded) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := mocks.NewMockSdkClient(gomock.NewController(t))
			tc.prepare(api)
			c := &Client{fgaClient: api, modelID: "pinned", requestTimeout: time.Second}

			err := c.CheckModelAvailability(t.Context())
			tc.wantErr(t, err)
		})
	}
}
