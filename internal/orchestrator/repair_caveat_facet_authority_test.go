package orchestrator

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This is the shipping path for an accepted answer, not a prose completeness
// oracle. A visible relation and a call-edge annotation do not implicitly set
// the block's facet ownership; that missing metadata cannot prove missing prose.
func TestB1632FacetCoverageAppendDoesNotClaimMissingModelContent(t *testing.T) {
	SetSoftViolationKinds(nil, nil)
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	for _, lang := range []string{"zh", "en"} {
		for _, withDiagram := range []bool{false, true} {
			name := lang + "/prose"
			if withDiagram {
				name += "_and_model_diagram"
			}
			t.Run(name, func(t *testing.T) {
				doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
					{ID: "summary", Kind: types.BlockSummary, Text: "The caller invokes the callee before returning."},
					{ID: "steps", Kind: types.BlockOrderedList, SurfaceRole: "principal",
						FacetIDs:  []string{string(types.FacetCurrentCodePath)},
						ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, EvidenceID: "ev-call"}},
						Items:     []types.AnswerBlockItem{{ID: "invoke", Label: "caller", Text: "`caller` -> `callee`: invoke the handler, then return its result."}}},
				}}
				if withDiagram {
					doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "model-graph", Kind: types.BlockDiagram,
						Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Body: "flowchart LR\n  caller --> callee"}})
				}
				view := &types.AnswerSemanticView{FacetCoverage: &types.FacetCoverageContract{
					Required: []types.FacetRequirement{{Kind: types.FacetPrincipalPathEdge,
						Tier: types.TierExpected, SourceCandidate: []string{"ev-call"}}},
				}}
				rm := types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
					AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)}}
				mut := types.NewMutableState("explain the mechanism")
				mut.SetRequestModel(rm)
				mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
				ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
				before, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				violations := validateFacetCoverage(doc, view)
				if len(violations) != 1 || violations[0].Kind != types.ViolFacetUncovered ||
					violations[0].ClusterKey != types.FacetClusterKey(string(types.FacetPrincipalPathEdge), "answer_facet_coverage") {
					t.Fatalf("typed ownership remains unconfirmed despite visible relations: %+v", violations)
				}
				answer := render.RenderAnswerDocument(doc, lang)
				out := AppendSoftContractCaveatsToAnswerForBus(answer, violations, lang, ctx)
				if !strings.HasPrefix(out, answer) {
					t.Fatalf("append changed existing model answer bytes:\n%s", out)
				}
				caveat := strings.TrimPrefix(out, answer)
				b1632AssertUnconfirmedCoverage(t, caveat, lang)
				if withDiagram && !strings.Contains(out, doc.Blocks[2].Diagram.Body) {
					t.Fatalf("model-authored diagram was changed or removed:\n%s", out)
				}
				after, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) {
					t.Fatalf("coverage disclosure mutated model document:\nbefore=%s\nafter=%s", before, after)
				}
				stored, err := json.Marshal(mut.AnswerDocumentV2())
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(stored) {
					t.Fatalf("coverage disclosure mutated accepted document: %s", stored)
				}
				if got := validateFacetCoverage(doc, view); !reflect.DeepEqual(got, violations) {
					t.Fatalf("append changed coverage obligations: %+v", got)
				}

				// A model's explicit metadata submission still satisfies the
				// existing predicate. The formatter never makes that submission.
				doc.Blocks[1].FacetIDs = append(doc.Blocks[1].FacetIDs, string(types.FacetPrincipalPathEdge))
				resolved := validateFacetCoverage(doc, view)
				if len(resolved) != 0 {
					t.Fatalf("explicit facet ownership no longer resolves coverage: %+v", resolved)
				}
				if got := AppendSoftContractCaveatsToAnswerForBus(answer, resolved, lang, ctx); got != answer {
					t.Fatalf("resolved coverage changed answer: %q", got)
				}
			})
		}
	}
}

func TestB1632EveryExactFacetHasSameDisclosureAuthority(t *testing.T) {
	facets := []types.AnswerFacetKind{
		types.FacetObservedArtifactFact, types.FacetCurrentCodePath, types.FacetNearestMechanism,
		types.FacetUncertaintyBoundary, types.FacetConfigPrecedenceRole, types.FacetResolvedLiteralOrSymbol,
		types.FacetEnumerationItem, types.FacetBucketLabel, types.FacetPrincipalPathEdge,
		types.FacetBranchGuard, types.FacetComponentRelation, types.FacetDiagramSpine,
	}
	for _, lang := range []string{"zh", "en"} {
		var combined []types.Violation
		var labels []string
		for _, facet := range facets {
			t.Run(lang+"/"+string(facet), func(t *testing.T) {
				v := b1632FacetViolation(facet)
				got := MaterializeUnresolvedViolationsAsCaveats([]types.Violation{v}, lang)
				if len(got) != 1 {
					t.Fatalf("want one exact coverage caveat, got %v", got)
				}
				b1632AssertUnconfirmedCoverage(t, got[0], lang)
				label := answerFacetCoverageCaveatLabel(facet, lang == "zh")
				if label == "" || !strings.Contains(got[0], label) || strings.Contains(got[0], string(facet)) {
					t.Fatalf("lost reader label or exposed facet enum: %q", got[0])
				}
			})
			combined = append(combined, b1632FacetViolation(facet))
			labels = append(labels, answerFacetCoverageCaveatLabel(facet, lang == "zh"))
		}
		first := MaterializeUnresolvedViolationsAsCaveats(combined, lang)
		var reversedDuplicates []types.Violation
		for i := len(combined) - 1; i >= 0; i-- {
			reversedDuplicates = append(reversedDuplicates, combined[i], combined[i])
		}
		second := MaterializeUnresolvedViolationsAsCaveats(reversedDuplicates, lang)
		if len(first) != 1 || !reflect.DeepEqual(first, second) {
			t.Fatalf("family grouping/order/dedup changed: first=%v second=%v", first, second)
		}
		separator := ", "
		if lang == "zh" {
			separator = "、"
		}
		if !strings.Contains(first[0], strings.Join(labels, separator)) {
			t.Fatalf("facet display order changed: %q", first[0])
		}
	}
}

func TestB1632MixedAndUnknownCoverageKeepFamilyFallback(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		fallback := MaterializeUnresolvedViolationsAsCaveats([]types.Violation{{Kind: types.ViolBlockCoverageMissing}}, lang)
		if len(fallback) != 1 {
			t.Fatalf("missing existing family fallback: %v", fallback)
		}
		for name, violations := range map[string][]types.Violation{
			"mixed":         {b1632FacetViolation(types.FacetPrincipalPathEdge), {Kind: types.ViolBlockCoverageMissing}},
			"unknown_facet": {b1632FacetViolation("future_facet")},
			"unclustered":   {{Kind: types.ViolFacetUncovered}},
			"other_root":    {{Kind: types.ViolFacetUncovered, ClusterKey: types.FacetClusterKey(string(types.FacetPrincipalPathEdge), "other_root")}},
		} {
			t.Run(lang+"/"+name, func(t *testing.T) {
				got := MaterializeUnresolvedViolationsAsCaveats(violations, lang)
				if !reflect.DeepEqual(got, fallback) {
					t.Fatalf("non-exact coverage must retain original fallback: got=%v want=%v", got, fallback)
				}
			})
		}
	}
}

func b1632FacetViolation(facet types.AnswerFacetKind) types.Violation {
	return types.Violation{Kind: types.ViolFacetUncovered,
		ClusterKey: types.FacetClusterKey(string(facet), "answer_facet_coverage")}
}

func b1632AssertUnconfirmedCoverage(t *testing.T, text, lang string) {
	t.Helper()
	wants := []string{"has not confirmed which parts of the answer address", "does not mean the answer lacks", "check", "not globally downgraded"}
	forbidden := "The answer did not fully present these requested elements"
	if lang == "zh" {
		wants = []string{"尚未确认答案中哪些部分对应以下内容", "不表示正文缺少这些内容", "核对", "不因此整体降级"}
		forbidden = "答案未完整呈现这些已要求的内容"
	}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("disclosure needs %q, got %q", want, text)
		}
	}
	if strings.Contains(text, forbidden) {
		t.Errorf("metadata absence overclaimed missing answer content: %q", text)
	}
}
