package orchestrator

import (
	"context"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/render"
	answertool "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const receiptRecoveryTrace = "idle-0 (0) [000] .... 0.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=11 next_prio=120\n" +
	"worker-11 (11) [000] .... 1.000000: tracing_mark_write: B|11|Load image\n" +
	"worker-11 (11) [000] .... 1.001000: block_rq_issue: 8,0 R 4096 () 8 + 8 [worker]\n" +
	"irq-2 (2) [000] .... 1.005000: block_rq_complete: 8,0 R () 8 + 8 [0]\n" +
	"worker-11 (11) [000] .... 1.006000: tracing_mark_write: E|11\n" +
	"worker-11 (11) [000] .... 1.010000: sched_switch: prev_comm=worker prev_pid=11 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"

func nativeReceiptRecoveryFixture(t *testing.T) (*Orchestrator, *types.AnswerDocumentV2, types.ToolResult, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.systrace")
	if err := os.WriteFile(path, []byte(receiptRecoveryTrace), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.01
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure, types.RuntimeQuestionFactOtherObservedValue}, RuntimeWorkRelationRequested: true},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart: &start, TimeEnd: &end, SourceQuote: "1..1.01 seconds", Confidence: 1},
	}
	mut := types.NewMutableState("Describe IO measurements and the observed business work within 1..1.01 seconds")
	mut.SetRequestModel(rm)
	bus := &types.BusContext{Ctx: context.Background(), RepoRoot: dir, WorkDir: dir, Language: "en", Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}}
	o := &Orchestrator{busCtx: bus}
	result := nativeReceiptRecoveryQuery(t, bus, path, start, end)
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if view == nil || !view.RuntimeMeasurementContract.Active() || !view.RuntimeWorkRelationContract.Active() {
		t.Fatalf("native provider prerequisites unavailable: %+v", view)
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "lead", Kind: types.BlockSummary, Text: "Original measured answer."}}}
	for _, choice := range view.RuntimeMeasurementContract.Choices() {
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: string(choice.View), Kind: types.BlockTable,
			RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: choice.ObservationID, View: choice.View}})
	}
	row := view.RuntimeWorkRelationContract.Rows[0]
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "work", Kind: types.BlockSection,
		RuntimeWorkRelation: &types.AnswerRuntimeWorkRelationReceipt{ObservationID: row.ObservationID, Conclusion: types.RuntimeWorkRelationConclusionRelationUnproven}})
	if !types.RebindRuntimeAnswerReceipts(doc, view) {
		t.Fatal("native selections could not bind")
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	return o, doc, result, path
}

func nativeReceiptRecoveryQuery(t *testing.T, bus *types.BusContext, path string, start, end float64) types.ToolResult {
	t.Helper()
	args, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&answertool.TraceQuery{}).Execute(bus, args)
	if err != nil || !result.Success {
		t.Fatalf("native query failed: %v / %s", err, result.Summary)
	}
	return result
}

func assertRecoveredRuntimeReceipts(t *testing.T, got, want *types.AnswerDocumentV2, visible string) {
	t.Helper()
	if got == nil || len(got.Blocks) != len(want.Blocks) {
		t.Fatalf("restored document shape changed: %+v", got)
	}
	visible = html.UnescapeString(visible)
	for i, block := range want.Blocks {
		if block.RuntimeMeasurement != nil {
			if !reflect.DeepEqual(got.Blocks[i].RuntimeMeasurement, block.RuntimeMeasurement) {
				t.Fatalf("restored measurement differs: %+v", got.Blocks[i])
			}
			for _, row := range block.RuntimeMeasurement.BoundTable.Rows {
				for _, cell := range row {
					if cell != "" && !strings.Contains(visible, cell) {
						t.Fatalf("restored answer lost trusted cell %q:\n%s", cell, visible)
					}
				}
			}
		}
		if block.RuntimeWorkRelation != nil {
			if !reflect.DeepEqual(got.Blocks[i].RuntimeWorkRelation, block.RuntimeWorkRelation) || !strings.Contains(visible, "Load image") {
				t.Fatalf("restored work measurement lost or changed: %+v\n%s", got.Blocks[i], visible)
			}
		}
	}
}

func TestRuntimeReceiptRecoveryRecordRestoreRender(t *testing.T) {
	o, original, _, _ := nativeReceiptRecoveryFixture(t)
	mut := o.busCtx.Mutable
	ledger := recordFinalizeRepairDraft(nil, &agent.StageOutput{FinalAnswer: render.RenderAnswerDocument(original, "en")}, mut, nil, 0, 2)
	if len(ledger) != 1 {
		t.Fatal("draft not recorded")
	}
	var wire types.AnswerDocumentV2
	if err := json.Unmarshal(ledger[0].DocJSON, &wire); err != nil {
		t.Fatal(err)
	}
	for _, block := range wire.Blocks {
		if block.RuntimeMeasurement.IsBound() || block.RuntimeWorkRelation.IsBound() {
			t.Fatal("private bound facts crossed the answer JSON boundary")
		}
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "later", Kind: types.BlockSummary, Text: "Later answer."}}})
	out := &agent.StageOutput{FinalAnswer: "Later answer."}
	if !o.restoreFinalizeRepairDraft(out, &ledger[0]) {
		t.Fatal("current native evidence did not restore saved selectors")
	}
	restored := mut.AnswerDocumentV2()
	assertRecoveredRuntimeReceipts(t, restored, original, out.FinalAnswer)
	// Neither a caller's restored copy nor the provider view can mutate the
	// accepted record; retrying the JSON snapshot must reconstruct fresh data.
	for i := range restored.Blocks {
		if r := restored.Blocks[i].RuntimeMeasurement; r != nil {
			r.BoundTable.Rows[0][0] = "corrupted reader copy"
		}
		if r := restored.Blocks[i].RuntimeWorkRelation; r != nil {
			r.BoundRow.AllowedConclusions[0] = types.RuntimeWorkRelationConclusionCausalContributionSupported
		}
	}
	assertRecoveredRuntimeReceipts(t, mut.AnswerDocumentV2(), original, out.FinalAnswer)
	if !o.restoreFinalizeRepairDraft(out, &ledger[0]) {
		t.Fatal("repeat restore failed")
	}
	assertRecoveredRuntimeReceipts(t, mut.AnswerDocumentV2(), original, out.FinalAnswer)
}

func TestRuntimeReceiptRecoveryStaleSupplyKeepsAcceptedDraft(t *testing.T) {
	for _, change := range []string{"missing", "different_source", "different_window", "changed_request_window", "work_only_missing"} {
		t.Run(change, func(t *testing.T) {
			o, original, _, path := nativeReceiptRecoveryFixture(t)
			mut := o.busCtx.Mutable
			if change == "work_only_missing" {
				original.Blocks = []types.AnswerBlock{original.Blocks[len(original.Blocks)-1]}
				mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, original)
			}
			ledger := recordFinalizeRepairDraft(nil, &agent.StageOutput{FinalAnswer: "STALE recorded rendered values"}, mut, nil, 0, 2)
			switch change {
			case "missing", "work_only_missing":
				mut.SetTurnAArtifacts(types.TurnAArtifacts{})
			case "different_source":
				other := filepath.Join(filepath.Dir(path), "other.systrace")
				if err := os.WriteFile(other, []byte(receiptRecoveryTrace), 0600); err != nil {
					t.Fatal(err)
				}
				result := nativeReceiptRecoveryQuery(t, o.busCtx, other, 1, 1.01)
				mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			case "different_window":
				result := nativeReceiptRecoveryQuery(t, o.busCtx, path, 1, 1.008)
				mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			case "changed_request_window":
				rm := o.busCtx.AnalysisIR.RequestModel
				start, end := 2.0, 3.0
				rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
					TimeStart: &start, TimeEnd: &end, SourceQuote: "2..3 seconds", Confidence: 1}
				o.busCtx.AnalysisIR.RequestModel = rm
				mut.SetRequestModel(rm)
			}
			current := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "current", Kind: types.BlockSummary, Text: "Keep the accepted answer."}}}
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, current)
			before := mut.AnswerDocumentV2()
			out := &agent.StageOutput{FinalAnswer: "Current visible answer", Data: json.RawMessage("{\"kept\":true}")}
			outBefore := *out
			if o.restoreFinalizeRepairDraft(out, &ledger[0]) {
				t.Fatal("stale saved selector restored against changed/missing evidence")
			}
			if !reflect.DeepEqual(before, mut.AnswerDocumentV2()) || !reflect.DeepEqual(outBefore, *out) {
				t.Fatal("failed restore replaced accepted document or shipped old rendered text")
			}
		})
	}
}

func TestRuntimeReceiptRecoveryRetryStateAndRejectedCandidates(t *testing.T) {
	o, original, _, _ := nativeReceiptRecoveryFixture(t)
	mut := o.busCtx.Mutable
	populateRetryState(mut, contract.Result{}, 0)
	mut.ResetAnswerDocumentV2()
	doc, source := o.finalizerRecoveryDraftCandidate()
	if doc == nil || source != "retry_state" {
		t.Fatalf("saved retry candidate not recovered: %s", source)
	}
	assertRecoveredRuntimeReceipts(t, doc, original, render.RenderAnswerDocument(doc, "en"))
	// Rejected memory copies must also be checked against the current supply,
	// not trusted merely because their former BoundTable survived in memory.
	mut.SetLastRejectedAnswerDocumentV2(original)
	mut.SetTurnAArtifacts(types.TurnAArtifacts{})
	if doc, source := o.finalizerRecoveryDraftCandidate(); doc != nil || source != "" {
		t.Fatalf("stale rejected/retry draft became recovery candidate: %s", source)
	}
	current := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "current", Kind: types.BlockSummary, Text: "Keep current answer."}}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, current)
	if doc, source := o.finalizerRecoveryDraftCandidate(); source != "accepted" || !reflect.DeepEqual(doc, current) {
		t.Fatalf("invalid earlier candidates hid current accepted answer: %s / %+v", source, doc)
	}
	if !reflect.DeepEqual(mut.AnswerDocumentV2(), current) {
		t.Fatal("candidate selection mutated the accepted document")
	}
}
