// Package response формирует ошибочные ответы для HTTP-хендлеров всех версий API.
package response

import (
	"errors"
	"net/http"

	"auth-service/internal/service/fga"
	"auth-service/pkg/audit"
)

type classification struct {
	StatusCode int
	Code       audit.ErrorCode
	Message    string
}

//nolint:cyclop // Явные ветви сохраняют существующий порядок распознавания ошибок.
func classify(err error) classification {
	if err == nil {
		return classification{StatusCode: http.StatusOK}
	}

	switch {
	case errors.Is(err, fga.ErrResounceAlreadyExistsOrNotFound):
		return classification{StatusCode: http.StatusConflict, Code: audit.ErrCodeResourceAlreadyExistsOrNotFound, Message: err.Error()}
	case errors.Is(err, fga.ErrNoTuplesToWriteOrDelete):
		return classification{StatusCode: http.StatusBadRequest, Code: audit.ErrNoTuplesToWriteOrDelete, Message: err.Error()}
	case errors.Is(err, fga.ErrUserNotFound):
		return classification{StatusCode: http.StatusNotFound, Code: audit.ErrCodeUserNotFound, Message: err.Error()}
	case errors.Is(err, fga.ErrOwnerRequired):
		return classification{StatusCode: http.StatusBadRequest, Code: audit.ErrCodeOwnerRequired, Message: err.Error()}
	case errors.Is(err, fga.ErrParentRequired):
		return classification{StatusCode: http.StatusBadRequest, Code: audit.ErrCodeParentRequired, Message: err.Error()}
	case errors.Is(err, fga.ErrChangeTypeInvalid):
		return classification{StatusCode: http.StatusBadRequest, Code: audit.ErrCodeChangeTypeInvalid, Message: err.Error()}
	case errors.Is(err, fga.ErrEventTypeInvalid):
		return classification{StatusCode: http.StatusBadRequest, Code: audit.ErrCodeEventTypeInvalid, Message: err.Error()}
	case errors.Is(err, fga.ErrParentNotAllowed):
		return classification{StatusCode: http.StatusBadRequest, Code: audit.ErrCodeParentNotAllowed, Message: err.Error()}
	case errors.Is(err, fga.ErrResourceEmpty):
		return classification{StatusCode: http.StatusBadRequest, Code: audit.ErrCodeResourceEmpty, Message: err.Error()}
	case errors.Is(err, fga.ErrOwnerTypeInvalid):
		return classification{StatusCode: http.StatusBadRequest, Code: audit.ErrCodeOwnerTypeInvalid, Message: err.Error()}
	case errors.Is(err, fga.ErrParentTypeInvalid):
		return classification{StatusCode: http.StatusBadRequest, Code: audit.ErrCodeParentTypeInvalid, Message: err.Error()}
	default:
		return classification{StatusCode: http.StatusInternalServerError, Code: audit.ErrInternalServerError, Message: "internal server error"}
	}
}
