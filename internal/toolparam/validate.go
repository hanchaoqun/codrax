package toolparam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SchemaViolation is the first precise structural mismatch found between one
// native tool-argument value and the schema presented for that tool call. It
// intentionally reports JSON paths and typed constraints only; no user/model
// prose participates in validation.
type SchemaViolation struct {
	Path   string
	Reason string
}

func (e *SchemaViolation) Error() string {
	if e == nil {
		return ""
	}
	path := strings.TrimSpace(e.Path)
	if path == "" {
		path = "$"
	}
	return fmt.Sprintf("%s: %s", path, strings.TrimSpace(e.Reason))
}

// Validate checks the executable JSON-Schema subset used by Codrax tool
// contracts: type, enum/const, object properties/required/additionalProperties,
// array items and size, scalar bounds, and allOf/if/then/else. Unknown advisory
// keywords are ignored, so this function cannot invent a stricter contract
// than the schema. It is used after compatibility normalization and before a
// payload receives schema-validated authority.
func Validate(raw json.RawMessage, schema json.RawMessage) error {
	value, err := decodeValidationJSON(raw)
	if err != nil {
		return &SchemaViolation{Path: "$", Reason: "invalid JSON: " + err.Error()}
	}
	if len(bytes.TrimSpace(schema)) == 0 {
		return nil
	}
	return validateSchemaValue(value, schema, "$")
}

// ValidateRepairs re-validates only the schema subtrees changed by Normalize.
// This is the compatibility-boundary contract: a repaired array/object/key or
// enum must satisfy its own schema (including nested items), while unrelated
// legacy/defaulted fields are left to the owning tool's existing validator.
// That avoids turning one mechanical repair into an accidental whole-tool
// schema migration.
func ValidateRepairs(raw json.RawMessage, schema json.RawMessage, report Report) error {
	if !report.Changed() {
		return nil
	}
	value, err := decodeValidationJSON(raw)
	if err != nil {
		return &SchemaViolation{Path: "$", Reason: "invalid JSON after normalization: " + err.Error()}
	}
	strictByPath := make(map[string]bool, len(report.Repairs))
	for _, repair := range report.Repairs {
		path := validationRepairTargetPath(repair)
		if path == "" {
			continue
		}
		strictByPath[path] = strictByPath[path] || validationRepairChangesValueShape(repair)
	}
	for path, strictRoot := range strictByPath {
		subValue, subSchema, ok := validationResolvePath(value, schema, path)
		if !ok {
			// A path outside the schema cannot be granted checked authority by
			// this helper; the owning tool's strict decoder remains the guard.
			continue
		}
		if err := validateRepairedSchemaValue(subValue, subSchema, path, strictRoot); err != nil {
			return err
		}
	}
	return nil
}

func validationRepairChangesValueShape(repair Repair) bool {
	if strings.HasPrefix(repair.From, "key:") || strings.HasPrefix(repair.To, "key:") ||
		strings.HasPrefix(repair.Rule, "property_key_") {
		return false
	}
	return true
}

// validateRepairedSchemaValue enforces the exact repaired carrier and every
// nested enum/const authority it exposes. Descendant type mismatches are left
// to an owning flexible decoder unless that descendant had its own value-shape
// repair; this preserves established compatibility defaults while preventing
// a decoded parent array/object from laundering forbidden typed members.
func validateRepairedSchemaValue(value any, rawSchema json.RawMessage, path string, strictType bool) error {
	trimmed := bytes.TrimSpace(rawSchema)
	if bytes.Equal(trimmed, []byte("true")) {
		return nil
	}
	if bytes.Equal(trimmed, []byte("false")) {
		return &SchemaViolation{Path: path, Reason: "schema forbids this value"}
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &schema); err != nil {
		return nil
	}
	if raw, ok := schema["type"]; ok && !validationTypeMatches(value, raw) {
		if strictType {
			return &SchemaViolation{Path: path, Reason: "repaired value type does not match schema type " + compactSchemaToken(raw)}
		}
		return nil
	}
	if raw, ok := schema["const"]; ok {
		want, err := decodeValidationJSON(raw)
		if err == nil && !reflect.DeepEqual(value, want) {
			return &SchemaViolation{Path: path, Reason: "value does not match const " + compactSchemaToken(raw)}
		}
	}
	if raw, ok := schema["enum"]; ok {
		var members []json.RawMessage
		if err := json.Unmarshal(raw, &members); err == nil && len(members) > 0 {
			matched := false
			for _, member := range members {
				want, err := decodeValidationJSON(member)
				if err == nil && reflect.DeepEqual(value, want) {
					matched = true
					break
				}
			}
			if !matched {
				return &SchemaViolation{Path: path, Reason: "value is not one of the schema enum members"}
			}
		}
	}
	switch typed := value.(type) {
	case map[string]any:
		var properties map[string]json.RawMessage
		if raw, ok := schema["properties"]; ok {
			_ = json.Unmarshal(raw, &properties)
		}
		for name, child := range typed {
			if childSchema, ok := properties[name]; ok {
				if err := validateRepairedSchemaValue(child, childSchema, validationPropertyPath(path, name), false); err != nil {
					return err
				}
			}
		}
	case []any:
		if strictType {
			if min, ok := validationIntKeyword(schema["minItems"]); ok && len(typed) < min {
				return &SchemaViolation{Path: path, Reason: fmt.Sprintf("repaired array has %d items; minimum is %d", len(typed), min)}
			}
			if max, ok := validationIntKeyword(schema["maxItems"]); ok && len(typed) > max {
				return &SchemaViolation{Path: path, Reason: fmt.Sprintf("repaired array has %d items; maximum is %d", len(typed), max)}
			}
		}
		if items, ok := schema["items"]; ok {
			for i, child := range typed {
				if err := validateRepairedSchemaValue(child, items, fmt.Sprintf("%s[%d]", path, i), false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validationRepairTargetPath(repair Repair) string {
	path := strings.TrimSpace(repair.Path)
	if path == "" {
		path = "$"
	}
	if strings.HasPrefix(repair.To, "key:") {
		name := strings.TrimSpace(strings.TrimPrefix(repair.To, "key:"))
		if name != "" {
			if at := strings.LastIndex(path, "."); at >= 0 {
				return path[:at+1] + name
			}
			return "$." + name
		}
	}
	return path
}

type validationPathToken struct {
	property string
	index    int
	isIndex  bool
}

func validationResolvePath(root any, rootSchema json.RawMessage, path string) (any, json.RawMessage, bool) {
	if path == "$" {
		return root, rootSchema, true
	}
	tokens, ok := validationParsePath(path)
	if !ok {
		return nil, nil, false
	}
	value, schema := root, rootSchema
	for _, token := range tokens {
		var schemaMap map[string]json.RawMessage
		if err := json.Unmarshal(schema, &schemaMap); err != nil {
			return nil, nil, false
		}
		if token.isIndex {
			items, exists := schemaMap["items"]
			array, typeOK := value.([]any)
			if !exists || !typeOK || token.index < 0 || token.index >= len(array) {
				return nil, nil, false
			}
			value, schema = array[token.index], items
			continue
		}
		var properties map[string]json.RawMessage
		if raw, exists := schemaMap["properties"]; !exists || json.Unmarshal(raw, &properties) != nil {
			return nil, nil, false
		}
		childSchema, exists := properties[token.property]
		object, typeOK := value.(map[string]any)
		if !exists || !typeOK {
			return nil, nil, false
		}
		child, exists := object[token.property]
		if !exists {
			return nil, nil, false
		}
		value, schema = child, childSchema
	}
	return value, schema, true
}

func validationParsePath(path string) ([]validationPathToken, bool) {
	if !strings.HasPrefix(path, "$") {
		return nil, false
	}
	var out []validationPathToken
	for i := 1; i < len(path); {
		switch path[i] {
		case '.':
			start := i + 1
			i = start
			for i < len(path) && path[i] != '.' && path[i] != '[' {
				i++
			}
			if i == start {
				return nil, false
			}
			out = append(out, validationPathToken{property: path[start:i]})
		case '[':
			end := strings.IndexByte(path[i:], ']')
			if end <= 1 {
				return nil, false
			}
			end += i
			index, err := strconv.Atoi(path[i+1 : end])
			if err != nil || index < 0 {
				return nil, false
			}
			out = append(out, validationPathToken{index: index, isIndex: true})
			i = end + 1
		default:
			return nil, false
		}
	}
	return out, true
}

func decodeValidationJSON(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func validateSchemaValue(value any, rawSchema json.RawMessage, path string) error {
	trimmed := bytes.TrimSpace(rawSchema)
	if bytes.Equal(trimmed, []byte("true")) {
		return nil
	}
	if bytes.Equal(trimmed, []byte("false")) {
		return &SchemaViolation{Path: path, Reason: "schema forbids this value"}
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &schema); err != nil {
		// A malformed schema is a programming error, not a model failure.
		return &SchemaViolation{Path: path, Reason: "invalid tool schema: " + err.Error()}
	}

	if raw, ok := schema["type"]; ok && !validationTypeMatches(value, raw) {
		return &SchemaViolation{Path: path, Reason: "value type does not match schema type " + compactSchemaToken(raw)}
	}
	if raw, ok := schema["const"]; ok {
		want, err := decodeValidationJSON(raw)
		if err == nil && !reflect.DeepEqual(value, want) {
			return &SchemaViolation{Path: path, Reason: "value does not match const " + compactSchemaToken(raw)}
		}
	}
	if raw, ok := schema["enum"]; ok {
		var members []json.RawMessage
		if err := json.Unmarshal(raw, &members); err == nil && len(members) > 0 {
			matched := false
			for _, member := range members {
				want, err := decodeValidationJSON(member)
				if err == nil && reflect.DeepEqual(value, want) {
					matched = true
					break
				}
			}
			if !matched {
				return &SchemaViolation{Path: path, Reason: "value is not one of the schema enum members"}
			}
		}
	}

	switch typed := value.(type) {
	case map[string]any:
		if err := validateSchemaObject(typed, schema, path); err != nil {
			return err
		}
	case []any:
		if err := validateSchemaArray(typed, schema, path); err != nil {
			return err
		}
	case string:
		if err := validateSchemaString(typed, schema, path); err != nil {
			return err
		}
	case json.Number:
		if err := validateSchemaNumber(typed, schema, path); err != nil {
			return err
		}
	}

	if raw, ok := schema["allOf"]; ok {
		var branches []json.RawMessage
		if err := json.Unmarshal(raw, &branches); err == nil {
			for _, branch := range branches {
				if err := validateSchemaValue(value, branch, path); err != nil {
					return err
				}
			}
		}
	}
	if condition, ok := schema["if"]; ok {
		if validateSchemaValue(value, condition, path) == nil {
			if branch, exists := schema["then"]; exists {
				return validateSchemaValue(value, branch, path)
			}
		} else if branch, exists := schema["else"]; exists {
			return validateSchemaValue(value, branch, path)
		}
	}
	return nil
}

func validateSchemaObject(value map[string]any, schema map[string]json.RawMessage, path string) error {
	if raw, ok := schema["required"]; ok {
		var required []string
		if err := json.Unmarshal(raw, &required); err == nil {
			for _, name := range required {
				if _, exists := value[name]; !exists {
					return &SchemaViolation{Path: validationPropertyPath(path, name), Reason: "required property is missing"}
				}
			}
		}
	}
	properties := map[string]json.RawMessage{}
	if raw, ok := schema["properties"]; ok {
		_ = json.Unmarshal(raw, &properties)
	}
	for name, child := range value {
		childSchema, exists := properties[name]
		if exists {
			if err := validateSchemaValue(child, childSchema, validationPropertyPath(path, name)); err != nil {
				return err
			}
			continue
		}
		if raw, ok := schema["additionalProperties"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if bytes.Equal(trimmed, []byte("false")) {
				return &SchemaViolation{Path: validationPropertyPath(path, name), Reason: "additional property is not allowed"}
			}
			if len(trimmed) > 0 && trimmed[0] == '{' {
				if err := validateSchemaValue(child, raw, validationPropertyPath(path, name)); err != nil {
					return err
				}
			}
		}
	}
	if min, ok := validationIntKeyword(schema["minProperties"]); ok && len(value) < min {
		return &SchemaViolation{Path: path, Reason: fmt.Sprintf("object has %d properties; minimum is %d", len(value), min)}
	}
	if max, ok := validationIntKeyword(schema["maxProperties"]); ok && len(value) > max {
		return &SchemaViolation{Path: path, Reason: fmt.Sprintf("object has %d properties; maximum is %d", len(value), max)}
	}
	return nil
}

func validateSchemaArray(value []any, schema map[string]json.RawMessage, path string) error {
	if min, ok := validationIntKeyword(schema["minItems"]); ok && len(value) < min {
		return &SchemaViolation{Path: path, Reason: fmt.Sprintf("array has %d items; minimum is %d", len(value), min)}
	}
	if max, ok := validationIntKeyword(schema["maxItems"]); ok && len(value) > max {
		return &SchemaViolation{Path: path, Reason: fmt.Sprintf("array has %d items; maximum is %d", len(value), max)}
	}
	items, ok := schema["items"]
	if !ok {
		return nil
	}
	for i, child := range value {
		if err := validateSchemaValue(child, items, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaString(value string, schema map[string]json.RawMessage, path string) error {
	length := utf8.RuneCountInString(value)
	if min, ok := validationIntKeyword(schema["minLength"]); ok && length < min {
		return &SchemaViolation{Path: path, Reason: fmt.Sprintf("string length %d is below minimum %d", length, min)}
	}
	if max, ok := validationIntKeyword(schema["maxLength"]); ok && length > max {
		return &SchemaViolation{Path: path, Reason: fmt.Sprintf("string length %d exceeds maximum %d", length, max)}
	}
	return nil
}

func validateSchemaNumber(value json.Number, schema map[string]json.RawMessage, path string) error {
	number, err := value.Float64()
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return &SchemaViolation{Path: path, Reason: "invalid JSON number"}
	}
	if min, ok := validationFloatKeyword(schema["minimum"]); ok && number < min {
		return &SchemaViolation{Path: path, Reason: fmt.Sprintf("number %s is below minimum %s", value, strconv.FormatFloat(min, 'g', -1, 64))}
	}
	if max, ok := validationFloatKeyword(schema["maximum"]); ok && number > max {
		return &SchemaViolation{Path: path, Reason: fmt.Sprintf("number %s exceeds maximum %s", value, strconv.FormatFloat(max, 'g', -1, 64))}
	}
	return nil
}

func validationTypeMatches(value any, raw json.RawMessage) bool {
	var types []string
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		types = []string{one}
	} else if err := json.Unmarshal(raw, &types); err != nil {
		return true
	}
	for _, want := range types {
		switch want {
		case "null":
			if value == nil {
				return true
			}
		case "object":
			_, ok := value.(map[string]any)
			if ok {
				return true
			}
		case "array":
			_, ok := value.([]any)
			if ok {
				return true
			}
		case "string":
			_, ok := value.(string)
			if ok {
				return true
			}
		case "boolean":
			_, ok := value.(bool)
			if ok {
				return true
			}
		case "number":
			_, ok := value.(json.Number)
			if ok {
				return true
			}
		case "integer":
			if number, ok := value.(json.Number); ok && validationJSONInteger(number) {
				return true
			}
		}
	}
	return false
}

func validationJSONInteger(number json.Number) bool {
	if _, err := number.Int64(); err == nil {
		return true
	}
	value, err := number.Float64()
	return err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && math.Trunc(value) == value
}

func validationIntKeyword(raw json.RawMessage) (int, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return 0, false
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func validationFloatKeyword(raw json.RawMessage) (float64, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	return value, true
}

func validationPropertyPath(parent, name string) string {
	if parent == "" {
		parent = "$"
	}
	return parent + "." + name
}

func compactSchemaToken(raw json.RawMessage) string {
	return string(bytes.TrimSpace(raw))
}
