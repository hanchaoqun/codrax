package types

import (
	"testing"
)

// HMC377 round 13 (batch-owner ruling R-377-13, 2026-11-22), lane face: the
// projection carriers the tree face reads.

// R-377-14 ③ (a badge sits beside its own rank row's value): the row that
// absorbed a wakeup_chain seat as evidence wears its OWN ordinal — the smallest
// ordinal of the census account members whose value it prints — never the
// absorbed seat's. The absorbed seat is a second ruler over an account another
// rank row already carries (register R10S-0), so its ordinal stays on no tree
// line until that account has one rank row.
//
// EVOLUTION RECORD (HMC377 round 14, R-377-14 ③, review-13 R13P-1): this test
// was TestHMC377R13AbsorbedSeatKeepsItsOrdinal and pinned the round-13
// adoption (the row carried the absorbed seat's smaller ordinal). On 15 real
// windows the adopted badge then sat beside the row's residual value: 105 抖音
// 极速版 #tp-default-8-11271 (n06w) 「➋ ⧖ 自身·runnable 2.450ms [E6(+2)]」
// while native rank 2 is the 27.417 ms wakeup_chain ruler of the ready account
// rank 1 (164.312 ms) already carries; the row's own ➎ was on no line and the
// side file listed 164.312, 2.450, 13.074, 8.122
// (~/hmc-work/hmc377/fix13/review-population/logs/own/{badges_head.tsv,lose.tsv};
// the absorbed seat's class per window: ~/hmc-work/hmc377/fix14/face/f1/absorbed_class_15.tsv).
// The same fixtures now pin the ruling: old → new = #3 → #6.
func TestHMC377R14AbsorbingRowWearsItsOwnOrdinal(t *testing.T) {
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
			if row.Rank != 6 {
				t.Fatalf("order %v: the row prints the census account and wears its own ordinal #6, never the absorbed seat's #3 (rank 3 is the 25.846 ruler); got #%d", order, row.Rank)
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
		if len(out) != 1 || out[0].Rank != 6 || !r12Near(out[0].ImpactMS, 4.312) {
			t.Fatalf("the single census seat is the account and keeps its own #6: %d rows, rank %d", len(out), out[0].Rank)
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

// R-377-12 ③ carriers of the restriction subset's typed fields
// (RestrictedReadyAccountMS / ShareCountedElsewhereMS / ShareCountedHereMS) in
// the projection merges — review-12 R12N-5 survivors S12, S13, S14
// (~/hmc-work/hmc377/fix12/review-pins/typesprobe/*): each branch below
// changes a carrier no test distinguished.
func TestHMC377R13RestrictedSubsetCarriers(t *testing.T) {
	restriction := func(id string, start, end, v float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{Role: TraceCausalRoleRootCauseContext, EvidenceID: id, Subject: "JankManager-9655",
			Predicate: "root_cause_adjacent", Object: "cpu_affinity_or_cpuset", TypeToken: "cpu_affinity_or_cpuset", StateKind: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Tier: "tertiary",
			ImpactMS: v, CumulativeImpactMS: v, EffectiveImpactMS: v, RunnableMS: v,
			QueryWindowStartTs: start, QueryWindowEndTs: end, StartTs: start + 0.01, EndTs: start + 0.01 + v/1000}
	}
	carved := func(n TraceCausalProjectionNode, account, elsewhere, here float64) TraceCausalProjectionNode {
		n.RestrictedReadyAccountMS, n.ShareCountedElsewhereMS, n.ShareCountedHereMS = account, elsewhere, here
		n.ShareClaims = []TraceCausalProjectionShareClaim{{Part: "anchored", Role: "chain_seat", ShareMS: elsewhere,
			Resolved: true, Exact: true, HolderEvidenceIDs: []string{"chain-" + n.EvidenceID}}}
		return n
	}
	// S14: the restriction row that left part of its instants to the
	// thread's on-chain seats is its own account form — it never Σ-merges
	// with the thread's plain restriction rows of other windows (the S14
	// mutant merged it into one ×4 row of 42.748, dropping its split).
	t.Run("S14 the carved row keeps its own account form", func(t *testing.T) {
		a := carved(restriction("cpu-a", 1.0, 1.2, 25.748), 27.507, 1.759, 25.748)
		b, c, d := restriction("cpu-b", 2.0, 2.2, 10.0), restriction("cpu-c", 3.0, 3.2, 5.0), restriction("cpu-d", 4.0, 4.2, 2.0)
		if got := traceCausalProjectionAnchorFormKey(a); got != "restricted_subset" {
			t.Fatalf("the carved restriction row's form key is restricted_subset, got %q", got)
		}
		for _, nodes := range [][]TraceCausalProjectionNode{{a, b, c, d}, {b, c, d, a}} {
			out := traceCausalProjectionAggregateSameKind(nodes)
			if len(out) != 2 {
				t.Fatalf("the carved row and the ×3 plain Σ row stay apart, got %d rows", len(out))
			}
			for _, n := range out {
				switch n.EvidenceID {
				case "cpu-a":
					if n.MergedCount != 0 || !r12Near(n.EffectiveImpactMS, 25.748) || !r12Near(n.RestrictedReadyAccountMS, 27.507) ||
						!r12Near(n.ShareCountedElsewhereMS, 1.759) || !r12Near(n.ShareCountedHereMS, 25.748) || len(n.ShareClaims) != 1 {
						t.Fatalf("the carved row keeps its own account and split: %+v", n)
					}
				default:
					if n.MergedCount != 3 || !r12Near(n.EffectiveImpactMS, 17.0) {
						t.Fatalf("the plain rows Σ-merge ×3 to 17.000: %+v", n)
					}
				}
			}
		}
	})
	// S13: a ×N Σ of carved rows keeps the Σ and no member's split — the
	// per-member split and its claims have no single referent on a member Σ
	// (the S13 mutant left cpu-a's 27.507 / 1.759 / 25.748 on the ×3 row of
	// 48.748).
	t.Run("S13 a Σ of carved rows keeps no member's split", func(t *testing.T) {
		out := traceCausalProjectionAggregateSameKind([]TraceCausalProjectionNode{
			carved(restriction("cpu-a", 1.0, 1.2, 25.748), 27.507, 1.759, 25.748),
			carved(restriction("cpu-a2", 5.0, 5.2, 20.0), 21.0, 1.0, 20.0),
			carved(restriction("cpu-a3", 6.0, 6.2, 3.0), 4.0, 1.0, 3.0),
		})
		if len(out) != 1 || out[0].MergedCount != 3 || !r12Near(out[0].EffectiveImpactMS, 48.748) {
			t.Fatalf("the three carved rows Σ-merge ×3 to 48.748: %+v", out)
		}
		if n := out[0]; n.RestrictedReadyAccountMS != 0 || n.ShareCountedElsewhereMS != 0 || n.ShareCountedHereMS != 0 || n.ShareClaims != nil {
			t.Fatalf("the Σ row wears no member's split: account %.3f elsewhere %.3f here %.3f claims %v",
				n.RestrictedReadyAccountMS, n.ShareCountedElsewhereMS, n.ShareCountedHereMS, n.ShareClaims)
		}
	})
	// S12: the same fact published twice (one record carrying the carve, its
	// twin bare) is one row, and the row keeps the carve with the claims whose
	// resolution wrote it — whichever record survives (the S12 mutant dropped
	// the carve when the bare twin survived).
	t.Run("S12 the same-fact twin keeps the carve", func(t *testing.T) {
		bare := restriction("cpu-twin", 1.0, 1.2, 25.748)
		bare.LineStart, bare.LineEnd = 400, 480
		withCarve := carved(bare, 27.507, 1.759, 25.748)
		withCarve.EvidenceID = "cpu-carved"
		for _, nodes := range [][]TraceCausalProjectionNode{{bare, withCarve}, {withCarve, bare}} {
			projection := TraceCausalProjection{AdjacentCauses: append([]TraceCausalProjectionNode(nil), nodes...)}
			traceCausalProjectionMergeSameFacts(&projection)
			if len(projection.AdjacentCauses) != 1 {
				t.Fatalf("one fact, one row: got %d", len(projection.AdjacentCauses))
			}
			n := projection.AdjacentCauses[0]
			if !r12Near(n.RestrictedReadyAccountMS, 27.507) || !r12Near(n.ShareCountedElsewhereMS, 1.759) || !r12Near(n.ShareCountedHereMS, 25.748) {
				t.Fatalf("survivor %s: the carve travels with the fact: account %.3f elsewhere %.3f here %.3f",
					n.EvidenceID, n.RestrictedReadyAccountMS, n.ShareCountedElsewhereMS, n.ShareCountedHereMS)
			}
			if len(n.ShareClaims) != 1 || n.ShareClaims[0].Part != "anchored" || !r12Near(n.ShareClaims[0].ShareMS, 1.759) {
				t.Fatalf("survivor %s: the carve's claims travel with it: %+v", n.EvidenceID, n.ShareClaims)
			}
		}
	})
	// A survivor with its own claims keeps them and takes no carve whose
	// claims it does not carry.
	t.Run("S12 a survivor with its own claims takes no foreign carve", func(t *testing.T) {
		own := restriction("cpu-own", 1.0, 1.2, 25.748)
		own.LineStart, own.LineEnd = 400, 480
		own.ShareClaims = []TraceCausalProjectionShareClaim{{Part: "subset", Role: "subset_seat", ShareMS: 1.0}}
		withCarve := carved(own, 27.507, 1.759, 25.748)
		withCarve.EvidenceID = "cpu-carved"
		projection := TraceCausalProjection{AdjacentCauses: []TraceCausalProjectionNode{own, withCarve}}
		traceCausalProjectionMergeSameFacts(&projection)
		if len(projection.AdjacentCauses) != 1 {
			t.Fatalf("one fact, one row: got %d", len(projection.AdjacentCauses))
		}
		if n := projection.AdjacentCauses[0]; n.ShareCountedElsewhereMS != 0 || len(n.ShareClaims) != 1 || n.ShareClaims[0].Part != "subset" {
			t.Fatalf("the survivor keeps its own claims and no foreign carve: %+v", n)
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
