package response

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

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

	ctx := ResourceContext{
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
		want Result[ResourceBody]
	}{
		{
			name: "plain",
			err:  fga.ErrOwnerRequired,
			want: Result[ResourceBody]{
				StatusCode: http.StatusBadRequest,
				Body: ResourceBody{
					RequestID: ctx.RequestID,
					Status:    model.StatusError,
					Result:    model.ResultFailed,
					Resource: Resource{
						ID:   ctx.Resource.ID,
						Type: model.ResourceTypeNote,
					},
					Meta: Meta{
						AuthModelID: "model-id",
					},
					ErrorMessage: Message{
						Code:    audit.ErrCodeOwnerRequired,
						Message: "owner is required",
						Details: Details{
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
			want: Result[ResourceBody]{
				StatusCode: http.StatusBadRequest,
				Body: ResourceBody{
					RequestID: ctx.RequestID,
					Status:    model.StatusError,
					Result:    model.ResultFailed,
					Resource: Resource{
						ID:   ctx.Resource.ID,
						Type: model.ResourceTypeNote,
					},
					Meta: Meta{
						AuthModelID: "model-id",
					},
					ErrorMessage: Message{
						Code:    audit.ErrCodeOwnerRequired,
						Message: "owner is required",
						Details: Details{
							Operation: model.OperationCreate,
							DetailedError: &DetailedError{
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
			want: Result[ResourceBody]{
				StatusCode: http.StatusInternalServerError,
				Body: ResourceBody{
					RequestID: ctx.RequestID,
					Status:    model.StatusError,
					Result:    model.ResultFailed,
					Resource: Resource{
						ID:   ctx.Resource.ID,
						Type: model.ResourceTypeNote,
					},
					Meta: Meta{
						AuthModelID: "model-id",
					},
					ErrorMessage: Message{
						Code:    audit.ErrInternalServerError,
						Message: "internal server error",
						Details: Details{
							Operation: model.OperationCreate,
						},
					},
				},
			},
		},
		{
			name: "nil",
			err:  nil,
			want: Result[ResourceBody]{
				StatusCode: http.StatusOK,
				Body: ResourceBody{
					RequestID: ctx.RequestID,
					Status:    model.StatusError,
					Result:    model.ResultFailed,
					Resource: Resource{
						ID:   ctx.Resource.ID,
						Type: model.ResourceTypeNote,
					},
					Meta: Meta{
						AuthModelID: "model-id",
					},
					ErrorMessage: Message{
						Details: Details{
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

			got := ResourceError(tt.err, ctx)
			require.Equal(t, tt.want, got)
		})
	}
}
