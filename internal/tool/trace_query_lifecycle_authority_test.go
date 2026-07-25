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
