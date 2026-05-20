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

type fakeSemanticQualityReviewer struct {
	verdict *SemanticQualityResult
	err     error
	seen    *SemanticQualityInput
}

func (f fakeSemanticQualityReviewer) Review(_ context.Context, in SemanticQualityInput) (*SemanticQualityResult, error) {
	if f.seen != nil {
		*f.seen = in
	}
	return f.verdict, f.err
}

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
				{"topic": "facet:principal_mechanism", "kind": "coverage_gap", "observation": "body lists API surfaces but does not name the dispatching call site",
				 "suggestion": "add a body item naming the call edge with file:line", "repair_locus": "evidence_gap"},
				{"topic": "weak", "kind": "coverage_gap", "observation": "", "suggestion": "fix", "repair_locus": "local_doc_defect"}
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
	if got.Concerns[0].Kind != semanticConcernCoverageGap {
		t.Errorf("concern Kind = %q, want %q", got.Concerns[0].Kind, semanticConcernCoverageGap)
	}
	if got.Concerns[0].RepairLocus != semanticRepairEvidenceGap {
		t.Errorf("concern RepairLocus = %q, want %q", got.Concerns[0].RepairLocus, semanticRepairEvidenceGap)
	}
}

func TestSemanticQualityReviewer_RepairLocusSchemaAndBackCompat(t *testing.T) {
	corpus := string(semanticQualityTool.Parameters)
	for _, want := range []string{
		`"repair_locus"`,
		`"local_doc_defect"`,
		`"evidence_gap"`,
		`"analysis_gap"`,
		`"presentation_advisory"`,
		`"safety"`,
		`"required": ["topic", "kind", "observation", "suggestion", "repair_locus"]`,
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("semantic quality tool schema missing %q:\n%s", want, corpus)
		}
	}
	got, err := unmarshalSemanticQualityResult(json.RawMessage(`{
		"sufficient": false,
		"confidence": 0.91,
		"concerns": [
			{"topic": "requested topic", "kind": "topic_mismatch", "observation": "wrong subject", "suggestion": "answer the requested subject"}
		]
	}`))
	if err != nil {
		t.Fatalf("unmarshal old-shape concern: %v", err)
	}
	if len(got.Concerns) != 1 || got.Concerns[0].RepairLocus != semanticRepairLocalDocDefect {
		t.Fatalf("old-shape concern should default to local_doc_defect, got %+v", got.Concerns)
	}
}

func TestSemanticQualityReviewer_FallbackConcernForInsufficientWithoutConcerns(t *testing.T) {
	// sufficient=false is a load-bearing model verdict. If the model omits
	// concerns, keep the retry signal alive with a generic concern derived
	// from the model-emitted reasoning instead of dropping the verdict as a
	// non-fatal dispatch failure.
	adapter := &fakeSemanticQualityAdapter{
		wantParams: `{"sufficient": false, "confidence": 0.93, "reasoning": "the answer follows the wrong comparison axis"}`,
	}
	r := NewSemanticQualityReviewer(adapter)
	got, err := r.Review(context.Background(), SemanticQualityInput{AnswerSummary: "x", AnswerBody: "y"})
	if err != nil {
		t.Fatalf("review err: %v", err)
	}
	if got == nil || got.Sufficient || len(got.Concerns) != 1 {
		t.Fatalf("expected one fallback concern, got %+v", got)
	}
	if got.Concerns[0].Topic != "semantic quality review" ||
		!strings.Contains(got.Concerns[0].Observation, "wrong comparison axis") {
		t.Fatalf("fallback concern should preserve reviewer rationale, got %+v", got.Concerns[0])
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

func TestSemanticQualityReviewer_TopicMismatchUsesLowerTypedFloor(t *testing.T) {
	verdict := &SemanticQualityResult{
		Sufficient: false,
		Confidence: 0.90,
		Concerns: []SemanticQualityConcern{{
			Kind:        semanticConcernTopicMismatch,
			Topic:       "requested topic",
			Observation: "body answers a different symbol",
			Suggestion:  "answer the requested issue instead",
		}},
	}
	if got := semanticQualityEffectiveConfidenceFloor(0.92, verdict); got != SemanticQualityTopicMismatchMinConfidence {
		t.Fatalf("topic mismatch floor = %.2f, want %.2f", got, SemanticQualityTopicMismatchMinConfidence)
	}
	ordinary := &SemanticQualityResult{
		Sufficient: false,
		Confidence: 0.90,
		Concerns: []SemanticQualityConcern{{
			Kind:        semanticConcernCoverageGap,
			Topic:       "mechanism",
			Observation: "body is thin",
			Suggestion:  "add evidence",
		}},
	}
	if got := semanticQualityEffectiveConfidenceFloor(0.92, ordinary); got != 0.92 {
		t.Fatalf("ordinary floor = %.2f, want 0.92", got)
	}
	mixed := []SemanticQualityConcern{
		{Kind: semanticConcernCoverageGap, Topic: "coverage"},
		{Kind: semanticConcernTopicMismatch, Topic: "requested topic"},
	}
	filtered := filterSemanticQualityConcernsByKind(mixed, semanticConcernTopicMismatch)
	if len(filtered) != 1 || filtered[0].Topic != "requested topic" {
		t.Fatalf("topic-mismatch filter = %+v", filtered)
	}
	if got := semanticQualityViolationKind(verdict.Concerns[0]); got != types.ViolAnswerTopicMismatch {
		t.Fatalf("topic mismatch violation kind = %q, want %q", got, types.ViolAnswerTopicMismatch)
	}
	if got := semanticQualityViolationKind(ordinary.Concerns[0]); got != types.ViolAnswerSemanticUnderfilled {
		t.Fatalf("ordinary semantic violation kind = %q, want %q", got, types.ViolAnswerSemanticUnderfilled)
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
			{
				ID:   "anchors",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "a", Label: "A"},
					{ID: "b", Label: "B"},
				},
			},
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

func TestBuildSemanticQualityInput_DiagramContractCountsVisibleSequenceCalls(t *testing.T) {
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			EdgeRelations: []types.DiagramEdgeRelationContract{
				{Kind: types.DiagramRelCall, Min: 1, ClaimForm: types.ClaimCallEdge},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "anchors",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "auth", Label: "Auth"},
					{ID: "worker", Label: "Worker"},
				},
			},
			{
				ID:   "d1",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramSequence,
					Language: "mermaid",
					Body:     "sequenceDiagram\n  Auth->>Worker: BuildAnalysisSkill\n  Worker-->>Auth: result\n",
				},
			},
		},
	}
	in := BuildSemanticQualityInput("o", "s", "b", doc, view, nil)
	if in.DiagramContract == nil || len(in.DiagramContract.Edges) != 1 {
		t.Fatalf("missing diagram contract projection: %+v", in.DiagramContract)
	}
	if got := in.DiagramContract.Edges[0].MinSatisfied; got != 1 {
		t.Fatalf("solid sequence call should satisfy visible call minimum, got %d", got)
	}
}

func TestBuildSemanticQualityInput_DiagramContractCountsLabelRelations(t *testing.T) {
	cases := []struct {
		name  string
		rel   types.DiagramRelationKind
		label string
	}{
		{name: "call", rel: types.DiagramRelCall, label: "invoke"},
		{name: "guard", rel: types.DiagramRelGuard, label: "if ready"},
		{name: "import", rel: types.DiagramRelImport, label: "depends on module"},
		{name: "precedence", rel: types.DiagramRelPrecedence, label: "override default"},
		{name: "contain", rel: types.DiagramRelContain, label: "contain module"},
		{name: "observe", rel: types.DiagramRelObserve, label: "metric observation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := &types.AnswerSemanticView{
				DiagramPlan: &types.DiagramFacetGraph{
					Required: true,
					Kind:     types.DiagramSequence,
					EdgeRelations: []types.DiagramEdgeRelationContract{{
						Kind:      tc.rel,
						Min:       1,
						ClaimForm: types.ClaimFormForRelation(tc.rel),
					}},
				},
			}
			doc := &types.AnswerDocumentV2{
				Blocks: []types.AnswerBlock{
					{
						ID:   "anchors",
						Kind: types.BlockOrderedList,
						Items: []types.AnswerBlockItem{
							{ID: "auth", Label: "Auth"},
							{ID: "worker", Label: "Worker"},
						},
					},
					{
						ID:   "d1",
						Kind: types.BlockDiagram,
						Diagram: &types.AnswerDiagramBlock{
							Kind:     types.DiagramSequence,
							Language: "mermaid",
							Body:     "sequenceDiagram\n  Auth->>Worker: " + tc.label + "\n",
						},
					},
				},
			}
			in := BuildSemanticQualityInput("o", "s", "b", doc, view, nil)
			if in.DiagramContract == nil || len(in.DiagramContract.Edges) != 1 {
				t.Fatalf("missing diagram contract projection: %+v", in.DiagramContract)
			}
			if got := in.DiagramContract.Edges[0].MinSatisfied; got != 1 {
				t.Fatalf("label-only relation %s should satisfy min via unified relation resolver, got %d", tc.rel, got)
			}
		})
	}
}

func TestBuildSemanticQualityInput_DiagramContractDoesNotCountDashedReplyAsCall(t *testing.T) {
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			EdgeRelations: []types.DiagramEdgeRelationContract{
				{Kind: types.DiagramRelCall, Min: 1, ClaimForm: types.ClaimCallEdge},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "anchors",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "auth", Label: "Auth"},
					{ID: "worker", Label: "Worker"},
				},
			},
			{
				ID:   "d1",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramSequence,
					Language: "mermaid",
					Body:     "sequenceDiagram\n  Worker-->>Auth: result\n",
				},
			},
		},
	}
	in := BuildSemanticQualityInput("o", "s", "b", doc, view, nil)
	if in.DiagramContract == nil || len(in.DiagramContract.Edges) != 1 {
		t.Fatalf("missing diagram contract projection: %+v", in.DiagramContract)
	}
	if got := in.DiagramContract.Edges[0].MinSatisfied; got != 0 {
		t.Fatalf("dashed sequence reply must not satisfy call minimum, got %d", got)
	}
}

func TestBuildSemanticQualityInput_DiagramContractDoesNotCountPlainSolidLineAsCall(t *testing.T) {
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			EdgeRelations: []types.DiagramEdgeRelationContract{
				{Kind: types.DiagramRelCall, Min: 1, ClaimForm: types.ClaimCallEdge},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "anchors",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "auth", Label: "Auth"},
					{ID: "worker", Label: "Worker"},
				},
			},
			{
				ID:   "d1",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramSequence,
					Language: "mermaid",
					Body:     "sequenceDiagram\n  Auth->Worker: notify\n",
				},
			},
		},
	}
	in := BuildSemanticQualityInput("o", "s", "b", doc, view, nil)
	if in.DiagramContract == nil || len(in.DiagramContract.Edges) != 1 {
		t.Fatalf("missing diagram contract projection: %+v", in.DiagramContract)
	}
	if got := in.DiagramContract.Edges[0].MinSatisfied; got != 0 {
		t.Fatalf("plain solid line without arrowhead must not satisfy call minimum, got %d", got)
	}
}

func TestBuildSemanticQualityInput_DiagramContractCountsCallDAGDirectedFlowchartEdges(t *testing.T) {
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			Kind:     types.DiagramCallDAG,
			EdgeRelations: []types.DiagramEdgeRelationContract{
				{Kind: types.DiagramRelCall, Min: 1, ClaimForm: types.ClaimCallEdge},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "d1",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramCallDAG,
					Language: "mermaid",
					Body:     "flowchart TD\n  Auth[Auth] --> Worker[Worker]\n",
				},
			},
		},
	}
	in := BuildSemanticQualityInput("o", "s", "b", doc, view, nil)
	if in.DiagramContract == nil || len(in.DiagramContract.Edges) != 1 {
		t.Fatalf("missing diagram contract projection: %+v", in.DiagramContract)
	}
	if got := in.DiagramContract.Edges[0].MinSatisfied; got != 1 {
		t.Fatalf("directed CallDAG flowchart edge should satisfy call minimum, got %d", got)
	}
}

func TestBuildSemanticQualityInput_DiagramContractCountsFlowDecisionEdgesAsGuard(t *testing.T) {
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			Kind:     types.DiagramFlow,
			EdgeRelations: []types.DiagramEdgeRelationContract{
				{Kind: types.DiagramRelGuard, Min: 1, ClaimForm: types.ClaimGuardCondition},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "d1",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramFlow,
					Language: "mermaid",
					Body:     "flowchart TD\n  Check{ready?} --> Handle[Handle]\n",
				},
			},
		},
	}
	in := BuildSemanticQualityInput("o", "s", "b", doc, view, nil)
	if in.DiagramContract == nil || len(in.DiagramContract.Edges) != 1 {
		t.Fatalf("missing diagram contract projection: %+v", in.DiagramContract)
	}
	if got := in.DiagramContract.Edges[0].MinSatisfied; got != 1 {
		t.Fatalf("decision flow edge should satisfy guard minimum, got %d", got)
	}
}

func TestBuildSemanticQualityInput_DiagramContractDoesNotCountPlainFlowEdgeAsGuard(t *testing.T) {
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			Kind:     types.DiagramFlow,
			EdgeRelations: []types.DiagramEdgeRelationContract{
				{Kind: types.DiagramRelGuard, Min: 1, ClaimForm: types.ClaimGuardCondition},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "d1",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramFlow,
					Language: "mermaid",
					Body:     "flowchart TD\n  Start[Start] --> Handle[Handle]\n",
				},
			},
		},
	}
	in := BuildSemanticQualityInput("o", "s", "b", doc, view, nil)
	if in.DiagramContract == nil || len(in.DiagramContract.Edges) != 1 {
		t.Fatalf("missing diagram contract projection: %+v", in.DiagramContract)
	}
	if got := in.DiagramContract.Edges[0].MinSatisfied; got != 0 {
		t.Fatalf("plain flow edge must not satisfy guard minimum, got %d", got)
	}
}

func TestSemanticQualityReviewBodyIncludesDiagramBlocks(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
			{ID: "d1", Kind: types.BlockDiagram, Title: "Flow",
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramArchitecture,
					Language: "mermaid",
					Body:     "flowchart TD\n  BaseAgent --> SubExplorer",
				}},
		},
	}
	selfConsistencyBody := renderConsistencyReviewBodyV2(doc)
	if strings.Contains(selfConsistencyBody, "flowchart TD") {
		t.Fatalf("self-consistency body should continue to skip diagrams:\n%s", selfConsistencyBody)
	}
	semanticBody := renderSemanticQualityReviewBodyV2(doc)
	if !strings.Contains(semanticBody, "```mermaid") || !strings.Contains(semanticBody, "BaseAgent --> SubExplorer") {
		t.Fatalf("semantic quality body must include diagram source:\n%s", semanticBody)
	}
}

func TestSemanticQualityReviewBodyIncludesTableBlockText(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
			{ID: "t1", Kind: types.BlockTable, Text: "| 覆盖层 | 搜索结果 |\n|---|---|\n| CLI flag | 不存在 |"},
		},
	}
	got := renderSemanticQualityReviewBodyV2(doc)
	if !strings.Contains(got, "| 覆盖层 | 搜索结果 |") || !strings.Contains(got, "| CLI flag | 不存在 |") {
		t.Fatalf("semantic quality body must include table text:\n%s", got)
	}
}

func TestBuildSemanticQualityInput_DiagramFacetDepthAnchoredByDiagramSurface(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "d1", Kind: types.BlockDiagram, FacetIDs: []string{"diagram_spine"},
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramArchitecture,
					Language: "mermaid",
					Body:     "flowchart TD\n  A --> B",
				}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "diagram_spine", Required: types.FacetHardRequired,
					PromotionPolicy: types.PromotionAlwaysHard,
					SourceCandidate: []string{"ev-a"}},
			},
		},
	}
	in := BuildSemanticQualityInput("o", "s", renderSemanticQualityReviewBodyV2(doc), doc, view, nil)
	if len(in.PromotedFacetCoverage) != 1 {
		t.Fatalf("PromotedFacetCoverage len = %d, want 1: %+v", len(in.PromotedFacetCoverage), in.PromotedFacetCoverage)
	}
	got := in.PromotedFacetCoverage[0]
	if got.DeclaredCount != 1 || got.AnchoredCount != 1 {
		t.Fatalf("diagram facet should be declared and anchored by typed diagram body, got %+v", got)
	}
}

func TestBuildSemanticQualityInput_NilGuards(t *testing.T) {
	in := BuildSemanticQualityInput("o", "s", "b", nil, nil, nil)
	if len(in.RequiredFacets) != 0 || in.DiagramContract != nil || len(in.RichnessCandidates) != 0 {
		t.Errorf("nil doc/view must yield no projections; got %+v", in)
	}
}

// G5-2: semanticQualityReviewerEligible gate.
//
// P4 (2026-05-10) prefilter additions: tests now seed FacetIDs so
// the new "no-facet-declared → skip" branch doesn't make every
// fixture vacuously skip.
func TestSemanticQualityReviewerEligible_GateSemantics(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	body := []types.AnswerBlock{
		{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
		{ID: "b1", Kind: types.BlockOrderedList, FacetIDs: []string{"enumeration_item"}, Items: []types.AnswerBlockItem{{ID: "i1", Label: "x"}}},
	}
	docFull := &types.AnswerDocumentV2{Blocks: body}

	// Single-block summary-only — skip.
	docSummaryOnly := &types.AnswerDocumentV2{Blocks: body[:1]}
	if semanticQualityReviewerEligible(docSummaryOnly, nil) {
		t.Error("single-block doc should skip reviewer")
	}

	// Body present + no hard violations + facet declared → review.
	if !semanticQualityReviewerEligible(docFull, nil) {
		t.Error("body present + facet declared + no hard violations → reviewer should fire")
	}

	// Default-soft finalizer roots should not block the reviewer.
	defaultSoftViolations := []types.Violation{
		{Kind: types.ViolFacetUncovered, Detail: "x"},
		{Kind: types.ViolDiagramEdgeUnsupported, Detail: "x"},
	}
	if !semanticQualityReviewerEligible(docFull, defaultSoftViolations) {
		t.Error("default-soft finalizer roots → reviewer should still fire")
	}

	// Strict-promoted retry root present → skip.
	SetSoftViolationKinds(nil, []string{string(types.ViolFacetUncovered)})
	strictViolations := []types.Violation{
		{Kind: types.ViolFacetUncovered, Detail: "x"},
	}
	if semanticQualityReviewerEligible(docFull, strictViolations) {
		t.Error("strict-promoted retry root present → reviewer should skip")
	}
	SetSoftViolationKinds(nil, nil)

	// SOFT violation present → review still fires.
	softViolations := []types.Violation{
		{Kind: types.ViolRichnessRegression, Detail: "x"},
	}
	if !semanticQualityReviewerEligible(docFull, softViolations) {
		t.Error("only SOFT violations → reviewer should fire")
	}
}

// P4 (2026-05-10) — no-facet-declared prefilter. A doc with body
// blocks but ZERO declared facets (no block.facet_ids[] and no
// claim_uses[].facet_id) cannot have the AnchoredCount/DeclaredCount
// gap the reviewer is looking for; skip the dispatch.
func TestSemanticQualityReviewerEligible_P4_NoFacetDeclared_Skip(t *testing.T) {
	docNoFacets := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
			{ID: "b1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "i1", Label: "x"}}},
		},
	}
	if semanticQualityReviewerEligible(docNoFacets, nil) {
		t.Error("doc with zero declared facets should skip reviewer (P4 prefilter)")
	}

	// Same shape but with one block.facet_ids declared → review.
	docWithFacet := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
			{ID: "b1", Kind: types.BlockOrderedList, FacetIDs: []string{"enumeration_item"}, Items: []types.AnswerBlockItem{{ID: "i1", Label: "x"}}},
		},
	}
	if !semanticQualityReviewerEligible(docWithFacet, nil) {
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
	if !semanticQualityReviewerEligible(docWithClaimUseFacet, nil) {
		t.Error("doc with claim_uses[].facet_id should fire reviewer")
	}
}

func TestSkipSemanticQualityForObservationOnlyArtifact_SkipsOnlyPureSimpleArtifactAnswers(t *testing.T) {
	bundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type:    "panic",
			Message: "called Option::unwrap() on a None value",
		}},
	}
	mut := types.NewMutableState("这是什么错误？")
	mut.SetRequestModel(types.RequestModel{
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexitySimple,
		LogTriage:  bundle,
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
		},
	})
	ctx := &types.BusContext{Mutable: mut}
	pureArtifactDoc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:       "s1",
				Kind:     types.BlockSummary,
				Text:     "这是 Rust panic：运行时日志观察到 Option::unwrap() 在 None 值上触发崩溃，栈帧属于外部 artifact，不能当作当前仓库源码位置。这里的 src/config.rs:42 和 src/main.rs:12 是日志里的运行时帧位置，不是当前 checkout 已验证的文件行号。",
				FacetIDs: []string{string(types.FacetObservedArtifactFact)},
				ClaimUses: []types.RenderedClaimUse{{
					ClaimForm: types.ClaimExternalObservation,
					FacetID:   string(types.FacetObservedArtifactFact),
				}},
			},
			{
				ID:       "frames",
				Kind:     types.BlockOrderedList,
				FacetIDs: []string{string(types.FacetObservedArtifactFact)},
				ClaimUses: []types.RenderedClaimUse{{
					ClaimForm: types.ClaimExternalObservation,
					FacetID:   string(types.FacetObservedArtifactFact),
				}},
				Items: []types.AnswerBlockItem{
					{ID: "f0", Label: "panic", CitationRef: -1},
					{ID: "f1", Label: "rust_begin_unwind", CitationRef: -1},
					{ID: "f2", Label: "my_app::config::load", CitationRef: -1},
					{ID: "f3", Label: "my_app::main", CitationRef: -1},
				},
			},
			{
				ID:       "boundary",
				Kind:     types.BlockCaveat,
				FacetIDs: []string{string(types.FacetUncertaintyBoundary)},
				Text:     "resolved_files=0",
			},
		},
	}
	if !semanticQualityReviewerEligible(pureArtifactDoc, nil) {
		t.Fatal("fixture should satisfy the normal semantic-quality eligibility gate before artifact skip")
	}
	if !shouldReviewConsistencyV2(pureArtifactDoc) {
		t.Fatal("fixture should satisfy the normal self-consistency eligibility gate before artifact skip")
	}
	if !skipSemanticQualityForObservationOnlyArtifact(ctx, pureArtifactDoc, nil) {
		t.Fatal("simple pure observation-only artifact answer should skip semantic-quality reviewer")
	}
	if !skipLLMAnswerReviewForObservationOnlyArtifact(ctx, pureArtifactDoc, nil) {
		t.Fatal("shared LLM-review gate should skip the same pure observation-only artifact answer")
	}

	withCitation := *pureArtifactDoc
	withCitation.Citations = []types.Citation{{File: "src/config.rs", Line: 42}}
	if skipSemanticQualityForObservationOnlyArtifact(ctx, &withCitation, nil) {
		t.Fatal("doc with repo-style citations must not skip semantic-quality reviewer")
	}

	withCurrentCodeFacet := *pureArtifactDoc
	withCurrentCodeFacet.Blocks = append([]types.AnswerBlock(nil), pureArtifactDoc.Blocks...)
	withCurrentCodeFacet.Blocks[1].FacetIDs = []string{string(types.FacetCurrentCodePath)}
	if skipSemanticQualityForObservationOnlyArtifact(ctx, &withCurrentCodeFacet, nil) {
		t.Fatal("doc that declares current-code facets must not skip semantic-quality reviewer")
	}

	complexMut := types.NewMutableState("分析完整调用链")
	complexMut.SetRequestModel(types.RequestModel{
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexityComplex,
		LogTriage:  bundle,
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
		},
	})
	if !skipLLMAnswerReviewForObservationOnlyArtifact(&types.BusContext{Mutable: complexMut}, pureArtifactDoc, nil) {
		t.Fatal("complexity alone is not a safe reason to run LLM reviewers on pure observation-only artifacts")
	}

	diagramMut := types.NewMutableState("画出这个外部日志的时序图")
	diagramMut.SetRequestModel(types.RequestModel{
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexityComplex,
		LogTriage:  bundle,
		DiagramHint: &types.DiagramHint{
			Kind: types.DiagramSequence,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
		},
	})
	if skipLLMAnswerReviewForObservationOnlyArtifact(&types.BusContext{Mutable: diagramMut}, pureArtifactDoc, nil) {
		t.Fatal("diagram-requested artifact answers should keep LLM review available")
	}
}

// TestSemanticQualityReviewerEligible_DecisionStableAcrossSelfConsistencyOutput
// pins the P2C parallel-dispatch invariant: the gate decision MUST be
// identical whether evaluated BEFORE self_consistency runs (using only
// the block-oracle violations on result.Violations) or AFTER (with
// self_consistency's ViolSelfContradiction kinds appended). This is the
// load-bearing property that lets the two reviewers run concurrently —
// the gate is determined by the time the deterministic-reviewer block
// exits, and self_consistency cannot change the outcome.
//
// If a future commit makes self_consistency emit one of the 7 hard
// kinds in the gate list, this test fails and the parallelism must be
// re-evaluated (or this test updated to reflect the new gate semantics).
func TestSemanticQualityReviewerEligible_DecisionStableAcrossSelfConsistencyOutput(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	body := []types.AnswerBlock{
		{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
		{ID: "b1", Kind: types.BlockOrderedList, FacetIDs: []string{"enumeration_item"}, Items: []types.AnswerBlockItem{{ID: "i1", Label: "x"}}},
	}
	doc := &types.AnswerDocumentV2{Blocks: body}

	// Pre-flight: block oracles produced zero hard kinds → gate runs.
	preFlight := []types.Violation{
		{Kind: types.ViolRichnessRegression},
	}
	wantEligible := semanticQualityReviewerEligible(doc, preFlight)

	// Post-self-consistency: SC adds N contradictions. The kind it
	// emits is ViolSelfContradiction, NOT in the hard-kind list. Gate
	// MUST stay identical.
	postSC := append([]types.Violation(nil), preFlight...)
	for i := 0; i < 3; i++ {
		postSC = append(postSC, types.Violation{Kind: types.ViolSelfContradiction})
	}
	gotEligible := semanticQualityReviewerEligible(doc, postSC)

	if wantEligible != gotEligible {
		t.Fatalf("gate decision changed by self_consistency output (pre=%v post=%v); P2C parallel dispatch is unsafe", wantEligible, gotEligible)
	}

	// Sanity: if an operator strict-promotes a pre-existing kind,
	// BOTH pre-flight and post-SC evaluations must skip — also stable.
	SetSoftViolationKinds(nil, []string{string(types.ViolFacetUncovered)})
	preFlightHard := []types.Violation{{Kind: types.ViolFacetUncovered}}
	postSCHard := append(append([]types.Violation(nil), preFlightHard...), types.Violation{Kind: types.ViolSelfContradiction})
	if semanticQualityReviewerEligible(doc, preFlightHard) || semanticQualityReviewerEligible(doc, postSCHard) {
		t.Fatal("strict-promoted violation in pre-flight must block reviewer regardless of self_consistency output")
	}
}

// P4 — SemanticQualityMinConfidenceDefault constant pinned to 0.92.
func TestSemanticQualityMinConfidenceDefault_P4(t *testing.T) {
	if SemanticQualityMinConfidenceDefault != 0.92 {
		t.Errorf("P4 default floor should be 0.92, got %v", SemanticQualityMinConfidenceDefault)
	}
}

func TestSemanticQualityTopicMismatchMinConfidence(t *testing.T) {
	if SemanticQualityTopicMismatchMinConfidence != 0.85 {
		t.Errorf("topic mismatch floor should be 0.85, got %v", SemanticQualityTopicMismatchMinConfidence)
	}
}

func TestRunSemanticQualityReview_TopicMismatchEvidenceGapRoutesUpstream(t *testing.T) {
	mut := types.NewMutableState("compare codrax and opencode hallucination prevention")
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Ctx:      context.Background(),
		Mutable:  mut,
		Language: "zh",
	}
	o.SetSemanticQualityReviewer(fakeSemanticQualityReviewer{
		verdict: &SemanticQualityResult{
			Sufficient: false,
			Confidence: 0.91,
			Concerns: []SemanticQualityConcern{{
				Kind:        semanticConcernTopicMismatch,
				Topic:       "requested topic",
				Observation: "body answers a different subject",
				Suggestion:  "collect the missing comparison evidence before revising",
				RepairLocus: semanticRepairEvidenceGap,
			}},
		},
	})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary", FacetIDs: []string{"principal_mechanism"}},
			{ID: "b1", Kind: types.BlockSection, Text: "body", FacetIDs: []string{"principal_mechanism"}},
		},
	}
	got := o.runSemanticQualityReview(doc, mut)
	if len(got) != 1 {
		t.Fatalf("violations len=%d, want 1: %+v", len(got), got)
	}
	if got[0].Kind != types.ViolAnswerTopicMismatch {
		t.Fatalf("kind=%q, want %q", got[0].Kind, types.ViolAnswerTopicMismatch)
	}
	if got[0].RepairLocusOverride != types.LocusExplore {
		t.Fatalf("repair locus override=%q, want explore", got[0].RepairLocusOverride)
	}
	if target := FallbackTargetForViolation(got[0]); target != FallbackBackToExplore {
		t.Fatalf("fallback target=%s, want %s", target, FallbackBackToExplore)
	}
}

func TestRunSemanticQualityReview_PresentationAdvisoryDoesNotHardTopicRetry(t *testing.T) {
	mut := types.NewMutableState("explain module")
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{Ctx: context.Background(), Mutable: mut}
	o.SetSemanticQualityReviewer(fakeSemanticQualityReviewer{
		verdict: &SemanticQualityResult{
			Sufficient: false,
			Confidence: 0.95,
			Concerns: []SemanticQualityConcern{{
				Kind:        semanticConcernTopicMismatch,
				Topic:       "section title",
				Observation: "title is awkward but body is relevant",
				Suggestion:  "surface as a note if needed",
				RepairLocus: semanticRepairPresentationAdvisor,
			}},
		},
	})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary", FacetIDs: []string{"principal_mechanism"}},
			{ID: "b1", Kind: types.BlockSection, Text: "body", FacetIDs: []string{"principal_mechanism"}},
		},
	}
	got := o.runSemanticQualityReview(doc, mut)
	if len(got) != 1 {
		t.Fatalf("violations len=%d, want 1: %+v", len(got), got)
	}
	if got[0].Kind != types.ViolAnswerSemanticUnderfilled {
		t.Fatalf("presentation advisory should be soft semantic underfill, got %q", got[0].Kind)
	}
	if got[0].RepairLocusOverride != types.LocusTerminal {
		t.Fatalf("presentation advisory override=%q, want terminal", got[0].RepairLocusOverride)
	}
}

func TestRunSemanticQualityReview_DiagramGapWithoutHardContractDropped(t *testing.T) {
	mut := types.NewMutableState("show sequence")
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{Ctx: context.Background(), Mutable: mut}
	o.SetSemanticQualityReviewer(fakeSemanticQualityReviewer{
		verdict: &SemanticQualityResult{
			Sufficient: false,
			Confidence: 0.95,
			Concerns: []SemanticQualityConcern{{
				Kind:        semanticConcernDiagramGap,
				Topic:       "diagram",
				Observation: "reviewer thinks a diagram edge is missing",
				Suggestion:  "add an edge",
			}},
		},
	})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary"},
			{ID: "d1", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{
				Kind:     types.DiagramSequence,
				Language: "mermaid",
				Body:     "sequenceDiagram\n  Auth->>Worker: BuildAnalysisSkill\n",
			}},
		},
	}
	if got := o.runSemanticQualityReview(doc, mut); len(got) != 0 {
		t.Fatalf("diagram_gap without deterministic hard contract should be dropped, got %+v", got)
	}
}

func TestRunSemanticQualityReview_InputIncludesCitationLineIdentifier(t *testing.T) {
	mut := types.NewMutableState("explain opencode read limits")
	var seen SemanticQualityInput
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{
		Ctx:     context.Background(),
		Mutable: mut,
		ToolResults: []types.ToolResult{{
			ToolName: "read_file",
			Success:  true,
			Summary: "[packages/opencode/src/tool/read.ts: showing lines 14-14 of 338 total]\n" +
				"    14│ const DEFAULT_READ_LIMIT = 2000\n",
		}},
	}
	o.SetSemanticQualityReviewer(fakeSemanticQualityReviewer{
		seen: &seen,
		verdict: &SemanticQualityResult{
			Sufficient: true,
			Confidence: 0.99,
		},
	})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "opencode uses DEFAULT_READ_LIMIT for read limits.", FacetIDs: []string{"principal_mechanism"}},
			{ID: "b1", Kind: types.BlockSection, Text: "DEFAULT_READ_LIMIT is cited from read.ts.", FacetIDs: []string{"principal_mechanism"}},
		},
		Citations: []types.Citation{{File: "packages/opencode/src/tool/read.ts", Line: 14}},
	}

	_ = o.runSemanticQualityReview(doc, mut)
	if !containsString(seen.EvidenceAnchorSet, "DEFAULT_READ_LIMIT") {
		t.Fatalf("semantic-quality reviewer anchor set should include final document citation identifiers; got %v", seen.EvidenceAnchorSet)
	}
}

func TestSemanticQualityStrictCoverageViolation_ShallowPromotedFacet(t *testing.T) {
	in := SemanticQualityInput{
		PromotedFacetCoverage: []FacetCoverageDepth{
			{Kind: string(types.FacetCurrentCodePath), DeclaredCount: 4, AnchoredCount: 0},
		},
	}
	concern := SemanticQualityConcern{
		Kind:  semanticConcernCoverageGap,
		Topic: "Current code path",
	}
	got, ok := semanticQualityStrictCoverageViolation(concern, in)
	if !ok {
		t.Fatal("shallow promoted facet coverage gap should become a strict runtime violation")
	}
	if got != types.ViolPrincipalProseUnderfilled {
		t.Fatalf("kind=%q, want %q", got, types.ViolPrincipalProseUnderfilled)
	}
}

func TestSemanticQualityStrictCoverageViolation_RichnessGapRemainsSoft(t *testing.T) {
	in := SemanticQualityInput{
		PromotedFacetCoverage: []FacetCoverageDepth{
			{Kind: string(types.FacetCurrentCodePath), DeclaredCount: 4, AnchoredCount: 0},
		},
	}
	concern := SemanticQualityConcern{
		Kind:  semanticConcernRichnessGap,
		Topic: "Current code path",
	}
	if got, ok := semanticQualityStrictCoverageViolation(concern, in); ok {
		t.Fatalf("richness_gap must remain soft, got strict kind %q", got)
	}
}

func TestSemanticQualityStrictCoverageViolation_RequiredDiagramRelationGapStaysSoft(t *testing.T) {
	in := SemanticQualityInput{
		DiagramContract: &SemanticDiagramSummary{
			Required:       true,
			BlockPresent:   true,
			BodyTrimmedLen: 42,
			Edges: []SemanticDiagramEdgeContract{
				{RelationKind: "call", MinExpected: 2, MinSatisfied: 1},
			},
		},
	}
	concern := SemanticQualityConcern{
		Kind:  semanticConcernDiagramGap,
		Topic: "diagram",
	}
	if got, ok := semanticQualityStrictCoverageViolation(concern, in); ok {
		t.Fatalf("relation-min diagram shortfall should stay soft when a diagram body exists; got %q", got)
	}
}

func TestSemanticQualityStrictCoverageViolation_MissingRequiredDiagramStaysStrict(t *testing.T) {
	in := SemanticQualityInput{
		DiagramContract: &SemanticDiagramSummary{
			Required:     true,
			BlockPresent: false,
		},
	}
	concern := SemanticQualityConcern{
		Kind:  semanticConcernDiagramGap,
		Topic: "diagram",
	}
	got, ok := semanticQualityStrictCoverageViolation(concern, in)
	if !ok {
		t.Fatal("missing required diagram should remain a strict runtime violation")
	}
	if got != types.ViolDiagramEdgeUnsupported {
		t.Fatalf("kind=%q, want %q", got, types.ViolDiagramEdgeUnsupported)
	}
}

func TestSemanticQualityStrictCoverageViolation_SatisfiedDiagramGapStaysNonStrict(t *testing.T) {
	in := SemanticQualityInput{
		DiagramContract: &SemanticDiagramSummary{
			Required:       true,
			BlockPresent:   true,
			BodyTrimmedLen: 42,
			Edges: []SemanticDiagramEdgeContract{
				{RelationKind: "call", MinExpected: 1, MinSatisfied: 1},
			},
		},
	}
	concern := SemanticQualityConcern{
		Kind:  semanticConcernDiagramGap,
		Topic: "diagram",
	}
	if got, ok := semanticQualityStrictCoverageViolation(concern, in); ok {
		t.Fatalf("satisfied deterministic diagram contract must not become strict, got %q", got)
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
