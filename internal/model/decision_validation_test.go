package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvalidDecisionRequest(t *testing.T) {
	t.Parallel()

	var validation *DecisionValidationError

	err := InvalidDecisionRequest("invalid field")
	require.ErrorAs(t, err, &validation)

	require.Equal(t, "invalid field", validation.message)
}

func TestError(t *testing.T) {
	t.Parallel()

	err := &DecisionValidationError{message: "missing field"}
	require.Equal(t, "missing field", err.Error())
}

func TestUnwrap(t *testing.T) {
	t.Parallel()

	err := &DecisionValidationError{}
	require.Equal(t, ErrInvalidDecisionRequest, err.Unwrap())
}
