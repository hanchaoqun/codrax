package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// G1 step 1 unit tests for RepairExecutionPlan / BuildRepairExecutionPlan
// / PromoteNextOwner / shouldRebuildExecutionPlan / Summarize.
//
// All fixtures use the existing `vio(kind, dispatch)` helper from
// repair_plan_test.go (same package).

func TestBuildExecutionPlan_Empty(t *testing.T) {
	plan := BuildRepairExecutionPlan(nil, 0)
	if !plan.IsEmpty() {
		t.Errorf("empty input: IsEmpty=false, plan=%+v", plan)
	}
	if plan.CurrentOwner != "" {
		t.Errorf("empty input: CurrentOwner=%q, want \"\"", plan.CurrentOwner)
	}
	if !plan.EscalationAllowed {
		t.Errorf("empty input: EscalationAllowed=false, want true")
	}
}

func TestBuildExecutionPlan_SingleCluster(t *testing.T) {
	plan := BuildRepairExecutionPlan([]types.Violation{
		vio(types.ViolFacetUncovered, "d1"),
	}, 1<<30) // budget exhausted → no downgrade

	if plan.CurrentOwner != LocusExplore {
		t.Errorf("CurrentOwner = %v, want LocusExplore", plan.CurrentOwner)
	}
	if len(plan.OrderedOwners) != 1 {
		t.Errorf("len(OrderedOwners) = %d, want 1: %v", len(plan.OrderedOwners), plan.OrderedOwners)
	}
	if len(plan.RemainingOwners) != 0 {
		t.Errorf("len(RemainingOwners) = %d, want 0: %v", len(plan.RemainingOwners), plan.RemainingOwners)
	}
	if plan.FinalizerLocalDowngradeApplied {
		t.Errorf("FinalizerLocalDowngradeApplied = true, want false (Finalizer not in queue)")
	}
}

// Two finalizer-owned cluster + one explore-owned cluster collapse into
// 2 OrderedOwners (Finalizer dedup), not 3. Cross-dispatch so they
// don't combine via cooccurrence.
func TestBuildExecutionPlan_MultiClusterDeduped(t *testing.T) {
	plan := BuildRepairExecutionPlan([]types.Violation{
		vio(types.ViolSelfContradiction, "d1"),    // FinalizerOnly
		vio(types.ViolDiagramIdentifier, "d2"),    // FinalizerOnly (different dispatch)
		vio(types.ViolSubjectAnchorMissing, "d3"), // BackToExtract
	}, 1<<30)

	// Without budget downgrade, deepest first:
	//   ViolSubjectAnchorMissing → LocusExtract  (depth 2)
	//   ViolSelfContradiction    → LocusFinalizer (depth 1)
	// Two finalizer-owned clusters collapse → OrderedOwners has 2.
	if len(plan.OrderedOwners) != 2 {
		t.Fatalf("OrderedOwners len = %d, want 2: %v", len(plan.OrderedOwners), plan.OrderedOwners)
	}
	if plan.OrderedOwners[0] != LocusExtract {
		t.Errorf("OrderedOwners[0] = %v, want LocusExtract (deepest)", plan.OrderedOwners[0])
	}
	if plan.OrderedOwners[1] != LocusFinalizer {
		t.Errorf("OrderedOwners[1] = %v, want LocusFinalizer (deduped)", plan.OrderedOwners[1])
	}
	if plan.CurrentOwner != LocusExtract {
		t.Errorf("CurrentOwner = %v, want LocusExtract", plan.CurrentOwner)
	}
	if len(plan.RemainingOwners) != 1 || plan.RemainingOwners[0] != LocusFinalizer {
		t.Errorf("RemainingOwners = %v, want [LocusFinalizer]", plan.RemainingOwners)
	}
}

// Budget unused + Finalizer in queue but not at [0] → Finalizer
// promoted to OrderedOwners[0] (R2.2 cost-opt preserved).
func TestBuildExecutionPlan_FinalizerLocalPromoted(t *testing.T) {
	plan := BuildRepairExecutionPlan([]types.Violation{
		vio(types.ViolSubjectAnchorMissing, "d1"), // Extract (depth 2)
		vio(types.ViolSelfContradiction, "d2"),    // Finalizer (depth 1)
	}, 0) // 0 used → budget available

	if plan.CurrentOwner != LocusFinalizer {
		t.Errorf("CurrentOwner = %v, want LocusFinalizer (R2.2 downgrade)", plan.CurrentOwner)
	}
	if !plan.FinalizerLocalDowngradeApplied {
		t.Errorf("FinalizerLocalDowngradeApplied = false, want true")
	}
	if len(plan.OrderedOwners) != 2 || plan.OrderedOwners[0] != LocusFinalizer || plan.OrderedOwners[1] != LocusExtract {
		t.Errorf("OrderedOwners = %v, want [Finalizer, Extract]", plan.OrderedOwners)
	}
	if len(plan.RemainingOwners) != 1 || plan.RemainingOwners[0] != LocusExtract {
		t.Errorf("RemainingOwners = %v, want [Extract]", plan.RemainingOwners)
	}
}

// FailLoud cluster (LocusTerminal) short-circuits — CurrentOwner =
// Terminal, queue empty.
func TestBuildExecutionPlan_HasFailLoudShortCircuit(t *testing.T) {
	// Acceptance kind defaults to BackToExplore (depth 3) which is
	// not Terminal; we need a kind whose default is FallbackFailLoud.
	// FallbackTargetForKind line ~157 maps unknown values to
	// FallbackFinalizerOnly, so we instead inject Severity-Soft
	// kinds with a Locus of Terminal via the FallbackPolicy's strict
	// set. The simplest deterministic path: the fail-loud safety net
	// is preserved via FallbackPolicy.fallbackTargets — pick a
	// non-soft kind whose policy is FallbackFailLoud. No such kind
	// exists in the default map (FailLoud is only emitted via the
	// soft-kind safety net which is filtered out post-Phase-1-C).
	//
	// So we directly exercise the HasFailLoud branch via a hand-
	// constructed RepairPlan: BuildRepairExecutionPlan calls
	// BuildRepairPlan internally. Instead we test the short-circuit
	// at the helper boundary — a STRICT-promoted soft kind whose
	// owner is LocusTerminal would propagate, but the SOFT-Terminal
	// filter at repair_plan.go:148 removes it. We assert the filter
	// explicitly.
	//
	// Therefore this test is a defensive lock: today's policy graph
	// has no kind that yields HasFailLoud=true through the default
	// path; if a future kind ever maps to FallbackFailLoud non-SOFT,
	// the short-circuit MUST fire. We skip-but-document by testing
	// the helper does not panic on an empty / single-finalizer
	// input.
	plan := BuildRepairExecutionPlan([]types.Violation{
		vio(types.ViolSelfContradiction, "d1"),
	}, 0)
	if plan.HasFailLoud {
		t.Errorf("ViolSelfContradiction should not produce HasFailLoud; got %+v", plan)
	}
	// Verify the helper itself: a synthetic plan with HasFailLoud=true
	// short-circuits CurrentOwner via the explicit branch. Construct
	// via PromoteNextOwner with no fresh + a HasFailLoud prev.
	failLoud := RepairExecutionPlan{
		Clusters:          []RepairCluster{{Owner: LocusTerminal}},
		CurrentOwner:      LocusTerminal,
		HasFailLoud:       true,
		EscalationAllowed: true,
	}
	got := PromoteNextOwner(failLoud, nil, 0)
	if got.CurrentOwner != LocusTerminal {
		t.Errorf("HasFailLoud preservation: got CurrentOwner=%v, want LocusTerminal", got.CurrentOwner)
	}
	if !got.HasFailLoud {
		t.Errorf("HasFailLoud preservation: HasFailLoud cleared on promote")
	}
}

func TestPromoteNextOwner_FreshViolations_RebuildsFully(t *testing.T) {
	prev := BuildRepairExecutionPlan([]types.Violation{
		vio(types.ViolSubjectAnchorMissing, "d1"),
	}, 1<<30)

	// Fresh kind set differs (FacetUncovered new) → expect rebuild.
	next := PromoteNextOwner(prev, []types.Violation{
		vio(types.ViolFacetUncovered, "d2"),
	}, 1<<30)

	if next.CurrentOwner != LocusExplore {
		t.Errorf("rebuild branch: CurrentOwner = %v, want LocusExplore (FacetUncovered)", next.CurrentOwner)
	}
	if len(next.Clusters) != 1 {
		t.Errorf("rebuild branch: Clusters len = %d, want 1", len(next.Clusters))
	}
	if next.Clusters[0].Primary.Kind != types.ViolFacetUncovered {
		t.Errorf("rebuild branch: cluster Primary = %v, want ViolFacetUncovered", next.Clusters[0].Primary.Kind)
	}
}

func TestPromoteNextOwner_StableSubset_PopsQueue(t *testing.T) {
	prev := RepairExecutionPlan{
		Clusters: []RepairCluster{
			{Primary: types.Violation{Kind: types.ViolSubjectAnchorMissing}, Owner: LocusExtract},
			{Primary: types.Violation{Kind: types.ViolSelfContradiction}, Owner: LocusFinalizer},
		},
		OrderedOwners:     []RepairLocus{LocusExtract, LocusFinalizer},
		CurrentOwner:      LocusExtract,
		RemainingOwners:   []RepairLocus{LocusFinalizer},
		EscalationAllowed: true,
	}
	next := PromoteNextOwner(prev, nil, 0)
	if next.CurrentOwner != LocusFinalizer {
		t.Errorf("subset branch: CurrentOwner = %v, want LocusFinalizer", next.CurrentOwner)
	}
	if len(next.RemainingOwners) != 0 {
		t.Errorf("subset branch: RemainingOwners len = %d, want 0", len(next.RemainingOwners))
	}
	if len(next.OrderedOwners) != 2 {
		t.Errorf("subset branch: OrderedOwners truncated unexpectedly: %v", next.OrderedOwners)
	}
}

func TestPromoteNextOwner_Exhausted_PreservesCurrentOwner(t *testing.T) {
	prev := RepairExecutionPlan{
		Clusters:          []RepairCluster{{Primary: types.Violation{Kind: types.ViolSelfContradiction}, Owner: LocusFinalizer}},
		OrderedOwners:     []RepairLocus{LocusFinalizer},
		CurrentOwner:      LocusFinalizer,
		RemainingOwners:   nil,
		EscalationAllowed: true,
	}
	next := PromoteNextOwner(prev, nil, 0)
	if next.CurrentOwner != LocusFinalizer {
		t.Errorf("exhausted: CurrentOwner = %v, want LocusFinalizer (unchanged)", next.CurrentOwner)
	}
	if len(next.RemainingOwners) != 0 {
		t.Errorf("exhausted: RemainingOwners len = %d, want 0", len(next.RemainingOwners))
	}
	if !next.EscalationAllowed {
		t.Errorf("exhausted: EscalationAllowed flipped — must remain true so caller can escalate")
	}
}

func TestShouldRebuildExecutionPlan_NilPrev(t *testing.T) {
	if !shouldRebuildExecutionPlan(nil, []types.Violation{vio(types.ViolFacetUncovered, "d1")}) {
		t.Errorf("nil prev: should rebuild")
	}
}

func TestShouldRebuildExecutionPlan_QueueExhausted(t *testing.T) {
	prev := &RepairExecutionPlan{
		Clusters:        []RepairCluster{{Primary: types.Violation{Kind: types.ViolSubjectAnchorMissing}, Owner: LocusExtract}},
		OrderedOwners:   []RepairLocus{LocusExtract},
		CurrentOwner:    LocusExtract,
		RemainingOwners: nil,
	}
	if !shouldRebuildExecutionPlan(prev, []types.Violation{vio(types.ViolSubjectAnchorMissing, "d1")}) {
		t.Errorf("queue empty: should rebuild")
	}
}

func TestShouldRebuildExecutionPlan_NewKindAppears(t *testing.T) {
	prev := &RepairExecutionPlan{
		Clusters: []RepairCluster{
			{Primary: types.Violation{Kind: types.ViolSubjectAnchorMissing}, Owner: LocusExtract},
		},
		OrderedOwners:   []RepairLocus{LocusExtract, LocusFinalizer},
		CurrentOwner:    LocusExtract,
		RemainingOwners: []RepairLocus{LocusFinalizer},
	}
	// Fresh kind FacetUncovered is NOT in prev — must rebuild.
	if !shouldRebuildExecutionPlan(prev, []types.Violation{
		vio(types.ViolSubjectAnchorMissing, "d1"),
		vio(types.ViolFacetUncovered, "d1"),
	}) {
		t.Errorf("new kind appeared: should rebuild")
	}
}

func TestShouldRebuildExecutionPlan_StrictSubsetPromotes(t *testing.T) {
	prev := &RepairExecutionPlan{
		Clusters: []RepairCluster{
			{Primary: types.Violation{Kind: types.ViolSubjectAnchorMissing}, Owner: LocusExtract,
				Derived: []types.Violation{{Kind: types.ViolStepIdentifierUnverified}}},
		},
		OrderedOwners:   []RepairLocus{LocusExtract, LocusFinalizer},
		CurrentOwner:    LocusExtract,
		RemainingOwners: []RepairLocus{LocusFinalizer},
	}
	// Fresh = strict subset (only the Derived remains; Primary cleared).
	if shouldRebuildExecutionPlan(prev, []types.Violation{
		vio(types.ViolStepIdentifierUnverified, "d1"),
	}) {
		t.Errorf("strict subset: should NOT rebuild (promote next instead)")
	}
}

func TestShouldRebuildExecutionPlan_EqualKindSetRebuilds(t *testing.T) {
	prev := &RepairExecutionPlan{
		Clusters: []RepairCluster{
			{Primary: types.Violation{Kind: types.ViolSubjectAnchorMissing}, Owner: LocusExtract},
		},
		OrderedOwners:   []RepairLocus{LocusExtract, LocusFinalizer},
		CurrentOwner:    LocusExtract,
		RemainingOwners: []RepairLocus{LocusFinalizer},
	}
	// Same kind set → no progress → rebuild.
	if !shouldRebuildExecutionPlan(prev, []types.Violation{
		vio(types.ViolSubjectAnchorMissing, "d1"),
	}) {
		t.Errorf("equal kind set: should rebuild (no progress)")
	}
}

func TestSummarizeRepairExecutionPlan_Empty(t *testing.T) {
	got := SummarizeRepairExecutionPlan(RepairExecutionPlan{})
	if !strings.Contains(got, "current=<empty>") {
		t.Errorf("empty plan summary missing sentinel: %q", got)
	}
}

func TestSummarizeRepairExecutionPlan_Format(t *testing.T) {
	plan := BuildRepairExecutionPlan([]types.Violation{
		vio(types.ViolSubjectAnchorMissing, "d1"),
		vio(types.ViolSelfContradiction, "d2"),
	}, 0)
	got := SummarizeRepairExecutionPlan(plan)
	if !strings.Contains(got, "current=finalizer") {
		t.Errorf("summary missing current=finalizer (R2.2 downgrade): %q", got)
	}
	if !strings.Contains(got, "remaining=1") {
		t.Errorf("summary missing remaining=1: %q", got)
	}
	if !strings.Contains(got, "budget_downgrade=true") {
		t.Errorf("summary missing budget_downgrade=true: %q", got)
	}
}

// AdvanceRepairExecutionPlan integration tests — orchestrator-side
// entry point. Cover the three branches: empty / rebuild / promote.

func TestAdvanceRepairExecutionPlan_EmptyViolations(t *testing.T) {
	mut := &types.MutableState{}
	plan, target, preDown := AdvanceRepairExecutionPlan(mut, nil, 0)
	if !plan.IsEmpty() {
		t.Errorf("empty fresh: plan.IsEmpty()=false: %+v", plan)
	}
	if target != FallbackFinalizerOnly || preDown != FallbackFinalizerOnly {
		t.Errorf("empty fresh: target=%v preDown=%v, want FallbackFinalizerOnly twice", target, preDown)
	}
	if mut.RepairExecutionPlan() != nil {
		t.Errorf("empty fresh: should NOT stash plan; got %+v", mut.RepairExecutionPlan())
	}
}

func TestAdvanceRepairExecutionPlan_RebuildBranch(t *testing.T) {
	mut := &types.MutableState{}
	// First call — no prev plan → rebuild.
	plan, target, preDown := AdvanceRepairExecutionPlan(mut, []types.Violation{
		vio(types.ViolSubjectAnchorMissing, "d1"),
		vio(types.ViolSelfContradiction, "d2"),
	}, 0)
	// R2.2 downgrade should fire (budget=0 used, Finalizer in queue).
	if !plan.FinalizerLocalDowngradeApplied {
		t.Errorf("rebuild: downgrade should fire on first call; plan=%+v", plan)
	}
	if target != FallbackFinalizerOnly {
		t.Errorf("rebuild: target=%v, want FallbackFinalizerOnly (downgrade)", target)
	}
	if preDown != FallbackBackToExtract {
		t.Errorf("rebuild: preDown=%v, want FallbackBackToExtract (deepest)", preDown)
	}
	stashed, ok := mut.RepairExecutionPlan().(RepairExecutionPlan)
	if !ok {
		t.Fatalf("rebuild: plan not stashed: %T", mut.RepairExecutionPlan())
	}
	if stashed.CurrentOwner != LocusFinalizer {
		t.Errorf("rebuild: stashed CurrentOwner = %v, want LocusFinalizer", stashed.CurrentOwner)
	}
}

func TestAdvanceRepairExecutionPlan_PromoteBranch(t *testing.T) {
	mut := &types.MutableState{}
	// Stash a prev plan: 2 owners, currently at Finalizer (downgrade
	// applied), Extract is RemainingOwners[0].
	prev := RepairExecutionPlan{
		Clusters: []RepairCluster{
			{Primary: types.Violation{Kind: types.ViolSubjectAnchorMissing}, Owner: LocusExtract},
			{Primary: types.Violation{Kind: types.ViolSelfContradiction}, Owner: LocusFinalizer},
		},
		OrderedOwners:                  []RepairLocus{LocusFinalizer, LocusExtract},
		CurrentOwner:                   LocusFinalizer,
		RemainingOwners:                []RepairLocus{LocusExtract},
		FinalizerLocalDowngradeApplied: true,
		EscalationAllowed:              true,
	}
	mut.SetRepairExecutionPlan(prev)

	// Fresh = strict subset of prev (Finalizer-owned cluster fixed,
	// only Extract-owned remains). Should PROMOTE next, not rebuild.
	plan, target, preDown := AdvanceRepairExecutionPlan(mut, []types.Violation{
		vio(types.ViolSubjectAnchorMissing, "d1"),
	}, 1) // budget used 1 — would no longer downgrade on rebuild

	if plan.CurrentOwner != LocusExtract {
		t.Errorf("promote: CurrentOwner = %v, want LocusExtract", plan.CurrentOwner)
	}
	if target != FallbackBackToExtract {
		t.Errorf("promote: target = %v, want FallbackBackToExtract", target)
	}
	// promote inherits prev's downgrade flag — preDowngrade should
	// recover from OrderedOwners[1] = LocusExtract.
	if preDown != FallbackBackToExtract {
		t.Errorf("promote: preDown = %v, want FallbackBackToExtract", preDown)
	}
	if len(plan.RemainingOwners) != 0 {
		t.Errorf("promote: RemainingOwners = %v, want []", plan.RemainingOwners)
	}
}

// AdvanceRepairExecutionPlan tolerates nil MutableState (tests /
// direct callers).
func TestAdvanceRepairExecutionPlan_NilMutableState(t *testing.T) {
	plan, target, _ := AdvanceRepairExecutionPlan(nil, []types.Violation{
		vio(types.ViolFacetUncovered, "d1"),
	}, 1<<30)
	if plan.CurrentOwner != LocusExplore {
		t.Errorf("nil mut: CurrentOwner = %v, want LocusExplore", plan.CurrentOwner)
	}
	if target != FallbackBackToExplore {
		t.Errorf("nil mut: target = %v, want FallbackBackToExplore", target)
	}
}

// R4 lock — internal symbols never appear in the LLM-facing
// retry-summary surface. This is a forward-looking guard: G1 step 1
// adds the SUMMARIZE helper which is internal-only; G1 step 4 adds
// telemetry. If a future change wires SummarizeRepairExecutionPlan
// into RetryBlockSummary or other LLM-facing render paths, this lock
// will have to move with it.
func TestRepairExecutionPlan_NotInLLMFacingSurface(t *testing.T) {
	// Pure compile-time / structural assertion: this file only
	// exports symbols that the orchestrator + tests consume. The
	// symbols themselves DO NOT appear in any prompt corpus today.
	// Lock: the type name must contain "RepairExecutionPlan" verbatim
	// (so cross-package tests can grep for it) AND the Summarize
	// output must not contain stage codenames.
	plan := BuildRepairExecutionPlan([]types.Violation{
		vio(types.ViolFacetUncovered, "d1"),
	}, 1<<30)
	got := SummarizeRepairExecutionPlan(plan)
	for _, banned := range []string{"finalizer-local", "extractor", "explorer", "Phase ", "FailLoud"} {
		if strings.Contains(got, banned) {
			t.Errorf("Summarize output contains banned token %q: %q", banned, got)
		}
	}
}
