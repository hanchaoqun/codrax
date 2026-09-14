package agent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1672Context(lang string) (*types.BusContext, *types.AgentContext, *answerDocumentEvaluator) {
	mu := types.NewMutableState("Explain the observed response delay")
	mu.SetTraceFindingContract(&types.TraceFindingContract{RootCauseReportEnabled: true, CandidateSetID: "b1672",
		Candidates: []types.TraceFindingCandidateV1{{PrimaryEligible: true, Decision: types.TraceCauseDecision{
			CandidateID: "candidate-wait", SubjectName: "worker-200",
			Token:           types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
			Magnitude:       &types.TypedMagnitude{Value: 12.4, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: "effective_attribution"},
			CausalQualifier: types.TraceCausalQualifierProven, EvidenceRefs: []string{"E-wait"},
		}}}})
	bus := &types.BusContext{Mutable: mu, Language: lang}
	ctx := promptcontext.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	e := &answerDocumentEvaluator{}
	e.BuildInitialInstruction(ctx, nil)
	return bus, ctx, e
}

func b1672Emit(t *testing.T, bus *types.BusContext, extra string) types.ToolResult {
	t.Helper()
	res, err := (&tool.EmitAnswerDocument{}).Execute(bus, json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"Original model conclusion, with its own uncertainty."}]`+extra+`}`))
	if err != nil || !res.Success {
		t.Fatalf("full emit premise failed: %v %+v", err, res)
	}
	return res
}

func b1672Snapshot(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestB1672OptionalRootSelectionPublicRoundAndRealSidecar(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, choice := range []string{"selected", "empty", "omitted", "invalid", "bad_patch"} {
			t.Run(lang+"/"+choice, func(t *testing.T) {
				bus, ctx, e := b1672Context(lang)
				res := b1672Emit(t, bus, "")
				if !strings.Contains(res.Summary, "trace_root_causes omitted") || len(res.OptionalCarrierOutcomes) != 0 {
					t.Fatal("legal omission must retain its plain note, not become a rejected carrier")
				}
				before := b1672Snapshot(t, bus.Mutable.AnswerDocumentV2())
				contract := b1672Snapshot(t, bus.Mutable.TraceFindingContract())
				sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, LastToolResult: &res})
				if !sig.HintRequested || sig.StopRequested || sig.HintKey != postEmitAdvisoryHintKey || !strings.Contains(sig.Hint, "replace_trace_root_causes") {
					t.Fatalf("accepted answer with no selection needs the one shared optional opportunity: %+v", sig)
				}
				if !sig.BypassBudget || e.retriesUsed != 0 || e.rejectHintsUsed != 0 || e.emitFullDocFailStreak != 0 {
					t.Fatal("optional choice was charged as a hard rejection")
				}
				if before != b1672Snapshot(t, bus.Mutable.AnswerDocumentV2()) || contract != b1672Snapshot(t, bus.Mutable.TraceFindingContract()) || bus.Mutable.TraceRootCauseReport() != nil {
					t.Fatal("advice changed model prose, facts or selected roots")
				}
				patch := `{"unchanged_block_ids":["summary"]`
				switch choice {
				case "selected":
					patch += `,"replace_trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"candidate-wait"}]}`
				case "empty":
					patch += `,"replace_trace_root_causes":{"schema_version":2,"root_causes":[]}`
				case "invalid":
					patch += `,"replace_trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"foreign"}]}`
				case "bad_patch":
					patch = `{"unchanged_block_ids":["missing"]`
				}
				patched, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(patch+`}`))
				if choice != "bad_patch" && (err != nil || !patched.Success) {
					t.Fatalf("optional selector rejected body: %v %+v", err, patched)
				}
				if choice == "bad_patch" && patched.Success {
					t.Fatal("invalid block ID unexpectedly passed")
				}
				stop := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, LastToolResult: &patched})
				if !stop.StopRequested || stop.HintRequested || e.retriesUsed != 0 || e.rejectHintsUsed != 0 {
					t.Fatalf("second optional round or hard rejection: %+v", stop)
				}
				if before != b1672Snapshot(t, bus.Mutable.AnswerDocumentV2()) {
					t.Fatal("root-only opportunity rewrote accepted model answer")
				}
				report := bus.Mutable.TraceRootCauseReport()
				available := choice == "selected" || choice == "empty"
				if (report != nil) != available {
					t.Fatalf("choice=%s report=%+v", choice, report)
				}
				reason := outputdump.RootCauseReasonValidSelectionUnavailable
				if bus.Mutable.TraceRootCauseSelectorRejected() {
					reason = outputdump.RootCauseReasonSelectionRejected
				}
				out := outputdump.WriteRootCauseOnly(outputdump.Args{Dir: t.TempDir(), Max: 10, HasTrace: true, RootCauseReport: report, RootCauseUnavailableReason: reason, Now: time.Unix(1700000000, 0), PID: 7})
				raw, err := os.ReadFile(out.RootCauseJSONPath)
				if err != nil {
					t.Fatal(err)
				}
				var file outputdump.DefaultRootCauseArtifact
				if err := json.Unmarshal(raw, &file); err != nil {
					t.Fatal(err)
				}
				if file.SchemaVersion != 2 || file.RootCauses == nil || (file.Status == "available") != available {
					t.Fatalf("dishonest JSON: %s", raw)
				}
				if !available && file.ReasonCode != reason {
					t.Fatalf("lost unavailable reason: %s", raw)
				}
				if choice == "selected" && (len(file.RootCauses) != 1 || file.RootCauses[0].ThreadName != "worker-200" || *file.RootCauses[0].ImpactSeconds != .0124) {
					t.Fatalf("selected facts changed: %s", raw)
				}
				if choice != "selected" && len(file.RootCauses) != 0 {
					t.Fatal("system fabricated a selection")
				}
			})
		}
	}
}

func TestB1672RootSelectionOpportunityPreciseNegativeStates(t *testing.T) {
	for _, state := range []string{"no_contract", "disabled", "no_candidates", "accepted", "accepted_empty", "pending", "pending_empty", "already_offered"} {
		t.Run(state, func(t *testing.T) {
			bus, ctx, e := b1672Context("en")
			b1672Emit(t, bus, "")
			switch state {
			case "no_contract":
				bus.Mutable.SetTraceFindingContract(nil)
			case "disabled":
				contract := bus.Mutable.TraceFindingContract()
				contract.RootCauseReportEnabled = false
				bus.Mutable.SetTraceFindingContract(contract)
			case "no_candidates":
				contract := bus.Mutable.TraceFindingContract()
				contract.Candidates = nil
				bus.Mutable.SetTraceFindingContract(contract)
			case "accepted", "accepted_empty", "pending", "pending_empty":
				extra := `,"trace_root_causes":{"schema_version":2,"root_causes":[]}`
				if state == "accepted" || state == "pending" {
					extra = `,"trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"candidate-wait"}]}`
				}
				b1672Emit(t, bus, extra)
				if strings.HasPrefix(state, "pending") {
					report := bus.Mutable.TraceRootCauseReport()
					bus.Mutable.SetTraceRootCauseReport(nil)
					bus.Mutable.SetPendingTraceRootCauseReport(report)
				}
			case "already_offered":
				e.postEmitAdvisoryDelivered = true
			}
			before := b1672Snapshot(t, []any{bus.Mutable.AnswerDocumentV2(), bus.Mutable.TraceRootCauseReport(), bus.Mutable.PendingTraceRootCauseReport()})
			sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
			if !sig.StopRequested || sig.HintRequested {
				t.Fatalf("negative state=%s invited new selection: %+v", state, sig)
			}
			if before != b1672Snapshot(t, []any{bus.Mutable.AnswerDocumentV2(), bus.Mutable.TraceRootCauseReport(), bus.Mutable.PendingTraceRootCauseReport()}) {
				t.Fatal("negative state mutated")
			}
		})
	}
}

func TestB1672RejectedFirstSelectorRetainsAnswerAndOffersOnlyOneChoice(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			bus, ctx, e := b1672Context(lang)
			res := b1672Emit(t, bus, `,"trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"foreign"}]}`)
			if !bus.Mutable.TraceRootCauseSelectorRejected() || bus.Mutable.TraceRootCauseReport() != nil || len(res.OptionalCarrierOutcomes) == 0 {
				t.Fatal("invalid selector must be disclosed without rejecting the accepted answer")
			}
			before := b1672Snapshot(t, bus.Mutable.AnswerDocumentV2())
			sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, LastToolResult: &res})
			state := "The previous root-cause selection was not accepted"
			if lang == "zh" {
				state = "之前提交的根因选择未被接收"
			}
			if !sig.HintRequested || sig.StopRequested || !strings.Contains(sig.Hint, state) {
				t.Fatalf("rejected selection must get accurate optional guidance: %+v", sig)
			}
			patched, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["summary"]}`))
			if err != nil || !patched.Success {
				t.Fatalf("declining optional selection failed: %v %+v", err, patched)
			}
			stop := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, LastToolResult: &patched})
			if !stop.StopRequested || stop.HintRequested || e.retriesUsed != 0 || e.rejectHintsUsed != 0 || before != b1672Snapshot(t, bus.Mutable.AnswerDocumentV2()) || bus.Mutable.TraceRootCauseReport() != nil {
				t.Fatal("declined optional recovery retried, chose roots or rewrote the accepted answer")
			}
		})
	}
}
