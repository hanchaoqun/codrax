package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryPublishesLifecycleSuppressionAuthorityAndObservation(t *testing.T) {
	result := lifecycleAuthorityFixtureResult()
	authority := traceQueryEvidenceAuthority(result)
	if authority == nil || len(authority.LifecycleBoundaries) != 1 {
		t.Fatalf("lifecycle evidence authority missing: %+v", authority)
	}
	boundary := authority.LifecycleBoundaries[0]
	if boundary.ConflictTID != 42 || boundary.BoundaryLine != 20 ||
		boundary.FrameOwnershipStatus != "unavailable" ||
		!containsString(boundary.PreservedLanes, "cpu_busy_idle") {
		t.Fatalf("lifecycle evidence authority drifted: %+v", boundary)
	}

	summary := traceQuerySummary(result, traceQueryParams{View: result.View}, "customer.systrace", "")
	for _, want := range []string{
		"lifecycle_suppression conflict_tid=42",
		"affected_lanes=thread_timeline,wakeup_chain,frame_ownership",
		"preserved_lanes=cpu_busy_idle",
		"suggested_queries=pid=42,line_end=19|pid=42,line_start=20",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}

	observations := traceQueryTypedObservations(result, "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	found := false
	for _, observation := range observations {
		if observation.Predicate != "thread_incarnation_suppression" {
			continue
		}
		found = true
		notes := strings.Join(observation.RichNotes, " ")
		for _, want := range []string{
			"boundary_line=20",
			"frame_ownership_status=unavailable",
			"preserved_lanes=cpu_busy_idle",
			"suggested_queries=pid=42,line_end=19|pid=42,line_start=20",
		} {
			if !strings.Contains(notes, want) {
				t.Fatalf("typed suppression missing %q: %+v", want, observation)
			}
		}
	}
	if !found {
		t.Fatalf("typed lifecycle suppression observation missing: %+v", observations)
	}
}

func TestTraceCoverageLeadsWithLifecycleCauseAndExecutableRemedy(t *testing.T) {
	authority := traceQueryEvidenceAuthority(lifecycleAuthorityFixtureResult())
	block := runtimeTraceCausalProjectionCoverageBlock(types.ObservationLedgerInput{
		ToolResults: []types.ToolResult{{
			ToolName:               "trace_query",
			Success:                true,
			TraceEvidenceAuthority: authority,
			Refinement: &types.ToolRefinementHint{
				ReasonCode: "generic_refinement_limit",
			},
		}},
	}, "zh")
	if block == nil {
		t.Fatal("lifecycle suppression must create a deterministic coverage block")
	}
	for _, want := range []string{
		"suppression_reason=thread_incarnation_conflict",
		"boundary_line=20",
		"affected_lanes=thread_timeline,wakeup_chain,frame_ownership",
		"preserved_lanes=cpu_busy_idle",
		"suggested_queries=pid=42,line_end=19|pid=42,line_start=20",
		"不能把同窗重复探索或通用限流当成首要原因",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("coverage block missing %q:\n%s", want, block.Text)
		}
	}
	lifecycleAt := strings.Index(block.Text, "suppression_reason=thread_incarnation_conflict")
	genericAt := strings.Index(block.Text, "reason_code=generic_refinement_limit")
	if lifecycleAt < 0 || genericAt < 0 || lifecycleAt > genericAt {
		t.Fatalf("precise lifecycle cause must precede generic reason:\n%s", block.Text)
	}
}

func TestTraceCoverageDeduplicatesMergesAndWindowScopesLifecycleBoundaries(t *testing.T) {
	anchor := types.ObservationRecord{
		ID:              "frame-window",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "frame_target_resolution",
		ClaimKey:        "frame_target_resolution:app",
		Subject:         "app-32788",
		Object:          "frame_timeline_ui_unique",
		Span:            types.ObservationSpan{StartTs: 1, EndTs: 2},
		RichNotes:       []string{"window_source=query_window", "window=1.000000..2.000000"},
		Confidence:      1,
	}
	root := types.ObservationRecord{
		ID:              "root",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary",
		Subject:         "worker-1",
		Object:          "runnable",
		Value:           "1.000",
		Unit:            "ms",
		RichNotes:       []string{"tier=primary", "rank=1", "type=runnable", "impact_ms=1.000", "selected_window=1.000000..2.000000"},
		Confidence:      1,
	}
	input := types.ObservationLedgerInput{}
	for i := 0; i < 10; i++ {
		common := types.TraceLifecycleBoundaryAuthority{
			ConflictTID: 42, Signal: "sched_wakeup_new", BoundaryLine: 20, BoundaryTs: 1.2,
			Scope: "pid_keyed", AffectedLanes: []string{"wakeup_chain"},
			CandidateSelectors: []string{"pid=42"},
			SuggestedQueries:   []string{"pid=42,line_end=19"},
		}
		if i == 9 {
			common.Scope = "target_and_global_pid_keyed_aggregates"
			common.AffectsTarget = true
			common.AffectedLanes = []string{"frame_ownership", "wakeup_chain"}
			common.PreservedLanes = []string{"cpu_busy_idle"}
			common.CandidateSelectors = []string{"pid=33410"}
			common.SuggestedQueries = []string{"pid=42,line_start=20"}
			common.FrameOwnershipStatus = "unavailable"
		}
		boundaries := []types.TraceLifecycleBoundaryAuthority{
			common,
			{
				ConflictTID: 100 + i, BoundaryLine: 100 + i, BoundaryTs: 1.30 + float64(i)*0.05,
				Scope: "pid_keyed", AffectedLanes: []string{"thread_timeline"},
			},
			{
				ConflictTID: 500, BoundaryLine: 500, BoundaryTs: 0.5,
				Scope: "pid_keyed", AffectedLanes: []string{"thread_timeline"},
			},
		}
		result := types.ToolResult{
			ToolName: "trace_query", Success: true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
				View: "frame_root_cause_bundle", CausalConclusion: "unproven",
				LifecycleBoundaries: boundaries,
			},
		}
		if i == 0 {
			result.Observations = []types.ObservationRecord{anchor, root}
		}
		input.ToolResults = append(input.ToolResults, result)
	}

	block := runtimeTraceCausalProjectionCoverageBlock(input, "zh")
	if block == nil {
		t.Fatal("lifecycle authority must create a coverage block")
	}
	if got := strings.Count(block.Text, "boundary_line=20 "); got != 1 {
		t.Fatalf("one physical boundary must render once, got %d:\n%s", got, block.Text)
	}
	for _, want := range []string{
		"window_relation=in_window",
		"affects_target=true",
		"frame_ownership_status=unavailable",
		"candidate_selectors=pid=42,pid=33410",
		"suggested_queries=pid=42,line_end=19|pid=42,line_start=20",
		"omitted_unique_boundaries=3",
		"outside_window_boundaries=1",
		"身份审计边界，不是目标线程销毁、重建或重复 incarnation 的证明",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("coverage block missing %q:\n%s", want, block.Text)
		}
	}
	if strings.Contains(block.Text, "boundary_line=500 ") {
		t.Fatalf("out-of-window boundary detail must fold into the typed count:\n%s", block.Text)
	}
}

func TestTraceCoveragePublishesTypedSelectorMismatchWithoutChangingRouting(t *testing.T) {
	mismatch := types.ObservationRecord{
		ID:              "selector-mismatch",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "thread_selector_exact_name_mismatch",
		ClaimKey:        "thread_selector_resolution:unknown-32788",
		Subject:         "unknown-32788",
		Object:          "ss.hm.ugc.aweme",
		RichNotes: []string{
			"selector_status=exact_tid_name_mismatch",
			"requested_pid=32788",
			"requested_name=ss.hm.ugc.aweme",
			"selected_thread=unknown-32788",
			"routing=exact_tid_preserved",
			"name_candidates=[ss.hm.ugc.aweme-33410,ss.hm.ugc.aweme-33411]",
			"name_candidate_role_authority=none",
		},
		Confidence: 1,
	}
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{mismatch},
	}}}
	block := runtimeTraceCausalProjectionCoverageBlock(input, "zh")
	if block == nil {
		t.Fatal("typed selector mismatch must have a deterministic answer channel")
	}
	for _, want := range []string{
		"requested_pid=32788",
		"requested_name=ss.hm.ugc.aweme",
		"selected_thread=unknown-32788",
		"routing=exact_tid_preserved",
		"name_candidates=[ss.hm.ugc.aweme-33410,ss.hm.ugc.aweme-33411]",
		"exact PID 路由未改变",
		"名字候选仅用于诊断且不具备角色权限",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("selector mismatch block missing %q:\n%s", want, block.Text)
		}
	}
	if strings.Contains(block.Text, "没有产出有数据支撑的") {
		t.Fatalf("identity-only disclosure must not manufacture an empty-causality claim:\n%s", block.Text)
	}
}

func lifecycleAuthorityFixtureResult() tracequery.Result {
	return tracequery.Result{
		View: "frame_root_cause_bundle",
		LifecycleSuppressions: []tracequery.TraceLifecycleSuppression{{
			ConflictTID:          42,
			Signal:               "sched_wakeup_new",
			PreviousLine:         10,
			BoundaryLine:         20,
			BoundaryTs:           1.200,
			Scope:                "target_and_global_pid_keyed_aggregates",
			AffectsTarget:        true,
			AffectedLanes:        []string{"thread_timeline", "wakeup_chain", "frame_ownership"},
			PreservedLanes:       []string{"cpu_busy_idle"},
			CandidateSelectors:   []string{"pid=42", "pid=33410"},
			SuggestedQueries:     []string{"pid=42,line_end=19", "pid=42,line_start=20"},
			FrameOwnershipStatus: "unavailable",
		}},
	}
}

func TestRuntimeTraceLifecycleBoundaryDedupeKeepsDistinctPhysicalBoundaries(t *testing.T) {
	// NEGARM (§12.4 不变量负臂, 2026-07-25): 去重不得丢失任何不同物理边界——
	// 同 ConflictTID 不同 BoundaryLine 是两个边界;BoundaryLine<=0 时按
	// ts-fallback key 区分,同 tid 不同 ts 也是两个边界。
	merged := runtimeTraceMergeLifecycleBoundaries(nil, []types.TraceLifecycleBoundaryAuthority{
		{ConflictTID: 42, BoundaryLine: 20, BoundaryTs: 1.2, Signal: "sched_wakeup_new"},
		{ConflictTID: 42, BoundaryLine: 99, BoundaryTs: 1.9, Signal: "sched_wakeup_new"},
		{ConflictTID: 7, BoundaryLine: 0, BoundaryTs: 1.5, Signal: "sched_wakeup_new"},
		{ConflictTID: 7, BoundaryLine: 0, BoundaryTs: 2.5, Signal: "sched_wakeup_new"},
	})
	if len(merged) != 4 {
		t.Fatalf("distinct physical boundaries must all survive dedupe, got %d: %+v", len(merged), merged)
	}
	// Same physical boundary from two queries still collapses to one.
	merged = runtimeTraceMergeLifecycleBoundaries(
		[]types.TraceLifecycleBoundaryAuthority{{ConflictTID: 42, BoundaryLine: 20, BoundaryTs: 1.2}},
		[]types.TraceLifecycleBoundaryAuthority{{ConflictTID: 42, BoundaryLine: 20, BoundaryTs: 1.2, CandidateSelectors: []string{"pid=42"}}},
	)
	if len(merged) != 1 || len(merged[0].CandidateSelectors) != 1 {
		t.Fatalf("identical boundary must collapse with union: %+v", merged)
	}
	// ts-fallback keys must not collide with line-keyed entries of the same tid.
	merged = runtimeTraceMergeLifecycleBoundaries(nil, []types.TraceLifecycleBoundaryAuthority{
		{ConflictTID: 9, BoundaryLine: 30, BoundaryTs: 3.0},
		{ConflictTID: 9, BoundaryLine: 0, BoundaryTs: 3.0},
	})
	if len(merged) != 2 {
		t.Fatalf("line-keyed and ts-fallback boundaries of one tid must stay distinct: %+v", merged)
	}
}

func TestRuntimeTraceCoverageKeepsUnknowableBoundaryOutOfOutsideCount(t *testing.T) {
	// TSZERO (P3-5a, 2026-07-25): 窗已知但 BoundaryTs==0 的边界窗别不可知——
	// 不得被折进 outside_window_boundaries(把猜测当窗判),须以
	// window_relation=unknown 留在展示名册上。
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View: "window_stats",
			LifecycleBoundaries: []types.TraceLifecycleBoundaryAuthority{
				{ConflictTID: 42, BoundaryLine: 20, BoundaryTs: 1.2, Signal: "sched_wakeup_new"},
				{ConflictTID: 7, BoundaryLine: 30, BoundaryTs: 0, Signal: "sched_wakeup_new"},
				{ConflictTID: 9, BoundaryLine: 40, BoundaryTs: 9.9, Signal: "sched_wakeup_new"},
			},
		},
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:tszero#window_stats:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_primary",
			ClaimKey:        "root_cause_primary",
			Subject:         "worker-1",
			Object:          "runnable",
			Value:           "1.000",
			Unit:            "ms",
			RichNotes:       []string{"tier=primary", "rank=1", "type=runnable", "impact_ms=1.000", "selected_window=1.000000..2.000000"},
			Confidence:      1,
		}},
	}}}
	authority := runtimeTraceCoverageAuthority(input)
	if !authority.analysisWindowKnown {
		t.Fatalf("fixture must resolve a unique analysis window: %+v", authority)
	}
	if authority.lifecycleOutside != 1 {
		t.Fatalf("only the ts=9.9 boundary is provably outside, got outside=%d", authority.lifecycleOutside)
	}
	if len(authority.lifecycleBoundaries) != 2 {
		t.Fatalf("in-window + unknowable boundaries must stay displayed, got %+v", authority.lifecycleBoundaries)
	}
	var sawUnknowable bool
	for _, boundary := range authority.lifecycleBoundaries {
		if boundary.ConflictTID == 7 {
			sawUnknowable = true
			if got := runtimeTraceLifecycleWindowRelation(boundary, authority); got != "unknown" {
				t.Fatalf("ts=0 boundary relation = %q, want unknown", got)
			}
		}
	}
	if !sawUnknowable {
		t.Fatalf("unknowable boundary vanished from the roster: %+v", authority.lifecycleBoundaries)
	}
}

func TestRuntimeTraceCoverageZeroProjectionAdoptsConsistentLedgerWindow(t *testing.T) {
	// NG-3 (§13.4): 零投影恰是最需要窗判的降级运行(第四放 4 个窗内边界全
	// 标 unknown)。ledger 全部 typed selected_window 记录一致(容差内)才
	// 采信——非投票非并集非猜;不一致维持 unknown。
	record := func(id, predicate string, notes ...string) types.ObservationRecord {
		return types.ObservationRecord{
			ID:              id,
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       predicate,
			Subject:         "worker",
			Object:          "state",
			RichNotes:       notes,
			Confidence:      1,
		}
	}
	boundaries := []types.TraceLifecycleBoundaryAuthority{
		{ConflictTID: 42, BoundaryLine: 20, BoundaryTs: 1.2, Signal: "sched_wakeup_new"},
		{ConflictTID: 9, BoundaryLine: 40, BoundaryTs: 9.9, Signal: "sched_wakeup_new"},
	}
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                "window_stats",
			LifecycleBoundaries: boundaries,
		},
		Observations: []types.ObservationRecord{
			record("a", "scheduler_head_coverage", "selected_window=1.000000..2.000000"),
			record("b", "vsync_generator_census", "selected_window=1.000000..2.000000"),
		},
	}}}
	ledger := types.CompileObservationLedger(input)
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) > 1 ||
		(len(set.Projections) == 1 && set.Projections[0].WindowEndTs > set.Projections[0].WindowStartTs) {
		t.Fatalf("fixture guard: expected an anchorless (windowless) ledger, got %+v", set.Projections)
	}
	authority := runtimeTraceCoverageAuthority(input)
	if !authority.analysisWindowKnown {
		t.Fatalf("consistent ledger windows must resolve the analysis window: %+v", authority)
	}
	if authority.lifecycleOutside != 1 || len(authority.lifecycleBoundaries) != 1 ||
		authority.lifecycleBoundaries[0].ConflictTID != 42 {
		t.Fatalf("window verdicts drifted: outside=%d displayed=%+v", authority.lifecycleOutside, authority.lifecycleBoundaries)
	}
	if got := runtimeTraceLifecycleWindowRelation(authority.lifecycleBoundaries[0], authority); got != "in_window" {
		t.Fatalf("in-window boundary relation = %q", got)
	}
	// 不一致窗 → 维持 unknown(禁猜)。
	input.ToolResults[0].Observations = []types.ObservationRecord{
		record("a", "scheduler_head_coverage", "selected_window=1.000000..2.000000"),
		record("b", "vsync_generator_census", "selected_window=5.000000..6.000000"),
	}
	authority = runtimeTraceCoverageAuthority(input)
	if authority.analysisWindowKnown {
		t.Fatalf("disagreeing ledger windows must stay unknown: %+v", authority)
	}
	if len(authority.lifecycleBoundaries) != 2 || authority.lifecycleOutside != 0 {
		t.Fatalf("unknown window must keep every boundary displayed: %+v", authority)
	}
}

func TestTraceCoveragePublishesTargetWindowStateAccount(t *testing.T) {
	// GAP-B1 (§13.3): 覆盖块正面陈述目标窗内状态账(第四放留给模型叙事的
	// 真空)——主导状态+五车道+ SYM-2 的症状/可拆解自因边界。
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View: "frame_root_cause_bundle",
			LifecycleBoundaries: []types.TraceLifecycleBoundaryAuthority{{
				ConflictTID: 50173, Signal: "sched_wakeup_new", BoundaryLine: 52108, BoundaryTs: 69326.875412,
			}},
		},
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:x#target_window_states",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "target_window_states",
			ClaimKey:        "target_window_states:unknown-32788",
			Subject:         "unknown-32788",
			Object:          "state_partition",
			Value:           "227.000",
			Unit:            "ms",
			RichNotes: []string{
				types.TraceNoteKeyRunning + "=1.200",
				types.TraceNoteKeyRunnable + "=0.800",
				types.TraceNoteKeySleep + "=210.000",
				types.TraceNoteKeySleepIOWait + "=0.000",
				types.TraceNoteKeyDState + "=15.000",
				types.TraceNoteKeyIOWait + "=0.000",
				types.TraceNoteKeyWindowMS + "=227.367",
			},
			Confidence: 0.8,
		}},
	}}}
	block := runtimeTraceCausalProjectionCoverageBlock(input, "zh")
	if block == nil {
		t.Fatal("coverage block missing")
	}
	for _, want := range []string{
		"目标窗内状态账: unknown-32788 窗227.367ms",
		"主导状态=sleep 210.000ms(92.4%)",
		"d_state=15.000ms",
		"休眠、等锁或等对端属于待下钻的症状，不直接占根因排序席",
		"唤醒后的可运行等待、已确认的 D/IO 阻塞及有正向提升量的运行供给仍可按其 typed 证据进入排序",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("state account line missing %q:\n%s", want, block.Text)
		}
	}
}

func TestTraceCoverageTargetStateBoundaryKeepsDecomposableSelfCauses(t *testing.T) {
	authority := runtimeTraceCoverageAuthorityBoundary{targetStates: []runtimeTraceCoverageTargetState{{
		subject: "app-100", windowMS: 7, running: 1.2, runnable: 0.8, sleep: 5,
	}}}
	for _, tc := range []struct {
		zh   bool
		want []string
		not  []string
	}{
		{true,
			[]string{"休眠、等锁或等对端属于待下钻的症状", "可运行等待", "D/IO 阻塞", "运行供给仍可", "进入排序"},
			[]string{"等待型自身状态是症状面", "不作为可消除影响参与根因排序席位"}},
		{false,
			[]string{"sleep, lock wait, and peer wait are drill-down symptoms", "post-wakeup runnable delay", "typed D/IO blocking", "positive compute-supply opportunity", "enter the ranking"},
			[]string{"the target's own wait states are the symptom face", "never hold eliminable root-cause seats"}},
	} {
		got := runtimeTraceCoverageAuthorityText(authority, tc.zh, true)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Fatalf("zh=%v missing %q:\n%s", tc.zh, want, got)
			}
		}
		for _, bad := range tc.not {
			if strings.Contains(got, bad) {
				t.Fatalf("zh=%v retained overbroad target-self wording %q:\n%s", tc.zh, bad, got)
			}
		}
	}
}
