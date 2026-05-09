package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// vio is a test helper to construct a Violation with explicit
// DispatchID. All test fixtures in this file use the same dispatch
// "d1" unless they specifically test cross-dispatch grouping.
func vio(kind types.ViolationKind, dispatch string) types.Violation {
	return types.Violation{Kind: kind, DispatchID: dispatch}
}

func TestBuildRepairPlan_Empty(t *testing.T) {
	plan := BuildRepairPlan(nil)
	if plan.PrimaryOwner != LocusFinalizer {
		t.Errorf("empty input: PrimaryOwner = %v, want LocusFinalizer", plan.PrimaryOwner)
	}
	if len(plan.Clusters) != 0 {
		t.Errorf("empty input: Clusters = %v, want []", plan.Clusters)
	}
	if plan.HasFailLoud {
		t.Errorf("empty input: HasFailLoud = true, want false")
	}
}

// Rule C1 — SubjectAnchorMissing peels StepIdentifierUnverified +
// SymbolAnchorMismatch as Derived. Primary owner = Extract (the
// FallbackTargetForKind for ViolSubjectAnchorMissing).
func TestBuildRepairPlan_SubjectAnchorClustering(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolSubjectAnchorMissing, "d1"),
		vio(types.ViolStepIdentifierUnverified, "d1"),
		vio(types.ViolSymbolAnchorMismatch, "d1"),
	})

	if len(plan.Clusters) != 1 {
		t.Fatalf("got %d clusters, want 1\n%+v", len(plan.Clusters), plan.Clusters)
	}
	c := plan.Clusters[0]
	if c.Primary.Kind != types.ViolSubjectAnchorMissing {
		t.Errorf("Primary kind = %v, want %v", c.Primary.Kind, types.ViolSubjectAnchorMissing)
	}
	if len(c.Derived) != 2 {
		t.Errorf("Derived len = %d, want 2", len(c.Derived))
	}
	if c.Owner != LocusExtract {
		t.Errorf("Owner = %v, want LocusExtract", c.Owner)
	}
	if plan.PrimaryOwner != LocusExtract {
		t.Errorf("PrimaryOwner = %v, want LocusExtract", plan.PrimaryOwner)
	}
}

// Rule C2 — BlockCoverageMissing peels PrincipalClaimUseMissing +
// DiagramEdgeUnsupported. Primary owner = Extract (default).
func TestBuildRepairPlan_BlockCoverageClustering(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolBlockCoverageMissing, "d1"),
		vio(types.ViolPrincipalClaimUseMissing, "d1"),
		vio(types.ViolDiagramEdgeUnsupported, "d1"),
	})

	if len(plan.Clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(plan.Clusters))
	}
	c := plan.Clusters[0]
	if c.Primary.Kind != types.ViolBlockCoverageMissing {
		t.Errorf("Primary kind = %v, want %v", c.Primary.Kind, types.ViolBlockCoverageMissing)
	}
	if len(c.Derived) != 2 {
		t.Errorf("Derived len = %d, want 2", len(c.Derived))
	}
}

// Rule C3 — FacetUncovered peels PrincipalClaimUseMissing as Derived.
// Owner = Explore (FacetUncovered's default fallback).
func TestBuildRepairPlan_FacetUncoveredClustering(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolFacetUncovered, "d1"),
		vio(types.ViolPrincipalClaimUseMissing, "d1"),
	})

	if len(plan.Clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(plan.Clusters))
	}
	if plan.Clusters[0].Owner != LocusExplore {
		t.Errorf("Owner = %v, want LocusExplore", plan.Clusters[0].Owner)
	}
	if plan.PrimaryOwner != LocusExplore {
		t.Errorf("PrimaryOwner = %v, want LocusExplore", plan.PrimaryOwner)
	}
}

// Phase 1-C (V2 runtime eval followup, 2026-05-04): C3 rule's
// derived list expanded so RichnessRegression + DiagramEdgeUnsupported
// also peel into the FacetUncovered cluster instead of ballooning
// cluster count via singleton fallback. Pre-fix, the m1a-2 / u3a-1
// outlier traces showed 4 clusters where 1 was the real root.
func TestBuildRepairPlan_FacetUncoveredClustersAllDerived(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolFacetUncovered, "d1"),
		vio(types.ViolPrincipalClaimUseMissing, "d1"),
		vio(types.ViolRichnessRegression, "d1"),
		vio(types.ViolDiagramEdgeUnsupported, "d1"),
	})
	if len(plan.Clusters) != 1 {
		t.Fatalf("Phase 1-C: 4-violation set should peel to 1 cluster (FacetUncovered + 3 derived); got %d (%+v)",
			len(plan.Clusters), plan.Clusters)
	}
	c := plan.Clusters[0]
	if c.Primary.Kind != types.ViolFacetUncovered {
		t.Errorf("cluster Primary = %q, want ViolFacetUncovered", c.Primary.Kind)
	}
	if len(c.Derived) != 3 {
		t.Errorf("cluster Derived count = %d, want 3 (PrincipalClaim + RichnessRegression + DiagramEdge)", len(c.Derived))
	}
}

// Multiple independent root causes: each becomes its own cluster.
// PrimaryOwner = deepest among them.
//
// Setup: ChainDemoted (Explore) + DiagramIdentifier (Finalizer).
// Expected: 2 clusters; PrimaryOwner = Explore (deeper than Finalizer).
//
// (Note: ChainDemoted has Rule C7 with StepIdentifierUnverified +
// PrincipalClaimUseMissing as Derived — neither is in this fixture,
// so ChainDemoted is its own singleton cluster, NOT a Primary with
// peeled Derived.)
func TestBuildRepairPlan_MultipleIndependentClusters(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolChainDemoted, "d1"),
		vio(types.ViolDiagramIdentifier, "d1"),
	})

	if len(plan.Clusters) != 2 {
		t.Fatalf("got %d clusters, want 2 (independent)", len(plan.Clusters))
	}
	if plan.PrimaryOwner != LocusExplore {
		t.Errorf("PrimaryOwner = %v, want LocusExplore (deepest among independent clusters)", plan.PrimaryOwner)
	}
	// Order: deepest first.
	if plan.Clusters[0].Owner != LocusExplore {
		t.Errorf("Clusters[0].Owner = %v, want LocusExplore (sorted deepest first)", plan.Clusters[0].Owner)
	}
}

// SOFT-Terminal cluster does NOT force PrimaryOwner=Terminal —
// Phase 1-C SOFT-immune semantic (V2 runtime eval followup,
// 2026-05-04). Pre-fix, the FailLoud safety-net mapping for
// SOFT-default kinds (e.g. ViolDemotionStorm SeveritySoft → mapped
// to FallbackFailLoud as a safety net for operator-promoted
// strict mode) was leaking into BuildRepairPlan and forcing the
// entire plan to fail_loud whenever the SOFT violation appeared.
// The m1a-2 / u3a-1 outlier traces were the real-eval symptom.
//
// New semantic: SOFT-Terminal clusters stay in clusters[] for
// telemetry but do NOT trigger HasFailLoud; operators who want
// fail_loud on a normally-SOFT kind must promote it via
// pipeline_contract_strict_kinds yaml first.
func TestBuildRepairPlan_SoftFailLoudKindDoesNotForceTerminal(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolDemotionStorm, "d1"), // SOFT FailLoud-mapped kind
		vio(types.ViolFacetUncovered, "d1"),
	})
	if plan.HasFailLoud {
		t.Errorf("SOFT FailLoud kind MUST NOT trigger HasFailLoud (Phase 1-C); got true")
	}
	if plan.PrimaryOwner == LocusTerminal {
		t.Errorf("SOFT FailLoud kind MUST NOT force PrimaryOwner=Terminal (Phase 1-C); got %v", plan.PrimaryOwner)
	}
}

// Operator-promoted SOFT kind via extraSoft → extraStrict path
// removes the kind from the soft override map; downstream
// hasOverride=false → falls back to intrinsic Severity Soft.
// Intentional design choice: SOFT-by-default kinds with
// SeveritySoft are PERMANENTLY soft — operator-yaml cannot bump
// them to strict (the safety-net FailLoud mapping in
// fallback_policy.go is documentation of intent, not an active
// gate). Test covers the operator-add-to-soft path which IS
// honoured.
//
// (A real operator-promote-to-strict on a SeveritySoft kind would
// require redesigning ViolationProfileFor's isStrict argument
// interplay, which is a separate change beyond Phase 1-C scope.)
func TestBuildRepairPlan_OperatorAddedSoftKindStillSkipsFailLoud(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	// Operator marks ViolFacetUncovered explicitly soft; combined
	// with a normally-SOFT FailLoud-mapped kind, plan stays
	// non-terminal.
	SetSoftViolationKinds([]string{string(types.ViolFacetUncovered)}, nil)
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolDemotionStorm, "d1"),
		vio(types.ViolFacetUncovered, "d1"),
	})
	if plan.HasFailLoud {
		t.Errorf("all-SOFT plan MUST NOT trigger HasFailLoud; got true")
	}
}

// Cross-dispatch isolation: violations from different DispatchIDs do
// NOT cluster together even if they match a cooccurrence rule pattern.
// SubjectAnchorMissing in d1 + StepIdentifierUnverified in d2 should
// produce 2 singleton clusters, not 1 paired cluster.
func TestBuildRepairPlan_CrossDispatchDoesNotCluster(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolSubjectAnchorMissing, "d1"),
		vio(types.ViolStepIdentifierUnverified, "d2"),
	})

	if len(plan.Clusters) != 2 {
		t.Fatalf("got %d clusters, want 2 (different DispatchIDs)", len(plan.Clusters))
	}
	for _, c := range plan.Clusters {
		if len(c.Derived) != 0 {
			t.Errorf("cluster Primary=%v has Derived=%v, want empty (cross-dispatch should not pair)",
				c.Primary.Kind, c.Derived)
		}
	}
}

// Singleton cluster: a violation kind that doesn't match any rule
// (or whose rule-derived peers aren't in the input) becomes its own
// Primary. Reason should indicate no rule applied.
func TestBuildRepairPlan_SingletonCluster(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		// ViolSelfContradiction is not in any rule's Primary or Derived
		// set, so it must be a singleton.
		vio(types.ViolSelfContradiction, "d1"),
	})

	if len(plan.Clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(plan.Clusters))
	}
	c := plan.Clusters[0]
	if len(c.Derived) != 0 {
		t.Errorf("singleton has Derived=%v, want empty", c.Derived)
	}
	if c.Reason == "" {
		t.Errorf("singleton Reason is empty; want '<no cooccurrence rule applied — independent root cause>'")
	}
}

// SummarizeRepairPlan format guard — telemetry consumers parse this
// string, so changes need a deliberate audit.
func TestSummarizeRepairPlan_Format(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolSubjectAnchorMissing, "d1"),
		vio(types.ViolStepIdentifierUnverified, "d1"),
	})
	got := SummarizeRepairPlan(plan)
	want := "primary=extract clusters=1 kinds=[subject_anchor_missing]"
	if got != want {
		t.Errorf("Summarize:\n got: %q\nwant: %q", got, want)
	}
}

// Rule precedence: when a violation could be Derived of two different
// Primaries, the rule earlier in defaultCooccurrenceRules wins.
//
// Setup: ChainDemoted (C7) + Citation (C5) both list
// StepIdentifierUnverified as Derived. C5 (Citation) appears earlier
// in defaultCooccurrenceRules, so when both Primaries are present
// the StepIdentifierUnverified should peel into Citation's cluster.
//
// Wait: actually re-reading the rule list, C5 is declared BEFORE C7
// (Citation rules come before ChainDemoted rule). matchRule iterates
// rules in order and returns the first match — so when StepIdentifierUnverified
// + ChainDemoted + Citation are all present, Citation's cluster will
// be built first, and ChainDemoted will form a singleton (since its
// Derived peers were already consumed).
func TestBuildRepairPlan_RulePrecedence(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolCitation, "d1"),
		vio(types.ViolChainDemoted, "d1"),
		vio(types.ViolStepIdentifierUnverified, "d1"),
	})

	if len(plan.Clusters) != 2 {
		t.Fatalf("got %d clusters, want 2 (Citation cluster + ChainDemoted singleton)", len(plan.Clusters))
	}
	// Find the Citation cluster — it should have Derived.
	var citationCluster *RepairCluster
	for i := range plan.Clusters {
		if plan.Clusters[i].Primary.Kind == types.ViolCitation {
			citationCluster = &plan.Clusters[i]
			break
		}
	}
	if citationCluster == nil {
		t.Fatalf("Citation cluster not found in clusters: %+v", plan.Clusters)
	}
	if len(citationCluster.Derived) != 1 || citationCluster.Derived[0].Kind != types.ViolStepIdentifierUnverified {
		t.Errorf("Citation cluster Derived = %+v, want [ViolStepIdentifierUnverified]", citationCluster.Derived)
	}
}

// TestEveryHardDefaultViolKindHasCooccurrenceCoverage pins Phase 1-C
// (V2 runtime eval followup, 2026-05-04) checklist for new
// ViolationKind on-line: every HARD-default kind MUST appear in
// at least one cooccurrence rule (as Primary or Derived) UNLESS
// it is explicitly listed in `intentionalSingletons` below.
//
// Why this matters: pre-Phase-1-C, Phase 3-C5 + Phase 5-E1 added
// ViolDiagramEdgeUnsupported / ViolFacetUncovered without
// updating defaultCooccurrenceRules. New violations went singleton,
// inflating cluster count to 4 in real-eval m1a-2 / u3a-1, which
// dragged RepairPlan toward fail_loud through the SOFT-Terminal
// safety-net leak. Lock so future kinds can't repeat the omission.
//
// Intentional singletons: kinds whose semantics are genuinely
// standalone (e.g. structural-baseline checks with no co-firing
// peer). Add a kind here ONLY when you can document why it has no
// cooccurring partner. The list is intentionally short so adding
// to it forces a discussion.
func TestEveryHardDefaultViolKindHasCooccurrenceCoverage(t *testing.T) {
	covered := make(map[types.ViolationKind]bool)
	for _, rule := range defaultCooccurrenceRules {
		covered[rule.Primary] = true
		for _, d := range rule.Derived {
			covered[d] = true
		}
	}

	// Kinds documented as genuinely standalone — must include
	// rationale in adjacent godoc.
	intentionalSingletons := map[types.ViolationKind]string{
		types.ViolMustInclude:                     "answer-shape baseline; fires alone when must-include list violated",
		types.ViolMustExclude:                     "answer-shape baseline; fires alone when must-exclude list violated",
		types.ViolAcceptance:                      "criterion-acceptance baseline; fires alone when acceptance test fails",
		types.ViolSuccessCriterion:                "scheduler success criterion; fires alone at validate-node level",
		types.ViolLiteralFormFailed:               "literal-form-validator baseline; standalone schema check",
		types.ViolSelfRefLiteral:                  "self-reference literal grounder; fires alone when literal grounds to itself",
		types.ViolDiagramIdentifier:               "deprecated V1 diagram bare-identifier check; standalone",
		types.ViolPreCompleteDowngrade:            "pre-complete soft downgrade marker; fires alone when downgrade applied",
		types.ViolViewSwap:                        "view-swap retry hint marker; fires alone when subject/view mismatch detected at pre-complete",
		types.ViolFamilyMismatch:                  "family-mismatch baseline; fires alone when QFamily does not match family contract",
		types.ViolExternalArtifactUnderdecoded:    "external-artifact decode floor; fires alone on under-decode of attached log/perf trace",
		types.ViolValueSecondaryCitationOffFocus:  "scalar-value secondary citation gate; standalone",
		types.ViolCrossCitationConflict:           "cross-citation single-locus oracle; standalone",
		types.ViolStructuralEnumerationDivergence: "code-vs-comment enumeration divergence; standalone caveat marker",
		types.ViolSelfContradiction:               "reviewer-detected SUMMARY-vs-BODY contradiction; LLM-text comparator with no structural co-fire",
		types.ViolAbsenceScopeExceeded:            "extractor framed absence too broadly; standalone semantic boundary check",
		types.ViolMissingRequestedRoleUndisclosed: "typed missing-layer disclosure is a finalize-local standalone repair",
		// B2 v3 (2026-05-04) — three-layer quality contract violations
		// fire as independent finalize-locus signals; the LLM either
		// re-emits with the missing facet declared (richness gap) or
		// adds inline-code anchors (prose underfilled). No upstream
		// cooccurrence — they're orthogonal to V1 oracle clusters.
		types.ViolRichnessGlaringGap:        "v3 B2 richness escalation; finalizer re-emits to surface missing optional facet",
		types.ViolPrincipalProseUnderfilled: "v3 B2 prose-density escalation; finalizer re-emits with inline anchors",
		// Lane → block-kind compliance is a typed-signal finalize-
		// local repair: the principal block kind is wrong for the
		// lane its citations resolve to. Repair is "re-render the
		// same content under a different block kind" — strictly
		// finalizer-side, no upstream stage participates and no
		// other oracle reports the same condition.
		types.ViolLaneBlockKindMismatch: "lane → block-kind alignment is a finalizer-local re-render; standalone with no upstream cooccurrence",
		// Phase 2.B Tier 2 ERM completeness violations (2026-05-09).
		// Each dimension is a structurally-independent answer-coverage
		// gap: a ScalarCount problem (visual count vs deterministic
		// tool) does NOT imply a PathDepth problem (chain too short)
		// or vice versa. They fire on disjoint question shapes
		// (count question / call-chain / declared-count enumeration /
		// comparison) so cooccurrence rules would never apply. The
		// repair path is per-dimension and orthogonal — explore for
		// missing tool/function evidence, extract for slate
		// re-emission. Standalone by design; documented in
		// docs/design/commercial_grade_3_pattern_remediation.md
		// Phase 2.B.
		types.ViolScalarCountUnsourced:   "Tier 2 scalar-count completeness; fires alone on count questions lacking deterministic-tool output",
		types.ViolPathDepthInsufficient:  "Tier 2 call-chain depth completeness; fires alone when distinct function mentions fall short of entry+exit+mid",
		types.ViolCardinalityShort:       "Tier 2 declared-count cardinality completeness; fires alone when answer items < EnumerationBoundary.DeclaredCount",
		types.ViolEntityParityImbalanced: "Tier 2 comparison sampling-parity completeness; fires alone on lopsided per-bucket evidence",
	}

	for _, kind := range types.AllViolationKinds() {
		if isSoftViolationKind(kind) {
			continue // SOFT-default kinds are telemetry; cluster coverage optional
		}
		if covered[kind] {
			continue
		}
		if reason, ok := intentionalSingletons[kind]; ok {
			_ = reason
			continue
		}
		t.Errorf("Phase 1-C checklist: HARD-default ViolationKind %q has no cooccurrence rule and is not in intentionalSingletons. Add a rule in repair_cooccurrence.go OR document standalone semantics by adding the kind to intentionalSingletons here.", kind)
	}
}
