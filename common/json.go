package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func UnmarshalJsonStr(data string, v any) error {
	return json.Unmarshal(StringToByteSlice(data), v)
}

func DecodeJson(reader io.Reader, v any) error {
	return json.NewDecoder(reader).Decode(v)
}

func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// FriendlyJSONError rewrites encoding/json decode errors into messages safe to
// return to API callers: the raw errors leak Go struct and type names
// (e.g. `json: cannot unmarshal number -1 into Go struct field
// GeneralOpenAIRequest.max_tokens of type uint`). Non-decode errors pass through.
func FriendlyJSONError(err error) error {
	if err == nil {
		return nil
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Field == "" {
			return fmt.Errorf("invalid request body: unexpected %s", typeErr.Value)
		}
		return fmt.Errorf("invalid value for field %q (got %s)", typeErr.Field, typeErr.Value)
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Errorf("request body is not valid JSON (syntax error at offset %d)", syntaxErr.Offset)
	}
	return err
}

func GetJsonType(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "unknown"
	}
	firstChar := trimmed[0]
	switch firstChar {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// JsonRawMessageToString returns JSON strings as their decoded value and other JSON values as raw text.
func JsonRawMessageToString(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] != '"' {
		return string(trimmed)
	}
	var value string
	if err := Unmarshal(trimmed, &value); err != nil {
		return string(trimmed)
	}
	return value
}
