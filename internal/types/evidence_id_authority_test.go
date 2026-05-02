package types

import "testing"

// TestStableEvidenceID_AuthorityDifferentiates: two items at the
// same anchor (kind/source/lines/etc.) but different Authority must
// hash to DIFFERENT IDs. Pre-round-4, identical IDs would dedupe-
// merge two items with conflicting ceilings, silently losing the
// stronger one.
func TestStableEvidenceID_AuthorityDifferentiates(t *testing.T) {
	base := EvidenceItem{
		Kind:      EvidenceDirect,
		Scope:     ScopeLine,
		Source:    "x.go",
		LineStart: 10,
		Subject:   "X",
	}
	a := base
	a.Authority = AuthorityFactual
	b := base
	b.Authority = AuthorityHistorical
	if StableEvidenceID(a) == StableEvidenceID(b) {
		t.Errorf("Authority did not differentiate IDs: a=%s b=%s",
			StableEvidenceID(a), StableEvidenceID(b))
	}
}

// TestStableEvidenceID_OriginDifferentiates: same shape for Origin.
// A current_repo emit and a log-derived emit at the same anchor are
// epistemically distinct items.
func TestStableEvidenceID_OriginDifferentiates(t *testing.T) {
	base := EvidenceItem{
		Kind:      EvidenceDirect,
		Scope:     ScopeLine,
		Source:    "x.go",
		LineStart: 10,
		Subject:   "X",
		Authority: AuthorityFactual,
	}
	a := base
	a.Origin = ClaimOriginCurrentRepo
	b := base
	b.Origin = ClaimOriginLog
	if StableEvidenceID(a) == StableEvidenceID(b) {
		t.Errorf("Origin did not differentiate IDs: a=%s b=%s",
			StableEvidenceID(a), StableEvidenceID(b))
	}
}

// TestStableEvidenceID_IdenticalAuthoritySameID: two items with the
// SAME (kind/source/lines/Authority/Origin) hash to the SAME ID —
// the differentiation is one-way.
func TestStableEvidenceID_IdenticalAuthoritySameID(t *testing.T) {
	a := EvidenceItem{
		Kind:      EvidenceDirect,
		Scope:     ScopeLine,
		Source:    "x.go",
		LineStart: 10,
		Subject:   "X",
		Authority: AuthorityFactual,
		Origin:    ClaimOriginCurrentRepo,
	}
	b := a
	if StableEvidenceID(a) != StableEvidenceID(b) {
		t.Errorf("identical Authority/Origin produced different IDs")
	}
}
