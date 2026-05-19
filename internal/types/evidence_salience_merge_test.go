package types

import "testing"

func TestMergeExactResolutionSurfaceEvidenceCarriesSalience(t *testing.T) {
	dst := EvidenceItem{
		Kind:      EvidenceDirect,
		Subject:   "A",
		Source:    "a.go",
		LineStart: 10,
		Salience:  SalienceSupporting,
	}
	src := dst
	src.Salience = SalienceExhaustListed

	got := mergeExactResolutionSurfaceEvidence(dst, src)
	if got.Salience != SalienceExhaustListed {
		t.Fatalf("salience = %q, want exhaust_listed", got.Salience)
	}

	src.Salience = SalienceUnset
	got = mergeExactResolutionSurfaceEvidence(got, src)
	if got.Salience != SalienceExhaustListed {
		t.Fatalf("unset source must not clear salience, got %q", got.Salience)
	}
}
