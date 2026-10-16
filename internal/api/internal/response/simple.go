package response

// SimpleBody сохраняет короткий формат транспортных ошибок до вызова сервиса.
type SimpleBody struct {
	Error string `json:"error"`
}

// SimpleError формирует короткий ответ с явно выбранным транспортным статусом.
func SimpleError(statusCode int, err error) Result[SimpleBody] {
	return Result[SimpleBody]{StatusCode: statusCode, Body: SimpleBody{Error: err.Error()}}
}
