package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A rebased native trace starts at a real zero, not at the next positive
// event. Whole-artifact measurement and final-answer delivery must preserve
// that start without turning it into an explicitly requested time window.
func TestZeroOriginNativeTraceAccountReachesFinalizer(t *testing.T) {
	const trace = "idle-0 (0) [000] .... 0.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=41 next_prio=120\n" +
		"client-41 (41) [000] .... 0.002000: sched_switch: prev_comm=client prev_pid=41 prev_prio=120 prev_state=D ==> next_comm=idle next_pid=0 next_prio=120\n" +
		"irq-9 (9) [000] .... 0.004000: sched_wakeup: comm=client pid=41 prio=120 target_cpu=0\n" +
		"irq-9 (9) [000] .... 0.004001: sched_blocked_reason: pid=41 iowait=1 caller=io_wait_site\n" +
		"idle-0 (0) [000] .... 0.005000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=41 next_prio=120\n" +
		"client-41 (41) [000] .... 0.010000: sched_switch: prev_comm=client prev_pid=41 prev_prio=120 prev_state=R+ ==> next_comm=idle next_pid=0 next_prio=120\n"
	for _, view := range []string{"window_stats", "thread_timeline"} {
		t.Run(view, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "zero-origin.ftrace")
			if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 41})
			result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("public native query failed: %v; %s", err, result.Summary)
			}
			const request = "Report the scheduler states and IO waits of client-41 in the whole trace."
			for _, lang := range []string{"en", "zh"} {
				t.Run(lang, func(t *testing.T) {
					rm := types.RequestModel{
						RawRequest: request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, Language: lang,
						RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "client-41", Confidence: 1},
						RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "client-41", Source: "user_explicit", Confidence: 1}},
						RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeFullArtifact, SourceQuote: "the whole trace", Confidence: 1},
						RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
							FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences}, SourceQuote: "scheduler states and IO waits", Confidence: 1},
					}
					mu := types.NewMutableState(request)
					mu.SetRequestModel(rm)
					mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
					mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "answer", Kind: types.BlockSummary, Text: "Model-owned draft."}}})
					ctx := ctxbuilder.BuildAgentContext(&types.BusContext{
						RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: mu,
						AnalysisIR:               &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}},
						RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}},
					}, types.AgentFinalizer, types.StageFinalize)
					ledger := answerDocObservationLedger(ctx)
					before, _ := json.Marshal([]any{result, ledger, rm, mu.AnswerDocumentV2()})
					instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
					// The whole-artifact finite lane owns the native state
					// observation and its caliber guide. It need not mint an
					// explicit-window principal_state recap or a causal anchor.
					hmcAssertHandoffLine(t, instruction, "- `trace_query:",
						"claim=\"target_window_states:client-41\"", "value=\"10.000\"", "selected_window=0.000000..0.010000",
						"\"running=7.000\"", "\"runnable=1.000\"", "\"sleep=0.000\"", "\"d_state=0.000\"", "\"io_wait=2.000\"", "\"total=10.000\"")
					for _, want := range []string{"io_wait_occurrences=1", "wall_clock_sum=2.000ms", "0.002000..0.004000", types.TraceSchedulerWaitPartitionTeaching} {
						if !strings.Contains(instruction, want) {
							t.Errorf("zero-origin final input lost %q", want)
						}
					}
					if strings.Contains(instruction, "## Trace Decision Inputs (Model Owns The Conclusion)") {
						t.Error("a measured zero origin must not grant a causal report")
					}
					after, _ := json.Marshal([]any{result, answerDocObservationLedger(ctx), ctx.AnalysisIR.RequestModel, mu.AnswerDocumentV2()})
					if string(before) != string(after) || rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
						t.Fatal("measurement delivery changed the request range, native facts, or model answer")
					}
				})
			}
		})
	}
}
