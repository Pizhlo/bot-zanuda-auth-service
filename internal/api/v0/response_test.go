package v0

import (
	"auth-service/internal/service/fga"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

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
