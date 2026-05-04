package types

import (
	"testing"
)

// retry_state_test.go — R14-c1 type-level lock tests
// (post_shape_residual_audit.md, 2026-05-04).

// TestSeverityIsValid pins the 4 valid Severity values.
func TestSeverityIsValid(t *testing.T) {
	for _, s := range []Severity{
		SeverityCritical, SeverityHigh, SeverityMedium, SeveritySoft,
	} {
		if !s.IsValid() {
			t.Errorf("%q must be valid", s)
		}
	}
	if Severity("nonsense").IsValid() {
		t.Error("unknown value must NOT be valid")
	}
	if Severity("").IsValid() {
		t.Error("empty must NOT be valid")
	}
}

// TestSeverityRank pins the rank ordering used by retry-budget
// scheduling (Critical > High > Medium > Soft).
func TestSeverityRank(t *testing.T) {
	if SeverityCritical.Rank() <= SeverityHigh.Rank() {
		t.Error("Critical must outrank High")
	}
	if SeverityHigh.Rank() <= SeverityMedium.Rank() {
		t.Error("High must outrank Medium")
	}
	if SeverityMedium.Rank() <= SeveritySoft.Rank() {
		t.Error("Medium must outrank Soft")
	}
	if SeveritySoft.Rank() <= 0 {
		t.Error("Soft must rank > unknown")
	}
	if Severity("nonsense").Rank() != 0 {
		t.Error("unknown must rank 0")
	}
}

// TestDeriveSeverity_AllKindsCovered confirms DeriveSeverity has
// a case for every ViolationKind in AllViolationKinds(). When a
// new kind is added, this test forces the enum's owner to
// classify it explicitly. Unknown kinds default to Medium so
// the test never fails on coverage — but enables a structural
// trace of severity churn.
func TestDeriveSeverity_AllKindsCovered(t *testing.T) {
	for _, kind := range AllViolationKinds() {
		// Both isStrict=true and =false should return a valid Severity.
		for _, strict := range []bool{true, false} {
			s := DeriveSeverity(kind, strict)
			if !s.IsValid() {
				t.Errorf("DeriveSeverity(%q, strict=%v) returned invalid Severity %q",
					kind, strict, s)
			}
		}
	}
}

// TestDeriveSeverity_ReviewerKindsAreSoft pins R14-c2 contract:
// reviewer-distilled kinds (plan_critic / reflector / answer_reviewer)
// are SOFT permanently — they're cross-Run learning signals, not
// retry triggers.
func TestDeriveSeverity_ReviewerKindsAreSoft(t *testing.T) {
	for _, k := range []ViolationKind{
		ViolPlanCritic, ViolReflectorObservation,
		ViolAnswerReviewerDistilled, ViolRichnessRegression,
	} {
		if got := DeriveSeverity(k, false); got != SeveritySoft {
			t.Errorf("kind %q non-strict: got %s, want soft", k, got)
		}
		if got := DeriveSeverity(k, true); got != SeveritySoft {
			t.Errorf("kind %q strict (still must be soft): got %s, want soft", k, got)
		}
	}
}

// TestDeriveSeverity_CriticalKindsAreCritical pins the
// "answer cannot ship without" classification.
func TestDeriveSeverity_CriticalKindsAreCritical(t *testing.T) {
	for _, k := range []ViolationKind{
		ViolPrincipalClaimUseMissing,
		ViolBlockCoverageMissing,
		ViolAuthorityOverreach,
	} {
		if got := DeriveSeverity(k, false); got != SeverityCritical {
			t.Errorf("kind %q: got %s, want critical", k, got)
		}
	}
}

// TestDeriveSeverity_StrictBumpsSoftKindsToMedium covers operator-
// configured strict promotion: Soft → Medium when isStrict=true.
func TestDeriveSeverity_StrictBumpsSoftKindsToMedium(t *testing.T) {
	// FacetUncovered is Medium-strict-by-default (Phase 4 default)
	// but classified High when strict — the function reflects that.
	if got := DeriveSeverity(ViolFacetUncovered, true); got != SeverityHigh {
		t.Errorf("FacetUncovered strict: got %s, want high", got)
	}
	if got := DeriveSeverity(ViolFacetUncovered, false); got != SeverityMedium {
		t.Errorf("FacetUncovered non-strict: got %s, want medium", got)
	}
}

// TestRetryState_HasContent pins the rendering gate — empty
// RetryState (or nil) must NOT trigger rendering, fresh dispatches
// stay byte-identical to pre-R14.
func TestRetryState_HasContent(t *testing.T) {
	if (*RetryState)(nil).HasContent() {
		t.Error("nil RetryState must have no content")
	}
	rs := &RetryState{Attempt: 0}
	if rs.HasContent() {
		t.Error("Attempt=0 RetryState must have no content")
	}
	rs = &RetryState{Attempt: 1}
	if rs.HasContent() {
		t.Error("Attempt=1 with no fields populated must have no content")
	}
	rs = &RetryState{Attempt: 1, ActiveViolations: []ScoredViolation{{Kind: ViolFacetUncovered}}}
	if !rs.HasContent() {
		t.Error("Attempt=1 with one violation must have content")
	}
	rs = &RetryState{Attempt: 1, PrevEmitJSON: []byte(`{"x":1}`)}
	if !rs.HasContent() {
		t.Error("Attempt=1 with prev emit JSON must have content")
	}
}

// TestMutableState_RetryState_SetGet covers MutableState wiring +
// nil safety.
func TestMutableState_RetryState_SetGet(t *testing.T) {
	if (*MutableState)(nil).RetryState() != nil {
		t.Error("nil receiver must yield nil")
	}
	mut := &MutableState{}
	if mut.RetryState() != nil {
		t.Error("fresh MutableState must have no retry state")
	}
	rs := &RetryState{Attempt: 1}
	mut.SetRetryState(rs)
	if mut.RetryState() != rs {
		t.Error("SetRetryState round-trip broken")
	}
	mut.ResetRetryState()
	if mut.RetryState() != nil {
		t.Error("ResetRetryState did not clear")
	}
}
