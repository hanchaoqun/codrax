package types

import (
	"testing"
)

// HMC377 round 13 (batch-owner ruling R-377-13, 2026-11-22), lane face: the
// projection carriers the tree face reads.

// R-377-13 ③ (badges follow their seat): a row that absorbed a seat as
// evidence carries the seat's ordinal. The R-377-12 ⑤ two-ruler merge kept
// the census account members' own best ordinal, so the absorbed wakeup_chain
// seat's ordinal was published on no row. Real window at 532dc208d (review-12
// R12P-3, ~/hmc-work/hmc377/fix12/review-population/evidence/D3_badges.txt):
// 105 某应用 #ActionReaper-65009 (n13w) — census #6 4.312 + census #7 2.506
// with the wakeup_chain seat #3 25.846 inside the account; the merged row
// E8(+2) wore #6 and ➌ appeared on no line (base fa2f01465 wore ➌ on the
// summed 32.664 row).
func TestHMC377R13AbsorbedSeatKeepsItsOrdinal(t *testing.T) {
	t.Run("seat ordinal smaller than the account's", func(t *testing.T) {
		for _, order := range [][]int{{0, 1, 2}, {2, 0, 1}, {1, 2, 0}} {
			records := []ObservationRecord{
				r12RankRecord(6, "window_stats", 4.312, 100, 100, "f1"),
				r12RankRecord(7, "window_stats", 2.506, 100, 100, "f1"),
				r12RankRecord(3, "wakeup_chain", 25.846, 100, 100, "f1"),
			}
			ordered := make([]ObservationRecord, 0, len(records))
			for _, i := range order {
				ordered = append(ordered, records[i])
			}
			out := traceCausalProjectionAggregateSameKind(r12Nodes(ordered...))
			if len(out) != 1 {
				t.Fatalf("order %v: one account, one row; got %d", order, len(out))
			}
			row := out[0]
			if !r12Near(row.ImpactMS, 6.818) || !r12Near(row.EffectiveImpactMS, 6.818) {
				t.Fatalf("order %v: the row's value is the census account 6.818 (never + the seat), got %.3f / eff %.3f", order, row.ImpactMS, row.EffectiveImpactMS)
			}
			if row.Rank != 3 {
				t.Fatalf("order %v: the row carries the absorbed seat's ordinal #3, got #%d", order, row.Rank)
			}
			if !r12Near(row.RankQueryWindowStartTs, 10.0) || !r12Near(row.RankQueryWindowEndTs, 10.1) {
				t.Fatalf("order %v: the ordinal's window is the board's, got %.6f..%.6f", order, row.RankQueryWindowStartTs, row.RankQueryWindowEndTs)
			}
			if row.RankBoardTarget != "app-100" || row.RankBoardParamsFingerprint != "f1" {
				t.Fatalf("order %v: the ordinal's board follows the seat that supplies it, got %q/%q", order, row.RankBoardTarget, row.RankBoardParamsFingerprint)
			}
			if !r12HasEvidence(row, "#root_cause_rank:3") {
				t.Fatalf("order %v: the seat stays the row's evidence: %s %v", order, row.EvidenceID, row.MergedEvidenceIDs)
			}
		}
	})
	t.Run("single census seat", func(t *testing.T) {
		out := traceCausalProjectionAggregateSameKind(r12Nodes(
			r12RankRecord(6, "window_stats", 4.312, 100, 100, "f1"),
			r12RankRecord(3, "wakeup_chain", 25.846, 100, 100, "f1"),
			r12RankRecord(9, "wakeup_chain.causal_impacts", 2.0, 100, 100, "f1"),
		))
		if len(out) != 1 || out[0].Rank != 3 || !r12Near(out[0].ImpactMS, 4.312) {
			t.Fatalf("the single census seat is the account and carries the seat's #3: %d rows, rank %d", len(out), out[0].Rank)
		}
	})
	t.Run("account ordinal smaller than the seat's", func(t *testing.T) {
		out := traceCausalProjectionAggregateSameKind(r12Nodes(
			r12RankRecord(2, "window_stats", 60, 100, 100, "f1"),
			r12RankRecord(8, "window_stats", 10, 100, 100, "f1"),
			r12RankRecord(4, "wakeup_chain", 40, 100, 100, "f1"),
		))
		if len(out) != 1 || out[0].Rank != 2 {
			t.Fatalf("the account's own smaller ordinal stays: %d rows, rank %d", len(out), out[0].Rank)
		}
	})
}

// R-377-13 ②: the typed analysed-thread identity — the ranked member is its
// rank board's own target, by TID (the engine's tid-first identity); a nil
// carrier, an unknown TID or a comm label never matches.
func TestHMC377R13MemberIsTarget(t *testing.T) {
	coordinate := func(tid int, comm string) TraceThreadCoordinate {
		return TraceThreadCoordinate{TID: tid, TGID: 10, Comm: comm}
	}
	for _, c := range []struct {
		name     string
		subjects *TraceRankSubjects
		want     bool
	}{
		{"nil carrier", nil, false},
		{"same thread", &TraceRankSubjects{Target: coordinate(42, "app"), Member: coordinate(42, "app")}, true},
		{"same thread, renamed comm", &TraceRankSubjects{Target: coordinate(42, "app"), Member: coordinate(42, "worker")}, true},
		{"other thread, same comm", &TraceRankSubjects{Target: coordinate(42, "app"), Member: coordinate(43, "app")}, false},
		{"unknown target tid", &TraceRankSubjects{Target: coordinate(0, "app"), Member: coordinate(0, "app")}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := c.subjects.MemberIsTarget(); got != c.want {
				t.Fatalf("MemberIsTarget = %t, want %t", got, c.want)
			}
		})
	}
}
