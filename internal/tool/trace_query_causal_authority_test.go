package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryEvidenceAuthorityWithdrawsFrameCausality(t *testing.T) {
	result := tracequery.Result{
		View:              "frame_root_cause_bundle",
		PrioritySemantics: "HarmonyOS/hitrace user-space priority: larger numeric value means higher priority; 1-40=CFS, 41-159=RT.",
		FrameRootCauseBundle: &tracequery.FrameRootCauseBundle{
			FrameTimeline: &tracequery.FrameTimelineResult{},
			RootCauseRank: &tracequery.RootCauseRankResult{
				Items: []tracequery.RootCauseRankItem{{Rank: 0, Tier: tracequery.RootCauseTierContextOnly}},
			},
			Caveats: []string{"thread_identity_fail_closed=true; thread_incarnation_conflict"},
		},
	}
	authority := traceQueryEvidenceAuthority(result)
	if authority == nil ||
		authority.FrameEvidenceStatus != "unavailable" ||
		authority.FrameItemCount != 0 ||
		authority.TypedCausalRowCount != 0 ||
		authority.CausalConclusion != "unproven" {
		t.Fatalf("authority boundary drifted: %+v", authority)
	}
}

func TestTraceQueryEvidenceAuthorityKeepsTemporalFrameEdgesUnproven(t *testing.T) {
	result := tracequery.Result{
		View: "frame_flow",
		FrameTimeline: &tracequery.FrameTimelineResult{
			Items: []tracequery.FrameTimelineItem{{Index: 1}, {Index: 2}},
			Flows: []tracequery.FrameFlowEdge{{
				FromIndex:           1,
				ToIndex:             2,
				RelationKind:        tracequery.FrameFlowRelationTemporalSequence,
				RelationSource:      tracequery.FrameFlowSourceSortedSpanAdjacency,
				CausalityConclusion: tracequery.FrameFlowCausalityUnproven,
			}},
		},
	}
	authority := traceQueryEvidenceAuthority(result)
	if authority == nil ||
		authority.FrameEvidenceStatus != "present" ||
		authority.FrameFlowEdgeCount != 1 ||
		authority.FrameFlowRelationAuthority != tracequery.FrameFlowRelationTemporalSequence ||
		authority.FrameFlowCausalConclusion != tracequery.FrameFlowCausalityUnproven ||
		authority.CausalConclusion != "unproven" {
		t.Fatalf("temporal edge authority drifted: %+v", authority)
	}
}

func TestTraceCausalCoveragePublishesTemporalFrameEdgeCeiling(t *testing.T) {
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                       "frame_flow",
			FrameEvidenceStatus:        "present",
			FrameFlowEdgeCount:         3,
			FrameFlowRelationAuthority: tracequery.FrameFlowRelationTemporalSequence,
			FrameFlowCausalConclusion:  tracequery.FrameFlowCausalityUnproven,
			CausalConclusion:           "unproven",
		},
	}}}
	block := runtimeTraceCausalProjectionCoverageBlock(input, "zh")
	if block == nil {
		t.Fatal("typed temporal frame-edge ceiling must create a deterministic coverage block")
	}
	for _, want := range []string{
		"frame_flow_causality=unproven",
		"relation=temporal_sequence",
		"edges=3",
		"不能升级为已确认的跨线程因果 flow",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("coverage block missing %q:\n%s", want, block.Text)
		}
	}
}

func TestTraceCausalCoverageDoesNotSumRepeatedFrameViewEdgeCounts(t *testing.T) {
	authority := func(view string, edges int) *types.TraceEvidenceAuthority {
		return &types.TraceEvidenceAuthority{
			View:                       view,
			FrameEvidenceStatus:        "present",
			FrameFlowEdgeCount:         edges,
			FrameFlowRelationAuthority: tracequery.FrameFlowRelationTemporalSequence,
			FrameFlowCausalConclusion:  tracequery.FrameFlowCausalityUnproven,
			CausalConclusion:           "unproven",
		}
	}
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{
		{ToolName: "trace_query", Success: true, TraceEvidenceAuthority: authority("frame_timeline", 1)},
		{ToolName: "trace_query", Success: true, TraceEvidenceAuthority: authority("frame_flow", 1)},
		{ToolName: "trace_query", Success: true, TraceEvidenceAuthority: authority("frame_timeline", 3)},
		{ToolName: "trace_query", Success: true, TraceEvidenceAuthority: authority("frame_flow", 3)},
	}}
	got := runtimeTraceCoverageAuthority(input)
	if got.frameFlowEdgeCount != 3 {
		t.Fatalf("repeated per-view frame census was summed: edges=%d, want most complete view=3", got.frameFlowEdgeCount)
	}
	block := runtimeTraceCausalProjectionCoverageBlock(input, "zh")
	if block == nil || !strings.Contains(block.Text, "edges=3") || strings.Contains(block.Text, "edges=8") {
		t.Fatalf("coverage block must publish the most complete single-view census:\n%+v", block)
	}
}

func TestTraceCausalCoverageBlockPublishesAuthorityCeiling(t *testing.T) {
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                "frame_root_cause_bundle",
			FrameEvidenceStatus: "unavailable",
			CausalConclusion:    "unproven",
		},
	}}}
	block := runtimeTraceCausalProjectionCoverageBlock(input, "zh")
	if block == nil {
		t.Fatal("typed authority boundary must create a deterministic coverage block")
	}
	for _, want := range []string{
		"frame_causality=unproven",
		"frame_evidence_status=unavailable",
		"不能证明具体丢帧因果",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("coverage block missing %q:\n%s", want, block.Text)
		}
	}
}

func TestTraceCausalCoverageLocalRefinementDoesNotOverridePublishedCausalRows(t *testing.T) {
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		Refinement: &types.ToolRefinementHint{
			ReasonCode: "trace_query_event_search_zero_match",
		},
	}}}
	if block := runtimeTraceCausalProjectionCoverageBlockForProjection(input, "zh", true, true); block != nil {
		t.Fatalf("query-local zero match must not become a report-wide no-causal-row claim: %+v", block)
	}
}

func TestTraceCausalCoverageFrameUnprovenKeepsTypedChainAndBackgroundAuthoritySeparate(t *testing.T) {
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                "frame_root_cause_bundle",
			FrameEvidenceStatus: "absent",
			CausalConclusion:    "unproven",
		},
	}}}
	block := runtimeTraceCausalProjectionCoverageBlockForProjection(input, "zh", true, true)
	if block == nil {
		t.Fatal("frame authority must remain visible beside a typed chain projection")
	}
	for _, want := range []string{
		"frame_causality=unproven",
		"typed 唤醒/阻塞链只支持所选窗口内的链上候选与可消除量",
		"无链上凭证的调度、IO、频率观察仍只能作为邻近或背景",
		"不证明具体丢帧因果",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("typed chain/frame boundary missing %q:\n%s", want, block.Text)
		}
	}
	if strings.Contains(block.Text, "未获得可绑定到目标的 frame/deadline 证据或 typed causal row") ||
		strings.Contains(block.Text, "调度、IO、频率观察只能描述窗口背景") {
		t.Fatalf("typed chain rows were demoted by the no-chain wording:\n%s", block.Text)
	}

	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		OnChainCauses: []types.TraceCausalProjectionNode{{Subject: "worker-7", ChainRelevance: "on_chain"}},
	}}}
	if !runtimeTraceProjectionSetHasTypedChainRows(set) {
		t.Fatal("an exact on-chain seat must select the typed-chain authority wording")
	}
	set.Projections[0].OnChainCauses = nil
	set.Projections[0].AdjacentCauses = []types.TraceCausalProjectionNode{{Subject: "worker-7", ChainRelevance: "adjacent"}}
	if runtimeTraceProjectionSetHasTypedChainRows(set) {
		t.Fatal("an adjacent-only projection must not gain typed-chain authority")
	}
}

func TestTraceCausalProjectionMaterializationUsesTypedQuestionAuthority(t *testing.T) {
	generic := newBusForMutationTest()
	generic.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		Scenario:      types.ScenarioGeneric,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
	}}
	empty := types.TraceCausalProjectionSet{}
	if runtimeTraceCausalProjectionMaterializationAllowed(generic, empty) {
		t.Fatal("generic non-diagnostic trace fact must not materialize an empty causal projection")
	}

	start, end := 34579.45, 34579.50
	generic.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "34579.45..34579.50",
	}
	if !runtimeTraceCausalProjectionMaterializationAllowed(generic, empty) {
		t.Fatal("exact explicit-window trace analysis must retain causal projection authority")
	}

	generic.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = nil
	semanticOnly := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Subject:        "VerifyClass com.example.Foo",
			SemanticClass:  "class_verification",
			ChainRelevance: "background",
		}},
	}}}
	if runtimeTraceCausalProjectionMaterializationAllowed(generic, semanticOnly) {
		t.Fatal("off-chain semantic optimization rows must not mint a causal report for a generic trace fact")
	}
	if !runtimeTraceProjectionSetHasCausalRows(semanticOnly) {
		t.Fatal("semantic rows must remain visible to the broad coverage/content predicate")
	}
	if runtimeTraceProjectionSetHasPublicationGradeCausalRows(semanticOnly) {
		t.Fatal("semantic-only projection unexpectedly gained publication-grade causal authority")
	}

	withCausalRows := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		PrimaryRootCause: &types.TraceCausalProjectionNode{Subject: "ui-thread"},
	}}}
	if !runtimeTraceCausalProjectionMaterializationAllowed(generic, withCausalRows) {
		t.Fatal("compiled causal rows must remain publishable for every question family")
	}

	generic.AnalysisIR.RequestModel.Predicates.IsDiagnosticQuestion = true
	if !runtimeTraceCausalProjectionMaterializationAllowed(generic, empty) {
		t.Fatal("typed diagnostic questions need the empty causal-authority boundary")
	}
}

func TestTraceCausalProjectionExplicitWindowRetainsSemanticOnlyProjection(t *testing.T) {
	ctx := newBusForMutationTest()
	start, end := 10.0, 10.010
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:   types.IntentExplain,
		Scenario: types.ScenarioGeneric,
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
			RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart:      &start,
			TimeEnd:        &end,
			SourceQuote:    "10.000..10.010",
		},
	}}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Subject:        "VerifyClass",
			SemanticClass:  "class_verification",
			ChainRelevance: "background",
		}},
	}}}
	if !runtimeTraceCausalProjectionMaterializationAllowed(ctx, set) {
		t.Fatal("explicit typed time windows must retain causal projection materialization")
	}
}

func TestTraceCausalProjectionGenericEmptyAuthorityDoesNotCreateReport(t *testing.T) {
	ctx := newBusForMutationTest()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		Scenario:      types.ScenarioGeneric,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
	}}
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:             "event_search",
			CausalConclusion: "unproven",
		},
	})
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	if materializeRuntimeTraceCausalProjectionBlock(doc, ctx) {
		t.Fatalf("generic empty trace authority unexpectedly created causal report: %+v", doc.Blocks)
	}
	if len(doc.Blocks) != 0 {
		t.Fatalf("generic empty trace authority mutated document: %+v", doc.Blocks)
	}
}

func TestTraceQuerySummaryDistinguishesUnavailableAndMeasuredZeroCPU(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{
			SchedulerHeadCoverage: &tracequery.SchedulerHeadCoverage{
				Status: "unknown", Reason: "thread_incarnation_conflict",
				SubjectCensusStatus: "not_evaluated",
			},
			CPU: []tracequery.CPUStats{
				{CPU: 0, BusyIdleStatus: tracequery.CPUBusyIdleStatusUnavailable, BusyIdleReason: "no_sched_switch_observation", Frequency: 1000},
				{CPU: 1, BusyMs: 0, IdleMs: 10, BusyIdleStatus: tracequery.CPUBusyIdleStatusMeasured},
			},
			CoreTopology: []tracequery.CoreClassStats{
				{
					Class: "big", CPUs: []int{0},
					BusyIdleStatus: tracequery.CPUBusyIdleStatusUnavailable,
					BusyIdleReason: "no_measured_cpu_busy_idle",
					MaxFrequency:   1000, ComputeSupplySignal: "class_frequency_observed",
				},
				{
					Class: "small", CPUs: []int{1}, BusyMs: 0, IdleMs: 10,
					BusyIdleStatus: tracequery.CPUBusyIdleStatusMeasured,
				},
			},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "")
	for _, want := range []string{
		"subject_census=not_evaluated missing_cpus=not_evaluated missing_threads=not_evaluated",
		"cpu=0 core_class= busy=unavailable idle=unavailable busy_idle_status=unavailable",
		"cpu=1 core_class= busy=0.000ms idle=10.000ms busy_idle_status=measured",
		"core_class=big cpus=[0] busy=unavailable idle=unavailable busy_idle_status=unavailable busy_idle_reason=no_measured_cpu_busy_idle",
		"core_class=small cpus=[1] busy=0.000ms idle=10.000ms busy_idle_status=measured",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "cpu=0 core_class= busy=0.000ms") {
		t.Fatalf("unavailable CPU was rendered as measured zero:\n%s", summary)
	}
	if strings.Contains(summary, "core_class=big cpus=[0] busy=0.000ms") {
		t.Fatalf("unavailable core class was rendered as measured zero:\n%s", summary)
	}
}

func TestTraceQueryEvidenceAuthorityAbsentArmDistinctFromUnavailable(t *testing.T) {
	// TESTS-2 (2026-07-24, §9 fixture 7 字面形单元封口): frame 类视图、零帧、
	// 无 withdrawal caveat → absent(诚实缺席,非撤销)+ unproven;同形加一条
	// withdrawal caveat → unavailable。二臂机械可区分。
	result := tracequery.Result{
		View:          "frame_timeline",
		FrameTimeline: &tracequery.FrameTimelineResult{},
	}
	authority := traceQueryEvidenceAuthority(result)
	if authority == nil || authority.FrameEvidenceStatus != "absent" ||
		authority.FrameItemCount != 0 || authority.CausalConclusion != "unproven" {
		t.Fatalf("absent arm drifted: %+v", authority)
	}
	result.FrameTimeline.Caveats = []string{"thread_incarnation_conflict pid=50173"}
	authority = traceQueryEvidenceAuthority(result)
	if authority == nil || authority.FrameEvidenceStatus != "unavailable" ||
		authority.CausalConclusion != "unproven" {
		t.Fatalf("withdrawal arm drifted: %+v", authority)
	}
}

func TestTraceQueryEvidenceAuthorityIgnoresUnrelatedTypedLifecycleWithdrawal(t *testing.T) {
	result := tracequery.Result{
		View: "frame_timeline",
		FrameTimeline: &tracequery.FrameTimelineResult{
			Caveats: []string{"thread_identity_fail_closed=true; thread_incarnation_conflict pid=50173"},
		},
		LifecycleSuppressions: []tracequery.TraceLifecycleSuppression{{
			ConflictTID: 50173, BoundaryLine: 200, BoundaryTs: 1.2,
			Scope: "global_pid_keyed_aggregates", AffectsTarget: false,
			AffectedLanes:        []string{"pid_tid_scheduler_aggregates"},
			FrameOwnershipStatus: "not_applicable",
		}},
	}
	authority := traceQueryEvidenceAuthority(result)
	if authority == nil || authority.FrameEvidenceStatus != "absent" ||
		authority.CausalConclusion != "unproven" {
		t.Fatalf("unrelated typed lifecycle boundary must remain honest absence: %+v", authority)
	}

	result.LifecycleSuppressions[0].AffectsTarget = true
	result.LifecycleSuppressions[0].FrameOwnershipStatus = "unavailable"
	authority = traceQueryEvidenceAuthority(result)
	if authority == nil || authority.FrameEvidenceStatus != "unavailable" {
		t.Fatalf("target-affecting typed lifecycle boundary must withdraw frame authority: %+v", authority)
	}

	result.LifecycleSuppressions[0].AffectsTarget = false
	result.LifecycleSuppressions[0].FrameOwnershipStatus = "not_applicable"
	result.FrameTimeline.Caveats = []string{"lifecycle_audit_truncated=true; fail_closed"}
	authority = traceQueryEvidenceAuthority(result)
	if authority == nil || authority.FrameEvidenceStatus != "unavailable" {
		t.Fatalf("incomplete lifecycle audit must remain fail-closed: %+v", authority)
	}
}

func TestTraceQueryEvidenceAuthorityGenericFailClosedArmHonorsTypedRoster(t *testing.T) {
	// NW2-03b (NG-2, §13.4): typed roster 在场且 affects_target=false 时,
	// resource/pairing 族 fail_closed 词形不得把诚实 absent 翻成 unavailable
	// (第四放 unavailable 疑此铸);目标专属 thread_identity_target_fail_closed
	// 仍应撤销;无 roster 的 legacy 形保持保守。
	base := func(caveat string) tracequery.Result {
		return tracequery.Result{
			View:          "frame_timeline",
			FrameTimeline: &tracequery.FrameTimelineResult{Caveats: []string{caveat}},
			LifecycleSuppressions: []tracequery.TraceLifecycleSuppression{{
				ConflictTID: 50173, Signal: "sched_wakeup_new",
				BoundaryLine: 52108, BoundaryTs: 69326.875412,
				AffectsTarget: false, FrameOwnershipStatus: "not_applicable",
			}},
		}
	}
	unrelated := traceQueryEvidenceAuthority(base("thread_identity_resource_fail_closed=true; task-incarnation boundary crosses the window"))
	if unrelated == nil || unrelated.FrameEvidenceStatus != "absent" {
		t.Fatalf("resource fail_closed must not override the typed roster verdict: %+v", unrelated)
	}
	targeted := traceQueryEvidenceAuthority(base("thread_identity_target_fail_closed=true; target lifecycle scope is not unique"))
	if targeted == nil || targeted.FrameEvidenceStatus != "unavailable" {
		t.Fatalf("target-specific fail_closed must still withdraw: %+v", targeted)
	}
	legacy := traceQueryEvidenceAuthority(tracequery.Result{
		View:          "frame_timeline",
		FrameTimeline: &tracequery.FrameTimelineResult{Caveats: []string{"binder_pairing_fail_closed=true"}},
	})
	if legacy == nil || legacy.FrameEvidenceStatus != "unavailable" {
		t.Fatalf("roster-less legacy shape must stay conservative: %+v", legacy)
	}
}
