package tracequery

import (
	"strings"
	"testing"
)

func TestFrameTimelineKeepsSpanNameRolesAtPipelineStageCaliber(t *testing.T) {
	frame := buildFrameTimelineFromPipeline(Query{}, FramePipelineResult{
		Items: []FramePhaseSummary{
			{Thread: ThreadRef{Comm: "ui", PID: 11}, Phase: "frame_schedule", Name: "Choreographer#doFrame", StartTs: 1, EndTs: 1.01},
			{Thread: ThreadRef{Comm: "rs", PID: 12}, Phase: "render", Name: "RenderService RSUniRender", StartTs: 1.02, EndTs: 1.03},
			{Thread: ThreadRef{Comm: "marker", PID: 13}, Phase: "frame_related", Name: "expected frame 7", StartTs: 1.04, EndTs: 1.05},
		},
	})
	if len(frame.Items) != 3 {
		t.Fatalf("frame items missing: %+v", frame.Items)
	}
	ui := frame.Items[0].RoleAuthority
	if ui == nil || ui.Role != "ui" || ui.Kind != "pipeline_stage_role" ||
		ui.Source != "trace_span_name_semantics" || ui.Confidence <= 0 {
		t.Fatalf("UI stage-role authority missing: %+v", ui)
	}
	rs := frame.Items[1].RoleAuthority
	if rs == nil || rs.Role != "render_service" || rs.Kind != "pipeline_stage_role" {
		t.Fatalf("render-service stage authority missing: %+v", rs)
	}
	marker := frame.Items[2].RoleAuthority
	if marker == nil || marker.Role != "expected" || marker.Kind != "frame_marker_role" {
		t.Fatalf("expected marker must not become a thread role: %+v", marker)
	}
}

func TestFrameTimelineAdjacencyDoesNotMintCausalFlowAuthority(t *testing.T) {
	frame := buildFrameTimelineFromPipeline(Query{}, FramePipelineResult{
		Items: []FramePhaseSummary{
			{Thread: ThreadRef{Comm: "ui", PID: 11}, Phase: "frame_schedule", Name: "Choreographer#doFrame frame=7", StartTs: 1, EndTs: 1.01, StartLine: 3, EndLine: 4},
			{Thread: ThreadRef{Comm: "rs", PID: 12}, Phase: "render", Name: "RenderFrame frame=7", StartTs: 1.02, EndTs: 1.03, StartLine: 5, EndLine: 6},
		},
	})
	if len(frame.Flows) != 1 {
		t.Fatalf("temporal sequence edge missing: %+v", frame.Flows)
	}
	edge := frame.Flows[0]
	if edge.RelationKind != FrameFlowRelationTemporalSequence ||
		edge.RelationSource != FrameFlowSourceSortedSpanAdjacency ||
		edge.CausalityConclusion != FrameFlowCausalityUnproven {
		t.Fatalf("adjacent spans gained causal authority: %+v", edge)
	}
	if !strings.Contains(edge.Summary, "causal_link=unproven") {
		t.Fatalf("text face hid causal ceiling: %q", edge.Summary)
	}
	if len(frame.Caveats) == 0 || !strings.Contains(frame.Caveats[len(frame.Caveats)-1], "temporal_sequence_only") {
		t.Fatalf("frame-level causal ceiling missing: %+v", frame.Caveats)
	}
	facts := evidenceFromFrameTimeline(frame)
	if len(facts) < 3 || facts[2].Predicate != "frame_temporal_sequence" {
		t.Fatalf("evidence lane mislabeled adjacency as causal flow: %+v", facts)
	}
}

func TestFrameTargetRequiresTypedThreadRoleAuthority(t *testing.T) {
	q := Query{TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true}
	unproven := FrameTimelineResult{Items: []FrameTimelineItem{{
		Thread: ThreadRef{Comm: "same-name-candidate", PID: 11},
		Role:   "ui", Phase: "ui_traversal", Name: "same-name-candidate",
		StartTs: 1.1, EndTs: 1.2,
	}}}
	resolution := ResolveFrameTarget(nil, q, unproven)
	if resolution.Target.PID != 0 || len(resolution.Candidates) != 0 {
		t.Fatalf("role string without typed authority must not lock a target: %+v", resolution)
	}

	proven := unproven
	proven.Items[0].RoleAuthority = &FrameRoleAuthority{
		Role: "ui", Kind: "pipeline_stage_role", Source: "trace_span_name_semantics", Confidence: 0.9,
	}
	resolution = ResolveFrameTarget(nil, q, proven)
	if resolution.Target.PID != 11 || resolution.TargetRoleAuthority == nil ||
		resolution.TargetRoleAuthority.Kind != "pipeline_stage_role" ||
		resolution.Source != "frame_timeline_stage_anchor_unique" {
		t.Fatalf("typed UI stage should select only a navigation anchor: %+v", resolution)
	}
}
