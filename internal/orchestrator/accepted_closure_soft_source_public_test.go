package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Only the model's decisions are scripted. Run owns the state and each
// completion mark/caveat below must come from the real completion tool.
func TestAcceptedClosureSoftSourcePublicRun(t *testing.T) {
	for _, reconcile := range []bool{false, true} {
		t.Run(fmt.Sprintf("reconcile=%t", reconcile), func(t *testing.T) {
			got := runSoftSourcePublic(t, reconcile, "")
			if got.err != nil {
				t.Fatalf("Run failed: %v", got.err)
			}
			if got.accepted == 0 || got.firstGeneration == 0 || !got.firstCaveat {
				t.Fatalf("no real accepted source-caveat receipt: %+v", got)
			}
			if got.calls != 1 {
				t.Errorf("accepted soft current-source receipt reopened exploration: calls=%d accepted=%d first_generation=%d dispatches=%v", got.calls, got.accepted, got.firstGeneration, got.dispatches)
			}
			if got.finalized != 1 {
				t.Errorf("finalizer calls=%d, want 1", got.finalized)
			}
			if !got.firstExploreClosable || !got.firstReconcileClosable {
				t.Errorf("actual accepted receipt was not honored by both consumers: explore=%t reconcile=%t", got.firstExploreClosable, got.firstReconcileClosable)
			}
			if got.finalAuthority.CurrentSourceSatisfied || !got.finalAuthority.CurrentSourceRequired || !got.finalAuthority.CanUseRuntimeOnlyWithCaveat || got.finalAuthority.CanHardBlockCompletion {
				t.Errorf("closure must retain the unsatisfied soft source boundary: %+v", got.finalAuthority)
			}
		})
	}
}

func TestAcceptedClosureSoftSourcePublicMixedFirstCloseStillRejected(t *testing.T) {
	for _, quote := range []string{"explain the current implementation", "main.go:2"} {
		t.Run(quote, func(t *testing.T) {
			got := runSoftSourcePublic(t, false, quote)
			if got.err != nil || !got.stoppedAfterFirst {
				t.Fatalf("did not observe and stop after actual completion attempt: %+v", got)
			}
			if got.calls != 1 || got.accepted != 0 || got.firstGeneration != 0 || got.firstCaveat {
				t.Fatalf("a real source obligation silently acquired accepted closure: %+v", got)
			}
			if !strings.Contains(got.firstCompletion, "current-source") || !strings.Contains(got.firstCompletion, "DOWNGRADED") {
				t.Fatalf("source obligation was not the actual first-close blocker: %s", got.firstCompletion)
			}
		})
	}
}

var errSoftSourcePublicStop = errors.New("soft-source public test: first-close observed")

type softSourcePublicResult struct {
	err                        error
	calls, accepted, finalized int
	firstGeneration            uint64
	firstCaveat                bool
	stoppedAfterFirst          bool
	firstCompletion            string
	firstExploreClosable       bool
	firstReconcileClosable     bool
	dispatches                 []string
	finalAuthority             types.RuntimeSourceAnswerAuthoritySnapshot
	orchestrator               *Orchestrator
}

func runSoftSourcePublic(t *testing.T, reconcile bool, sourceQuote string) softSourcePublicResult {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "native.systrace")
	for name, body := range map[string]string{"native.systrace": softSourcePublicTrace, "main.go": "package main\nfunc main() {}\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	request := "Use trace_query on " + path + " for app-100 in 2.000s to 2.020s; explain the blocking cause and wakeup chain."
	if sourceQuote != "" {
		request += " Also " + sourceQuote + "."
	}
	var got softSourcePublicResult
	var o *Orchestrator
	fns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentPerfTriager: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			result, err := (&tool.EmitPerfTrace{}).Execute(o.busCtx, json.RawMessage(`{"meta":{"source":"hitrace","app_pid":100},"observations":[{"kind":"scheduler_state","subject":"app-100","summary":"app-100 switches out in S at 2.000s","evidence":"prev_pid=100 prev_prio=52 prev_state=S","line_start":2,"line_end":2,"start_ts_ms":2000,"confidence":1}]}`))
			if err != nil || !result.Success {
				t.Fatalf("actual perf triage: %v / %s", err, result.Summary)
			}
			return &agent.StageOutput{MissingPiece: types.MissingFacts, ToolResults: []types.ToolResult{result}}, nil
		},
		types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			params := softSourcePublicAnalysis(sourceQuote)
			raw, _ := json.Marshal(params)
			result, err := (&tool.EmitAnalysis{}).Execute(o.busCtx, raw)
			if err != nil || !result.Success {
				t.Fatalf("actual emit_analysis: %v / %s", err, result.Summary)
			}
			rm := ctx.Mutable.RequestModel()
			if rm.ExternalObservationPolicy == nil || rm.ExternalObservationPolicy.CurrentSourceMode != types.ExternalObservationCurrentSourceAllow {
				t.Fatalf("route did not produce allow policy: %+v", rm.ExternalObservationPolicy)
			}
			compiled := compiler.Compile(*rm, budget.BudgetSignals{})
			// Keep the real normalized request and compiled evidence plan, but
			// isolate the two scheduler consumers with an otherwise empty answer
			// contract: unrelated presentation gates are not under test.
			ir := dagIR(types.AnswerContract{Language: "en"})
			ir.RequestModel, ir.EvidencePlan = *rm, compiled.EvidencePlan
			// No synthetic hypothesis set is installed in this scheduler
			// fixture; the template's all-hypotheses stop is vacuously true.
			ir.EvidencePlan.StopConditions = nil
			if reconcile {
				ir.TaskGraph.Nodes = []types.TaskNode{
					{ID: "probe", Type: types.NodeProbe, Objective: "query native trace"},
					{ID: "reconcile", Type: types.NodeReconcile, Objective: "reconcile accepted runtime closure", SuccessCriteria: []types.Criterion{{Kind: types.CritHasEnoughFacts}}},
					{ID: "finalize", Type: types.NodeFinalize, Objective: "render accepted boundary"},
				}
				ir.TaskGraph.Edges = []types.TaskEdge{{From: "probe", To: "reconcile", EdgeType: types.EdgeHardDependency}, {From: "reconcile", To: "finalize", EdgeType: types.EdgeHardDependency}}
				ir.TaskGraph.ExecutionPolicy.CriticalPath = []string{"probe", "reconcile", "finalize"}
			}
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir, ToolResults: []types.ToolResult{result}}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			got.calls++
			got.dispatches = append(got.dispatches, ctx.ExploreDispatchKey)
			if o.busCtx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo() {
				t.Fatal("source-empty waiver would mask the test")
			}
			var results []types.ToolResult
			for _, view := range []string{"thread_timeline", "wakeup_chain", "root_cause_rank"} {
				raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": 2, "time_end": 2.020, "trace_flavor": "harmony_hitrace"})
				result, err := (&tool.TraceQuery{}).Execute(o.busCtx, raw)
				if err != nil || !result.Success || len(result.Observations) == 0 {
					t.Fatalf("actual trace_query %s: %v / %s", view, err, result.Summary)
				}
				ctx.Mutable.AppendDispatchToolResult(result)
				results = append(results, result)
			}
			authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(o.busCtx, types.ObservationLedger{})
			assertSoftSourcePublicNativeWindow(t, o.busCtx)
			if !authority.Active || !authority.HasRuntimeCarrier() || authority.CurrentSourceSatisfied || !authority.CurrentSourceRequired {
				t.Fatalf("wrong actual source authority: %+v", authority)
			}
			if sourceQuote == "" && (authority.CurrentSourceRequirement != types.RuntimeSourceRequirementSoft || authority.CanHardBlockCompletion || !authority.CanDowngradeToCaveat) {
				t.Fatalf("expected only soft source debt: %+v", authority)
			}
			result, err := (&tool.EmitInvestigationComplete{}).Execute(o.busCtx, json.RawMessage(`{"reason":"Native timeline and wakeup evidence delimit the runtime cause; current implementation remains unverified.","confidence":"high","result_kind":"resolved"}`))
			if err != nil {
				t.Fatal(err)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			results = append(results, result)
			complete := ctx.Mutable.IsInvestigationComplete()
			if complete {
				got.accepted++
			}
			if got.calls == 1 {
				got.firstCompletion = result.Summary
				got.firstGeneration = ctx.Mutable.InvestigationCompleteGeneration()
				got.firstCaveat = ctx.Mutable.EvidenceClosure().HasCompletionCaveat(types.DowngradeLaneCurrentSourceLane)
				t.Logf("actual first completion: accepted=%t generation=%d source_caveat=%t authority=%+v summary=%s", complete, got.firstGeneration, got.firstCaveat, authority, result.Summary)
			}
			if sourceQuote != "" {
				got.stoppedAfterFirst = true
				// Run records an explorer failure as advisory and may finalize;
				// the assertion concerns the real tool's FIRST refusal, not a
				// fabricated accepted state or an expected Run transport error.
				return nil, errSoftSourcePublicStop
			}
			if !complete {
				t.Fatalf("fixture failed to reach actual accepted closure: %s", result.Summary)
			}
			// Mirror the explorer's accepted handoff using ONLY values produced
			// by the tool. This is not a fabricated completion/caveat setter.
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, AcceptedClosureReason: ctx.Mutable.StableInvestigationCompleteReason(), AcceptedResultKind: ctx.Mutable.StableInvestigationResultKind(), AcceptedAggregateFacts: ctx.Mutable.StableInvestigationAggregateFacts(), RuntimeObservationOnlyCompletion: authority.CanUseRuntimeOnlyWithCaveat})
			if got.calls == 1 {
				got.firstExploreClosable = o.shouldAutoCompleteExploreWindowFromAcceptedClosure(nil, "", "")
				got.firstReconcileClosable = o.acceptedClosureCanSatisfyReconcileEnoughFacts()
			}
			return &agent.StageOutput{MissingPiece: types.MissingNone, ToolResults: results, SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: false}}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			got.finalized++
			got.finalAuthority = types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(o.busCtx, types.ObservationLedger{})
			assertSoftSourcePublicNativeWindow(t, o.busCtx)
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "Native runtime observations only; current source remains unverified."}, nil
		},
	}
	ar, sr, sar := buildRegistries(fns)
	sr.Register(&skill.Config{Name: "perf-triage-skill", Goal: "extract native trace observation"})
	o = New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(16)
	o.SetAttachedHitrace(softSourcePublicTrace)
	o.SetTurnRouteHint(types.TurnRouteHint{Route: "repo", Source: "artifact", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired, Confidence: 1})
	_, got.err = o.Run(request, root, "main")
	got.orchestrator = o
	t.Logf("Run receipt: calls=%d state=%+v", got.calls, o.busCtx.TaskState)
	for name, want := range map[string]string{"native.systrace": softSourcePublicTrace, "main.go": "package main\nfunc main() {}\n"} {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(body) != want {
			t.Fatalf("Run altered source input %s: %v", name, err)
		}
	}
	return got
}

func assertSoftSourcePublicNativeWindow(t *testing.T, bus *types.BusContext) {
	t.Helper()
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	for _, account := range types.BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger) {
		if account.Subject != "app-100" || account.WindowStartTs != 2 || account.WindowEndTs != 2.020 {
			continue
		}
		if fmt.Sprintf("%.3f/%.3f", account.SleepMS, account.TotalMS) != "20.000/20.000" || account.RunningMS != 0 || account.RunnableMS != 0 || account.DStateMS != 0 || account.IOWaitMS != 0 {
			t.Fatalf("original native window/values changed: %+v", account)
		}
		return
	}
	t.Fatal("actual native target window disappeared")
}

func softSourcePublicAnalysis(sourceQuote string) map[string]any {
	predicates := map[string]any{"is_diagnostic_question": true}
	for _, key := range []string{"is_scalar_answer", "is_role_locate_lookup", "is_count_question", "is_cross_component", "is_relational_lookup", "is_category_enumeration", "is_history_lookup", "has_per_member_table"} {
		predicates[key] = false
	}
	p := map[string]any{
		"intent": "root_cause", "scenario": "root_cause", "complexity": "moderate", "language": "en",
		"intent_confidence": .95, "complexity_confidence": .95, "kind_confidence": .95,
		"keywords": []string{"app-100", "sched_wakeup"}, "entities": []string{"app-100"},
		"question_kind": "mechanism", "predicate_axis": "condition",
		"completeness_obligation":        map[string]any{"required": false, "source_quote": ""},
		"answer_role_profile":            map[string]any{"is_role_binding_requested": false, "confidence": 1},
		"error_granularity_profile":      map[string]any{"is_granularity_question": false, "confidence": 1},
		"requested_answer_dimensions":    map[string]any{"is_dimensioned_answer": true, "confidence": 1, "dimensions": []any{map[string]any{"index": 1, "label": "blocking cause", "role": "causal_attribution", "required": true, "source_quote": "blocking cause"}}},
		"runtime_selection_profile":      map[string]any{"is_selection_question": false, "confidence": 1},
		"predicates":                     predicates,
		"diagnostic_profile":             map[string]any{"is_diagnostic": true, "current_risk": false, "current_version_check": false, "historical_regression": false, "confidence": .95},
		"runtime_target_profile":         map[string]any{"declaration": "named_target", "source_quote": "app-100", "confidence": 1},
		"runtime_targets":                []any{map[string]any{"kind": "thread", "thread": "app-100", "pid": 100, "source": "user_explicit", "confidence": 1}},
		"runtime_artifact_scope_profile": map[string]any{"requested_scope": "explicit_time_window", "source_quote": "2.000s to 2.020s", "time_start": 2, "time_end": 2.020, "confidence": 1},
		"runtime_question_profile":       map[string]any{"scope": "causal_diagnosis", "runtime_work_relation_requested": true, "frame_causality_requested": false, "confidence": .95},
	}
	if sourceQuote != "" {
		p["current_source_explanation_profile"] = map[string]any{"is_current_source_explanation_requested": true, "modes": []string{"explain_current_mechanism"}, "source_quotes": []string{sourceQuote}, "confidence": 1}
	}
	return p
}

const softSourcePublicTrace = `# tracer: nop
app-100 (100) [001] .... 2.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
cookie-200 (100) [002] .... 2.001000: sched_switch: prev_comm=cookie prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
network-300 (100) [003] .... 2.002000: sched_switch: prev_comm=network prev_pid=300 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
threadpool-400 (100) [004] .... 2.003000: sched_switch: prev_comm=threadpool prev_pid=400 prev_prio=20 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120
irq-2 (2) [004] .... 2.004000: sched_blocked_reason: pid=400 iowait=1 caller=fscache_page_wait_on_page_bit
irq-2 (2) [004] .... 2.014000: sched_wakeup: comm=threadpool pid=400 prio=20 target_cpu=004
threadpool-400 (100) [004] .... 2.015000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=threadpool next_pid=400 next_prio=20
threadpool-400 (100) [004] .... 2.016000: sched_wakeup: comm=network pid=300 prio=20 target_cpu=003
network-300 (100) [003] .... 2.017000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=network next_pid=300 next_prio=20
network-300 (100) [003] .... 2.018000: sched_wakeup: comm=cookie pid=200 prio=20 target_cpu=002
cookie-200 (100) [002] .... 2.019000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=cookie next_pid=200 next_prio=20
cookie-200 (100) [002] .... 2.020000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
app-100 (100) [001] .... 2.020020: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`
