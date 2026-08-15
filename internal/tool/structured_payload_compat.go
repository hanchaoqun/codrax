package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// applyStructuredPayloadCompat is the shared tool-boundary compatibility layer
// for deterministic JSON carrier repairs. It delegates schema-proven rewrites
// to internal/toolparam so individual tools do not grow one-off repair paths.
//
// Red lines:
//   - repair is structural only: no answer text is invented or deleted;
//   - repairs are schema-driven, not based on user prose or model free text;
//   - ambiguous / non-lossless payloads pass through to the tool's validator.
func applyStructuredPayloadCompat(toolName string, raw json.RawMessage, schema json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 || len(bytes.TrimSpace(schema)) == 0 {
		return raw
	}
	if repaired, ok := repairRedundantToolNameTypeField(toolName, raw, schema); ok {
		logging.Warning("[structured_payload_compat] tool=%s redundant top-level type field removed before schema normalization", toolName)
		raw = repaired
	}
	if repaired, fields, ok := repairRedundantUniqueNestedBooleanFields(raw, schema); ok {
		logging.Warning("[structured_payload_compat] tool=%s redundant top-level boolean field(s) removed; identical canonical nested decisions retained: %s",
			toolName, strings.Join(fields, ", "))
		raw = repaired
	}
	repaired, report := toolparam.Normalize(raw, schema, types.DefaultToolParamCompatConfig())
	if !report.Changed() {
		return raw
	}
	logging.Warning("[structured_payload_compat] tool=%s bytes=%d→%d arrays=%s repairs=%s",
		toolName, len(raw), len(repaired), topLevelArrayLengthSummary(repaired), report.Summary(8))
	return repaired
}

// repairRedundantUniqueNestedBooleanFields is a schema-driven lossless repair
// for a recurring structured-emission class: a model emits a decision both in
// its canonical nested object and as an extra top-level key. A field is removed
// only when all of these precise conditions hold:
//   - the top-level key is not owned by the schema;
//   - the schema contains exactly one nested object path with that key and its
//     declared type is boolean;
//   - the canonical nested payload is present as a native JSON boolean;
//   - the redundant value is that same boolean (native or "true"/"false").
//
// Missing, ambiguous, malformed, or conflicting shapes pass through unchanged
// to strict decoding. Arrays are deliberately outside this repair because a
// top-level scalar cannot identify which array member it duplicates.
func repairRedundantUniqueNestedBooleanFields(raw json.RawMessage, schema json.RawMessage) (json.RawMessage, []string, bool) {
	var root map[string]json.RawMessage
	var schemaRoot struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(raw, &root) != nil || json.Unmarshal(schema, &schemaRoot) != nil || len(root) == 0 {
		return raw, nil, false
	}
	keys := make([]string, 0, len(root))
	for key := range root {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	removed := make([]string, 0, 2)
	for _, key := range keys {
		if _, schemaOwnsTopLevel := schemaRoot.Properties[key]; schemaOwnsTopLevel {
			continue
		}
		paths := structuredNestedBooleanSchemaPaths(schema, key)
		if len(paths) != 1 {
			continue
		}
		canonicalRaw, ok := structuredRawValueAtObjectPath(raw, paths[0])
		if !ok {
			continue
		}
		var canonical bool
		if json.Unmarshal(canonicalRaw, &canonical) != nil {
			continue
		}
		legacy, ok := structuredCompatBool(root[key])
		if !ok || legacy != canonical {
			continue
		}
		delete(root, key)
		removed = append(removed, key+"→"+strings.Join(paths[0], "."))
	}
	if len(removed) == 0 {
		return raw, nil, false
	}
	repaired, err := json.Marshal(root)
	if err != nil || !json.Valid(repaired) {
		return raw, nil, false
	}
	return repaired, removed, true
}

func structuredNestedBooleanSchemaPaths(schema json.RawMessage, field string) [][]string {
	var out [][]string
	var walk func(json.RawMessage, []string, bool)
	walk = func(nodeRaw json.RawMessage, prefix []string, root bool) {
		var node struct {
			Type       string                     `json:"type"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal(nodeRaw, &node) != nil || len(node.Properties) == 0 {
			return
		}
		keys := make([]string, 0, len(node.Properties))
		for key := range node.Properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := node.Properties[key]
			path := append(append([]string(nil), prefix...), key)
			var shape struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(child, &shape) != nil {
				continue
			}
			if !root && key == field && shape.Type == "boolean" {
				out = append(out, path)
			}
			if shape.Type == "object" {
				walk(child, path, false)
			}
		}
	}
	walk(schema, nil, true)
	return out
}

func structuredRawValueAtObjectPath(raw json.RawMessage, path []string) (json.RawMessage, bool) {
	current := raw
	for _, key := range path {
		var obj map[string]json.RawMessage
		if json.Unmarshal(current, &obj) != nil {
			return nil, false
		}
		next, ok := obj[key]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func structuredCompatBool(raw json.RawMessage) (bool, bool) {
	var value bool
	if json.Unmarshal(raw, &value) == nil {
		return value, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return false, false
	}
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func repairRedundantToolNameTypeField(toolName string, raw json.RawMessage, schema json.RawMessage) (json.RawMessage, bool) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return raw, false
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil || len(root) == 0 {
		return raw, false
	}
	typeRaw, ok := root["type"]
	if !ok {
		return raw, false
	}
	var emittedType string
	if err := json.Unmarshal(typeRaw, &emittedType); err != nil || strings.TrimSpace(emittedType) != toolName {
		return raw, false
	}
	var schemaRoot struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &schemaRoot); err != nil || len(schemaRoot.Properties) == 0 {
		return raw, false
	}
	if _, schemaOwnsType := schemaRoot.Properties["type"]; schemaOwnsType {
		return raw, false
	}
	hasSchemaPayload := false
	for key := range root {
		if key == "type" {
			continue
		}
		if _, ok := schemaRoot.Properties[key]; ok {
			hasSchemaPayload = true
			break
		}
	}
	if !hasSchemaPayload {
		return raw, false
	}
	delete(root, "type")
	repaired, err := json.Marshal(root)
	if err != nil || !json.Valid(repaired) {
		return raw, false
	}
	return repaired, true
}

// applyStructuredPayloadCompatWithLegacyStringFieldRepair keeps the remaining
// flat-mode string-wrapper tolerance behind the shared compatibility boundary.
// The legacy helpers handle provider/local-model artefacts that are still
// outside internal/toolparam's fully schema-driven normalizer, such as
// object-fragment arrays or selected object fields that arrived as JSON
// strings. They are still structural-only repairs: no answer rows, summaries,
// citations, or user intent are authored here.
func applyStructuredPayloadCompatWithLegacyStringFieldRepair(toolName string, raw json.RawMessage, schema json.RawMessage, objectFields ...string) json.RawMessage {
	if repaired, fields, ok := repairStringWrappedArrayFields(raw); ok {
		logging.Warning("[structured_payload_compat] tool=%s legacy string-wrapped array field(s) repaired before schema normalization: %s",
			toolName, strings.Join(fields, ", "))
		raw = repaired
	}
	if len(objectFields) > 0 {
		if repaired, fields, ok := repairStringWrappedObjectFields(raw, objectFields...); ok {
			logging.Warning("[structured_payload_compat] tool=%s legacy string-wrapped object field(s) repaired before schema normalization: %s",
				toolName, strings.Join(fields, ", "))
			raw = repaired
		}
	}
	return applyStructuredPayloadCompat(toolName, raw, schema)
}

func applyStructuredPayloadCompatWithSelectedStringFieldRepair(toolName string, raw json.RawMessage, schema json.RawMessage, arrayFields []string, objectFields ...string) json.RawMessage {
	if len(arrayFields) > 0 {
		if repaired, fields, ok := repairSelectedStringWrappedArrayFields(raw, arrayFields...); ok {
			logging.Warning("[structured_payload_compat] tool=%s string-wrapped array field(s) repaired before schema normalization: %s",
				toolName, strings.Join(fields, ", "))
			raw = repaired
		}
	} else if repaired, fields, ok := repairStringWrappedArrayFields(raw); ok {
		logging.Warning("[structured_payload_compat] tool=%s legacy string-wrapped array field(s) repaired before schema normalization: %s",
			toolName, strings.Join(fields, ", "))
		raw = repaired
	}
	if len(objectFields) > 0 {
		if repaired, fields, ok := repairStringWrappedObjectFields(raw, objectFields...); ok {
			logging.Warning("[structured_payload_compat] tool=%s string-wrapped object field(s) repaired before schema normalization: %s",
				toolName, strings.Join(fields, ", "))
			raw = repaired
		}
	}
	return applyStructuredPayloadCompat(toolName, raw, schema)
}

func topLevelArrayLengthSummary(raw json.RawMessage) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || len(obj) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(obj))
	lengths := make(map[string]int, len(obj))
	for key, val := range obj {
		val = bytes.TrimSpace(val)
		if len(val) == 0 || val[0] != '[' {
			continue
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(val, &arr); err != nil {
			continue
		}
		keys = append(keys, key)
		lengths[key] = len(arr)
	}
	if len(keys) == 0 {
		return "-"
	}
	sort.Strings(keys)
	const maxKeys = 6
	parts := make([]string, 0, structuredPayloadMinInt(len(keys), maxKeys)+1)
	for i, key := range keys {
		if i >= maxKeys {
			parts = append(parts, fmt.Sprintf("+%d", len(keys)-maxKeys))
			break
		}
		parts = append(parts, fmt.Sprintf("%s=%d", key, lengths[key]))
	}
	return strings.Join(parts, ",")
}

func structuredPayloadMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
