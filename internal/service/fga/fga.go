package fga

import (
	"auth-service/internal/model"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
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
	userRepo           userRepo
}

//go:generate mockgen -source=fga.go -destination=mocks/user_repo_mock.go -package=mocks userRepo
type userRepo interface {
	GetUserIDByTelegramID(ctx context.Context, telegramID string) (uuid.UUID, error)
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

// WithUserRepo устанавливает репозиторий пользователей.
func WithUserRepo(userRepo userRepo) option {
	return func(c *Client) {
		c.userRepo = userRepo
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

	if c.userRepo == nil {
		return nil, fmt.Errorf("fga: user repo is required")
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

// ErrResourceAlreadyExistsOrNotFound ошибка о том, что либо создаваемый ресурс уже существует, либо удаляемый - не найден.
var (
	ErrResourceAlreadyExistsOrNotFound = errors.New("new resource already exists or deleted resource not found")
	ErrUserNotFound                    = errors.New("user not found")
)

const (
	// OperationUpdateResource связывает аудит HTTP-запроса и синхронизации в FGA.
	OperationUpdateResource = "fga.update_resource"
	stageIdentifyActor      = "identify_actor"
	stageValidateResource   = "validate_resource"

	stageMapTuples   = "map_tuples"
	stageWriteTuples = "write_tuples"
)

// UpdateResource обновляет ресурс в OpenFGA.
// Если создаваемый ресурс уже существует или удаляемый ресурс не найден, возвращает ошибку ErrResourceAlreadyExistsOrNotFound.
//
//nolint:funlen // много строк из-за аудита
func (c *Client) UpdateResource(ctx context.Context, req model.UpdateResourceRequest) (model.UpdateResourceResponse, error) {
	a := audit.Current(ctx, OperationUpdateResource)
	if a == nil {
		return model.UpdateResourceResponse{}, audit.ErrMissingAttempt
	}

	a.Correlate(req.RequestID)
	a.At(stageIdentifyActor)

	userID, err := c.userRepo.GetUserIDByTelegramID(ctx, strconv.Itoa(req.TelegramID))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			a.Fail(audit.ErrCodeUserNotFound, audit.KindValidation, audit.ErrLevelError, "User is not registered", storage.ErrNotFound)

			return model.UpdateResourceResponse{}, &DetailedError{
				Err:   ErrUserNotFound,
				Value: nil,
			}
		}

		a.Fail(audit.ErrCodeServiceUnavailable, audit.KindInfra, audit.ErrLevelError, "Identity store is unavailable", nil)

		return model.UpdateResourceResponse{}, &DetailedError{
			Err:   fmt.Errorf("fga: error getting user id by telegram id: %w", err),
			Value: req.TelegramID,
		}
	}

	a.Context(audit.EventContext{
		audit.KeyUserID:  userID.String(),
		audit.KeySubject: string(model.ResourceTypeUser) + ":" + userID.String(),
	})
	a.At(stageValidateResource)

	if err := c.validateRequest(req); err != nil {
		var reason error = err
		if errors.Is(err, errInvalidResourceType) {
			reason = errInvalidResourceType
		}

		a.Fail(audit.ErrCodeWriteFailedDueToInvalidInput, audit.KindValidation, audit.ErrLevelDebug, "Invalid resource update request", reason)

		return model.UpdateResourceResponse{}, err
	}

	a.At(stageMapTuples)

	fgaRequest, err := toOpenFGAWriteRequest(req)
	if err != nil {
		if errors.Is(err, ErrNoTuplesToWriteOrDelete) {
			a.Fail(audit.ErrCodeWriteFailedDueToInvalidInput, audit.KindValidation, audit.ErrLevelError, "Could not map resource update to tuples", ErrNoTuplesToWriteOrDelete)

			return model.UpdateResourceResponse{}, &DetailedError{
				Err:   ErrNoTuplesToWriteOrDelete,
				Value: nil,
			}
		}

		a.Fail(audit.ErrCodeWriteFailedDueToInvalidInput, audit.KindValidation, audit.ErrLevelError, "Could not map resource update to tuples", nil)

		return model.UpdateResourceResponse{}, &DetailedError{
			Err:   fmt.Errorf("convert request to openfga tuples: %w", err),
			Value: nil,
		}
	}

	writtenTuples, deletedTuples := toModelTuples(fgaRequest.Writes, fgaRequest.Deletes)

	a.At(stageWriteTuples)

	_, err = c.fgaClient.Write(ctx).Body(fgaRequest).Options(openFGAClient.ClientWriteOptions{
		AuthorizationModelId: &c.modelID,
	}).Execute()
	if err != nil {
		return model.UpdateResourceResponse{}, tupleWriteFailure(a, err)
	}

	a.Context(audit.EventContext{audit.KeyResult: model.ResultApplied, audit.KeyAuthorizationModelID: c.modelID})

	return model.UpdateResourceResponse{
		RequestID:      req.RequestID,
		IdempotencyKey: req.IdempotencyKey,
		Status:         model.StatusCompleted,
		Result:         model.ResultApplied,
		Resource:       req.Resource,
		WrittenTuples:  writtenTuples,
		DeletedTuples:  deletedTuples,
		Meta: model.Meta{
			AuthModelID: c.modelID,
		},
	}, nil
}

type DetailedError struct {
	Err     error  `json:"-"` // поле для внутреннего использования
	Message string `json:"message"`
	Value   any    `json:"value"`
}

func (d *DetailedError) Error() string {
	return d.Err.Error()
}

func (d *DetailedError) Unwrap() error {
	return d.Err
}

func (c *Client) validateRequest(req model.UpdateResourceRequest) *DetailedError {
	if req.Resource.IsEmpty() {
		return &DetailedError{
			Err:   ErrResourceEmpty,
			Value: req.Resource,
		}
	}

	switch req.Resource.Type {
	case model.ResourceTypeNote:
		return c.validateNoteRequest(req)
	case model.ResourceTypeReminder:
		return c.validateReminderRequest(req)
	case model.ResourceTypeSpace:
		return c.validateSpaceRequest(req)
	default:
		return &DetailedError{
			Err:   fmt.Errorf("%w: %s", errInvalidResourceType, req.Resource.Type),
			Value: req.Resource.Type,
		}
	}
}

var (
	errInvalidResourceType = errors.New("invalid resource type")
	ErrOwnerRequired       = errors.New("owner is required")
	ErrParentRequired      = errors.New("parent is required")
	ErrChangeTypeInvalid   = errors.New("change type is invalid")
	ErrOperationInvalid    = errors.New("operation is invalid")
	ErrEventTypeInvalid    = errors.New("event type is invalid")
	ErrParentNotAllowed    = errors.New("parent is not allowed")
	ErrResourceEmpty       = errors.New("resource is empty")
	ErrOwnerTypeInvalid    = errors.New("owner type is invalid: must be user")
	ErrParentTypeInvalid   = errors.New("parent type is invalid: must be space")
)

// validateNoteRequest проверяет запрос на обновление заметки.
// У заметок должны быть родители и владельцы.
// Допустимые типы изменений: добавление, удаление, изменение.
//
//nolint:dupl // нам нужно оставить разделение на разные функции для расширения
func (c *Client) validateNoteRequest(req model.UpdateResourceRequest) *DetailedError {
	if req.Relations.Owner.IsEmpty() {
		return &DetailedError{
			Err:   ErrOwnerRequired,
			Value: req.Relations.Owner,
		}
	}

	if req.Relations.Owner.Type != model.ResourceTypeUser {
		return &DetailedError{
			Err:   ErrOwnerTypeInvalid,
			Value: req.Relations.Owner,
		}
	}

	if req.Relations.Parent.IsEmpty() {
		return &DetailedError{
			Err:   ErrParentRequired,
			Value: req.Relations.Parent,
		}
	}

	if req.Relations.Parent.Type != model.ResourceTypeSpace {
		return &DetailedError{
			Err:   ErrParentTypeInvalid,
			Value: req.Relations.Parent,
		}
	}

	noteAllowedChangeTypes := []model.ChangeType{
		model.ChangeTypeResourceAdded,
		model.ChangeTypeResourceRemoved,
		model.ChangeTypeResourceMoved,
	}

	if err := checkChangeType(req.ChangeType, noteAllowedChangeTypes); err != nil {
		return err
	}

	if err := checkOperation(req.ChangeType, req.Operation); err != nil {
		return err
	}

	if err := checkEventType(req.Operation, req.Context.EventType, model.EventTypePrefixNote); err != nil {
		return err
	}

	return nil
}

// validateReminderRequest проверяет запрос на обновление напоминания.
// У напоминаний должны быть родители и владельцы.
// Допустимые типы изменений: добавление, удаление, изменение.
//
//nolint:dupl // нам нужно оставить разделение на разные функции для расширения
func (c *Client) validateReminderRequest(req model.UpdateResourceRequest) *DetailedError {
	if req.Relations.Owner.IsEmpty() {
		return &DetailedError{
			Err:   ErrOwnerRequired,
			Value: req.Relations.Owner,
		}
	}

	if req.Relations.Owner.Type != model.ResourceTypeUser {
		return &DetailedError{
			Err:   ErrOwnerTypeInvalid,
			Value: req.Relations.Owner,
		}
	}

	if req.Relations.Parent.IsEmpty() {
		return &DetailedError{
			Err:   ErrParentRequired,
			Value: req.Relations.Parent,
		}
	}

	if req.Relations.Parent.Type != model.ResourceTypeSpace {
		return &DetailedError{
			Err:   ErrParentTypeInvalid,
			Value: req.Relations.Parent,
		}
	}

	reminderAllowedChangeTypes := []model.ChangeType{
		model.ChangeTypeResourceAdded,
		model.ChangeTypeResourceRemoved,
		model.ChangeTypeResourceMoved,
	}

	if err := checkChangeType(req.ChangeType, reminderAllowedChangeTypes); err != nil {
		return err
	}

	if err := checkOperation(req.ChangeType, req.Operation); err != nil {
		return err
	}

	if err := checkEventType(req.Operation, req.Context.EventType, model.EventTypePrefixReminder); err != nil {
		return err
	}

	return nil
}

// validateSpaceRequest проверяет запрос на обновление пространства.
// У пространств нет родителей, только владельцы.
// Допустимые типы изменений: добавление, удаление, изменение прав и состава участников.
func (c *Client) validateSpaceRequest(req model.UpdateResourceRequest) *DetailedError {
	if req.Relations.Owner.IsEmpty() {
		return &DetailedError{
			Err:   ErrOwnerRequired,
			Value: req.Relations.Owner,
		}
	}

	if !req.Relations.Parent.IsEmpty() {
		return &DetailedError{
			Err:     ErrParentNotAllowed,
			Message: "spaces do not have parents, only owners",
			Value:   req.Relations.Parent,
		}
	}

	spaceAllowedChangeTypes := []model.ChangeType{
		model.ChangeTypeResourceAdded,
		model.ChangeTypeResourceRemoved,
		model.ChangeTypeMembershipChanged,
		model.ChangeTypeMembershipAdded,
		model.ChangeTypeMembershipRemoved,
	}

	if err := checkChangeType(req.ChangeType, spaceAllowedChangeTypes); err != nil {
		return err
	}

	if err := checkOperation(req.ChangeType, req.Operation); err != nil {
		return err
	}

	if err := checkEventType(req.Operation, req.Context.EventType, model.EventTypePrefixSpace); err != nil {
		return err
	}

	return nil
}

func checkOperation(changeType model.ChangeType, originalOperation model.Operation) *DetailedError {
	switch originalOperation {
	case model.OperationCreate, model.OperationUpdate, model.OperationDelete:
	default:
		expected := []string{string(model.OperationCreate), string(model.OperationUpdate), string(model.OperationDelete)}

		return &DetailedError{
			Err:     ErrOperationInvalid,
			Message: fmt.Sprintf("operation mismatch: expected one of `%s`, got `%s`", strings.Join(expected, ", "), originalOperation),
			Value:   originalOperation,
		}
	}

	operation := model.ChangeTypeToOperation[changeType]
	if operation != originalOperation {
		return &DetailedError{
			Err:     ErrChangeTypeInvalid,
			Message: fmt.Sprintf("unexpected operation for change type: expected `%s`, got `%s`", operation, originalOperation),
			Value:   originalOperation,
		}
	}

	return nil
}

func checkChangeType(changeType model.ChangeType, allowedChangeTypes []model.ChangeType) *DetailedError {
	if !changeType.IsOneOf(allowedChangeTypes...) {
		allowed := make([]string, len(allowedChangeTypes))
		for i, ct := range allowedChangeTypes {
			allowed[i] = string(ct)
		}

		return &DetailedError{
			Err:     ErrChangeTypeInvalid,
			Message: fmt.Sprintf("change type mismatch: expected one of `%s`, got `%s`", strings.Join(allowed, ", "), changeType),
			Value:   changeType,
		}
	}

	return nil
}

func checkEventType(operation model.Operation, originalEventType string, eventTypePrefix string) *DetailedError {
	eventTypePostfix := ""

	switch operation {
	case model.OperationCreate:
		eventTypePostfix = model.EventTypeOperationCreatedPostfix
	case model.OperationDelete:
		eventTypePostfix = model.EventTypeOperationDeletedPostfix
	case model.OperationUpdate:
		eventTypePostfix = model.EventTypeOperationUpdatedPostfix
	}

	eventType := model.FormatEventType(eventTypePrefix, eventTypePostfix)
	if eventType != originalEventType {
		return &DetailedError{
			Err:     ErrEventTypeInvalid,
			Message: fmt.Sprintf("event type mismatch: expected `%s`, got `%s`", eventType, originalEventType),
			Value:   originalEventType,
		}
	}

	return nil
}

func tupleWriteFailure(a *audit.Attempt, err error) error {
	validation, invalid := errors.AsType[openfga.FgaApiValidationError](err)
	conflict, apiError := errors.AsType[openfga.FgaApiError](err)
	duplicate := invalid && validation.ResponseCode() == openfga.ERRORCODE_WRITE_FAILED_DUE_TO_INVALID_INPUT
	aborted := apiError && conflict.ResponseStatusCode() == http.StatusConflict && conflict.ResponseCode() == string(openfga.INTERNALERRORCODE_ABORTED)

	if duplicate {
		a.Fail(audit.ErrCodeWriteFailedDueToInvalidInput, audit.KindValidation, audit.ErrLevelError, "OpenFGA rejected the tuple changes", validation)
		return &DetailedError{Err: ErrResourceAlreadyExistsOrNotFound}
	}

	if aborted {
		a.Fail(audit.ErrCodeWriteFailedDueToInvalidInput, audit.KindValidation, audit.ErrLevelError, "OpenFGA rejected the tuple changes", ErrResourceAlreadyExistsOrNotFound)
		return &DetailedError{Err: ErrResourceAlreadyExistsOrNotFound}
	}

	a.Fail(audit.ErrCodeServiceUnavailable, audit.KindInfra, audit.ErrLevelError, "Could not write tuples to OpenFGA", nil)

	return &DetailedError{Err: fmt.Errorf("write tuples to openfga: %w", err)}
}
