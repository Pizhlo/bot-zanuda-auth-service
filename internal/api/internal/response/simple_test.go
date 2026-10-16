package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimpleError(t *testing.T) {
	t.Parallel()

	result := SimpleError(http.StatusBadRequest, errors.New("cannot bind request"))
	require.Equal(t, http.StatusBadRequest, result.StatusCode)

	body, err := json.Marshal(result.Body)
	require.NoError(t, err)

	require.JSONEq(t, `{"error":"cannot bind request"}`, string(body))
}

func TestSimpleBodyPreservesEmptyFields(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(SimpleBody{})
	require.NoError(t, err)

	require.JSONEq(t, `{"error":""}`, string(body))
}
