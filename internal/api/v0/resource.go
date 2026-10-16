package v0

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"auth-service/internal/api/internal/response"
	"auth-service/internal/model"

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

		result := response.SimpleError(http.StatusBadRequest, fmt.Errorf("header '%s' is required", userIDHeader))

		return c.JSON(result.StatusCode, result.Body)
	}

	telegramIDInt, err := strconv.Atoi(telegramID)
	if err != nil {
		logrus.WithError(err).Error("error converting telegram id to int")

		result := response.SimpleError(http.StatusBadRequest, fmt.Errorf("invalid telegram id in header '%s'", userIDHeader))

		return c.JSON(result.StatusCode, result.Body)
	}

	req.TelegramID = telegramIDInt

	if err := c.Bind(&req); err != nil {
		logrus.WithError(err).Error("error binding request")

		result := response.SimpleError(http.StatusBadRequest, errors.New("cannot bind request"))

		return c.JSON(result.StatusCode, result.Body)
	}

	resp, err := h.resourceService.UpdateResource(c.Request().Context(), req)
	if err != nil {
		logrus.WithError(err).Error("error updating resource")

		result := response.ResourceError(err, response.ResourceContext{
			RequestID: req.RequestID, Resource: req.Resource, Operation: req.Operation, Meta: resp.Meta,
		})

		return c.JSON(result.StatusCode, result.Body)
	}

	return c.JSON(http.StatusOK, resp)
}
