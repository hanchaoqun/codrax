package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Each capture has an ordinary Binder sleep as well as an independently
// completion-closed IO wait. Only the latter optionally enters the scheduler
// D/IO roster. All observations come from the public trace query producer.
func b1646WaitScopeContext(t *testing.T, lang, state string) *types.AgentContext {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "wait-scopes.ftrace")
	sleep := "S"
	if state == "d" {
		sleep = "D"
	}
	rows := []string{
		"idle-0 (0) [001] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20",
		"target-41 (41) [001] .... 5.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [target]",
		fmt.Sprintf("target-41 (41) [001] .... 5.002000: sched_switch: prev_comm=target prev_pid=41 prev_prio=20 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120", sleep),
		"irq-2 (2) [001] .... 5.011000: block_rq_complete: 8,0 R () 123 + 8 [0]",
		"irq-2 (2) [001] .... 5.011010: sched_wakeup: comm=target pid=41 prio=20 target_cpu=001",
	}
	if state == "marked_s" {
		rows = append(rows, "irq-2 (2) [001] .... 5.011011: sched_blocked_reason: pid=41 iowait=1 caller=marker_wait_site")
	}
	rows = append(rows,
		"idle-0 (0) [001] .... 5.012000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20",
		"target-41 (41) [001] .... 5.014000: binder_transaction: transaction=7 dest_proc=51 dest_thread=51 reply=0 flags=0x0 code=0x1",
		"target-41 (41) [001] .... 5.015000: sched_switch: prev_comm=target prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=server next_pid=51 next_prio=120",
		"server-51 (51) [001] .... 5.015010: binder_transaction_received: transaction=7",
		"server-51 (51) [001] .... 5.018000: binder_transaction: transaction=8 dest_proc=41 dest_thread=41 reply=1 flags=0x0 code=0x0",
		"server-51 (51) [001] .... 5.018010: sched_wakeup: comm=target pid=41 prio=20 target_cpu=001",
		"server-51 (51) [001] .... 5.018100: sched_switch: prev_comm=server prev_pid=51 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20",
		"target-41 (41) [001] .... 5.018200: binder_transaction_received: transaction=8",
		"target-41 (41) [001] .... 5.020000: sched_switch: prev_comm=target prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120",
	)
	if err := os.WriteFile(path, []byte(strings.Join(rows, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 5.0, 5.02
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: types.NewMutableState("wait scopes")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 41, "time_start": start, "time_end": end, "limit": 100})
	result, err := (&tool.TraceQuery{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("actual query failed: %v; %s", err, result.Summary)
	}
	ctx := tracePrincipalValueAuthorityTestContext("target-41", 41, result.Observations)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ctx.Language, ctx.AgentName, ctx.Stage = lang, types.AgentFinalizer, types.StageFinalize
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}
	return ctx
}

func TestB1646WaitScopeActualFinalizer(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, state := range []string{"plain_s", "marked_s", "d"} {
			t.Run(lang+"/"+state, func(t *testing.T) {
				ctx := b1646WaitScopeContext(t, lang, state)
				before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				ledger := answerDocObservationLedger(ctx)
				waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
				count, wall := 1, "9.010"
				if state == "plain_s" {
					count, wall = 0, "0.000"
				}
				if len(waits) != 1 || waits[0].Count != count || fmt.Sprintf("%.3f", waits[0].WallClockMS) != wall {
					t.Fatalf("native scheduler roster premise: %+v", waits)
				}
				binder, completion := false, false
				for _, r := range ledger.Records {
					if r.Predicate == "target_binder_wait_inventory" && r.ResultCount != nil && *r.ResultCount == 1 && r.Value == "3.010" {
						binder = true
					}
					if r.Predicate == "io_latency" && strings.Contains(strings.Join(r.RichNotes, "\n"), "completion_woke_issuer=true") && strings.Contains(strings.Join(r.RichNotes, "\n"), "issuer_blocked=9.010") {
						completion = true
					}
				}
				if !binder || !completion {
					t.Fatalf("independent native positive premises absent: binder=%t completion=%t", binder, completion)
				}
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				var conclusion string
				for _, line := range strings.Split(prompt, "\n") {
					if strings.Contains(line, "principal_conclusion_") && !strings.Contains(line, "关于 ") && !strings.Contains(line, "block_io_") && !strings.Contains(line, "io_latency") {
						conclusion = line
						break
					}
				}
				want := fmt.Sprintf("D/IO-state wait roster contains exactly %d interval(s), totaling %sms", count, wall)
				boundary := "does not count unmarked S-state sleep or runnable time"
				if lang == "zh" {
					want = fmt.Sprintf("D/IO 状态等待清单共 %d 段，墙钟合计 %sms", count, wall)
					boundary = "不统计未带 IO 等待标记的 S 态睡眠或可运行等待"
				}
				if !strings.Contains(conclusion, want) || !strings.Contains(conclusion, boundary) || !strings.Contains(conclusion, "Binder") {
					t.Errorf("actual finalizer overstates roster scope: want %q and %q; got %s", want, boundary, conclusion)
				}
				if !strings.Contains(prompt, "verified_wait_union=3.010ms") || !strings.Contains(prompt, "block_io_completion_closed_issuer_wait") || !strings.Contains(prompt, "occurrence_count="+fmt.Sprint(count)) {
					t.Fatal("display dropped independent accounts or original roster values")
				}
				after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				if string(before) != string(after) {
					t.Fatal("prompt rendering mutated published query data")
				}
			})
		}
	}
}

func TestB1646WaitScopeDoesNotBorrowOrInventQueryWindow(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, mode := range []string{"different_request", "legacy_unstated", "no_observations"} {
			t.Run(lang+"/"+mode, func(t *testing.T) {
				ctx := b1646WaitScopeContext(t, lang, "marked_s")
				switch mode {
				case "different_request":
					start, end := 4.0, 6.0
					ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &start
					ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
				case "legacy_unstated":
					// Simulate the supported old published record shape, without
					// a selected-window declaration. Physical span endpoints stay
					// present, but must not become the declared query window.
					artifacts := ctx.Mutable.TurnAArtifacts()
					for i := range artifacts.ToolResults {
						for j := range artifacts.ToolResults[i].Observations {
							r := &artifacts.ToolResults[i].Observations[j]
							var notes []string
							for _, note := range r.RichNotes {
								if !strings.HasPrefix(note, "selected_window=") {
									notes = append(notes, note)
								}
							}
							r.RichNotes = notes
						}
					}
					ctx.Mutable.SetTurnAArtifacts(*artifacts)
				case "no_observations":
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
				}
				before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				var conclusion string
				for _, line := range strings.Split(prompt, "\n") {
					if strings.Contains(line, "principal_conclusion_") && (strings.Contains(line, "D/IO 状态等待清单") || strings.Contains(line, "D/IO-state wait roster")) {
						conclusion = line
						break
					}
				}
				switch mode {
				case "no_observations":
					if conclusion != "" {
						t.Fatalf("missing observations minted a wait roster: %s", conclusion)
					}
				case "different_request":
					if !strings.Contains(conclusion, "5.000000..5.020000") || strings.Contains(conclusion, "4.000000..6.000000") || !strings.Contains(conclusion, "9.010ms") {
						t.Fatalf("wait conclusion borrowed the requested window: %s", conclusion)
					}
				case "legacy_unstated":
					unknown := "actual query window is unknown"
					if lang == "zh" {
						unknown = "实际查询范围未明确"
					}
					if !strings.Contains(conclusion, unknown) || !strings.Contains(conclusion, "9.010ms") || strings.Contains(conclusion, "0.000000..0.000000") || strings.Contains(conclusion, "5.000000..5.020000") {
						t.Fatalf("unstated query window became a real window or lost its value: %s", conclusion)
					}
				}
				after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				if string(before) != string(after) {
					t.Fatal("display mutated scope input")
				}
			})
		}
	}
}
