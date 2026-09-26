package agent

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Consume the registered typed projection, not raw event text. The parent maps
// are detached JSON display objects; the ledger and its exact fields stay intact.
func traceInventorySemanticBudgetFields(object map[string]any) []traceInventoryBudgetField {
	wire, _ := json.Marshal(object)
	var semantics types.TraceEventSemantics
	if json.Unmarshal(wire, &semantics) != nil || !types.ValidateTraceEventSemantics(&semantics) {
		return nil
	}
	rows, ok := object["fields"].([]any)
	if !ok || len(rows) != len(semantics.Fields) {
		return nil
	}
	var fields []traceInventoryBudgetField
	for n, field := range semantics.Fields {
		descriptor, known := types.LookupTraceEventSemanticDescriptor(field.Key)
		if !known || descriptor.Type != "text" || field.Status != "known" || field.Value == nil {
			continue
		}
		parent, ok := rows[n].(map[string]any)
		if !ok {
			continue
		}
		reduced := types.OmitTraceEventSemanticValue(field, "prompt_budget")
		encoded, _ := json.Marshal(reduced)
		original, _ := json.Marshal(field)
		if len(encoded) >= len(original) {
			continue // Omitting a short identity would cost more and help less.
		}
		var replacement map[string]any
		_ = json.Unmarshal(encoded, &replacement)
		fields = append(fields, traceInventoryBudgetField{
			parent: parent, key: "value", path: fmt.Sprintf("/semantics/fields/%d/value", n),
			bytes: len(original) - len(encoded), replacement: replacement,
		})
	}
	return fields
}
