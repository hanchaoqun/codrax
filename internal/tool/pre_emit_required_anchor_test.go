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

func TestRunPreEmitChecks_CallChainEndpointMissingIsTypedHard(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "gate.RunWith is the observed callee.",
	}}}
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{
			{Text: "gate.Run", Kind: types.ContractTermSymbol},
		},
	}
	hints := runPreEmitChecks(doc, view, nil)
	found := false
	for _, hint := range hints {
		if hint.Kind == types.ViolCallChainEndpointOmitted &&
			strings.Contains(hint.ExpectedShape, "gate.Run") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing exact call-chain endpoint must use its typed violation: %+v", hints)
	}
	hard, _ := splitPreEmitHintsByGate(hints)
	if len(hard) != 1 || hard[0].Kind != types.ViolCallChainEndpointOmitted {
		t.Fatalf("typed call-chain endpoint omission must be same-turn hard: %+v", hard)
	}

	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: "endpoint", Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{
			Label: "gate.Run",
			Text:  "The collected typed call-edge evidence did not prove a path to this exact endpoint.",
		}},
	})
	for _, hint := range runPreEmitChecks(doc, view, nil) {
		if hint.Kind == types.ViolCallChainEndpointOmitted {
			t.Fatalf("exact structured endpoint should close the typed hard gate: %+v", hint)
		}
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

func TestNormalizeRequiredMechanismAnchorCarriers_CallChainMaterializesExactUncitedBoundaries(t *testing.T) {
	mu := types.NewMutableState("call-chain endpoints")
	mu.AppendEvidence([]types.EvidenceItem{
		{
			ID: "build", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
			Subject: "buildAnalysisIR", AnchorSymbol: "buildAnalysisIR",
			Source: "internal/agent/analyzer.go", LineStart: 10,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID: "runwith", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
			Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorSymbol: "RunWith",
			Source: "internal/agent/analyzer.go", LineStart: 20,
			GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.BusContext{
		Mutable:  mu,
		Language: "en",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
		}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{
			{Text: "buildAnalysisIR", Kind: types.ContractTermSymbol},
			{Text: "gate.Run", Kind: types.ContractTermSymbol},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "Call path."}}}

	if fixed := normalizeRequiredMechanismAnchorCarriers(doc, view, ctx); fixed != 2 {
		t.Fatalf("fixed=%d want 2: %+v", fixed, doc.Blocks)
	}
	block := doc.Blocks[len(doc.Blocks)-1]
	if len(block.Items) != 2 {
		t.Fatalf("endpoint block items=%+v", block.Items)
	}
	if block.Items[0].Label != "buildAnalysisIR" ||
		!strings.Contains(block.Items[0].Text, "resolved this exact requested endpoint") {
		t.Fatalf("exact evidence endpoint should be disclosed as resolved: %+v", block.Items[0])
	}
	if block.Items[1].Label != "gate.Run" ||
		!strings.Contains(block.Items[1].Text, "did not resolve this exact requested endpoint") {
		t.Fatalf("sibling-only evidence must be disclosed as unresolved: %+v", block.Items[1])
	}
	for _, item := range block.Items {
		if item.CitationRef != -1 {
			t.Fatalf("system endpoint identity rows must not borrow sibling citations: %+v", item)
		}
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("call-chain endpoint materialization must not synthesize citations: %+v", doc.Citations)
	}
	for _, hint := range runPreEmitChecks(doc, view, nil, ctx) {
		if hint.Kind == types.ViolCallChainEndpointOmitted {
			t.Fatalf("materialized exact endpoint rows should satisfy hard gate: %+v", hint)
		}
	}
}

func TestPreEmitExactEvidenceResolvesRequiredAnchor_DoesNotUseSamePrefixSibling(t *testing.T) {
	evidence := []types.EvidenceItem{{
		Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorSymbol: "RunWith",
	}}
	if preEmitExactEvidenceResolvesRequiredAnchor("gate.Run", evidence) {
		t.Fatal("gate.RunWith must not resolve exact endpoint gate.Run")
	}
	if !preEmitExactEvidenceResolvesRequiredAnchor("buildAnalysisIR", evidence) {
		t.Fatal("exact evidence subject should resolve the endpoint")
	}
}
