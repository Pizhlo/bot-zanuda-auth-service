package response

// Result содержит готовое типизированное тело ответа и HTTP-статус.
type Result[T any] struct {
	StatusCode int
	Body       T
}
