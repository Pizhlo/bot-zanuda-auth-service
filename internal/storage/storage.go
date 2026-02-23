package storage

import "errors"

// ErrNotFound ошибка о том, что запрос ничего не нашел.
var ErrNotFound = errors.New("not found")
