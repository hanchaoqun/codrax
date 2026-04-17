package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestCountTier1Evidence covers the tier1/total classification
// independently of the orchestrator wiring.
func TestCountTier1Evidence(t *testing.T) {
	items := []types.EvidenceItem{
		{GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText},
		{GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierSymbolTable},
		{GroundingStatus: types.GroundingRecovered, GroundingTier: types.TierFQNameSameFile},
		{GroundingStatus: types.GroundingUngrounded},
		{}, // legacy empty-status → counts as Tier-1
	}
	tier1, total := countTier1Evidence(items)
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	// Tier-1 (line_text): 1
	// Tier-2 grounded (symbol_table): 0 (NOT tier 1)
	// Recovered / Ungrounded: 0
	// Legacy empty: 1
	if tier1 != 2 {
		t.Errorf("tier1 = %d, want 2", tier1)
	}
}

// TestCountTier1Evidence_AllRecovered — pure-recovery investigation
// (the log 172408 scenario): zero Tier-1 against total evidence.
func TestCountTier1Evidence_AllRecovered(t *testing.T) {
	items := []types.EvidenceItem{
		{GroundingStatus: types.GroundingRecovered},
		{GroundingStatus: types.GroundingRecovered},
		{GroundingStatus: types.GroundingRecovered},
		{GroundingStatus: types.GroundingRecovered},
	}
	tier1, total := countTier1Evidence(items)
	if tier1 != 0 {
		t.Errorf("tier1 = %d, want 0", tier1)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
}
