package types

import "testing"

func TestAgentContextAnswerSurfacePlanCacheReturnsDefensiveCopies(t *testing.T) {
	ctx := &AgentContext{
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent: IntentTrace,
			},
		},
		EvidenceItems: []EvidenceItem{{
			ID:           "ev-1",
			Kind:         EvidenceMechanism,
			Subject:      "Caller",
			Object:       "Callee",
			AnchorSymbol: "Callee",
			Source:       "internal/demo.go",
			LineStart:    12,
		}},
	}

	first := BuildAnswerSurfacePlanForAgentContext(ctx)
	if first == nil {
		t.Fatal("first surface plan is nil")
	}
	first.SurfaceEvidence = nil
	first.StepBackbone = append(first.StepBackbone, StepSurfaceAnchor{Name: "mutated"})

	second := BuildAnswerSurfacePlanForAgentContext(ctx)
	if second == nil {
		t.Fatal("second surface plan is nil")
	}
	if len(second.SurfaceEvidence) != 1 {
		t.Fatalf("cached surface plan leaked caller mutation: SurfaceEvidence len=%d", len(second.SurfaceEvidence))
	}
	for _, anchor := range second.StepBackbone {
		if anchor.Name == "mutated" {
			t.Fatalf("cached surface plan leaked caller-added StepBackbone: %+v", second.StepBackbone)
		}
	}
}

func TestAgentContextAnswerSemanticAndSupportPlanCachesReturnCopies(t *testing.T) {
	logBundle := &LogBundle{
		Errors: []LogError{{
			Type: "runtime error",
			Frames: []LogFrame{{
				File:       "internal/demo.go",
				Line:       12,
				Func:       "Callee",
				Raw:        "internal/demo.go:12 Callee",
				Confidence: 0.9,
			}},
		}},
		ResolvedFiles: []string{"internal/demo.go"},
	}
	ctx := &AgentContext{
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent:    IntentRootCause,
				LogTriage: logBundle,
			},
		},
		LogTriage: logBundle,
		EvidenceItems: []EvidenceItem{{
			ID:           "ev-1",
			Kind:         EvidenceMechanism,
			Subject:      "Caller",
			Object:       "Callee",
			AnchorSymbol: "Callee",
			Source:       "internal/demo.go",
			LineStart:    12,
		}},
	}

	view := BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil {
		t.Fatal("semantic view is nil")
	}
	view.RequiredBlocks = nil
	if again := BuildAnswerSemanticViewForAgentContext(ctx); again == nil || len(again.RequiredBlocks) == 0 {
		t.Fatalf("semantic view cache leaked caller mutation: %+v", again)
	}

	support := BuildAnswerSupportPlanForAgentContext(ctx)
	if support == nil {
		t.Fatal("support plan is nil")
	}
	support.Lanes = nil
	if again := BuildAnswerSupportPlanForAgentContext(ctx); again == nil || len(again.Lanes) == 0 {
		t.Fatalf("support plan cache leaked caller mutation: %+v", again)
	}
}

func TestBusContextAnswerPlanCacheInvalidatesOnMutableRevision(t *testing.T) {
	mut := &MutableState{}
	bus := &BusContext{
		Mutable: mut,
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent: IntentTrace,
			},
		},
		EvidenceItems: []EvidenceItem{{
			ID:           "ev-1",
			Kind:         EvidenceMechanism,
			Subject:      "Caller",
			Object:       "Callee",
			AnchorSymbol: "Callee",
			Source:       "internal/demo.go",
			LineStart:    12,
		}},
	}

	first := BuildAnswerSurfacePlanForBusContext(bus)
	if first == nil {
		t.Fatal("first surface plan is nil")
	}
	first.SurfaceEvidence = nil
	if again := BuildAnswerSurfacePlanForBusContext(bus); again == nil || len(again.SurfaceEvidence) != 1 {
		t.Fatalf("bus surface cache leaked caller mutation: %+v", again)
	}

	mut.AppendEvidence([]EvidenceItem{{
		ID:           "ev-2",
		Kind:         EvidenceMechanism,
		Subject:      "Other",
		Object:       "Target",
		AnchorSymbol: "Target",
		Source:       "internal/other.go",
		LineStart:    21,
	}})
	afterRevision := BuildAnswerSurfacePlanForBusContext(bus)
	if afterRevision == nil {
		t.Fatal("surface plan after mutable revision is nil")
	}
	if len(afterRevision.SurfaceEvidence) < 2 {
		t.Fatalf("bus surface cache did not invalidate on mutable revision: SurfaceEvidence len=%d", len(afterRevision.SurfaceEvidence))
	}
}

func TestBusContextAnswerSupportPlanCacheReturnsDefensiveCopies(t *testing.T) {
	logBundle := &LogBundle{
		Errors: []LogError{{
			Type: "runtime error",
			Frames: []LogFrame{{
				File:       "internal/demo.go",
				Line:       12,
				Func:       "Callee",
				Raw:        "internal/demo.go:12 Callee",
				Confidence: 0.9,
			}},
		}},
		ResolvedFiles: []string{"internal/demo.go"},
	}
	bus := &BusContext{
		Mutable: &MutableState{},
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{
				Intent:    IntentRootCause,
				LogTriage: logBundle,
			},
		},
		EvidenceItems: []EvidenceItem{{
			ID:           "ev-1",
			Kind:         EvidenceMechanism,
			Subject:      "Caller",
			Object:       "Callee",
			AnchorSymbol: "Callee",
			Source:       "internal/demo.go",
			LineStart:    12,
		}},
	}
	bus.Mutable.SetLogTriage(logBundle)

	view := BuildAnswerSemanticViewForBusContext(bus)
	if view == nil {
		t.Fatal("bus semantic view is nil")
	}
	view.RequiredBlocks = nil
	if again := BuildAnswerSemanticViewForBusContext(bus); again == nil || len(again.RequiredBlocks) == 0 {
		t.Fatalf("bus semantic view cache leaked caller mutation: %+v", again)
	}

	support := BuildAnswerSupportPlanForBusContext(bus)
	if support == nil {
		t.Fatal("bus support plan is nil")
	}
	support.Lanes = nil
	if again := BuildAnswerSupportPlanForBusContext(bus); again == nil || len(again.Lanes) == 0 {
		t.Fatalf("bus support plan cache leaked caller mutation: %+v", again)
	}
}
