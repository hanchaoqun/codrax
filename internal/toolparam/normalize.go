package toolparam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Repair describes one deterministic, schema-proven tool-argument rewrite.
// It is telemetry only; the caller decides whether audit mode logs it or repair
// mode applies the normalized payload.
type Repair struct {
	Path string
	Rule string
	From string
	To   string
}

// Report is returned for both audit and repair modes.
type Report struct {
	Repairs []Repair
}

func (r Report) Changed() bool { return len(r.Repairs) > 0 }

func (r Report) Summary(limit int) string {
	if len(r.Repairs) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(r.Repairs) {
		limit = len(r.Repairs)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		repair := r.Repairs[i]
		parts = append(parts, fmt.Sprintf("%s %s->%s via %s", repair.Path, repair.From, repair.To, repair.Rule))
	}
	if len(r.Repairs) > limit {
		parts = append(parts, fmt.Sprintf("... %d more", len(r.Repairs)-limit))
	}
	return strings.Join(parts, "; ")
}

type schemaNode struct {
	Type       any                        `json:"type"`
	Properties map[string]json.RawMessage `json:"properties"`
	Items      json.RawMessage            `json:"items"`
}

// Normalize applies bounded, schema-aware compatibility rewrites to a tool
// parameter object. It never fills missing required fields, invents values,
// reads prose, or drops unknown keys. Empty/off policy returns raw unchanged.
func Normalize(raw json.RawMessage, schema json.RawMessage, cfg types.ToolParamCompatConfig) (json.RawMessage, Report) {
	mode := cfg.NormalizedMode()
	if mode != types.ToolParamCompatAudit && mode != types.ToolParamCompatRepair {
		return raw, Report{}
	}
	value, ok := decodeJSONValue(raw)
	if !ok {
		return raw, Report{}
	}
	normalized, repairs := normalizeValue(value, schema, "$", cfg)
	if len(repairs) == 0 {
		return raw, Report{}
	}
	report := Report{Repairs: repairs}
	if mode == types.ToolParamCompatAudit {
		return raw, report
	}
	encoded, err := json.Marshal(normalized)
	if err != nil || !json.Valid(encoded) {
		return raw, Report{}
	}
	return json.RawMessage(encoded), report
}

func normalizeValue(value any, schema json.RawMessage, path string, cfg types.ToolParamCompatConfig) (any, []Repair) {
	node, ok := parseSchema(schema)
	if !ok {
		return value, nil
	}

	if schemaExpectsObject(node) {
		if s, ok := value.(string); ok && !typeAllows(node.Type, "string") {
			if decoded, ok := decodeJSONStringAs(s, "object"); ok {
				repairs := []Repair{repair(path, "json_string_object", valueKind(value), "object")}
				value = decoded
				if nested, ok := value.(map[string]any); ok {
					out, nestedRepairs := normalizeObject(nested, node, path, cfg)
					return out, append(repairs, nestedRepairs...)
				}
				return value, repairs
			}
		}
		if m, ok := value.(map[string]any); ok {
			return normalizeObject(m, node, path, cfg)
		}
	}

	if schemaExpectsArray(node) {
		if s, ok := value.(string); ok && !typeAllows(node.Type, "string") {
			if decoded, ok := decodeJSONStringAs(s, "array"); ok {
				repairs := []Repair{repair(path, "json_string_array", valueKind(value), "array")}
				value = decoded
				if arr, ok := value.([]any); ok {
					out, nestedRepairs := normalizeArray(arr, node, path, cfg)
					return out, append(repairs, nestedRepairs...)
				}
				return value, repairs
			}
			if cfg.SplitStringArraysEnabled() && arrayItemsAllowOnlyString(node) {
				if arr, ok := splitStringArray(s); ok {
					return arr, []Repair{repair(path, "delimited_string_array", valueKind(value), "array")}
				}
			}
		}
		if arr, ok := value.([]any); ok {
			return normalizeArray(arr, node, path, cfg)
		}
	}

	if s, ok := value.(string); ok && !typeAllows(node.Type, "string") {
		if typeAllows(node.Type, "integer") {
			if v, ok := parseJSONStringInteger(s); ok {
				return v, []Repair{repair(path, "string_integer", valueKind(value), "integer")}
			}
		}
		if typeAllows(node.Type, "number") {
			if v, ok := parseJSONStringNumber(s); ok {
				return v, []Repair{repair(path, "string_number", valueKind(value), "number")}
			}
		}
		if typeAllows(node.Type, "boolean") {
			if v, ok := parseJSONStringBool(s); ok {
				return v, []Repair{repair(path, "string_boolean", valueKind(value), "boolean")}
			}
		}
	}

	return value, nil
}

func normalizeObject(in map[string]any, node schemaNode, path string, cfg types.ToolParamCompatConfig) (map[string]any, []Repair) {
	if len(node.Properties) == 0 {
		return in, nil
	}
	var out map[string]any
	var repairs []Repair
	for key, propSchema := range node.Properties {
		current, exists := in[key]
		if !exists {
			continue
		}
		normalized, fieldRepairs := normalizeValue(current, propSchema, path+"."+key, cfg)
		if len(fieldRepairs) == 0 {
			continue
		}
		if out == nil {
			out = make(map[string]any, len(in))
			for k, v := range in {
				out[k] = v
			}
		}
		out[key] = normalized
		repairs = append(repairs, fieldRepairs...)
	}
	if out == nil {
		return in, nil
	}
	return out, repairs
}

func normalizeArray(in []any, node schemaNode, path string, cfg types.ToolParamCompatConfig) ([]any, []Repair) {
	if len(node.Items) == 0 {
		return in, nil
	}
	var out []any
	var repairs []Repair
	for i, item := range in {
		normalized, itemRepairs := normalizeValue(item, node.Items, fmt.Sprintf("%s[%d]", path, i), cfg)
		if len(itemRepairs) == 0 {
			continue
		}
		if out == nil {
			out = append([]any(nil), in...)
		}
		out[i] = normalized
		repairs = append(repairs, itemRepairs...)
	}
	if out == nil {
		return in, nil
	}
	return out, repairs
}

func parseSchema(raw json.RawMessage) (schemaNode, bool) {
	if len(raw) == 0 {
		return schemaNode{}, false
	}
	var node schemaNode
	if err := json.Unmarshal(raw, &node); err != nil {
		return schemaNode{}, false
	}
	return node, true
}

func typeAllows(raw any, want string) bool {
	switch v := raw.(type) {
	case string:
		return v == want
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func schemaExpectsObject(node schemaNode) bool {
	return typeAllows(node.Type, "object") || (node.Type == nil && len(node.Properties) > 0)
}

func schemaExpectsArray(node schemaNode) bool {
	return typeAllows(node.Type, "array") || (node.Type == nil && len(node.Items) > 0)
}

func arrayItemsAllowOnlyString(node schemaNode) bool {
	child, ok := parseSchema(node.Items)
	if !ok {
		return false
	}
	return typeAllows(child.Type, "string") &&
		!typeAllows(child.Type, "object") &&
		!typeAllows(child.Type, "array") &&
		!typeAllows(child.Type, "integer") &&
		!typeAllows(child.Type, "number") &&
		!typeAllows(child.Type, "boolean")
}

func decodeJSONValue(raw json.RawMessage) (any, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, false
	}
	return value, true
}

func decodeJSONStringAs(s string, want string) (any, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil, false
	}
	switch want {
	case "object":
		if !strings.HasPrefix(trimmed, "{") {
			return nil, false
		}
	case "array":
		if !strings.HasPrefix(trimmed, "[") {
			return nil, false
		}
	default:
		return nil, false
	}
	return decodeJSONValue(json.RawMessage(trimmed))
}

func splitStringArray(s string) ([]any, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return nil, false
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return nil, false
	}
	out := make([]any, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func parseJSONStringInteger(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	for i, r := range s {
		if (r == '+' || r == '-') && i == 0 {
			continue
		}
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}

func parseJSONStringNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func parseJSONStringBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func valueKind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number, float64, int, int64:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func repair(path, rule, from, to string) Repair {
	return Repair{Path: path, Rule: rule, From: from, To: to}
}
