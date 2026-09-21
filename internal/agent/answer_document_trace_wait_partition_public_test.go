package agent

import (
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

// Use the real trace producer, not hand-authored io_wait observations: this
// bucket is a refinement of D, whereas the IO-marked S overlay stays sleep.
func TestTraceWaitPartitionPublicFinalInstruction(t *testing.T) {
	fixture, err := filepath.Abs("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, trace, subject string
		pid                  int
		start, end           float64
		d, io, sleep         int
		sum                  string
	}{
		{"customer_three_D_IO", string(data), "com.baidu.tieba-59566", 59566, 34579.45, 34579.6, 0, 3, 0, "0.635"},
		{"D_marked_IO", traceWaitPartitionRawCycle("D", 1), "reader-77", 77, 1, 1.004, 0, 1, 0, "1.000"},
		{"non_IO_D", traceWaitPartitionRawCycle("D", 0), "reader-77", 77, 1, 1.004, 1, 0, 0, "1.000"},
		{"S_marked_IO", traceWaitPartitionRawCycle("S", 1), "reader-77", 77, 1, 1.004, 0, 0, 1, "1.000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			params, _ := json.Marshal(map[string]any{
				"source": "attached_trace", "view": "window_stats", "pid": tc.pid,
				"time_start": tc.start, "time_end": tc.end, "trace_flavor": "harmony_hitrace",
			})
			queryBus := &types.BusContext{
				RepoRoot: dir, WorkDir: dir, AttachedHitrace: tc.trace, AttachedHitraceSource: "harmony_hitrace",
				RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: fixture, Carrier: "attachment"}}},
			}
			result, err := (&tool.TraceQuery{}).Execute(queryBus, params)
			if err != nil || !result.Success {
				t.Fatalf("public query: %v; %s", err, result.Summary)
			}
			if strings.Count(result.Summary, types.TraceSchedulerWaitPartitionTeaching) != 1 {
				t.Error("explorer tool preview must carry the same scheduler bucket definition exactly once")
			}
			for _, lang := range []string{"zh", "en"} {
				t.Run(lang, func(t *testing.T) {
					ctx := tracePrincipalValueAuthorityTestContext(tc.subject, tc.pid, result.Observations)
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
					rm := &ctx.AnalysisIR.RequestModel
					rm.Intent, rm.Language = types.IntentExplain, lang
					rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
						Scope:        types.RuntimeQuestionScopeBoundedFactSet,
						FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences},
					}
					rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
						RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &tc.start, TimeEnd: &tc.end,
						SourceQuote: fmt.Sprintf("%.6f..%.6f", tc.start, tc.end),
					}
					ctx.Mutable.SetRequestModel(*rm)
					ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
						DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model-owned conclusion, not a system diagnosis."}},
					})
					ctx = ctxbuilder.BuildAgentContext(&types.BusContext{
						RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR,
					}, types.AgentFinalizer, types.StageFinalize)
					ledger := answerDocObservationLedger(ctx)
					waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
					if len(waits) != 1 {
						t.Fatalf("expected complete same-window roster: %+v", waits)
					}
					wait := waits[0]
					if wait.Count != tc.d+tc.io+tc.sleep || wait.DStateOccurrences != tc.d || wait.IOWaitOccurrences != tc.io || wait.SleepIOWaitOccurrences != tc.sleep || fmt.Sprintf("%.3f", wait.WallClockMS) != tc.sum || wait.WindowStartTs != tc.start || wait.WindowEndTs != tc.end {
						t.Fatalf("actual producer classification/count/scope changed: %+v", wait)
					}
					before, _ := json.Marshal([]any{result, ledger, types.CompileTraceCausalProjectionSet(ledger), ctx.Mutable.AnswerDocumentV2()})
					instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
					for _, want := range []string{
						"## Runtime Trace Principal Values — Final Typed Recap",
						fmt.Sprintf("d_state_occurrences=%d; io_wait_occurrences=%d; sleep_iowait_occurrences=%d;", tc.d, tc.io, tc.sleep),
						"wall_clock_sum=" + tc.sum + "ms",
						types.FormatTargetStateAccountCaliber(lang),
						"A zero non-IO D bucket alone does not prove absence of D-state waiting",
						"remain interruptible S-state waiting, not D-state",
						"not device/request IO latency or independently completion-closed IO blocking",
					} {
						if !strings.Contains(instruction, want) {
							t.Errorf("actual finalizer lost consistent caliber %q", want)
						}
					}
					if strings.Contains(instruction, "Do not rename an `io_wait` row to D-state") {
						t.Error("finalizer contradicts real D-opened IO producer and existing uninterruptible fold")
					}
					afterLedger := answerDocObservationLedger(ctx)
					after, _ := json.Marshal([]any{result, afterLedger, types.CompileTraceCausalProjectionSet(afterLedger), ctx.Mutable.AnswerDocumentV2()})
					if string(before) != string(after) {
						t.Fatal("caliber teaching mutated physical facts, causal projection, or model document")
					}
				})
			}
		})
	}
}

func traceWaitPartitionRawCycle(state string, marker int) string {
	return fmt.Sprintf(`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=reader next_pid=77 next_prio=120
reader-77 (77) [000] .... 1.001000: sched_switch: prev_comm=reader prev_pid=77 prev_prio=120 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120
irq-9 (9) [001] .... 1.002000: sched_waking: comm=reader pid=77 prio=120 target_cpu=0
irq-9 (9) [001] .... 1.002001: sched_blocked_reason: pid=77 iowait=%d caller=wait_site
idle-0 (0) [000] .... 1.003000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=reader next_pid=77 next_prio=120
reader-77 (77) [000] .... 1.004000: sched_switch: prev_comm=reader prev_pid=77 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`, state, marker)
}
