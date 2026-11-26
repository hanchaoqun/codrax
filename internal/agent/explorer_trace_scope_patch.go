package agent

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceScopeChoices = `{"type":"string","enum":["referenced_instance","explicit_window"]}`
const traceScopeTeaching = "Choose referenced_instance for the complete identified work or explicit_window for the supplied times. Preserve the query view and other options. Do not expand a user-requested time window."

func newExplorerTraceScopePatch(call llm.ToolCall, repair *types.ToolRepair, question string) *explorerMeasurementPatch {
	patch := &explorerMeasurementPatch{}
	if json.Unmarshal(call.Params, &patch.draft) != nil {
		return nil
	}
	patch.schema = llm.ToolSchema{
		Name: call.Name, Description: "Choose the intended time scope for the saved trace query.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"scope":` + traceScopeChoices + `},"required":["scope"],"additionalProperties":false}`),
	}
	patch.message = llm.Message{Role: "user", Content: fmt.Sprintf(
		"The saved trace query requested two incompatible windows. %s\nTask: %s\nReferenced instance: %s\nSaved query: %s",
		traceScopeTeaching, question, repair.Metadata["referenced_window"], call.Params)}
	return patch
}

// Trace batches run in order. Use only a batch of trace calls whose results
// align, and send back only failed selectors, leaving successful evidence in
// history for the normal investigation.
func newExplorerTraceScopeBatchPatch(question string, calls []llm.ToolCall, results []types.ToolResult) *explorerMeasurementPatch {
	if len(calls) < 2 || len(calls) != len(results) {
		return nil
	}
	patch := &explorerMeasurementPatch{}
	properties := map[string]json.RawMessage{}
	var required []string
	var prompt strings.Builder
	for i, call := range calls {
		result := results[i]
		if call.Name != "trace_query" || result.ToolName != call.Name {
			return nil
		}
		if result.Success {
			continue
		}
		if result.Repair == nil || result.Repair.Code != tool.TraceQueryBusinessWindowConflict {
			return nil
		}
		slot := fmt.Sprintf("query_%d", len(patch.traceBatch)+1)
		required = append(required, slot)
		properties[slot] = json.RawMessage(traceScopeChoices)
		patch.traceBatch = append(patch.traceBatch, call)
		fmt.Fprintf(&prompt, "\n%s referenced instance: %s\n%s saved query: %s\n",
			slot, result.Repair.Metadata["referenced_window"], slot, call.Params)
	}
	if len(required) == 0 {
		return nil
	}
	params, _ := json.Marshal(map[string]any{
		"type": "object", "properties": properties, "required": required, "additionalProperties": false,
	})
	patch.schema = llm.ToolSchema{Name: "trace_query", Description: "Choose the time scope of each rejected query in one call.", Parameters: params}
	patch.message = llm.Message{Role: "user", Content: traceScopeTeaching +
		" Return one call containing every listed field.\nTask: " + types.StripConversationPrefix(question) + "\n" + prompt.String()}
	return patch
}

func (p *explorerMeasurementPatch) expandTraceScopeBatch(call llm.ToolCall) []llm.ToolCall {
	var choices map[string]string
	if json.Unmarshal(call.Params, &choices) != nil || len(choices) != len(p.traceBatch) {
		choices = nil
	}
	for i := range p.traceBatch {
		switch choices[fmt.Sprintf("query_%d", i+1)] {
		case "referenced_instance", "explicit_window":
		default:
			choices = nil
		}
	}
	expanded := append([]llm.ToolCall(nil), p.traceBatch...)
	for i := range expanded {
		slot := fmt.Sprintf("query_%d", i+1)
		expanded[i].ID = call.ID + "_" + slot
		if choices != nil {
			var draft map[string]json.RawMessage
			// The trace tool already decoded this saved query successfully.
			_ = json.Unmarshal(expanded[i].Params, &draft)
			applyTraceScopeChoice(draft, choices[slot])
			expanded[i].Params, _ = json.Marshal(draft)
		}
	}
	return expanded
}

func (p *explorerMeasurementPatch) expandTraceScope(raw json.RawMessage) json.RawMessage {
	var choice struct {
		Scope string `json:"scope"`
	}
	draft := maps.Clone(p.draft)
	if json.Unmarshal(raw, &choice) == nil {
		applyTraceScopeChoice(draft, choice.Scope)
	}
	// An invalid choice keeps the original conflict for the normal validator.
	expanded, _ := json.Marshal(draft)
	return expanded
}

func applyTraceScopeChoice(draft map[string]json.RawMessage, scope string) {
	switch scope {
	case "referenced_instance":
		delete(draft, "time_start")
		delete(draft, "time_end")
	case "explicit_window":
		delete(draft, "business_span_ref")
	}
}
