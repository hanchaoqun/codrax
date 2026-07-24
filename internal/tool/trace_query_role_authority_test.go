package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQueryRendersFrameRoleAuthorityAndCandidateNonRole(t *testing.T) {
	role := &tracequery.FrameRoleAuthority{
		Role: "ui", Kind: "thread_role", Source: "trace_span_name_semantics", Confidence: 0.9,
	}
	result := tracequery.Result{
		View: "frame_root_cause_bundle",
		ThreadSelection: &tracequery.ThreadSelectorResolution{
			Status: "exact_tid_name_mismatch", RequestedPID: 10, RequestedName: "app",
			Selected: tracequery.ThreadRef{Comm: "unknown", PID: 10}, NameMismatch: true,
			NameCandidates: []tracequery.ThreadRef{{Comm: "app", PID: 11}},
			Routing:        "exact_tid_preserved",
		},
		FrameTimeline: &tracequery.FrameTimelineResult{Items: []tracequery.FrameTimelineItem{{
			Index: 1, Thread: tracequery.ThreadRef{Comm: "ui", PID: 11},
			Role: "ui", RoleAuthority: role, Phase: "frame_schedule", Name: "Choreographer#doFrame",
		}}},
		FrameRootCauseBundle: &tracequery.FrameRootCauseBundle{
			Target: tracequery.ThreadRef{Comm: "ui", PID: 11},
			TargetResolution: &tracequery.FrameTargetResolution{
				Target: tracequery.ThreadRef{Comm: "ui", PID: 11},
				Source: "frame_timeline_ui_unique", TargetRoleAuthority: role,
				SelectedFrame: &tracequery.FrameTargetCandidate{
					Thread: tracequery.ThreadRef{Comm: "ui", PID: 11},
					Role:   "ui", RoleAuthority: role,
				},
			},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: result.View}, "path", "")
	for _, want := range []string{
		"role=ui role_kind=thread_role role_source=trace_span_name_semantics role_confidence=0.90",
		"target_role=ui target_role_kind=thread_role",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	observations := traceQueryTypedObservations(result, "path", "payload", "raw", "", time.Unix(0, 0))
	for _, observation := range observations {
		if observation.Predicate == "thread_selector_exact_name_mismatch" {
			notes := strings.Join(observation.RichNotes, " ")
			if !strings.Contains(notes, "name_candidate_role_authority=none") {
				t.Fatalf("name candidate role ceiling missing: %+v", observation)
			}
			return
		}
	}
	t.Fatalf("selector mismatch observation missing: %+v", observations)
}
