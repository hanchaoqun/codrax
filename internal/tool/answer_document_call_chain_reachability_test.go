package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func callChainReachabilityTestEvidence(subject, object, anchor string, line int) types.EvidenceItem {
	return types.EvidenceItem{
		ID:              subject + "-to-" + object,
		Kind:            types.EvidenceRelationship,
		Scope:           types.ScopeLine,
		Source:          "internal/test/path.go",
		LineStart:       line,
		AnchorKind:      types.AnchorCall,
		Subject:         subject,
		Object:          object,
		AnchorSymbol:    anchor,
		GroundingStatus: types.GroundingGrounded,
	}
}

func callChainReachabilityTestView(family types.QuestionFamily, anchors ...string) *types.AnswerSemanticView {
	view := &types.AnswerSemanticView{Family: family}
	for _, anchor := range anchors {
		view.RequiredMechanismAnchors = append(view.RequiredMechanismAnchors, types.AnswerRequiredAnchor{
			Text: anchor, Kind: types.ContractTermSymbol,
		})
	}
	return view
}

func TestCompileCallChainReachability_RequiresDirectedTypedPath(t *testing.T) {
	view := callChainReachabilityTestView(types.QFCallChain, "buildAnalysisIR", "gate.Run")
	evidence := []types.EvidenceItem{
		callChainReachabilityTestEvidence("buildAnalysisIR", "gate.RunWith", "RunWith", 10),
		callChainReachabilityTestEvidence("gate.Run", "gate.RunWith", "RunWith", 20),
	}
	got, active := compileCallChainReachability(view, evidence)
	if !active || got.Proven {
		t.Fatalf("converging edges do not prove source→target reachability: active=%v got=%+v", active, got)
	}

	evidence = append(evidence,
		callChainReachabilityTestEvidence("gate.RunWith", "gate.Run", "Run", 30))
	got, active = compileCallChainReachability(view, evidence)
	if !active || !got.Proven {
		t.Fatalf("typed multi-hop source→target path should prove reachability: active=%v got=%+v", active, got)
	}
}

func TestCompileCallChainReachability_DefinitionsAndSiblingPrefixesAreNotEdges(t *testing.T) {
	view := callChainReachabilityTestView(types.QFCallChain, "buildAnalysisIR", "gate.Run")
	evidence := []types.EvidenceItem{
		callChainReachabilityTestEvidence("buildAnalysisIR", "gate.RunWith", "RunWith", 10),
		{
			ID: "run-definition", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
			Source: "internal/test/path.go", LineStart: 20, AnchorKind: types.AnchorDefinition,
			Subject: "gate.Run", AnchorSymbol: "Run", GroundingStatus: types.GroundingGrounded,
		},
	}
	got, active := compileCallChainReachability(view, evidence)
	if !active || got.Proven {
		t.Fatalf("endpoint definition plus same-prefix sibling call must not prove a path: active=%v got=%+v", active, got)
	}
}

func TestNormalizeCallChainReachabilityAuthority_RewritesUnprovenSummaryAndPrincipalPath(t *testing.T) {
	view := callChainReachabilityTestView(types.QFCallChain, "buildAnalysisIR", "gate.Run")
	ctx := &types.BusContext{Language: "zh", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh"}}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "错误声称 gate.RunWith 调用 gate.Run，形成完整链。"},
		{
			ID: "chain", Kind: types.BlockOrderedList, Text: "完整调用顺序",
			FacetIDs: []string{string(types.FacetCurrentCodePath), string(types.FacetPrincipalPathEdge)},
			Items:    []types.AnswerBlockItem{{Label: "gate.RunWith", Text: "调用 gate.Run", CitationRef: -1}},
		},
		{ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\nA->>B: call"}},
	}}
	evidence := []types.EvidenceItem{
		callChainReachabilityTestEvidence("buildAnalysisIR", "gate.RunWith", "RunWith", 10),
		callChainReachabilityTestEvidence("gate.Run", "gate.RunWith", "RunWith", 20),
	}

	if fixed := normalizeCallChainReachabilityAuthority(doc, view, ctx, evidence); fixed != 2 {
		t.Fatalf("fixed=%d want summary+path rewrite: %+v", fixed, doc.Blocks)
	}
	if !strings.Contains(doc.Blocks[0].Text, "未证明 `buildAnalysisIR` 到 `gate.Run` 的有向调用路径") {
		t.Fatalf("summary must lead with typed unproven reachability: %q", doc.Blocks[0].Text)
	}
	if doc.Blocks[1].Title != "调用链可达性判定" || len(doc.Blocks[1].Items) != 2 ||
		doc.Blocks[1].Items[0].Label != "buildAnalysisIR" || doc.Blocks[1].Items[1].Label != "gate.Run" {
		t.Fatalf("principal path must become exact endpoint boundary: %+v", doc.Blocks[1])
	}
	if doc.Blocks[2].Diagram == nil || doc.Blocks[2].Diagram.Body != "sequenceDiagram\nA->>B: call" {
		t.Fatalf("verified diagram carrier must remain untouched: %+v", doc.Blocks[2])
	}
}

func TestNormalizeCallChainReachabilityAuthority_LeavesProvenAndRootCauseFamiliesUntouched(t *testing.T) {
	evidence := []types.EvidenceItem{callChainReachabilityTestEvidence("A", "B", "B", 10)}
	base := func() *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "original"},
			{ID: "path", Kind: types.BlockOrderedList, FacetIDs: []string{string(types.FacetPrincipalPathEdge)}, Items: []types.AnswerBlockItem{{Label: "A"}, {Label: "B"}}},
		}}
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{}}
	proven := base()
	if fixed := normalizeCallChainReachabilityAuthority(proven, callChainReachabilityTestView(types.QFCallChain, "A", "B"), ctx, evidence); fixed != 0 || proven.Blocks[0].Text != "original" {
		t.Fatalf("proven call path must remain model-authored: fixed=%d doc=%+v", fixed, proven.Blocks)
	}
	rootCause := base()
	if fixed := normalizeCallChainReachabilityAuthority(rootCause, callChainReachabilityTestView(types.QFRootCauseTrace, "A", "B"), ctx, nil); fixed != 0 || rootCause.Blocks[0].Text != "original" {
		t.Fatalf("root-cause/time-window trace family must stay outside call-chain reachability normalizer: fixed=%d doc=%+v", fixed, rootCause.Blocks)
	}
}

func TestNormalizeAnswerDocumentForPreEmit_WiresCallChainReachabilityAuthority(t *testing.T) {
	evidence := []types.EvidenceItem{
		callChainReachabilityTestEvidence("buildAnalysisIR", "gate.RunWith", "RunWith", 10),
		callChainReachabilityTestEvidence("gate.Run", "gate.RunWith", "RunWith", 20),
	}
	mu := types.NewMutableState("call reachability production wiring")
	mu.AppendEvidence(evidence)
	ctx := &types.BusContext{
		Mutable:  mu,
		Language: "en",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace, PredicateAxis: types.AxisCall, Language: "en",
		}},
	}
	view := callChainReachabilityTestView(types.QFCallChain, "buildAnalysisIR", "gate.Run")
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Incorrect complete path."},
		{
			ID: "path", Kind: types.BlockOrderedList,
			FacetIDs: []string{string(types.FacetCurrentCodePath), string(types.FacetPrincipalPathEdge)},
			Items:    []types.AnswerBlockItem{{ID: "wrong", Label: "gate.RunWith", CitationRef: -1}},
		},
	}}

	normalizeAnswerDocumentForPreEmit("emit_answer_document", doc, view, ctx, newPreEmitCheckContext(ctx))
	if !strings.Contains(doc.Blocks[0].Text, "did not prove a directed path") {
		t.Fatalf("production pre-emit chain did not apply typed reachability authority: %+v", doc.Blocks)
	}
	if len(doc.Blocks[1].Items) != 2 || doc.Blocks[1].Items[1].Label != "gate.Run" {
		t.Fatalf("production pre-emit chain did not preserve exact endpoint boundary: %+v", doc.Blocks[1])
	}
	for _, hint := range runPreEmitChecks(doc, view, nil, ctx) {
		if hint.Kind == types.ViolCallChainEndpointOmitted {
			t.Fatalf("reachability authority must keep the exact endpoint hard contract closed: %+v", hint)
		}
	}
}
