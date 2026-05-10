package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// G5-1 (post_v2_runtime_gap_remediation, 2026-05-04). Unit tests for
// SemanticQualityReviewer + the typed input projection helper.

// fakeAdapter mirrors the pattern used by self-consistency tests —
// returns a canned tool-call response.
type fakeSemanticQualityAdapter struct {
	wantParams string
}

func (f *fakeSemanticQualityAdapter) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	return llm.Response{
		ToolCalls: []llm.ToolCall{
			{
				Name:   "emit_semantic_quality_review",
				Params: json.RawMessage(f.wantParams),
			},
		},
	}, nil
}

func (f *fakeSemanticQualityAdapter) ModelID() string               { return "fake" }
func (f *fakeSemanticQualityAdapter) MaxContextTokens() int         { return 1000 }
func (f *fakeSemanticQualityAdapter) MaxOutputTokens() int          { return 1000 }
func (f *fakeSemanticQualityAdapter) RequestTimeout() time.Duration { return time.Second }
func (f *fakeSemanticQualityAdapter) RetryMaxAttempts() int         { return 1 }

func TestSemanticQualityReviewer_NilAdapterDisabled(t *testing.T) {
	r := NewSemanticQualityReviewer(nil)
	got, err := r.Review(context.Background(), SemanticQualityInput{
		OriginalRequest: "x", AnswerSummary: "y", AnswerBody: "z",
	})
	if err != nil || got != nil {
		t.Errorf("nil adapter must yield (nil, nil); got (%+v, %v)", got, err)
	}
}

func TestSemanticQualityReviewer_HappyPathSufficientTrue(t *testing.T) {
	adapter := &fakeSemanticQualityAdapter{
		wantParams: `{"sufficient": true, "confidence": 0.9}`,
	}
	r := NewSemanticQualityReviewer(adapter)
	got, err := r.Review(context.Background(), SemanticQualityInput{
		OriginalRequest: "what does X do?",
		AnswerSummary:   "X does Y",
		AnswerBody:      "1. detailed point",
	})
	if err != nil {
		t.Fatalf("review err: %v", err)
	}
	if got == nil || !got.Sufficient || got.Confidence != 0.9 {
		t.Errorf("got %+v, want Sufficient=true conf=0.9", got)
	}
}

func TestSemanticQualityReviewer_ConcernsRequireBothPartsOfThePair(t *testing.T) {
	adapter := &fakeSemanticQualityAdapter{
		wantParams: `{
			"sufficient": false,
			"confidence": 0.92,
			"concerns": [
				{"topic": "facet:principal_mechanism", "observation": "body lists API surfaces but does not name the dispatching call site",
				 "suggestion": "add a body item naming the call edge with file:line"},
				{"topic": "weak", "observation": "", "suggestion": "fix"}
			]
		}`,
	}
	r := NewSemanticQualityReviewer(adapter)
	got, err := r.Review(context.Background(), SemanticQualityInput{AnswerSummary: "x", AnswerBody: "y"})
	if err != nil {
		t.Fatalf("review err: %v", err)
	}
	if got.Sufficient {
		t.Error("Sufficient=false expected")
	}
	if len(got.Concerns) != 1 {
		t.Errorf("empty-field concern must drop; got %d concerns: %+v", len(got.Concerns), got.Concerns)
	}
	if got.Concerns[0].Topic != "facet:principal_mechanism" {
		t.Errorf("concern Topic = %q, want facet:principal_mechanism", got.Concerns[0].Topic)
	}
}

func TestSemanticQualityReviewer_RejectsContradictoryEmit(t *testing.T) {
	// sufficient=false but no concerns named — reviewer emit is
	// itself inconsistent; promote to err so caller drops verdict.
	adapter := &fakeSemanticQualityAdapter{
		wantParams: `{"sufficient": false, "confidence": 0.9}`,
	}
	r := NewSemanticQualityReviewer(adapter)
	_, err := r.Review(context.Background(), SemanticQualityInput{AnswerSummary: "x", AnswerBody: "y"})
	if err == nil {
		t.Error("sufficient=false with empty concerns must err")
	}
}

func TestSemanticQualityReviewer_RejectsConfidenceOutOfRange(t *testing.T) {
	adapter := &fakeSemanticQualityAdapter{
		wantParams: `{"sufficient": true, "confidence": 1.5}`,
	}
	r := NewSemanticQualityReviewer(adapter)
	_, err := r.Review(context.Background(), SemanticQualityInput{AnswerSummary: "x", AnswerBody: "y"})
	if err == nil {
		t.Error("confidence>1 must err")
	}
}

// G5-1 SST locks for the typed projection helper.
func TestBuildSemanticQualityInput_FacetCoverageProjection(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, FacetIDs: []string{"current_code_path"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "current_code_path", Required: types.FacetSoftRequired,
					PromotionPolicy: types.PromotionWhenEvidenceSufficient,
					SourceCandidate: []string{"ev-a"}}, // promoted=true
				{Kind: "principal_mechanism", Required: types.FacetHardRequired,
					PromotionPolicy: types.PromotionAlwaysHard}, // promoted=true, covered=false
			},
			Optional: []types.FacetRequirement{
				{Kind: "diagram_spine", Required: types.FacetOptional,
					PromotionPolicy: types.PromotionAdvisoryOnly,
					SourceCandidate: []string{"ev-b", "ev-c"}}, // richness candidate
			},
		},
	}
	in := BuildSemanticQualityInput("orig", "summary", "body", doc, view, nil)
	if len(in.RequiredFacets) != 2 {
		t.Errorf("RequiredFacets len = %d, want 2: %+v", len(in.RequiredFacets), in.RequiredFacets)
	}
	// First entry: current_code_path is promoted + covered.
	if !in.RequiredFacets[0].Promoted || !in.RequiredFacets[0].Covered {
		t.Errorf("current_code_path: promoted=%t covered=%t (want both true): %+v", in.RequiredFacets[0].Promoted, in.RequiredFacets[0].Covered, in.RequiredFacets[0])
	}
	// Second entry: principal_mechanism promoted=true (essential always)
	// covered=false.
	if !in.RequiredFacets[1].Promoted || in.RequiredFacets[1].Covered {
		t.Errorf("principal_mechanism: promoted=%t covered=%t (want true, false): %+v", in.RequiredFacets[1].Promoted, in.RequiredFacets[1].Covered, in.RequiredFacets[1])
	}
	// Richness candidate: 1 entry, evidence_count=2.
	if len(in.RichnessCandidates) != 1 || in.RichnessCandidates[0].EvidenceCount != 2 {
		t.Errorf("RichnessCandidates: %+v (want 1 entry with EvidenceCount=2)", in.RichnessCandidates)
	}
}

func TestBuildSemanticQualityInput_DiagramContractProjection(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "d1", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Body: "flowchart LR\n  A --> B\n"},
				EdgeAnchors: []types.DiagramEdgeAnchor{
					{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall},
				}},
		},
	}
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			EdgeRelations: []types.DiagramEdgeRelationContract{
				{Kind: types.DiagramRelCall, Min: 1, ClaimForm: types.ClaimCallEdge},
				{Kind: types.DiagramRelGuard, Min: 1, ClaimForm: types.ClaimGuardCondition},
			},
		},
	}
	in := BuildSemanticQualityInput("o", "s", "b", doc, view, nil)
	if in.DiagramContract == nil || !in.DiagramContract.Required {
		t.Fatal("DiagramContract must be set for required diagram")
	}
	if !in.DiagramContract.BlockPresent {
		t.Error("BlockPresent must be true when doc has BlockDiagram")
	}
	if len(in.DiagramContract.Edges) != 2 {
		t.Fatalf("Edges len = %d, want 2", len(in.DiagramContract.Edges))
	}
	// Call: typed RelationKind=Call has 1 anchor → MinSatisfied=1.
	if in.DiagramContract.Edges[0].MinSatisfied != 1 {
		t.Errorf("Call MinSatisfied = %d, want 1", in.DiagramContract.Edges[0].MinSatisfied)
	}
	// Guard: no anchor → MinSatisfied=0, MinExpected=1 → reviewer
	// will see a gap.
	if in.DiagramContract.Edges[1].MinSatisfied != 0 || in.DiagramContract.Edges[1].MinExpected != 1 {
		t.Errorf("Guard min_satisfied/min_expected = %d/%d, want 0/1",
			in.DiagramContract.Edges[1].MinSatisfied, in.DiagramContract.Edges[1].MinExpected)
	}
}

func TestBuildSemanticQualityInput_NilGuards(t *testing.T) {
	in := BuildSemanticQualityInput("o", "s", "b", nil, nil, nil)
	if len(in.RequiredFacets) != 0 || in.DiagramContract != nil || len(in.RichnessCandidates) != 0 {
		t.Errorf("nil doc/view must yield no projections; got %+v", in)
	}
}

// G5-2: shouldReviewSemanticQuality gate.
//
// P4 (2026-05-10) prefilter additions: tests now seed FacetIDs so
// the new "no-facet-declared → skip" branch doesn't make every
// fixture vacuously skip.
func TestShouldReviewSemanticQuality_GateSemantics(t *testing.T) {
	body := []types.AnswerBlock{
		{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
		{ID: "b1", Kind: types.BlockOrderedList, FacetIDs: []string{"enumeration_item"}, Items: []types.AnswerBlockItem{{ID: "i1", Label: "x"}}},
	}
	docFull := &types.AnswerDocumentV2{Blocks: body}

	// Single-block summary-only — skip.
	docSummaryOnly := &types.AnswerDocumentV2{Blocks: body[:1]}
	if shouldReviewSemanticQuality(docSummaryOnly, nil) {
		t.Error("single-block doc should skip reviewer")
	}

	// Body present + no hard violations + facet declared → review.
	if !shouldReviewSemanticQuality(docFull, nil) {
		t.Error("body present + facet declared + no hard violations → reviewer should fire")
	}

	// Hard violation present → skip.
	hardViolations := []types.Violation{
		{Kind: types.ViolFacetUncovered, Detail: "x"},
	}
	if shouldReviewSemanticQuality(docFull, hardViolations) {
		t.Error("hard violation present → reviewer should skip")
	}

	// SOFT violation present → review still fires.
	softViolations := []types.Violation{
		{Kind: types.ViolRichnessRegression, Detail: "x"},
	}
	if !shouldReviewSemanticQuality(docFull, softViolations) {
		t.Error("only SOFT violations → reviewer should fire")
	}
}

// P4 (2026-05-10) — no-facet-declared prefilter. A doc with body
// blocks but ZERO declared facets (no block.facet_ids[] and no
// claim_uses[].facet_id) cannot have the AnchoredCount/DeclaredCount
// gap the reviewer is looking for; skip the dispatch.
func TestShouldReviewSemanticQuality_P4_NoFacetDeclared_Skip(t *testing.T) {
	docNoFacets := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "i1", Label: "x"}}},
		},
	}
	if shouldReviewSemanticQuality(docNoFacets, nil) {
		t.Error("doc with zero declared facets should skip reviewer (P4 prefilter)")
	}

	// Same shape but with one block.facet_ids declared → review.
	docWithFacet := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
			{ID: "b1", Kind: types.BlockOrderedList, FacetIDs: []string{"enumeration_item"}, Items: []types.AnswerBlockItem{{ID: "i1", Label: "x"}}},
		},
	}
	if !shouldReviewSemanticQuality(docWithFacet, nil) {
		t.Error("doc with declared facet should fire reviewer")
	}

	// Facet declared via claim_uses[].facet_id (alternate path) → review.
	docWithClaimUseFacet := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
			{ID: "b1", Kind: types.BlockOrderedList,
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact, FacetID: "current_code_path"}},
				Items:     []types.AnswerBlockItem{{ID: "i1", Label: "x"}}},
		},
	}
	if !shouldReviewSemanticQuality(docWithClaimUseFacet, nil) {
		t.Error("doc with claim_uses[].facet_id should fire reviewer")
	}
}

// P4 — SemanticQualityMinConfidenceDefault constant pinned to 0.92.
func TestSemanticQualityMinConfidenceDefault_P4(t *testing.T) {
	if SemanticQualityMinConfidenceDefault != 0.92 {
		t.Errorf("P4 default floor should be 0.92, got %v", SemanticQualityMinConfidenceDefault)
	}
}

// R4 lock — reviewer prompts + retry hint surface must NOT contain
// stage codenames or Go type names.
func TestSemanticQualityReviewer_Prompts_NoLLMFacingJargon(t *testing.T) {
	bannedTokens := []string{
		"finalizer", "extractor", "explorer", "analyzer",
		"FacetCoverageContract", "FacetRequirement", "AnswerSemanticView",
		"orchestrator", "TaskGraph", "BusContext",
		"Phase ", "Tier-1", "Tier-2", "Layer ",
	}
	corpus := semanticQualityReviewerSystemPrompt + "\n" + string(semanticQualityTool.Parameters) + "\n" + semanticQualityTool.Description
	for _, banned := range bannedTokens {
		if strings.Contains(corpus, banned) {
			t.Errorf("LLM-facing prompt contains banned token %q", banned)
		}
	}
}

func TestRenderSemanticQualityUserMessage_UsesPublicFacetLabels(t *testing.T) {
	in := SemanticQualityInput{
		RequiredFacets: []SemanticFacetSummary{
			{Kind: "diagram_spine", Tier: "expected", Promoted: true, Covered: false},
		},
		RichnessCandidates: []SemanticRichnessSummary{
			{Kind: "current_code_path", EvidenceCount: 2},
		},
		SystemDetectedGaps: []SystemDetectedGap{
			{Kind: gapKindEnrichmentUncovered, FacetKind: "diagram_spine", EvidenceCount: 3},
		},
		PromotedFacetCoverage: []FacetCoverageDepth{
			{Kind: "diagram_spine", DeclaredCount: 1, AnchoredCount: 0},
		},
	}
	msg := renderSemanticQualityUserMessage(in)
	if strings.Contains(msg, "`diagram_spine`") || strings.Contains(msg, "kind=`diagram_spine`") {
		t.Fatalf("reviewer user message must not expose raw facet ids; got\n%s", msg)
	}
	for _, want := range []string{
		`facet="Diagram facet (every node grounded in a citation; relationships supported by typed claim annotations)"`,
		`facet="Current code path (cite the live source file:line that proves what the code does today)"`,
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("reviewer user message missing public facet label %q; got\n%s", want, msg)
		}
	}
}

func TestCountFacetCoverageDepth_ListFacetCountsIdentifierAnchors(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "path",
			Kind:        types.BlockOrderedList,
			FacetIDs:    []string{"current_code_path"},
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{
				{ID: "h1", Label: "buildAnalysisIR", CitationRef: 0},
				{ID: "h2", Label: "analyzerEvaluator.ParseOutput", CitationRef: 1},
			},
		}},
		Citations: []types.Citation{
			{File: "internal/agent/analyzer.go", Line: 977},
			{File: "internal/agent/analyzer.go", Line: 743},
		},
	}
	declared, anchored := countFacetCoverageDepth(doc, "current_code_path")
	if declared != 1 || anchored != 1 {
		t.Fatalf("current_code_path list facet should count citation-backed identifier rows as anchored; got declared=%d anchored=%d", declared, anchored)
	}
}
