package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// denialStubOracle answers not-found for every symbol — the
// confirmed-fabrication shape.
type denialStubOracle struct{}

func (denialStubOracle) SymbolExists(string) (bool, int)     { return false, 0 }
func (denialStubOracle) SymbolExistsFlat(string) (bool, int) { return false, 0 }

// A confirmed-fabricated enumeration label stamps the oracle-symbol
// denial alongside the violation; a nil set stays a no-op.
func TestHallucinationSiteStampsOracleDenial(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "l1", Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{ID: "i1", Label: "completelyFabricatedIdentifierName"}},
	}}}
	denials := types.NewTypedDenialSet()
	v := validateEnumerationItemLabelHallucination(doc, denialStubOracle{}, denials)
	if len(v) == 0 {
		t.Fatalf("fabricated label must produce the violation")
	}
	if !denials.IsSymbolDenied("completelyFabricatedIdentifierName") {
		t.Fatalf("confirmed fabrication must stamp the symbol denial")
	}
	// nil set: violation still fires, no panic.
	if v := validateEnumerationItemLabelHallucination(doc, denialStubOracle{}, nil); len(v) == 0 {
		t.Fatalf("nil denial set must not change the violation behavior")
	}
}

// Finalize-entry evidence stamping: only the final-Ungrounded +
// dual-oracle-miss shape stamps; grounded rows and short/prose
// subjects never do.
func TestStampUngroundedEvidenceDenials(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{ID: "ev-1", AnchorSymbol: "fabricatedEvidenceAnchor", GroundingStatus: types.GroundingUngrounded},
		{ID: "ev-2", AnchorSymbol: "legitGroundedSymbolName", GroundingStatus: types.GroundingGrounded},
		{ID: "ev-3", Subject: "short", GroundingStatus: types.GroundingUngrounded},
	})
	denials := types.NewTypedDenialSet()
	stampUngroundedEvidenceDenials(denials, mut, denialStubOracle{})
	if !denials.IsSymbolDenied("fabricatedEvidenceAnchor") {
		t.Fatalf("final-ungrounded dual-miss subject must be stamped")
	}
	if denials.IsSymbolDenied("legitGroundedSymbolName") {
		t.Fatalf("grounded rows must never stamp")
	}
	if denials.IsSymbolDenied("short") {
		t.Fatalf("sub-floor tokens must never stamp")
	}

	// An oracle that vouches suppresses the stamp even when the row
	// stayed ungrounded (line/quote mismatch on a REAL symbol — the
	// repair-loop protection).
	vouching := vouchingStubOracle{}
	denials2 := types.NewTypedDenialSet()
	stampUngroundedEvidenceDenials(denials2, mut, vouching)
	if denials2.Len() != 0 {
		t.Fatalf("oracle-vouched subjects must never stamp, got %d", denials2.Len())
	}
}

type vouchingStubOracle struct{}

func (vouchingStubOracle) SymbolExists(string) (bool, int)     { return true, 1 }
func (vouchingStubOracle) SymbolExistsFlat(string) (bool, int) { return true, 1 }

// The perf-stall class is dual-shaped: its symbol half now reaches
// the symbol gates, per the long-standing IsSymbolDenied contract.
func TestPerfStallClassSymbolShaped(t *testing.T) {
	set := types.NewTypedDenialSet()
	set.Add(types.TypedDenial{
		Class:  types.TypedDenialExternalPerfStallUnresolved,
		Token:  "stalledRuntimeSymbol",
		Reason: "test",
	})
	if !set.IsSymbolDenied("stalledRuntimeSymbol") {
		t.Fatalf("perf-stall symbol token must be symbol-gate load-bearing")
	}
	set.Add(types.TypedDenial{
		Class:  types.TypedDenialExternalPerfStallUnresolved,
		Token:  "vendor/path/file.cc",
		Reason: "test",
	})
	if _, ok := set.FirstPathDenial("vendor/path/file.cc"); !ok {
		t.Fatalf("perf-stall path token must stay path-gate load-bearing")
	}
}
