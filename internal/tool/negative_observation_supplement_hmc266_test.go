package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC266NegativeObservationSchemaUsesSharedDeclarationTeaching(t *testing.T) {
	parameters := (&EmitInvestigationComplete{}).Parameters()
	var decoded map[string]any
	if err := json.Unmarshal(parameters, &decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(parameters), types.NegativeObservationTeaching) || strings.Contains(string(parameters), "__NEGATIVE_OBSERVATION_TEACHING__") {
		t.Fatal("schema must carry the shared declaration boundary without an unexpanded placeholder")
	}
}

// Exercise actual publication, not only the supplement helper. A model's
// well-shaped zero claim must not manufacture a system-owned no-hit proof.
func TestHMC266NegativeObservationPublicSupplementRequiresObservation(t *testing.T) {
	for _, origin := range []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginRuntimeArtifact, types.AnswerEvidenceOriginMCPResource, types.AnswerEvidenceOriginVCSMetadata} {
		for _, patch := range []bool{false, true} {
			name := string(origin) + "/emit"
			if patch {
				name = string(origin) + "/patch"
			}
			t.Run(name, func(t *testing.T) {
				bus := &types.BusContext{Language: "en", Mutable: types.NewMutableState("Describe the available evidence"),
					AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain, Language: "en"}}}
				model := json.RawMessage(`{"blocks":[{"id":"model","kind":"summary","text":"The selected observation is not available yet."}]}`)
				if patch {
					result, err := (&EmitAnswerDocument{}).Execute(bus, model)
					if err != nil || !result.Success {
						t.Fatalf("base publication failed: %v %s", err, result.Summary)
					}
				}
				facts, err := types.NormalizeAnswerAggregateFacts([]types.AnswerAggregateFact{{
					Kind: types.AnswerAggregateNegativeObservation, Label: "claim without execution", Value: "0", Role: types.AnswerAggregateRolePrincipalAnswer,
					Provenance: "trace_query", SupportRefs: []string{"mcp:resource://test/items"},
					Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: string(origin)}, {Name: "target", Value: "missing-item"}, {Name: "scope", Value: "bounded-resource"}, {Name: "tool_result", Value: "trace_query[0]"}, {Name: "payload_ref", Value: "blob://model-authored"}},
				}})
				if err != nil {
					t.Fatal(err)
				}
				bus.Mutable.SetInvestigationAggregateFacts(facts)
				bus.Mutable.RetainInvestigationAggregateFacts()
				before := bus.Mutable.StableInvestigationAggregateFacts()
				if patch {
					result, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"add_blocks":[{"id":"followup","kind":"caveat","text":"Execution still needs to be completed."}]}`))
					if err != nil || !result.Success {
						t.Fatalf("patch failed: %v %s", err, result.Summary)
					}
				} else {
					result, err := (&EmitAnswerDocument{}).Execute(bus, model)
					if err != nil || !result.Success {
						t.Fatalf("publication failed: %v %s", err, result.Summary)
					}
				}
				for _, block := range bus.Mutable.AnswerDocumentV2().Blocks {
					if block.SystemGeneratedKind == types.AnswerSystemGeneratedEvidenceSupplement && strings.Contains(block.ID, "negative-proof") {
						t.Errorf("unexecuted model claim became system-owned proof: %+v", block)
					}
				}
				if !reflect.DeepEqual(before, bus.Mutable.StableInvestigationAggregateFacts()) {
					t.Fatal("publication changed the retained model audit record")
				}
			})
		}
	}
}

func TestHMC266NegativeSearchPublicSupplementCompatibility(t *testing.T) {
	bus := &types.BusContext{Language: "en", Mutable: types.NewMutableState("Describe search evidence"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain, Language: "en"}}}
	facts, err := types.NormalizeAnswerAggregateFacts([]types.AnswerAggregateFact{{Kind: types.AnswerAggregateNegativeSearch, Label: "old option", Value: "0", Role: types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{{Name: "repo", Value: "."}, {Name: "scope", Value: "settings.go"}, {Name: "pattern", Value: "old_option"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	bus.Mutable.SetInvestigationAggregateFacts(facts)
	bus.Mutable.RetainInvestigationAggregateFacts()
	result, err := (&EmitAnswerDocument{}).Execute(bus, minimalV2EmitJSON())
	if err != nil || !result.Success {
		t.Fatalf("publication failed: %v %s", err, result.Summary)
	}
	found := false
	for _, block := range bus.Mutable.AnswerDocumentV2().Blocks {
		found = found || (block.SystemGeneratedKind == types.AnswerSystemGeneratedEvidenceSupplement && strings.Contains(block.ID, "negative-proof"))
	}
	if !found {
		t.Fatal("unrelated negative_search compatibility changed")
	}
}
