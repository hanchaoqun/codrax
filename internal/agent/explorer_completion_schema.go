package agent

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

const explorerCompletionToolName = "emit_investigation_complete"

// projectExplorerCompletionRelationClaimSchema removes an optional field that
// has no legal value in the current typed context. It deliberately uses the
// same observation-ledger authority compiler as emit_investigation_complete;
// user text, model prose, trace filename heuristics, and loose tool-summary
// substring matches do not participate.
//
// The input slice and its RawMessage bytes are never mutated. Malformed or
// unfamiliar schemas fail open so schema projection cannot strand the agent.
func projectExplorerCompletionRelationClaimSchema(ctx *types.AgentContext, schemas []llm.ToolSchema) []llm.ToolSchema {
	if explorerHasCopyableTraceRelationAuthority(ctx) {
		return schemas
	}
	var out []llm.ToolSchema
	for i, schema := range schemas {
		if strings.TrimSpace(schema.Name) != explorerCompletionToolName {
			continue
		}
		projected, ok := omitJSONSchemaTopLevelProperty(schema.Parameters, "relation_claims")
		if !ok {
			continue
		}
		if out == nil {
			out = append([]llm.ToolSchema(nil), schemas...)
		}
		out[i].Parameters = projected
	}
	if out == nil {
		return schemas
	}
	return out
}

func explorerHasCopyableTraceRelationAuthority(ctx *types.AgentContext) bool {
	// ObservationLedgerInputFromAgentContext intentionally reads the shared
	// mutable dispatch buffer. A nil Mutable means no current-dispatch typed
	// trace result can exist, and also avoids asking the context adapter to read
	// mutable-only supplement state.
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromAgentContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit,
	))
	return len(types.CompileTraceAnswerRelationAuthoritiesFromLedger(ledger)) > 0
}

func omitJSONSchemaTopLevelProperty(raw json.RawMessage, property string) (json.RawMessage, bool) {
	if len(raw) == 0 || strings.TrimSpace(property) == "" {
		return nil, false
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, false
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(root["properties"], &properties); err != nil {
		return nil, false
	}
	if _, exists := properties[property]; !exists {
		return nil, false
	}
	delete(properties, property)
	propertiesJSON, err := json.Marshal(properties)
	if err != nil {
		return nil, false
	}
	root["properties"] = propertiesJSON
	projected, err := json.Marshal(root)
	if err != nil {
		return nil, false
	}
	return json.RawMessage(projected), true
}
