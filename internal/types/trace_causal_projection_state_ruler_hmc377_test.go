package types

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// HMC377 round 12 (R11P-2, owner ruling R-377-12 ⑤): a projection same-kind
// merge never adds two rulers of one thread's same-state account. The real
// shape (105 网易云音乐 终端重启后冷启动 #5, TID 11405 [149.811770,150.011770),
// hmc377/fix11/review-population/raw/evidence/twoRuler_n24w.txt): one rank
// board publishes the thread's ready time as two window_stats census seats
// (158.904 on the edge lane, 0.123 on the wall lane — together the census
// account) and one wakeup_chain seat (32.616, inside that account). The ×N
// merge summed all three to 191.643 ms (raw ready 167.116, 8.089 of it held
// by the inversion seat). Here the board is a hand board with the same three
// seats in round numbers: census 60 + 10, wakeup_chain seat 40, window 100.

func r12RankRecord(rank int, source string, ms float64, tid, tgid int, board string) ObservationRecord {
	return ObservationRecord{
		ID:        fmt.Sprintf("trace_query:pub#root_cause_rank:%d", rank),
		Subject:   "app-100",
		Predicate: "root_cause_secondary",
		Object:    "runnable_wait",
		Value:     fmt.Sprintf("%.3f", ms),
		Unit:      "ms",
		RichNotes: []string{
			fmt.Sprintf("rank=%d", rank), "type=runnable_wait",
			fmt.Sprintf("impact_ms=%.3f", ms), fmt.Sprintf("effective_impact_ms=%.3f", ms),
			"source=" + source, "dominant_state=runnable", "chain_relevance=on_chain",
			"selected_window=10.000000..10.100000", "rank_board_target=app-100", "rank_board_params_fingerprint=" + board,
		},
		TraceRankSubjects: &TraceRankSubjects{Target: TraceThreadCoordinate{TID: 100, TGID: 100, Comm: "app"},
			Member: TraceThreadCoordinate{TID: tid, TGID: tgid, Comm: "app"}},
	}
}

func r12Nodes(records ...ObservationRecord) []TraceCausalProjectionNode {
	nodes := make([]TraceCausalProjectionNode, 0, len(records))
	for _, record := range records {
		nodes = append(nodes, traceCausalProjectionNodeFromRecord("root_cause", record))
	}
	return nodes
}

func r12Near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func r12HasEvidence(node TraceCausalProjectionNode, suffix string) bool {
	if strings.HasSuffix(node.EvidenceID, suffix) {
		return true
	}
	for _, id := range node.MergedEvidenceIDs {
		if strings.HasSuffix(id, suffix) {
			return true
		}
	}
	return false
}

func TestHMC377R12TwoRulersAreNeverSummed(t *testing.T) {
	const window = 100.0
	t.Run("census_pair_and_wakeup_seat", func(t *testing.T) {
		for _, order := range [][]int{{0, 1, 2}, {1, 0, 2}, {2, 1, 0}} {
			records := []ObservationRecord{
				r12RankRecord(2, "window_stats", 60, 100, 100, "f1"),
				r12RankRecord(3, "wakeup_chain", 40, 100, 100, "f1"),
				r12RankRecord(8, "window_stats", 10, 100, 100, "f1"),
			}
			ordered := make([]ObservationRecord, 0, len(records))
			for _, i := range order {
				ordered = append(ordered, records[i])
			}
			out := traceCausalProjectionAggregateSameKind(r12Nodes(ordered...))
			if len(out) != 1 {
				t.Fatalf("order %v: the three seats of one account form one row, got %d", order, len(out))
			}
			merged := out[0]
			// The account ruler: the census seats' Σ, never + the wakeup_chain seat.
			if !r12Near(merged.ImpactMS, 70) || !r12Near(merged.EffectiveImpactMS, 70) || !r12Near(merged.CumulativeImpactMS, 70) {
				t.Fatalf("order %v: merged row %.3f / eff %.3f / cum %.3f, want the census account 70.000 (never 110 = 60+40+10)",
					order, merged.ImpactMS, merged.EffectiveImpactMS, merged.CumulativeImpactMS)
			}
			if merged.EffectiveImpactMS > window {
				t.Fatalf("order %v: merged effective %.3f exceeds the %.0f ms window", order, merged.EffectiveImpactMS, window)
			}
			// The ×N grammar counts the summed instances only.
			if merged.MergedCount != 2 || !r12Near(merged.MergedMinMS, 10) || !r12Near(merged.MergedMaxMS, 60) {
				t.Fatalf("order %v: ×N grammar %d(%.3f~%.3f), want 2(10.000~60.000)", order, merged.MergedCount, merged.MergedMinMS, merged.MergedMaxMS)
			}
			// The wakeup_chain seat stays lossless evidence of the row, and the
			// row is led by the account, whatever the bucket order.
			if !r12HasEvidence(merged, "#root_cause_rank:3") || !r12HasEvidence(merged, "#root_cause_rank:8") {
				t.Fatalf("order %v: every seat's evidence rides the row: %s %v", order, merged.EvidenceID, merged.MergedEvidenceIDs)
			}
			if strings.HasSuffix(merged.EvidenceID, "#root_cause_rank:3") || !strings.HasSuffix(merged.EvidenceID, "#root_cause_rank:2") && !strings.HasSuffix(merged.EvidenceID, "#root_cause_rank:8") {
				t.Fatalf("order %v: the row is the census account, not the wakeup_chain seat: %s", order, merged.EvidenceID)
			}
			if merged.Rank != 2 {
				t.Fatalf("order %v: the account keeps its own best ordinal #2, got #%d", order, merged.Rank)
			}
		}
	})
	t.Run("one_census_seat_and_two_wakeup_seats", func(t *testing.T) {
		nodes := r12Nodes(
			r12RankRecord(3, "wakeup_chain", 40, 100, 100, "f1"),
			r12RankRecord(2, "window_stats", 70, 100, 100, "f1"),
			r12RankRecord(6, "wakeup_chain.causal_impacts", 20, 100, 100, "f1"),
		)
		// A wakeup_chain seat that already absorbed its same-fact chain view.
		nodes[2].MergedEvidenceIDs = []string{"trace_query:pub#wakeup_causal_impact:9"}
		out := traceCausalProjectionAggregateSameKind(nodes)
		if len(out) != 1 {
			t.Fatalf("one account, one row: got %d", len(out))
		}
		if !r12HasEvidence(out[0], "#wakeup_causal_impact:9") {
			t.Fatalf("a seat's own merged evidence rides the row: %v", out[0].MergedEvidenceIDs)
		}
		row := out[0]
		if !r12Near(row.ImpactMS, 70) || !r12Near(row.EffectiveImpactMS, 70) || row.MergedCount != 0 {
			t.Fatalf("a single census seat is the account itself (70.000, no ×N grammar), got %.3f eff %.3f ×%d", row.ImpactMS, row.EffectiveImpactMS, row.MergedCount)
		}
		if !r12HasEvidence(row, "#root_cause_rank:3") || !r12HasEvidence(row, "#root_cause_rank:6") {
			t.Fatalf("both wakeup_chain seats ride as evidence: %v", row.MergedEvidenceIDs)
		}
		if len(row.MeasurementOrigins) != 3 {
			t.Fatalf("the row keeps every seat's measurement origin (3), got %d", len(row.MeasurementOrigins))
		}
	})
	t.Run("absorb_never_writes_the_census_node", func(t *testing.T) {
		nodes := r12Nodes(
			r12RankRecord(2, "window_stats", 70, 100, 100, "f1"),
			r12RankRecord(3, "wakeup_chain", 40, 100, 100, "f1"),
			r12RankRecord(6, "wakeup_chain.causal_impacts", 20, 100, 100, "f1"),
		)
		// The census node already carries a merged id with spare capacity.
		nodes[0].MergedEvidenceIDs = append(make([]string, 0, 4), "trace_query:pub#earlier")
		out := traceCausalProjectionAggregateSameKind(nodes)
		if len(out) != 1 || len(out[0].MergedEvidenceIDs) != 3 {
			t.Fatalf("one row with the earlier id and both seats: %+v", out)
		}
		if spare := nodes[0].MergedEvidenceIDs[:2]; len(nodes[0].MergedEvidenceIDs) != 1 || spare[1] != "" {
			t.Fatalf("the absorb wrote into the census node's own evidence slice: %v", spare)
		}
	})
	t.Run("other_accounts_keep_the_sum", func(t *testing.T) {
		for _, c := range []struct {
			name string
			seat ObservationRecord
		}{
			// The same display label on another process's incarnation of the TID.
			{"other_thread_incarnation", r12RankRecord(3, "wakeup_chain", 40, 100, 200, "f1")},
			// Another board (other rank parameters) is another account.
			{"other_board", r12RankRecord(3, "wakeup_chain", 40, 100, 100, "f2")},
			// A wakeup_chain seat whose census account is not in the group.
			{"no_census_member", r12RankRecord(3, "wakeup_chain", 40, 100, 100, "f1")},
			// A seat of neither ruler (no source note on the record).
			{"no_ruler", func() ObservationRecord {
				r := r12RankRecord(3, "", 40, 100, 100, "f1")
				r.RichNotes = append([]string(nil), r.RichNotes...)
				for i, note := range r.RichNotes {
					if note == "source=" {
						r.RichNotes = append(r.RichNotes[:i], r.RichNotes[i+1:]...)
						break
					}
				}
				return r
			}()},
		} {
			t.Run(c.name, func(t *testing.T) {
				census := "window_stats"
				if c.name == "no_census_member" {
					census = "scheduler_latency_stats"
				}
				out := traceCausalProjectionAggregateSameKind(r12Nodes(
					r12RankRecord(2, census, 60, 100, 100, "f1"), c.seat, r12RankRecord(8, census, 10, 100, 100, "f1")))
				if len(out) != 1 || !r12Near(out[0].ImpactMS, 110) || out[0].MergedCount != 3 {
					t.Fatalf("no typed account identity, the existing ×N Σ stands (110.000 ×3); got %d rows, %.3f ×%d", len(out), out[0].ImpactMS, out[0].MergedCount)
				}
			})
		}
	})
	t.Run("co_published_board", func(t *testing.T) {
		// Two boards (two rank parameter sets) over one window: the same-fact
		// merge kept board f1's copy of the census seat and recorded f2 as its
		// co-publisher, while the wakeup_chain seat survived with f2's identity.
		// Which copy survives follows the order the boards were asked in; the
		// account identity must not (TestHMC376CrossBoardMergeKeepsTheLargest-
		// BoardsOwnAccountPublic, tieba boards asked as [2, 0.5]).
		nodes := r12Nodes(
			r12RankRecord(2, "window_stats", 60, 100, 100, "f1"),
			r12RankRecord(3, "wakeup_chain", 40, 100, 100, "f2"),
			r12RankRecord(8, "window_stats", 10, 100, 100, "f1"),
		)
		nodes[0].SameFactRankBoards = []TraceSameFactRankBoard{{Target: "app-100", ParamsFingerprint: "f2",
			QueryWindowStartTs: 10.0, QueryWindowEndTs: 10.1, AccountMS: 60}}
		out := traceCausalProjectionAggregateSameKind(nodes)
		if len(out) != 1 || !r12Near(out[0].ImpactMS, 70) || out[0].MergedCount != 2 || !r12HasEvidence(out[0], "#root_cause_rank:3") {
			t.Fatalf("a board that co-published the census seat is the wakeup_chain seat's board: want 70.000 ×2 with the seat as evidence, got %+v", out)
		}
	})
	t.Run("board_parts_read_the_same_account", func(t *testing.T) {
		nodes := r12Nodes(
			r12RankRecord(2, "window_stats", 60, 100, 100, "f1"),
			r12RankRecord(3, "wakeup_chain", 40, 100, 100, "f1"),
			r12RankRecord(8, "window_stats", 10, 100, 100, "f1"),
		)
		merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1, 2})
		if len(merged.SameFactRankBoards) == 0 {
			t.Fatal("prerequisite: the one board published every member")
		}
		for _, board := range merged.SameFactRankBoards {
			if !r12Near(board.AccountMS, 70) {
				t.Fatalf("the board vouches for the account ruler 70.000, got %.3f", board.AccountMS)
			}
		}
		for _, part := range merged.RankBoardPartAccounts {
			if part.CumulativeMS > 70+1e-6 || part.EffectiveMS > 70+1e-6 || part.AccountMS > 70+1e-6 {
				t.Fatalf("a board part never adds the two rulers: %+v", part)
			}
		}
	})
}
