package fga

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/openfga/language/pkg/go/transformer"

	openfga "github.com/openfga/go-sdk"
	openFGAClient "github.com/openfga/go-sdk/client"
	"github.com/sirupsen/logrus"
)

// Client - клиент для работы с OpenFGA.
type Client struct {
	apiURL             string
	fgaClient          openClient
	authorizationModel string
	storeID            string
	storeName          string
	modelID            string
	applyModelOnStart  bool
}

//go:generate mockgen -destination=mocks/mocks.go -package=mocks github.com/openfga/go-sdk/client SdkClient
type openClient = openFGAClient.SdkClient

type option func(*Client)

// WithAPIURL устанавливает URL для API OpenFGA.
func WithAPIURL(apiURL string) option {
	return func(c *Client) {
		c.apiURL = apiURL
	}
}

// WithAuthorizationModel устанавливает путь к файлу с моделью авторизации.
func WithAuthorizationModel(authorizationModel string) option {
	return func(c *Client) {
		c.authorizationModel = authorizationModel
	}
}

// WithStoreID устанавливает ID store для поиска или создания.
// Если storeID не установлен, то будет использоваться storeName.
// Если storeID и storeName установлены, store создаваться не будет.
func WithStoreID(storeID string) option {
	return func(c *Client) {
		c.storeID = storeID
	}
}

// WithStoreName устанавливает имя store для поиска или создания.
func WithStoreName(storeName string) option {
	return func(c *Client) {
		c.storeName = storeName
	}
}

// WithApplyModelOnStart устанавливает флаг applyModelOnStart.
// Он влияет на то, будет ли модель применена при старте (устанавливать, если нужно применить новую версию модели при старте).
func WithApplyModelOnStart(apply bool) option {
	return func(c *Client) {
		c.applyModelOnStart = apply
	}
}

// NewClient создает новый экземпляр клиента для работы с OpenFGA.
func NewClient(opts ...option) (*Client, error) {
	c := &Client{}

	for _, opt := range opts {
		opt(c)
	}

	if len(c.authorizationModel) == 0 {
		return nil, fmt.Errorf("fga: authorization model is required")
	}

	if len(c.apiURL) == 0 {
		return nil, fmt.Errorf("fga: api url is required")
	}

	if c.storeID == "" && c.storeName == "" {
		return nil, fmt.Errorf("fga: store id or store name is required")
	}

	return c, nil
}

// Connect подключается к OpenFGA.
// Создает клиент OpenFGA, резолвит store и model.
func (c *Client) Connect(ctx context.Context) error {
	fgaClient, err := openFGAClient.NewSdkClient(&openFGAClient.ClientConfiguration{
		ApiUrl: c.apiURL,
	})
	if err != nil {
		return fmt.Errorf("fga: error creating client: %w", err)
	}

	c.fgaClient = fgaClient

	logrus.WithField("api_url", c.apiURL).Info("openFGA client created")

	if err := c.resolveStore(ctx); err != nil {
		return fmt.Errorf("fga: error resolving store: %w", err)
	}

	if err := c.resolveModel(ctx); err != nil {
		return fmt.Errorf("fga: error resolving authorization model: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"model_id": c.modelID,
		"store_id": c.storeID,
	}).Info("openFGA client connected")

	return nil
}

// Stop завершает работу клиента OpenFGA.
func (c *Client) Stop(_ context.Context) error {
	logrus.Info("openFGA client stopped")

	return nil
}

// resolveStore находит или создает store в OpenFGA.
// Должно быть задано либо storeID, либо storeName.
func (c *Client) resolveStore(ctx context.Context) error {
	if c.storeID != "" {
		if err := c.fgaClient.SetStoreId(c.storeID); err != nil {
			return fmt.Errorf("fga: error setting store id: %w", err)
		}

		logrus.WithField("store_id", c.storeID).Info("openFGA store configured")

		return nil
	}

	stores, err := c.findStoreByName(ctx, c.storeName)
	if err != nil {
		return fmt.Errorf("fga: error listing stores: %w", err)
	}

	if len(stores.GetStores()) > 0 {
		c.storeID = stores.GetStores()[0].GetId()

		if err := c.fgaClient.SetStoreId(c.storeID); err != nil {
			return fmt.Errorf("fga: error setting store id: %w", err)
		}

		logrus.WithFields(logrus.Fields{
			"store_id":   c.storeID,
			"store_name": c.storeName,
		}).Info("openFGA store found")

		return nil
	}

	if err := c.createStore(ctx); err != nil {
		return fmt.Errorf("fga: error creating store: %w", err)
	}

	return nil
}

func (c *Client) findStoreByName(ctx context.Context, storeName string) (*openFGAClient.ClientListStoresResponse, error) {
	stores, err := c.fgaClient.ListStores(ctx).Options(openFGAClient.ClientListStoresOptions{
		Name: openfga.PtrString(storeName),
	}).Execute()
	if err != nil {
		return nil, fmt.Errorf("fga: error listing stores: %w", err)
	}

	return stores, err
}

func (c *Client) createStore(ctx context.Context) error {
	resp, err := c.fgaClient.CreateStore(ctx).Body(openFGAClient.ClientCreateStoreRequest{
		Name: c.storeName,
	}).Execute()
	if err != nil {
		return fmt.Errorf("fga: error creating store: %w", err)
	}

	if err := c.fgaClient.SetStoreId(resp.GetId()); err != nil {
		return fmt.Errorf("fga: error setting store id: %w", err)
	}

	c.storeID = resp.GetId()

	logrus.WithFields(logrus.Fields{
		"store_id":   c.storeID,
		"store_name": c.storeName,
	}).Info("openFGA store created")

	return nil
}

// resolveModel читает или применяет модель авторизации.
// Если applyModelOnStart = true, то применяет модель и устанавливает modelID.
// Иначе читает последнюю модель авторизации и устанавливает modelID.
func (c *Client) resolveModel(ctx context.Context) error {
	if c.applyModelOnStart {
		if err := c.applyModelAndSetModelID(ctx); err != nil {
			return fmt.Errorf("fga: error applying model: %w", err)
		}

		return nil
	}

	if err := c.readLatestAuthorizationModelAndSetModelID(ctx); err != nil {
		return fmt.Errorf("fga: error reading latest authorization model: %w", err)
	}

	return nil
}

// applyModelAndSetModelID применяет модель и устанавливает modelID.
func (c *Client) applyModelAndSetModelID(ctx context.Context) error {
	if err := c.applyModel(ctx); err != nil {
		return fmt.Errorf("fga: error applying model: %w", err)
	}

	if err := c.fgaClient.SetAuthorizationModelId(c.modelID); err != nil {
		return fmt.Errorf("fga: error setting authorization model id: %w", err)
	}

	logrus.WithField("model_id", c.modelID).Info("openFGA authorization model applied")

	return nil
}

// readLatestAuthorizationModelAndSetModelID читает последнюю модель авторизации и устанавливает modelID.
func (c *Client) readLatestAuthorizationModelAndSetModelID(ctx context.Context) error {
	model, err := c.fgaClient.ReadLatestAuthorizationModel(ctx).Execute()
	if err != nil {
		return fmt.Errorf("fga: read latest authorization model failed: %w", err)
	}

	c.modelID = model.GetAuthorizationModel().Id

	if err := c.fgaClient.SetAuthorizationModelId(c.modelID); err != nil {
		return fmt.Errorf("fga: set model id failed: %w", err)
	}

	logrus.WithField("model_id", c.modelID).Info("openFGA latest authorization model loaded")

	return nil
}

func (c *Client) applyModel(ctx context.Context) error {
	content, err := readFile(c.authorizationModel)
	if err != nil {
		return fmt.Errorf("fga: read file: %w", err)
	}

	var body openfga.WriteAuthorizationModelRequest
	if err := transformDSLToJSON(content, &body); err != nil {
		return fmt.Errorf("transforming DSL model: %w", err)
	}

	data, err := c.writeAuthorizationModel(ctx, body)
	if err != nil {
		return fmt.Errorf("writing authorization model: %w", err)
	}

	c.modelID = data.AuthorizationModelId

	return nil
}

func readFile(path string) ([]byte, error) {
	file, err := os.Open(path) //nolint:gosec // путь приходит из конфига
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			logrus.WithError(err).Error("error closing file")
		}
	}()

	return io.ReadAll(file)
}

func transformDSLToJSON(data []byte, v any) error {
	jsonModel, err := transformer.TransformDSLToJSON(string(data))
	if err != nil {
		return fmt.Errorf("parsing DSL model: %w", err)
	}

	if err := json.Unmarshal([]byte(jsonModel), v); err != nil {
		return fmt.Errorf("unmarshalling transformed model: %w", err)
	}

	return nil
}

func (c *Client) writeAuthorizationModel(ctx context.Context, body openfga.WriteAuthorizationModelRequest) (*openFGAClient.ClientWriteAuthorizationModelResponse, error) {
	data, err := c.fgaClient.WriteAuthorizationModel(ctx).Body(body).Execute()
	if err != nil {
		return nil, fmt.Errorf("fga: %w", err)
	}

	return data, nil
}
