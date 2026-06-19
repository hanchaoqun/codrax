package reasoninggraph

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEventsFromToolResultObservationsProjectsRuntimeArtifactEvidence(t *testing.T) {
	result := types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:       "trace-root-cause-1",
			Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query",
			SourceRef: types.ObservationSourceRef{
				Kind:       types.ObservationSourceRuntimeArtifact,
				ArtifactID: "attached-hitrace",
				RawRef:     "blob:trace",
			},
			SupportRefs: []string{"span:1.00-1.20"},
			Confidence:  0.91,
		}},
	}

	events := EventsFromToolResultObservations("graph-aux", "tool:trace_query", result)
	view := ReduceEvents(events)
	if len(events) != 1 || len(view.AuxiliaryEvents) != 1 {
		t.Fatalf("aux events = events %+v view %+v", events, view)
	}
	got := view.AuxiliaryEvents[0]
	if got.Kind != ReasoningEventAuxiliaryEvidenceProjected ||
		got.ExternalOrigin != string(types.AnswerEvidenceOriginRuntimeArtifact) ||
		got.SourceKind != string(types.ObservationSourceRuntimeArtifact) ||
		got.Producer != "trace_query" ||
		got.ObservationID != "trace-root-cause-1" {
		t.Fatalf("aux summary = %+v", got)
	}
	if events[0].NodeKind != ReasoningNodeLogTrace {
		t.Fatalf("node kind = %q, want log_trace", events[0].NodeKind)
	}
	if len(view.EvidenceRefs) != 2 || view.EvidenceRefs[0].Confidence != "high" {
		t.Fatalf("evidence refs = %+v", view.EvidenceRefs)
	}
	audit := AuditSummaryFromEvents(events, "aux_test", 5)
	if audit == nil || laneCount(audit.Lanes, "auxiliary") != 1 || len(audit.AuxiliaryEvents) != 1 {
		t.Fatalf("audit = %+v", audit)
	}
}

func TestEventsFromMCPResponseObservationsProjectsExternalResourceBoundary(t *testing.T) {
	resp := types.MCPResponse{
		ServerName:  "docs",
		Method:      "read",
		ResourceURI: "mcp://docs/file",
		PayloadRef:  "payload:docs",
		Observations: []types.MCPTypedObservation{{
			ResourceURI: "mcp://docs/file#section",
			PayloadRef:  "payload:docs-section",
			JSONPointer: "/items/0",
			Row:         7,
		}},
	}

	events := EventsFromMCPResponseObservations("graph-mcp", "mcp:docs", resp)
	view := ReduceEvents(events)
	if len(events) != 1 || len(view.AuxiliaryEvents) != 1 {
		t.Fatalf("mcp events = events %+v view %+v", events, view)
	}
	got := view.AuxiliaryEvents[0]
	if got.Kind != ReasoningEventMCPObservationProjected ||
		got.ExternalOrigin != string(types.AnswerEvidenceOriginMCPResource) ||
		got.SourceKind != string(types.ObservationSourceMCPResource) ||
		got.ResourceURI != "mcp://docs/file#section" ||
		got.PayloadRef != "payload:docs-section" {
		t.Fatalf("mcp summary = %+v", got)
	}
	if events[0].NodeKind != ReasoningNodeOperation {
		t.Fatalf("node kind = %q, want operation", events[0].NodeKind)
	}
	if len(events[0].ArtifactRefs) < 2 {
		t.Fatalf("artifact refs = %+v", events[0].ArtifactRefs)
	}
}
