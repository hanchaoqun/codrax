package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeTemporalCapsuleRecord(id, subject, predicate, object string, start, end float64, line int) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query:run1",
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: types.ObservationSourceRef{
			Kind:                types.ObservationSourceRuntimeArtifact,
			Path:                "/tmp/materialized.trace",
			CaptureIdentityPath: "/captures/customer.systrace",
		},
		Span: types.ObservationSpan{
			LineStart: line,
			LineEnd:   line + 1,
			StartTs:   start,
			EndTs:     end,
		},
		ClaimKey:  "evidence_fact:" + predicate,
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		Summary:   "call submit wait 完成 — this prose must not classify the relation",
	}
}

func completeRuntimeTemporalCapsuleResult() types.ToolResult {
	rows := []types.ObservationRecord{
		runtimeTemporalCapsuleRecord("i1", "ui-10", "frame_timeline_app", "Begin", 1.000000, 1.001000, 10),
		runtimeTemporalCapsuleRecord("i2", "render-20", "frame_timeline_render", "Draw", 1.001000, 1.003000, 20),
		runtimeTemporalCapsuleRecord("i3", "render-20", "frame_timeline_render", "Flush", 1.003000, 1.004000, 30),
		runtimeTemporalCapsuleRecord("i4", "gpu-30", "frame_timeline_gpu", "Fence", 1.004000, 1.006000, 40),
		runtimeTemporalCapsuleRecord("e1", "ui-10", "frame_temporal_sequence", "render-20", 0, 0, 11),
		runtimeTemporalCapsuleRecord("e2", "render-20", "frame_temporal_sequence", "render-20", 0, 0, 21),
		runtimeTemporalCapsuleRecord("e3", "render-20", "frame_temporal_sequence", "gpu-30", 0, 0, 31),
		// A separately typed row in the same result must not be converted into
		// an arrow merely because its Summary contains relation-like words.
		runtimeTemporalCapsuleRecord("x1", "ui-10", "critical_blocking", "binder-40", 1.0, 1.1, 50),
	}
	return types.ToolResult{
		ToolName:     "trace_query",
		Success:      true,
		Observations: rows,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                       "frame_timeline",
			FrameEvidenceStatus:        "present",
			FrameItemCount:             4,
			FrameFlowEdgeCount:         3,
			FrameFlowRelationAuthority: "temporal_sequence",
			FrameFlowCausalConclusion:  "unproven",
		},
	}
}

func runtimeTemporalCapsuleAnchors(t *testing.T, capsule string) []types.DiagramEdgeAnchor {
	t.Helper()
	const marker = "edge_anchors_json=`"
	start := strings.Index(capsule, marker)
	if start < 0 {
		t.Fatalf("capsule missing anchors: %s", capsule)
	}
	raw := capsule[start+len(marker):]
	end := strings.Index(raw, "`")
	if end < 0 {
		t.Fatalf("capsule has unterminated anchors: %s", capsule)
	}
	var anchors []types.DiagramEdgeAnchor
	if err := json.Unmarshal([]byte(raw[:end]), &anchors); err != nil {
		t.Fatalf("unmarshal anchors: %v\n%s", err, raw[:end])
	}
	return anchors
}

func TestRuntimeTemporalCapsuleRendersCompleteTypedSequence(t *testing.T) {
	capsule, ok := answerDocRuntimeTemporalDiagramCapsule(completeRuntimeTemporalCapsuleResult())
	if !ok {
		t.Fatal("complete report-local typed result did not produce a capsule")
	}
	for _, want := range []string{
		"Copy-ready optional report-local temporal diagram",
		"sequenceDiagram",
		"participant t1 as ui-10",
		"participant t2 as render-20",
		"participant t3 as gpu-30",
		"Note over t1: item_stage_role=app; phase=Begin; interval=1.000000..1.001000; lines=10..11",
		"t1-->>t2: temporal adjacency (unproven)",
		"t2-->>t2: temporal adjacency (unproven)",
		"t2-->>t3: temporal adjacency (unproven)",
	} {
		if !strings.Contains(capsule, want) {
			t.Fatalf("capsule missing %q:\n%s", want, capsule)
		}
	}
	for _, forbidden := range []string{"binder-40", "critical_blocking", "call submit wait"} {
		if strings.Contains(capsule, forbidden) {
			t.Fatalf("capsule promoted non-frame/prose content %q:\n%s", forbidden, capsule)
		}
	}
	if strings.Count(capsule, `"owning_thread_role_authority":"not_provided_by_this_item"`) != 4 ||
		strings.Count(capsule, `"internal_work_authority":"not_provided_by_this_item"`) != 4 ||
		!strings.Contains(capsule, `item_authority_json=`) {
		t.Fatalf("compact item-local semantic ceilings must travel beside the diagram:\n%s", capsule)
	}
	if strings.Contains(capsule, "Note over t1: item_stage_role=app; owning_thread_role_authority") {
		t.Fatalf("authority audit fields must not overload visible Mermaid Notes:\n%s", capsule)
	}
	anchors := runtimeTemporalCapsuleAnchors(t, capsule)
	if len(anchors) != 3 || strings.Count(capsule, "-->>") != len(anchors) {
		t.Fatalf("arrow/anchor cardinality drift: arrows=%d anchors=%d\n%s", strings.Count(capsule, "-->>"), len(anchors), capsule)
	}
	fence := strings.SplitN(capsule, "```mermaid\n", 2)
	if len(fence) != 2 {
		t.Fatalf("capsule missing Mermaid fence:\n%s", capsule)
	}
	body := strings.SplitN(fence[1], "```", 2)[0]
	parsed := mermaidcompat.ParseEdges(body)
	if len(parsed) != len(anchors) {
		t.Fatalf("shared Mermaid parser sees %d edges, anchors=%d:\n%s", len(parsed), len(anchors), body)
	}
	for i, edge := range parsed {
		if edge.From != anchors[i].FromNode || edge.To != anchors[i].ToNode {
			t.Fatalf("parsed edge/anchor mismatch at %d: edge=%+v anchor=%+v", i, edge, anchors[i])
		}
	}
	for _, anchor := range anchors {
		if anchor.RelationKind != types.DiagramRelTemporal {
			t.Fatalf("non-temporal anchor: %+v", anchor)
		}
	}
	if anchors[1].FromNode != "t2" || anchors[1].ToNode != "t2" {
		t.Fatalf("self edge was not preserved: %+v", anchors[1])
	}
}

func TestRuntimeTemporalCapsuleWithholdsIncompleteLocalRows(t *testing.T) {
	result := completeRuntimeTemporalCapsuleResult()
	result.Observations = result.Observations[:6] // four items, only two of three edges
	if capsule, ok := answerDocRuntimeTemporalDiagramCapsule(result); ok || capsule != "" {
		t.Fatalf("incomplete local rows must withhold copy-ready capsule: %s", capsule)
	}
}

func TestRuntimeTemporalCapsuleDoesNotBorrowRowsAcrossReports(t *testing.T) {
	left := completeRuntimeTemporalCapsuleResult()
	right := completeRuntimeTemporalCapsuleResult()
	left.Observations = left.Observations[:6]
	right.Observations = right.Observations[6:]
	if got := renderAnswerDocRuntimeTemporalDiagramCapsulesFromResults([]types.ToolResult{left, right}); got != "" {
		t.Fatalf("separate incomplete reports must not lend rows to each other:\n%s", got)
	}
}

func TestRuntimeTemporalCapsuleDeduplicatesExactRepublication(t *testing.T) {
	result := completeRuntimeTemporalCapsuleResult()
	got := renderAnswerDocRuntimeTemporalDiagramCapsulesFromResults([]types.ToolResult{result, result})
	if count := strings.Count(got, "Copy-ready optional report-local temporal diagram"); count != 1 {
		t.Fatalf("duplicate report emitted %d capsules:\n%s", count, got)
	}
}

func TestRuntimeTemporalCapsuleKeepsReportLocalAuthority(t *testing.T) {
	temporal := completeRuntimeTemporalCapsuleResult()
	proved := completeRuntimeTemporalCapsuleResult()
	proved.TraceEvidenceAuthority = &types.TraceEvidenceAuthority{
		View:                      "wakeup_chain",
		TypedCausalRowCount:       2,
		CausalConclusion:          "bounded_by_typed_rows",
		FrameFlowCausalConclusion: "bounded_by_typed_rows",
	}
	got := renderAnswerDocRuntimeTemporalDiagramCapsulesFromResults([]types.ToolResult{temporal, proved})
	if count := strings.Count(got, "Copy-ready optional report-local temporal diagram"); count != 1 {
		t.Fatalf("proved sibling report changed temporal report-local capsule count=%d:\n%s", count, got)
	}
}

func TestRuntimeTemporalCapsuleRequiresTypedAuthorityAndHardRows(t *testing.T) {
	result := completeRuntimeTemporalCapsuleResult()
	result.TraceEvidenceAuthority = nil
	if _, ok := answerDocRuntimeTemporalDiagramCapsule(result); ok {
		t.Fatal("missing typed authority produced capsule")
	}
	result = completeRuntimeTemporalCapsuleResult()
	result.Observations[0].GroundingPolicy = types.ClaimGroundingSoft
	if _, ok := answerDocRuntimeTemporalDiagramCapsule(result); ok {
		t.Fatal("advisory item row produced a supposedly complete capsule")
	}
}

func TestRuntimeTemporalCapsuleWithholdsOverCapWithoutTruncating(t *testing.T) {
	result := completeRuntimeTemporalCapsuleResult()
	result.TraceEvidenceAuthority.FrameFlowEdgeCount = answerDocRuntimeTemporalCapsuleEdgeLimit + 1
	if _, ok := answerDocRuntimeTemporalDiagramCapsule(result); ok {
		t.Fatal("over-cap authority must be withheld, not truncated")
	}
}

func TestRuntimeTraceGuidanceWiresReportLocalTemporalCapsule(t *testing.T) {
	mut := types.NewMutableState("trace frame")
	mut.AppendDispatchToolResult(completeRuntimeTemporalCapsuleResult())
	ctx := &types.AgentContext{Mutable: mut}
	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	for _, want := range []string{
		"Runtime frame-edge authority hint",
		"Copy-ready optional report-local temporal diagram",
		"t2-->>t2: temporal adjacency (unproven)",
		`"relation_kind":"temporal"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime guidance wiring missing %q:\n%s", want, got)
		}
	}
}
