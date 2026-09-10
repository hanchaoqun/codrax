package types

import (
	"reflect"
	"testing"
)

func b1638b3RestrictionRecord(id string, value float64, lineStart, lineEnd int) ObservationRecord {
	r := aggregateTestRecord(id, "root_cause_context", "root_cause_context:"+id,
		"worker-42", "running", "5.000", value, lineStart, lineEnd,
		"chain_relevance=adjacent", "dominant_state=running", "selected_window=1.000000..2.000000")
	r.SourceRef = ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "/capture/original.trace",
		PayloadRef: "/results/" + id + ".json", QueryScopeID: "query-" + id}
	r.ObservedAt = "2026-09-10T01:00:00Z"
	r.MeasurementSources = TraceSchedulerMeasurementSourcesFromDomain(&TraceSchedulerMeasurementDomain{
		Version: 1, Status: "constructed_partition", Method: "thread_timeline", TargetTID: 42,
		WindowStartTs: 1, WindowEndTs: 2, PartitionID: "native-" + id,
	})
	return r
}

func b1638b3RestrictionNode(id string, filtered bool) TraceCausalProjectionNode {
	r := b1638b3RestrictionRecord(id, 5, 100, 110)
	if filtered {
		r.MeasurementSources.Domains[0].QueryLineStart = 1
		r.MeasurementSources.Domains[0].QueryLineEnd = 400
	}
	return traceCausalProjectionNodeFromRecord("adjacent_cause", r)
}

func TestB1638B3R1RestrictionCandidatesPreserveSameScopeAcrossOrder(t *testing.T) {
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}, {1, 0, 2}, {0, 2, 1}} {
		nodes := []TraceCausalProjectionNode{b1638b3RestrictionNode("A0", false), b1638b3RestrictionNode("B1", true), b1638b3RestrictionNode("C1", true)}
		projection := TraceCausalProjection{}
		for _, i := range order {
			projection.AdjacentCauses = append(projection.AdjacentCauses, nodes[i])
		}
		traceCausalProjectionMergeSameFacts(&projection)
		if len(projection.AdjacentCauses) != 2 {
			t.Fatalf("different restrictions need two facts, not one lost or three unmerged (order=%v): %+v", order, projection.AdjacentCauses)
		}
		for _, row := range projection.AdjacentCauses {
			if row.ImpactMS != 5 || row.MergedCount != 0 {
				t.Fatalf("R1 changed the old one-fact value/grammar: %+v", row)
			}
			if row.EvidenceID == "A0" {
				if len(row.MergedEvidenceIDs) != 0 {
					t.Fatal("unrestricted row absorbed a differently filtered fact")
				}
			} else if len(row.MergedEvidenceIDs) != 1 || row.MergedEvidenceIDs[0] == "A0" {
				t.Fatalf("same-filter cross-parent publications failed to converge: %+v", row)
			}
		}
	}
}

func TestB1638B3R1CandidateBackfillStateDoesNotCrossRestrictions(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		a, b := b1638b3RestrictionNode("A0", false), b1638b3RestrictionNode("B1", true)
		aDonor, aConflict := b1638b3RestrictionNode("A0-donor", false), b1638b3RestrictionNode("A0-conflict", false)
		bDonor := b1638b3RestrictionNode("B1-donor", true)
		for _, donor := range []*TraceCausalProjectionNode{&aDonor, &aConflict, &bDonor} {
			donor.SupplyFoldComputed, donor.SupplyFoldKnownMS, donor.SupplyFoldIdealMS = true, 5, 1
		}
		aDonor.SupplyFoldDeficitMS, aConflict.SupplyFoldDeficitMS, bDonor.SupplyFoldDeficitMS = 3, 4, 2
		rows := []TraceCausalProjectionNode{a, b, aDonor, aConflict, bDonor}
		if reverse {
			rows = []TraceCausalProjectionNode{b, a, bDonor, aDonor, aConflict}
		}
		projection := TraceCausalProjection{AdjacentCauses: rows}
		traceCausalProjectionMergeSameFacts(&projection)
		if len(projection.AdjacentCauses) != 2 {
			t.Fatalf("two restriction-specific survivors required: %+v", projection.AdjacentCauses)
		}
		for _, row := range projection.AdjacentCauses {
			if row.EvidenceID == "A0" && row.SupplyFoldComputed {
				t.Fatal("its conflicting donors must retain the existing clear-and-poison rule")
			}
			if row.EvidenceID == "B1" && (!row.SupplyFoldComputed || row.SupplyFoldDeficitMS != 2) {
				t.Fatal("another restriction's donor conflict poisoned this survivor")
			}
		}
	}
}

func TestB1638B3R1UnknownAbsorbCannotBridgeKnownRestrictionConflict(t *testing.T) {
	for _, firstUnknown := range []bool{false, true} {
		a, b := b1638b3RestrictionNode("A0", false), b1638b3RestrictionNode("B1", true)
		unknown := b1638b3RestrictionNode("unknown", false)
		unknown.MeasurementOrigins = []TraceSchedulerMeasurementOrigin{{}}
		rows := []TraceCausalProjectionNode{a, unknown, b}
		if firstUnknown {
			rows = []TraceCausalProjectionNode{unknown, a, b}
		}
		projection := TraceCausalProjection{AdjacentCauses: rows}
		traceCausalProjectionMergeSameFacts(&projection)
		if len(projection.AdjacentCauses) != 2 {
			t.Fatalf("unknown must not hide an already-bound conflicting filter: %+v", projection.AdjacentCauses)
		}
		var mixed *TraceCausalProjectionNode
		for i := range projection.AdjacentCauses {
			if projection.AdjacentCauses[i].EvidenceID != "B1" {
				mixed = &projection.AdjacentCauses[i]
			}
		}
		if mixed == nil || len(mixed.MeasurementOrigins) != 2 {
			t.Fatal("the prior legacy absorb must retain known and unknown origins")
		}
		if _, groupable := TraceSchedulerMeasurementRestrictionKey(mixed.MeasurementOrigins); groupable {
			t.Fatal("mixed provenance cannot borrow its known member's group qualification")
		}
	}
}

func TestB1638B3V4RestrictionConflictPreservesExactAndNearValueRules(t *testing.T) {
	for _, value := range []float64{5, 5.1} {
		for _, conflict := range []bool{false, true} {
			a := b1638b3RestrictionNode("A", false)
			b := b1638b3RestrictionNode("B", conflict)
			b.ImpactMS, b.CumulativeImpactMS = value, value
			b.LineStart, b.LineEnd = 102, 115 // Force V4 rather than R1.
			before := []TraceCausalProjectionNode{a, b}
			rows := traceCausalProjectionDedupDuplicatePublications(append([]TraceCausalProjectionNode(nil), before...))
			if conflict {
				if !reflect.DeepEqual(rows, before) {
					t.Fatalf("known different filters cannot be called duplicate publications: %+v", rows)
				}
				continue
			}
			if len(rows) != 1 || rows[0].ImpactMS != value || rows[0].CumulativeImpactMS != value ||
				rows[0].DuplicatePublications != 2 || rows[0].MergedCount != 0 {
				t.Fatalf("same-filter cross-parent publications must retain exact/MAX-not-SUM semantics: %+v", rows)
			}
		}
	}
}

func TestB1638B3PublicProjectionRetainsConflictingR1Facts(t *testing.T) {
	a := b1638b3RestrictionRecord("A", 5, 100, 110)
	b := b1638b3RestrictionRecord("B", 5, 100, 110)
	b.MeasurementSources.Domains[0].QueryLineStart, b.MeasurementSources.Domains[0].QueryLineEnd = 1, 400
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{a, b})
	if len(projection.AdjacentCauses) != 2 {
		t.Fatalf("public compiler must preserve both source-qualified facts: %+v", projection.AdjacentCauses)
	}
}

func TestB1638B3ExactStateAccountStillConvergesAcrossPublicationRestrictions(t *testing.T) {
	const key = "state_account:v2:complete-physical-inventory"
	rank := stateAccountOneSeatNode("rank", "root_cause_primary", key, 1, 100, 110)
	impact := stateAccountOneSeatNode("impact", "wakeup_causal_impact", key, 0, 99, 110)
	rank.MeasurementOrigins = b1638b3RestrictionNode("rank-origin", false).MeasurementOrigins
	impact.MeasurementOrigins = b1638b3RestrictionNode("impact-origin", true).MeasurementOrigins
	if !TraceSchedulerMeasurementRestrictionsConflict(rank.MeasurementOrigins, impact.MeasurementOrigins) {
		t.Fatal("fixture must carry different publication filters, not just different parent IDs")
	}
	projection := TraceCausalProjection{PrimaryRootCauses: []TraceCausalProjectionNode{rank}, OnChainCauses: []TraceCausalProjectionNode{impact}}
	traceCausalProjectionAggregateForPresentation(&projection)
	count, seats := oneSeatSeatCount(projection, "app-20")
	if count != 1 || seats[0].EvidenceID != "rank" || seats[0].Rank != 1 || seats[0].ImpactMS != 5 {
		t.Fatalf("complete physical account identity must retain its independent unique-seat authority: %+v", seats)
	}
	if len(seats[0].MergedEvidenceIDs) != 1 || seats[0].MergedEvidenceIDs[0] != "impact" {
		t.Fatal("the cross-view account mirror lost its evidence")
	}
}
