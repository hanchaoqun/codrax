package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The source rows are grounded declaration fixtures, not execution-membership
// receipts. Exercise the shared public full/partial persistence chokepoint;
// merely rendering a helper would miss a second supplement producer.
func b1658WorkflowPublicationContext(t *testing.T, lang string, refs bool) *types.BusContext {
	t.Helper()
	rm := types.RequestModel{
		Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
		Language: lang, PredicateAxis: types.AxisFlow,
		AnalyzerHints:          types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "the selected workflow"},
		Predicates:             types.SemanticPredicates{HasPerMemberTable: true, IsCrossComponent: true},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []types.RequestedAnswerDimension{{Index: 1, Required: true,
				Role: types.RequestedAnswerDimensionStageWorkflow, Label: "model-selected workflow table"}},
		},
	}
	mu := types.NewMutableState("Explain the selected workflow and its stage inputs and outputs")
	fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet,
		Role: types.AnswerAggregateRolePrincipalAnswer, Label: "model-selected roster, not a scope selector",
		Value: "7", Provenance: "model_emitted",
		Members: []string{"PrefixOne", "PrefixTwo", "OtherMode", "PhaseAlpha", "PhaseBeta", "PhaseGamma", "PhaseDelta"},
	}
	for i, member := range fact.Members {
		source, line := "workflow.go", i+2
		mu.AppendEvidence([]types.EvidenceItem{enumEvidence(fmt.Sprintf("definition-%d", i), member, source, line, "model explanation remains a candidate")})
		fact.MemberNotes = append(fact.MemberNotes, "model note for "+member)
		if refs {
			fact.SupportRefs = append(fact.SupportRefs, fmt.Sprintf("%s @ %s:%d", member, source, line))
		}
	}
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
	mu.RetainInvestigationAggregateFacts()
	return &types.BusContext{Mode: types.ModeRead, Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
}

func b1658ModelWorkflowDocument() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "model-summary", Kind: types.BlockSummary, Text: "These are the stages selected by the author, not an inventory of every declaration."},
		{ID: "model-table", Kind: types.BlockTable, Text: "| Stage | Input | Output |\n|---|---|---|\n| PhaseAlpha | request | classification |\n| PhaseBeta | classification | evidence |\n| PhaseGamma | evidence | structured facts |\n| PhaseDelta | structured facts | answer |"},
	}, Citations: []types.Citation{{File: "workflow.go", Line: 5, Quote: "PhaseAlpha"}}}
}

func TestB1658PublicWorkflowPublicationDoesNotPromoteDeclarationRoster(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en"} {
		for _, refs := range []bool{true, false} {
			for _, partial := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/refs=%t/partial=%t", lang, refs, partial), func(t *testing.T) {
					ctx := b1658WorkflowPublicationContext(t, lang, refs)
					doc := b1658ModelWorkflowDocument()
					modelBefore, _ := json.Marshal(doc.Blocks)
					factsBefore, _ := json.Marshal(ctx.Mutable.StableInvestigationAggregateFacts())
					citationsBefore, _ := json.Marshal(doc.Citations)
					originalCitation := doc.Citations[0]
					mutation, prev := types.NewReplaceAllMutation(doc), (*types.AnswerDocumentV2)(nil)
					if partial {
						prev = doc
						ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
						mutation = types.NewPartialMutation(&types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"model-summary", "model-table"}})
					}
					result, err := ApplyAndPersistMutation(ctx, "b1658-public", mutation, prev, time.Now())
					if err != nil || !result.Success {
						t.Fatalf("public persistence must not gain a workflow-completeness gate: err=%v result=%+v", err, result)
					}
					got := ctx.Mutable.AnswerDocumentV2()
					if got == nil || len(got.Blocks) < 2 {
						t.Fatal("model document was not persisted")
					}
					modelAfter, _ := json.Marshal(got.Blocks[:2])
					if string(modelBefore) != string(modelAfter) {
						t.Fatal("model blocks changed while narrowing system membership authority")
					}
					factsAfter, _ := json.Marshal(ctx.Mutable.StableInvestigationAggregateFacts())
					if string(factsBefore) != string(factsAfter) {
						t.Fatal("original aggregate facts or member notes changed")
					}
					if len(got.Citations) < 1 || !reflect.DeepEqual(got.Citations[0], originalCitation) {
						t.Fatal("model citation was removed or changed")
					}
					for _, block := range got.Blocks[2:] {
						if block.SystemGeneratedKind == types.AnswerSystemGeneratedPrincipalEnumerationMissing ||
							block.SystemGeneratedKind == types.AnswerSystemGeneratedPrincipalEnumerationFields ||
							strings.Contains(types.AnswerBlockVisibleSurface(block), "PrefixOne") ||
							strings.Contains(types.AnswerBlockVisibleSurface(block), "OtherMode") {
							t.Errorf("declaration existence became a system-authored workflow member roster: %+v", block)
						}
					}
					citationsAfter, _ := json.Marshal(got.Citations)
					if string(citationsBefore) != string(citationsAfter) {
						t.Errorf("unselected declarations acquired automatic citations: before=%s after=%s", citationsBefore, citationsAfter)
					}
					if !t.Failed() {
						replay := types.NewPartialMutation(&types.AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"model-summary", "model-table"}})
						replayed, replayErr := ApplyAndPersistMutation(ctx, "b1658-replay", replay, got, time.Now())
						if replayErr != nil || !replayed.Success {
							t.Fatalf("unchanged public replay failed: %v %+v", replayErr, replayed)
						}
						stored := ctx.Mutable.AnswerDocumentV2()
						if !reflect.DeepEqual(stored.Blocks, got.Blocks) || !reflect.DeepEqual(stored.Citations, got.Citations) {
							t.Fatal("replay reintroduced supplementation or changed model content")
						}
					}
				})
			}
		}
	}
}

func TestB1658PublicWorkflowKeepsIndependentTypedAuthority(t *testing.T) {
	for _, origin := range []string{"source_inventory_marker", "typed_relation_marker", "explicit_runtime"} {
		for _, refs := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/refs=%t", origin, refs), func(t *testing.T) {
				ctx := b1658WorkflowPublicationContext(t, "en", refs)
				facts := ctx.Mutable.StableInvestigationAggregateFacts()
				switch origin {
				case "source_inventory_marker":
					facts[0].Provenance = types.SourceInventoryPrincipalRowSetAggregateProvenance
				case "typed_relation_marker":
					facts[0].Provenance = types.TypedRelationPrincipalMemberSetAggregateProvenance
				case "explicit_runtime":
					facts[0].Dimensions = []types.AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}}
				}
				ctx.Mutable.SetInvestigationAggregateFacts(facts)
				ctx.Mutable.RetainInvestigationAggregateFacts()
				before, _ := json.Marshal(ctx.Mutable.StableInvestigationAggregateFacts())
				sets := types.CompileEnumerationDisplaySets(&ctx.AnalysisIR.RequestModel, answerSurfacePlan(ctx))
				if len(sets) != 1 || !types.AnswerAggregateFactAuthorizesPrincipalContract(facts[0], &ctx.AnalysisIR.RequestModel) ||
					!types.EnumerationDisplaySetAuthorizesPrincipalContract(&ctx.AnalysisIR.RequestModel, facts[0], sets[0]) {
					t.Fatal("workflow presentation erased independently typed membership/origin authority")
				}
				doc := b1658ModelWorkflowDocument()
				modelBefore, _ := json.Marshal(doc.Blocks)
				result, err := ApplyAndPersistMutation(ctx, "b1658-independent-authority", types.NewReplaceAllMutation(doc), nil, time.Now())
				if err != nil || !result.Success {
					t.Fatalf("independent typed authority failed public persistence: %v %+v", err, result)
				}
				got := ctx.Mutable.AnswerDocumentV2()
				if got == nil || len(got.Blocks) < 2 {
					t.Fatal("independent authority lost the model document")
				}
				// Runtime-origin facts retain their independent qualification but
				// the existing runtime-only publisher does not add source tables.
				if origin != "explicit_runtime" && len(got.Blocks) < 3 {
					t.Fatal("typed positive control lost its existing supplement")
				}
				if origin == "explicit_runtime" && !answerDocumentRuntimeObservationOnly(ctx) {
					t.Fatal("runtime positive must retain the existing origin-specific publication lane")
				}
				modelAfter, _ := json.Marshal(got.Blocks[:2])
				if string(modelBefore) != string(modelAfter) {
					t.Fatal("independent authority changed the model's authored blocks")
				}
				after, _ := json.Marshal(ctx.Mutable.StableInvestigationAggregateFacts())
				if string(before) != string(after) {
					t.Fatal("typed origin/provenance/source facts changed at publication")
				}
			})
		}
	}
}

func TestB1658PublicOrdinaryDeclarationInventoryStillPublishes(t *testing.T) {
	ctx := b1658WorkflowPublicationContext(t, "en", true)
	ctx.AnalysisIR.RequestModel = types.RequestModel{Intent: types.IntentEnumerate, Language: "en",
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true, HasPerMemberTable: true}}
	result, err := ApplyAndPersistMutation(ctx, "b1658-inventory-positive", types.NewReplaceAllMutation(b1658ModelWorkflowDocument()), nil, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("ordinary inventory failed: %v %+v", err, result)
	}
	found := false
	for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
		if block.SystemGeneratedKind == types.AnswerSystemGeneratedPrincipalEnumerationMissing && strings.Contains(types.AnswerBlockVisibleSurface(block), "PrefixOne") {
			found = true
		}
	}
	if !found {
		t.Fatal("genuine declaration enumeration lost its existing system supplement")
	}
}
