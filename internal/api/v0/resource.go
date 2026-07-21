package v0

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"auth-service/internal/model"
	"auth-service/internal/service/fga"
	"auth-service/pkg/audit"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

// ResourceHandler - хендлер обновления ресурсов.
type ResourceHandler struct {
	resourceService resourceService // сервис обновления ресурса
}

// resourceService - сервис обновления ресурса.
//
//go:generate mockgen -source=resource.go -destination=mocks/resource_svc_mock.go -package=mocks resourceService
type resourceService interface {
	// UpdateResource обновляет ресурс.
	UpdateResource(ctx context.Context, req model.UpdateResourceRequest) (model.UpdateResourceResponse, error)
}

// resourceHandlerOption - опция для хендлера обновления ресурсов.
type resourceHandlerOption func(*ResourceHandler)

// WithResourceService устанавливает сервис обновления ресурсов.
func WithResourceService(svc resourceService) resourceHandlerOption {
	return func(h *ResourceHandler) {
		h.resourceService = svc
	}
}

// NewResourceHandler создает новый хендлер обновления ресурсов.
func NewResourceHandler(opts ...resourceHandlerOption) (*ResourceHandler, error) {
	h := &ResourceHandler{}

	for _, opt := range opts {
		opt(h)
	}

	if h.resourceService == nil {
		return nil, errors.New("resource service is not set")
	}

	return h, nil
}

// UpdateResource обновляет ресурс.
func (h *ResourceHandler) UpdateResource(c echo.Context) error {
	var req model.UpdateResourceRequest

	telegramID := c.Request().Header.Get(userIDHeader)

	if telegramID == "" {
		logrus.Errorf("invalid telegram id in header '%s'", userIDHeader)
		return errResponse(c, http.StatusBadRequest, fmt.Errorf("header '%s' is required", userIDHeader))
	}

	telegramIDInt, err := strconv.Atoi(telegramID)
	if err != nil {
		logrus.WithError(err).Error("error converting telegram id to int")
		return errResponse(c, http.StatusBadRequest, fmt.Errorf("invalid telegram id in header '%s'", userIDHeader))
	}

	req.TelegramID = telegramIDInt

	if err := c.Bind(&req); err != nil {
		logrus.WithError(err).Error("error binding request")
		return errResponse(c, http.StatusBadRequest, errors.New("cannot bind request"))
	}

	resp, err := h.resourceService.UpdateResource(c.Request().Context(), req)
	if err != nil {
		logrus.WithError(err).Error("error updating resource")

		if fgaErr, ok := errors.AsType[*fga.DetailedError](err); ok {
			return h.responseFromError(c, req, resp, fgaErr.Err, fgaErr)
		}

		return h.responseFromError(c, req, resp, err, nil)
	}

	return c.JSON(http.StatusOK, resp)
}

//nolint:cyclop // много тест-кейсов, но это зависит от количества ошибок.
func (h *ResourceHandler) responseFromError(c echo.Context, req model.UpdateResourceRequest, resp model.UpdateResourceResponse, err error, detailedError *fga.DetailedError) error {
	switch {
	case errors.Is(err, fga.ErrResounceAlreadyExistsOrNotFound):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeResourceAlreadyExistsOrNotFound, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusConflict, errResp)

	case errors.Is(err, fga.ErrNoTuplesToWriteOrDelete):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrNoTuplesToWriteOrDelete, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusBadRequest, errResp)

	case errors.Is(err, fga.ErrUserNotFound):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeUserNotFound, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusNotFound, errResp)

	case errors.Is(err, fga.ErrOwnerRequired):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeOwnerRequired, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusBadRequest, errResp)

	case errors.Is(err, fga.ErrParentRequired):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeParentRequired, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusBadRequest, errResp)

	case errors.Is(err, fga.ErrChangeTypeInvalid):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeChangeTypeInvalid, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusBadRequest, errResp)

	case errors.Is(err, fga.ErrEventTypeInvalid):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeEventTypeInvalid, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusBadRequest, errResp)

	case errors.Is(err, fga.ErrParentNotAllowed):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeParentNotAllowed, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusBadRequest, errResp)

	case errors.Is(err, fga.ErrResourceEmpty):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeResourceEmpty, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusBadRequest, errResp)

	case errors.Is(err, fga.ErrOwnerTypeInvalid):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeOwnerTypeInvalid, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusBadRequest, errResp)

	case errors.Is(err, fga.ErrParentTypeInvalid):
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeParentTypeInvalid, err.Error(), req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusBadRequest, errResp)

	default:
		errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrInternalServerError, "internal server error", req.Operation, detailedError), resp.Meta)
		return c.JSON(http.StatusInternalServerError, errResp)
	}
}

// errorResponse - ответ для случая, когда операция прошла неуспешно.
type errorResponse struct {
	RequestID    uuid.UUID      `json:"request_id"`
	Status       model.Status   `json:"status"`
	Result       model.Result   `json:"result"`
	Resource     model.Resource `json:"resource"`
	ErrorMessage errorMessage   `json:"error"`
	Meta         model.Meta     `json:"meta"`
}

type errorMessage struct {
	Code    audit.ErrorCode `json:"code"`
	Message string          `json:"message"`
	Details struct {
		Operation          model.Operation `json:"operation"`
		*fga.DetailedError `json:"detailed_error"`
	} `json:"details"`
}

//nolint:unparam // в будущем функция будет получать и другие статусы.
func (h *ResourceHandler) createErrorResponse(requestID uuid.UUID, status model.Status, result model.Result, resource model.Resource, errorMessage errorMessage, meta model.Meta) errorResponse {
	return errorResponse{
		RequestID:    requestID,
		Status:       status,
		Result:       result,
		Resource:     resource,
		ErrorMessage: errorMessage,
		Meta:         meta,
	}
}

func newMessage(code audit.ErrorCode, message string, operation model.Operation, detailedError *fga.DetailedError) errorMessage {
	return errorMessage{
		Code:    code,
		Message: message,
		Details: struct {
			Operation          model.Operation `json:"operation"`
			*fga.DetailedError `json:"detailed_error"`
		}{
			Operation:     operation,
			DetailedError: detailedError,
		},
	}
}

func errResponse(c echo.Context, statusCode int, err error) error {
	return c.JSON(statusCode, map[string]string{"error": err.Error()})
}
