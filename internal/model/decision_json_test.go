package model

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestValidateDecisionJSON(t *testing.T) {
	t.Parallel()

	invalid := func(message string) require.ErrorAssertionFunc {
		return func(t require.TestingT, err error, _ ...any) {
			require.ErrorIs(t, err, ErrInvalidDecisionRequest)

			var validation *DecisionValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, message, validation.Error())
			require.NotContains(t, validation.Error(), "private-input")
			require.NotContains(t, validation.Error(), "{{target_user_id}}")
		}
	}

	for _, tc := range []struct {
		name, body string
		wantErr    require.ErrorAssertionFunc
	}{
		{"valid object", `{"subject":{"type":"user"}}`, require.NoError},
		{"empty body", "", invalid("body is required and must be a JSON object")},
		{"wrong root", "[]", invalid("body must be a JSON object")},
		{"empty target UUID", `{"resource":{"id":""}}`, invalid("resource.id must be a nonzero UUID")},
		{"invalid subject UUID", `{"subject":{"id":"private-input"}}`, invalid("subject.id must be a nonzero UUID")},
		{"invalid parent UUID", `{"resource":{"attributes":{"space_id":"private-input"}}}`, invalid("resource.attributes.space_id must be a nonzero UUID")},
		{"unresolved Postman UUID", `{"resource":{"id":"{{target_user_id}}"}}`, invalid("resource.id must be a nonzero UUID")},
		{"numeric UUID", `{"resource":{"id":123}}`, invalid("resource.id must be a nonzero UUID")},
		{"null UUID", `{"resource":{"id":null}}`, invalid("resource.id must not be null")},
		{"object UUID", `{"resource":{"id":{}}}`, invalid("resource.id must be a nonzero UUID")},
		{"nonstring name", `{"action":{"name":123}}`, invalid("action.name must be a string")},
		{"nonboolean reasons", `{"options":{"include_reasons":"private-input"}}`, invalid("options.include_reasons must be a boolean")},
		{"null reasons", `{"options":{"include_reasons":null}}`, invalid("options.include_reasons must not be null")},
		{"nonarray mask", `{"action":{"attributes":{"field_mask":"private-input"}}}`, invalid("action.attributes.field_mask must be an array")},
		{"invalid mask element", `{"action":{"attributes":{"field_mask":["text",123]}}}`, invalid("action.attributes.field_mask[1] must be a string")},
		{"null mask element", `{"action":{"attributes":{"field_mask":[null]}}}`, invalid("action.attributes.field_mask[0] must not be null")},
		{"null mask", `{"action":{"attributes":{"field_mask":null}}}`, invalid("action.attributes.field_mask must not be null")},
		{"invalid attributes", `{"resource":{"attributes":[]}}`, invalid("resource.attributes must be a JSON object")},
		{"unknown root field", `{"private-input":true}`, invalid("body contains an unknown field")},
		{"unknown nested field", `{"subject":{"private-input":true}}`, invalid("subject contains an unknown field")},
		{"unknown attribute", `{"action":{"attributes":{"private-input":true}}}`, invalid("action.attributes contains an unknown field")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateDecisionJSON([]byte(tc.body))
			tc.wantErr(t, err)
		})
	}
}

func TestValidateDecisionValue(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, data, path string
		wantErr          require.ErrorAssertionFunc
	}{
		{"pointer value", `"bot"`, "client", require.NoError},
		{"null pointer", "null", "client", wantDecisionError("client must not be null")},
		{"missing body", "", "body", wantDecisionError("body is required")},
		{"incomplete field", "", "client", wantDecisionError("body contains incomplete JSON")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateDecisionValue(json.NewDecoder(strings.NewReader(tc.data)), reflect.TypeFor[**string](), tc.path)
			tc.wantErr(t, err)
		})
	}
}

func TestValidateDecisionToken(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		target  reflect.Type
		data    string
		wantErr require.ErrorAssertionFunc
	}{
		{"boolean", reflect.TypeFor[bool](), "true", require.NoError},
		{"string in place of boolean", reflect.TypeFor[bool](), `"true"`, wantDecisionError("must be a boolean")},
		{"string", reflect.TypeFor[string](), `"text"`, require.NoError},
		{"boolean in place of string", reflect.TypeFor[string](), "true", wantDecisionError("must be a string")},
		{"valid UUID", reflect.TypeFor[uuid.UUID](), `"00000000-0000-4000-8000-000000000001"`, require.NoError},
		{"malformed UUID", reflect.TypeFor[uuid.UUID](), `"invalid"`, wantDecisionError("must be a nonzero UUID")},
		{"nonstring UUID", reflect.TypeFor[uuid.UUID](), "42", wantDecisionError("must be a nonzero UUID")},
		{"object", reflect.TypeFor[DecisionAction](), `{"name":"read"}`, require.NoError},
		{"array in place of object", reflect.TypeFor[DecisionAction](), "[]", wantDecisionError("must be a JSON object")},
		{"slice", reflect.TypeFor[[]string](), `["title"]`, require.NoError},
		{"unsupported schema type", reflect.TypeFor[int](), "1", func(t require.TestingT, err error, _ ...any) {
			require.EqualError(t, err, "unsupported decision schema type: int")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := json.NewDecoder(strings.NewReader(tc.data))
			token, err := d.Token()
			require.NoError(t, err)

			err = validateDecisionToken(d, tc.target, "value", token)
			tc.wantErr(t, err)
		})
	}
}

func TestValidateDecisionArray(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, data string
		wantErr    require.ErrorAssertionFunc
	}{
		{"empty array", "[]", require.NoError},
		{"string array", `["title","text"]`, require.NoError},
		{"wrong container", "{}", wantDecisionError("field_mask must be an array")},
		{"invalid second element", `["title",null]`, wantDecisionError("field_mask[1] must not be null")},
		{"missing closing bracket", `["title"`, wantDecisionError("incomplete JSON")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := json.NewDecoder(strings.NewReader(tc.data))
			token, err := d.Token()
			require.NoError(t, err)

			err = validateDecisionArray(d, reflect.TypeFor[string](), "field_mask", token)
			tc.wantErr(t, err)
		})
	}
}

func TestValidateDecisionObject(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, data string
		wantErr    require.ErrorAssertionFunc
	}{
		{"empty object", "{}", require.NoError},
		{"known fields", `{"include_reasons":true,"consistency":"higher_consistency"}`, require.NoError},
		{"unknown field", `{"secret-key":true}`, func(t require.TestingT, err error, _ ...any) {
			require.ErrorIs(t, err, ErrInvalidDecisionRequest)
			require.ErrorContains(t, err, "options contains an unknown field")
			require.NotContains(t, err.Error(), "secret-key")
		}},
		{"wrong value type", `{"include_reasons":"true"}`, wantDecisionError("options.include_reasons must be a boolean")},
		{"missing closing brace", `{"include_reasons":true`, wantDecisionError("incomplete JSON")},
		{"same canonical field twice", `{"include_reasons":true,"include_reasons":true}`, wantDecisionError("options.include_reasons must appear only once")},
		{"case collision", `{"consistency":"x","CONSISTENCY":"x"}`, wantDecisionError("options contains an unknown field")},
		{"invalid field name syntax", `{!:true}`, wantDecisionError("invalid JSON")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := json.NewDecoder(strings.NewReader(tc.data))
			_, err := d.Token()
			require.NoError(t, err)

			err = validateDecisionObject(d, reflect.TypeFor[DecisionOptions](), "options")
			tc.wantErr(t, err)
		})
	}
}

func TestDecisionJSONField(t *testing.T) {
	t.Parallel()

	target := reflect.TypeFor[DecisionOptions]()

	for _, tc := range []struct {
		name, field string
		found       bool
	}{
		{"include_reasons", "IncludeReasons", true},
		{"INCLUDE_REASONS", "", false},
		{"consistency", "Consistency", true},
		{"unknown", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			field, ok := decisionJSONField(target, tc.name)

			require.Equal(t, tc.found, ok)
			require.Equal(t, tc.field, field.Name)
		})
	}
}

func TestHasUniqueDecisionRequestID(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"one canonical key", `{"request_id":"id"}`, true},
		{"nested keys ignored", `{"subject":{"request_id":"other"},"request_id":"id"}`, true},
		{"missing", `{"subject":{}}`, false},
		{"case variant", `{"Request_ID":"id"}`, false},
		{"identical keys", `{"request_id":"id","request_id":"id"}`, false},
		{"escaped spelling", `{"request_id":"id","request_\u0069d":"id"}`, false},
		{"case collision", `{"request_id":"id","REQUEST_ID":"other"}`, false},
		{"wrong root", `[]`, false},
		{"incomplete value", `{"request_id":`, false},
		{"missing closing brace", `{"request_id":"id"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := hasUniqueDecisionRequestID([]byte(tc.body))

			require.Equal(t, tc.want, got)
		})
	}
}

func TestCloseDecisionJSONValue(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		data    string
		wantErr error
	}{{"{}", nil}, {"{", ErrInvalidDecisionRequest}} {
		t.Run(tc.data, func(t *testing.T) {
			t.Parallel()

			d := json.NewDecoder(strings.NewReader(tc.data))
			_, err := d.Token()
			require.NoError(t, err)

			err = closeDecisionJSONValue(d)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestDecisionJSONSyntaxError(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		cause   error
		message string
	}{
		{"unexpected end", io.ErrUnexpectedEOF, "incomplete JSON"},
		{"end of file", io.EOF, "incomplete JSON"},
		{"syntax offset", &json.SyntaxError{Offset: 7}, "at byte 7"},
		{"other reader failure", io.ErrClosedPipe, "body contains invalid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := decisionJSONSyntaxError(tc.cause)
			require.ErrorIs(t, err, tc.cause)
			require.ErrorIs(t, err, ErrInvalidDecisionRequest)
			require.ErrorContains(t, err, tc.message)
		})
	}
}
