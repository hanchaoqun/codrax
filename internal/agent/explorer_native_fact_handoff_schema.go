package agent

import (
	"encoding/json"
	"slices"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Use the same current contract as completion and finalization. These bounded
// choices teach existing identities, not publication, population or causal
// authority. Event inventory uses another publication lane and is not offered.
func projectExplorerNativeFactHandoffSchema(ctx *types.AgentContext, schemas []llm.ToolSchema) []llm.ToolSchema {
	const field = "native_fact_handoff"
	bus := types.ToolBusContext(ctx, types.AgentExplorer)
	choices := tool.NativeFactHandoffChoices(bus)
	displayChoices := tool.NativeFactAnswerDisplayChoices(bus)
	required := tool.ManagedDeliveryRequiresNativeFactHandoff(bus)
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
		projectExplorerNativeRowReferenceSchema(ctx, fields, choices)
		if len(displayChoices) == 0 {
			delete(fields, "answer_display")
		} else {
			var display map[string]json.RawMessage
			if json.Unmarshal(fields["answer_display"], &display) == nil {
				items["enum"], _ = json.Marshal(displayChoices)
				display["items"], _ = json.Marshal(items)
				display["description"], _ = json.Marshal(types.NativeAnswerDisplayTeaching)
				fields["answer_display"], _ = json.Marshal(display)
			}
		}
		handoff["properties"], _ = json.Marshal(fields)
		// New model calls declare the lane explicitly. The wire validator keeps
		// accepting older persisted calls without a role as legacy advisory.
		var handoffRequired []string
		if json.Unmarshal(handoff["required"], &handoffRequired) != nil {
			continue
		}
		if !slices.Contains(handoffRequired, "interpretation_role") {
			handoffRequired = append(handoffRequired, "interpretation_role")
		}
		handoff["required"], _ = json.Marshal(handoffRequired)
		if required {
			handoff["description"], _ = json.Marshal(tool.ManagedDeliveryNativeFactHandoffTeaching)
			var fieldsRequired []string
			_ = json.Unmarshal(root["required"], &fieldsRequired)
			found := false
			for _, name := range fieldsRequired {
				found = found || name == field
			}
			if !found {
				fieldsRequired = append(fieldsRequired, field)
			}
			root["required"], _ = json.Marshal(fieldsRequired)
		}
		properties[field], _ = json.Marshal(handoff)
		var reason map[string]json.RawMessage
		var description string
		if json.Unmarshal(properties["reason"], &reason) == nil && json.Unmarshal(reason["description"], &description) == nil {
			if required {
				reason["description"], _ = json.Marshal("Lifecycle rationale only. Keep independent analysis in native_fact_handoff.interpretation_units or its legacy interpretation, not in reason. Delivery status and links belong to the terminal component.")
			} else {
				reason["description"], _ = json.Marshal("When native_fact_handoff is selected, reason is lifecycle rationale only; put every independent conclusion, diagnosis and scope boundary in its interpretation_units or legacy interpretation. Otherwise use the following legacy reason contract: " + description)
			}
			properties["reason"], _ = json.Marshal(reason)
		}
		root["properties"], _ = json.Marshal(properties)
		out[i].Parameters, _ = json.Marshal(root)
	}
	return out
}
