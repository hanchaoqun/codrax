package types

// trace_causal_projection_supplyfold_join_test.go — SFD (§15.A display half,
// user q6 issue 1, real_trace_campaign ledger): the compile-side supply-fold
// twin join. ONE engine WakeupCausalImpact publishes two projection records —
// the causal-impact row carrying the typed fold_basis accounting and the
// root-cause running twin whose funnel never sets a basis — and the join
// re-uses the ALREADY-PUBLISHED accounting on the running twin keyed by the
// precise engine-published segment identity (canonical subject + exact line
// range). Everything here runs through the REAL compile chain
// (TraceCausalProjectionFromObservationRecords), never hand-built nodes
// (P0-A2 lesson).

import (
	"fmt"
	"testing"
)

// sfdJoinRecord builds one trace_query hard-grounded observation of the q6
// two-row shape. impact differs between the twins in the specimen (58.919 vs
// 37.409) so the R1 same-fact merge (subject+value+range identity) never
// collapses them — exactly the customer render where BOTH rows survived.
func sfdJoinRecord(id, predicate, claimKey, subject string, impact float64, lineStart, lineEnd int, notes ...string) ObservationRecord {
	return aggregateTestRecord(id, predicate, claimKey, subject, "running",
		fmt.Sprintf("%.3f", impact), impact, lineStart, lineEnd, notes...)
}

// sfdJoinQ6Records is the q6 specimen: the running twin (rank funnel,
// basis=nil → no fold notes) and its causal-impact sibling with the full
// typed fold accounting on the SAME subject + SAME line range :45689-79142.
func sfdJoinQ6Records(donorNotes ...string) []ObservationRecord {
	twin := sfdJoinRecord("E4", "root_cause_primary", "root_cause_primary:running",
		"#RxComputationT-16816", 58.919, 45689, 79142,
		"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=running", "priority_inversion_candidate=true",
		"effective_impact_ms=58.919")
	donor := sfdJoinRecord("E7", "wakeup_causal_impact", "wakeup_causal_impact:running",
		"#RxComputationT-16816", 37.409, 45689, 79142,
		append([]string{
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
			"dominant_state=running", "priority_inversion_candidate=true",
			"runnable=20.713", "gated_runnable=20.713", "gated_running_deficit=16.697",
			"effective_impact_ms=37.409",
		}, donorNotes...)...)
	return []ObservationRecord{twin, donor}
}

func sfdJoinQ6FoldNotes() []string {
	return []string{
		"supply_fold_deficit_ms=17.702", "supply_fold_ideal_ms=41.217",
		"fold_basis=known=58.919ms,unknown=0.000ms",
	}
}

func sfdJoinFindRunningTwin(t *testing.T, nodes []TraceCausalProjectionNode, evidenceID string) *TraceCausalProjectionNode {
	t.Helper()
	for i := range nodes {
		if nodes[i].EvidenceID == evidenceID {
			return &nodes[i]
		}
	}
	return nil
}

// Pin ①: the q6 two-row shape joins — the running twin takes the sibling's
// engine-published accounting verbatim (same basis, so every display surface
// derives the SAME deficit number), while its own attribution values stay
// untouched (改值是引擎裁定,不在本批).
func TestSupplyFoldTwinJoinQ6Shape(t *testing.T) {
	got := TraceCausalProjectionFromObservationRecords(sfdJoinQ6Records(sfdJoinQ6FoldNotes()...))
	twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4")
	if twin == nil {
		t.Fatalf("running twin missing from primaries: %+v", got.PrimaryRootCauses)
	}
	if !twin.SupplyFoldComputed {
		t.Fatalf("running twin must take the sibling's fold accounting: %+v", twin)
	}
	if twin.SupplyFoldDeficitMS != 17.702 || twin.SupplyFoldIdealMS != 41.217 ||
		twin.SupplyFoldKnownMS != 58.919 || twin.SupplyFoldUnknownMS != 0 {
		t.Fatalf("joined accounting must be the donor's verbatim: %+v", twin)
	}
	// The twin's own attribution values never change — the fold is a
	// supplementary caliber disclosure, not a re-attribution.
	if twin.ImpactMS != 58.919 || twin.EffectiveImpactMS != 58.919 {
		t.Fatalf("join must not touch the twin's own attribution values: %+v", twin)
	}
	// The twin's runnable/gated lanes stay its OWN (never copied — demand-side
	// data of the donor is not this row's fact).
	if twin.RunnableMS != 0 || twin.GatedRunnableMS != 0 || twin.GatedRunningDeficitMS != 0 {
		t.Fatalf("join must copy ONLY the SupplyFold group: %+v", twin)
	}
	// Every cross-bucket copy of the twin joins from the same donor map —
	// surfaces can never disagree.
	if onChain := sfdJoinFindRunningTwin(t, got.OnChainCauses, "E4"); onChain != nil {
		if !onChain.SupplyFoldComputed || onChain.SupplyFoldDeficitMS != 17.702 {
			t.Fatalf("on-chain copy must join identically: %+v", onChain)
		}
	}
	// The donor keeps its own accounting untouched.
	donor := sfdJoinFindRunningTwin(t, got.SupportingHops, "E7")
	if donor == nil {
		donor = sfdJoinFindRunningTwin(t, got.OnChainCauses, "E7")
	}
	if donor == nil {
		t.Fatalf("donor row must survive: %+v", got)
	}
	if !donor.SupplyFoldComputed || donor.SupplyFoldDeficitMS != 17.702 || donor.SupplyFoldIdealMS != 41.217 {
		t.Fatalf("donor accounting must stay untouched: %+v", donor)
	}
}

// Pin ②: the join key is precise — a different line range or a different
// subject is a different segment and never joins (fail-open keeps the bare
// value). Also: unknown-thread sentinel subjects are not an identity.
func TestSupplyFoldTwinJoinKeyPrecision(t *testing.T) {
	// Line range differs by one line → no join.
	records := sfdJoinQ6Records(sfdJoinQ6FoldNotes()...)
	records[1].Span.LineEnd = 79143
	got := TraceCausalProjectionFromObservationRecords(records)
	if twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4"); twin == nil || twin.SupplyFoldComputed {
		t.Fatalf("different line range must never join: %+v", twin)
	}

	// Subject differs → no join.
	records = sfdJoinQ6Records(sfdJoinQ6FoldNotes()...)
	records[1].Subject = "OtherThread-999"
	got = TraceCausalProjectionFromObservationRecords(records)
	if twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4"); twin == nil || twin.SupplyFoldComputed {
		t.Fatalf("different subject must never join: %+v", twin)
	}

	// Unknown-thread sentinel on both sides: same range, but the sentinel is
	// not an identity — no join.
	records = sfdJoinQ6Records(sfdJoinQ6FoldNotes()...)
	records[0].Subject = "unknown-thread"
	records[1].Subject = "unknown-thread"
	got = TraceCausalProjectionFromObservationRecords(records)
	for _, bucket := range [][]TraceCausalProjectionNode{got.PrimaryRootCauses, got.OnChainCauses, got.SupportingHops} {
		if twin := sfdJoinFindRunningTwin(t, bucket, "E4"); twin != nil && twin.SupplyFoldComputed {
			t.Fatalf("sentinel subject must never be a join identity: %+v", twin)
		}
	}
}

// Pin ②b: donors that disagree on the accounting under ONE key mark it
// conflicted — ambiguity never joins.
func TestSupplyFoldTwinJoinConflictingDonorsNeverJoin(t *testing.T) {
	records := sfdJoinQ6Records(sfdJoinQ6FoldNotes()...)
	second := sfdJoinRecord("E8", "wakeup_causal_impact", "wakeup_causal_impact:running2",
		"#RxComputationT-16816", 22.000, 45689, 79142,
		"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
		"dominant_state=running",
		"supply_fold_deficit_ms=9.000", "supply_fold_ideal_ms=49.919",
		"fold_basis=known=58.919ms,unknown=0.000ms")
	records = append(records, second)
	got := TraceCausalProjectionFromObservationRecords(records)
	if twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4"); twin == nil || twin.SupplyFoldComputed {
		t.Fatalf("conflicting donors must fail open to the bare value: %+v", twin)
	}
}

// Pin ②c: a donor/recipient pair that BOTH declare typed selected_windows
// disagreeing beyond the F-2 tolerance re-measured the segment in different
// windows — the fold describes the donor's clamping, never joined.
func TestSupplyFoldTwinJoinCrossWindowVeto(t *testing.T) {
	records := sfdJoinQ6Records(sfdJoinQ6FoldNotes()...)
	records[0].RichNotes = append(records[0].RichNotes, "selected_window=200.000000..203.000000")
	records[1].RichNotes = append(records[1].RichNotes, "selected_window=100.000000..103.000000")
	got := TraceCausalProjectionFromObservationRecords(records)
	if twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4"); twin == nil || twin.SupplyFoldComputed {
		t.Fatalf("cross-window twins must never join: %+v", twin)
	}

	// Same declared window → the join proceeds.
	records = sfdJoinQ6Records(sfdJoinQ6FoldNotes()...)
	records[0].RichNotes = append(records[0].RichNotes, "selected_window=100.000000..103.000000")
	records[1].RichNotes = append(records[1].RichNotes, "selected_window=100.000000..103.000000")
	got = TraceCausalProjectionFromObservationRecords(records)
	if twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4"); twin == nil || !twin.SupplyFoldComputed {
		t.Fatalf("same-window twins must join: %+v", twin)
	}
}

// Pin ②d (M2 表驱动): recipients are running-STATE rows only — the fold's
// deficit is defined over the segment's OWN running wall clock; a same-key
// row of ANY other state (or with no state semantics at all) never inherits.
func TestSupplyFoldTwinJoinRunningStateOnly(t *testing.T) {
	cases := []struct {
		name          string
		object        string
		dominantState string // "" removes the dominant_state note entirely
	}{
		{name: "runnable", object: "runnable", dominantState: "runnable"},
		{name: "io_wait", object: "io_wait", dominantState: "io_wait"},
		// No state semantics anywhere: a non-state cause Object and no
		// dominant_state note leave StateKind empty — empty is not running.
		{name: "empty_state", object: "compute_supply", dominantState: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			records := sfdJoinQ6Records(sfdJoinQ6FoldNotes()...)
			records[0].Object = tc.object
			notes := make([]string, 0, len(records[0].RichNotes))
			for _, note := range records[0].RichNotes {
				if note == "dominant_state=running" {
					if tc.dominantState == "" {
						continue
					}
					note = "dominant_state=" + tc.dominantState
				}
				notes = append(notes, note)
			}
			records[0].RichNotes = notes
			got := TraceCausalProjectionFromObservationRecords(records)
			if twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4"); twin == nil || twin.SupplyFoldComputed {
				t.Fatalf("%s recipient must never inherit a running fold: %+v", tc.name, twin)
			}
		})
	}
}

// Pin ④ (compile half): an AllKnown=false basis joins as-is — the honest
// unknown coverage travels with the group, so the display layer's 频点数据不全
// branch fires on the running twin exactly as on the donor.
func TestSupplyFoldTwinJoinUnknownBasis(t *testing.T) {
	got := TraceCausalProjectionFromObservationRecords(sfdJoinQ6Records(
		"supply_fold_deficit_ms=0.400", "supply_fold_ideal_ms=30.000",
		"fold_basis=known=30.400ms,unknown=28.519ms"))
	twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4")
	if twin == nil || !twin.SupplyFoldComputed {
		t.Fatalf("unknown-basis fold must still join (exclusion is information): %+v", twin)
	}
	if twin.SupplyFoldUnknownMS != 28.519 || twin.SupplyFoldKnownMS != 30.400 {
		t.Fatalf("unknown coverage must travel verbatim: %+v", twin)
	}
}

// Pin ③ (compile half): no sibling → no donors → the running twin stays
// byte-identical to the pre-join shape (SupplyFold group all zero).
func TestSupplyFoldTwinJoinNoSiblingStable(t *testing.T) {
	records := sfdJoinQ6Records(sfdJoinQ6FoldNotes()...)[:1]
	got := TraceCausalProjectionFromObservationRecords(records)
	twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4")
	if twin == nil {
		t.Fatalf("running twin missing: %+v", got.PrimaryRootCauses)
	}
	if twin.SupplyFoldComputed || twin.SupplyFoldDeficitMS != 0 || twin.SupplyFoldIdealMS != 0 ||
		twin.SupplyFoldKnownMS != 0 || twin.SupplyFoldUnknownMS != 0 {
		t.Fatalf("no sibling must leave the twin untouched: %+v", twin)
	}
}

// R1-merge lane of the same join (equal projected values → the twins collapse
// into ONE row before the post-aggregation pass could see the donor): the
// absorb back-fill carries the fold group onto the survivor.
func TestSupplyFoldAbsorbBackfillOnSameFactMerge(t *testing.T) {
	records := sfdJoinQ6Records(sfdJoinQ6FoldNotes()...)
	// Make the twins R1-identical (same subject + same %.3f value + same
	// range): the survivor is the primary twin, the causal-impact donor is
	// absorbed.
	records[1].RichNotes[0] = "impact_ms=58.919"
	records[1].RichNotes[1] = "cumulative_impact_ms=58.919"
	got := TraceCausalProjectionFromObservationRecords(records)
	twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4")
	if twin == nil {
		t.Fatalf("merged twin missing: %+v", got.PrimaryRootCauses)
	}
	if len(twin.MergedEvidenceIDs) != 1 || twin.MergedEvidenceIDs[0] != "E7" {
		t.Fatalf("donor must be absorbed into the survivor: %+v", twin)
	}
	if !twin.SupplyFoldComputed || twin.SupplyFoldDeficitMS != 17.702 || twin.SupplyFoldIdealMS != 41.217 ||
		twin.SupplyFoldKnownMS != 58.919 || twin.SupplyFoldUnknownMS != 0 {
		t.Fatalf("absorb must back-fill the published fold group: %+v", twin)
	}
}

// sfdJoinR1Twins builds the value-EQUAL R1 shape (same subject + same %.3f
// value + same range → the causal-impact donor is absorbed into the primary
// survivor before the join pass could see it): the absorb lane's guards are
// exercised here.
func sfdJoinR1Twins(foldNotes ...string) []ObservationRecord {
	records := sfdJoinQ6Records(foldNotes...)
	records[1].RichNotes[0] = "impact_ms=58.919"
	records[1].RichNotes[1] = "cumulative_impact_ms=58.919"
	return records
}

// SFD 复核 F1: the absorb lane mirrors the join lane's cross-window veto —
// R1 twins (identical %.3f value + range, reachable via overlapping
// window_sweep re-measurements) declaring DIFFERENT typed selected_windows
// never back-fill; same declared windows do.
func TestSupplyFoldAbsorbCrossWindowVeto(t *testing.T) {
	records := sfdJoinR1Twins(sfdJoinQ6FoldNotes()...)
	records[0].RichNotes = append(records[0].RichNotes, "selected_window=200.000000..203.000000")
	records[1].RichNotes = append(records[1].RichNotes, "selected_window=100.000000..103.000000")
	got := TraceCausalProjectionFromObservationRecords(records)
	twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4")
	if twin == nil || len(twin.MergedEvidenceIDs) != 1 {
		t.Fatalf("R1 merge must still happen (one fact re-measured): %+v", twin)
	}
	if twin.SupplyFoldComputed {
		t.Fatalf("cross-window R1 twins must never back-fill the fold: %+v", twin)
	}

	// Same declared window → the back-fill proceeds.
	records = sfdJoinR1Twins(sfdJoinQ6FoldNotes()...)
	records[0].RichNotes = append(records[0].RichNotes, "selected_window=100.000000..103.000000")
	records[1].RichNotes = append(records[1].RichNotes, "selected_window=100.000000..103.000000")
	got = TraceCausalProjectionFromObservationRecords(records)
	if twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4"); twin == nil || !twin.SupplyFoldComputed {
		t.Fatalf("same-window R1 twins must back-fill: %+v", twin)
	}
}

// SFD 复核 F4(a): a survivor that PUBLISHED its own basis keeps it — an
// absorbed view with a different accounting never overwrites it (the
// priority-scan doctrine of every typed slot).
func TestSupplyFoldAbsorbSurvivorOwnBasisWins(t *testing.T) {
	records := sfdJoinR1Twins(sfdJoinQ6FoldNotes()...)
	records[0].RichNotes = append(records[0].RichNotes,
		"supply_fold_deficit_ms=3.000", "supply_fold_ideal_ms=55.919",
		"fold_basis=known=58.919ms,unknown=0.000ms")
	got := TraceCausalProjectionFromObservationRecords(records)
	twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4")
	if twin == nil || !twin.SupplyFoldComputed {
		t.Fatalf("survivor's own basis must survive the merge: %+v", twin)
	}
	if twin.SupplyFoldDeficitMS != 3.000 || twin.SupplyFoldIdealMS != 55.919 {
		t.Fatalf("survivor's own accounting must never be overwritten by a loser: %+v", twin)
	}
}

// SFD 复核 F4(b): sequential absorbed views that DISAGREE on the accounting
// are ambiguity — the back-filled group clears and the key stays poisoned
// (fail-open to the bare value; never first-writer-wins, mirroring the join
// lane's donor-conflict rule). A third agreeing view arriving after the
// conflict must not resurrect it.
func TestSupplyFoldAbsorbConflictingLosersClear(t *testing.T) {
	records := sfdJoinR1Twins(sfdJoinQ6FoldNotes()...)
	second := sfdJoinRecord("E8", "wakeup_causal_impact", "wakeup_causal_impact:running2",
		"#RxComputationT-16816", 58.919, 45689, 79142,
		"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
		"dominant_state=running",
		"supply_fold_deficit_ms=9.000", "supply_fold_ideal_ms=49.919",
		"fold_basis=known=58.919ms,unknown=0.000ms")
	third := sfdJoinRecord("E9", "wakeup_causal_impact", "wakeup_causal_impact:running3",
		"#RxComputationT-16816", 58.919, 45689, 79142,
		append([]string{
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
			"dominant_state=running",
		}, sfdJoinQ6FoldNotes()...)...)
	records = append(records, second, third)
	got := TraceCausalProjectionFromObservationRecords(records)
	twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4")
	if twin == nil || len(twin.MergedEvidenceIDs) != 3 {
		t.Fatalf("all three views must merge into one fact: %+v", twin)
	}
	if twin.SupplyFoldComputed || twin.SupplyFoldDeficitMS != 0 || twin.SupplyFoldIdealMS != 0 ||
		twin.SupplyFoldKnownMS != 0 || twin.SupplyFoldUnknownMS != 0 {
		t.Fatalf("conflicting absorbed accountings must clear and stay cleared: %+v", twin)
	}
}

// SFD 复核 F2 (recipient arm, M4 咬合): an R2 ×N SUM row is never a join
// recipient even when its envelope range and subject exactly match a donor —
// a single segment's fold does not describe a three-member sum.
func TestSupplyFoldJoinXNAggregateNeverRecipient(t *testing.T) {
	members := []ObservationRecord{
		sfdJoinRecord("M1", "root_cause_primary", "root_cause_primary:m1",
			"#RxComputationT-16816", 10.000, 45689, 50000,
			"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=running"),
		sfdJoinRecord("M2", "root_cause_primary", "root_cause_primary:m2",
			"#RxComputationT-16816", 12.000, 50001, 60000,
			"rank=3", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=running"),
		sfdJoinRecord("M3", "root_cause_primary", "root_cause_primary:m3",
			"#RxComputationT-16816", 20.000, 60001, 79142,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=running"),
	}
	donor := sfdJoinRecord("D1", "wakeup_causal_impact", "wakeup_causal_impact:supply",
		"#RxComputationT-16816", 25.000, 45689, 79142,
		append([]string{
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
			"dominant_state=running",
		}, "supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
			"fold_basis=known=20.000ms,unknown=0.000ms")...)
	// A distinct Object keeps the donor out of the members' R2 (subject,
	// object) group — it stays a single fold-carrying row.
	donor.Object = "compute_supply"
	got := TraceCausalProjectionFromObservationRecords(append(members, donor))
	var aggregate *TraceCausalProjectionNode
	for i := range got.PrimaryRootCauses {
		if got.PrimaryRootCauses[i].MergedCount == 3 {
			aggregate = &got.PrimaryRootCauses[i]
		}
	}
	if aggregate == nil {
		t.Fatalf("three same-kind members must aggregate: %+v", got.PrimaryRootCauses)
	}
	// Fixture self-check: the ×N envelope + subject + state DO equal the
	// donor's join identity — ONLY the ×N guard keeps the fold off the row.
	if aggregate.LineStart != 45689 || aggregate.LineEnd != 79142 ||
		aggregate.StateKind != "running" {
		t.Fatalf("fixture must reproduce the key-matching envelope: %+v", aggregate)
	}
	if aggregate.SupplyFoldComputed {
		t.Fatalf("a ×N SUM row must never take a single segment's fold: %+v", aggregate)
	}
	// The donor row itself keeps its accounting.
	donorNode := sfdJoinFindRunningTwin(t, got.SupportingHops, "D1")
	if donorNode == nil {
		donorNode = sfdJoinFindRunningTwin(t, got.OnChainCauses, "D1")
	}
	if donorNode == nil || !donorNode.SupplyFoldComputed || donorNode.SupplyFoldDeficitMS != 5.000 {
		t.Fatalf("donor accounting must stay untouched: %+v", donorNode)
	}
}

// SFD 复核 F2 (donor arm, M4 咬合): a fold-carrying ×N row (the producer-side
// folded_rows lane can carry both note families) never DONATES — its line
// range is a member envelope, not a segment identity.
func TestSupplyFoldJoinProducerFoldRowNeverDonor(t *testing.T) {
	records := sfdJoinQ6Records()[:1] // the bare running twin only
	foldRow := sfdJoinRecord("D2", "wakeup_causal_impact", "wakeup_causal_impact:fold",
		"#RxComputationT-16816", 25.000, 45689, 79142,
		append([]string{
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
			"dominant_state=running",
			"folded_rows=3", "folded_min_ms=2.000", "folded_max_ms=12.000",
			"folded_subjects=of-1,of-2",
		}, sfdJoinQ6FoldNotes()...)...)
	got := TraceCausalProjectionFromObservationRecords(append(records, foldRow))
	// Fixture self-check: the fold row really is a fold-carrying ×N node
	// sharing the twin's exact key.
	foldNode := sfdJoinFindRunningTwin(t, got.SupportingHops, "D2")
	if foldNode == nil {
		foldNode = sfdJoinFindRunningTwin(t, got.OnChainCauses, "D2")
	}
	if foldNode == nil || foldNode.MergedCount != 3 || !foldNode.SupplyFoldComputed {
		t.Fatalf("fixture must produce a fold-carrying ×N row: %+v", foldNode)
	}
	if twin := sfdJoinFindRunningTwin(t, got.PrimaryRootCauses, "E4"); twin == nil || twin.SupplyFoldComputed {
		t.Fatalf("a ×N row must never donate its fold to a bare twin: %+v", twin)
	}
}

// SFD 复核 F3: the R2 ×N SUM row clears the group-first seed's inherited
// SupplyFold group unconditionally (DuplicatePublications precedent) — a
// single member's deficit clause beside the member SUM was a 伪对比.
func TestSupplyFoldXNSumClearsInheritedFoldGroup(t *testing.T) {
	records := []ObservationRecord{
		sfdJoinRecord("M1", "root_cause_primary", "root_cause_primary:m1",
			"#RxComputationT-16816", 10.000, 45689, 50000,
			"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=running"),
		sfdJoinRecord("M2", "root_cause_primary", "root_cause_primary:m2",
			"#RxComputationT-16816", 12.000, 50001, 60000,
			"rank=3", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"chain_depth=1", "dominant_state=running"),
		// The impact-major sort seats the LARGEST member as the group-first
		// seed — the fold notes ride it so the inheritance path is provably
		// exercised (self-checked below).
		sfdJoinRecord("M3", "root_cause_primary", "root_cause_primary:m3",
			"#RxComputationT-16816", 20.000, 60001, 79142,
			append([]string{
				"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
				"chain_depth=1", "dominant_state=running",
			}, "supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
				"fold_basis=known=20.000ms,unknown=0.000ms")...),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	for _, bucket := range [][]TraceCausalProjectionNode{got.PrimaryRootCauses, got.OnChainCauses} {
		var aggregate *TraceCausalProjectionNode
		for i := range bucket {
			if bucket[i].MergedCount == 3 {
				aggregate = &bucket[i]
			}
		}
		if aggregate == nil {
			t.Fatalf("three same-kind members must aggregate: %+v", bucket)
		}
		if aggregate.EvidenceID != "M3" {
			t.Fatalf("fixture self-check: the fold carrier must seed the aggregate: %+v", aggregate)
		}
		if aggregate.ImpactMS != 42.000 {
			t.Fatalf("the ×N row must carry the member SUM: %+v", aggregate)
		}
		if aggregate.SupplyFoldComputed || aggregate.SupplyFoldDeficitMS != 0 || aggregate.SupplyFoldIdealMS != 0 ||
			aggregate.SupplyFoldKnownMS != 0 || aggregate.SupplyFoldUnknownMS != 0 {
			t.Fatalf("the ×N SUM row must clear the seed's inherited fold group: %+v", aggregate)
		}
	}
}
