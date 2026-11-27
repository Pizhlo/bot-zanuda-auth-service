package v0

import (
	"auth-service/internal/model"
	"auth-service/internal/service/decision"
	"auth-service/internal/service/fga"
	"auth-service/internal/storage"
	"auth-service/pkg/audit"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

// MiddlewareHandler - хендлер для использования middleware.
type MiddlewareHandler struct {
	requestAuditor audit.Source
	authService    AuthService
}

// AuthService - интерфейс для доступа к сервису авторизации для работы с токеном.
//
//go:generate mockgen -source=middleware.go -destination=mocks/mocks.go -package=mocks AuthService
type AuthService interface {
	CheckToken(ctx context.Context, authHeader string) (*jwt.Token, error)
	GetPayload(token *jwt.Token) (jwt.MapClaims, bool)
	GetServiceClient(ctx context.Context, clientID string) (model.ServiceClient, error)
	GetIssuer() string
}

type option func(*MiddlewareHandler)

// WithMiddlewareAuthService устанавливает сервис авторизации.
func WithMiddlewareAuthService(svc AuthService) option {
	return func(h *MiddlewareHandler) {
		h.authService = svc
	}
}

// WithRequestAuditor подключает аудит пользовательских запросов до проверки токена.
func WithRequestAuditor(source audit.Source) option {
	return func(h *MiddlewareHandler) { h.requestAuditor = source }
}

// NewMiddlewareHandler создает новый хендлер для middleware.
func NewMiddlewareHandler(opts ...option) (*MiddlewareHandler, error) {
	h := &MiddlewareHandler{}

	for _, opt := range opts {
		opt(h)
	}

	if h.authService == nil {
		return nil, errors.New("auth service is required")
	}

	if h.requestAuditor == nil {
		return nil, errors.New("request auditor is required")
	}

	logrus.Info("created handler")

	return h, nil
}

var (
	errNoTokenPayload    = errors.New("invalid token: no payload")
	errInvalidClientID   = errors.New("invalid token: invalid client id")
	errInvalidTokenScope = errors.New("invalid token: invalid scope")
	errInactiveClient    = errors.New("client is inactive")
	errMissingScope      = errors.New("client does not have required scope")
)

const userIDHeader = "X-Telegram-User-Id"
const auditServiceName = "auth-service"

// AuditResourceUpdate создаёт одну попытку для проверки токена, разбора запроса,
// синхронизации и записи ответа.
func (s *MiddlewareHandler) AuditResourceUpdate(next echo.HandlerFunc) echo.HandlerFunc {
	return s.auditRequest(fga.OperationUpdateResource, next)
}

// AuditNotesFilter охватывает проверку подлинности, фильтрацию заметок и запись ответа.
func (s *MiddlewareHandler) AuditNotesFilter(next echo.HandlerFunc) echo.HandlerFunc {
	return s.auditRequest(audit.OperationFilterNotes, next)
}

func (s *MiddlewareHandler) auditRequest(operation string, next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) (err error) {
		ctx, a, err := audit.Begin(connectionContext(c.Request()), s.requestAuditor, operation, audit.StageReceiveRequest.String())
		if err != nil {
			return err
		}
		defer a.Finish(ctx, &err)

		id, _ := uuid.Parse(c.Request().Header.Get("X-Request-ID"))
		a.Correlate(id)
		c.SetRequest(c.Request().WithContext(ctx))

		return next(c)
	}
}

// TokenContract указывает CheckToken, как продолжить обработку конкретного маршрута.
// У notes/filter и resources/update middleware проверяет Telegram-заголовок и сохраняет
// прежний формат ошибок. У decisions/check сервис сверяет заголовок с subject.id,
// а middleware оформляет отказ с error_code и request_id. JWT и scope bot проверяются одинаково.
type TokenContract uint8

const (
	// ResourceToken требует Telegram-заголовок до синхронизации или фильтрации.
	ResourceToken TokenContract = iota + 1
	// DecisionToken передаёт проверку Telegram ID сервису решений для сверки с subject.
	DecisionToken
)

// CheckToken проверяет подпись и срок JWT, затем находит активного клиента и сверяет его scope.
// Все пользовательские маршруты требуют bot в списке scope токена. Контракт маршрута
// определяет формат отказа и место проверки Telegram ID.
func (s *MiddlewareHandler) CheckToken(contract TokenContract) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			ctx := c.Request().Context()
			if startErr := beginAuthentication(ctx, contract); startErr != nil {
				if contract == DecisionToken {
					return decisionError(ctx, c, startErr, decisionRequestHeaderID(c.Request().Header))
				}

				return errResponse(c, http.StatusInternalServerError, startErr)
			}

			defer recordResponseWriteFailure(audit.FromContext(ctx), c.Response(), &err)

			claims, err := s.verifiedPayload(ctx, c.Request().Header.Get("Authorization"))
			if err != nil {
				return rejectInvalidToken(c, contract, err)
			}

			clientID, err := s.verifiedClient(ctx, claims, model.BotScope)
			if err != nil {
				return rejectServiceClient(c, contract, err)
			}

			return serveAuthenticated(c, contract, clientID, next)
		}
	}
}

// recordResponseWriteFailure дополняет аудит notes/filter и resources/update, если
// запись начатого ответа вернула ошибку. У решения свой обработчик записи ответа.
func recordResponseWriteFailure(attempt *audit.Attempt, response *echo.Response, result *error) {
	if attempt != nil && response.Committed && *result != nil {
		attempt.Context(audit.EventContext{audit.KeyResponseWriteFailed: true})
	}
}

// beginAuthentication требует событие выбранной операции до чтения JWT и обращения к реестру клиентов.
func beginAuthentication(ctx context.Context, contract TokenContract) error {
	if contract == DecisionToken {
		if err := decision.RequireAudit(ctx); err != nil {
			return err
		}

		decision.AuditAuthenticationStarted(ctx)

		return nil
	}

	a := audit.FromContext(ctx)
	if a == nil {
		return audit.ErrMissingAttempt
	}

	a.At(audit.StageAuthenticate.String())

	return nil
}

// serveAuthenticated передаёт обработчику проверенный ID сервисного клиента.
// У decisions/check сервис сверяет Telegram ID с subject из тела. Для синхронизации
// и фильтрации обязательный заголовок проверяется здесь, до вызова обработчика.
func serveAuthenticated(c echo.Context, contract TokenContract, clientID string, next echo.HandlerFunc) error {
	ctx := context.WithValue(c.Request().Context(), clientIDKey{}, clientID)
	if contract == DecisionToken {
		decision.AuditAuthenticated(ctx, clientID)
	} else {
		audit.FromContext(ctx).Context(audit.EventContext{audit.KeyCaller: clientID})

		userID := c.Request().Header.Get(userIDHeader)
		if userID == "" {
			return tokenRejection(c, http.StatusUnauthorized, fmt.Errorf("invalid request: header '%s' not found", userIDHeader), audit.ErrCodeValidationFailed, "Telegram user header is missing")
		}

		if !model.ValidTelegramID(userID) {
			return tokenRejection(c, http.StatusBadRequest, errors.New("invalid Telegram user ID"), audit.ErrCodeValidationFailed, "Telegram user ID must be a positive decimal int64")
		}

		ctx = withUserID(ctx, userID)
	}

	c.SetRequest(c.Request().WithContext(ctx))

	return next(c)
}

type clientIDKey struct{}

// serviceClientID возвращает идентификатор зарегистрированного клиента, проверенный CheckToken.
func serviceClientID(ctx context.Context) string {
	id, _ := ctx.Value(clientIDKey{}).(string)
	return id
}

func rejectInvalidToken(c echo.Context, contract TokenContract, err error) error {
	if contract == DecisionToken {
		return decisionError(c.Request().Context(), c, fmt.Errorf("%w: %w", decision.ErrUnauthorized, err), decisionRequestHeaderID(c.Request().Header))
	}

	if errors.Is(err, errNoTokenPayload) {
		logrus.Error(errNoTokenPayload)
		return tokenRejection(c, http.StatusUnauthorized, err, audit.ErrCodeTokenInvalid, "Token has no payload")
	}

	logrus.WithError(err).Error("error checking token")

	return tokenRejection(c, http.StatusUnauthorized, err, audit.ErrCodeTokenInvalid, audit.MessageTokenVerificationFailed)
}

func rejectServiceClient(c echo.Context, contract TokenContract, err error) error {
	if contract == DecisionToken {
		return decisionError(c.Request().Context(), c, decisionClientError(err), decisionRequestHeaderID(c.Request().Header))
	}

	status := http.StatusUnauthorized

	switch {
	case errors.Is(err, storage.ErrNotFound):
		err = errors.New("invalid client")
	case errors.Is(err, errInvalidClientID), errors.Is(err, errInvalidTokenScope), errors.Is(err, errInactiveClient), errors.Is(err, errMissingScope):
		// Сообщения этих ошибок уже предназначены для клиента.
	default:
		status, err = http.StatusInternalServerError, errors.New("internal server error")
	}

	return tokenRejection(c, status, err, audit.ErrCodeServiceInvalid, "Service client verification failed")
}

func tokenRejection(c echo.Context, status int, err error, code audit.ErrorCode, reason string) error {
	if a := audit.FromContext(c.Request().Context()); a != nil {
		a.Correlate(rejectedRequestID(c))

		if status >= http.StatusInternalServerError {
			a.Fail(audit.ErrCodeServiceUnavailable, audit.KindInfra, audit.ErrLevelError, reason, nil)
		} else {
			a.Fail(code, audit.KindValidation, audit.ErrLevelWarn, reason, nil)
		}
	}

	return errResponse(c, status, err)
}

// FillCtx заполняет контекст запроса данными из заголовков.
func FillCtx(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Аудит пользовательской операции уже прочитал заголовки при входе в HTTP.
		if c.Request().Context().Value(userAgentKey{}) == nil {
			c.SetRequest(c.Request().WithContext(connectionContext(c.Request())))
		}

		return next(c)
	}
}

// connectionContext собирает заголовки для ConnectionHook при открытии события.
func connectionContext(r *http.Request) context.Context {
	ctx := r.Context()
	ctx = withTraceID(ctx, r.Header.Get("X-Trace-ID"))
	ctx = withRequestID(ctx, r.Header.Get("X-Request-ID"))

	ipAddress := r.Header.Get("X-Forwarded-For")

	if ipAddress == "" {
		ipAddress = r.RemoteAddr
	}

	ctx = withIPAddress(ctx, ipAddress)

	userAgent := r.Header.Get("User-Agent")

	if userAgent == "" {
		userAgent = "unknown"
	}

	ctx = withUserAgent(ctx, userAgent)

	return ctx
}

type ipAddressKey struct{}

func withIPAddress(ctx context.Context, ipAddress string) context.Context {
	return context.WithValue(ctx, ipAddressKey{}, ipAddress)
}

type userAgentKey struct{}

func withUserAgent(ctx context.Context, userAgent string) context.Context {
	return context.WithValue(ctx, userAgentKey{}, userAgent)
}

type traceIDKey struct{}

func withTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

type requestIDKey struct{}

func withRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// ConnectionHook добавляет данные из соединения (IP-адрес, User-Agent, User-ID) в событие.
func ConnectionHook(ctx context.Context, stash audit.Stash) audit.Stash {
	stash = stash.Append(audit.ServiceName(auditServiceName))
	msgCtx := audit.EventContext{}

	if traceID, ok := ctx.Value(traceIDKey{}).(string); ok {
		stash = stash.Append(audit.TraceID(traceID))
	}

	if requestID, ok := ctx.Value(requestIDKey{}).(string); ok {
		stash = stash.Append(audit.RequestID(requestID))
	}

	if ipAddress, ok := ctx.Value(ipAddressKey{}).(string); ok {
		msgCtx[audit.KeyIPAddress] = ipAddress
	}

	if userAgent, ok := ctx.Value(userAgentKey{}).(string); ok {
		msgCtx[audit.KeyUserAgent] = userAgent
	}

	if userID, ok := ctx.Value(userIDKey{}).(string); ok {
		msgCtx[audit.KeyUserID] = userID
	}

	if len(msgCtx) > 0 {
		stash = stash.Append(audit.ContextField(msgCtx, stash))
	}

	return stash
}

// verifiedPayload проверяет подпись и срок JWT до чтения его заявлений.
func (s *MiddlewareHandler) verifiedPayload(ctx context.Context, header string) (jwt.MapClaims, error) {
	token, err := s.authService.CheckToken(ctx, header)
	if err != nil {
		return nil, err
	}

	payload, ok := s.authService.GetPayload(token)
	if !ok {
		return nil, errNoTokenPayload
	}

	return payload, nil
}

// verifiedClient проверяет идентичность и активность зарегистрированного клиента.
// Список scope токена должен включать обязательное разрешение маршрута. Каждый scope
// в токене должен быть выдан тому же клиенту в реестре сервисных клиентов.
func (s *MiddlewareHandler) verifiedClient(ctx context.Context, payload jwt.MapClaims, required model.Scope) (string, error) {
	clientID, ok := payload["sub"].(string)
	if !ok || clientID == "" {
		return "", errInvalidClientID
	}

	scope, ok := payload["scope"].(string)
	if !ok || !validScopeList(scope) {
		return "", errInvalidTokenScope
	}

	client, err := s.authService.GetServiceClient(ctx, clientID)
	if err != nil {
		return "", fmt.Errorf("look up service client: %w", err)
	}

	if !client.IsActive {
		return "", errInactiveClient
	}

	scopes := strings.Fields(scope)
	if !slices.Contains(scopes, string(required)) {
		return "", errMissingScope
	}

	for _, granted := range scopes {
		if !slices.Contains(client.Scopes, granted) {
			return "", errMissingScope
		}
	}

	return clientID, nil
}

// validScopeList отличает выданный login пустой список прав от повреждённого
// claim. Имена разрешений в непустом списке разделяет обычный пробел.
func validScopeList(scope string) bool {
	if scope == "" {
		return true
	}

	return strings.TrimSpace(scope) != "" && !strings.ContainsFunc(scope, func(ch rune) bool {
		return ch != ' ' && unicode.IsSpace(ch)
	})
}

type userIDKey struct{}

func withUserID(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, userIDKey{}, value)
}

func parseUserID(value any) string {
	switch t := value.(type) {
	case string:
		return t
	default:
		return ""
	}
}
