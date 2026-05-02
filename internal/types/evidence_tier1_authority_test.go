package types

import "testing"

// TestEvidenceCountsTowardTier1Floor_ExcludesIllustrativeAuthority:
// round-3 fix — Tier-1 floor MUST exclude items whose Authority is
// AuthorityIllustrative, not just those with ContextRole=
// illustrative_only. ComputeForEvidence sets Authority=Illustrative
// for any GroundingUngrounded item regardless of ContextRole; closure
// + render must agree on which items are "real evidence".
func TestEvidenceCountsTowardTier1Floor_ExcludesIllustrativeAuthority(t *testing.T) {
	ev := EvidenceItem{
		Kind:      EvidenceDirect,
		Scope:     ScopeLine,
		Source:    "x.go",
		LineStart: 10,
		// ContextRole NOT illustrative_only — only Authority is.
		ContextRole: EvidenceContextRoleDefining,
		Authority:   AuthorityIllustrative,
	}
	if EvidenceCountsTowardTier1Floor(ev) {
		t.Errorf("illustrative-Authority item counted toward Tier-1 floor")
	}
}

// TestEvidenceCountsTowardTier1Floor_FactualCountsRegardless: a
// factual item with line-shaped scope counts even if other fields
// vary. Defends against an over-eager Authority exclusion.
func TestEvidenceCountsTowardTier1Floor_FactualCountsRegardless(t *testing.T) {
	ev := EvidenceItem{
		Kind:      EvidenceDirect,
		Scope:     ScopeLine,
		Source:    "x.go",
		LineStart: 10,
		Authority: AuthorityFactual,
	}
	if !EvidenceCountsTowardTier1Floor(ev) {
		t.Errorf("factual line-shaped item NOT counted toward Tier-1 floor")
	}
}

// TestEvidenceCountsTowardTier1Floor_ConditionalAndHistoricalCount:
// these ceilings still represent grounded evidence (just hedged). They
// must still count toward Tier-1 floor — without them a drift-bounded
// answer would never satisfy the floor and force re-explore loops.
func TestEvidenceCountsTowardTier1Floor_ConditionalAndHistoricalCount(t *testing.T) {
	for _, a := range []AuthorityCeiling{AuthorityConditional, AuthorityHistorical} {
		ev := EvidenceItem{
			Kind:      EvidenceDirect,
			Scope:     ScopeLine,
			Source:    "x.go",
			LineStart: 10,
			Authority: a,
		}
		if !EvidenceCountsTowardTier1Floor(ev) {
			t.Errorf("Authority=%q not counted toward Tier-1 floor", a)
		}
	}
}
