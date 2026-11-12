//nolint:funlen // Таблица содержит HTTP-контракт синхронизации для всех исходов.
package v0

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"auth-service/internal/api/internal/response"
	"auth-service/internal/api/v0/mocks"
	"auth-service/internal/model"
	"auth-service/internal/service/fga"
	"auth-service/pkg/audit"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestNewResourceHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      func(t *testing.T, ctrl *gomock.Controller) []resourceHandlerOption
		checkWant func(t *testing.T, handler *ResourceHandler)
		wantErr   require.ErrorAssertionFunc
	}{
		{
			name: "success",
			opts: func(t *testing.T, ctrl *gomock.Controller) []resourceHandlerOption {
				t.Helper()

				return []resourceHandlerOption{
					WithResourceService(mocks.NewMockresourceService(ctrl)),
					WithDecisionService(mocks.NewMockdecisionService(ctrl)),
				}
			},
			checkWant: func(t *testing.T, handler *ResourceHandler) {
				t.Helper()

				require.NotNil(t, handler)
				require.NotNil(t, handler.resourceService)
				require.NotNil(t, handler.decisionService)
			},
			wantErr: require.NoError,
		},
		{
			name: "without resource service",
			opts: func(t *testing.T, ctrl *gomock.Controller) []resourceHandlerOption {
				t.Helper()

				return []resourceHandlerOption{}
			},
			checkWant: func(t *testing.T, handler *ResourceHandler) {
				t.Helper()

				require.Nil(t, handler)
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "resource service is not set")
			},
		},
		{
			name: "without decision service",
			opts: func(t *testing.T, ctrl *gomock.Controller) []resourceHandlerOption {
				t.Helper()
				return []resourceHandlerOption{WithResourceService(mocks.NewMockresourceService(ctrl))}
			},
			checkWant: func(t *testing.T, handler *ResourceHandler) {
				t.Helper()
				require.Nil(t, handler)
			},
			wantErr: func(t require.TestingT, err error, _ ...any) {
				require.EqualError(t, err, "decision service is not set")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			opts := tt.opts(t, ctrl)
			handler, err := NewResourceHandler(opts...)
			tt.wantErr(t, err)

			tt.checkWant(t, handler)
		})
	}
}

func TestUpdateResource(t *testing.T) {
	t.Parallel()

	note := model.UpdateResourceRequest{
		RequestID: uuid.New(), DecisionID: uuid.New(), IdempotencyKey: "idempotency-key", TelegramID: 12345,
		Resource:  model.Resource{ID: uuid.New(), Type: model.ResourceTypeNote},
		Operation: model.OperationCreate, ChangeType: model.ChangeTypeResourceAdded,
		Relations: model.Relation{
			Owner:  model.Resource{ID: uuid.New(), Type: model.ResourceTypeUser},
			Parent: model.Resource{ID: uuid.New(), Type: model.ResourceTypeSpace},
		},
		Context: model.Context{SourceService: "notes-service", EventType: "note.created", TraceID: "trace-id"},
	}
	space := model.UpdateResourceRequest{
		RequestID: uuid.New(), TelegramID: 12345,
		Resource: model.Resource{ID: uuid.New(), Type: model.ResourceTypeSpace}, Operation: model.OperationDelete,
	}
	applied := model.UpdateResourceResponse{
		RequestID: note.RequestID, IdempotencyKey: note.IdempotencyKey,
		Status: model.StatusCompleted, Result: model.ResultApplied, Resource: note.Resource, Meta: model.Meta{AuthModelID: "model-id"},
	}
	detailed := &fga.DetailedError{Err: fga.ErrOwnerRequired, Message: "missing owner", Value: "empty owner"}
	publicDetails := &response.DetailedError{Message: "missing owner", Value: "empty owner"}
	noCall := func(*mocks.MockresourceService) {}
	withAudit := (*MiddlewareHandler).AuditResourceUpdate

	request := func(req model.UpdateResourceRequest) string {
		body, err := json.Marshal(req)
		require.NoError(t, err)

		return string(body)
	}
	noteBody := request(note)
	spaceBody := request(space)
	fail := func(cause error) func(*mocks.MockresourceService) {
		return expectResourceUpdate(note, model.UpdateResourceResponse{Meta: applied.Meta}, cause)
	}
	expected := func(code audit.ErrorCode, message string, details *response.DetailedError) response.ResourceBody {
		return expectedResourceFailure(note, applied.Meta, code, message, details)
	}

	for _, tc := range []struct {
		name             string
		startAudit       func(*MiddlewareHandler, echo.HandlerFunc) echo.HandlerFunc
		body, telegramID string
		expect           func(*mocks.MockresourceService)
		status           int
		reply            any
	}{
		{"without audit", withoutRequestAudit, noteBody, "12345", noCall, http.StatusInternalServerError, map[string]string{"error": audit.ErrMissingAttempt.Error()}},
		{"missing Telegram header", withAudit, noteBody, "", noCall, http.StatusBadRequest, map[string]string{"error": "header 'X-Telegram-User-Id' is required"}},
		{"invalid Telegram header", withAudit, noteBody, "abc", noCall, http.StatusBadRequest, map[string]string{"error": "invalid telegram id in header 'X-Telegram-User-Id'"}},
		{"invalid JSON", withAudit, "{", "12345", noCall, http.StatusBadRequest, map[string]string{"error": "cannot bind request"}},
		{"applied", withAudit, noteBody, "12345", expectResourceUpdate(note, applied, nil), http.StatusOK, applied},
		{"no tuple changes", withAudit, noteBody, "12345", fail(fga.ErrNoTuplesToWriteOrDelete), http.StatusBadRequest, expected(audit.ErrNoTuplesToWriteOrDelete, fga.ErrNoTuplesToWriteOrDelete.Error(), nil)},
		{"missing owner", withAudit, noteBody, "12345", fail(fga.ErrOwnerRequired), http.StatusBadRequest, expected(audit.ErrCodeOwnerRequired, "owner is required", nil)},
		{"missing user", withAudit, noteBody, "12345", fail(fga.ErrUserNotFound), http.StatusNotFound, expected(audit.ErrCodeUserNotFound, "user not found", nil)},
		{"resource conflict", withAudit, noteBody, "12345", fail(fga.ErrResourceAlreadyExistsOrNotFound), http.StatusConflict, expected(audit.ErrCodeResourceAlreadyExistsOrNotFound, "new resource already exists or deleted resource not found", nil)},
		{"unclassified failure", withAudit, noteBody, "12345", fail(errors.New("private failure")), http.StatusInternalServerError, expected(audit.ErrInternalServerError, "internal server error", nil)},
		{"public details", withAudit, noteBody, "12345", fail(detailed), http.StatusBadRequest, expected(audit.ErrCodeOwnerRequired, "owner is required", publicDetails)},
		{"wrapped public details", withAudit, noteBody, "12345", fail(fmt.Errorf("outer: %w", detailed)), http.StatusBadRequest, expected(audit.ErrCodeOwnerRequired, "owner is required", publicDetails)},
		{"wrapped sentinel", withAudit, noteBody, "12345", fail(fmt.Errorf("outer: %w", fga.ErrOwnerRequired)), http.StatusBadRequest, expected(audit.ErrCodeOwnerRequired, "outer: owner is required", nil)},
		{"unknown cause with public details", withAudit, noteBody, "12345", fail(&fga.DetailedError{Err: errors.New("private failure"), Message: "details"}), http.StatusInternalServerError, expected(audit.ErrInternalServerError, "internal server error", &response.DetailedError{Message: "details"})},
		{"space privacy denies deletion", withAudit, spaceBody, "12345", expectResourceUpdate(space, model.UpdateResourceResponse{}, fga.ErrSpacePrivacy), http.StatusBadRequest, expectedResourceFailure(space, model.Meta{}, audit.ErrCodeValidationFailed, "invalid space privacy operation", nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := mocks.NewMockresourceService(gomock.NewController(t))
			tc.expect(svc)
			h := ResourceHandler{resourceService: svc}

			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v0/resources/update", strings.NewReader(tc.body))
			r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			r.Header.Set(userIDHeader, tc.telegramID)

			rec := httptest.NewRecorder()
			c := echo.New().NewContext(r, rec)

			err := tc.startAudit(testAuditMiddleware(t), h.UpdateResource)(c)
			require.NoError(t, err)

			require.Equal(t, tc.status, rec.Code)
			wantJSON, err := json.Marshal(tc.reply)
			require.NoError(t, err)

			require.JSONEq(t, string(wantJSON), rec.Body.String())
		})
	}
}

func expectResourceUpdate(req model.UpdateResourceRequest, reply model.UpdateResourceResponse, cause error) func(*mocks.MockresourceService) {
	return func(svc *mocks.MockresourceService) {
		svc.EXPECT().UpdateResource(gomock.Any(), req).Return(reply, cause)
	}
}

func expectedResourceFailure(req model.UpdateResourceRequest, meta model.Meta, code audit.ErrorCode, message string, details *response.DetailedError) response.ResourceBody {
	return response.ResourceBody{
		RequestID: req.RequestID, Status: model.StatusError, Result: model.ResultFailed,
		Resource:     response.Resource{ID: req.Resource.ID, Type: req.Resource.Type},
		Meta:         response.Meta{AuthModelID: meta.AuthModelID},
		ErrorMessage: response.Message{Code: code, Message: message, Details: response.Details{Operation: req.Operation, DetailedError: details}},
	}
}

func TestDecodeResourceUpdate(t *testing.T) {
	t.Parallel()

	id := uuid.New()

	wantError := func(message string) require.ErrorAssertionFunc {
		return func(t require.TestingT, err error, _ ...any) {
			require.ErrorContains(t, err, message)
		}
	}

	for _, tt := range []struct {
		name, header, body string
		want               model.UpdateResourceRequest
		wantErr            require.ErrorAssertionFunc
	}{
		{"missing header", "", "", model.UpdateResourceRequest{}, wantError("is required")},
		{"nonnumeric header", "x", "{}", model.UpdateResourceRequest{}, wantError("invalid telegram id")},
		{"decimal header", "123.45", "{}", model.UpdateResourceRequest{}, wantError("invalid telegram id")},
		{"invalid JSON", "42", "{", model.UpdateResourceRequest{TelegramID: 42}, wantError("cannot bind request")},
		{"valid", "42", fmt.Sprintf(`{"request_id":%q}`, id), model.UpdateResourceRequest{TelegramID: 42, RequestID: id}, require.NoError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), "POST", "/", strings.NewReader(tt.body))
			r.Header.Set(userIDHeader, tt.header)
			r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

			var req model.UpdateResourceRequest

			err := decodeResourceUpdate(echo.New().NewContext(r, httptest.NewRecorder()), &req)
			tt.wantErr(t, err)

			require.Equal(t, tt.want, req)
		})
	}
}
