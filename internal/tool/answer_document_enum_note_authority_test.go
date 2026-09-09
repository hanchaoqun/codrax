package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1620ShippingEnumerationSupplementDoesNotAdoptModelExplanation(t *testing.T) {
	mu := types.NewMutableState("list declarations and their purpose")
	mu.AppendEvidence([]types.EvidenceItem{enumEvidence("widget", "Widget", "src/Widget.go", 12, "the model infers ownership of another declaration")})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{Kind: types.AnswerAggregateMemberSet,
		Role: types.AnswerAggregateRolePrincipalAnswer, Label: "declarations", Value: "1", Members: []string{"Widget"},
		SupportRefs: []string{"Widget @ src/Widget.go:12"}, MemberNotes: []string{"the model proposes a business explanation"}}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentEnumerate, Language: "en", Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true,
			Dimensions: []types.RequestedAnswerDimension{{Index: 1, Label: "purpose", Role: types.RequestedAnswerDimensionFunctionOrPurpose}}},
	}}}
	authored := types.AnswerBlock{ID: "model-intro", Kind: types.BlockSection, Text: "My investigation is incomplete; my hypothesis remains mine."}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{authored}}
	before, _ := json.Marshal(doc.Blocks[0])
	if n := appendPrincipalEnumerationTypedSupplements(doc, ctx); n == 0 {
		t.Fatal("positive control: missing proved declaration still needs a typed supplement")
	}
	after, _ := json.Marshal(doc.Blocks[0])
	if string(before) != string(after) {
		t.Fatal("model-owned content was rewritten")
	}
	var surface strings.Builder
	for _, block := range doc.Blocks[1:] {
		surface.WriteString(types.AnswerBlockVisibleSurface(block))
	}
	for _, note := range []string{"the model infers ownership", "the model proposes a business explanation"} {
		if strings.Contains(surface.String(), note) {
			t.Errorf("system declaration supplement adopted an unproved explanation: %s", surface.String())
		}
	}
	if !strings.Contains(surface.String(), "Widget") || len(doc.Citations) == 0 {
		t.Fatal("exact declaration/citation facts were lost")
	}
	sets := types.CompileEnumerationDisplaySets(&ctx.AnalysisIR.RequestModel, answerSurfacePlan(ctx))
	if len(sets) == 0 || !strings.Contains(sets[0].Rows[0].Note, "business explanation") {
		t.Fatal("model guidance lost the candidate explanation")
	}
	// The second shipping carrier also runs at pre-emit/persist. It must not
	// reintroduce the same explanation, including through its evidence fallback.
	for _, rows := range [][]types.EnumerationDisplayRow{sets[0].Rows, nil} {
		carrier := &types.AnswerDocumentV2{DocumentModel: "v2"}
		fact := answerSurfacePlan(ctx).StableAggregateFacts[0]
		if n := appendAggregateMemberSetCarrier(carrier, ctx, 0, fact, rows, "declarations", ""); n == 0 {
			t.Fatal("second carrier positive control failed")
		}
		for _, item := range carrier.Blocks[0].Items {
			if item.Text != "" {
				t.Errorf("second carrier borrowed a summary: %+v", item)
			}
		}
	}
}

func TestB1620EnumerationRuntimeTemplatePreservesValuesForMixedOriginOrder(t *testing.T) {
	for _, origins := range [][]types.AnswerEvidenceOrigin{
		{types.AnswerEvidenceOriginRuntimeArtifact},
		{types.AnswerEvidenceOriginCurrentSource, types.AnswerEvidenceOriginRuntimeArtifact},
		{types.AnswerEvidenceOriginRuntimeArtifact, types.AnswerEvidenceOriginCurrentSource},
	} {
		row := types.EnumerationDisplayRow{Member: "worker", DisplayLabel: "worker", EvidenceOrigins: origins, Note: "runnable 1.234ms @ cpu=2"}
		doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
		block := buildPrincipalEnumerationRowsBlock(doc, types.EnumerationDisplaySet{ID: "runtime"}, []types.EnumerationDisplayRow{row}, false, principalEnumerationSupplementMissing)
		if !strings.Contains(types.AnswerBlockVisibleSurface(block), "1.234ms") {
			t.Errorf("runtime template lost typed values for origins %v", origins)
		}
	}
}
