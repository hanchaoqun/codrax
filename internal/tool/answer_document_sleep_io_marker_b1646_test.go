package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The marker and the independently closed request describe the same S segment
// when both are present. Neither view changes the native scheduling partition.
// This is a synthetic capture consumed by the real query and publication entry
// points; no source observation or projection field is patched by the test.
func b1646SleepIOMarkerBus(t *testing.T, lang string, marked, completion bool) *types.BusContext {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "marker-and-completion.ftrace")
	rows := []string{
		"idle-0 (0) [001] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20",
	}
	if completion {
		rows = append(rows, "target-41 (41) [001] .... 5.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [target]")
	}
	rows = append(rows, "target-41 (41) [001] .... 5.002000: sched_switch: prev_comm=target prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120")
	if completion {
		rows = append(rows, "irq-2 (2) [001] .... 5.011000: block_rq_complete: 8,0 R () 123 + 8 [0]")
	}
	rows = append(rows, "irq-2 (2) [001] .... 5.011010: sched_wakeup: comm=target pid=41 prio=20 target_cpu=001")
	if marked {
		rows = append(rows, "irq-2 (2) [001] .... 5.011011: sched_blocked_reason: pid=41 iowait=1 caller=marker_wait_site")
	}
	rows = append(rows,
		"idle-0 (0) [001] .... 5.012000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20",
		"target-41 (41) [001] .... 5.020000: sched_switch: prev_comm=target prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120",
	)
	if err := os.WriteFile(path, []byte(strings.Join(rows, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 5.0, 5.02
	bus := &types.BusContext{
		RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: types.NewMutableState("typed marker and independent completion"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace, Language: lang, Scenario: types.ScenarioPerformanceBottleneck,
				RuntimeTargets:         []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "target-41", Source: "user_explicit"}},
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
					TimeStart: &start, TimeEnd: &end, SourceQuote: "5.0 to 5.02"},
			},
			AnswerContract: types.AnswerContract{Language: lang},
		},
	}
	params, err := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": "root_cause_rank", "pid": 41,
		"time_start": start, "time_end": end, "limit": 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("actual query: %v; %s", err, result.Summary)
	}
	bus.ToolResults = []types.ToolResult{result}
	return bus
}

func TestB1646SleepIOMarkerActualPublication(t *testing.T) {
	for _, marked := range []bool{false, true} {
		for _, completion := range []bool{false, true} {
			for _, lang := range []string{"zh", "en"} {
				t.Run(fmt.Sprintf("%s/marked=%t/completion=%t", lang, marked, completion), func(t *testing.T) {
					bus := b1646SleepIOMarkerBus(t, lang, marked, completion)
					before, err := json.Marshal(bus.ToolResults)
					if err != nil {
						t.Fatal(err)
					}
					ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
					projection := types.CompileTraceCausalProjection(ledger)
					state := projection.TargetStateAccount
					markerMS := "0.000"
					if marked {
						markerMS = "9.010"
					}
					if state == nil || fmt.Sprintf("%.3f/%.3f/%.3f/%.3f/%.3f/%.3f", state.RunningMS, state.RunnableMS, state.SleepMS, state.DStateMS, state.IOWaitMS, state.TotalMS) != "10.000/0.990/9.010/0.000/0.000/20.000" || fmt.Sprintf("%.3f", state.SleepIOWaitMS) != markerMS {
						t.Fatalf("native state/marker premise changed: %+v", state)
					}
					pairs := 0
					for _, r := range ledger.Records {
						notes := strings.Join(r.RichNotes, "\n")
						if r.Predicate != "io_latency" || !strings.Contains(notes, "request_residence_caliber=block_rq_issue_to_complete") {
							continue
						}
						pairs++
						for _, want := range []string{"request_residence=10.000", "issuer_blocked=9.010", "completion_woke_issuer=true", "issuer_blocked_state=s_sleep", "causal_wait_caliber=completion_closed_issuer_blocked"} {
							if !strings.Contains(notes, want) {
								t.Fatalf("native independent completion premise missing %q: %s", want, notes)
							}
						}
					}
					if pairs != map[bool]int{false: 0, true: 1}[completion] {
						t.Fatalf("native request count=%d completion=%t", pairs, completion)
					}
					authorities := types.BuildTraceBlockingWallClockAuthorities(ledger, &bus.AnalysisIR.RequestModel)
					closedAccounts := 0
					for _, authority := range authorities {
						if authority.Type != "block_io_completion_closed_issuer_wait" {
							continue
						}
						closedAccounts++
						if authority.Subject != "target-41" || fmt.Sprintf("%.3f", authority.ObservedMS) != "9.010" || len(authority.Occurrences) != 1 || authority.Occurrences[0].StartTs != 5.002 || authority.Occurrences[0].EndTs != 5.011010 {
							t.Fatalf("native closure must retain its own exact interval: %+v", authority)
						}
					}
					if closedAccounts != pairs {
						t.Fatalf("marker presence must neither suppress nor mint completion authority: %+v", authorities)
					}
					doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
						ID: "model-summary", Kind: types.BlockSummary, Text: "Model wording stays: S sleep 9.010 ms; no system-authored diagnosis.",
					}}}
					modelWire, err := modelOwnedAnswerBlockWire(doc)
					if err != nil {
						t.Fatal(err)
					}
					result, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
					if err != nil || !result.Success {
						t.Fatalf("actual publication: %v; %+v", err, result)
					}
					stored := bus.Mutable.AnswerDocumentV2()
					if err := requireModelOwnedAnswerBlockWirePreserved(modelWire, stored); err != nil {
						t.Fatal(err)
					}
					faces := map[string]string{}
					for _, block := range stored.Blocks {
						if block.ID == runtimeTraceTargetStateAuthorityBlockID {
							faces["state"] = block.Text
						}
						if RuntimeTraceSystemBlock(block) && strings.HasSuffix(block.ID, runtimeTraceCausalProjectionOccupancySuffix) {
							faces["occupancy"] = block.Text
						}
					}
					if len(faces) != 2 {
						t.Fatalf("actual publication must exercise both state surfaces, got %v", faces)
					}
					label, zero, overlap := "S-state wait confirmed by kernel IO-wait markers", "A zero value does not rule out blocking independently proven by an IO-completion wakeup", "do not count the same time interval twice"
					if lang == "zh" {
						label, zero, overlap = "内核 IO 等待标记确认的 S 态等待", "零值不排除由 IO 完成唤醒独立证明的线程阻塞", "同一段时间不得重复计入"
					}
					markedValue := markerMS + "ms of " + label
					if lang == "zh" {
						markedValue = label + " " + markerMS + "ms"
					}
					for name, face := range faces {
						for _, want := range []string{markedValue, zero, overlap, "10.000ms", "0.990ms", "9.010ms"} {
							if !strings.Contains(face, want) {
								t.Errorf("%s lacks marker-only display boundary %q:\n%s", name, want, face)
							}
						}
						if strings.Count(face, zero) != 1 || strings.Count(face, markedValue) != 1 {
							t.Errorf("%s must show the unchanged marker value and its boundary once: %s", name, face)
						}
					}
					// Display cannot mutate native facts, authority or model bytes.
					after, _ := json.Marshal(bus.ToolResults)
					afterLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
					if string(before) != string(after) || !reflect.DeepEqual(authorities, types.BuildTraceBlockingWallClockAuthorities(afterLedger, &bus.AnalysisIR.RequestModel)) {
						t.Fatal("display changed native query bytes or blocking authority")
					}
				})
			}
		}
	}
}
