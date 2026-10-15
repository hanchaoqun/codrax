package agent

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Use the same current contract as completion and finalization. These bounded
// choices teach existing identities, not publication, population or causal
// authority. Event inventory uses another publication lane and is not offered.
func projectExplorerNativeFactHandoffSchema(ctx *types.AgentContext, schemas []llm.ToolSchema) []llm.ToolSchema {
	const field = "native_fact_handoff"
	var choices []types.AnswerRuntimeMeasurementReceipt
	if ctx != nil && ctx.Mutable != nil {
		if view := types.BuildAnswerSemanticViewForAgentContext(ctx); view != nil {
			for _, table := range view.RuntimeMeasurementContract.Choices() {
				if table.EventData != nil {
					continue
				}
				choices = append(choices, types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View})
				if len(choices) == 32 {
					break
				}
			}
		}
	}
	out := append([]llm.ToolSchema(nil), schemas...)
	for i, schema := range out {
		if schema.Name != explorerCompletionToolName {
			continue
		}
		if len(choices) == 0 {
			if projected, ok := omitJSONSchemaTopLevelProperty(schema.Parameters, field); ok {
				out[i].Parameters = projected
			}
			continue
		}
		var root, properties, handoff, fields, selections, items map[string]json.RawMessage
		if json.Unmarshal(schema.Parameters, &root) != nil || json.Unmarshal(root["properties"], &properties) != nil ||
			json.Unmarshal(properties[field], &handoff) != nil || json.Unmarshal(handoff["properties"], &fields) != nil ||
			json.Unmarshal(fields["runtime_measurements"], &selections) != nil || json.Unmarshal(selections["items"], &items) != nil {
			continue
		}
		items["enum"], _ = json.Marshal(choices)
		selections["items"], _ = json.Marshal(items)
		fields["runtime_measurements"], _ = json.Marshal(selections)
		handoff["properties"], _ = json.Marshal(fields)
		properties[field], _ = json.Marshal(handoff)
		var reason map[string]json.RawMessage
		var description string
		if json.Unmarshal(properties["reason"], &reason) == nil && json.Unmarshal(reason["description"], &description) == nil {
			reason["description"], _ = json.Marshal("When native_fact_handoff is selected, reason is lifecycle rationale only; put every independent conclusion, diagnosis and scope boundary in its interpretation. Otherwise use the following legacy reason contract: " + description)
			properties["reason"], _ = json.Marshal(reason)
		}
		root["properties"], _ = json.Marshal(properties)
		out[i].Parameters, _ = json.Marshal(root)
	}
	return out
}
