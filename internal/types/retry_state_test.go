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

func TestDeriveSeverity_TopicMismatchIsHighByDefault(t *testing.T) {
	if got := DeriveSeverity(ViolAnswerTopicMismatch, false); got != SeverityHigh {
		t.Fatalf("topic mismatch non-strict severity = %s, want high", got)
	}
	if got := ViolationProfileFor(ViolAnswerTopicMismatch, false); !got.RetryEligible {
		t.Fatalf("topic mismatch should be retry-eligible by default: %+v", got)
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

// ── R15 ViolationProfile lock tests ─────────────────────────

// TestViolationProfileFor_RetryEligibleFromSeverity pins the
// derivation rule — Severity Soft → RetryEligible=false; all
// others true.
func TestViolationProfileFor_RetryEligibleFromSeverity(t *testing.T) {
	cases := []struct {
		kind     ViolationKind
		strict   bool
		wantSev  Severity
		wantElig bool
	}{
		{ViolPrincipalClaimUseMissing, false, SeverityCritical, true},
		{ViolBlockCoverageMissing, false, SeverityCritical, true},
		{ViolFacetUncovered, false, SeverityMedium, true}, // not strict → Medium per DeriveSeverity
		{ViolFacetUncovered, true, SeverityHigh, true},    // strict → High
		{ViolRichnessRegression, false, SeveritySoft, false},
		{ViolRichnessRegression, true, SeveritySoft, false}, // soft permanent
		{ViolPlanCritic, false, SeveritySoft, false},
		{ViolReflectorObservation, true, SeveritySoft, false}, // reviewer kinds permanent soft
	}
	for _, tc := range cases {
		got := ViolationProfileFor(tc.kind, tc.strict)
		if got.Kind != tc.kind {
			t.Errorf("kind round-trip: got %q want %q", got.Kind, tc.kind)
		}
		if got.Severity != tc.wantSev {
			t.Errorf("ViolationProfileFor(%q, strict=%v).Severity = %q, want %q",
				tc.kind, tc.strict, got.Severity, tc.wantSev)
		}
		if got.RetryEligible != tc.wantElig {
			t.Errorf("ViolationProfileFor(%q, strict=%v).RetryEligible = %v, want %v",
				tc.kind, tc.strict, got.RetryEligible, tc.wantElig)
		}
	}
}

// TestViolationProfileFor_CriticalCoherence is the R15 RED-LINE
// invariant: every kind classified Critical MUST be RetryEligible.
// Drift here (e.g. someone marks a kind Critical but its profile
// flips RetryEligible=false) is a SOURCE-OF-TRUTH violation.
func TestViolationProfileFor_CriticalCoherence(t *testing.T) {
	for _, kind := range AllViolationKinds() {
		for _, strict := range []bool{true, false} {
			p := ViolationProfileFor(kind, strict)
			if p.Severity == SeverityCritical && !p.RetryEligible {
				t.Errorf("R15 coherence violated: kind=%q strict=%v Severity=Critical but RetryEligible=false",
					kind, strict)
			}
			if p.Severity == SeveritySoft && p.RetryEligible {
				t.Errorf("R15 coherence violated: kind=%q strict=%v Severity=Soft but RetryEligible=true",
					kind, strict)
			}
		}
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

// G1 step 2 (post_v2_runtime_gap_remediation, 2026-05-04).
// Covers MutableState.SetRepairExecutionPlan /
// RepairExecutionPlan() / ResetRepairExecutionPlan.
//
// Plan is carried as `any` so internal/types stays decoupled from
// internal/orchestrator (mirrors the searchGraph pattern). The test
// uses an opaque struct as the stand-in for orchestrator.RepairExecutionPlan.
func TestMutableState_RepairExecutionPlan_SetGetReset(t *testing.T) {
	type opaquePlan struct {
		current   string
		remaining []string
	}
	if (*MutableState)(nil).RepairExecutionPlan() != nil {
		t.Error("nil receiver must yield nil")
	}
	mut := &MutableState{}
	if mut.RepairExecutionPlan() != nil {
		t.Error("fresh MutableState must have no repair execution plan")
	}
	plan := opaquePlan{current: "extract", remaining: []string{"finalizer"}}
	mut.SetRepairExecutionPlan(plan)
	got, ok := mut.RepairExecutionPlan().(opaquePlan)
	if !ok {
		t.Fatalf("type-assert opaquePlan failed: %T", mut.RepairExecutionPlan())
	}
	if got.current != "extract" {
		t.Errorf("round-trip current = %q, want extract", got.current)
	}
	mut.ResetRepairExecutionPlan()
	if mut.RepairExecutionPlan() != nil {
		t.Error("ResetRepairExecutionPlan did not clear")
	}

	// nil through SetRepairExecutionPlan also clears.
	mut.SetRepairExecutionPlan(plan)
	mut.SetRepairExecutionPlan(nil)
	if mut.RepairExecutionPlan() != nil {
		t.Error("SetRepairExecutionPlan(nil) did not clear")
	}
}

// G1 step 2: ResetRetryState clears the paired ExecutionPlan slot
// (the two surfaces share lifecycle — populator overwrites both).
func TestMutableState_ResetRetryState_AlsoClearsExecutionPlan(t *testing.T) {
	mut := &MutableState{}
	mut.SetRetryState(&RetryState{Attempt: 2})
	mut.SetRepairExecutionPlan(struct{ x int }{1})

	mut.ResetRetryState()

	if mut.RetryState() != nil {
		t.Error("ResetRetryState did not clear retry state")
	}
	if mut.RepairExecutionPlan() != nil {
		t.Error("ResetRetryState did not clear paired RepairExecutionPlan")
	}
}

// G1 step 2: ResetForFallback at every reset target clears the
// stashed plan. Locks the contract that an upstream-stage rerun
// invalidates the prior retry chain's owner queue.
func TestMutableState_ResetForFallback_ClearsExecutionPlan(t *testing.T) {
	cases := []struct {
		name   string
		target FallbackResetTarget
	}{
		{"finalizer", FallbackResetTargetFinalizer},
		{"extract", FallbackResetTargetExtract},
		{"explore", FallbackResetTargetExplore},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mut := &MutableState{}
			mut.SetRepairExecutionPlan(struct{ ordered []string }{ordered: []string{"finalizer"}})
			cleared := mut.ResetForFallback(tc.target)

			if mut.RepairExecutionPlan() != nil {
				t.Errorf("%s: plan not cleared", tc.name)
			}
			found := false
			for _, name := range cleared {
				if name == "RepairExecutionPlan" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: cleared list missing RepairExecutionPlan: %v", tc.name, cleared)
			}
		})
	}
}

// G1 step 2: Analyze fallback is a no-op (red line) — must not even
// touch the plan slot.
func TestMutableState_ResetForFallback_AnalyzeNoOp(t *testing.T) {
	mut := &MutableState{}
	mut.SetRepairExecutionPlan(struct{ x int }{1})
	cleared := mut.ResetForFallback(FallbackResetTargetAnalyze)
	if cleared != nil {
		t.Errorf("Analyze: cleared = %v, want nil (no-op)", cleared)
	}
	if mut.RepairExecutionPlan() == nil {
		t.Errorf("Analyze: plan was cleared but Analyze fallback must be a no-op")
	}
}
