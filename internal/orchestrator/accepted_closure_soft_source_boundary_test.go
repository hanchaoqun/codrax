package orchestrator

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Every positive receipt below originates in the public Run fixture's real
// completion tool. Mutations represent a later exact boundary, not a way of
// manufacturing the accepted state whose bug is under test.
func TestAcceptedClosureSoftSourceReceiptBoundaries(t *testing.T) {
	for _, name := range []string{"missing_receipt", "other_lane", "reset", "precise_source", "source_landed", "fresh_run", "no_runtime", "pending_read", "backtrack", "strict", "other_origin"} {
		t.Run(name, func(t *testing.T) {
			got := runSoftSourcePublic(t, false, "")
			if got.err != nil || got.calls != 1 {
				t.Fatalf("real accepted closure prerequisite failed: %+v", got)
			}
			o := got.orchestrator
			bus, mut := o.busCtx, o.busCtx.Mutable
			if !o.acceptedSoftCurrentSourceCompletionReceipt() {
				t.Fatal("real Run did not preserve its accepted soft-source receipt")
			}
			wantReceipt, wantConsumers := false, false
			checkConsumers := true
			switch name {
			case "missing_receipt", "other_lane":
				mut.EvidenceClosure().ClearCompletionCaveat(types.DowngradeLaneCurrentSourceLane)
				if name == "other_lane" {
					mut.EvidenceClosure().AppendCompletionCaveat(types.CompletionCaveat{Lane: types.DowngradeLaneCompletionForm, ReasonCode: types.ProgressReasonConverged})
				}
			case "reset":
				mut.ResetInvestigationComplete()
			case "precise_source":
				rm := *mut.RequestModel()
				rm.AnalyzerHints.ExactTargets = []string{"main.go"}
				mut.SetRequestModel(rm)
				bus.AnalysisIR.RequestModel = rm
			case "source_landed":
				result, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"main.go"}`))
				if err != nil || !result.Success {
					t.Fatalf("public source read failed: %v / %s", err, result.Summary)
				}
				mut.AppendDispatchToolResult(result)
				ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
				if !types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(bus, ledger).CurrentSourceSatisfied {
					t.Fatal("source control did not land a real current-source witness")
				}
				wantConsumers = true // normal proof, not a missing-source waiver
			case "fresh_run":
				rm := *mut.RequestModel()
				bus.Mutable = types.NewMutableState(rm.RawRequest)
				bus.Mutable.SetRequestModel(rm)
			case "no_runtime":
				// Deliberately forged/legacy empty receipt is a NEGATIVE control.
				// It must not turn the mere completion flag into runtime proof.
				rm := *mut.RequestModel()
				rm.PerfTrace, rm.LogTriage = nil, nil
				empty := types.NewMutableState(rm.RawRequest)
				empty.SetRequestModel(rm)
				empty.SetInvestigationComplete("empty legacy receipt")
				empty.EvidenceClosure().AppendCompletionCaveat(types.CompletionCaveat{Lane: types.DowngradeLaneCurrentSourceLane, ReasonCode: types.ProgressReasonConverged})
				bus.Mutable, bus.ToolResults = empty, nil
				bus.AttachedHitrace, bus.AttachedLog = "", ""
				bus.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{}
				bus.AnalysisIR.RequestModel = rm
				// With no runtime contract there is no mixed-origin gate to
				// exercise. Preserve that existing policy; test only that this
				// new receipt arm cannot grant an empty runtime waiver.
				checkConsumers = false
			case "pending_read":
				mut.EvidenceClosure().AddPendingRead(types.PendingRead{File: "main.go", Origin: "required_file", Rationale: "exact outstanding source obligation"})
				wantReceipt = true // a source-lane receipt cannot consume another gate
			case "backtrack":
				mut.SetRetryState(&types.RetryState{LastPrimaryOwner: string(LocusExplore), ActiveViolations: []types.ScoredViolation{{Kind: types.ViolRequiredDiagramEdgeAbsent, Severity: types.SeverityHigh}}})
				mut.ResetForFallback(types.FallbackResetTargetExplore)
				mut.ResetInvestigationComplete()
			case "strict":
				o.settings.Agent.InvestigationCompletePolicy = types.ICPolicyStrict
				wantReceipt = true
			case "other_origin":
				wantReceipt, wantConsumers = true, true
				required := []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginCurrentSource, types.AnswerEvidenceOriginRuntimeArtifact, types.AnswerEvidenceOriginExternalDocument}
				want := []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginRuntimeArtifact, types.AnswerEvidenceOriginExternalDocument}
				for label, filter := range map[string]func([]types.AnswerEvidenceOrigin) []types.AnswerEvidenceOrigin{
					"pre_mint":    o.withholdWaivedCurrentSourceOriginLaneBeforeDebtMint,
					"post_filter": o.dropWaivedCurrentSourceOriginDebt,
				} {
					if actual := filter(append([]types.AnswerEvidenceOrigin(nil), required...)); !reflect.DeepEqual(actual, want) {
						t.Errorf("%s dropped an unrelated origin: %v", label, actual)
					}
				}
			}
			if actual := o.acceptedSoftCurrentSourceCompletionReceipt(); actual != wantReceipt {
				t.Errorf("receipt=%t, want %t", actual, wantReceipt)
			}
			if !checkConsumers {
				return
			}
			if actual := o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", ""); actual != wantConsumers {
				t.Errorf("explore auto-complete=%t, want %t", actual, wantConsumers)
			}
			if actual := o.acceptedClosureCanSatisfyReconcileEnoughFacts(); actual != wantConsumers {
				t.Errorf("reconcile auto-complete=%t, want %t", actual, wantConsumers)
			}
		})
	}
}
