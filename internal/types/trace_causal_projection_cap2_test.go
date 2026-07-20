package types

import "testing"

// trace_causal_projection_cap2_test.go — CAP-2+THERM (§28.4/§28.5,
// real_trace_campaign_20260705.md, 2026-07-09) note→node parse pins: the
// typed cluster-topology tokens and the THERM press value ride the SAME
// presence gates as their CAP siblings (fold keys behind fold_basis, the
// gated key beside gated_capability); absence stays empty so every explicit-
// topology / legacy record keeps its wording byte-identically.

func cap2FoldRecord(notes ...string) ObservationRecord {
	base := []string{"type=running", "supply_fold_deficit_ms=0.186",
		"supply_fold_ideal_ms=2.455", "fold_basis=known=2.641ms,unknown=0.000ms",
		"fold_capability=default_table"}
	return ObservationRecord{
		ID: "cap2-1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "root_cause_context", Subject: "worker-9", Object: "running",
		RichNotes: append(base, notes...),
	}
}

func TestTraceCausalProjectionCAP2TopologyAndThermalParse(t *testing.T) {
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext,
		cap2FoldRecord("fold_cluster_topology=keyed_rail", "thermal_cap_khz=1850000"))
	if node.SupplyFoldTopologySource != "keyed_rail" {
		t.Fatalf("fold_cluster_topology must reach the node verbatim: %q", node.SupplyFoldTopologySource)
	}
	if node.ThermalCapKHz != 1850000 {
		t.Fatalf("thermal_cap_khz must reach the node: %d", node.ThermalCapKHz)
	}
	// Absence = explicit/legacy: no topology claim, no THERM sentence.
	bare := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, cap2FoldRecord())
	if bare.SupplyFoldTopologySource != "" || bare.ThermalCapKHz != 0 {
		t.Fatalf("absent notes must stay empty (byte-preserving legacy): %q/%d",
			bare.SupplyFoldTopologySource, bare.ThermalCapKHz)
	}
	// The fold keys ride the fold_basis presence gate: without the fold, a
	// stray topology/thermal note claims nothing.
	stray := ObservationRecord{
		ID: "cap2-2", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "root_cause_context", Subject: "worker-9", Object: "running",
		RichNotes: []string{"type=running", "fold_cluster_topology=keyed_rail", "thermal_cap_khz=1850000"},
	}
	if node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, stray); node.SupplyFoldComputed ||
		node.SupplyFoldTopologySource != "" || node.ThermalCapKHz != 0 {
		t.Fatalf("a topology/thermal claim without a fold is meaningless: %+v", node)
	}
}

func TestTraceCausalProjectionCAP2GatedTopologyParse(t *testing.T) {
	record := ObservationRecord{
		ID: "cap2-3", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "root_cause_context", Subject: "worker-9", Object: "priority_inversion_candidate",
		RichNotes: []string{"type=priority_inversion_candidate", "gated_runnable=20.713",
			"gated_running_deficit=16.697", "gated_capability=default_table",
			"gated_cluster_topology=freq_comovement"},
	}
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record)
	if node.GatedTopologySource != "freq_comovement" {
		t.Fatalf("gated_cluster_topology must reach the node verbatim: %q", node.GatedTopologySource)
	}
	record.RichNotes = record.RichNotes[:4]
	if node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record); node.GatedTopologySource != "" {
		t.Fatalf("absent gated_cluster_topology must stay empty: %q", node.GatedTopologySource)
	}
}

// DISPHYG-3 件7 (CLUSTER-FIX-2 D5 gated reason twin, 2026-07-20): the
// gated_capability_freq_only_reason note reaches the node verbatim; absence
// stays empty (byte-preserving legacy — every reason-less record keeps the
// ruled generic 簇结构不可判 wording downstream).
func TestTraceCausalProjectionDisphyg3GatedFreqOnlyReasonParse(t *testing.T) {
	record := ObservationRecord{
		ID: "dh3-7", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "root_cause_context", Subject: "worker-9", Object: "priority_inversion_candidate",
		RichNotes: []string{"type=priority_inversion_candidate", "gated_runnable=20.713",
			"gated_running_deficit=16.697", "gated_capability=freq_only",
			"gated_capability_freq_only_reason=single_cluster"},
	}
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record)
	if node.GatedCapabilityFreqOnlyReason != "single_cluster" {
		t.Fatalf("gated_capability_freq_only_reason must reach the node verbatim: %q", node.GatedCapabilityFreqOnlyReason)
	}
	record.RichNotes = record.RichNotes[:4]
	if node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record); node.GatedCapabilityFreqOnlyReason != "" {
		t.Fatalf("absent gated_capability_freq_only_reason must stay empty: %q", node.GatedCapabilityFreqOnlyReason)
	}
}
