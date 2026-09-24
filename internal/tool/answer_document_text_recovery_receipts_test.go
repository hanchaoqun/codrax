package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func textRecoveryReceiptFixture(t *testing.T) *types.AnswerSemanticView {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.systrace")
	const trace = "idle-0 (0) [000] .... 0.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=11 next_prio=120\n" +
		"worker-11 (11) [000] .... 1.000000: tracing_mark_write: B|11|Load image\n" +
		"worker-11 (11) [000] .... 1.001000: block_rq_issue: 8,0 R 4096 () 8 + 8 [worker]\n" +
		"irq-2 (2) [000] .... 1.005000: block_rq_complete: 8,0 R () 8 + 8 [0]\n" +
		"worker-11 (11) [000] .... 1.006000: tracing_mark_write: E|11\n" +
		"worker-11 (11) [000] .... 1.010000: sched_switch: prev_comm=worker prev_pid=11 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.01
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, RuntimeWorkRelationRequested: true},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart: &start, TimeEnd: &end, SourceQuote: "1..1.01 seconds", Confidence: 1}}
	mut := types.NewMutableState("Describe IO and observed business work within 1..1.01 seconds")
	mut.SetRequestModel(rm)
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "en", Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end})
	result, err := (&TraceQuery{}).Execute(bus, args)
	if err != nil || !result.Success {
		t.Fatalf("native fixture query failed: %v / %s", err, result.Summary)
	}
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if view == nil || len(view.RuntimeMeasurementContract.Choices()) != 3 || !view.RuntimeWorkRelationContract.Active() {
		t.Fatalf("native measurement/work prerequisites missing: %+v", view)
	}
	return view
}

func textRecoveryWithBadSibling(t *testing.T, blocks ...any) AnswerDocumentTextRecovery {
	t.Helper()
	blocks = append(blocks, map[string]any{"id": "bad-sibling", "kind": "not_a_kind", "text": "Malformed sibling prose survives."})
	raw, err := json.Marshal(map[string]any{"blocks": blocks})
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := RecoverAnswerDocumentV2FromText(string(raw))
	if !ok || rec.Document == nil || rec.Lossless {
		t.Fatalf("expected real lossy sibling salvage, got %+v / %t", rec, ok)
	}
	if blockByID(t, rec.Document, "bad-sibling").Text != "Malformed sibling prose survives." {
		t.Fatal("preserving selectors changed existing visible sibling salvage")
	}
	return rec
}

func TestRecoverAnswerDocumentTextKeepsNativeSelectorsWithBadSibling(t *testing.T) {
	view := textRecoveryReceiptFixture(t)
	var blocks []any
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		blocks = append(blocks, map[string]any{"id": string(table.View), "kind": "table", "runtime_measurement": map[string]any{
			"observation_id": table.ObservationID, "view": table.View,
			// JSON cannot supply the private binding, even on a salvage path.
			"bound_table": map[string]any{"Rows": [][]string{{"FORGED-NUMBER"}}}}})
	}
	work := view.RuntimeWorkRelationContract.Rows[0]
	blocks = append(blocks, map[string]any{"id": "work", "kind": "section", "runtime_work_relation": map[string]any{
		"observation_id": work.ObservationID, "conclusion": types.RuntimeWorkRelationConclusionRelationUnproven,
		"bound_row": map[string]any{"MeasuredDurationMS": 999}}})
	rec := textRecoveryWithBadSibling(t, blocks...)
	if len(rec.Document.Blocks) != 5 {
		t.Fatalf("bad sibling erased otherwise valid selectors: %+v", rec.Document.Blocks)
	}
	for _, block := range rec.Document.Blocks {
		if block.RuntimeMeasurement.IsBound() || block.RuntimeWorkRelation.IsBound() {
			t.Fatal("model JSON minted bound authority before current-evidence validation")
		}
	}
	if !types.RebindRuntimeAnswerReceipts(rec.Document, view) {
		t.Fatal("exact native selections did not rebind after lossy sibling salvage")
	}
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		block := blockByID(t, rec.Document, string(table.View))
		if !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(*block.RuntimeMeasurement.BoundTable, table) {
			t.Fatalf("recovered selector did not preserve native %s table", table.View)
		}
	}
	gotWork := blockByID(t, rec.Document, "work").RuntimeWorkRelation
	if !gotWork.IsBound() || !reflect.DeepEqual(gotWork.BoundRow, work) {
		t.Fatal("same-class work receipt was lost or replaced by model data")
	}
	visible := render.RenderAnswerDocument(rec.Document, "en")
	if strings.Contains(visible, "FORGED-NUMBER") || !strings.Contains(visible, "Actual start (s)") || !strings.Contains(visible, "Concurrent requests") {
		t.Fatalf("renderer lost native tables or trusted a model binding: %s", visible)
	}
}

func TestRecoverAnswerDocumentTextDoesNotUpgradeInvalidSelectors(t *testing.T) {
	view := textRecoveryReceiptFixture(t)
	id := view.RuntimeMeasurementContract.Choices()[0].ObservationID
	for _, fault := range []string{"kind", "text", "items", "columns", "diagram", "mixed_receipts", "bad_view", "empty_id", "bad_work_conclusion"} {
		t.Run(fault, func(t *testing.T) {
			selector := map[string]any{"observation_id": id, "view": "summary"}
			block := map[string]any{"id": "bad-selector", "kind": "table", "runtime_measurement": selector}
			switch fault {
			case "kind":
				block["kind"], block["text"] = "summary", "Model prose"
			case "text":
				block["text"] = "| Model value |\n|---|\n|999|"
			case "items":
				block["items"] = []any{map[string]any{"text": "999"}}
			case "columns":
				block["columns"] = []string{"Model column"}
			case "diagram":
				block["diagram"] = map[string]any{"kind": "flow", "language": "mermaid", "body": "graph TD\nA-->B"}
			case "mixed_receipts":
				block["runtime_work_relation"] = map[string]any{"observation_id": view.RuntimeWorkRelationContract.Rows[0].ObservationID, "conclusion": "relation_unproven"}
			case "bad_view":
				selector["view"] = "root_cause"
			case "empty_id":
				selector["observation_id"] = ""
			case "bad_work_conclusion":
				delete(block, "runtime_measurement")
				block["kind"], block["text"] = "section", "Unverified work explanation"
				block["runtime_work_relation"] = map[string]any{"observation_id": view.RuntimeWorkRelationContract.Rows[0].ObservationID, "conclusion": "fabricated_causality"}
			}
			rec := textRecoveryWithBadSibling(t, block)
			for _, recovered := range rec.Document.Blocks {
				if recovered.RuntimeMeasurement != nil || recovered.RuntimeWorkRelation != nil {
					t.Fatalf("invalid/competing carrier gained a selector via salvage: %+v", recovered)
				}
			}
		})
	}
}

func TestRecoverAnswerDocumentTextSelectorsStillRequireCurrentSupply(t *testing.T) {
	view := textRecoveryReceiptFixture(t)
	id := view.RuntimeMeasurementContract.Choices()[0].ObservationID
	for _, fault := range []string{"unknown_selector", "missing_supply", "unavailable_work_conclusion"} {
		t.Run(fault, func(t *testing.T) {
			block := map[string]any{"id": "selector", "kind": "table", "runtime_measurement": map[string]any{"observation_id": id, "view": "summary"}}
			current := view
			switch fault {
			case "unknown_selector":
				block["runtime_measurement"] = map[string]any{"observation_id": id + ":different-query", "view": "summary"}
			case "missing_supply":
				current = nil
			case "unavailable_work_conclusion":
				delete(block, "runtime_measurement")
				block["kind"] = "section"
				block["runtime_work_relation"] = map[string]any{"observation_id": view.RuntimeWorkRelationContract.Rows[0].ObservationID, "conclusion": "causal_contribution_supported"}
			}
			rec := textRecoveryWithBadSibling(t, block)
			selected := blockByID(t, rec.Document, "selector")
			if selected.RuntimeMeasurement == nil && selected.RuntimeWorkRelation == nil {
				t.Fatal("syntactically valid selection was dropped before supply validation")
			}
			if types.RebindRuntimeAnswerReceipts(rec.Document, current) {
				t.Fatal("salvage accepted an unknown or unauthorized current selection")
			}
			for _, recovered := range rec.Document.Blocks {
				if recovered.RuntimeMeasurement.IsBound() || recovered.RuntimeWorkRelation.IsBound() {
					t.Fatal("failed recovery left trusted bound data")
				}
			}
		})
	}
}
