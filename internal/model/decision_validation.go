package model

// DecisionValidationError хранит только безопасное публичное объяснение ошибки входа.
// Обёртки этапов и исходные значения запроса не входят в это сообщение.
type DecisionValidationError struct {
	message string
}

// InvalidDecisionRequest создаёт ошибку с заранее подготовленным сообщением о поле
// и нарушенном ограничении. Нельзя передавать сюда значения запроса или текст ошибки зависимости.
func InvalidDecisionRequest(message string) error {
	return &DecisionValidationError{message: message}
}

// Error возвращает публичное сообщение валидации без внутренних обёрток.
func (e *DecisionValidationError) Error() string { return e.message }

// Unwrap сохраняет классификацию errors.Is с прежней категорией invalid_request.
func (e *DecisionValidationError) Unwrap() error { return ErrInvalidDecisionRequest }
