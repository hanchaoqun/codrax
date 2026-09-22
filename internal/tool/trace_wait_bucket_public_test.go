package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Follow native events through both public emit paths. Prompt-only tests do
// not see the system-owned state/wait appendix that is added at publication.
func TestTraceWaitBucketPublicQueryEmitRenderPatch(t *testing.T) {
	customer, err := os.ReadFile("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	var many strings.Builder
	for i := 0; i < 11; i++ {
		many.WriteString(traceWaitBucketNativeCycle(1+float64(i)*.01, "D", 1))
	}
	cases := []struct {
		name, trace, subject, view string
		pid                        int
		start, end                 float64
		full                       bool
		d, io, sleep               int
		total                      string
		rawState                   string
	}{
		{"zero_origin_D_IO", traceWaitBucketNativeCycle(0, "D", 1), "reader-77", "thread_timeline", 77, 0, .004, true, 0, 1, 0, "1.000", "D"},
		{"D_IO", traceWaitBucketNativeCycle(1, "D", 1), "reader-77", "window_stats", 77, 1, 1.004, false, 0, 1, 0, "1.000", "D"},
		{"D_variant_IO", traceWaitBucketNativeCycle(1, "D|K", 1), "reader-77", "window_stats", 77, 1, 1.004, false, 0, 1, 0, "1.000", "D|K"},
		{"D_unmarked", traceWaitBucketNativeCycle(1, "D", 0), "reader-77", "window_stats", 77, 1, 1.004, false, 1, 0, 0, "1.000", "D"},
		{"S_IO", traceWaitBucketNativeCycle(1, "S", 1), "reader-77", "window_stats", 77, 1, 1.004, false, 0, 0, 1, "1.000", "S"},
		{"customer_D_IO", string(customer), "com.baidu.tieba-59566", "window_stats", 59566, 34579.45, 34579.6, false, 0, 3, 0, "0.635", "D"},
		{"eleven_D_IO_beyond_preview", many.String(), "reader-77", "window_stats", 77, 1, 1.104, false, 0, 11, 0, "11.000", "D"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "native.systrace")
			if err := os.WriteFile(path, []byte(tc.trace), 0600); err != nil {
				t.Fatal(err)
			}
			params := map[string]any{"source": "path", "path": path, "view": tc.view, "pid": tc.pid, "trace_flavor": "harmony_hitrace"}
			if !tc.full {
				params["time_start"], params["time_end"] = tc.start, tc.end
			}
			raw, _ := json.Marshal(params)
			result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, raw)
			if err != nil || !result.Success {
				t.Fatalf("native query failed: %v; %s", err, result.Summary)
			}
			for _, lang := range []string{"zh", "en"} {
				t.Run(lang, func(t *testing.T) {
					quote := fmt.Sprintf("%.6f..%.6f", tc.start, tc.end)
					scope := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &tc.start, TimeEnd: &tc.end, SourceQuote: quote, Confidence: 1}
					if tc.full {
						quote = "whole trace"
						scope = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeFullArtifact, SourceQuote: quote, Confidence: 1}
					}
					request := "Report scheduler states and waits of " + tc.subject + " in " + quote
					rm := types.RequestModel{
						RawRequest: request, Language: lang, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
						RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: tc.pid, Thread: tc.subject, Source: "user_explicit", Confidence: 1}},
						RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: tc.subject, Confidence: 1},
						RuntimeArtifactScopeProfile: scope,
						RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
							FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences}, SourceQuote: "scheduler states and waits", Confidence: 1},
					}
					mu := types.NewMutableState(request)
					mu.SetRequestModel(rm)
					mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
					bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: mu,
						AnalysisIR:               &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}},
						RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}},
					}
					ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
					waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &rm)
					if len(waits) != 1 || waits[0].DStateOccurrences != tc.d || waits[0].IOWaitOccurrences != tc.io || waits[0].SleepIOWaitOccurrences != tc.sleep || waits[0].Count != tc.d+tc.io+tc.sleep || fmt.Sprintf("%.3f", waits[0].WallClockMS) != tc.total || waits[0].WindowStartTs != tc.start || waits[0].WindowEndTs != tc.end {
						t.Fatalf("native classification, measure or scope changed: %+v", waits)
					}
					before, _ := json.Marshal([]any{result, ledger, types.CompileTraceCausalProjectionSet(ledger), rm})
					const model = "模型自己的业务判断与排查方向。 Model-owned business conclusion."
					payload, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "model_summary", "kind": "summary", "text": model}}})
					emit, err := (&EmitAnswerDocument{}).Execute(bus, payload)
					if err != nil || !emit.Success {
						t.Fatalf("public emit failed: %v; %s", err, emit.Summary)
					}
					check := func() string {
						t.Helper()
						doc := mu.AnswerDocumentV2()
						count, modelCount := 0, 0
						var appendix string
						for _, block := range doc.Blocks {
							if block.ID == runtimeTraceTargetStateAuthorityBlockID {
								count++
								appendix = block.Text
							}
							if block.ID == "model_summary" {
								modelCount++
								if block.Text != model {
									t.Fatal("system rewrote model conclusion")
								}
							}
						}
						if count != 1 || modelCount != 1 {
							t.Fatalf("appendix/model duplication: %d/%d", count, modelCount)
						}
						want := fmt.Sprintf("non-IO D-state %d, scheduler-marked IO wait %d, interruptible sleep carrying an IO-wait marker %d", tc.d, tc.io, tc.sleep)
						if lang == "zh" {
							want = fmt.Sprintf("非 IO D-state %d、调度器标记的 IO 等待 %d、带 IO 等待标记的可中断睡眠 %d", tc.d, tc.io, tc.sleep)
						}
						if !strings.Contains(appendix, want) {
							t.Errorf("system counter mislabels exclusive D bucket; missing %q:\n%s", want, appendix)
						}
						if !strings.Contains(appendix, tc.total+"ms") {
							t.Errorf("wait wall clock lost: %s", appendix)
						}
						rawLabel := "original scheduler state: " + tc.rawState
						category := "accounting category: "
						if lang == "zh" {
							rawLabel = "原始调度状态：" + tc.rawState
							category = "统计类别："
						}
						rowCount := 0
						for _, line := range strings.Split(appendix, "\n") {
							if !strings.HasPrefix(line, "- ") {
								continue
							}
							rowCount++
							if !strings.Contains(line, rawLabel) || !strings.Contains(line, category) {
								t.Errorf("native physical state must accompany each unchanged accounting category: %s", line)
							}
						}
						if rowCount != tc.d+tc.io+tc.sleep {
							t.Errorf("wait rows lost: %d", rowCount)
						}
						text := render.RenderAnswerDocument(doc, lang)
						if !strings.Contains(text, model) || !strings.Contains(text, want) {
							t.Error("published answer lost model content or bucket label")
						}
						if strings.Contains(text, "Trace 因果投影") || strings.Contains(text, "Trace causal projection") {
							t.Error("finite state inventory acquired causal report authority")
						}
						return text
					}
					published := check()
					docBeforePatch := mu.AnswerDocumentV2()
					patch, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["model_summary"]}`))
					if err != nil || !patch.Success {
						t.Fatalf("public no-op patch failed: %v; %s", err, patch.Summary)
					}
					if check() != published || !reflect.DeepEqual(docBeforePatch, mu.AnswerDocumentV2()) {
						t.Error("no-op patch duplicated or changed published answer")
					}
					afterLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
					after, _ := json.Marshal([]any{result, afterLedger, types.CompileTraceCausalProjectionSet(afterLedger), bus.AnalysisIR.RequestModel})
					if string(before) != string(after) {
						t.Fatal("publication mutated native evidence, causal projection, target or scope")
					}
				})
			}
		})
	}
}

func traceWaitBucketNativeCycle(start float64, state string, marker int) string {
	return fmt.Sprintf(`idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=reader next_pid=77 next_prio=120
reader-77 (77) [000] .... %.6f: sched_switch: prev_comm=reader prev_pid=77 prev_prio=120 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120
irq-9 (9) [001] .... %.6f: sched_waking: comm=reader pid=77 prio=120 target_cpu=0
irq-9 (9) [001] .... %.6f: sched_blocked_reason: pid=77 iowait=%d caller=wait_site
idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=reader next_pid=77 next_prio=120
reader-77 (77) [000] .... %.6f: sched_switch: prev_comm=reader prev_pid=77 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`, start, start+.001, state, start+.002, start+.002001, marker, start+.003, start+.004)
}
