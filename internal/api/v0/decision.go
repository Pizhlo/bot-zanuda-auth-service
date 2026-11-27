package v0

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"auth-service/internal/api/internal/response"
	"auth-service/internal/model"
	"auth-service/internal/service/decision"
	"auth-service/pkg/audit"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

//go:generate mockgen -source=decision.go -destination=mocks/decision_svc_mock.go -package=mocks

type decisionService interface {
	Check(context.Context, decision.Caller, model.CheckDecisionRequest) (model.CheckDecisionResponse, error)
}

// WithDecisionService подключает отдельный сервис вычисления решений к HTTP-обработчику.
// Сервис синхронизации ресурсов остаётся самостоятельной зависимостью.
func WithDecisionService(svc decisionService) resourceHandlerOption {
	return func(h *ResourceHandler) { h.decisionService = svc }
}

// CheckDecision читает тело запроса, валидирует контракт и передаёт доверенного caller.
// Маршрут должен быть защищён CheckToken с контрактом DecisionToken; завершённый deny возвращается с 200,
// а ошибки валидации или зависимостей — отдельным телом без allow/result.
// @Summary Проверить одно действие
// @Description Проверка отношений и всей маски полей с сохранением решения в PostgreSQL. Deny возвращается с HTTP 200. При include_reasons причины содержат проверенные relation/object/subject и имена запрошенных полей. Ошибка 400 указывает в message проблемный заголовок или JSON-путь и ограничение, включая ошибки UUID и типов.
// @Tags decisions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Request-ID header string false "UUID для корреляции отказа до чтения тела"
// @Param X-Telegram-User-Id header string true "Доверенный Telegram ID"
// @Param request body model.CheckDecisionRequest true "Одно действие"
// @Success 200 {object} model.CheckDecisionResponse
// @Failure 400,401,403,404,500,503 {object} response.DecisionErrorBody
// @Router /api/v0/decisions/check [post].
func (h *ResourceHandler) CheckDecision(c echo.Context) error {
	ctx := c.Request().Context()
	if err := decision.RequireAudit(ctx); err != nil {
		return decisionError(ctx, c, err, uuid.Nil)
	}

	// DecodeDecision проверяет структуру JSON и возвращает путь ошибочного поля.
	// Обычный Bind допускает неизвестные поля, а при ошибке UUID теряет путь.
	data, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return decisionError(ctx, c, fmt.Errorf("read decision body: %w: %w", model.InvalidDecisionRequest("body could not be read"), err), uuid.Nil)
	}

	req, err := model.DecodeDecision(data)
	if err != nil {
		return decisionError(ctx, c, err, req.RequestID)
	}

	caller := decision.Caller{ClientID: serviceClientID(ctx), TelegramID: c.Request().Header.Get(userIDHeader)}

	resp, err := h.decisionService.Check(ctx, caller, req)
	if err != nil {
		return decisionError(ctx, c, err, req.RequestID)
	}

	c.Response().Header().Set(echo.HeaderXRequestID, resp.RequestID.String())

	return c.JSON(http.StatusOK, resp)
}

func decisionError(ctx context.Context, c echo.Context, err error, requestID uuid.UUID) error {
	correlationID := requestID
	if correlationID == uuid.Nil {
		transportID, _ := ctx.Value(requestIDKey{}).(string)

		correlationID, _ = uuid.Parse(transportID)
		if correlationID == uuid.Nil {
			correlationID = uuid.New()
		}
	}

	decision.AuditRejection(ctx, correlationID, err)

	return writeDecisionError(c, err, requestID, correlationID)
}

// DecisionHTTPErrorHandler сохраняет формат ошибки check после записи паники в аудит.
// Остальные ошибки обрабатывает стандартный обработчик Echo.
func DecisionHTTPErrorHandler(err error, c echo.Context) {
	requestID, correlationID := decision.PanicCorrelation(c.Request().Context())
	if correlationID == uuid.Nil {
		c.Echo().DefaultHTTPErrorHandler(err, c)
		return
	}

	if c.Response().Committed {
		return
	}

	if requestID == uuid.Nil {
		requestID = decisionRequestHeaderID(c.Request().Header)
	}

	if writeErr := writeDecisionError(c, errors.New("decision request panicked"), requestID, correlationID); writeErr != nil {
		logrus.WithField(audit.KeyRequestID, correlationID.String()).Warn("failed to write decision error response")
	}
}

func writeDecisionError(c echo.Context, err error, requestID, correlationID uuid.UUID) error {
	result := response.DecisionError(err, requestID)
	id := correlationID.String()
	c.Response().Header().Set(echo.HeaderXRequestID, id)
	logrus.WithFields(logrus.Fields{"error_code": result.Body.ErrorCode, "request_id": id}).Warn("decision request failed")

	return c.JSON(result.StatusCode, result.Body)
}
