package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Replay the public query shapes from HMC308 A, not its accepted prose or
// serialized observations. The original tool and finalizer own the handoff.
func TestHMC309TraceGapPublicQueryToFinalizer(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh",
		AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Explain the observed business response and dependency waits"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: "zh", Intent: types.IntentRootCause, PerfTrace: &types.PerfBundle{},
			AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
				Scope: types.RuntimeQuestionScopeCausalDiagnosis, RuntimeWorkRelationRequested: true,
			},
		}},
	}
	var rootPayload []byte
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "event_search"} {
		params := map[string]any{"source": "path", "path": path, "view": view,
			"time_start": .999, "time_end": 1.051, "trace_flavor": "harmony_hitrace"}
		if view == "event_search" {
			params["event_types"] = []string{"trace_mark", "sched_wakeup", "sched_switch", "file_io"}
			params["limit"] = 40
		} else {
			params["pid"] = 100
		}
		data, _ := json.Marshal(params)
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), data)
		if err != nil || !result.Success {
			t.Fatalf("public %s failed: %v %s", view, err, result.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(result)
		if view == "root_cause_rank" {
			for _, row := range result.Observations {
				if row.SourceRef.PayloadRef != "" {
					rootPayload, err = os.ReadFile(row.SourceRef.PayloadRef)
					if err != nil {
						t.Fatal(err)
					}
					break
				}
			}
		}
	}
	var native tracequery.Result
	if err := json.Unmarshal(rootPayload, &native); err != nil {
		t.Fatalf("missing source-bound native JSON: %v", err)
	}
	gap, completion := false, false
	if native.WakeupChain != nil {
		for _, row := range native.WakeupChain.RootEvidence {
			if row.Thread.PID == 80 && row.Type == "trace_gap" && row.GapKind == "no_sched_data" {
				gap = true
			}
		}
	}
	if native.WindowStats != nil {
		for _, row := range native.WindowStats.IOLatencies {
			if row.IssueThread.PID == 200 && row.CompleteThread.PID == 80 && row.CompleteThread.TGID == 2 && row.CompletionWokeIssuer {
				completion = true
			}
		}
	}
	if !gap || !completion {
		t.Fatalf("public premise failed: no scheduler data=%t, independently proved completion=%t", gap, completion)
	}
	t.Logf("public_native_payload:\n%s", rootPayload)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	for _, language := range []string{"zh", "en"} {
		ctx.Language, ctx.AnalysisIR.RequestModel.Language = language, language
		prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
		t.Logf("language=%s actual_initial_instruction:\n%s", language, prompt)
		boundaryFound := false
		for _, line := range strings.Split(prompt, "\n") {
			if !strings.Contains(line, "evidence_boundary: subject=`storage-irq-80`") {
				continue
			}
			boundaryFound = true
			for _, want := range []string{"kind=`trace_gap`", "gap_kind=`no_sched_data`", "status=`no_scheduler_intervals_in_aligned_scope`", "aligned_scope_bounds=`not_published`"} {
				if !strings.Contains(line, want) {
					t.Errorf("%s native boundary lost %q: %s", language, want, line)
				}
			}
			for _, forbidden := range []string{"no_matching_sched_wakeup_row", "causal_identity=`unresolved`", "observed_sleep_interval="} {
				if strings.Contains(line, forbidden) {
					t.Errorf("%s scheduler-only gap asserted unrelated absence %q: %s", language, forbidden, line)
				}
			}
		}
		if !boundaryFound {
			t.Errorf("%s no native trace boundary reached finalizer", language)
		}
		for _, want := range []string{"complete_thread=`storage-irq-80`", "completion_woke_issuer=`true`", "issuer_blocked=`31.000`", "request_residence=`35.000`", "storage-irq-80 -> document-worker-200 -> app-main-100"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s independently proved fact lost %q", language, want)
			}
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("display repair mutated the original ledger")
	}
}

func TestHMC309TraceGapPublicCriteriaStayDistinct(t *testing.T) {
	const start = "target-410 (410) [001] .... 3.000000: sched_switch: prev_comm=target prev_pid=410 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n"
	const wake = "sender-720 (720) [003] .... 3.030000: sched_wakeup: comm=target pid=410 prio=52 target_cpu=001\n"
	const end = "target-410 (410) [001] .... 3.035000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=410 next_prio=52\ntarget-410 (410) [001] .... 3.039000: sched_switch: prev_comm=target prev_pid=410 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n"
	for _, tc := range []struct{ name, input, kind, gap, status, subject string }{
		{"empty_timeline", start + wake + end, "trace_gap", "no_sched_data", "no_scheduler_intervals_in_aligned_scope", "sender-720"},
		{"sub_floor_timeline", start + "sender-720 (720) [003] .... 3.029960: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=sender next_pid=720 next_prio=30\n" + wake + end, "trace_gap", "no_eligible_wait", "no_interval_meets_min_duration_in_aligned_scope", "sender-720"},
		{"missing_wakeup", start + end, "missing_wakeup", "", "no_matching_sched_wakeup_row_in_selected_window", "target-410"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "capture.systrace")
			if err := os.WriteFile(path, []byte(tc.input), 0600); err != nil {
				t.Fatal(err)
			}
			ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "en",
				AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("Explain the native wait evidence"),
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "en", Intent: types.IntentRootCause,
					PerfTrace: &types.PerfBundle{}, AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
					RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}}}}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "wakeup_chain", "pid": 410,
				"time_start": 3, "time_end": 3.04, "min_duration_ms": .05, "trace_flavor": "harmony_hitrace"})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !result.Success {
				t.Fatalf("public query failed: %v %s", err, result.Summary)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			found := false
			for _, line := range strings.Split(prompt, "\n") {
				if !strings.Contains(line, "evidence_boundary: subject=`"+tc.subject+"`") || !strings.Contains(line, "kind=`"+tc.kind+"`") {
					continue
				}
				found = true
				t.Logf("public %s boundary: %s", tc.name, line)
				if !strings.Contains(line, "status=`"+tc.status+"`") || !strings.Contains(line, "evidence_id=`trace_query:") {
					t.Errorf("wrong criterion or lost receipt: %s", line)
				}
				if tc.gap != "" && (!strings.Contains(line, "gap_kind=`"+tc.gap+"`") || strings.Contains(line, "observed_sleep_interval=")) {
					t.Errorf("scheduler criterion became measured sleep: %s", line)
				}
				if tc.kind == "missing_wakeup" && !strings.Contains(line, "observed_sleep_interval=`3.000000..3.035000`") {
					t.Errorf("real missing wakeup lost its measured sleep: %s", line)
				}
			}
			if !found {
				t.Fatalf("native criterion missing from actual finalizer:\n%s", prompt)
			}
		})
	}
}

func TestHMC309TraceGapUnknownsDoNotGuessFromProseOrParentWindow(t *testing.T) {
	for _, tc := range []struct{ name, kind, gap string }{
		{"legacy", "trace_gap", ""}, {"future_criterion", "trace_gap", "future_gap"}, {"future_domain", "future_data_gap", "no_sched_data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := types.TraceCausalProjectionNode{Subject: "no_sched_data missing_wakeup-80", EvidenceID: "unknown-row",
				TypeToken: tc.kind, Predicate: tc.kind, Object: tc.kind, Tier: types.TraceCausalTierDataGap, TraceGapKind: tc.gap,
				Summary: "no scheduler intervals; no matching wakeup; completion unknown", StartTs: 9, EndTs: 10, ImpactMS: 1000}
			projection := types.TraceCausalProjection{WindowStartTs: 1, WindowEndTs: 20, SupportingHops: []types.TraceCausalProjectionNode{node}}
			got := renderAnswerDocTraceDecisionHandoffSet(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}, runtimeTraceGuidanceView{})
			found := false
			for _, line := range strings.Split(got, "\n") {
				if !strings.HasPrefix(line, "- evidence_boundary:") {
					continue
				}
				found = true
				if !strings.Contains(line, "status=`unspecified_evidence_gap`") || !strings.Contains(line, "source_query_window=`not_published`") {
					t.Errorf("unknown acquired a criterion or parent window: %s", line)
				}
				for _, forbidden := range []string{"status=`no_matching", "status=`no_scheduler", "observed_sleep_interval=", "causal_identity=`unresolved`"} {
					if strings.Contains(line, forbidden) {
						t.Errorf("unknown boundary guessed %s: %s", forbidden, line)
					}
				}
			}
			if !found {
				t.Fatal("unknown typed boundary disappeared")
			}
		})
	}
}

func TestHMC309TraceGapDisplayIdentityKeepsSourceAndCriterion(t *testing.T) {
	base := types.TraceCausalProjectionNode{EvidenceID: "gap-a", Subject: "same-80", TypeToken: "trace_gap", Predicate: "trace_gap",
		TraceGapKind: "no_sched_data", QueryWindowStartTs: 1, QueryWindowEndTs: 2,
		MeasurementOrigins: []types.TraceSchedulerMeasurementOrigin{{SourceRef: types.ObservationSourceRef{
			Kind: types.ObservationSourceRuntimeArtifact, Path: "/source-a", PayloadRef: "/payload-a", QueryScopeID: "query-a"}}}}
	for _, tc := range []struct {
		name string
		edit func(*types.TraceCausalProjectionNode)
		want int
	}{
		{"same_receipt_copy", func(n *types.TraceCausalProjectionNode) { n.EvidenceID = "second-lane-copy" }, 1},
		{"different_criterion", func(n *types.TraceCausalProjectionNode) { n.TraceGapKind = "no_eligible_wait" }, 2},
		{"different_source", func(n *types.TraceCausalProjectionNode) { n.MeasurementOrigins[0].SourceRef.Path = "/source-b" }, 2},
		{"different_generation", func(n *types.TraceCausalProjectionNode) { n.MeasurementOrigins[0].SourceRef.PayloadRef = "/payload-b" }, 2},
		{"different_query", func(n *types.TraceCausalProjectionNode) { n.MeasurementOrigins[0].SourceRef.QueryScopeID = "query-b" }, 2},
		{"different_scope", func(n *types.TraceCausalProjectionNode) { n.QueryWindowStartTs = 1.1 }, 2},
		{"unknown_receipt", func(n *types.TraceCausalProjectionNode) { n.MeasurementOrigins = nil; n.EvidenceID = "unknown" }, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := base
			other.MeasurementOrigins = types.CloneTraceSchedulerMeasurementOrigins(base.MeasurementOrigins)
			tc.edit(&other)
			projection := types.TraceCausalProjection{SupportingHops: []types.TraceCausalProjectionNode{base, other}}
			if got := len(traceDecisionEvidenceBoundaries(projection, 8)); got != tc.want {
				t.Fatalf("display boundary count=%d want=%d", got, tc.want)
			}
		})
	}
	unknown := base
	unknown.MeasurementOrigins, unknown.EvidenceID = nil, ""
	if got := len(traceDecisionEvidenceBoundaries(types.TraceCausalProjection{SupportingHops: []types.TraceCausalProjectionNode{unknown, unknown}}, 8)); got != 2 {
		t.Fatalf("unidentified boundaries merged from display equality: %d", got)
	}
}
