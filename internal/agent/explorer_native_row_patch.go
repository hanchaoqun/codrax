package agent

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

type explorerNativeRowRepair struct {
	units  []types.InvestigationInterpretationUnit
	slots  []types.InvalidInvestigationNativeRow
	tables map[string]types.RuntimeMeasurementTable
}

func prepareExplorerNativeRowRepair(p *explorerMeasurementPatch, name string, repair *types.ToolRepair, receipts []types.AnswerRuntimeMeasurementReceipt, contract *types.RuntimeMeasurementContract) *explorerMeasurementPatch {
	r := &explorerNativeRowRepair{tables: map[string]types.RuntimeMeasurementTable{}}
	if json.Unmarshal([]byte(repair.Metadata["invalid_native_rows"]), &r.slots) != nil || len(r.slots) == 0 ||
		json.Unmarshal([]byte(repair.Metadata["current_interpretation_units"]), &r.units) != nil {
		return nil
	}
	var roster []map[string]any
	var alternatives []any
	for _, receipt := range receipts {
		if !types.BindRuntimeMeasurementReceipt(&receipt, contract) || !receipt.IsBound() {
			continue
		}
		table := *receipt.BoundTable
		if _, current := table.NativeSourceTitleBasis(); !current || len(table.Rows) == 0 {
			continue
		}
		id := fmt.Sprintf("r%d", len(roster)+1)
		r.tables[id] = table
		roster = append(roster, map[string]any{"ref": id, "label": table.Label, "columns": table.Columns, "example_rows": table.Rows[:min(2, len(table.Rows))], "row_count": len(table.Rows)})
		alternatives = append(alternatives, map[string]any{
			"type": "object", "properties": map[string]any{
				"ref": map[string]any{"const": id}, "row": map[string]any{"type": "integer", "minimum": 0, "maximum": len(table.Rows) - 1},
			}, "required": []string{"ref", "row"}, "additionalProperties": false,
		})
	}
	if len(alternatives) == 0 {
		return nil
	}
	alternatives = append(alternatives, map[string]any{
		"type": "object", "properties": map[string]any{"ref": map[string]any{"const": "no_match"}},
		"required": []string{"ref"}, "additionalProperties": false,
	})
	properties := map[string]any{}
	var required []string
	var targets []map[string]any
	for i, slot := range r.slots {
		unit := r.units[slot.Unit]
		bad := unit.NativeRefs[slot.Ref]
		key := fmt.Sprintf("s%d", i+1)
		properties[key] = map[string]any{"oneOf": alternatives}
		required = append(required, key)
		targets = append(targets, map[string]any{"slot": key, "finding": unit.Text, "rejected_ref": bad})
	}
	params, _ := json.Marshal(map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false})
	candidates, _ := json.Marshal(roster)
	invalid, _ := json.Marshal(targets)
	p.rowRepair = r
	p.schema = llm.ToolSchema{Name: name, Description: "Replace only rejected row addresses with exact current evidence.", Parameters: params}
	p.message = llm.Message{Role: "user", Content: "Select a current table and its original zero-based row for each rejected address. Use only evidence that supports the saved finding; never use a wakeup as evidence of a blocking mechanism. Choose no_match if no listed evidence supports that address. Existing findings, boundaries and every unaffected reference stay unchanged.\nRejected addresses: " + string(invalid) + "\nCurrent tables: " + string(candidates)}
	return p
}

func (r *explorerNativeRowRepair) expand(p *explorerMeasurementPatch, raw json.RawMessage) json.RawMessage {
	var values map[string]struct {
		Ref string `json:"ref"`
		Row *int   `json:"row"`
	}
	if json.Unmarshal(raw, &values) != nil || len(values) != len(r.slots) {
		return raw
	}
	for i := len(r.slots) - 1; i >= 0; i-- {
		slot := r.slots[i]
		edit := values[fmt.Sprintf("s%d", i+1)]
		unit := &r.units[slot.Unit]
		if edit.Ref == "no_match" && edit.Row == nil {
			unit.NativeRefs = append(unit.NativeRefs[:slot.Ref], unit.NativeRefs[slot.Ref+1:]...)
			const limit = "One requested evidence row had no verified current match."
			if !slices.Contains(unit.Boundaries, limit) {
				unit.Boundaries = append(unit.Boundaries, limit)
			}
			continue
		}
		table, known := r.tables[edit.Ref]
		if !known || edit.Row == nil || *edit.Row < 0 || *edit.Row >= len(table.Rows) {
			return raw
		}
		unit.NativeRefs[slot.Ref] = types.InvestigationNativeRowReference{
			ObservationID: table.ObservationID, View: table.View, RowIndices: []int{*edit.Row},
		}
	}
	handoff := maps.Clone(p.handoff)
	handoff["interpretation_units"], _ = json.Marshal(r.units)
	p.draft["native_fact_handoff"], _ = json.Marshal(handoff)
	out, _ := json.Marshal(p.draft)
	return out
}
