package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/google/uuid"
)

// validateDecisionJSON восстанавливает путь ошибки по существующим DTO, не дублируя
// схему полей. Потоковый проход обнаруживает неизвестные и повторяющиеся имена
// до потери информации о ключах при декодировании в DTO.
func validateDecisionJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := validateDecisionValue(decoder, reflect.TypeFor[CheckDecisionRequest](), "body"); err != nil {
		return fmt.Errorf("validate JSON structure: %w", err)
	}

	return nil
}

func validateDecisionValue(decoder *json.Decoder, target reflect.Type, path string) error {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}

	token, err := decoder.Token()
	if err != nil {
		if path == "body" && errors.Is(err, io.EOF) {
			return InvalidDecisionRequest("body is required and must be a JSON object")
		}

		return decisionJSONSyntaxError(err)
	}

	if token == nil {
		return InvalidDecisionRequest(path + " must not be null")
	}

	return validateDecisionToken(decoder, target, path, token)
}

func validateDecisionToken(decoder *json.Decoder, target reflect.Type, path string, token json.Token) error {
	if target == reflect.TypeFor[uuid.UUID]() {
		value, ok := token.(string)
		if !ok {
			return InvalidDecisionRequest(path + " must be a nonzero UUID")
		}

		if _, err := uuid.Parse(value); err != nil {
			return fmt.Errorf("parse UUID: %w: %w", InvalidDecisionRequest(path+" must be a nonzero UUID"), err)
		}

		return nil
	}

	switch target.Kind() {
	case reflect.Struct:
		if token != json.Delim('{') {
			return InvalidDecisionRequest(path + " must be a JSON object")
		}

		return validateDecisionObject(decoder, target, path)
	case reflect.Slice:
		return validateDecisionArray(decoder, target.Elem(), path, token)
	case reflect.String:
		if _, ok := token.(string); !ok {
			return InvalidDecisionRequest(path + " must be a string")
		}
	case reflect.Bool:
		if _, ok := token.(bool); !ok {
			return InvalidDecisionRequest(path + " must be a boolean")
		}
	default:
		return fmt.Errorf("unsupported decision schema type: %s", target)
	}

	return nil
}

func validateDecisionArray(decoder *json.Decoder, element reflect.Type, path string, token json.Token) error {
	if token != json.Delim('[') {
		return InvalidDecisionRequest(path + " must be an array")
	}

	for i := 0; decoder.More(); i++ {
		if err := validateDecisionValue(decoder, element, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return fmt.Errorf("validate array element: %w", err)
		}
	}

	return closeDecisionJSONValue(decoder)
}

func validateDecisionObject(decoder *json.Decoder, target reflect.Type, path string) error {
	seen := make(map[string]bool, target.NumField())

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return decisionJSONSyntaxError(err)
		}

		name, ok := token.(string)
		if !ok {
			return InvalidDecisionRequest(path + " must contain named fields")
		}

		field, found := decisionJSONField(target, name)
		if !found {
			// Имя неизвестного ключа может содержать секрет; показываем только известный контейнер.
			return InvalidDecisionRequest(path + " contains an unknown field")
		}

		fieldPath := name
		if path != "body" {
			fieldPath = path + "." + name
		}

		if seen[name] {
			return InvalidDecisionRequest(fieldPath + " must appear only once")
		}

		seen[name] = true

		if err := validateDecisionValue(decoder, field.Type, fieldPath); err != nil {
			return fmt.Errorf("validate object field: %w", err)
		}
	}

	return closeDecisionJSONValue(decoder)
}

// decisionJSONField ищет точное имя поля из публичного DTO запроса.
func decisionJSONField(target reflect.Type, name string) (reflect.StructField, bool) {
	for i := 0; i < target.NumField(); i++ {
		field := target.Field(i)

		jsonName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == jsonName {
			return field, true
		}
	}

	return reflect.StructField{}, false
}

// hasUniqueDecisionRequestID проверяет источник уже разобранного UUID при отказе.
// Первое нарушение структуры может встретиться до неоднозначного request_id, поэтому
// для корреляции ошибки проверяем все ключи корневого объекта.
func hasUniqueDecisionRequestID(data []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))

	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return false
	}

	var (
		found bool
		value json.RawMessage
	)

	for decoder.More() {
		token, err := decoder.Token()

		name, ok := token.(string)
		if err != nil || !ok {
			return false
		}

		if strings.EqualFold(name, "request_id") {
			if found || name != "request_id" {
				return false
			}

			found = true
		}

		if err := decoder.Decode(&value); err != nil {
			return false
		}
	}

	_, err = decoder.Token()

	return found && err == nil
}

func closeDecisionJSONValue(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err != nil {
		return decisionJSONSyntaxError(err)
	}

	return nil
}

// decisionJSONSyntaxError не переносит текст ошибки парсера и присланные значения в ответ.
func decisionJSONSyntaxError(err error) error {
	message := "body contains invalid JSON"

	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		message = fmt.Sprintf("body contains invalid JSON at byte %d", syntax.Offset)
	} else if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		message = "body contains incomplete JSON"
	}

	return fmt.Errorf("read JSON token: %w: %w", InvalidDecisionRequest(message), err)
}
