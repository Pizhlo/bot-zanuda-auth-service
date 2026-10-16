package v0

import "github.com/labstack/echo/v4"

func errResponse(c echo.Context, statusCode int, err error) error {
	return c.JSON(statusCode, map[string]string{"error": err.Error()})
}
