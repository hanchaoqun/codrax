package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreCheckRequiredMechanismAnchors_RequiresStructuredAnchorLabel(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{
			{Text: "runTaskGraph", Kind: types.ContractTermSymbol},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "The mechanism starts from runTaskGraph but lacks a structured anchor item.",
		},
	}}
	hints := preCheckRequiredMechanismAnchors(doc, view)
	if len(hints) != 1 {
		t.Fatalf("hints len=%d want 1: %+v", len(hints), hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "runTaskGraph") {
		t.Fatalf("hint should name missing typed anchor, got %+v", hints[0])
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:          "anchors",
		Kind:        types.BlockOrderedList,
		SurfaceRole: types.SurfacePrincipal,
		ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}},
		Items:       []types.AnswerBlockItem{{Label: "runTaskGraph", CitationRef: 0}},
	})
	if hints := preCheckRequiredMechanismAnchors(doc, view); len(hints) != 0 {
		t.Fatalf("structured anchor item should satisfy contract, got %+v", hints)
	}
}

func TestRunPreEmitChecks_RequiredMechanismAnchorsIntegrated(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Mechanism explanation."},
	}}
	view := &types.AnswerSemanticView{
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{
			{Text: "runTaskGraph", Kind: types.ContractTermSymbol},
		},
	}
	hints := runPreEmitChecks(doc, view, nil)
	if len(hints) == 0 {
		t.Fatal("expected required mechanism anchor hint")
	}
	found := false
	for _, hint := range hints {
		if strings.Contains(hint.ExpectedShape, "runTaskGraph") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("required mechanism anchor hint missing from runPreEmitChecks: %+v", hints)
	}
}

func TestNormalizeRequiredMechanismAnchorCarriers_AppendsCitedAnchorBlock(t *testing.T) {
	mu := types.NewMutableState("required anchor repair")
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "explorer-name",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       32,
		Subject:         "SubExplorer.Name",
		Object:          "explorer",
		AnchorSymbol:    "explorer",
		Snippet:         `return "explorer"`,
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.BusContext{
		Mutable:  mu,
		Language: "zh",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SubTopics: []types.SubTopic{{Summary: "agent"}, {Summary: "tool"}},
		}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric,
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{{
			Text: "explorer",
			Kind: types.ContractTermSymbol,
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "The prose mentions explorer, but prose is not a structured anchor carrier.",
	}}}

	if fixed := normalizeRequiredMechanismAnchorCarriers(doc, view, ctx); fixed != 1 {
		t.Fatalf("fixed=%d want 1", fixed)
	}
	if missing := types.MissingRequiredMechanismAnchors(doc, view.RequiredMechanismAnchors); len(missing) != 0 {
		t.Fatalf("anchor repair should satisfy required anchors, still missing %+v", missing)
	}
	if len(doc.Citations) != 1 || doc.Citations[0].File != "internal/agent/sub_explorer.go" || doc.Citations[0].Line != 32 {
		t.Fatalf("anchor repair should append matching citation, got %+v", doc.Citations)
	}
	if got := doc.Blocks[len(doc.Blocks)-1].Title; got != "关键锚点" {
		t.Fatalf("system-injected anchor title should follow requested language, got %q", got)
	}
	if hints := preCheckItemCitationAlignment(doc, view, ctx); len(hints) != 0 {
		t.Fatalf("repaired anchor citation should align, got %+v", hints)
	}
}

func TestNormalizeRequiredMechanismAnchorCarriers_DoesNotCreateBlockForArchitecture(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "The mechanism is already explained in prose sections.",
	}}}
	view := &types.AnswerSemanticView{
		Family: types.QFArchitecture,
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{{
			Text: "runTaskGraph",
			Kind: types.ContractTermSymbol,
		}},
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		SubTopics: []types.SubTopic{{Summary: "runTaskGraph"}, {Summary: "AnalysisIR"}},
	}}}

	if fixed := normalizeRequiredMechanismAnchorCarriers(doc, view, ctx); fixed != 0 {
		t.Fatalf("architecture mechanism answers should not get system-injected key-anchor blocks, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	for _, block := range doc.Blocks {
		if block.ID == "required_mechanism_anchors" {
			t.Fatalf("system-injected key-anchor block should stay support-only for architecture answers: %+v", block)
		}
	}
}

func TestNormalizeRequiredMechanismAnchorCarriers_EnglishTitle(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "The prose mentions runAnalyzePhase.",
	}}}
	view := &types.AnswerSemanticView{RequiredMechanismAnchors: []types.AnswerRequiredAnchor{{
		Text: "runAnalyzePhase",
		Kind: types.ContractTermSymbol,
	}}}
	view.Family = types.QFGeneric
	ctx := &types.BusContext{
		Language: "en",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SubTopics: []types.SubTopic{{Summary: "analyze"}, {Summary: "retry"}},
		}},
	}

	if fixed := normalizeRequiredMechanismAnchorCarriers(doc, view, ctx); fixed != 1 {
		t.Fatalf("fixed=%d want 1", fixed)
	}
	if got := doc.Blocks[len(doc.Blocks)-1].Title; got != "Key anchors" {
		t.Fatalf("English request should keep English system title, got %q", got)
	}
}
