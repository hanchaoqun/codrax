package agent

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Publish only exact available choices. The model chooses their relevance;
// neither schema projection nor completion creates a population from prose.
func projectExplorerMeasurementMemberSchema(ctx *types.AgentContext, schemas []llm.ToolSchema) []llm.ToolSchema {
	const field = "runtime_measurement_member_sets"
	var choices []types.AnswerRuntimeMeasurementReceipt
	if ctx != nil && ctx.Mutable != nil && ctx.AnalysisIR != nil {
		rm := &ctx.AnalysisIR.RequestModel
		if types.RuntimeMeasurementMemberSetDomain(rm, types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, types.ObservationLedger{})) {
			for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
				if table.CoversMemberSet(rm) {
					choices = append(choices, types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View})
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
		var root map[string]json.RawMessage
		var properties map[string]json.RawMessage
		if json.Unmarshal(schema.Parameters, &root) != nil || json.Unmarshal(root["properties"], &properties) != nil {
			continue
		}
		var property map[string]json.RawMessage
		var items map[string]json.RawMessage
		if json.Unmarshal(properties[field], &property) != nil || json.Unmarshal(property["items"], &items) != nil {
			continue
		}
		items["enum"], _ = json.Marshal(choices)
		property["items"], _ = json.Marshal(items)
		properties[field], _ = json.Marshal(property)
		root["properties"], _ = json.Marshal(properties)
		out[i].Parameters, _ = json.Marshal(root)
	}
	return out
}
