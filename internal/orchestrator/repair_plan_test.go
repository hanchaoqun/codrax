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

// FailLoud violation in input forces PrimaryOwner = Terminal regardless
// of whether deeper clusters exist. Tests the failure-of-last-resort
// red line (mirrors FallbackTargetForViolations behaviour).
func TestBuildRepairPlan_FailLoudOverridesAll(t *testing.T) {
	plan := BuildRepairPlan([]types.Violation{
		vio(types.ViolDemotionStorm, "d1"), // FailLoud kind
		vio(types.ViolFacetUncovered, "d1"),
	})

	if !plan.HasFailLoud {
		t.Errorf("HasFailLoud = false, want true")
	}
	if plan.PrimaryOwner != LocusTerminal {
		t.Errorf("PrimaryOwner = %v, want LocusTerminal", plan.PrimaryOwner)
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
