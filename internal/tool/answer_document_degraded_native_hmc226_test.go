package tool

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc226DegradedNativeBlocks(doc *types.AnswerDocumentV2) []types.AnswerBlock {
	var blocks []types.AnswerBlock
	for _, block := range doc.Blocks {
		if block.SystemGeneratedKind == types.AnswerSystemGeneratedNativeFacts {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func TestHMC226DegradedNativeFactsPublicExport(t *testing.T) {
	for _, domain := range []string{"log", "measurement"} {
		t.Run(domain, func(t *testing.T) {
			bus, wantRows := nativeMixedPublicContext(t, domain)
			originalPath := ""
			if domain == "measurement" {
				originalPath = hmc287MeasurementOriginal(t)
			}
			metadataOracle := hmc287CurrentMetadataOracle(t, bus, originalPath, "3000000000")
			model := types.AnswerBlock{ID: "lead", Kind: types.BlockSummary, Text: "The original records are separate from this unaccepted draft."}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{model}}
			view := types.BuildAnswerSemanticViewForBusContext(bus)
			var original types.RuntimeMeasurementTable
			for _, table := range view.RuntimeMeasurementContract.Choices() {
				if table.DefaultPresentation && !metadataOracle.matches(t, table) {
					original = table
				}
			}
			before, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
			projection, _ := json.Marshal(bus.Mutable.TraceRootCauseReport())
			for i := 0; i < 2; i++ {
				sections := MaterializeDeterministicAnswerSectionsForDegradedDoc(bus, doc)
				blocks := hmc226DegradedNativeBlocks(doc)
				facts, metadata := metadataOracle.splitBlocks(t, blocks)
				wantMetadata := 0
				if domain == "measurement" {
					wantMetadata = 1
				}
				if !slices.Contains(sections, "native_facts") || len(facts) != 1 || len(metadata) != wantMetadata || len(blocks) != len(facts)+len(metadata) {
					t.Fatalf("degraded export lost native facts/disclosure: sections=%v blocks=%d", sections, len(blocks))
				}
				receipt := facts[0].RuntimeMeasurement
				if receipt == nil || !receipt.IsBound() || len(receipt.BoundTable.Rows) != wantRows || !reflect.DeepEqual(receipt.BoundTable.Rows, original.Rows) {
					t.Fatal("degraded export reconstructed or duplicated original rows")
				}
				if !reflect.DeepEqual(doc.Blocks[0], model) || bus.Mutable.AnswerDocumentV2() != nil || bus.Mutable.IsInvestigationComplete() {
					t.Fatal("display rewrote the model draft or promoted answer/completion authority")
				}
			}
			after, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
			projectionAfter, _ := json.Marshal(bus.Mutable.TraceRootCauseReport())
			if string(before) != string(after) || string(projection) != string(projectionAfter) {
				t.Fatal("display changed observation or causal authority")
			}
		})
	}
}

func TestHMC226DegradedNativeFactsCurrentScopeBoundaries(t *testing.T) {
	for _, negative := range []string{"no_fact_dimension", "source_inventory", "outside_window", "cleared_supply"} {
		t.Run(negative, func(t *testing.T) {
			bus, _ := nativeMixedPublicContext(t, "measurement")
			metadataOracle := hmc287CurrentMetadataOracle(t, bus, hmc287MeasurementOriginal(t), "3000000000")
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "lead", Kind: types.BlockSummary, Text: "Unaccepted draft."}}}
			MaterializeDeterministicAnswerSectionsForDegradedDoc(bus, doc)
			if len(hmc226DegradedNativeBlocks(doc)) == 0 {
				t.Fatal("positive control failed to materialize current facts")
			}
			rm := &bus.AnalysisIR.RequestModel
			switch negative {
			case "no_fact_dimension":
				rm.RequestedAnswerDimensions = nil // causal scope alone is not a fact request
			case "source_inventory":
				rm.SourceInventoryProfile = &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}}
			case "outside_window":
				start, end := 8.0, 9.0
				rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "8..9"}
			case "cleared_supply":
				bus.Mutable.ResetTurnAArtifacts()
			}
			bus.Mutable.SetRequestModel(*rm)
			sections := MaterializeDeterministicAnswerSectionsForDegradedDoc(bus, doc)
			blocks := hmc226DegradedNativeBlocks(doc)
			facts, metadata := metadataOracle.splitBlocks(t, blocks)
			wantMetadata := 0
			if negative == "outside_window" {
				wantMetadata = 1
			}
			if len(facts) != 0 || len(metadata) != wantMetadata || len(blocks) != wantMetadata || slices.Contains(sections, "native_facts") != (wantMetadata != 0) {
				t.Fatal("recovery retained stale/foreign default facts or claimed nonexistent output")
			}
		})
	}
}

func TestHMC226DegradedNativeFactsRejectOldPairEpoch(t *testing.T) {
	bus, result := hmc220PairPublicFixture(t)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "lead", Kind: types.BlockSummary, Text: "Unaccepted comparison draft."}}}
	MaterializeDeterministicAnswerSectionsForDegradedDoc(bus, doc)
	oldIDs := map[string]bool{}
	for _, block := range hmc226DegradedNativeBlocks(doc) {
		if block.RuntimeMeasurement.BoundTable.View == types.RuntimeMeasurementMembers {
			oldIDs[block.RuntimeMeasurement.ObservationID] = true
		}
	}
	if len(oldIDs) == 0 {
		t.Fatal("positive pair control did not publish measured rows")
	}
	bus.Mutable.ResetTurnAArtifacts()
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	MaterializeDeterministicAnswerSectionsForDegradedDoc(bus, doc)
	for _, block := range hmc226DegradedNativeBlocks(doc) {
		if oldIDs[block.RuntimeMeasurement.ObservationID] {
			t.Fatal("restoring an old private pair receipt revived measured facts")
		}
	}
	if bus.Mutable.AnswerDocumentV2() != nil {
		t.Fatal("degraded display created an accepted answer")
	}
}
