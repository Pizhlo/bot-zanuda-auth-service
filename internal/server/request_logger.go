package server

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// requestLogger сохраняет JSON-поля прежнего Echo Logger и обработку HTTP-ошибок.
func requestLogger() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		HandleError: true,
		LogLatency:  true, LogRemoteIP: true, LogHost: true, LogMethod: true,
		LogURI: true, LogRequestID: true, LogUserAgent: true, LogStatus: true,
		LogError: true, LogContentLength: true, LogResponseSize: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			errorText := ""
			if v.Error != nil {
				errorText = v.Error.Error()
			}

			bytesIn, _ := strconv.ParseInt(v.ContentLength, 10, 64)

			return json.NewEncoder(c.Logger().Output()).Encode(map[string]any{
				"time": time.Now().Format(time.RFC3339Nano),
				"id":   v.RequestID, "remote_ip": v.RemoteIP, "host": v.Host,
				"method": v.Method, "uri": v.URI, "user_agent": v.UserAgent,
				"status": v.Status, "error": errorText,
				"latency": v.Latency.Nanoseconds(), "latency_human": v.Latency.String(),
				"bytes_in": bytesIn, "bytes_out": v.ResponseSize,
			})
		},
	})
}
