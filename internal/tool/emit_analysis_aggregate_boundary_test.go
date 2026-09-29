package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Replay the public emit shape that previously erased the count predicate and
// forced a repo_map-only probe for a directory census. The wording is not a gate.
func TestEmitAnalysisAggregateInputCoverageDoesNotBecomeRoster(t *testing.T) {
	for _, scalar := range []bool{true, false} {
		raw := "Count every source file under src, explain the measurement, and cite current source."
		var payload map[string]any
		if err := json.Unmarshal([]byte(withV4Required(`{"intent":"enumerate","scenario":"generic","complexity":"moderate","question_kind":"enumeration","keywords":["src"],"entities":["src"]}`)), &payload); err != nil {
			t.Fatal(err)
		}
		preds := payload["predicates"].(map[string]any)
		preds["is_scalar_answer"], preds["is_count_question"] = scalar, scalar
		preds["is_category_enumeration"], preds["has_per_member_table"] = false, false
		payload["completeness_obligation"] = map[string]any{"required": true, "source_quote": "every source file under src"}
		payload["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": 0.95, "dimensions": []any{
			map[string]any{"index": 1, "label": "Count", "role": "count", "source_quote": "Count every source file under src", "required": true},
			map[string]any{"index": 2, "label": "Explanation", "role": "function_or_purpose", "source_quote": "explain the measurement", "required": true},
			map[string]any{"index": 3, "label": "Source", "role": "source_location", "source_quote": "cite current source", "required": true},
		}}
		payload["source_inventory_profile"] = map[string]any{"is_source_inventory": true, "confidence": 0.95, "target_roles": []string{"file"}, "requested_fields": []string{"name", "location", "count"}, "source_quotes": []string{"every source file under src"}}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		ctx := &types.BusContext{Mutable: types.NewMutableState(raw)}
		result, err := (&EmitAnalysis{}).Execute(ctx, data)
		if err != nil || !result.Success {
			t.Fatalf("scalar=%v: %v %s", scalar, err, result.Summary)
		}
		rm := ctx.Mutable.RequestModel()
		if rm.Predicates.IsCountQuestion != scalar || !types.RequestsAggregateWithoutMemberRoster(*rm) || types.HasPrincipalAnswerSetObligation(*rm) || types.RequiresExhaustiveEnumerationMemberSetHandoff(*rm) || types.SourceInventoryPrincipalNavigationActive(*rm) {
			t.Fatalf("count transformed into an input roster: %+v", rm)
		}
		if !rm.CompletenessObligation.IsActive() || rm.RequestedAnswerDimensions == nil || len(rm.RequestedAnswerDimensions.Dimensions) != 3 {
			t.Fatal("coverage or independent explanation dimensions lost")
		}
	}
}

func TestCountReconciliationRequiresIndependentRoster(t *testing.T) {
	preds := types.SemanticPredicates{IsCountQuestion: true, IsScalarAnswer: true}
	for _, required := range []bool{false, true} {
		dims := &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionMemberSet, Required: required}}}
		got, _ := reconcileSetValuedCountPredicates(preds, dims)
		if got.IsCountQuestion == required || got.IsScalarAnswer == required {
			t.Fatalf("required=%t predicates=%+v", required, got)
		}
	}
}
