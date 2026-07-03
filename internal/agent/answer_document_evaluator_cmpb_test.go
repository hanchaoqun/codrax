package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CMP-5 pins (customer compare audit 2026-07-03, docs/design/
// customer_dead_session_audit_20260703.md §7): the trace_query supplement
// folds zero-value blocked_reason walls into one counted line (valued
// observations never fold) and marks observations whose own window sits
// entirely outside the artifact's projection window.

func cmpbZeroBlockedReason(id, subject, locator string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "seven.systrace"},
		ClaimKey:        "critical_blocking:blocked_reason",
		Subject:         subject,
		Predicate:       "critical_blocking",
		Object:          "unknown-thread",
		RichNotes:       []string{"type=blocked_reason", "peer=unknown-thread"},
		SupportRefs:     []string{locator},
	}
}

func cmpbSupplementParse(t *testing.T, observations []types.ObservationRecord) string {
	t.Helper()
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: observations,
	}}})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "trace 分析结论需要保留结构化核对。",
		}},
	})
	ctx := &types.AgentContext{Mutable: mu}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	return out.FinalAnswer
}

// CMP-5a pin: ≥2 zero-value blocked_reason rows fold into ONE counted line
// (count + first thread names); valued critical_blocking rows keep their full
// per-row rendering.
func TestAnswerDocumentEvaluator_TraceQuerySupplementFoldsZeroValueBlockedReason(t *testing.T) {
	observations := []types.ObservationRecord{
		cmpbZeroBlockedReason("zb1", "t1-1", "seven.systrace:32017"),
		cmpbZeroBlockedReason("zb2", "t2-2", "seven.systrace:35150"),
		cmpbZeroBlockedReason("zb3", "t3-3", "seven.systrace:47933"),
		{
			ID:              "valued",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "seven.systrace"},
			ClaimKey:        "critical_blocking:d_state_or_io_wait",
			Subject:         "AsyncSeq-11306",
			Predicate:       "critical_blocking",
			Object:          "unknown-thread",
			Value:           "159.921",
			Unit:            "ms",
			RichNotes:       []string{"type=d_state_or_io_wait", "peer=unknown-thread"},
			SupportRefs:     []string{"seven.systrace:47439-65053"},
		},
	}
	final := cmpbSupplementParse(t, observations)
	if !strings.Contains(final, "系统补充：trace_query 关键观测核对") {
		t.Fatalf("supplement missing:\n%s", final)
	}
	if !strings.Contains(final, "critical_blocking:blocked_reason：共 3 条零时长观测") {
		t.Fatalf("zero-value blocked_reason rows must fold into one counted line:\n%s", final)
	}
	if !strings.Contains(final, "t1-1、t2-2、t3-3") {
		t.Fatalf("fold line must list the first thread names:\n%s", final)
	}
	if strings.Contains(final, "t1-1 -> unknown-thread") {
		t.Fatalf("individual zero-value rows must not render next to the fold line:\n%s", final)
	}
	// The valued observation never folds and keeps its duration.
	if !strings.Contains(final, "critical_blocking:d_state_or_io_wait：AsyncSeq-11306 -> unknown-thread") ||
		!strings.Contains(final, "值=159.921ms") {
		t.Fatalf("valued critical_blocking row must keep its full rendering:\n%s", final)
	}
}

// CMP-5a negative pin: a SINGLE zero-value blocked_reason row keeps the legacy
// per-row rendering (nothing to fold).
func TestAnswerDocumentEvaluator_TraceQuerySupplementKeepsSingleZeroValueRow(t *testing.T) {
	final := cmpbSupplementParse(t, []types.ObservationRecord{
		cmpbZeroBlockedReason("zb1", "t1-1", "seven.systrace:32017"),
	})
	if !strings.Contains(final, "critical_blocking:blocked_reason：t1-1 -> unknown-thread") {
		t.Fatalf("single zero-value row must keep the per-row rendering:\n%s", final)
	}
	if strings.Contains(final, "条零时长观测") {
		t.Fatalf("single zero-value row must not fold:\n%s", final)
	}
}

// CMP-5b pin: an observation whose own window sits entirely OUTSIDE the
// artifact's projection window carries the 窗外观测 note; an intersecting
// observation does not.
func TestAnswerDocumentEvaluator_TraceQuerySupplementMarksOutsideWindowObservations(t *testing.T) {
	observations := []types.ObservationRecord{
		{
			// Precise window anchor (window_source=query_window) — gives the
			// compiled projection its window [3679.4, 3681.5].
			ID:              "frame",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "seven.systrace"},
			Predicate:       "frame_target_resolution",
			Span:            types.ObservationSpan{StartTs: 3679.4, EndTs: 3681.5},
			RichNotes:       []string{"window_source=query_window"},
		},
		{
			// Keeps the projection Active so the window survives compilation.
			ID:              "primary",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "seven.systrace"},
			ClaimKey:        "root_cause_primary",
			Subject:         "busy-1",
			Predicate:       "root_cause_primary",
			Object:          "runnable",
			Value:           "95.912",
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: 100, LineEnd: 200},
			SupportRefs:     []string{"seven.systrace:100-200"},
		},
		{
			ID:              "outside",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "seven.systrace"},
			ClaimKey:        "state_drilldown:Network-50391:d_sleep",
			Subject:         "Network-50391",
			Predicate:       "state_drilldown",
			Object:          "d_sleep",
			Value:           "20764.814",
			Unit:            "ms",
			Span:            types.ObservationSpan{StartTs: 3683.150, EndTs: 3703.915},
			RichNotes:       []string{"source=top_d_state"},
		},
		{
			ID:              "inside",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "seven.systrace"},
			ClaimKey:        "state_drilldown:JankManager-8684:runnable",
			Subject:         "JankManager-8684",
			Predicate:       "state_drilldown",
			Object:          "runnable",
			Value:           "802.250",
			Unit:            "ms",
			Span:            types.ObservationSpan{StartTs: 3680.0, EndTs: 3680.5},
			RichNotes:       []string{"source=top_runnable"},
		},
	}
	final := cmpbSupplementParse(t, observations)
	if !strings.Contains(final, "Network-50391") || !strings.Contains(final, "JankManager-8684") {
		t.Fatalf("both drilldown rows must render:\n%s", final)
	}
	if got := strings.Count(final, "(窗外观测)"); got != 1 {
		t.Fatalf("exactly the fully-outside observation must carry the 窗外观测 note, got %d:\n%s", got, final)
	}
	outsideLine := ""
	for _, line := range strings.Split(final, "\n") {
		if strings.Contains(line, "Network-50391") {
			outsideLine = line
			break
		}
	}
	if !strings.Contains(outsideLine, "(窗外观测)") {
		t.Fatalf("the outside-window row must carry the note:\n%s", outsideLine)
	}
}

// CMP-7b pin (supplement face): a missing_wakeup absence observation shows the
// artifact name without its synthetic line number.
func TestAnswerDocumentEvaluator_TraceQuerySupplementDropsSyntheticMissingWakeupLine(t *testing.T) {
	final := cmpbSupplementParse(t, []types.ObservationRecord{{
		ID:              "miss",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "seven.systrace"},
		ClaimKey:        "root_evidence:missing_wakeup",
		Subject:         "OS_FFRT_2_6-18695",
		Predicate:       "missing_wakeup",
		Value:           "701.000",
		Unit:            "ms",
		Span:            types.ObservationSpan{LineStart: 44},
		SupportRefs:     []string{"seven.systrace:44"},
		Summary:         "sleep interval has no matching sched_wakeup row in the selected trace window",
	}})
	if !strings.Contains(final, "root_evidence:missing_wakeup") || !strings.Contains(final, "seven.systrace") {
		t.Fatalf("missing_wakeup supplement row must render with its artifact name:\n%s", final)
	}
	if strings.Contains(final, "seven.systrace:44") {
		t.Fatalf("missing_wakeup synthetic line locator must not render:\n%s", final)
	}
}

// F1 pin (review 2026-07-04): a synthetic missing_wakeup locator whose path
// slot holds a lane placeholder ("attached_trace"/"trace_query"/
// "runtime_artifact") must KEEP its legacy line display — stripping the line
// suffix would leave a zero-information lane token as the whole locator.
func TestAnswerDocumentEvaluator_TraceQuerySupplementKeepsPlaceholderSyntheticLocator(t *testing.T) {
	final := cmpbSupplementParse(t, []types.ObservationRecord{{
		ID:              "miss-ph",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "trace_query"},
		ClaimKey:        "root_evidence:missing_wakeup",
		Subject:         "OS_FFRT_2_6-18695",
		Predicate:       "missing_wakeup",
		Value:           "701.000",
		Unit:            "ms",
		Span:            types.ObservationSpan{LineStart: 44},
		SupportRefs:     []string{"attached_trace:44"},
		Summary:         "sleep interval has no matching sched_wakeup row in the selected trace window",
	}})
	if !strings.Contains(final, "attached_trace:44") {
		t.Fatalf("placeholder-path synthetic locator must keep its legacy line display:\n%s", final)
	}
}
