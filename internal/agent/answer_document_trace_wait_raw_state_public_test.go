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

// A statistical io_wait bucket does not tell the model whether the physical
// switch-out state was D or S. Exercise the native query and actual finalizer
// instruction, including rows beyond the eight-row compact tool preview.
func TestTraceWaitRawStatePublicFinalInstruction(t *testing.T) {
	customer, err := os.ReadFile("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	var native strings.Builder
	rawStates := []string{"D", "D|K", "D+", "D", "S", "D", "D|K", "D+", "D", "S"}
	for i, state := range rawStates {
		marker := 1
		if i%5 == 3 {
			marker = 0
		}
		native.WriteString(traceWaitRawStateAgentCycle(1+float64(i)*.005, state, marker))
	}
	cases := []struct {
		name, trace, subject string
		pid                  int
		start, end           float64
		raw                  []string
		d, io, sleep         int
		total                string
	}{
		{"ten_native_rows", native.String(), "reader-77", 77, 1, 1.049, rawStates, 2, 6, 2, "10.000"},
		{"customer_three_D_IO_rows", string(customer), "com.baidu.tieba-59566", 59566, 34579.45, 34579.6, []string{"D", "D", "D"}, 0, 3, 0, "0.635"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "native.systrace")
			if err := os.WriteFile(path, []byte(tc.trace), 0600); err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{
				"source": "path", "path": path, "view": "window_stats", "pid": tc.pid,
				"time_start": tc.start, "time_end": tc.end, "trace_flavor": "harmony_hitrace",
			})
			result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("native public query: %v; %s", err, result.Summary)
			}
			if len(tc.raw) > 8 && !strings.Contains(result.Summary, "status=incomplete account_status=complete emitted=8 total=10") {
				t.Fatal("fixture must exercise the compact preview limit while retaining a complete native roster")
			}
			for _, legacy := range []bool{false, true} {
				for _, lang := range []string{"zh", "en"} {
					t.Run(fmt.Sprintf("legacy=%t/%s", legacy, lang), func(t *testing.T) {
						// A restored old observation has no optional raw-state note.
						// Keep the real query's facts, scope, counters and payload intact.
						queryResult := result
						if legacy {
							queryResult.Observations = traceWaitRawStateLegacyObservations(result.Observations)
						}
						request := "Report the target scheduler states and individual waits."
						rm := types.RequestModel{
							RawRequest: request, Language: lang, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
							RuntimeTargets:       []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: tc.pid, Thread: tc.subject, Source: "user_explicit", Confidence: 1}},
							RuntimeTargetProfile: &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: tc.subject, Confidence: 1},
							RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
								TimeStart: &tc.start, TimeEnd: &tc.end, SourceQuote: fmt.Sprintf("%.6f..%.6f", tc.start, tc.end), Confidence: 1},
							RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
								FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences},
								SourceQuote:  "scheduler states and individual waits", Confidence: 1},
						}
						mu := types.NewMutableState(request)
						mu.SetRequestModel(rm)
						mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{queryResult}})
						const model = "Model-owned business finding. 模型自己的业务判断。"
						mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
							DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: model}},
						})
						bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: mu,
							AnalysisIR:               &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}},
							RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}},
						}
						ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
						ledger := answerDocObservationLedger(ctx)
						waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
						if len(waits) != 1 || waits[0].Count != len(tc.raw) || waits[0].DStateOccurrences != tc.d || waits[0].IOWaitOccurrences != tc.io ||
							waits[0].SleepIOWaitOccurrences != tc.sleep || fmt.Sprintf("%.3f", waits[0].WallClockMS) != tc.total ||
							waits[0].WindowStartTs != tc.start || waits[0].WindowEndTs != tc.end {
							t.Fatalf("native counts, duration, target or scope changed: %+v", waits)
						}
						before, _ := json.Marshal([]any{queryResult, ledger, types.CompileTraceCausalProjectionSet(ledger), rm, mu.AnswerDocumentV2()})
						instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
						prefix := "  - principal_occurrence=`"
						var displayed []string
						for _, line := range strings.Split(instruction, "\n") {
							if strings.HasPrefix(line, prefix) {
								displayed = append(displayed, strings.TrimSuffix(strings.TrimPrefix(line, prefix), "`"))
							}
						}
						if len(displayed) != len(tc.raw) {
							t.Fatalf("final instruction lost complete rows beyond preview: got %d want %d", len(displayed), len(tc.raw))
						}
						for i, occurrence := range waits[0].Occurrences {
							want := occurrence.CanonicalLine()
							if !legacy {
								want += " prev_state_raw=" + tc.raw[i]
							}
							if displayed[i] != want {
								t.Errorf("native physical state lost or invented at row %d: got %q want %q", i+1, displayed[i], want)
							}
						}
						wantCounts := fmt.Sprintf("occurrence_count=%d; d_state_occurrences=%d; io_wait_occurrences=%d; sleep_iowait_occurrences=%d; other_wait_occurrences=0; wall_clock_sum=%sms", len(tc.raw), tc.d, tc.io, tc.sleep, tc.total)
						if !strings.Contains(instruction, wantCounts) || !strings.Contains(instruction, "permission=`exact_complete_rowset`") {
							t.Fatalf("raw-state decoration replaced typed accounting: %s", wantCounts)
						}
						if len(tc.raw) <= 8 {
							// The small complete reader roster and the uncapped final
							// recap must teach the same two axes of the same fact.
							wantLabel := "original scheduler state: D; accounting category:"
							badTeaching := "Field names and machine status codes are validation metadata and must not appear in customer-facing prose."
							if lang == "zh" {
								wantLabel = "原始调度状态：D；统计类别："
								badTeaching = "字段名和机器状态码只用于内部校验，不要写入面向客户的正文。"
							}
							if strings.Contains(instruction, wantLabel) == legacy {
								t.Errorf("reader roster lost known raw state or invented an absent one: legacy=%t", legacy)
							}
							if strings.Contains(instruction, badTeaching) {
								t.Error("reader teaching forbids standard physical D/S states that the public answer must explain")
							}
						}
						afterLedger := answerDocObservationLedger(ctx)
						after, _ := json.Marshal([]any{queryResult, afterLedger, types.CompileTraceCausalProjectionSet(afterLedger), bus.AnalysisIR.RequestModel, mu.AnswerDocumentV2()})
						if string(before) != string(after) {
							t.Fatal("final instruction mutated native evidence, model content, scope or causal projection")
						}
					})
				}
			}
		})
	}
}

func traceWaitRawStateLegacyObservations(records []types.ObservationRecord) []types.ObservationRecord {
	out := append([]types.ObservationRecord(nil), records...)
	for i := range out {
		out[i].RichNotes = nil
		for _, note := range records[i].RichNotes {
			if strings.HasPrefix(note, "target_wait_occurrence_prev_state_raw=") {
				continue
			}
			if strings.HasPrefix(note, types.TraceNoteKeyTargetWaitOccurrence+"=") {
				note, _, _ = strings.Cut(note, " prev_state_raw=")
			}
			out[i].RichNotes = append(out[i].RichNotes, note)
		}
	}
	return out
}

func traceWaitRawStateAgentCycle(start float64, state string, marker int) string {
	return fmt.Sprintf(`idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=reader next_pid=77 next_prio=120
reader-77 (77) [000] .... %.6f: sched_switch: prev_comm=reader prev_pid=77 prev_prio=120 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120
irq-9 (9) [001] .... %.6f: sched_waking: comm=reader pid=77 prio=120 target_cpu=0
irq-9 (9) [001] .... %.6f: sched_blocked_reason: pid=77 iowait=%d caller=wait_site
idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=reader next_pid=77 next_prio=120
reader-77 (77) [000] .... %.6f: sched_switch: prev_comm=reader prev_pid=77 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`, start, start+.001, state, start+.002, start+.002001, marker, start+.003, start+.004)
}
