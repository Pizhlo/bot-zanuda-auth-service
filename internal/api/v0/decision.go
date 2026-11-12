package v0

import (
	"context"
	"errors"
	"io"
	"net/http"

	"auth-service/internal/api/internal/response"
	"auth-service/internal/model"
	"auth-service/internal/service/decision"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

//go:generate mockgen -source=decision.go -destination=mocks/decision_svc_mock.go -package=mocks

type decisionService interface {
	Check(context.Context, decision.Caller, model.CheckDecisionRequest) (model.CheckDecisionResponse, error)
}

// WithDecisionService подключает вычисление решений к обработчику.
func WithDecisionService(svc decisionService) resourceHandlerOption {
	return func(h *ResourceHandler) { h.decisionService = svc }
}

// CheckDecision проверяет одно действие.
// @Summary Проверить одно действие
// @Description Проверка отношений и всей маски полей без изменения данных. Deny возвращается с HTTP 200.
// @Tags decisions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Telegram-User-Id header string true "Доверенный Telegram ID"
// @Param request body model.CheckDecisionRequest true "Одно действие"
// @Success 200 {object} model.CheckDecisionResponse
// @Failure 400,401,403,404,500,503 {object} response.DecisionErrorBody
// @Router /v0/decisions/check [post].
func (h *ResourceHandler) CheckDecision(c echo.Context) error {
	const maxDecisionBody = 1 << 20

	data, err := io.ReadAll(io.LimitReader(c.Request().Body, maxDecisionBody+1))
	if err != nil || len(data) > maxDecisionBody {
		return decisionError(c, model.ErrInvalidDecisionRequest, uuid.Nil)
	}

	req, err := model.DecodeDecision(data)
	if err != nil {
		return decisionError(c, err, req.RequestID)
	}

	if h.decisionService == nil {
		return decisionError(c, errors.New("decision service is not configured"), req.RequestID)
	}

	caller := decision.Caller{ClientID: decisionClient(c.Request().Context()), TelegramID: c.Request().Header.Get(userIDHeader)}

	resp, err := h.decisionService.Check(c.Request().Context(), caller, req)
	if err != nil {
		return decisionError(c, err, req.RequestID)
	}

	return c.JSON(http.StatusOK, resp)
}

func decisionError(c echo.Context, err error, requestID uuid.UUID) error {
	result := response.DecisionError(err, requestID)
	logrus.WithFields(logrus.Fields{"error_code": result.Body.ErrorCode, "request_id": requestID.String()}).Warn("decision request failed")

	return c.JSON(result.StatusCode, result.Body)
}
