package main

import (
	"encoding/json"

	"crossfuzz/harness/gofuzz"
)

func target(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return []byte("error"), nil
	}
	switch v.(type) {
	case map[string]any:
		return []byte("object"), nil
	case []any:
		return []byte("array"), nil
	case string:
		return []byte("string"), nil
	case float64:
		return []byte("number"), nil
	case bool:
		if v.(bool) {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case nil:
		return []byte("null"), nil
	default:
		return []byte("error"), nil
	}
}

func main() {
	gofuzz.Run(target)
}
