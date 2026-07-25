package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func decodeStrictToolParams(name string, raw json.RawMessage, schema json.RawMessage, dst any, hints []MisplacedFieldHint) (json.RawMessage, *types.ToolResult, error) {
	normalized := applyStructuredPayloadCompat(name, raw, schema)
	dec := json.NewDecoder(strings.NewReader(string(normalized)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		// LT-HYG decoder-remap hint (§29.75 立案, 2026-07-14): the schema is
		// in hand on this entry — fabricated-field rejections teach the real
		// top-level parameter list (reflected, never hand-copied).
		res, retErr := failStrictDecodeWithErrorSchema(name, time.Now(), err, hints, normalized, schema)
		appendStrictDecodeUnknownFieldCensus(&res, &retErr, err, normalized, dst)
		return normalized, &res, retErr
	}
	return normalized, nil, nil
}

func decodeStrictNormalizedToolParams(name string, normalized json.RawMessage, dst any, hints []MisplacedFieldHint) (json.RawMessage, *types.ToolResult, error) {
	dec := json.NewDecoder(strings.NewReader(string(normalized)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		res, retErr := failStrictDecodeWithError(name, time.Now(), err, hints, normalized)
		appendStrictDecodeUnknownFieldCensus(&res, &retErr, err, normalized, dst)
		return normalized, &res, retErr
	}
	return normalized, nil, nil
}

// appendStrictDecodeUnknownFieldCensus — EMITBURN-2 (NG-1, §13.4): Go's
// DisallowUnknownFields aborts on the FIRST unknown key with no path, no
// count, no roster, so a fabricated field living in two containers and
// several array items burned one retry round per occurrence (the no_touying
// fourth-replay 2m51s form). On the unknown-field class, one reject now
// enumerates EVERY unknown key with its JSON path. Report layer only: the
// verdict and the single-field message stay byte-identical (the roster note
// is appended only when there is more than one occurrence to teach).
func appendStrictDecodeUnknownFieldCensus(res *types.ToolResult, retErr *error, err error, normalized json.RawMessage, dst any) {
	if extractUnknownFieldName(err) == "" {
		return
	}
	census := strictDecodeUnknownFieldCensus(normalized, dst)
	if len(census) <= 1 {
		return
	}
	note := fmt.Sprintf("; all unknown fields in this payload (remove every one of them in a single retry): %s",
		strings.Join(census, ", "))
	res.Summary += note
	if *retErr != nil {
		*retErr = fmt.Errorf("%w%s", *retErr, note)
	}
}

// strictDecodeUnknownFieldCensus walks the payload against dst's reflected
// json-tag tree (the schema IS the struct — never hand-copied) and returns
// every unknown key as a JSON path, deterministically ordered. Any parse or
// shape irregularity fails open to nil: the roster is an additive teaching
// note, never a gate.
func strictDecodeUnknownFieldCensus(normalized json.RawMessage, dst any) []string {
	var payload any
	if json.Unmarshal(normalized, &payload) != nil {
		return nil
	}
	var out []string
	strictDecodeCensusWalk(payload, reflect.TypeOf(dst), "", &out)
	const rosterCap = 12
	if len(out) > rosterCap {
		out = append(out[:rosterCap], fmt.Sprintf("(+%d more)", len(out)-rosterCap))
	}
	return out
}

var strictDecodeRawMessageType = reflect.TypeOf(json.RawMessage{})

func strictDecodeCensusWalk(node any, t reflect.Type, path string, out *[]string) {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t == strictDecodeRawMessageType || t.Kind() == reflect.Interface {
		return
	}
	switch value := node.(type) {
	case map[string]any:
		switch t.Kind() {
		case reflect.Struct:
			fields := strictDecodeStructJSONFields(t)
			keys := make([]string, 0, len(value))
			for key := range value {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				childPath := key
				if path != "" {
					childPath = path + "." + key
				}
				fieldType, ok := strictDecodeLookupJSONField(fields, key)
				if !ok {
					*out = append(*out, childPath)
					continue
				}
				strictDecodeCensusWalk(value[key], fieldType, childPath, out)
			}
		case reflect.Map:
			keys := make([]string, 0, len(value))
			for key := range value {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				childPath := key
				if path != "" {
					childPath = path + "." + key
				}
				strictDecodeCensusWalk(value[key], t.Elem(), childPath, out)
			}
		}
	case []any:
		if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
			for i, child := range value {
				strictDecodeCensusWalk(child, t.Elem(), fmt.Sprintf("%s[%d]", path, i), out)
			}
		}
	}
}

// strictDecodeStructJSONFields maps json tag names (embedded structs
// promoted) to field types, mirroring encoding/json's naming rules.
func strictDecodeStructJSONFields(t reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	var walk func(reflect.Type)
	walk = func(structType reflect.Type) {
		for i := 0; i < structType.NumField(); i++ {
			field := structType.Field(i)
			if field.Anonymous {
				embedded := field.Type
				for embedded.Kind() == reflect.Pointer {
					embedded = embedded.Elem()
				}
				if embedded.Kind() == reflect.Struct {
					walk(embedded)
					continue
				}
			}
			if !field.IsExported() {
				continue
			}
			tag := strings.Split(field.Tag.Get("json"), ",")[0]
			if tag == "-" {
				continue
			}
			if tag == "" {
				tag = field.Name
			}
			if _, exists := fields[tag]; !exists {
				fields[tag] = field.Type
			}
		}
	}
	walk(t)
	return fields
}

// strictDecodeLookupJSONField mirrors encoding/json's exact-then-fold match.
func strictDecodeLookupJSONField(fields map[string]reflect.Type, key string) (reflect.Type, bool) {
	if fieldType, ok := fields[key]; ok {
		return fieldType, true
	}
	for tag, fieldType := range fields {
		if strings.EqualFold(tag, key) {
			return fieldType, true
		}
	}
	return nil, false
}
