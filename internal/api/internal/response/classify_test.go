package response

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"auth-service/internal/service/fga"
	"auth-service/pkg/audit"

	"github.com/stretchr/testify/require"
)

//nolint:funlen // Табличные случаи развёрнуты по полям для читаемости.
func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want classification
	}{
		{
			name: "ErrResounceAlreadyExistsOrNotFound",
			err:  fga.ErrResounceAlreadyExistsOrNotFound,
			want: classification{
				StatusCode: http.StatusConflict,
				Code:       audit.ErrCodeResourceAlreadyExistsOrNotFound,
				Message:    fga.ErrResounceAlreadyExistsOrNotFound.Error(),
			},
		},
		{
			name: "ErrNoTuplesToWriteOrDelete",
			err:  fga.ErrNoTuplesToWriteOrDelete,
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrNoTuplesToWriteOrDelete,
				Message:    fga.ErrNoTuplesToWriteOrDelete.Error(),
			},
		},
		{
			name: "ErrUserNotFound",
			err:  fga.ErrUserNotFound,
			want: classification{
				StatusCode: http.StatusNotFound,
				Code:       audit.ErrCodeUserNotFound,
				Message:    fga.ErrUserNotFound.Error(),
			},
		},
		{
			name: "ErrOwnerRequired",
			err:  fga.ErrOwnerRequired,
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeOwnerRequired,
				Message:    fga.ErrOwnerRequired.Error(),
			},
		},
		{
			name: "ErrParentRequired",
			err:  fga.ErrParentRequired,
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeParentRequired,
				Message:    fga.ErrParentRequired.Error(),
			},
		},
		{
			name: "ErrChangeTypeInvalid",
			err:  fga.ErrChangeTypeInvalid,
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeChangeTypeInvalid,
				Message:    fga.ErrChangeTypeInvalid.Error(),
			},
		},
		{
			name: "ErrEventTypeInvalid",
			err:  fga.ErrEventTypeInvalid,
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeEventTypeInvalid,
				Message:    fga.ErrEventTypeInvalid.Error(),
			},
		},
		{
			name: "ErrParentNotAllowed",
			err:  fga.ErrParentNotAllowed,
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeParentNotAllowed,
				Message:    fga.ErrParentNotAllowed.Error(),
			},
		},
		{
			name: "ErrResourceEmpty",
			err:  fga.ErrResourceEmpty,
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeResourceEmpty,
				Message:    fga.ErrResourceEmpty.Error(),
			},
		},
		{
			name: "ErrOwnerTypeInvalid",
			err:  fga.ErrOwnerTypeInvalid,
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeOwnerTypeInvalid,
				Message:    fga.ErrOwnerTypeInvalid.Error(),
			},
		},
		{
			name: "ErrParentTypeInvalid",
			err:  fga.ErrParentTypeInvalid,
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeParentTypeInvalid,
				Message:    fga.ErrParentTypeInvalid.Error(),
			},
		},
		{
			name: "wrapped sentinel",
			err:  fmt.Errorf("context: %w", fga.ErrOwnerRequired),
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeOwnerRequired,
				Message:    "context: " + fga.ErrOwnerRequired.Error(),
			},
		},
		{
			name: "detailed",
			err: &fga.DetailedError{
				Err:     fga.ErrOwnerRequired,
				Message: "details",
				Value:   "owner",
			},
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeOwnerRequired,
				Message:    fga.ErrOwnerRequired.Error(),
			},
		},
		{
			name: "wrapped detailed",
			err: fmt.Errorf("context: %w", &fga.DetailedError{
				Err: fga.ErrOwnerRequired,
			}),
			want: classification{
				StatusCode: http.StatusBadRequest,
				Code:       audit.ErrCodeOwnerRequired,
				Message:    "context: " + fga.ErrOwnerRequired.Error(),
			},
		},
		{
			name: "unknown",
			err:  errors.New("private failure"),
			want: classification{
				StatusCode: http.StatusInternalServerError,
				Code:       audit.ErrInternalServerError,
				Message:    "internal server error",
			},
		},
		{
			name: "nil",
			err:  nil,
			want: classification{
				StatusCode: http.StatusOK,
			},
		},
		{
			name: "multiple matches preserve priority",
			err:  errors.Join(fga.ErrOwnerRequired, fga.ErrUserNotFound),
			want: classification{
				StatusCode: http.StatusNotFound,
				Code:       audit.ErrCodeUserNotFound,
				Message:    errors.Join(fga.ErrOwnerRequired, fga.ErrUserNotFound).Error(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classify(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}
