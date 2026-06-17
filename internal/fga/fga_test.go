//nolint:funlen // тесты
package fga

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"auth-service/internal/fga/mocks"

	openfga "github.com/openfga/go-sdk"
	openFGAClient "github.com/openfga/go-sdk/client"
)

type listStoresRequestStub struct {
	ctx      context.Context
	response *openFGAClient.ClientListStoresResponse
	err      error
	options  *openFGAClient.ClientListStoresOptions
}

func (s *listStoresRequestStub) Options(options openFGAClient.ClientListStoresOptions) openFGAClient.SdkClientListStoresRequestInterface {
	s.options = &options
	return s
}

func (s *listStoresRequestStub) Execute() (*openFGAClient.ClientListStoresResponse, error) {
	return s.response, s.err
}

func (s *listStoresRequestStub) GetContext() context.Context {
	return s.ctx
}

func (s *listStoresRequestStub) GetOptions() *openFGAClient.ClientListStoresOptions {
	return s.options
}

type createStoreRequestStub struct {
	ctx      context.Context
	response *openFGAClient.ClientCreateStoreResponse
	err      error
	body     *openFGAClient.ClientCreateStoreRequest
	options  *openFGAClient.ClientCreateStoreOptions
}

func (s *createStoreRequestStub) Options(options openFGAClient.ClientCreateStoreOptions) openFGAClient.SdkClientCreateStoreRequestInterface {
	s.options = &options
	return s
}

func (s *createStoreRequestStub) Body(body openFGAClient.ClientCreateStoreRequest) openFGAClient.SdkClientCreateStoreRequestInterface {
	s.body = &body
	return s
}

func (s *createStoreRequestStub) Execute() (*openFGAClient.ClientCreateStoreResponse, error) {
	return s.response, s.err
}

func (s *createStoreRequestStub) GetContext() context.Context {
	return s.ctx
}

func (s *createStoreRequestStub) GetOptions() *openFGAClient.ClientCreateStoreOptions {
	return s.options
}

func (s *createStoreRequestStub) GetBody() *openFGAClient.ClientCreateStoreRequest {
	return s.body
}

type readLatestAuthorizationModelRequestStub struct {
	ctx      context.Context
	response *openFGAClient.ClientReadAuthorizationModelResponse
	err      error
	options  *openFGAClient.ClientReadLatestAuthorizationModelOptions
}

func (s *readLatestAuthorizationModelRequestStub) Options(options openFGAClient.ClientReadLatestAuthorizationModelOptions) openFGAClient.SdkClientReadLatestAuthorizationModelRequestInterface {
	s.options = &options
	return s
}

func (s *readLatestAuthorizationModelRequestStub) Execute() (*openFGAClient.ClientReadAuthorizationModelResponse, error) {
	return s.response, s.err
}

//nolint:revive // реализуем чужой интерфейс
func (s *readLatestAuthorizationModelRequestStub) GetStoreIdOverride() *string {
	if s.options == nil {
		return nil
	}

	return s.options.StoreId
}

func (s *readLatestAuthorizationModelRequestStub) GetContext() context.Context {
	return s.ctx
}

func (s *readLatestAuthorizationModelRequestStub) GetOptions() *openFGAClient.ClientReadLatestAuthorizationModelOptions {
	return s.options
}

type writeAuthorizationModelRequestStub struct {
	ctx      context.Context
	response *openFGAClient.ClientWriteAuthorizationModelResponse
	err      error
	body     *openFGAClient.ClientWriteAuthorizationModelRequest
	options  *openFGAClient.ClientWriteAuthorizationModelOptions
}

func (s *writeAuthorizationModelRequestStub) Options(options openFGAClient.ClientWriteAuthorizationModelOptions) openFGAClient.SdkClientWriteAuthorizationModelRequestInterface {
	s.options = &options
	return s
}

func (s *writeAuthorizationModelRequestStub) Body(body openFGAClient.ClientWriteAuthorizationModelRequest) openFGAClient.SdkClientWriteAuthorizationModelRequestInterface {
	s.body = &body
	return s
}

func (s *writeAuthorizationModelRequestStub) Execute() (*openFGAClient.ClientWriteAuthorizationModelResponse, error) {
	return s.response, s.err
}

//nolint:revive // реализуем чужой интерфейс
func (s *writeAuthorizationModelRequestStub) GetStoreIdOverride() *string {
	if s.options == nil {
		return nil
	}

	return s.options.StoreId
}

func (s *writeAuthorizationModelRequestStub) GetBody() *openFGAClient.ClientWriteAuthorizationModelRequest {
	return s.body
}

func (s *writeAuthorizationModelRequestStub) GetOptions() *openFGAClient.ClientWriteAuthorizationModelOptions {
	return s.options
}

func (s *writeAuthorizationModelRequestStub) GetContext() context.Context {
	return s.ctx
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    []option
		want    *Client
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			opts: []option{
				WithAPIURL("http://localhost:8080"),
				WithAuthorizationModel("test"),
				WithStoreID("test"),
				WithStoreName("test"),
				WithApplyModelOnStart(true),
			},
			want: &Client{
				apiURL:             "http://localhost:8080",
				authorizationModel: "test",
				storeID:            "test",
				storeName:          "test",
				applyModelOnStart:  true,
			},
			wantErr: require.NoError,
		},
		{
			name: "negative case: authorization model is required",
			opts: []option{
				WithAPIURL("http://localhost:8080"),
				WithStoreID("test"),
				WithStoreName("test"),
				WithApplyModelOnStart(true),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "authorization model is required")
			},
		},
		{
			name: "negative case: api url is required",
			opts: []option{
				WithAuthorizationModel("test"),
				WithStoreID("test"),
				WithStoreName("test"),
				WithApplyModelOnStart(true),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "api url is required")
			},
		},
		{
			name: "negative case: store id or store name is required",
			opts: []option{
				WithAuthorizationModel("test"),
				WithAPIURL("http://localhost:8080"),
				WithApplyModelOnStart(true),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "store id or store name is required")
			},
		},
		{
			name: "negative case: store id or store name is required",
			opts: []option{
				WithAuthorizationModel("test"),
				WithAPIURL("http://localhost:8080"),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "store id or store name is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewClient(tt.opts...)
			tt.wantErr(t, err)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestStop(t *testing.T) {
	t.Parallel()

	client := &Client{}

	require.NoError(t, client.Stop(t.Context()))
}

func TestFindStoreByName(t *testing.T) {
	t.Parallel()

	storeName := "test"
	storeID := "store-id"

	tests := []struct {
		name       string
		setupMocks func(t *testing.T, client *mocks.MockSdkClient) *listStoresRequestStub
		want       *openFGAClient.ClientListStoresResponse
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case: store found",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) *listStoresRequestStub {
				t.Helper()

				stub := &listStoresRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientListStoresResponse{
						Stores: []openfga.Store{
							{
								Id:   storeID,
								Name: storeName,
							},
						},
					},
				}
				client.EXPECT().ListStores(t.Context()).Return(stub)

				return stub
			},
			want: &openFGAClient.ClientListStoresResponse{
				Stores: []openfga.Store{
					{
						Id:   storeID,
						Name: storeName,
					},
				},
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: no stores found",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) *listStoresRequestStub {
				t.Helper()

				stub := &listStoresRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientListStoresResponse{
						Stores: []openfga.Store{},
					},
				}
				client.EXPECT().ListStores(t.Context()).Return(stub)

				return stub
			},
			want: &openFGAClient.ClientListStoresResponse{
				Stores: []openfga.Store{},
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: list stores fails",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) *listStoresRequestStub {
				t.Helper()

				stub := &listStoresRequestStub{
					ctx: t.Context(),
					err: errors.New("list stores failed"),
				}
				client.EXPECT().ListStores(t.Context()).Return(stub)

				return stub
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error listing stores")
				require.ErrorContains(t, err, "list stores failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockSdkClient(ctrl)
			stub := tt.setupMocks(t, mockClient)

			client := &Client{fgaClient: mockClient}

			got, err := client.findStoreByName(t.Context(), storeName)
			tt.wantErr(t, err)
			require.Equal(t, tt.want, got)

			require.NotNil(t, stub.options)
			require.NotNil(t, stub.options.Name)
			require.Equal(t, storeName, *stub.options.Name)
		})
	}
}

func TestCreateStore(t *testing.T) {
	t.Parallel()

	storeName := "test"
	storeID := "store-id"

	tests := []struct {
		name        string
		setupMocks  func(t *testing.T, client *mocks.MockSdkClient) *createStoreRequestStub
		wantStoreID string
		wantErr     require.ErrorAssertionFunc
	}{
		{
			name: "positive case: store created",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) *createStoreRequestStub {
				t.Helper()

				stub := &createStoreRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientCreateStoreResponse{
						Id:   storeID,
						Name: storeName,
					},
				}
				client.EXPECT().CreateStore(t.Context()).Return(stub)
				client.EXPECT().SetStoreId(storeID).Return(nil)

				return stub
			},
			wantStoreID: storeID,
			wantErr:     require.NoError,
		},
		{
			name: "error case: create store fails",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) *createStoreRequestStub {
				t.Helper()

				stub := &createStoreRequestStub{
					ctx: t.Context(),
					err: errors.New("create store failed"),
				}
				client.EXPECT().CreateStore(t.Context()).Return(stub)

				return stub
			},
			wantStoreID: "",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error creating store")
				require.ErrorContains(t, err, "create store failed")
			},
		},
		{
			name: "error case: set store id fails",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) *createStoreRequestStub {
				t.Helper()

				stub := &createStoreRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientCreateStoreResponse{
						Id:   storeID,
						Name: storeName,
					},
				}
				client.EXPECT().CreateStore(t.Context()).Return(stub)
				client.EXPECT().SetStoreId(storeID).Return(errors.New("error setting store id"))

				return stub
			},
			wantStoreID: "",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error setting store id")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockSdkClient(ctrl)
			stub := tt.setupMocks(t, mockClient)

			client := &Client{
				fgaClient: mockClient,
				storeName: storeName,
			}

			err := client.createStore(t.Context())
			tt.wantErr(t, err)
			require.Equal(t, tt.wantStoreID, client.storeID)

			require.NotNil(t, stub.body)
			require.Equal(t, storeName, stub.body.Name)
		})
	}
}

func TestResolveStore(t *testing.T) {
	t.Parallel()

	storeID := "test"
	storeName := "test"

	createClient := func(t *testing.T, client *mocks.MockSdkClient, clientStoreID string) *Client {
		t.Helper()

		return &Client{
			fgaClient: client,
			storeID:   clientStoreID,
			storeName: storeName,
		}
	}

	tests := []struct {
		name          string
		clientStoreID string
		setupMocks    func(t *testing.T, client *mocks.MockSdkClient)
		wantErr       require.ErrorAssertionFunc
	}{
		{
			name:          "positive case: storeID != nil",
			clientStoreID: storeID,
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().SetStoreId(gomock.Any()).Return(nil).Do(func(id string) {
					require.Equal(t, storeID, id)
				})
			},
			wantErr: require.NoError,
		},
		{
			name:          "error case: storeID != nil",
			clientStoreID: storeID,
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().SetStoreId(gomock.Any()).Return(errors.New("error setting store id")).Do(func(id string) {
					require.Equal(t, storeID, id)
				})
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error setting store id")
			},
		},
		{
			name:          "lists stores: store found",
			clientStoreID: "",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().ListStores(t.Context()).Return(&listStoresRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientListStoresResponse{
						Stores: []openfga.Store{
							{
								Id:   storeID,
								Name: storeName,
							},
						},
					},
				})

				client.EXPECT().SetStoreId(storeID).Return(nil).Do(func(id string) {
					require.Equal(t, storeID, id)
				})
			},
			wantErr: require.NoError,
		},
		{
			name:          "lists stores: store not found",
			clientStoreID: "",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().ListStores(t.Context()).Return(&listStoresRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientListStoresResponse{
						Stores: []openfga.Store{},
					},
				})

				client.EXPECT().CreateStore(t.Context()).Return(&createStoreRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientCreateStoreResponse{
						Id:   storeID,
						Name: storeName,
					},
				})

				client.EXPECT().SetStoreId(storeID).Return(nil).Do(func(id string) {
					require.Equal(t, storeID, id)
				})
			},
			wantErr: require.NoError,
		},
		{
			name:          "error case: list stores fails",
			clientStoreID: "",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().ListStores(t.Context()).Return(&listStoresRequestStub{
					ctx: t.Context(),
					err: errors.New("list stores failed"),
				})
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error listing stores")
				require.ErrorContains(t, err, "list stores failed")
			},
		},
		{
			name:          "error case: create store fails",
			clientStoreID: "",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().ListStores(t.Context()).Return(&listStoresRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientListStoresResponse{
						Stores: []openfga.Store{},
					},
				})

				client.EXPECT().CreateStore(t.Context()).Return(&createStoreRequestStub{
					ctx: t.Context(),
					err: errors.New("error creating store"),
				})
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error creating store")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			openFclient := mocks.NewMockSdkClient(ctrl)
			tt.setupMocks(t, openFclient)

			client := createClient(t, openFclient, tt.clientStoreID)

			err := client.resolveStore(t.Context())
			tt.wantErr(t, err)
		})
	}
}

func TestResolveModel(t *testing.T) {
	t.Parallel()

	modelID := "model-id"

	tmpDir := t.TempDir()

	file, err := os.Create(filepath.Join(tmpDir, "auth-model.fga")) //nolint:gosec // тестовый временный файл
	require.NoError(t, err)

	defer func() {
		require.NoError(t, file.Close())
	}()

	authModelPath := file.Name()

	content := `
	model
  schema 1.1

type user
`
	_, err = file.WriteString(content)
	require.NoError(t, err)

	tests := []struct {
		name               string
		applyModelOnStart  bool
		authorizationModel string
		setupMocks         func(t *testing.T, client *mocks.MockSdkClient)
		wantModelID        string
		wantErr            require.ErrorAssertionFunc
	}{
		{
			name:              "positive case: read latest authorization model",
			applyModelOnStart: false,
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().ReadLatestAuthorizationModel(t.Context()).Return(&readLatestAuthorizationModelRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientReadAuthorizationModelResponse{
						AuthorizationModel: &openfga.AuthorizationModel{
							Id: modelID,
						},
					},
				})
				client.EXPECT().SetAuthorizationModelId(modelID).Return(nil)
			},
			wantModelID: modelID,
			wantErr:     require.NoError,
		},
		{
			name:              "error case: read latest authorization model fails",
			applyModelOnStart: false,
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().ReadLatestAuthorizationModel(t.Context()).Return(&readLatestAuthorizationModelRequestStub{
					ctx: t.Context(),
					err: errors.New("read failed"),
				})
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error reading latest authorization model")
			},
		},
		{
			name:               "positive case: apply model on start",
			applyModelOnStart:  true,
			authorizationModel: authModelPath,
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().WriteAuthorizationModel(t.Context()).Return(&writeAuthorizationModelRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientWriteAuthorizationModelResponse{
						AuthorizationModelId: modelID,
					},
				})
				client.EXPECT().SetAuthorizationModelId(modelID).Return(nil)
			},
			wantModelID: modelID,
			wantErr:     require.NoError,
		},
		{
			name:               "error case: apply model on start fails",
			applyModelOnStart:  true,
			authorizationModel: "nonexistent.fga",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error applying model")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockSdkClient(ctrl)
			tt.setupMocks(t, mockClient)

			client := &Client{
				fgaClient:          mockClient,
				authorizationModel: tt.authorizationModel,
				applyModelOnStart:  tt.applyModelOnStart,
			}

			err := client.resolveModel(t.Context())
			tt.wantErr(t, err)
			require.Equal(t, tt.wantModelID, client.modelID)
		})
	}
}

func TestApplyModelAndSetModelID(t *testing.T) {
	t.Parallel()

	modelID := "model-id"
	tmpDir := t.TempDir()

	file, err := os.Create(filepath.Join(tmpDir, "auth-model.fga")) //nolint:gosec // тестовый временный файл
	require.NoError(t, err)

	defer func() {
		require.NoError(t, file.Close())
	}()

	authModelPath := file.Name()

	content := `
	model
  schema 1.1

type user
`
	_, err = file.WriteString(content)
	require.NoError(t, err)

	tests := []struct {
		name               string
		authorizationModel string
		setupMocks         func(t *testing.T, client *mocks.MockSdkClient)
		wantModelID        string
		wantErr            require.ErrorAssertionFunc
	}{
		{
			name:               "positive case: model applied and model id set",
			authorizationModel: authModelPath,
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().WriteAuthorizationModel(t.Context()).Return(&writeAuthorizationModelRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientWriteAuthorizationModelResponse{
						AuthorizationModelId: modelID,
					},
				})
				client.EXPECT().SetAuthorizationModelId(modelID).Return(nil)
			},
			wantModelID: modelID,
			wantErr:     require.NoError,
		},
		{
			name:               "error case: apply model fails",
			authorizationModel: "nonexistent.fga",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error applying model")
			},
		},
		{
			name:               "error case: set authorization model id fails",
			authorizationModel: authModelPath,
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().WriteAuthorizationModel(t.Context()).Return(&writeAuthorizationModelRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientWriteAuthorizationModelResponse{
						AuthorizationModelId: modelID,
					},
				})
				client.EXPECT().SetAuthorizationModelId(modelID).Return(errors.New("set model id failed"))
			},
			wantModelID: modelID,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "error setting authorization model id")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockSdkClient(ctrl)
			tt.setupMocks(t, mockClient)

			client := &Client{
				fgaClient:          mockClient,
				authorizationModel: tt.authorizationModel,
			}

			err := client.applyModelAndSetModelID(t.Context())
			tt.wantErr(t, err)
			require.Equal(t, tt.wantModelID, client.modelID)
		})
	}
}

func TestReadLatestAuthorizationModelAndSetModelID(t *testing.T) {
	t.Parallel()

	modelID := "model-id"

	tests := []struct {
		name        string
		setupMocks  func(t *testing.T, client *mocks.MockSdkClient)
		wantModelID string
		wantErr     require.ErrorAssertionFunc
	}{
		{
			name: "positive case: latest model read and model id set",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().ReadLatestAuthorizationModel(t.Context()).Return(&readLatestAuthorizationModelRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientReadAuthorizationModelResponse{
						AuthorizationModel: &openfga.AuthorizationModel{
							Id: modelID,
						},
					},
				})
				client.EXPECT().SetAuthorizationModelId(modelID).Return(nil)
			},
			wantModelID: modelID,
			wantErr:     require.NoError,
		},
		{
			name: "error case: read latest authorization model fails",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().ReadLatestAuthorizationModel(t.Context()).Return(&readLatestAuthorizationModelRequestStub{
					ctx: t.Context(),
					err: errors.New("read failed"),
				})
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "read latest authorization model failed")
			},
		},
		{
			name: "error case: set authorization model id fails",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().ReadLatestAuthorizationModel(t.Context()).Return(&readLatestAuthorizationModelRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientReadAuthorizationModelResponse{
						AuthorizationModel: &openfga.AuthorizationModel{
							Id: modelID,
						},
					},
				})
				client.EXPECT().SetAuthorizationModelId(modelID).Return(errors.New("set model id failed"))
			},
			wantModelID: modelID,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "set model id failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockSdkClient(ctrl)
			tt.setupMocks(t, mockClient)

			client := &Client{fgaClient: mockClient}

			err := client.readLatestAuthorizationModelAndSetModelID(t.Context())
			tt.wantErr(t, err)
			require.Equal(t, tt.wantModelID, client.modelID)
		})
	}
}

func TestApplyModel(t *testing.T) {
	t.Parallel()

	modelID := "model-id"

	createValidModelFile := func(t *testing.T) string {
		t.Helper()

		path := filepath.Join(t.TempDir(), "auth-model.fga")
		content := `model
  schema 1.1

type user
`
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		return path
	}

	createInvalidModelFile := func(t *testing.T) string {
		t.Helper()

		path := filepath.Join(t.TempDir(), "invalid.fga")
		require.NoError(t, os.WriteFile(path, []byte("not valid dsl"), 0o600))

		return path
	}

	tests := []struct {
		name               string
		authorizationModel func(t *testing.T) string
		setupMocks         func(t *testing.T, client *mocks.MockSdkClient)
		wantModelID        string
		wantErr            require.ErrorAssertionFunc
	}{
		{
			name:               "positive case: model written",
			authorizationModel: createValidModelFile,
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().WriteAuthorizationModel(t.Context()).Return(&writeAuthorizationModelRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientWriteAuthorizationModelResponse{
						AuthorizationModelId: modelID,
					},
				})
			},
			wantModelID: modelID,
			wantErr:     require.NoError,
		},
		{
			name: "error case: read file fails",
			authorizationModel: func(t *testing.T) string {
				t.Helper()

				return "nonexistent.fga"
			},
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()
			},
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				t.Helper()

				require.Error(tt, err)
				require.ErrorContains(tt, err, "read file")
			},
		},
		{
			name:               "error case: transform DSL fails",
			authorizationModel: createInvalidModelFile,
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()
			},
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				t.Helper()

				require.Error(tt, err)
				require.ErrorContains(tt, err, "transforming DSL model")
			},
		},
		{
			name:               "error case: write authorization model fails",
			authorizationModel: createValidModelFile,
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) {
				t.Helper()

				client.EXPECT().WriteAuthorizationModel(t.Context()).Return(&writeAuthorizationModelRequestStub{
					ctx: t.Context(),
					err: errors.New("write failed"),
				})
			},
			wantErr: func(tt require.TestingT, err error, i ...interface{}) {
				t.Helper()

				require.Error(tt, err)
				require.ErrorContains(tt, err, "writing authorization model")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockSdkClient(ctrl)
			tt.setupMocks(t, mockClient)

			client := &Client{
				fgaClient:          mockClient,
				authorizationModel: tt.authorizationModel(t),
			}

			err := client.applyModel(t.Context())
			tt.wantErr(t, err)
			require.Equal(t, tt.wantModelID, client.modelID)
		})
	}
}

func TestReadFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		want    []byte
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "positive case: file read",
			setup: func(t *testing.T) string {
				t.Helper()

				path := filepath.Join(t.TempDir(), "model.fga")
				content := []byte("model\n  schema 1.1\n")
				require.NoError(t, os.WriteFile(path, content, 0o600))

				return path
			},
			want:    []byte("model\n  schema 1.1\n"),
			wantErr: require.NoError,
		},
		{
			name: "positive case: empty file",
			setup: func(t *testing.T) string {
				t.Helper()

				path := filepath.Join(t.TempDir(), "empty.fga")
				require.NoError(t, os.WriteFile(path, []byte{}, 0o600))

				return path
			},
			want:    []byte{},
			wantErr: require.NoError,
		},
		{
			name: "error case: file not found",
			setup: func(t *testing.T) string {
				t.Helper()

				return filepath.Join(t.TempDir(), "missing.fga")
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "open file")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := readFile(tt.setup(t))
			tt.wantErr(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTransformDSLToJSON(t *testing.T) {
	t.Parallel()

	validDSL := []byte(`model
  schema 1.1

type user
`)

	tests := []struct {
		name    string
		data    []byte
		target  func() any
		assert  func(t *testing.T, v any)
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "positive case: valid DSL transformed",
			data: validDSL,
			target: func() any {
				return &openfga.WriteAuthorizationModelRequest{}
			},
			assert: func(t *testing.T, v any) {
				t.Helper()

				body, ok := v.(*openfga.WriteAuthorizationModelRequest)
				require.True(t, ok)
				require.Equal(t, "1.1", body.SchemaVersion)
				require.Len(t, body.TypeDefinitions, 1)
				require.Equal(t, "user", body.TypeDefinitions[0].Type)
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: invalid DSL",
			data: []byte("not valid dsl"),
			target: func() any {
				return &openfga.WriteAuthorizationModelRequest{}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "parsing DSL model")
			},
		},
		{
			name: "error case: unmarshal fails",
			data: validDSL,
			target: func() any {
				return new(int)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "unmarshalling transformed model")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			target := tt.target()
			err := transformDSLToJSON(tt.data, target)
			tt.wantErr(t, err)

			if tt.assert != nil {
				tt.assert(t, target)
			}
		})
	}
}

func TestWriteAuthorizationModel(t *testing.T) {
	t.Parallel()

	modelID := "model-id"
	body := openfga.WriteAuthorizationModelRequest{
		SchemaVersion: "1.1",
		TypeDefinitions: []openfga.TypeDefinition{
			{Type: "user"},
		},
	}

	tests := []struct {
		name       string
		setupMocks func(t *testing.T, client *mocks.MockSdkClient) *writeAuthorizationModelRequestStub
		want       *openFGAClient.ClientWriteAuthorizationModelResponse
		wantErr    require.ErrorAssertionFunc
	}{
		{
			name: "positive case: model written",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) *writeAuthorizationModelRequestStub {
				t.Helper()

				stub := &writeAuthorizationModelRequestStub{
					ctx: t.Context(),
					response: &openFGAClient.ClientWriteAuthorizationModelResponse{
						AuthorizationModelId: modelID,
					},
				}
				client.EXPECT().WriteAuthorizationModel(t.Context()).Return(stub)

				return stub
			},
			want: &openFGAClient.ClientWriteAuthorizationModelResponse{
				AuthorizationModelId: modelID,
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: write authorization model fails",
			setupMocks: func(t *testing.T, client *mocks.MockSdkClient) *writeAuthorizationModelRequestStub {
				t.Helper()

				stub := &writeAuthorizationModelRequestStub{
					ctx: t.Context(),
					err: errors.New("write failed"),
				}
				client.EXPECT().WriteAuthorizationModel(t.Context()).Return(stub)

				return stub
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "fga:")
				require.ErrorContains(t, err, "write failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockSdkClient(ctrl)
			stub := tt.setupMocks(t, mockClient)

			client := &Client{fgaClient: mockClient}

			got, err := client.writeAuthorizationModel(t.Context(), body)
			tt.wantErr(t, err)
			require.Equal(t, tt.want, got)

			require.NotNil(t, stub.body)
			require.Equal(t, body.SchemaVersion, stub.body.SchemaVersion)
			require.Len(t, stub.body.TypeDefinitions, 1)
			require.Equal(t, "user", stub.body.TypeDefinitions[0].Type)
		})
	}
}
