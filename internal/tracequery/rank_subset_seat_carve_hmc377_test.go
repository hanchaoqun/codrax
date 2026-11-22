package tracequery

import (
	"context"
	"math"
	"strings"
	"testing"
)

// HMC377 round 11 (batch-owner ruling R-377-11 ②, 2026-11-21): one carrier per
// instant of a thread's ready time. A cpu_affinity_or_cpuset seat publishes a
// typed subset of the thread's ready time (its runnable segments under a CPU
// restriction); the thread's ◇ ready remainder seat publishes the rest of the
// census account outside the dependency windows; the chain object holds the
// instants inside them. Review 10 reproduced the three priced on the same
// instants on real windows (105 网易云 后台销毁 #4 nputmethod.anco-38135:
// 7.121 + 98.304 + 36.968 > raw ready 105.717). The in-repo donghu fixture
// carries the same shape at the base (b2325eba6):
//
//	JankManager-9655   ⛓ 1.759 + ◇ 29.432 + restriction 27.507 = 58.698
//	logd.writer-9163   ⛓ 0.033 + ◇ 49.623 + restriction 48.519 = 98.175
//	udk-irq-1-76               ◇ 0.484 + restriction 0.539 =  1.023
//
// on 31.191 / 49.656 / 0.539ms of ready time (raw rawpop.py over this window,
// ~/hmc-work/hmc377/fix11/r4c/pins/raw/dh.raw.json: 27.507 / 48.519 / 0.539
// of closed segments; 9655 and 9163 also hold a segment still open at the
// trace end, 3.684 and 1.137ms, which the census counts).

const hmc377R11DonghuFixture = "../../eval/fixtures/real_traces/donghu.ftrace"

func hmc377R11Board(t *testing.T, path string, q Query) RootCauseRankResult {
	t.Helper()
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	return BuildRootCauseRank(idx, q)
}

// hmc377R11ReadySum is the thread's ready time a board publishes: every
// rank row's runnable-family account (a chain inversion candidate through its
// gated runnable part, a window inversion row through its gated overlap —
// the value it ranks and is summed by).
func hmc377R11ReadySum(items []RootCauseRankItem, pid int) float64 {
	total := 0.0
	for _, it := range items {
		if it.Thread.PID != pid || it.AbsorbedByRankFamily || it.Rank <= 0 {
			continue
		}
		switch strings.TrimSpace(it.Type) {
		case "priority_inversion_candidate":
			total += it.GatedRunnableMs
		case "priority_inversion_runnable_wait":
			total += it.EffectiveImpactMs
		case "runnable_wait", "cpu_affinity_or_cpuset", "scheduler_latency", "fragmented_runnable_wait":
			total += it.EffectiveImpactMs
		}
	}
	return total
}

func TestHMC377R11RestrictionSeatAndReadyRemainderCarryEachInstantOnce(t *testing.T) {
	rank := hmc377R11Board(t, hmc377R11DonghuFixture, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	find := func(pid int, typ string, remainder bool) *RootCauseRankItem {
		for i := range rank.Items {
			it := &rank.Items[i]
			if it.Thread.PID == pid && it.Type == typ && it.ChainAnchorRemainderSeat == remainder {
				return it
			}
		}
		return nil
	}
	// logd.writer-9163 sits on the CompThread_0-2955 board of the same window.
	rank2955 := hmc377R11Board(t, hmc377R11DonghuFixture, Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	for _, tc := range []struct {
		name  string
		board []RootCauseRankItem
		pid   int
		ready float64 // census account: closed + open-at-trace-end segments
	}{
		{"JankManager-9655", rank.Items, 9655, 31.191},
		{"logd.writer-9163", rank2955.Items, 9163, 49.656},
		{"udk-irq-1-76", rank.Items, 76, 0.539},
	} {
		t.Run(tc.name+"/board sum within the ready account", func(t *testing.T) {
			got := hmc377R11ReadySum(tc.board, tc.pid)
			if got <= 0 {
				t.Fatalf("%s holds no ready-time seat on the board", tc.name)
			}
			if got > tc.ready+0.003 {
				t.Fatalf("%s board prices %.3fms of ready time on a %.3fms account", tc.name, got, tc.ready)
			}
		})
	}
	t.Run("JankManager-9655/restriction seat keeps its instants outside the chain seat", func(t *testing.T) {
		// EVOLUTION RECORD (HMC377 round 12, batch-owner ruling R-377-12 ③,
		// 2026-11-22): old → new = the seat's split rode the thread's
		// same-source split fields (ChainAnchoredMs 1.759 / ChainAnchorFullMs
		// 27.507 / ChainAnchorRemainderSeat) → its own typed subset fields
		// (RestrictedReadyAccountMs 27.507 / ShareCountedElsewhereMs 1.759 /
		// ShareCountedHereMs 25.748), and the split fields stay zero on it.
		// Raw evidence that the old fields misled every face: review-population
		// out/new/head/head_n21w.fence_zh.txt (the restriction row stated
		// 「同源二分:全窗182.107ms」 and was paired as the thread's ◇ seat).
		seat := find(9655, "cpu_affinity_or_cpuset", false)
		if seat == nil || math.Abs(seat.RunnableMs-25.748) > 0.002 || math.Abs(seat.EffectiveImpactMs-25.748) > 0.002 ||
			math.Abs(seat.RestrictedReadyAccountMs-27.507) > 0.002 || math.Abs(seat.ShareCountedElsewhereMs-1.759) > 0.002 ||
			math.Abs(seat.ShareCountedHereMs-25.748) > 0.002 ||
			seat.ChainAnchoredMs != 0 || seat.ChainAnchorFullMs != 0 || seat.ChainAnchorRemainderSeat ||
			seat.ChainRelevance != "adjacent" || seat.ChainCredentialLaneDemoted {
			t.Fatalf("restriction seat split: %+v", seat)
		}
		var anchored *RootCauseShareClaim
		for i := range seat.ShareClaims {
			if seat.ShareClaims[i].Part == RootCauseShareClaimAnchored {
				anchored = &seat.ShareClaims[i]
			}
		}
		if anchored == nil || !anchored.Resolved || anchored.Role != RootCauseShareHolderChainSeat || math.Abs(anchored.ShareMs-1.759) > 0.002 {
			t.Fatalf("the instants inside the walked window are handed to the chain seat: %+v", seat.ShareClaims)
		}
	})
	t.Run("JankManager-9655/the board orders the seats by the values they publish", func(t *testing.T) {
		// The carve runs before the cap: the enrich pool the cap reads
		// already holds the restriction seat's and the remainder's own
		// accounts, not the whole restricted / remaining ready time.
		//
		// HMC377 round 12 (R-377-12 ⑧, register R11T-1): the pool's order and
		// every value channel and score the cap and the sort read — review
		// 11's mutants P27 (score not scaled: remainder 25.724), P28
		// (cumulative kept: 29.432) and P29 (projected kept: 29.432) changed
		// them on this board (review-pins/probe/channels_probe.log) while this
		// subtest asserted the effective channel alone.
		seatAt, remAt := -1, -1
		var seat, rem *RootCauseRankItem
		for i := range rank.preTruncationItems {
			it := &rank.preTruncationItems[i]
			if it.Thread.PID != 9655 {
				continue
			}
			switch {
			case it.Type == "cpu_affinity_or_cpuset":
				seat, seatAt = it, i
			case it.Type == "runnable_wait" && it.ChainAnchorRemainderSeat:
				// The last remainder in the pool is the enrich lane's (the
				// pool appends the build lane's input first).
				rem, remAt = it, i
			}
		}
		if seat == nil || rem == nil {
			t.Fatalf("pre-cap pool: restriction=%+v remainder=%+v", seat, rem)
		}
		for _, ch := range []struct {
			name      string
			got, want float64
		}{
			{"restriction runnable", seat.RunnableMs, 25.748}, {"restriction impact", seat.ImpactMs, 25.748},
			{"restriction projected", seat.ProjectedImpactMs, 25.748}, {"restriction cumulative", seat.CumulativeImpactMs, 25.748},
			{"restriction effective", seat.EffectiveImpactMs, 25.748}, {"restriction score", seat.Score, 18.2206},
			{"remainder runnable", rem.RunnableMs, 3.684}, {"remainder impact", rem.ImpactMs, 3.684},
			{"remainder projected", rem.ProjectedImpactMs, 3.684}, {"remainder cumulative", rem.CumulativeImpactMs, 3.684},
			{"remainder effective", rem.EffectiveImpactMs, 3.684},
			// 25.7236 (its score on its whole remaining account) × 3.684 / 29.432.
			{"remainder score", rem.Score, 3.2198},
		} {
			if math.Abs(ch.got-ch.want) > 0.002 {
				t.Errorf("pre-cap pool %s = %.4f, want %.4f", ch.name, ch.got, ch.want)
			}
		}
		if seatAt > remAt {
			t.Fatalf("the sorted enrich pool must order the restriction seat (%.3f) before the remainder (%.3f): positions %d, %d",
				seat.EffectiveImpactMs, rem.EffectiveImpactMs, seatAt, remAt)
		}
	})
	t.Run("JankManager-9655/remainder carries the rest and names the restriction seat", func(t *testing.T) {
		rem := find(9655, "runnable_wait", true)
		seat := find(9655, "cpu_affinity_or_cpuset", false)
		if rem == nil || seat == nil || math.Abs(rem.RunnableMs-3.684) > 0.002 || math.Abs(rem.EffectiveImpactMs-3.684) > 0.002 ||
			math.Abs(rem.ChainAnchoredMs-1.759) > 0.002 || math.Abs(rem.ChainAnchorFullMs-31.191) > 0.002 {
			t.Fatalf("remainder: %+v", rem)
		}
		var subset *RootCauseShareClaim
		for i := range rem.ShareClaims {
			// The part name is spelled out so this pin compiles at the base.
			if rem.ShareClaims[i].Part == RootCauseShareClaimPart("subset") {
				subset = &rem.ShareClaims[i]
			}
		}
		if subset == nil || !subset.Resolved || !subset.Exact || len(subset.HolderKeys) != 1 || subset.HolderKeys[0] != seat.ShareHolderKey ||
			math.Abs(subset.ShareMs-25.748) > 0.002 {
			t.Fatalf("the remainder's subset claim must resolve to the restriction seat %q: %+v", seat.ShareHolderKey, rem.ShareClaims)
		}
		if !strings.Contains(rem.Summary, "25.748ms of this remainder lies under this thread's CPU affinity/cpuset restriction and is counted on that restriction's own row, not here") {
			t.Fatalf("remainder sentence: %s", rem.Summary)
		}
	})
	t.Run("udk-irq-1-76/emptied remainder rides as the restriction seat's detail", func(t *testing.T) {
		seat := find(76, "cpu_affinity_or_cpuset", false)
		if seat == nil || math.Abs(seat.EffectiveImpactMs-0.539) > 0.002 {
			t.Fatalf("restriction seat: %+v", seat)
		}
		if find(76, "runnable_wait", true) != nil {
			t.Fatal("the remainder whose every instant the restriction seat publishes must leave the board")
		}
		detail := false
		for _, it := range rank.AbsorbedItems {
			if it.Thread.PID == 76 && it.Type == "runnable_wait" && it.ChainAnchorRemainderSeat &&
				strings.HasPrefix(it.AbsorbedIntoFamily, "rank_pair:cpu_affinity_or_cpuset>runnable_wait|pid:76|") {
				detail = true
			}
		}
		if !detail {
			t.Fatal("the emptied remainder must stay lossless as the restriction seat's detail")
		}
	})
}

// A carved share is one the published board counts elsewhere: when the
// restriction seat is not on the board (cap) or the publication gate
// withdraws it, the share returns to the remainder (one carrier).
func TestHMC377R11CarvedShareReturnsWhenItsHolderLeavesTheBoard(t *testing.T) {
	rank := hmc377R11Board(t, hmc377R11DonghuFixture, Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	remIdx, seatIdx := -1, -1
	for i, it := range rank.Items {
		if it.Thread.PID != 9655 {
			continue
		}
		switch {
		case it.Type == "runnable_wait" && it.ChainAnchorRemainderSeat:
			remIdx = i
		case it.Type == "cpu_affinity_or_cpuset":
			seatIdx = i
		}
	}
	if remIdx < 0 || seatIdx < 0 {
		t.Fatalf("witness rows missing: remainder=%d restriction=%d", remIdx, seatIdx)
	}
	if math.Abs(rank.Items[remIdx].RunnableMs-3.684) > 0.002 {
		t.Fatalf("published remainder must leave the restriction seat's 25.748ms to it: %.3f", rank.Items[remIdx].RunnableMs)
	}
	t.Run("cap cut", func(t *testing.T) {
		board := make([]RootCauseRankItem, 0, len(rank.Items)-1)
		for i, it := range rank.Items {
			if i != seatIdx {
				board = append(board, it)
			}
		}
		resolveRankShareClaims(board, nil)
		for _, it := range board {
			if it.Thread.PID == 9655 && it.Type == "runnable_wait" && it.ChainAnchorRemainderSeat {
				if math.Abs(it.RunnableMs-29.432) > 0.002 || math.Abs(it.EffectiveImpactMs-29.432) > 0.002 {
					t.Fatalf("the share returns to the remainder: %.3f/%.3f", it.RunnableMs, it.EffectiveImpactMs)
				}
				if !strings.Contains(it.Summary, "(counted here: that restriction's row is not on the published board)") {
					t.Fatalf("remainder sentence: %s", it.Summary)
				}
				return
			}
		}
		t.Fatal("remainder missing")
	})
	t.Run("gate withdrawal", func(t *testing.T) {
		board := append([]RootCauseRankItem(nil), rank.Items...)
		withdrawn := make([]bool, len(board))
		withdrawn[seatIdx] = true
		WithdrawRankShareHolders(board, nil, withdrawn, nil)
		if got := board[remIdx].RunnableMs; math.Abs(got-29.432) > 0.002 {
			t.Fatalf("the share returns to the remainder: %.3f", got)
		}
		if got := board[seatIdx].RunnableMs; math.Abs(got-27.507) > 0.002 {
			t.Fatalf("a withdrawn seat holds and hands nothing: its own account again, %.3f", got)
		}
	})
}

// Several chain runnable seats of one thread (tieba CookieMonsterCl-59843,
// NetworkService-60595): the whole-account inversion row no longer stands on
// the board beside them — its headline was the whole ready account while the
// chain seats publish the walked share (raw rawpop.py over this window,
// ~/hmc-work/hmc377/fix11/r4c/pins/raw/tb.raw.json: ready 26.738 / 20.342;
// base tree 26.738 + 26.001 and 20.342 + 18.979). The chain seats keep the
// walked share, a plain ◇ remainder twin carries the rest, and the inversion
// row rides as the twin's lossless detail.
func TestHMC377R11SeveralChainSeatsKeepTheWalkedShareAndTheTwinTheRest(t *testing.T) {
	rank := hmc377R11Board(t, "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace", Query{PID: 59566,
		TimeStart: 34579.472865, TimeEnd: 34579.587805, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12})
	for _, tc := range []struct {
		name           string
		pid            int
		ready, walked  float64
		unwalked, rest float64
	}{
		{"CookieMonsterCl-59843", 59843, 26.738, 26.001, 0.208, 0.529},
		{"NetworkService-60595", 60595, 20.342, 18.979, 0, 1.363},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var twin *RootCauseRankItem
			chainRunnable := 0.0
			for i := range rank.Items {
				it := &rank.Items[i]
				if it.Thread.PID != tc.pid {
					continue
				}
				switch {
				case it.Type == "priority_inversion_runnable_wait":
					t.Fatalf("the whole-account inversion row must not stand beside the chain seats: %+v", it)
				case it.Type == "priority_inversion_candidate" && strings.HasPrefix(it.Source, "wakeup_chain"):
					chainRunnable += it.GatedRunnableMs
				case it.Type == "runnable_wait" && it.ChainAnchorRemainderSeat:
					twin = it
				}
			}
			if twin == nil || math.Abs(twin.RunnableMs-tc.rest) > 0.002 || math.Abs(twin.ChainAnchoredMs-tc.walked) > 0.002 ||
				math.Abs(twin.ChainAnchorUnwalkedMs-tc.unwalked) > 0.002 || math.Abs(twin.ChainAnchorFullMs-tc.ready) > 0.002 {
				t.Fatalf("remainder twin: %+v", twin)
			}
			if math.Abs(chainRunnable-tc.walked) > 0.002 {
				t.Fatalf("the chain seats keep the walked share: %.3f", chainRunnable)
			}
			if !strings.Contains(twin.Summary, "the same-CPU lower-priority running overlap") ||
				!strings.Contains(twin.Summary, "that whole-account row rides as this seat's lossless detail") {
				t.Fatalf("the twin must name the overlap measurement its detail carries: %s", twin.Summary)
			}
			detail := false
			for _, it := range rank.AbsorbedItems {
				if it.Thread.PID == tc.pid && it.Type == "priority_inversion_runnable_wait" &&
					it.inversionAccountRepresentedBy != "" && it.inversionAccountRepresentedBy == twin.inversionAccountRepresents &&
					math.Abs(it.RunnableMs-tc.ready) < 0.002 {
					detail = true
				}
			}
			if !detail {
				t.Fatal("the inversion row keeps every value as the twin's lossless detail")
			}
		})
	}
}
