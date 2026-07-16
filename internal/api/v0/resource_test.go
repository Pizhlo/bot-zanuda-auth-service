//nolint:funlen,dupl // тесты
package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

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
				}
			},
			checkWant: func(t *testing.T, handler *ResourceHandler) {
				t.Helper()

				require.NotNil(t, handler)
				require.NotNil(t, handler.resourceService)
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

func TestResourceHandler_UpdateResource(t *testing.T) {
	t.Parallel()

	type wantResponse struct {
		status  int
		resp    *model.UpdateResourceResponse
		errResp *errorResponse
	}

	requestID := uuid.New()
	decisionID := uuid.New()
	noteID := uuid.New()
	spaceID := uuid.New()
	ownerID := uuid.New()

	updateRequest := model.UpdateResourceRequest{
		RequestID:      requestID,
		IdempotencyKey: "idempotency-key",
		TelegramID:     12345,
		DecisionID:     decisionID,
		Resource:       model.Resource{ID: noteID, Type: model.ResourceTypeNote},
		Operation:      model.OperationCreate,
		ChangeType:     model.ChangeTypeResourceAdded,
		Relations: model.Relation{
			Owner:  model.Resource{ID: ownerID, Type: model.ResourceTypeUser},
			Parent: model.Resource{ID: spaceID, Type: model.ResourceTypeSpace},
		},
		Context: model.Context{
			SourceService: "notes-service",
			EventType:     "note.created",
			TraceID:       "trace-id",
		},
	}

	updateResponse := model.UpdateResourceResponse{
		RequestID:      requestID,
		IdempotencyKey: "idempotency-key",
		Status:         model.StatusCompleted,
		Result:         model.ResultApplied,
		Resource:       updateRequest.Resource,
		Meta: model.Meta{
			AuthModelID: "model-id",
		},
	}

	tests := []struct {
		name       string
		body       model.UpdateResourceRequest
		setupMocks func(t *testing.T, resourceSvc *mocks.MockresourceService)
		checkWant  func(t *testing.T, actual wantResponse)
	}{
		{
			name: "success",
			body: updateRequest,
			setupMocks: func(t *testing.T, resourceSvc *mocks.MockresourceService) {
				t.Helper()

				resourceSvc.EXPECT().
					UpdateResource(gomock.Any(), updateRequest).
					Return(updateResponse, nil)
			},
			checkWant: func(t *testing.T, actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusOK, actual.status)
				require.Equal(t, &updateResponse, actual.resp)
			},
		},
		{
			name: "error case: no tuples to write or delete",
			body: updateRequest,
			setupMocks: func(t *testing.T, resourceSvc *mocks.MockresourceService) {
				t.Helper()

				resourceSvc.EXPECT().
					UpdateResource(gomock.Any(), gomock.Any()).
					Return(model.UpdateResourceResponse{}, fga.ErrNoTuplesToWriteOrDelete)
			},
			checkWant: func(t *testing.T, actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusBadRequest, actual.status)
				require.Equal(t, &errorResponse{
					RequestID:    requestID,
					Status:       model.StatusError,
					Result:       model.ResultFailed,
					Resource:     updateRequest.Resource,
					ErrorMessage: newMessage(audit.ErrNoTuplesToWriteOrDelete, fga.ErrNoTuplesToWriteOrDelete.Error(), updateRequest.Operation, nil),
					Meta:         model.Meta{},
				}, actual.errResp)
			},
		},
		{
			name: "error case: resource already exists or not found",
			body: updateRequest,
			setupMocks: func(t *testing.T, resourceSvc *mocks.MockresourceService) {
				t.Helper()

				resourceSvc.EXPECT().
					UpdateResource(gomock.Any(), gomock.Any()).
					Return(model.UpdateResourceResponse{}, fga.ErrResounceAlreadyExistsOrNotFound)
			},
			checkWant: func(t *testing.T, actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusConflict, actual.status)
				require.Equal(t, &errorResponse{
					RequestID:    requestID,
					Status:       model.StatusError,
					Result:       model.ResultFailed,
					Resource:     updateRequest.Resource,
					ErrorMessage: newMessage(audit.ErrCodeResourceAlreadyExistsOrNotFound, fga.ErrResounceAlreadyExistsOrNotFound.Error(), updateRequest.Operation, nil),
					Meta:         model.Meta{},
				}, actual.errResp)
			},
		},
		{
			name: "error case: user not found",
			body: updateRequest,
			setupMocks: func(t *testing.T, resourceSvc *mocks.MockresourceService) {
				t.Helper()

				resourceSvc.EXPECT().
					UpdateResource(gomock.Any(), gomock.Any()).
					Return(model.UpdateResourceResponse{}, fga.ErrUserNotFound)
			},
			checkWant: func(t *testing.T, actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusNotFound, actual.status)
				require.Equal(t, &errorResponse{
					RequestID:    requestID,
					Status:       model.StatusError,
					Result:       model.ResultFailed,
					Resource:     updateRequest.Resource,
					ErrorMessage: newMessage(audit.ErrCodeUserNotFound, fga.ErrUserNotFound.Error(), updateRequest.Operation, nil),
					Meta:         model.Meta{},
				}, actual.errResp)
			},
		},
		{
			name: "error case: detailed error",
			body: updateRequest,
			setupMocks: func(t *testing.T, resourceSvc *mocks.MockresourceService) {
				t.Helper()

				resourceSvc.EXPECT().
					UpdateResource(gomock.Any(), gomock.Any()).
					Return(model.UpdateResourceResponse{}, &fga.DetailedError{
						Err:     fga.ErrOwnerRequired,
						Message: "owner is required",
						Value:   "empty owner",
					})
			},
			checkWant: func(t *testing.T, actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusBadRequest, actual.status)
				// Err не сериализуется в JSON (json:"-"), поэтому в ответе он nil.
				require.Equal(t, &errorResponse{
					RequestID: requestID,
					Status:    model.StatusError,
					Result:    model.ResultFailed,
					Resource:  updateRequest.Resource,
					ErrorMessage: newMessage(
						audit.ErrCodeOwnerRequired,
						fga.ErrOwnerRequired.Error(),
						updateRequest.Operation,
						&fga.DetailedError{
							Message: "owner is required",
							Value:   "empty owner",
						},
					),
					Meta: model.Meta{},
				}, actual.errResp)
			},
		},
		{
			name: "error case: internal server error",
			body: updateRequest,
			setupMocks: func(t *testing.T, resourceSvc *mocks.MockresourceService) {
				t.Helper()

				resourceSvc.EXPECT().
					UpdateResource(gomock.Any(), gomock.Any()).
					Return(model.UpdateResourceResponse{}, errors.New("write failed"))
			},
			checkWant: func(t *testing.T, actual wantResponse) {
				t.Helper()

				require.Equal(t, http.StatusInternalServerError, actual.status)
				require.Equal(t, &errorResponse{
					RequestID:    requestID,
					Status:       model.StatusError,
					Result:       model.ResultFailed,
					Resource:     updateRequest.Resource,
					ErrorMessage: newMessage(audit.ErrInternalServerError, "internal server error", updateRequest.Operation, nil),
					Meta:         model.Meta{},
				}, actual.errResp)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			resourceSvc := mocks.NewMockresourceService(ctrl)
			tt.setupMocks(t, resourceSvc)

			handler, err := NewResourceHandler(WithResourceService(resourceSvc))
			require.NoError(t, err)

			e := echo.New()

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/resources", bytes.NewReader(bodyBytes))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req.Header.Set(userIDHeader, strconv.Itoa(tt.body.TelegramID))

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err = handler.UpdateResource(c)
			require.NoError(t, err)

			respBody := rec.Body.Bytes()

			wantResp := wantResponse{
				status: rec.Code,
			}

			if rec.Code == http.StatusOK {
				var resp model.UpdateResourceResponse
				require.NoError(t, json.Unmarshal(respBody, &resp))
				wantResp.resp = &resp
			} else {
				var errResp errorResponse
				require.NoError(t, json.Unmarshal(respBody, &errResp))
				wantResp.errResp = &errResp
			}

			tt.checkWant(t, wantResp)
		})
	}
}

func TestResourceHandler_UpdateResource_InvalidTelegramID(t *testing.T) {
	t.Parallel()

	validBody := `{
		"request_id": "019c0000-0000-7000-8000-000000000001",
		"operation": "create",
		"resource": {"id": "019c0000-0000-7000-8000-000000000002", "type": "note"},
		"change_type": "resource_added"
	}`

	tests := []struct {
		name       string
		setHeader  func(req *http.Request)
		wantStatus int
		wantError  string
	}{
		{
			name: "missing telegram id header",
			setHeader: func(req *http.Request) {
				// заголовок не устанавливаем
			},
			wantStatus: http.StatusBadRequest,
			wantError:  fmt.Sprintf("header '%s' is required", userIDHeader),
		},
		{
			name: "empty telegram id header",
			setHeader: func(req *http.Request) {
				req.Header.Set(userIDHeader, "")
			},
			wantStatus: http.StatusBadRequest,
			wantError:  fmt.Sprintf("header '%s' is required", userIDHeader),
		},
		{
			name: "non-numeric telegram id",
			setHeader: func(req *http.Request) {
				req.Header.Set(userIDHeader, "abc")
			},
			wantStatus: http.StatusBadRequest,
			wantError:  fmt.Sprintf("invalid telegram id in header '%s'", userIDHeader),
		},
		{
			name: "telegram id with decimal",
			setHeader: func(req *http.Request) {
				req.Header.Set(userIDHeader, "123.45")
			},
			wantStatus: http.StatusBadRequest,
			wantError:  fmt.Sprintf("invalid telegram id in header '%s'", userIDHeader),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			resourceSvc := mocks.NewMockresourceService(ctrl)

			handler, err := NewResourceHandler(WithResourceService(resourceSvc))
			require.NoError(t, err)

			e := echo.New()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/resources", bytes.NewBufferString(validBody))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			tt.setHeader(req)

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err = handler.UpdateResource(c)
			require.NoError(t, err)

			var errResp map[string]string

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			require.Equal(t, tt.wantStatus, rec.Code)
			require.Equal(t, tt.wantError, errResp["error"])
		})
	}
}

func TestResourceHandler_UpdateResource_InvalidBody(t *testing.T) {
	t.Parallel()

	e := echo.New()

	body := `
	{
		"request_id": "019c0000-0000-7000-8000-000000000001",
		"operation": "create",
	}
	`

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/resources", bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(userIDHeader, "12345")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	resourceSvc := mocks.NewMockresourceService(ctrl)

	handler, err := NewResourceHandler(WithResourceService(resourceSvc))
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = handler.UpdateResource(c)
	require.NoError(t, err)

	var errResp map[string]string

	respBody := rec.Body.Bytes()

	_ = json.Unmarshal(respBody, &errResp)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "cannot bind request", errResp["error"])
}

func TestResourceHandler_CreateErrorResponse(t *testing.T) {
	t.Parallel()

	requestID := uuid.New()
	decisionID := uuid.New()
	noteID := uuid.New()

	updateRequest := model.UpdateResourceRequest{
		RequestID:      requestID,
		IdempotencyKey: "idempotency-key",
		TelegramID:     12345,
		DecisionID:     decisionID,
		Resource:       model.Resource{ID: noteID, Type: model.ResourceTypeNote},
		Operation:      model.OperationCreate,
		ChangeType:     model.ChangeTypeResourceAdded,
	}

	meta := model.Meta{
		AuthModelID: "model-id",
	}

	expectedErrResp := errorResponse{
		RequestID:    requestID,
		Status:       model.StatusError,
		Result:       model.ResultFailed,
		Resource:     updateRequest.Resource,
		ErrorMessage: newMessage(audit.ErrNoTuplesToWriteOrDelete, fga.ErrNoTuplesToWriteOrDelete.Error(), updateRequest.Operation, nil),
		Meta:         meta,
	}

	handler, err := NewResourceHandler(WithResourceService(mocks.NewMockresourceService(gomock.NewController(t))))
	require.NoError(t, err)

	errResp := handler.createErrorResponse(requestID, model.StatusError, model.ResultFailed, updateRequest.Resource, newMessage(audit.ErrNoTuplesToWriteOrDelete, fga.ErrNoTuplesToWriteOrDelete.Error(), updateRequest.Operation, nil), meta)

	require.Equal(t, expectedErrResp, errResp)
}

func TestResponseFromError(t *testing.T) {
	t.Parallel()

	requestID := uuid.New()
	noteID := uuid.New()
	meta := model.Meta{AuthModelID: "model-id"}

	req := model.UpdateResourceRequest{
		RequestID:  requestID,
		Resource:   model.Resource{ID: noteID, Type: model.ResourceTypeNote},
		Operation:  model.OperationCreate,
		ChangeType: model.ChangeTypeResourceAdded,
	}
	resp := model.UpdateResourceResponse{Meta: meta}

	detailedErr := &fga.DetailedError{
		Err:     fga.ErrOwnerRequired,
		Message: "owner is required",
		Value:   "empty owner",
	}
	// Err не сериализуется в JSON (json:"-").
	detailedErrInResponse := &fga.DetailedError{
		Message: "owner is required",
		Value:   "empty owner",
	}

	tests := []struct {
		name          string
		err           error
		detailedError *fga.DetailedError
		wantStatus    int
		wantErrResp   errorResponse
	}{
		{
			name:       "resource already exists or not found",
			err:        fga.ErrResounceAlreadyExistsOrNotFound,
			wantStatus: http.StatusConflict,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrCodeResourceAlreadyExistsOrNotFound, fga.ErrResounceAlreadyExistsOrNotFound.Error(), req.Operation, nil),
				Meta:         meta,
			},
		},
		{
			name:       "no tuples to write or delete",
			err:        fga.ErrNoTuplesToWriteOrDelete,
			wantStatus: http.StatusBadRequest,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrNoTuplesToWriteOrDelete, fga.ErrNoTuplesToWriteOrDelete.Error(), req.Operation, nil),
				Meta:         meta,
			},
		},
		{
			name:       "user not found",
			err:        fga.ErrUserNotFound,
			wantStatus: http.StatusNotFound,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrCodeUserNotFound, fga.ErrUserNotFound.Error(), req.Operation, nil),
				Meta:         meta,
			},
		},
		{
			name:          "owner required with detailed error",
			err:           fga.ErrOwnerRequired,
			detailedError: detailedErr,
			wantStatus:    http.StatusBadRequest,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrCodeOwnerRequired, fga.ErrOwnerRequired.Error(), req.Operation, detailedErrInResponse),
				Meta:         meta,
			},
		},
		{
			name:       "parent required",
			err:        fga.ErrParentRequired,
			wantStatus: http.StatusBadRequest,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrCodeParentRequired, fga.ErrParentRequired.Error(), req.Operation, nil),
				Meta:         meta,
			},
		},
		{
			name:       "change type invalid",
			err:        fga.ErrChangeTypeInvalid,
			wantStatus: http.StatusBadRequest,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrCodeChangeTypeInvalid, fga.ErrChangeTypeInvalid.Error(), req.Operation, nil),
				Meta:         meta,
			},
		},
		{
			name:       "event type invalid",
			err:        fga.ErrEventTypeInvalid,
			wantStatus: http.StatusBadRequest,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrCodeEventTypeInvalid, fga.ErrEventTypeInvalid.Error(), req.Operation, nil),
				Meta:         meta,
			},
		},
		{
			name:       "resource empty",
			err:        fga.ErrResourceEmpty,
			wantStatus: http.StatusBadRequest,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrCodeResourceEmpty, fga.ErrResourceEmpty.Error(), req.Operation, nil),
				Meta:         meta,
			},
		},
		{
			name:       "owner type invalid",
			err:        fga.ErrOwnerTypeInvalid,
			wantStatus: http.StatusBadRequest,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrCodeOwnerTypeInvalid, fga.ErrOwnerTypeInvalid.Error(), req.Operation, nil),
				Meta:         meta,
			},
		},
		{
			name:       "parent type invalid",
			err:        fga.ErrParentTypeInvalid,
			wantStatus: http.StatusBadRequest,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrCodeParentTypeInvalid, fga.ErrParentTypeInvalid.Error(), req.Operation, nil),
				Meta:         meta,
			},
		},
		{
			name:       "internal server error",
			err:        errors.New("unexpected"),
			wantStatus: http.StatusInternalServerError,
			wantErrResp: errorResponse{
				RequestID:    requestID,
				Status:       model.StatusError,
				Result:       model.ResultFailed,
				Resource:     req.Resource,
				ErrorMessage: newMessage(audit.ErrInternalServerError, "internal server error", req.Operation, nil),
				Meta:         meta,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, err := NewResourceHandler(WithResourceService(mocks.NewMockresourceService(gomock.NewController(t))))
			require.NoError(t, err)

			e := echo.New()
			httpReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/resources", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(httpReq, rec)

			err = handler.responseFromError(c, req, resp, tt.err, tt.detailedError)
			require.NoError(t, err)
			require.Equal(t, tt.wantStatus, rec.Code)

			var got errorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, tt.wantErrResp, got)
		})
	}
}

func TestNewMessage(t *testing.T) {
	t.Parallel()

	message := newMessage(audit.ErrNoTuplesToWriteOrDelete, fga.ErrNoTuplesToWriteOrDelete.Error(), model.OperationCreate, nil)
	require.Equal(t, errorMessage{
		Code:    audit.ErrNoTuplesToWriteOrDelete,
		Message: fga.ErrNoTuplesToWriteOrDelete.Error(),
		Details: struct {
			Operation          model.Operation `json:"operation"`
			*fga.DetailedError `json:"detailed_error"`
		}{Operation: model.OperationCreate, DetailedError: nil},
	}, message)
}
func TestErrResponse(t *testing.T) {
	t.Parallel()

	e := echo.New()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/resources", bytes.NewBufferString(""))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := errResponse(c, http.StatusBadRequest, fga.ErrNoTuplesToWriteOrDelete)
	require.NoError(t, err)

	var errResp map[string]string

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, fga.ErrNoTuplesToWriteOrDelete.Error(), errResp["error"])
}
