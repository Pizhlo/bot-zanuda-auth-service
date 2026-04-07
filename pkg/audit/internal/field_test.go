package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewField(t *testing.T) {
	t.Parallel()

	fieldID := 1
	value := "test"

	field := NewField(fieldID, value)

	assert.Equal(t, FieldID(fieldID), field.FieldID)
	assert.Equal(t, value, field.Value)
}
