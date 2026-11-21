package v0

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// rejectedRequestID извлекает ID из тела для аудита отказа до обработки запроса.
// При отсутствии корректного ID вызывающий код назначает корреляцию на сервере.
func rejectedRequestID(c echo.Context) uuid.UUID {
	r := c.Request()
	if r.Body == nil || r.Body == http.NoBody {
		return uuid.Nil
	}

	var envelope struct {
		RequestID uuid.UUID `json:"request_id"`
	}

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&envelope); err != nil {
		return uuid.Nil
	}

	if _, err := decoder.Token(); err != io.EOF {
		return uuid.Nil
	}

	return envelope.RequestID
}
