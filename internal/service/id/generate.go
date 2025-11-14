package id

import (
	"crypto/rand"
	"errors"
	"math/big"
)

// Generate генерирует случайный идентификатор длиной n символов.
func Generate(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("length must be greater than 0")
	}

	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}

		b[i] = letters[idx.Int64()]
	}

	return string(b), nil
}
