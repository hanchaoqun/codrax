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
