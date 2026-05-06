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

func TestBuildAnswerSupportPlan_RootCauseTraceKeepsGuardProtectedAccessOutOfNearestMechanism(t *testing.T) {
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
	var boundaryLane *AnswerSupportLane
	for i := range got.Lanes {
		if got.Lanes[i].Kind == SupportLaneNearestMechanism {
			mechanismLane = &got.Lanes[i]
		}
		if got.Lanes[i].Kind == SupportLaneUncertaintyBound {
			boundaryLane = &got.Lanes[i]
		}
	}
	if mechanismLane != nil && len(mechanismLane.Entries) > 0 {
		t.Fatalf("guard-protected post-check access should not be promoted into nearest_mechanism: %+v", mechanismLane.Entries)
	}
	if boundaryLane == nil || len(boundaryLane.Entries) == 0 {
		t.Fatal("expected uncertainty boundary lane when only a guard survives as grounded mechanism support")
	}
	if !strings.Contains(boundaryLane.Entries[len(boundaryLane.Entries)-1].Text, "only a protective guard") {
		t.Fatalf("weak guard-only case should stay in boundary lane, got %+v", boundaryLane.Entries)
	}
}

func TestBuildAnswerSupportPlan_RootCauseTracePathLanePrefersObservedFrameTransition(t *testing.T) {
	plan := &AnswerSurfacePlan{
		LogObservedAnchors: []LogSourceDriftAnchor{
			{File: "internal/agent/analyzer.go", Func: "buildAnalysisIR", ObservedLine: 250, AnchoredLine: 978},
			{File: "internal/agent/analyzer.go", Func: "(*analyzerEvaluator).ParseOutput", ObservedLine: 320, AnchoredLine: 743},
		},
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
				AnchorKind:   AnchorCall,
				Subject:      "buildAnalysisIR",
				Object:       "RequestModel",
				AnchorSymbol: "RequestModel",
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

	var pathLane *AnswerSupportLane
	for i := range got.Lanes {
		if got.Lanes[i].Kind == SupportLaneCurrentCodePath {
			pathLane = &got.Lanes[i]
			break
		}
	}
	if pathLane == nil {
		t.Fatal("expected current_code_path lane")
	}
	joined := make([]string, 0, len(pathLane.Entries))
	for _, entry := range pathLane.Entries {
		joined = append(joined, entry.Text)
	}
	body := strings.Join(joined, "\n")
	if !strings.Contains(body, "ParseOutput") || !strings.Contains(body, "buildAnalysisIR") {
		t.Fatalf("path lane should keep the observed frame transition, got:\n%s", body)
	}
	if strings.Contains(body, "RequestModel") {
		t.Fatalf("path lane should not elevate intra-function helper calls into the principal path, got:\n%s", body)
	}
}

func TestBuildAnswerSupportPlan_RootCauseTracePromotesIndependentMechanismCompanion(t *testing.T) {
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
				Condition:    "ctx == nil || ctx.Mutable == nil",
			},
			{
				Kind:         EvidenceDirect,
				Source:       "internal/agent/analyzer.go",
				LineStart:    1000,
				AnchorKind:   AnchorAssignment,
				AnchorSymbol: "buildAnalysisIR",
				Snippet:      "rm.AnalyzerHints.PrimaryEntities = append([]string(nil), rm.AnalyzerHints.Entities...)",
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
		t.Fatal("independent inner companion should still promote nearest_mechanism lane")
	}
	if !strings.Contains(strings.Join(func() []string {
		out := make([]string, 0, len(mechanismLane.Entries))
		for _, entry := range mechanismLane.Entries {
			out = append(out, entry.Text)
		}
		return out
	}(), "\n"), "PrimaryEntities") {
		t.Fatalf("mechanism lane should keep the independent companion, got %+v", mechanismLane.Entries)
	}
}

func TestBuildAnswerSupportPlan_RootCauseTraceKeepsControlHeaderAssignmentsOutOfNearestMechanism(t *testing.T) {
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
				Kind:         EvidenceConditional,
				Source:       "internal/agent/analyzer.go",
				LineStart:    1011,
				AnchorKind:   AnchorAssignment,
				AnchorSymbol: "reconcileEnumerationBoundaryScope",
				Snippet:      "if resolved, reason := reconcileEnumerationBoundaryScope(rm, graph); reason != \"\" {",
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

	for _, lane := range got.Lanes {
		if lane.Kind != SupportLaneNearestMechanism {
			continue
		}
		joined := make([]string, 0, len(lane.Entries))
		for _, entry := range lane.Entries {
			joined = append(joined, entry.Text)
		}
		if strings.Contains(strings.Join(joined, "\n"), "reconcileEnumerationBoundaryScope") {
			t.Fatalf("control-header assignment should not enter nearest_mechanism, got %+v", lane.Entries)
		}
	}
}
