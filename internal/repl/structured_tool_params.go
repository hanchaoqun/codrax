package repl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func unmarshalReplStructuredToolParams(tool llm.ToolSchema, raw []byte, dst any, scope string) error {
	normalized := normalizeReplStructuredToolParams(tool, json.RawMessage(raw), scope)
	if err := json.Unmarshal(normalized, dst); err == nil {
		return nil
	}
	if !bytes.Equal(bytes.TrimSpace(normalized), bytes.TrimSpace(raw)) {
		err := json.Unmarshal(normalized, dst)
		return fmt.Errorf("%s: unmarshal normalized tool params: %w; %s", scope, err, replStructuredToolRetryHint(tool))
	}
	err := json.Unmarshal(raw, dst)
	return fmt.Errorf("%s: unmarshal tool params: %w; %s", scope, err, replStructuredToolRetryHint(tool))
}

func normalizeReplStructuredToolParams(tool llm.ToolSchema, raw json.RawMessage, scope string) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw
	}
	candidate := json.RawMessage(trimmed)
	var repairs []string
	if repaired, ok := repairTurnPolicyParamsJSON(trimmed); ok {
		repaired = bytes.TrimSpace(repaired)
		if len(repaired) > 0 && !bytes.Equal(repaired, candidate) {
			candidate = json.RawMessage(repaired)
			repairs = append(repairs, "json_object_fragment")
		}
	}
	normalized, report := toolparam.Normalize(candidate, tool.Parameters, types.DefaultToolParamCompatConfig())
	if report.Changed() {
		candidate = normalized
		repairs = append(repairs, report.Summary(8))
	}
	if len(repairs) > 0 {
		logging.Warning("[repl/tool_param_compat] tool=%s scope=%s bytes=%d→%d repairs=%s",
			tool.Name, scope, len(raw), len(candidate), stringsJoinNonEmpty(repairs, "; "))
	}
	return candidate
}

func replStructuredToolRetryHint(tool llm.ToolSchema) string {
	name := tool.Name
	if name == "" {
		name = "the tool"
	}
	return fmt.Sprintf("re-emit %s with a single JSON object matching its input schema; keep arrays as JSON arrays, objects as JSON objects, booleans/numbers as typed values, and use exact schema field names", name)
}

func stringsJoinNonEmpty(values []string, sep string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return strings.Join(out, sep)
}
