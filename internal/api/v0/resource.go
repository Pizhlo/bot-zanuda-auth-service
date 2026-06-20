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

		switch {
		case errors.Is(err, fga.ErrResounceAlreadyExistsOrNotFound):
			errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeResourceAlreadyExistsOrNotFound, err.Error(), req.Operation), resp.Meta)
			return c.JSON(http.StatusConflict, errResp)

		case errors.Is(err, fga.ErrNoTuplesToWriteOrDelete):
			errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrNoTuplesToWriteOrDelete, err.Error(), req.Operation), resp.Meta)
			return c.JSON(http.StatusBadRequest, errResp)

		case errors.Is(err, fga.ErrUserNotFound):
			errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrCodeUserNotFound, err.Error(), req.Operation), resp.Meta)
			return c.JSON(http.StatusNotFound, errResp)
		default:
			errResp := h.createErrorResponse(req.RequestID, model.StatusError, model.ResultFailed, req.Resource, newMessage(audit.ErrInternalServerError, "internal server error", req.Operation), resp.Meta)
			return c.JSON(http.StatusInternalServerError, errResp)
		}
	}

	return c.JSON(http.StatusOK, resp)
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
		Operation model.Operation `json:"operation"`
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

func newMessage(code audit.ErrorCode, message string, operation model.Operation) errorMessage {
	return errorMessage{
		Code:    code,
		Message: message,
		Details: struct {
			Operation model.Operation "json:\"operation\""
		}{
			Operation: operation,
		},
	}
}

func errResponse(c echo.Context, statusCode int, err error) error {
	return c.JSON(statusCode, map[string]string{"error": err.Error()})
}
