package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// B54's soft severity already protects retry admission. This exercises its
// independent shipping boundary: a real completeness finding must remain in
// telemetry without becoming an assertion about the model's answer.
func TestB54Tier2ActualValidatorsDoNotAnnotateModelAnswer(t *testing.T) {
	SetSoftViolationKinds(nil, nil)
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	for _, lang := range []string{"zh", "en"} {
		for _, kind := range b54Tier2DisplayKinds() {
			t.Run(lang+"/"+string(kind), func(t *testing.T) {
				input, family := b54Tier2DisplayInput(kind)
				if got := types.ResolveQuestionFamily(input.IR.RequestModel); got != family {
					t.Fatalf("fixture family changed: got %s, want %s", got, family)
				}
				fail := agent.RunFamilyValidators(family, input)
				if fail == nil || fail.ViolationKind != kind || fail.FixHint == "" {
					t.Fatalf("actual heuristic must still report the original finding: %+v", fail)
				}
				// Same carrier adaptation as the scheduler's post-finalize hook.
				violations := []types.Violation{{Kind: fail.ViolationKind, Stage: string(types.StageFinalize),
					ClusterKey: "tier2/" + string(fail.Dimension), Detail: fail.Reason, Repair: fail.FixHint}}
				ctx := b54Tier2DisplayBus(input, lang)
				ctx.Mutable.SetTier2CompletenessHint(fail.FixHint)
				beforeInput := b54Tier2DisplayJSON(t, input)
				beforeViolations := b54Tier2DisplayJSON(t, violations)
				beforeDoc := b54Tier2DisplayJSON(t, ctx.Mutable.AnswerDocumentV2())
				answer := render.RenderAnswerDocument(input.AnswerDocumentV2, lang)
				if answer == "" {
					t.Fatal("public render must produce a nonempty answer")
				}
				b54AssertTier2AppendsUnchanged(t, answer, violations, lang, ctx)
				if got := agent.RunFamilyValidators(family, input); !reflect.DeepEqual(got, fail) {
					t.Fatalf("display filtering changed the diagnostic: before=%+v after=%+v", fail, got)
				}
				if b54Tier2DisplayJSON(t, input) != beforeInput ||
					b54Tier2DisplayJSON(t, violations) != beforeViolations ||
					b54Tier2DisplayJSON(t, ctx.Mutable.AnswerDocumentV2()) != beforeDoc {
					t.Fatal("display changed request, evidence, finding, or accepted model document bytes")
				}
				if ctx.Mutable.Tier2CompletenessHint() != fail.FixHint {
					t.Fatal("display filtering lost the internal exploration hint")
				}
				if got := FilterFinalizerRetryRootViolations(violations); len(got) != 0 {
					t.Fatalf("existing permanently-soft retry boundary changed: %+v", got)
				}
			})
		}
	}
}

func TestB54Tier2ComparisonIntentDoesNotProveEvidenceStrength(t *testing.T) {
	for _, explicitComparison := range []bool{false, true} {
		t.Run(fmt.Sprint(explicitComparison), func(t *testing.T) {
			input, family := b54Tier2DisplayInput(types.ViolEntityParityImbalanced)
			if explicitComparison {
				input.IR.RequestModel.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true,
					Dimensions: []types.RequestedAnswerDimension{{Index: 1, Required: true,
						Role: types.RequestedAnswerDimensionComparisonAxis, Label: "Compare implementations"}},
				}
			}
			fail := agent.RunFamilyValidators(family, input)
			if fail == nil || fail.ViolationKind != types.ViolEntityParityImbalanced {
				t.Fatalf("comparison declaration must not change the original heuristic: %+v", fail)
			}
			for _, lang := range []string{"zh", "en"} {
				answer := render.RenderAnswerDocument(input.AnswerDocumentV2, lang)
				b54AssertTier2AppendsUnchanged(t, answer, []types.Violation{{Kind: fail.ViolationKind,
					Detail: fail.Reason, Repair: fail.FixHint}}, lang, b54Tier2DisplayBus(input, lang))
			}
		})
	}
}

func TestB54Tier2DisplayPreservesOtherTypedCaveatsAndBudget(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		// The exact facet producer and other existing caveat families are not
		// part of the four noisy rules, even when they share soft severity.
		doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model answer."}}}
		view := &types.AnswerSemanticView{FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{{Kind: types.FacetPrincipalPathEdge, Tier: types.TierExpected,
				SourceCandidate: []string{"ev-call"}}},
		}}
		precise := validateFacetCoverage(doc, view)
		if len(precise) != 1 || precise[0].Kind != types.ViolFacetUncovered {
			t.Fatalf("real facet ownership producer did not fire: %+v", precise)
		}
		precise = append(precise, types.Violation{Kind: types.ViolGhostAnchor},
			types.Violation{Kind: types.ViolAuthorityOverreach}, types.Violation{Kind: types.ViolDiagramEdgeUnsupported})
		want := MaterializeUnresolvedViolationsAsCaveats(precise, lang)
		if len(want) != MaxMaterializedCaveats {
			t.Fatalf("positive control must exercise the original cap: %v", want)
		}
		var mixed []types.Violation
		for _, kind := range b54Tier2DisplayKinds() {
			mixed = append(mixed, types.Violation{Kind: kind}, types.Violation{Kind: kind})
		}
		mixed = append(mixed, precise...)
		for _, reverse := range []bool{false, true} {
			if reverse {
				for i, j := 0, len(mixed)-1; i < j; i, j = i+1, j-1 {
					mixed[i], mixed[j] = mixed[j], mixed[i]
				}
			}
			if got := MaterializeUnresolvedViolationsAsCaveats(mixed, lang); !reflect.DeepEqual(got, want) {
				t.Fatalf("noisy findings changed other families, cap, order, or dedup: got=%v want=%v", got, want)
			}
		}
	}
}

func TestB54Tier2TrackedAppendAndActualRerenderDoNotReviveCaveats(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, soft := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/soft=%v", lang, soft), func(t *testing.T) {
				input, family := b54Tier2DisplayInput(types.ViolEntityParityImbalanced)
				fail := agent.RunFamilyValidators(family, input)
				if fail == nil {
					t.Fatal("actual completeness producer must fire")
				}
				ctx := b54Tier2DisplayBus(input, lang)
				o := &Orchestrator{busCtx: ctx}
				before := b54Tier2DisplayJSON(t, ctx.Mutable.AnswerDocumentV2())
				baseline := o.renderFinalAnswerWithLastMileSupplements(ctx.Mutable.AnswerDocumentV2(), nil)
				violations := []types.Violation{{Kind: fail.ViolationKind, Detail: fail.Reason, Repair: fail.FixHint}}
				var got string
				if soft {
					got = o.appendSoftContractCaveatsTracked(baseline, violations)
				} else {
					got = o.appendUserCaveatsTracked(baseline, violations)
				}
				if got != baseline {
					t.Fatalf("tracked append shipped heuristic as a user claim:\n%s", got)
				}
				if o.answerCaveatReplay != nil && len(o.answerCaveatReplay.entries) != 0 {
					t.Fatalf("noisy finding was registered for later resurrection: %+v", o.answerCaveatReplay.entries)
				}
				// A separate, exact typed missing-facet note must survive the
				// same tracker and last-mile renderer, once, across two renders.
				precise := []types.Violation{{Kind: types.ViolFacetUncovered,
					ClusterKey: types.FacetClusterKey(string(types.FacetPrincipalPathEdge), "answer_facet_coverage")}}
				positive := MaterializeUnresolvedViolationsAsCaveats(precise, lang)
				if len(positive) != 1 {
					t.Fatalf("precise disclosure control is empty: %v", positive)
				}
				if soft {
					got = o.appendSoftContractCaveatsTracked(got, precise)
					got = o.appendSoftContractCaveatsTracked(got, precise)
				} else {
					got = o.appendUserCaveatsTracked(got, precise)
					got = o.appendUserCaveatsTracked(got, precise)
				}
				for i := 0; i < 2; i++ {
					rerendered := o.renderFinalAnswerWithLastMileSupplements(ctx.Mutable.AnswerDocumentV2(), nil)
					if rerendered != got || strings.Count(rerendered, positive[0]) != 1 {
						t.Fatalf("rerender lost/duplicated precise note or revived noisy note:\n%s", rerendered)
					}
				}
				if b54Tier2DisplayJSON(t, ctx.Mutable.AnswerDocumentV2()) != before {
					t.Fatal("tracked display mutated the model document or its graph")
				}
			})
		}
	}
}

func b54Tier2DisplayKinds() []types.ViolationKind {
	return []types.ViolationKind{types.ViolScalarCountUnsourced, types.ViolPathDepthInsufficient,
		types.ViolCardinalityShort, types.ViolEntityParityImbalanced}
}

func b54Tier2DisplayInput(kind types.ViolationKind) (agent.ValidatorInput, types.QuestionFamily) {
	rm := types.RequestModel{RawRequest: "Describe Port and its implementation types.", Intent: types.IntentEnumerate,
		PredicateAxis: types.AxisImplement, AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "model-summary", Kind: types.BlockSummary, Text: "Port is an interface. These are its 12 implementation types."},
		{ID: "model-members", Kind: types.BlockTable, Columns: []string{"Type", "File"}},
	}}
	var body strings.Builder
	body.WriteString("flowchart TD\n")
	evidence := []types.EvidenceItem{{AnchorSymbol: "Port"}}
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("Worker%02d", i)
		doc.Blocks[1].Items = append(doc.Blocks[1].Items, types.AnswerBlockItem{ID: name,
			Cells: []string{name, fmt.Sprintf("src/worker_%02d.go:10", i)}})
		fmt.Fprintf(&body, "  %s -->|implements| Port\n", name)
		evidence = append(evidence, types.EvidenceItem{AnchorSymbol: name})
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "model-graph", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: body.String()}})
	family := types.QFComparison
	switch kind {
	case types.ViolScalarCountUnsourced:
		rm.PredicateAxis = types.AxisUnknown
		rm.Predicates.IsCountQuestion = true
		doc.Blocks = []types.AnswerBlock{{ID: "model-count", Kind: types.BlockScalar, Text: "12"}}
		family = types.QFGeneric
	case types.ViolPathDepthInsufficient:
		rm.Intent, rm.PredicateAxis = types.IntentTrace, types.AxisCall
		rm.AnalyzerHints = types.AnalyzerHints{Kind: string(types.ReqCallChain), Entities: []string{"open", "close"}}
		doc.Blocks = []types.AnswerBlock{{ID: "model-path", Kind: types.BlockDiagram,
			Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n  open --> close"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "open", ToNode: "close", RelationKind: types.DiagramRelCall}}}}
		evidence = []types.EvidenceItem{{AnchorSymbol: "open"}, {AnchorSymbol: "close"}}
		family = types.QFCallChain
	case types.ViolCardinalityShort:
		rm.EnumerationBoundary = &types.RequestedEnumerationBoundary{DeclaredCount: 12}
		family = types.QFEnumeration
	case types.ViolEntityParityImbalanced:
		// Like r1055, an analyzer-supplied partition is not proof that the
		// user asked for comparison, nor that a stale anchor is a real side.
		rm.Buckets = []types.QuestionBucket{{Index: 1, Label: "Port", Anchors: []string{"Port"}},
			{Index: 2, Label: "implementation types", Anchors: []string{"BaseActor"}}}
	}
	return agent.ValidatorInput{IR: &types.AnalysisIR{RequestModel: rm}, EvidenceItems: evidence, AnswerDocumentV2: doc}, family
}

func b54Tier2DisplayBus(input agent.ValidatorInput, lang string) *types.BusContext {
	mut := types.NewMutableState(input.IR.RequestModel.RawRequest)
	mut.SetRequestModel(input.IR.RequestModel)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, input.AnswerDocumentV2)
	return &types.BusContext{Ctx: context.Background(), Language: lang, Mutable: mut,
		AnalysisIR: input.IR, EvidenceItems: input.EvidenceItems}
}

func b54AssertTier2AppendsUnchanged(t *testing.T, answer string, violations []types.Violation, lang string, ctx *types.BusContext) {
	t.Helper()
	if got := MaterializeUnresolvedViolationsAsCaveats(violations, lang); len(got) != 0 {
		t.Errorf("noisy completeness finding became a user assertion: %v", got)
	}
	for name, got := range map[string]string{
		"user":     AppendUserCaveatsToAnswer(answer, violations, lang),
		"soft":     AppendSoftContractCaveatsToAnswer(answer, violations, lang),
		"user_bus": AppendUserCaveatsToAnswerForBus(answer, violations, lang, ctx),
		"soft_bus": AppendSoftContractCaveatsToAnswerForBus(answer, violations, lang, ctx),
	} {
		if got != answer {
			t.Errorf("%s changed model answer/diagram bytes by appending a noisy finding:\n%s", name, got)
		}
	}
}

func b54Tier2DisplayJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
