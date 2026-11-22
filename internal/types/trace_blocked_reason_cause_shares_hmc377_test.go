package types

import (
	"math"
	"testing"
)

// HMC377 round 11 (R10P-4, R-377-11 ④): the ×N merge of one thread's
// same-kind cause-partition rows keeps the causes. Real shape: 105 元宝
// 后台销毁冷启动 #2, TID 58533 bindApplication [10834.099821,10834.620005):
// nine D partition seats (vfs_path_open_ng 3.435 ms, fscache_readahead_i
// 0.445 ms, mprotect_range 0.170 ms, …) merged into one 4.256 ms row and the
// callers left every face (fix10/review-population/out/ybb2/head_ybb2.*).
// These pins sit on the one merge body (traceCausalProjectionMergeSameKindMembers)
// and the one wording (TraceBlockedReasonCauseText).

func r10p4Seat(id, caller string, ms float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{EvidenceID: id, Subject: "app-100", Object: "d_state_or_io_wait",
		ImpactMS: ms, CumulativeImpactMS: ms, EffectiveImpactMS: ms, BlockedReasonCaller: caller}
}

func r10p4Members(n int) []int {
	members := make([]int, n)
	for i := range members {
		members[i] = i
	}
	return members
}

func r10p4AssertShares(t *testing.T, got []TraceBlockedReasonCallerShare, want []TraceBlockedReasonCallerShare) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cause list %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Caller != want[i].Caller || math.Abs(got[i].MS-want[i].MS) > 1e-9 {
			t.Fatalf("cause %d = %+v, want %+v (list %+v)", i, got[i], want[i], got)
		}
	}
}

func TestHMC377R10P4MergeKeepsBlockedReasonCauses(t *testing.T) {
	t.Run("d_partitions_with_remainder", func(t *testing.T) {
		remainder := r10p4Seat("rem", "", 0.8)
		remainder.DStateCauseUnprovenRemainder = true
		nodes := []TraceCausalProjectionNode{
			r10p4Seat("a", "alpha_wait", 3), r10p4Seat("b", "beta_wait", 2), r10p4Seat("g", "gamma_wait", 1),
			remainder, r10p4Seat("d", "delta_wait", 0.5),
		}
		merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, r10p4Members(len(nodes)))
		if merged.BlockedReasonCaller != "" {
			t.Fatalf("a non-unanimous group keeps no single caller: %q", merged.BlockedReasonCaller)
		}
		r10p4AssertShares(t, merged.BlockedReasonCallerShares, []TraceBlockedReasonCallerShare{
			{"alpha_wait", 3}, {"beta_wait", 2}, {"gamma_wait", 1}, {"delta_wait", 0.5}})
		if math.Abs(merged.BlockedReasonUnprovenMS-0.8) > 1e-9 {
			t.Fatalf("the remainder member's ms is the row's unproven part, got %.3f", merged.BlockedReasonUnprovenMS)
		}
		if merged.DStateCauseUnprovenRemainder {
			t.Fatal("a row that names proven causes is not the honest remainder as a whole")
		}
		if got, want := merged.BlockedReasonCauseText(true), "alpha_wait 3.000ms、beta_wait 2.000ms、gamma_wait 1.000ms 等4类"; got != want {
			t.Fatalf("zh cause words %q, want %q", got, want)
		}
		if got, want := merged.BlockedReasonCauseText(false), "alpha_wait 3.000ms, beta_wait 2.000ms, gamma_wait 1.000ms and 1 more"; got != want {
			t.Fatalf("en cause words %q, want %q", got, want)
		}
		if got := TraceBlockedReasonUnprovenText(merged.BlockedReasonUnprovenMS, true); got != "原因未证 0.800ms" {
			t.Fatalf("zh unproven words %q", got)
		}
		if got := TraceBlockedReasonUnprovenText(merged.BlockedReasonUnprovenMS, false); got != "cause unproven 0.800ms" {
			t.Fatalf("en unproven words %q", got)
		}
	})
	t.Run("remainder_seed", func(t *testing.T) {
		remainder := r10p4Seat("rem", "", 4)
		remainder.DStateCauseUnprovenRemainder = true
		nodes := []TraceCausalProjectionNode{remainder, r10p4Seat("a", "alpha_wait", 3), r10p4Seat("b", "beta_wait", 2)}
		merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, r10p4Members(len(nodes)))
		r10p4AssertShares(t, merged.BlockedReasonCallerShares, []TraceBlockedReasonCallerShare{{"alpha_wait", 3}, {"beta_wait", 2}})
		if merged.DStateCauseUnprovenRemainder || math.Abs(merged.BlockedReasonUnprovenMS-4) > 1e-9 {
			t.Fatalf("the remainder seed's flag must not speak for the named causes: flag=%t unproven=%.3f",
				merged.DStateCauseUnprovenRemainder, merged.BlockedReasonUnprovenMS)
		}
	})
	t.Run("io_wait_partitions", func(t *testing.T) {
		nodes := []TraceCausalProjectionNode{r10p4Seat("a", "z_erofs_vle_normala", 0.3), r10p4Seat("b", "fscache_readahead_i", 0.2), r10p4Seat("c", "vfs_path_open_ng", 0.1)}
		for i := range nodes {
			nodes[i].Object = "io_wait"
		}
		merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, r10p4Members(len(nodes)))
		r10p4AssertShares(t, merged.BlockedReasonCallerShares, []TraceBlockedReasonCallerShare{
			{"z_erofs_vle_normala", 0.3}, {"fscache_readahead_i", 0.2}, {"vfs_path_open_ng", 0.1}})
		if merged.BlockedReasonUnprovenMS != 0 {
			t.Fatalf("no remainder member, no unproven part: %.3f", merged.BlockedReasonUnprovenMS)
		}
		if got, want := merged.BlockedReasonCauseText(true), "z_erofs_vle_normala 0.300ms、fscache_readahead_i 0.200ms、vfs_path_open_ng 0.100ms"; got != want {
			t.Fatalf("three causes are all named, no count suffix: %q, want %q", got, want)
		}
	})
	t.Run("same_cause_members_sum_and_order_largest_first", func(t *testing.T) {
		nodes := []TraceCausalProjectionNode{r10p4Seat("a", "beta_wait", 1), r10p4Seat("b", "alpha_wait", 1.5), r10p4Seat("c", "beta_wait", 1), r10p4Seat("d", "gamma_wait", 1.5)}
		merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, r10p4Members(len(nodes)))
		// Ties keep first appearance: alpha_wait (member 1) before gamma_wait (member 3).
		r10p4AssertShares(t, merged.BlockedReasonCallerShares, []TraceBlockedReasonCallerShare{
			{"beta_wait", 2}, {"alpha_wait", 1.5}, {"gamma_wait", 1.5}})
	})
	t.Run("callerless_member_keeps_the_named_cause", func(t *testing.T) {
		nodes := []TraceCausalProjectionNode{r10p4Seat("a", "dma_fence_default_wait", 40), r10p4Seat("b", "dma_fence_default_wait", 5), r10p4Seat("c", "", 10)}
		merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, r10p4Members(len(nodes)))
		if merged.BlockedReasonCaller != "" {
			t.Fatalf("a callerless member still vetoes the unanimous single word: %q", merged.BlockedReasonCaller)
		}
		r10p4AssertShares(t, merged.BlockedReasonCallerShares, []TraceBlockedReasonCallerShare{{"dma_fence_default_wait", 45}})
		if merged.BlockedReasonUnprovenMS != 0 {
			t.Fatal("a callerless member that is not a remainder seat claims no unproven part")
		}
		if got := merged.BlockedReasonCauseText(true); got != "dma_fence_default_wait 45.000ms" {
			t.Fatalf("one named cause of a merged row carries its ms: %q", got)
		}
	})
	t.Run("unanimous_group_keeps_the_single_cause_word", func(t *testing.T) {
		nodes := []TraceCausalProjectionNode{r10p4Seat("a", "__d_path", 6.763), r10p4Seat("b", "__d_path", 3.286), r10p4Seat("c", "__d_path", 3.221)}
		merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, r10p4Members(len(nodes)))
		if merged.BlockedReasonCaller != "__d_path" || merged.BlockedReasonCallerShares != nil || merged.BlockedReasonUnprovenMS != 0 {
			t.Fatalf("a unanimous group keeps the single-cause word and no list: %+v", merged)
		}
		if got := merged.BlockedReasonCauseText(true); got != "__d_path" {
			t.Fatalf("single-cause word unchanged, got %q", got)
		}
	})
	t.Run("pre_merged_member_joins_its_cause_list", func(t *testing.T) {
		inner := []TraceCausalProjectionNode{r10p4Seat("a", "alpha_wait", 3), r10p4Seat("b", "beta_wait", 2), r10p4Seat("c", "gamma_wait", 1)}
		pre := traceCausalProjectionMergeSameKindMembers(inner, 0, r10p4Members(len(inner)))
		nodes := []TraceCausalProjectionNode{pre, r10p4Seat("d", "alpha_wait", 0.5), r10p4Seat("e", "delta_wait", 0.25)}
		merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, r10p4Members(len(nodes)))
		r10p4AssertShares(t, merged.BlockedReasonCallerShares, []TraceBlockedReasonCallerShare{
			{"alpha_wait", 3.5}, {"beta_wait", 2}, {"gamma_wait", 1}, {"delta_wait", 0.25}})
	})
	t.Run("overlapping_caliber_keeps_each_cause_lower_bound", func(t *testing.T) {
		nodes := []TraceCausalProjectionNode{r10p4Seat("a", "alpha_wait", 3), r10p4Seat("b", "alpha_wait", 2), r10p4Seat("c", "beta_wait", 1)}
		sum, _ := traceCausalProjectionMergedBlockedReasonCauses(nodes, r10p4Members(len(nodes)), traceCausalProjectionDisplayValue, true)
		r10p4AssertShares(t, sum, []TraceBlockedReasonCallerShare{{"alpha_wait", 5}, {"beta_wait", 1}})
		// A union / MAX / mirror value channel re-measures the same instants
		// across members, so a cause never sums them: its largest member.
		max, _ := traceCausalProjectionMergedBlockedReasonCauses(nodes, r10p4Members(len(nodes)), traceCausalProjectionDisplayValue, false)
		r10p4AssertShares(t, max, []TraceBlockedReasonCallerShare{{"alpha_wait", 3}, {"beta_wait", 1}})
	})
}

func TestHMC377R10P4CauseWordsAndAdoption(t *testing.T) {
	t.Run("valueless_cause_is_named_without_a_zero", func(t *testing.T) {
		got := TraceBlockedReasonCauseText("", []TraceBlockedReasonCallerShare{{"alpha_wait", 1}, {"beta_wait", 0}}, true)
		if got != "alpha_wait 1.000ms、beta_wait" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("no_cause_no_words", func(t *testing.T) {
		if got := TraceBlockedReasonCauseText(" ", nil, true); got != "" {
			t.Fatalf("got %q", got)
		}
		if got := TraceBlockedReasonUnprovenText(0, false); got != "" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("adopt_moves_the_cause_as_one_unit", func(t *testing.T) {
		src := TraceCausalProjectionNode{BlockedReasonCallerShares: []TraceBlockedReasonCallerShare{{"alpha_wait", 3}}, BlockedReasonUnprovenMS: 1}
		var dst TraceCausalProjectionNode
		TraceCausalProjectionAdoptBlockedReasonCause(&dst, src)
		if len(dst.BlockedReasonCallerShares) != 1 || dst.BlockedReasonUnprovenMS != 1 {
			t.Fatalf("empty slot adopts the list and its unproven ms: %+v", dst)
		}
		src.BlockedReasonCallerShares[0].Caller = "mutated"
		if dst.BlockedReasonCallerShares[0].Caller != "alpha_wait" {
			t.Fatal("the adopted list is a copy")
		}
		own := TraceCausalProjectionNode{BlockedReasonCaller: "own_wait"}
		TraceCausalProjectionAdoptBlockedReasonCause(&own, src)
		if own.BlockedReasonCaller != "own_wait" || own.BlockedReasonCallerShares != nil {
			t.Fatalf("a row that names a cause keeps its own: %+v", own)
		}
	})
	t.Run("same_fact_absorb_carries_the_list", func(t *testing.T) {
		survivor := TraceCausalProjectionNode{EvidenceID: "s"}
		loser := TraceCausalProjectionNode{EvidenceID: "l", BlockedReasonCallerShares: []TraceBlockedReasonCallerShare{{"alpha_wait", 3}, {"beta_wait", 2}}}
		traceCausalProjectionAbsorbSameFact(&survivor, loser, map[string]bool{}, nil)
		if len(survivor.BlockedReasonCallerShares) != 2 {
			t.Fatalf("one fact keeps its causes through the R1 absorb: %+v", survivor.BlockedReasonCallerShares)
		}
	})
}

// HMC377 round 12 (R11T-7, R-377-12 ⑧): on a Σ-caliber merge the unproven ms
// of the merged row is the Σ of every member's unproven ms, exactly as each
// named cause is the Σ of its members. Only a union / MAX / mirror merge
// keeps the largest member instead. Mutant MF (report the MAX on a Σ-caliber
// merge, hmc377/fix11/review-population/mutant/MF_unproven_max_on_sum.diff)
// survived every suite: no pin had two unproven contributions of different
// size on a Σ-caliber merge. The shape is reachable whenever one thread's
// D/IO partition seats come from two disjoint query windows (each window's
// engine mints its own honest-remainder seat; disjoint windows measured
// disjoint wall clock and the merge keeps the Σ).
func TestHMC377R12UnprovenMsSumsOnASumCaliberMerge(t *testing.T) {
	windowed := func(node TraceCausalProjectionNode, start, end, occStart, occEnd float64) TraceCausalProjectionNode {
		node.QueryWindowStartTs, node.QueryWindowEndTs = start, end
		node.StartTs, node.EndTs = occStart, occEnd
		return node
	}
	remainder := func(id string, ms float64) TraceCausalProjectionNode {
		node := r10p4Seat(id, "", ms)
		node.DStateCauseUnprovenRemainder = true
		return node
	}
	t.Run("disjoint_windows_merge", func(t *testing.T) {
		nodes := []TraceCausalProjectionNode{
			windowed(r10p4Seat("a", "alpha_wait", 3), 1.0, 1.1, 1.01, 1.04),
			windowed(remainder("ra", 0.8), 1.0, 1.1, 1.05, 1.06),
			windowed(remainder("rb", 1.2), 2.0, 2.1, 2.01, 2.03),
		}
		out := traceCausalProjectionAggregateSameKind(nodes)
		if len(out) != 1 || out[0].MergedCount != 3 {
			t.Fatalf("one thread's three D partition seats form one ×3 row, got %d rows (%+v)", len(out), out)
		}
		merged := out[0]
		if merged.MergedIntervalUnion || merged.MergedCrossWindowMax || merged.MergedSameSegmentMirror {
			t.Fatalf("disjoint windows keep the Σ caliber: union=%t max=%t mirror=%t",
				merged.MergedIntervalUnion, merged.MergedCrossWindowMax, merged.MergedSameSegmentMirror)
		}
		if math.Abs(merged.ImpactMS-5) > 1e-9 {
			t.Fatalf("the merged row's value is the Σ 5.000, got %.3f", merged.ImpactMS)
		}
		r10p4AssertShares(t, merged.BlockedReasonCallerShares, []TraceBlockedReasonCallerShare{{"alpha_wait", 3}})
		if math.Abs(merged.BlockedReasonUnprovenMS-2) > 1e-9 {
			t.Fatalf("unproven ms on a Σ merge = 0.800 + 1.200 = 2.000 (named 3.000 + unproven 2.000 = the row's 5.000), got %.3f",
				merged.BlockedReasonUnprovenMS)
		}
		if got, want := TraceBlockedReasonUnprovenText(merged.BlockedReasonUnprovenMS, true), "原因未证 2.000ms"; got != want {
			t.Fatalf("zh unproven words %q, want %q", got, want)
		}
	})
	t.Run("pre_merged_member_unproven_part", func(t *testing.T) {
		// A member that is itself a merged cause row brings its own
		// unproven part; a Σ merge adds it to the remainder member's.
		inner := []TraceCausalProjectionNode{r10p4Seat("a", "alpha_wait", 3), r10p4Seat("b", "beta_wait", 2), remainder("ri", 0.6)}
		pre := traceCausalProjectionMergeSameKindMembers(inner, 0, r10p4Members(len(inner)))
		nodes := []TraceCausalProjectionNode{pre, remainder("ro", 0.9), r10p4Seat("c", "alpha_wait", 1)}
		merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, r10p4Members(len(nodes)))
		if math.Abs(merged.BlockedReasonUnprovenMS-1.5) > 1e-9 {
			t.Fatalf("unproven ms = 0.600 (pre-merged member) + 0.900 (remainder member) = 1.500, got %.3f", merged.BlockedReasonUnprovenMS)
		}
	})
	t.Run("display_fold", func(t *testing.T) {
		// The display micro-fold is always Σ caliber (TraceCausalProjectionFoldBlockedReasonCauses).
		shares, unproven := TraceCausalProjectionFoldBlockedReasonCauses([]TraceCausalProjectionNode{
			r10p4Seat("a", "alpha_wait", 3), remainder("r1", 0.4), remainder("r2", 1.1)})
		r10p4AssertShares(t, shares, []TraceBlockedReasonCallerShare{{"alpha_wait", 3}})
		if math.Abs(unproven-1.5) > 1e-9 {
			t.Fatalf("the fold's unproven ms = 0.400 + 1.100 = 1.500, got %.3f", unproven)
		}
	})
	t.Run("overlapping_caliber_keeps_the_largest", func(t *testing.T) {
		nodes := []TraceCausalProjectionNode{r10p4Seat("a", "alpha_wait", 3), remainder("r1", 0.4), remainder("r2", 1.1)}
		_, unproven := traceCausalProjectionMergedBlockedReasonCauses(nodes, r10p4Members(len(nodes)), traceCausalProjectionDisplayValue, false)
		if math.Abs(unproven-1.1) > 1e-9 {
			t.Fatalf("a union / MAX / mirror merge keeps the largest unproven member 1.100, got %.3f", unproven)
		}
	})
}
