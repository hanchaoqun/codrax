package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreCheckArtifactObservedFrameCitations_RejectsObservedLineAsSourceCitation(t *testing.T) {
	ctx := artifactFrameDriftBusContext()
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 250}},
		Blocks: []types.AnswerBlock{{
			ID:   "observed",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "frame",
				Label:       "buildAnalysisIR",
				Text:        "observed runtime frame",
				CitationRef: 0,
			}},
		}},
	}

	hints := preCheckArtifactObservedFrameCitations(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("expected observed-frame citation rejection, got %d: %+v", len(hints), hints)
	}
	if hints[0].Field != "citations[0]" {
		t.Fatalf("hint field = %q, want citations[0]", hints[0].Field)
	}
	if hints[0].ExpectedShape == "" || hints[0].Reason == "" {
		t.Fatalf("hint should explain the typed artifact/source boundary: %+v", hints[0])
	}
	wantShape := "runtime artifact frame coordinates should stay in observed-artifact rows with `citation_ref=-1`; cite the current grounded source anchor instead: internal/agent/analyzer.go:861"
	if hints[0].ExpectedShape != wantShape {
		t.Fatalf("hint shape = %q, want %q", hints[0].ExpectedShape, wantShape)
	}
}

func TestPreCheckArtifactObservedFrameCitations_AllowsCurrentAnchoredLine(t *testing.T) {
	ctx := artifactFrameDriftBusContext()
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 861}},
		Blocks: []types.AnswerBlock{{
			ID:   "current",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "frame",
				Label:       "buildAnalysisIR",
				Text:        "current grounded source anchor",
				CitationRef: 0,
			}},
		}},
	}

	if hints := preCheckArtifactObservedFrameCitations(doc, ctx); len(hints) != 0 {
		t.Fatalf("current anchored source citation should pass, got %+v", hints)
	}
}

func TestPreCheckArtifactObservedFrameCitations_NoAnchorSeedDoesNotRejectCurrentCitation(t *testing.T) {
	logBundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go", Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				Lang:       "go",
				File:       "internal/agent/analyzer.go",
				Line:       250,
				Func:       "buildAnalysisIR",
				Raw:        "buildAnalysisIR at line 250",
				Confidence: 0.95,
			}},
		}},
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
		IntentHint:    types.IntentRootCause,
	}
	mut := types.NewMutableState("")
	mut.SetLogTriage(logBundle)
	ctx := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: logBundle,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
				},
			},
		},
		Mutable: mut,
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 250}},
	}

	if hints := preCheckArtifactObservedFrameCitations(doc, ctx); len(hints) != 0 {
		t.Fatalf("without a typed current-anchor mapping, observed/current line equality should not be rejected here: %+v", hints)
	}
}

func artifactFrameDriftBusContext() *types.BusContext {
	logBundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go", Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				Lang:       "go",
				File:       "internal/agent/analyzer.go",
				Line:       250,
				Func:       "buildAnalysisIR",
				Raw:        "buildAnalysisIR at old build line 250",
				Confidence: 0.95,
			}},
		}},
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
		IntentHint:    types.IntentRootCause,
	}
	mut := types.NewMutableState("")
	mut.SetLogTriage(logBundle)
	evidence := []types.EvidenceItem{{
		ID:              "current-buildAnalysisIR",
		Kind:            types.EvidenceDirect,
		Origin:          types.ClaimOriginCrossSource,
		Source:          "internal/agent/analyzer.go",
		LineStart:       861,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "buildAnalysisIR",
		Subject:         "buildAnalysisIR",
		GroundingStatus: types.GroundingGrounded,
		DriftReason:     types.DriftReasonLineDrift,
	}}
	mut.AppendEvidence(evidence)
	return &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: logBundle,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
				},
			},
		},
		Mutable:       mut,
		EvidenceItems: evidence,
	}
}
