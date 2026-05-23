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
	repaired, report := toolparam.Normalize(raw, schema, types.DefaultToolParamCompatConfig())
	if !report.Changed() {
		return raw
	}
	logging.Warning("[structured_payload_compat] tool=%s bytes=%d→%d arrays=%s repairs=%s",
		toolName, len(raw), len(repaired), topLevelArrayLengthSummary(repaired), report.Summary(8))
	return repaired
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
