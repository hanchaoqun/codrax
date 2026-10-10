package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A displayed identity does not replace a carrier's richer bounded values.
// Drive the actual producer and adapter: eleven complete waits produce an
// eight-row preview while retaining the full independent occurrence authority.
func TestHMC218TraceWaitCarrierActualFinalizerMessages(t *testing.T) {
	var trace strings.Builder
	for i := 0; i < 11; i++ {
		trace.WriteString(traceWaitRawStateAgentCycle(1+float64(i)*.005, "D", 1))
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "eleven-waits.systrace")
	if err := os.WriteFile(path, []byte(trace.String()), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.054
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 77,
		"time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, args)
	if err != nil || !result.Success {
		t.Fatalf("real native query failed: %v / %s", err, result.Summary)
	}
	var set types.ObservationRecord
	for _, row := range result.Observations {
		if row.Predicate == "target_window_wait_occurrences" {
			set = row
		}
	}
	if set.ID == "" || set.SourceRef.QueryScopeID == "" || set.SourceRef.PayloadRef == "" {
		t.Fatalf("fixture lacks genuine scoped native presentation receipt: %+v", set)
	}
	notes := targetWaitOccurrenceHandoffNotes(set.RichNotes)
	if len(notes) != 10 || notes[0] != "target_wait_occurrence_prompt=status=incomplete,emitted=8,total=11" ||
		notes[1] != "target_wait_occurrence_prompt_sum_ms=8.000" {
		t.Fatalf("fixture must exercise bounded values distinct from complete native authority: %q", notes)
	}
	result = types.AttachToolHandoffCarrier(result)
	for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeBoundedFactSet, types.RuntimeQuestionScopeCausalDiagnosis} {
		t.Run(string(scope), func(t *testing.T) {
			request := "请分析 reader-77 在 1.000 到 1.054 秒期间的等待。"
			rm := types.RequestModel{RawRequest: request, Language: "zh", Intent: types.IntentTrace, Scenario: types.ScenarioGeneric,
				RuntimeTargets:       []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 77, Thread: "reader-77", Source: "user_explicit", Confidence: 1}},
				RuntimeTargetProfile: &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "reader-77", Confidence: 1},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
					TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000..1.054", Confidence: 1},
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: scope, Confidence: 1,
					FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences}},
			}
			mu := types.NewMutableState(request)
			mu.SetRequestModel(rm)
			results := []types.ToolResult{result}
			mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
			bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "zh", Mutable: mu,
				AnalysisIR:               &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}},
				RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}},
			}
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			if !answerDocPresentedNativeObservationIDs(ctx)[set.ID] {
				t.Fatal("fixture does not exercise an already-displayed native identity with dedicated carrier values")
			}
			ledger := answerDocObservationLedger(ctx)
			var normalized types.ObservationRecord
			for _, row := range ledger.Records {
				if row.ID == set.ID {
					normalized = row
				}
			}
			if normalized.ClaimAuthority != types.ObservationClaimAuthorityDirectObservation || !types.IsNativeRuntimeFactPresentationRecord(normalized) {
				t.Fatalf("native query lost direct authority at the ledger boundary: %+v", normalized)
			}
			waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &rm)
			if len(waits) != 1 || waits[0].Count != 11 || len(waits[0].Occurrences) != 11 ||
				waits[0].IOWaitOccurrences != 11 || waits[0].WindowStartTs != start || waits[0].WindowEndTs != end ||
				fmt.Sprintf("%.3f", waits[0].WallClockMS) != "11.000" {
				t.Fatalf("complete native query authority changed: %+v", waits)
			}
			before := hmc218CausalProjection(t, ctx)
			messages := dependencyObservationMessages(t, ctx)
			for _, note := range notes {
				if !strings.Contains(messages, fmt.Sprintf("observation_value=%q", note)) {
					t.Errorf("already-published ID incorrectly erased dedicated carrier value: %q", note)
				}
			}
			for _, row := range waits[0].Occurrences {
				if !strings.Contains(messages, row.CanonicalLine()+" prev_state_raw=D") {
					t.Errorf("complete native wait beyond bounded preview missing from actual adapter: %s", row.CanonicalLine())
				}
			}
			if !bytes.Equal(before, hmc218CausalProjection(t, ctx)) || mu.TraceRootCauseReport() != nil {
				t.Fatal("presentation changed causal projection or minted a model-owned conclusion")
			}
		})
	}
}
