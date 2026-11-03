package tool

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type strictDecodeContainerShape struct {
	Path     string
	Expected string
	Actual   string
}

// Additional diagnostics only, after the owning strict decoder has rejected a
// native container. Never unwrap an item object, infer values, or change the
// rejection. Reflection follows that exact decoder's JSON-tag tree, not text.
func appendStrictDecodeContainerShapes(res *types.ToolResult, retErr *error, err error, raw []byte, dst any) {
	var mismatch *json.UnmarshalTypeError
	if !errors.As(err, &mismatch) || (mismatch.Value != "object" && mismatch.Value != "array") {
		return // Preserve the separate unknown-field/string-carrier lessons.
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || !json.Valid(raw) {
		return
	}
	const rosterLimit = 12
	var rows []strictDecodeContainerShape
	total, visited := 0, 0
	limited := false
	var walk func(any, reflect.Type, string, int)
	walk = func(value any, target reflect.Type, path string, depth int) {
		if visited >= 16384 || depth > 64 {
			limited = true
			return
		}
		visited++
		if value == nil || target == nil || strictDecodeShapeOpaque(target) {
			return
		}
		for target.Kind() == reflect.Pointer {
			target = target.Elem()
		}
		actual := ""
		switch value.(type) {
		case map[string]any:
			actual = "object"
		case []any:
			actual = "array"
		}
		want := strictDecodeContainerKind(target)
		if actual != "" && actual != want && want != "" {
			if len(rows) < rosterLimit {
				rows = append(rows, strictDecodeContainerShape{path, strictDecodeShapeLabel(target), actual})
			}
			total++
			return // Do not fabricate child ownership inside a wrong carrier.
		}
		switch node := value.(type) {
		case map[string]any:
			if target.Kind() != reflect.Struct && target.Kind() != reflect.Map {
				return
			}
			fields := map[string]reflect.Type{}
			if target.Kind() == reflect.Struct {
				fields = strictDecodeStructJSONFields(target)
			}
			keys := make([]string, 0, len(node))
			for key := range node {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				child, ok := strictDecodeLookupJSONField(fields, key)
				if target.Kind() == reflect.Map {
					child, ok = target.Elem(), true
				}
				if ok {
					walk(node[key], child, strictDecodeShapePath(path, key), depth+1)
				}
			}
		case []any:
			if target.Kind() == reflect.Slice || target.Kind() == reflect.Array {
				for i, child := range node {
					walk(child, target.Elem(), fmt.Sprintf("%s[%d]", path, i), depth+1)
				}
			}
		}
	}
	walk(value, reflect.TypeOf(dst), "$", 0)
	if len(rows) == 0 {
		return
	}
	parts, paths := make([]string, 0, len(rows)), make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, fmt.Sprintf("%s (expected %s; received native JSON %s)", row.Path, row.Expected, row.Actual))
		paths = append(paths, row.Path)
	}
	note := "; container shape mismatches: " + strings.Join(parts, ", ")
	if total > len(rows) {
		note += fmt.Sprintf("; additional shape mismatches omitted=%d", total-len(rows))
	}
	if limited {
		note += "; diagnostic traversal limit reached; this is not the complete mismatch roster"
	}
	note += "; re-emit the listed paths using their native JSON shapes and preserve all intended values, entries and answer facts; no wrapper conversion or content deletion was applied"
	res.Summary += note
	if retErr != nil && *retErr != nil {
		*retErr = fmt.Errorf("%w%s", *retErr, note)
	}
	if res.Repair == nil {
		res.Repair = &types.ToolRepair{Code: "tool_param_container_shape", Fields: paths, Hint: "Use the exact indexed paths and native JSON shapes in the rejection. Preserve all intended values and entries; this feedback does not stage, repair or validate their meaning."}
	}
}

var strictDecodeJSONUnmarshaler = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
var strictDecodeTextUnmarshaler = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
var strictDecodeJSONNumber = reflect.TypeOf(json.Number(""))

func strictDecodeShapeOpaque(t reflect.Type) bool {
	for {
		if t == strictDecodeRawMessageType || t == strictDecodeJSONNumber || t.Kind() == reflect.Interface || t.Implements(strictDecodeJSONUnmarshaler) || t.Implements(strictDecodeTextUnmarshaler) || (t.Kind() != reflect.Pointer && (reflect.PointerTo(t).Implements(strictDecodeJSONUnmarshaler) || reflect.PointerTo(t).Implements(strictDecodeTextUnmarshaler))) {
			return true
		}
		if t.Kind() != reflect.Pointer {
			return false
		}
		t = t.Elem()
	}
}

func strictDecodeContainerKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Struct, reflect.Map:
		return "object"
	case reflect.Slice:
		// encoding/json accepts a base64 string OR a native array for []byte.
		// Its owner remains authoritative; do not invent an object interpretation.
		return "array"
	case reflect.Array:
		return "array"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return "number"
	}
	return ""
}

func strictDecodeShapeLabel(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		item := t.Elem()
		for item.Kind() == reflect.Pointer {
			item = item.Elem()
		}
		kind := "values"
		switch item.Kind() {
		case reflect.String:
			kind = "strings"
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			kind = "integers"
		case reflect.Float32, reflect.Float64:
			kind = "numbers"
		case reflect.Bool:
			kind = "booleans"
		case reflect.Struct, reflect.Map:
			kind = "objects"
		case reflect.Slice, reflect.Array:
			kind = "arrays"
		}
		if item == strictDecodeJSONNumber {
			kind = "numbers"
		}
		return "native JSON array of " + kind
	}
	return "native JSON " + strictDecodeContainerKind(t)
}

func strictDecodeShapePath(parent, key string) string {
	plain := key != ""
	for i, r := range key {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			plain = false
			break
		}
	}
	if !plain {
		encoded, _ := json.Marshal(key)
		return parent + "[" + string(encoded) + "]"
	}
	if parent == "$" {
		return key
	}
	return parent + "." + key
}
