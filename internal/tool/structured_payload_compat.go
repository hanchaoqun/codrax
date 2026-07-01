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
	repaired, report := toolparam.Normalize(raw, schema, types.DefaultToolParamCompatConfig())
	if !report.Changed() {
		return raw
	}
	logging.Warning("[structured_payload_compat] tool=%s bytes=%d→%d arrays=%s repairs=%s",
		toolName, len(raw), len(repaired), topLevelArrayLengthSummary(repaired), report.Summary(8))
	return repaired
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
