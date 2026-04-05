package internal

// FieldID - идентификатор поля.
type FieldID int

// Field - поле аудита.
type Field struct {
	FieldID FieldID
	Value   any
}

// NewField создает новое поле.
func NewField[T ~int](fieldID T, value any) Field {
	return Field{
		FieldID: FieldID(fieldID),
		Value:   value,
	}
}
