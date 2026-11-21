package v0

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"auth-service/internal/api/internal/response"
	"auth-service/internal/model"
	"auth-service/internal/service/fga"
	"auth-service/pkg/audit"

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

const stageApplyResource = "update_resource"

// UpdateResource обновляет ресурс.
func (h *ResourceHandler) UpdateResource(c echo.Context) error {
	a := audit.Current(c.Request().Context(), fga.OperationUpdateResource)
	if a == nil {
		return errResponse(c, http.StatusInternalServerError, audit.ErrMissingAttempt)
	}

	a.At(audit.StageDecodeRequest.String())

	var req model.UpdateResourceRequest

	if err := decodeResourceUpdate(c, &req); err != nil {
		a.Correlate(req.RequestID)
		a.Fail(audit.ErrCodeValidationFailed, audit.KindValidation, audit.ErrLevelDebug, err.Error(), err)
		logrus.WithError(err).Warn("invalid resource update")

		result := response.SimpleError(http.StatusBadRequest, err)

		return c.JSON(result.StatusCode, result.Body)
	}

	a.Correlate(req.RequestID)
	a.At(stageApplyResource)

	resp, err := h.resourceService.UpdateResource(c.Request().Context(), req)
	if err != nil {
		a.NoteCancellation(err)
		logrus.WithError(err).Error("error updating resource")

		result := response.ResourceError(err, response.ResourceContext{
			RequestID: req.RequestID, Resource: req.Resource, Operation: req.Operation, Meta: resp.Meta,
		})

		a.Fail(result.Body.ErrorMessage.Code, audit.KindDomain, audit.ErrLevelError, result.Body.ErrorMessage.Message, nil)

		return c.JSON(result.StatusCode, result.Body)
	}

	a.At(audit.StageWriteResponse.String())

	return c.JSON(http.StatusOK, resp)
}

// decodeResourceUpdate возвращает только сообщения, безопасные для клиента и аудита.
func decodeResourceUpdate(c echo.Context, req *model.UpdateResourceRequest) error {
	header := c.Request().Header.Get(userIDHeader)
	if header == "" {
		return fmt.Errorf("header '%s' is required", userIDHeader)
	}

	id, err := strconv.Atoi(header)
	if err != nil {
		return fmt.Errorf("invalid telegram id in header '%s'", userIDHeader)
	}

	req.TelegramID = id
	if err := c.Bind(req); err != nil {
		return errors.New("cannot bind request")
	}

	return nil
}
