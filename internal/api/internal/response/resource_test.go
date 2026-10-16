package response_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"auth-service/internal/api/internal/response"
	"auth-service/internal/model"
	"auth-service/internal/service/fga"
	"auth-service/pkg/audit"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // Табличные случаи развёрнуты по полям для читаемости.
func TestResourceError(t *testing.T) {
	t.Parallel()

	reqID := uuid.New()
	resourceID := uuid.New()

	ctx := response.ResourceContext{
		RequestID: reqID,
		Resource: model.Resource{
			ID:   resourceID,
			Type: model.ResourceTypeNote,
		},
		Operation: model.OperationCreate,
		Meta: model.Meta{
			AuthModelID: "model-id",
		},
	}

	tests := []struct {
		name string
		err  error
		want response.Result[response.ResourceBody]
	}{
		{
			name: "plain",
			err:  fga.ErrOwnerRequired,
			want: response.Result[response.ResourceBody]{
				StatusCode: http.StatusBadRequest,
				Body: response.ResourceBody{
					RequestID: ctx.RequestID,
					Status:    model.StatusError,
					Result:    model.ResultFailed,
					Resource: response.Resource{
						ID:   ctx.Resource.ID,
						Type: model.ResourceTypeNote,
					},
					Meta: response.Meta{
						AuthModelID: "model-id",
					},
					ErrorMessage: response.Message{
						Code:    audit.ErrCodeOwnerRequired,
						Message: "owner is required",
						Details: response.Details{
							Operation: model.OperationCreate,
						},
					},
				},
			},
		},
		{
			name: "wrapped detailed",
			err: fmt.Errorf("outer: %w", &fga.DetailedError{
				Err:     fga.ErrOwnerRequired,
				Message: "missing",
				Value:   "owner",
			}),
			want: response.Result[response.ResourceBody]{
				StatusCode: http.StatusBadRequest,
				Body: response.ResourceBody{
					RequestID: ctx.RequestID,
					Status:    model.StatusError,
					Result:    model.ResultFailed,
					Resource: response.Resource{
						ID:   ctx.Resource.ID,
						Type: model.ResourceTypeNote,
					},
					Meta: response.Meta{
						AuthModelID: "model-id",
					},
					ErrorMessage: response.Message{
						Code:    audit.ErrCodeOwnerRequired,
						Message: "owner is required",
						Details: response.Details{
							Operation: model.OperationCreate,
							DetailedError: &response.DetailedError{
								Message: "missing",
								Value:   "owner",
							},
						},
					},
				},
			},
		},
		{
			name: "unknown",
			err:  errors.New("private failure"),
			want: response.Result[response.ResourceBody]{
				StatusCode: http.StatusInternalServerError,
				Body: response.ResourceBody{
					RequestID: ctx.RequestID,
					Status:    model.StatusError,
					Result:    model.ResultFailed,
					Resource: response.Resource{
						ID:   ctx.Resource.ID,
						Type: model.ResourceTypeNote,
					},
					Meta: response.Meta{
						AuthModelID: "model-id",
					},
					ErrorMessage: response.Message{
						Code:    audit.ErrInternalServerError,
						Message: "internal server error",
						Details: response.Details{
							Operation: model.OperationCreate,
						},
					},
				},
			},
		},
		{
			name: "nil",
			err:  nil,
			want: response.Result[response.ResourceBody]{
				StatusCode: http.StatusOK,
				Body: response.ResourceBody{
					RequestID: ctx.RequestID,
					Status:    model.StatusError,
					Result:    model.ResultFailed,
					Resource: response.Resource{
						ID:   ctx.Resource.ID,
						Type: model.ResourceTypeNote,
					},
					Meta: response.Meta{
						AuthModelID: "model-id",
					},
					ErrorMessage: response.Message{
						Details: response.Details{
							Operation: model.OperationCreate,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := response.ResourceError(tt.err, ctx)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResourceBodyPreservesEmptyFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body any
		want string
	}{
		{
			name: "zero resource body",
			body: response.ResourceBody{},
			want: `{"request_id":"00000000-0000-0000-0000-000000000000","status":"","result":"","resource":{"id":"00000000-0000-0000-0000-000000000000","type":""},"error":{"code":"","message":"","details":{"operation":"","detailed_error":null}},"meta":{"auth_model_id":""}}`,
		},
		{
			name: "unknown error without metadata",
			body: response.ResourceError(errors.New("private"), response.ResourceContext{}).Body,
			want: `{"request_id":"00000000-0000-0000-0000-000000000000","status":"error","result":"failed","resource":{"id":"00000000-0000-0000-0000-000000000000","type":""},"error":{"code":"INTERNAL_SERVER_ERROR","message":"internal server error","details":{"operation":"","detailed_error":null}},"meta":{"auth_model_id":""}}`,
		},
		{
			name: "partial resource and empty details",
			body: response.ResourceError(&fga.DetailedError{Err: fga.ErrOwnerRequired}, response.ResourceContext{
				Resource: model.Resource{Type: model.ResourceTypeNote},
			}).Body,
			want: `{"request_id":"00000000-0000-0000-0000-000000000000","status":"error","result":"failed","resource":{"id":"00000000-0000-0000-0000-000000000000","type":"note"},"error":{"code":"OWNER_REQUIRED","message":"owner is required","details":{"operation":"","detailed_error":{"message":"","value":null}}},"meta":{"auth_model_id":""}}`,
		},
		{
			name: "details with message only",
			body: response.ResourceError(&fga.DetailedError{Err: fga.ErrOwnerRequired, Message: "missing"}, response.ResourceContext{}).Body,
			want: `{"request_id":"00000000-0000-0000-0000-000000000000","status":"error","result":"failed","resource":{"id":"00000000-0000-0000-0000-000000000000","type":""},"error":{"code":"OWNER_REQUIRED","message":"owner is required","details":{"operation":"","detailed_error":{"message":"missing","value":null}}},"meta":{"auth_model_id":""}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body, err := json.Marshal(tt.body)
			require.NoError(t, err)
			require.JSONEq(t, tt.want, string(body))
		})
	}
}
