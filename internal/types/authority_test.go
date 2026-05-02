package types

import (
	"testing"
)

// TestClaimOriginIsValid_AcceptsAllCanonicalValues asserts that every
// declared ClaimOrigin value (including the empty zero value) reports
// itself as valid. Catches a regression where a new value is added to
// the const block but not to allClaimOrigins.
func TestClaimOriginIsValid_AcceptsAllCanonicalValues(t *testing.T) {
	for _, o := range AllClaimOrigins() {
		if !o.IsValid() {
			t.Errorf("ClaimOrigin %q reported !IsValid but is in canonical list", o)
		}
	}
}

// TestClaimOriginIsValid_RejectsUnknownValue asserts a non-canonical
// string is rejected. Defends against a typo in a yaml override.
func TestClaimOriginIsValid_RejectsUnknownValue(t *testing.T) {
	if ClaimOrigin("bogus").IsValid() {
		t.Errorf("ClaimOrigin(\"bogus\") reported IsValid; expected false")
	}
}

// TestClaimOriginIsLogDerived_IdentifiesLogAndPerfOnly asserts the
// log-derived predicate is exactly {log, perf} — current_repo and
// cross_source must NOT register as log-derived because they are
// independently confirmed by current code.
func TestClaimOriginIsLogDerived_IdentifiesLogAndPerfOnly(t *testing.T) {
	want := map[ClaimOrigin]bool{
		ClaimOriginUnknown:     false,
		ClaimOriginCurrentRepo: false,
		ClaimOriginLog:         true,
		ClaimOriginPerf:        true,
		ClaimOriginCrossSource: false,
	}
	for origin, expect := range want {
		if got := origin.IsLogDerived(); got != expect {
			t.Errorf("ClaimOrigin(%q).IsLogDerived() = %v; want %v", origin, got, expect)
		}
	}
}

// TestAuthorityCeilingIsValid_AcceptsAllCanonicalValues mirrors the
// origin validity test for the ceiling enum.
func TestAuthorityCeilingIsValid_AcceptsAllCanonicalValues(t *testing.T) {
	for _, a := range AllAuthorityCeilings() {
		if !a.IsValid() {
			t.Errorf("AuthorityCeiling %q reported !IsValid but is in canonical list", a)
		}
	}
}

// TestAuthorityCeilingIsValid_RejectsUnknownValue defends the closed
// enum guarantee.
func TestAuthorityCeilingIsValid_RejectsUnknownValue(t *testing.T) {
	if AuthorityCeiling("hedged").IsValid() {
		t.Errorf("AuthorityCeiling(\"hedged\") reported IsValid; expected false")
	}
}

// TestAuthorityCeilingAllowsStrongClaim_OnlyFactualAndUnknown asserts
// the renderer-facing predicate: only factual and the zero-value
// Unknown allow strong claims; conditional / historical / illustrative
// all force hedging.
func TestAuthorityCeilingAllowsStrongClaim_OnlyFactualAndUnknown(t *testing.T) {
	want := map[AuthorityCeiling]bool{
		AuthorityUnknown:      true,
		AuthorityFactual:      true,
		AuthorityConditional:  false,
		AuthorityHistorical:   false,
		AuthorityIllustrative: false,
	}
	for a, expect := range want {
		if got := a.AllowsStrongClaim(); got != expect {
			t.Errorf("AuthorityCeiling(%q).AllowsStrongClaim() = %v; want %v", a, got, expect)
		}
	}
}

// TestAuthorityCeilingIsCausalChainEligible_RejectsIllustrativeOnly
// asserts that the chain-eligibility predicate excludes illustrative
// (and only illustrative). Historical and conditional remain eligible
// because hedged causal claims are still valid causal claims.
func TestAuthorityCeilingIsCausalChainEligible_RejectsIllustrativeOnly(t *testing.T) {
	want := map[AuthorityCeiling]bool{
		AuthorityUnknown:      true,
		AuthorityFactual:      true,
		AuthorityConditional:  true,
		AuthorityHistorical:   true,
		AuthorityIllustrative: false,
	}
	for a, expect := range want {
		if got := a.IsCausalChainEligible(); got != expect {
			t.Errorf("AuthorityCeiling(%q).IsCausalChainEligible() = %v; want %v", a, got, expect)
		}
	}
}

// TestWeakerOf_PicksWeakestUnderStrictOrdering walks every pair of
// canonical ceilings and asserts WeakerOf respects the documented
// ordering (illustrative < historical < conditional < factual) AND
// the Unknown safety carve-out (Unknown must not pull down a sibling).
func TestWeakerOf_PicksWeakestUnderStrictOrdering(t *testing.T) {
	cases := []struct {
		a, b, want AuthorityCeiling
	}{
		// Same value: idempotent.
		{AuthorityFactual, AuthorityFactual, AuthorityFactual},
		{AuthorityConditional, AuthorityConditional, AuthorityConditional},
		{AuthorityHistorical, AuthorityHistorical, AuthorityHistorical},
		{AuthorityIllustrative, AuthorityIllustrative, AuthorityIllustrative},

		// Strict ordering.
		{AuthorityFactual, AuthorityConditional, AuthorityConditional},
		{AuthorityFactual, AuthorityHistorical, AuthorityHistorical},
		{AuthorityFactual, AuthorityIllustrative, AuthorityIllustrative},
		{AuthorityConditional, AuthorityHistorical, AuthorityHistorical},
		{AuthorityConditional, AuthorityIllustrative, AuthorityIllustrative},
		{AuthorityHistorical, AuthorityIllustrative, AuthorityIllustrative},

		// Symmetry.
		{AuthorityIllustrative, AuthorityFactual, AuthorityIllustrative},
		{AuthorityHistorical, AuthorityFactual, AuthorityHistorical},

		// Unknown safety: paired with anything set, defer to the set value.
		{AuthorityUnknown, AuthorityFactual, AuthorityFactual},
		{AuthorityUnknown, AuthorityConditional, AuthorityConditional},
		{AuthorityUnknown, AuthorityHistorical, AuthorityHistorical},
		{AuthorityUnknown, AuthorityIllustrative, AuthorityIllustrative},
		{AuthorityFactual, AuthorityUnknown, AuthorityFactual},
		{AuthorityIllustrative, AuthorityUnknown, AuthorityIllustrative},

		// Both Unknown: stays Unknown.
		{AuthorityUnknown, AuthorityUnknown, AuthorityUnknown},
	}
	for _, tc := range cases {
		if got := WeakerOf(tc.a, tc.b); got != tc.want {
			t.Errorf("WeakerOf(%q, %q) = %q; want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestEvidenceItemAuthorityFields_ZeroValueByDefault asserts a freshly
// constructed EvidenceItem leaves all three new fields at zero value.
// This guards the byte-identical legacy behaviour invariant: existing
// callers that do not opt in to AuthorityCeiling continue to see the
// same Origin (Unknown ⇒ legacy current_repo) and Authority (Unknown ⇒
// legacy factual-equivalent).
func TestEvidenceItemAuthorityFields_ZeroValueByDefault(t *testing.T) {
	var item EvidenceItem
	if item.Origin != ClaimOriginUnknown {
		t.Errorf("zero EvidenceItem.Origin = %q; want %q",
			item.Origin, ClaimOriginUnknown)
	}
	if item.Authority != AuthorityUnknown {
		t.Errorf("zero EvidenceItem.Authority = %q; want %q",
			item.Authority, AuthorityUnknown)
	}
	if item.AuthorityReason != "" {
		t.Errorf("zero EvidenceItem.AuthorityReason = %q; want \"\"",
			item.AuthorityReason)
	}
}

// TestAnswerSymbolAuthority_ZeroValueByDefault mirrors the EvidenceItem
// zero-value guarantee for AnswerSymbol — extractor code paths that do
// not opt in to AuthorityCeiling must continue to produce symbols with
// Authority="" so legacy renderer behaviour is preserved.
func TestAnswerSymbolAuthority_ZeroValueByDefault(t *testing.T) {
	var sym AnswerSymbol
	if sym.Authority != AuthorityUnknown {
		t.Errorf("zero AnswerSymbol.Authority = %q; want %q",
			sym.Authority, AuthorityUnknown)
	}
}
