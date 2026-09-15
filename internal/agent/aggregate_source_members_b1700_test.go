package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1700ObservedSourceCoordinateDoesNotProveUnrelatedAggregateMember(t *testing.T) {
	for _, member := range []string{"H:RenderService:DoFrame duration 86.111ms > 50ms", "unobserved runtime outcome"} {
		for _, external := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/external=%t", member, external), func(t *testing.T) {
				// b1695DefinitionBus uses the real parser, ReadFile and EmitEvidence;
				// this request deliberately has no relation/workflow restriction.
				bus := b1695DefinitionBus(t, "en")
				bus.AnalysisIR.RequestModel.Intent = types.IntentEnumerate
				bus.AnalysisIR.RequestModel.PredicateAxis = types.AxisDefine
				bus.AnalysisIR.RequestModel.Predicates = types.SemanticPredicates{IsCategoryEnumeration: true}
				bus.AnalysisIR.RequestModel.AnalyzerHints = types.AnalyzerHints{Kind: string(types.ReqEnumeration)}
				fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRolePrincipalAnswer,
					Label: "retained model claim", Value: "1", Members: []string{member},
					MemberNotes: []string{"retain the investigator's explanation verbatim"}, SupportRefs: []string{"worker.go:3"}}
				if external {
					fact.Dimensions = []types.AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}}
				}
				b1695RetainFacts(t, bus, []types.AnswerAggregateFact{fact})
				before, _ := json.Marshal(bus.Mutable.StableInvestigationAggregateFacts())
				ledger, prompt := b1695LedgerPrompt(t, bus)
				var record types.ObservationRecord
				for _, candidate := range ledger.Records {
					if candidate.ID == "aggregate:0#current_source" {
						record = candidate
						break
					}
				}
				if record.ClaimAuthority == types.ObservationClaimAuthorityIndependentlyProven {
					t.Errorf("unrelated member borrowed the observed definition's independent proof: %+v", record)
				}
				if record.SourceRef.Path != "worker.go" || record.Span.LineStart != 3 || !reflect.DeepEqual(record.SupportRefs, fact.SupportRefs) {
					t.Fatalf("claim qualification must retain the valid coordinate and declared support: %+v", record)
				}
				if !external && !strings.Contains(prompt, "principal_contract=`not_authorized`") || !strings.Contains(prompt, member) {
					t.Error("finalizer must retain the model member while withholding the source principal contract")
				}
				after, _ := json.Marshal(bus.Mutable.StableInvestigationAggregateFacts())
				if string(before) != string(after) {
					t.Fatal("qualification rewrote the retained model payload")
				}
				model := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
					{ID: "model-summary", Kind: types.BlockSummary, Text: "The observed result and the current source definition are separate facts."},
					{ID: "model-results", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
						{Text: member, CitationRef: -1},
						{Text: "The source defines Worker.Name.", CitationRefs: []int{0}},
					}},
				}, Citations: []types.Citation{{File: "worker.go", Line: 3, Quote: "func (w *Worker) Name() string { return \"worker\" }"}}}
				params, _ := json.Marshal(model)
				result, err := (&tool.EmitAnswerDocument{}).Execute(bus, params)
				if err != nil || !result.Success {
					t.Fatalf("actual full answer emission failed: %v %+v", err, result)
				}
				published := bus.Mutable.AnswerDocumentV2()
				if published == nil || len(published.Blocks) < len(model.Blocks) {
					t.Fatal("model blocks were not published")
				}
				for i, block := range model.Blocks {
					if block.ID != published.Blocks[i].ID || block.Kind != published.Blocks[i].Kind || types.AnswerBlockVisibleSurface(block) != types.AnswerBlockVisibleSurface(published.Blocks[i]) {
						t.Fatal("membership qualification changed model-authored text or block shape")
					}
				}
				for _, block := range published.Blocks {
					if block.SystemGeneratedKind == types.AnswerSystemGeneratedPrincipalEnumerationMissing || block.SystemGeneratedKind == types.AnswerSystemGeneratedPrincipalEnumerationFields {
						t.Errorf("unobserved model member became a system-authored source table: %+v", block)
					}
				}
				if len(published.Citations) != 1 || published.Citations[0].File != model.Citations[0].File ||
					published.Citations[0].Line != model.Citations[0].Line || published.Citations[0].Quote != model.Citations[0].Quote {
					t.Errorf("unobserved source member acquired new citations: %+v", published.Citations)
				}
				bad, good := published.Blocks[1].Items[0], published.Blocks[1].Items[1]
				if bad.CitationRef >= 0 || len(bad.CitationRefs) > 0 {
					t.Errorf("unobserved member acquired source bindings: %+v", bad)
				}
				if good.CitationRef != 0 {
					t.Errorf("actual source member lost original citation: %+v", good)
				}
				after, _ = json.Marshal(bus.Mutable.StableInvestigationAggregateFacts())
				if string(before) != string(after) {
					t.Fatal("answer publication changed the model aggregate")
				}
			})
		}
	}
}
