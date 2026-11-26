package agent

import (
	"bytes"
	"encoding/json"
	"maps"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type analyzerDecisionPatch struct {
	draft   map[string]json.RawMessage
	fields  []string
	message llm.Message
}

func newAnalyzerDecisionPatch(ctx *types.AgentContext, call llm.ToolCall, result types.ToolResult) *analyzerDecisionPatch {
	if call.Name != "emit_analysis" || result.ToolName != call.Name || result.Success ||
		result.Repair == nil || result.Repair.Code != tool.EmitAnalysisMissingDecisions || len(result.Repair.Fields) == 0 {
		return nil
	}
	patch := &analyzerDecisionPatch{fields: result.Repair.Fields}
	if json.Unmarshal(call.Params, &patch.draft) != nil || patch.draft == nil {
		return nil
	}
	for _, field := range patch.fields {
		if current := patch.draft[field]; len(current) != 0 && !bytes.Equal(bytes.TrimSpace(current), []byte("null")) {
			return nil
		}
	}
	prompt := "Complete the missing analysis decisions only. Decide from the current request and use its literal wording for source_quote. The existing analysis stays as submitted. For runtime_question_profile, use fact_families only with bounded_fact_set or bounded_effect_verdict. Omit fact_families for causal_diagnosis and other scopes. The two relation booleans are independent decisions. Call emit_analysis with only the fields in the provided schema."
	if len(patch.draft["requested_answer_dimensions"]) != 0 {
		prompt += "\nExisting answer dimensions to preserve:\n" + string(patch.draft["requested_answer_dimensions"])
	}
	patch.message = llm.Message{Role: "user", Content: prompt + "\nCurrent request:\n" + types.StripConversationPrefix(ctx.Objective)}
	return patch
}

func (p *analyzerDecisionPatch) project(schemas []llm.ToolSchema) (llm.ToolSchema, bool) {
	for _, schema := range schemas {
		if schema.Name != "emit_analysis" {
			continue
		}
		var full struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal(schema.Parameters, &full) != nil {
			return llm.ToolSchema{}, false
		}
		properties := make(map[string]json.RawMessage, len(p.fields))
		for _, field := range p.fields {
			if len(full.Properties[field]) == 0 {
				return llm.ToolSchema{}, false
			}
			properties[field] = full.Properties[field]
		}
		params, _ := json.Marshal(map[string]any{
			"type": "object", "properties": properties, "required": p.fields, "additionalProperties": false,
		})
		return llm.ToolSchema{Name: schema.Name, Description: "Supply only the missing analyzer decisions.", Parameters: params}, true
	}
	return llm.ToolSchema{}, false
}

func (p *analyzerDecisionPatch) expand(raw json.RawMessage) json.RawMessage {
	var patch map[string]json.RawMessage
	if json.Unmarshal(raw, &patch) != nil || len(patch) != len(p.fields) {
		return json.RawMessage("{}")
	}
	draft := maps.Clone(p.draft)
	for _, field := range p.fields {
		if len(patch[field]) == 0 || bytes.Equal(bytes.TrimSpace(patch[field]), []byte("null")) {
			return json.RawMessage("{}")
		}
		draft[field] = patch[field]
	}
	out, _ := json.Marshal(draft)
	return out
}
