package tool

import (
	"encoding/json"
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

func TestNormalizeAnswerDocumentForPreEmit_DoesNotAuthorRequiredMechanismAnchors(t *testing.T) {
	ctx := &types.BusContext{
		Mutable:  types.NewMutableState("call-chain endpoints"),
		Language: "en",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
		}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		RequiredMechanismAnchors: []types.AnswerRequiredAnchor{{
			Text: "gate.Run", Kind: types.ContractTermSymbol,
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "Call path.",
	}}}
	before, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal before: %v", err)
	}

	normalizeAnswerDocumentForPreEmit("test", doc, view, ctx, newPreEmitCheckContext(ctx))
	after, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("pre-emit normalization must not author visible endpoint rows or prose:\nbefore=%s\nafter=%s", before, after)
	}
	foundHard := false
	for _, hint := range runPreEmitChecks(doc, view, nil, ctx) {
		if hint.Kind == types.ViolCallChainEndpointOmitted {
			foundHard = preEmitHintHardByDefault(hint)
		}
	}
	if !foundHard {
		t.Fatal("missing call-chain endpoint must remain a model-actionable typed hard hint")
	}
}
