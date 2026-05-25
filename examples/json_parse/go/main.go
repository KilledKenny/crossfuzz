package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"crossfuzz/harness/go"
)

func exerciseCoverage() int {
	s := strings.Repeat("ab", 32)
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == 'a' {
			n++
		} else {
			n--
		}
	}
	return n
}

var FUZZ_DEBUG = os.Getenv("FUZZ_DEBUG") != ""
var i = 0

func target(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		if FUZZ_DEBUG {
			log.Printf("unknown: %#v", err)
		}
		if err2, ok := err.(*json.UnmarshalTypeError); ok {
			if FUZZ_DEBUG {
				log.Println(err2.Field)
				log.Println(err2.Type)
			}
			switch err2.Type.Kind() {
			case reflect.Map:
				return []byte("object"), nil
			case reflect.Array:
				return []byte("array"), nil
			case reflect.String:
				return []byte("string"), nil
			case reflect.Float64:
				return []byte("number"), nil
			case reflect.Bool:
				//Random guess
				return []byte("true"), nil
			case reflect.Ptr:
				return []byte("null"), nil
			default:
				return []byte(fmt.Sprintf("unknown: %#v", err2.Type)), nil
			}
		}
		return []byte("error"), nil
		//return []byte(fmt.Sprintf("error: %s", err)), nil
	}
	switch v := v.(type) {
	case map[string]any:
		return []byte("object"), nil
	case []any:
		return []byte("array"), nil
	case string:
		return []byte("string"), nil
	case float64:
		return []byte("number"), nil
	case bool:
		if v {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case nil:
		return []byte("null"), nil
	default:
		return []byte(fmt.Sprintf("unknown: %t", v)), nil
	}
}

func main() {
	crossfuzz.Fuzz(target)
}
