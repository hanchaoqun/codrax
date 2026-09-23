package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const schedulerConcurrencyContextTrace = "idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120\n" +
	"app-100 (100) [000] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [app]\n" +
	"app-100 (100) [000] .... 1.002000: sched_wakeup: comm=worker pid=300 prio=120 target_cpu=0\n" +
	"irq-2 (2) [000] .... 1.003000: block_rq_complete: 8,0 R () 123 + 8 [0]\n" +
	"app-100 (100) [000] .... 1.004000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=worker next_pid=300 next_prio=120\n" +
	"worker-300 (300) [000] .... 1.008000: sched_switch: prev_comm=worker prev_pid=300 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n" +
	"app-100 (100) [000] .... 1.010000: tracing_mark_write: I|100|window_end\n"

// Real parse -> TraceQuery publication -> dispatch/TurnA -> initial finalizer
// message; no synthetic observation can mask a missing producer connection.
func TestSchedulerConcurrencyPublicFinalizerContext(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeBoundedFactSet, types.RuntimeQuestionScopeCausalDiagnosis} {
			t.Run(lang+"/"+string(scope), func(t *testing.T) {
				ctx, result, native := schedulerConcurrencyContext(t, lang, scope)
				if native == nil || len(native.Groups) != 2 || native.Window == nil {
					t.Fatalf("engine prerequisite: %+v", native)
				}
				ledger := answerDocObservationLedger(ctx)
				before, _ := json.Marshal(ledger)
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				rows, omitted := answerDocSchedulerConcurrencyRows(ledger, &ctx.AnalysisIR.RequestModel)
				if len(rows) != 3 || omitted != 0 {
					t.Fatalf("actual handoff lost state/coverage rows: %d omitted=%d", len(rows), omitted)
				}
				for _, r := range rows {
					line := ioInFlightPublicAllPromptRows(prompt, r.ID)
					for _, want := range []string{"owner_scope=`selected_window_context`", "1.000000..1.010000", "accepted_closed_intervals", "all_positive_tids", "source_conflict="} {
						if !strings.Contains(line, want) {
							t.Errorf("real context lost %q: %s", want, line)
						}
					}
					if r.SourceRef.QueryScopeID == "" || r.SourceRef.PayloadRef == "" || r.Role != types.AnswerAggregateRoleSupportingCoverage {
						t.Fatalf("missing receipt/support-only authority: %+v", r)
					}
					if r.Predicate == "scheduler_concurrency" {
						mean, area := .2, 2.0
						if r.Subject == "running" {
							mean, area = .8, 8
						}
						for _, want := range []string{"峰值=1", fmt.Sprintf("全窗平均=%.9g", mean), fmt.Sprintf("线程时间合计=%.9g thread·ms", area), "已确认闭合区间", "不是目标等待或根因"} {
							if !strings.Contains(line, want) {
								t.Errorf("state %s lost %q: %s", r.Subject, want, line)
							}
						}
					}
				}
				projection := types.CompileTraceCausalProjection(types.ObservationLedger{Records: rows})
				if projection.PrimaryRootCause != nil || len(projection.RankedSeats)+len(projection.OnChainCauses)+len(projection.BackgroundCauses) != 0 {
					t.Fatal("population concurrency granted causal authority")
				}
				var oldOnly types.ObservationLedger
				for _, r := range ledger.Records {
					if !answerDocSchedulerConcurrencyPredicate(r.Predicate) {
						oldOnly.Records = append(oldOnly.Records, r)
					}
				}
				if renderAnswerDocCausalIOMeasurements(ctx, oldOnly) != renderAnswerDocCausalIOMeasurements(ctx, ledger) {
					t.Fatal("new scheduler population displaced or changed existing IO account")
				}
				after, _ := json.Marshal(answerDocObservationLedger(ctx))
				if string(before) != string(after) {
					t.Fatal("context rendering mutated original observations")
				}
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Thread = "running"
				collision := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				for _, r := range rows {
					if strings.Contains(ioInFlightPublicAllPromptRows(collision, r.ID), "owner_scope=`target_owned`") {
						t.Fatal("a target named running acquired global population ownership")
					}
				}
				start, end := 1.002, 1.004
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &start
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
				narrow := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				for _, r := range rows {
					if ioInFlightPublicAllPromptRows(narrow, r.ID) != "" {
						t.Fatal("wider-query concurrency leaked into narrower explicit window")
					}
				}
				_ = result
			})
		}
	}
}

func TestSchedulerConcurrencyPublicReceiptAndBudget(t *testing.T) {
	ctx, first, _ := schedulerConcurrencyContext(t, "en", types.RuntimeQuestionScopeCausalDiagnosis)
	path := first.Observations[0].SourceRef.Path
	for i := 0; i < 5; i++ {
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 1 + float64(i)/1000, "time_end": 1.01})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !result.Success {
			t.Fatalf("actual repeated query failed: %v", err)
		}
		ctx.Mutable.AppendDispatchToolResult(result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	ledger := answerDocObservationLedger(ctx)
	rows, omitted := answerDocSchedulerConcurrencyRows(ledger, &ctx.AnalysisIR.RequestModel)
	eligible := 0
	byID := map[string]types.ObservationRecord{}
	for _, r := range ledger.Records {
		if answerDocSchedulerConcurrencyPredicate(r.Predicate) {
			eligible++
			byID[r.ID] = r
		}
	}
	if len(rows) != 10 || eligible <= 10 || omitted != eligible-len(rows) {
		t.Fatalf("separate handoff budget/omission error: %d/%d omitted=%d", len(rows), eligible, omitted)
	}
	if rows[0].Subject != "runnable" || rows[1].Subject != "running" || rows[2].Predicate != "scheduler_concurrency_coverage" {
		t.Fatal("repeated queries crowded one state or exclusion receipt out")
	}
	for _, r := range rows {
		if !reflect.DeepEqual(r, byID[r.ID]) {
			t.Fatal("display changed a physical query receipt")
		}
	}
	if prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil); !strings.Contains(prompt, fmt.Sprintf("rendered_source_rows=10 omitted_source_rows=%d; independent display budget", omitted)) {
		t.Fatal("actual finalizer omitted bounded handoff disclosure")
	}
	for name, mutate := range map[string]func(*types.ObservationRecord){
		"nondeterministic": func(r *types.ObservationRecord) { r.Producer = "perf_trace" },
		"missing_receipt":  func(r *types.ObservationRecord) { r.SourceRef.PayloadRef = "" },
		"missing_scope":    func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID = "" },
		"unknown_window":   func(r *types.ObservationRecord) { r.SourceRef.QueryWindowKnown = false },
		"soft_grounding":   func(r *types.ObservationRecord) { r.GroundingPolicy = "" },
	} {
		t.Run(name, func(t *testing.T) {
			r := rows[0]
			mutate(&r)
			selected, _ := answerDocSchedulerConcurrencyRows(types.ObservationLedger{Records: []types.ObservationRecord{r}}, &ctx.AnalysisIR.RequestModel)
			if len(selected) != 0 {
				t.Fatalf("missing authority was silently repaired: %+v", selected)
			}
		})
	}
}

func schedulerConcurrencyContext(t *testing.T, lang string, scope types.RuntimeQuestionScope) (*types.AgentContext, types.ToolResult, *tracequery.SchedulerConcurrencyStats) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scheduler.systrace")
	if err := os.WriteFile(path, []byte(schedulerConcurrencyContextTrace), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, result, _ := ioInFlightPublicContext(t, lang, scope, path)
	payloadPath := result.RawRef
	for _, r := range result.Observations {
		if r.SourceRef.PayloadRef != "" {
			payloadPath = r.SourceRef.PayloadRef
			break
		}
	}
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	var native tracequery.Result
	if err := json.Unmarshal(data, &native); err != nil || native.WindowStats == nil {
		t.Fatalf("native query payload missing: %v", err)
	}
	return ctx, result, native.WindowStats.SchedulerConcurrency
}
