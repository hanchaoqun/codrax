package types

import (
	"strings"
	"testing"
)

func TestBuildAnswerSupportPlan_RootCauseTraceCompilesTypedLanes(t *testing.T) {
	plan := &AnswerSurfacePlan{
		ExternalObservationSeeds: []ExternalObservationSeed{
			{Kind: "error_type", Raw: "panic: runtime error"},
			{Kind: "frame", Raw: "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR(0x0)", File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR", AnchoredFile: "internal/agent/analyzer.go", AnchoredLine: 861},
		},
		LogSourceDriftAnchors: []LogSourceDriftAnchor{
			{File: "internal/agent/analyzer.go", Func: "buildAnalysisIR", ObservedLine: 250, AnchoredLine: 861},
		},
		DriftBoundedSurfaceItems: []EvidenceItem{
			{
				Kind:         EvidenceRelationship,
				Source:       "internal/agent/analyzer.go",
				LineStart:    651,
				AnchorKind:   AnchorCall,
				Subject:      "ParseOutput",
				Object:       "buildAnalysisIR",
				AnchorSymbol: "buildAnalysisIR",
			},
			{
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    861,
				AnchorKind:   AnchorCondition,
				AnchorSymbol: "buildAnalysisIR",
				Condition:    "ctx == nil || ctx.Mutable == nil",
			},
		},
	}

	got := BuildAnswerSupportPlan(RequestModel{
		Intent: IntentRootCause,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "panic: runtime error"}},
		},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	if got.Family != QFRootCauseTrace {
		t.Fatalf("family = %q, want %q", got.Family, QFRootCauseTrace)
	}
	if len(got.Lanes) < 3 {
		t.Fatalf("expected at least 3 support lanes, got %d", len(got.Lanes))
	}
	var sawObserved, sawPath, sawMechanism, sawBoundary bool
	var observedTexts []string
	var boundaryTexts []string
	for _, lane := range got.Lanes {
		switch lane.Kind {
		case SupportLaneObservedArtifact:
			sawObserved = len(lane.Entries) > 0
			for _, entry := range lane.Entries {
				observedTexts = append(observedTexts, entry.Text)
			}
		case SupportLaneCurrentCodePath:
			sawPath = len(lane.Entries) > 0
		case SupportLaneNearestMechanism:
			sawMechanism = len(lane.Entries) > 0
		case SupportLaneUncertaintyBound:
			sawBoundary = len(lane.Entries) > 0
			for _, entry := range lane.Entries {
				boundaryTexts = append(boundaryTexts, entry.Text)
			}
		}
	}
	if !sawObserved || !sawPath || !sawBoundary {
		t.Fatalf("missing compiled lanes: observed=%v path=%v mechanism=%v boundary=%v", sawObserved, sawPath, sawMechanism, sawBoundary)
	}
	if sawMechanism {
		t.Fatal("guard-only drift case should not promote a dedicated nearest_mechanism lane")
	}
	joinedObserved := strings.Join(observedTexts, "\n")
	if !strings.Contains(joinedObserved, `runtime artifact includes stack frame "buildAnalysisIR" at observed internal/agent/analyzer.go:250`) {
		t.Fatalf("observed artifact lane should surface structured frame text, got:\n%s", joinedObserved)
	}
	if strings.Contains(joinedObserved, "buildAnalysisIR(0x0)") {
		t.Fatalf("observed artifact lane should not expose raw stack arguments, got:\n%s", joinedObserved)
	}
	var observedGuidance string
	for _, lane := range got.Lanes {
		if lane.Kind == SupportLaneObservedArtifact {
			observedGuidance = lane.Guidance
			break
		}
	}
	if !strings.Contains(observedGuidance, "do not map them to source parameters or caller-side provenance") {
		t.Fatalf("observed artifact guidance missing raw-argument boundary: %q", observedGuidance)
	}
	joinedBoundary := strings.Join(boundaryTexts, "\n")
	if !strings.Contains(joinedBoundary, "current grounded code exposes only a protective guard") {
		t.Fatalf("weak guard-only case should surface a boundary-only note, got:\n%s", joinedBoundary)
	}
}

func TestBuildAnswerSupportPlan_RootCauseTracePromotesNearestMechanismWithCompanion(t *testing.T) {
	plan := &AnswerSurfacePlan{
		DriftBoundedSurfaceItems: []EvidenceItem{
			{
				Kind:         EvidenceRelationship,
				Source:       "internal/agent/analyzer.go",
				LineStart:    743,
				AnchorKind:   AnchorCall,
				Subject:      "ParseOutput",
				Object:       "buildAnalysisIR",
				AnchorSymbol: "buildAnalysisIR",
			},
			{
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    978,
				AnchorKind:   AnchorCondition,
				AnchorSymbol: "buildAnalysisIR",
				Snippet:      "if ctx == nil || ctx.Mutable == nil {",
			},
			{
				Kind:         EvidenceDirect,
				Source:       "internal/agent/analyzer.go",
				LineStart:    981,
				AnchorKind:   AnchorAssignment,
				AnchorSymbol: "buildAnalysisIR",
				Snippet:      "raw := ctx.Mutable.RequestModel()",
			},
		},
	}
	got := BuildAnswerSupportPlan(RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{Errors: []LogError{{Type: "panic: runtime error"}}},
	}, plan)
	if got == nil {
		t.Fatal("expected support plan")
	}
	var mechanismLane *AnswerSupportLane
	for i := range got.Lanes {
		if got.Lanes[i].Kind == SupportLaneNearestMechanism {
			mechanismLane = &got.Lanes[i]
			break
		}
	}
	if mechanismLane == nil || len(mechanismLane.Entries) == 0 {
		t.Fatal("expected dedicated nearest_mechanism lane when a non-guard companion exists")
	}
	if !strings.Contains(mechanismLane.Guidance, "does NOT prove the runtime artifact actually passed") {
		t.Fatalf("nearest mechanism guidance missing runtime-path warning: %q", mechanismLane.Guidance)
	}
}
