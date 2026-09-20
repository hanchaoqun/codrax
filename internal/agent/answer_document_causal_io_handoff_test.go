package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCausalIOHandoffRequestWitnessesSurviveAggregateGrowth(t *testing.T) {
	rm := &types.RequestModel{RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}}
	var records []types.ObservationRecord
	for i := 0; i < 20; i++ {
		records = append(records, types.ObservationRecord{
			ID: fmt.Sprintf("aggregate-%d", i), Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "storage_latency_by_layer", Subject: "block",
			Summary: "IO request distribution", RichNotes: []string{"dev=8,0"},
		})
	}
	coverage := records[0]
	coverage.ID, coverage.Predicate = "coverage", "io_latency_coverage"
	records = append(records, coverage)
	for i := 0; i < 2; i++ {
		request := records[0]
		request.ID, request.Predicate, request.Subject = fmt.Sprintf("request-window-%d", i), "io_latency", "worker-200"
		request.Summary = "A source-bound request, not a causal election"
		request.SourceRef.QueryWindowKnown = true
		request.SourceRef.QueryWindowStartTs = float64(i + 1)
		request.SourceRef.QueryWindowEndTs = float64(i + 2)
		records = append(records, request)
	}
	before, _ := json.Marshal(records)
	selected := answerDocRuntimeFactAuthorityRowsByKey(records, types.RuntimeQuestionFactIOLatency, rm, answerDocCausalIOScopedKey)
	if len(selected) != 10 {
		t.Fatalf("context budget changed: %d", len(selected))
	}
	counts := map[string]int{}
	for _, row := range selected {
		counts[row.Predicate]++
		if row.Role != types.AnswerAggregateRoleSupportingCoverage {
			t.Fatalf("display selection promoted a measurement: %+v", row)
		}
	}
	if counts["io_latency"] != 2 || counts["io_latency_coverage"] != 1 || counts["storage_latency_by_layer"] == 0 {
		t.Fatalf("aggregate growth crowded out requests or their coverage: %+v", counts)
	}
	after, _ := json.Marshal(records)
	if string(before) != string(after) {
		t.Fatal("display selection mutated source facts")
	}
}

func causalIOInstructionContext(ctx *types.AgentContext) {
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	ctx.AnalysisIR.RequestModel.PerfTrace = &types.PerfBundle{}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
}

func TestCausalIOHandoffKeepsClosureAuthorityAndOriginalEvidence(t *testing.T) {
	for _, family := range []string{"block_rq", "block_bio"} {
		for _, closure := range []string{"proven", "absent_wakeup", "different_waker", "missing_blocking_transition"} {
			t.Run(family+"/"+closure, func(t *testing.T) {
				ctx := b1644IOCompletionContext(t, family, closure, "en")
				causalIOInstructionContext(ctx)
				ledger := answerDocObservationLedger(ctx)
				before, _ := json.Marshal([]any{ledger, types.CompileTraceCausalProjectionSet(ledger), ctx.Mutable.AnswerDocumentV2()})
				authorities := types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if strings.Count(prompt, "### IO Measurements For Causal Interpretation") != 1 {
					t.Fatal("causal IO source handoff missing or duplicated")
				}
				line := b1644IOAuditLine(prompt)
				for _, want := range []string{"subject=`target-41`", "owner_scope=`target_owned`", "source_path=", "selected_window=", "trace_query:"} {
					if !strings.Contains(line, want) {
						t.Errorf("source-bound IO row missing %q: %s", want, line)
					}
				}
				if closure == "proven" {
					if !strings.Contains(line, "completion_woke_issuer=`true`") || !strings.Contains(line, "issuer_blocked=`0.090`") {
						t.Fatal("proven S-state IO wait was lost")
					}
				} else if !strings.Contains(line, "completion_woke_issuer=`false`") || strings.Contains(line, "issuer_blocked=`") ||
					!strings.Contains(line, "does not prove that no wakeup occurred") {
					t.Fatalf("missing completion closure became a proven wait or negative wakeup claim: %s", line)
				}
				afterLedger := answerDocObservationLedger(ctx)
				after, _ := json.Marshal([]any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger), ctx.Mutable.AnswerDocumentV2()})
				if string(before) != string(after) || !reflect.DeepEqual(authorities, types.BuildTraceBlockingWallClockAuthorities(afterLedger, &ctx.AnalysisIR.RequestModel)) {
					t.Fatal("IO handoff changed source evidence, root-cause projection, closure authority, or model answer")
				}
			})
		}
	}
}

func TestCausalIOHandoffPreservesBackgroundAndExplicitWindow(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "app-main", Source: "user_explicit"}}
	start, end := 0.998, 1.052
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "0.998..1.052",
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	var background string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.Contains(line, "subject=`backup-900`") && strings.Contains(line, "request_residence=`47.000`") {
			background = line
		}
	}
	if background == "" || !strings.Contains(background, "owner_scope=`selected_window_context`") ||
		strings.Contains(background, "owner_scope=`target_owned`") || strings.Contains(background, "issuer_blocked=`") {
		t.Fatalf("long background request disappeared or acquired target wait ownership: %s", background)
	}
	if !strings.Contains(prompt, "unless independent dependency evidence places that issuer's wait on the target's causal chain") {
		t.Fatal("same-window context cannot gain causal attribution without an independent chain")
	}
	// Change only the user's exact requested window. The producer's older,
	// wider query remains in the audit ledger but must not bypass the existing
	// finalizer window filter through the newly added handoff.
	start, end = 1.001, 1.046
	outOfScope := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(outOfScope, "### IO Measurements For Causal Interpretation") {
		filtered, _ := answerDocSelectedWindowObservationRecords(ctx, answerDocObservationLedger(ctx).Records)
		t.Fatalf("causal IO handoff reintroduced a wider query outside the explicit requested window: %s", renderAnswerDocCausalIOMeasurements(ctx, types.ObservationLedger{Records: filtered}))
	}
	if len(answerDocObservationLedger(ctx).Records) == 0 {
		t.Fatal("window display filtering destroyed the accepted audit ledger")
	}
}

func TestCausalIOHandoffLeavesFiniteFactContractUnchanged(t *testing.T) {
	for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeBoundedFactSet, types.RuntimeQuestionScopeBoundedEffectVerdict} {
		ctx := b1644IOCompletionContext(t, "block_rq", "proven", "en")
		ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = scope
		ctx.AnalysisIR.RequestModel.PerfTrace = &types.PerfBundle{}
		prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
		if strings.Contains(prompt, "### IO Measurements For Causal Interpretation") {
			t.Fatal("causal handoff duplicated the existing finite fact lane")
		}
		if !strings.Contains(prompt, "### Requested Runtime Fact Authority") || b1644IOAuditLine(prompt) == "" {
			t.Fatal("existing finite IO fact contract was removed")
		}
	}
}
