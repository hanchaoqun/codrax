package tracequery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// HMC377 fix round 2, item F3, on the complete original captures (opt-in:
// CODRAX_HMC377_HONEYBIRD_REBOOT, CODRAX_HMC377_DOUYIN_REBOOT). Expected values
// come from an independent raw parse (sched_switch / sched_wakeup state
// machine, integer microseconds; the analysed thread's waits and the wakeup
// row that ended each one rebuilt from the raw rows; the walked chain windows
// are the chain's own): every chain member's ready account = its time inside
// the walked chain windows + its time inside the analysed thread's waits its
// own wakeup ended that the chain did not walk + the rest.
//
//	Honeybird reboot [162.372184, 163.616645) TID 10566:
//	  launcher-4-10926  37.246 = 8.279 walked + 5.898 unwalked (last ended 163.397032) + 23.069
//	  Thread-5-9673     36.321 = 28.601 walked + 1.829 unwalked + 5.891
//	Douyin reboot5 [140.911304, 141.835458) TID 11105:
//	  udk-irq-7-76       6.606 = 0.056 walked + 3.857 unwalked + 2.693 (no chain-lane seat: one ⛓ seat of 3.913)
//	  #tp-io-21-11317   94.225 = 1.589 walked + 0.042 unwalked + 92.594
func hmc377F3Original(t *testing.T, variable, digest string, size int64) string {
	t.Helper()
	path := os.Getenv(variable)
	if path == "" {
		t.Skipf("requires the complete original capture in %s", variable)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.HasPrefix(got, digest) || n != size {
		t.Fatalf("%s is not the original capture: sha256=%s bytes=%d", variable, got, n)
	}
	return path
}

func hmc377F3Board(t *testing.T, path string, pid int, start, end float64) (RootCauseRankResult, ChainResult) {
	t.Helper()
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{TimeStart: start, TimeEnd: end, TimeStartSet: true, TimeEndSet: true,
		TimePaddingBefore: 0.5, TimePaddingAfter: 0.5, LinePaddingBefore: 200, LinePaddingAfter: 200, AllowWindowedParse: true,
		MaxEvents: (512 << 20) / 2048, ScopePID: pid})
	if err != nil {
		t.Fatal(err)
	}
	q := Query{PID: pid, TimeStart: start, TimeEnd: end, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	nq := ensureQueryFlavor(idx, normalizeQuery(idx, q))
	chain := buildWakeupChainWithCache(idx, nq, newChainQueryCache(idx, nq.runCancel))
	return BuildRootCauseRank(idx, q), chain
}

type hmc377F3Want struct {
	pid                       int
	walked, unwalked, outside float64
	lastWakeup                float64 // the unwalked seat's closing wakeup (0: not pinned)
	oneSeat                   bool    // no chain-lane seat: the window seat carries walked + unwalked
}

// hmc377F3Assert — HMC377 fix round 3: every part has exactly one owner and
// the records carry the three parts typed (walked on the chain seat, unwalked
// on the thread's own seat, outside on the remainder; case B: one clipped
// seat holds walked + unwalked).
func hmc377F3Assert(t *testing.T, rank RootCauseRankResult, chain ChainResult, wants []hmc377F3Want) {
	t.Helper()
	near := func(a, b float64) bool { return a-b <= 0.0015 && b-a <= 0.0015 }
	pool := append(append([]RootCauseRankItem(nil), rank.Items...), rank.preTruncationItems...)
	for _, w := range wants {
		var remainder, inside, clipped *RootCauseRankItem
		owners := map[string]bool{}
		for i := range pool {
			item := &pool[i]
			if item.Thread.PID != w.pid || item.DominantState != string(StateRunnable) || !strings.HasPrefix(item.Source, "window_stats") {
				continue
			}
			switch {
			case item.ChainAnchorRemainderSeat && item.Type == "runnable_wait":
				remainder = item
			case item.ChainAnchorUnwalkedSeat && item.Type == "runnable_wait":
				inside = item
				owners[fmt.Sprintf("%.6f", item.RunnableMs)] = true
			case item.ChainAnchorFullMs > 0 && item.Type == "runnable_wait" && rootCauseItemIsOnChain(*item):
				clipped = item
			}
		}
		full := w.walked + w.unwalked + w.outside
		if remainder == nil || !near(remainder.RunnableMs, w.outside) || !near(remainder.ChainAnchorFullMs, full) {
			t.Fatalf("pid %d: the remainder must be the raw %.3fms outside every dependency window of %.3f: %+v", w.pid, w.outside, full, remainder)
		}
		if w.oneSeat {
			if clipped == nil || !near(clipped.RunnableMs, w.walked+w.unwalked) || !near(clipped.rspaUnwalkedHeldMs, w.unwalked) ||
				!near(remainder.ChainAnchoredMs, w.walked+w.unwalked) || inside != nil || remainder.ChainAnchorUnwalkedMs != 0 {
				t.Fatalf("pid %d: one on-chain seat carries walked %.3f + unwalked %.3f: clipped=%+v inside=%+v", w.pid, w.walked, w.unwalked, clipped, inside)
			}
			continue
		}
		if !near(remainder.ChainAnchoredMs, w.walked) || !near(remainder.ChainAnchorUnwalkedMs, w.unwalked) || !near(remainder.ChainAnchorOutsideMs, w.outside) {
			t.Fatalf("pid %d: the remainder carries the three raw parts typed (%.3f walked + %.3f unwalked + %.3f outside): %+v", w.pid, w.walked, w.unwalked, w.outside, *remainder)
		}
		if inside == nil || len(owners) != 1 || !near(inside.RunnableMs, w.unwalked) || inside.HostWakeupEdgeAnchorTs <= 0 ||
			(w.lastWakeup > 0 && !near(inside.HostWakeupEdgeAnchorTs, w.lastWakeup)) || !rootCauseItemIsOnChain(*inside) {
			t.Fatalf("pid %d: exactly one on-chain seat holds the raw %.3fms inside the unwalked waits its wakeup ended (ended %.6f): %+v", w.pid, w.unwalked, w.lastWakeup, inside)
		}
	}
	// Structural, every chain member: a ready-time remainder either holds
	// nothing inside the walked windows or a wait of the analysed thread that
	// the thread's own wakeup ended, or says it keeps time inside those waits
	// (outside the WALKED windows only) and carries no three-way parts; and
	// the thread's on-chain ready time never exceeds its raw inventory inside
	// those windows (every instant priced at most once).
	walked := chainAnchorWindowsByPID(chain)
	onChain := map[int]float64{}
	for _, item := range rank.Items {
		if !rootCauseItemIsOnChain(item) || item.Thread.PID == chain.Target.PID {
			continue
		}
		switch {
		case strings.TrimSpace(item.Type) == "priority_inversion_candidate":
			onChain[item.Thread.PID] += item.GatedRunnableMs
		case item.DominantState == string(StateRunnable) && !rootCauseTypeIsSemanticSpanWork(item.Type) && strings.TrimSpace(item.OnChainBasis) != RootCauseOnChainBasisHostWakeupEdge:
			onChain[item.Thread.PID] += item.RunnableMs
		}
	}
	for _, item := range pool {
		if !item.ChainAnchorRemainderSeat || item.Type != "runnable_wait" || item.HostWakeupEdgeAnchorTs != 0 || len(walked[item.Thread.PID]) == 0 {
			continue
		}
		var cut []foldInterval
		for _, w := range walked[item.Thread.PID] {
			cut = append(cut, foldInterval{start: w.StartTs, end: w.EndTs})
		}
		for _, w := range hostDirectDependencyWindows(&chain, item.Thread) {
			cut = append(cut, foldInterval{start: w.startTs, end: w.endTs})
		}
		cut, _ = foldIntervalUnionWithDisjoint(cut)
		merged, _ := foldIntervalUnionWithDisjoint(item.runnableIntervals)
		outside, _ := foldIntervalsLengthMs(foldIntervalSubtract(merged, cut))
		total, _ := foldIntervalsLengthMs(merged)
		if !near(total, item.ChainAnchorFullMs) {
			continue // a seat whose inventory is not its account keeps the chain windows' partition (fail closed)
		}
		if !near(outside, item.RunnableMs) &&
			(item.ChainAnchorUnwalkedMs != 0 || !strings.Contains(item.Summary, "outside the dependency windows the chain walked for this thread")) {
			t.Fatalf("%s: remainder %.3fms but only %.3fms of its account lies outside its dependency windows: %s", threadLabel(item.Thread), item.RunnableMs, outside, item.Summary)
		}
		if onChain[item.Thread.PID] > total-outside+0.0015 {
			t.Fatalf("%s: %.3fms of on-chain ready time for %.3fms of raw ready time inside its dependency windows", threadLabel(item.Thread), onChain[item.Thread.PID], total-outside)
		}
	}
}

func TestHMC377WholeOriginalChainMembersArePricedInsideEveryWaitTheirWakeupEnded(t *testing.T) {
	t.Run("honeybird reboot", func(t *testing.T) {
		path := hmc377F3Original(t, "CODRAX_HMC377_HONEYBIRD_REBOOT", "54405241", 124269104)
		rank, chain := hmc377F3Board(t, path, 10566, 162.372184, 163.616645)
		hmc377F3Assert(t, rank, chain, []hmc377F3Want{
			{pid: 10926, walked: 8.279, unwalked: 5.898, outside: 23.069, lastWakeup: 163.397032},
			{pid: 9673, walked: 28.601, unwalked: 1.829, outside: 5.891, lastWakeup: 162.750755},
		})
	})
	t.Run("douyin reboot5", func(t *testing.T) {
		path := hmc377F3Original(t, "CODRAX_HMC377_DOUYIN_REBOOT", "8087138a", 138438125)
		rank, chain := hmc377F3Board(t, path, 11105, 140.911304, 141.835458)
		hmc377F3Assert(t, rank, chain, []hmc377F3Want{
			{pid: 76, walked: 0.056, unwalked: 3.857, outside: 2.693, oneSeat: true},
			{pid: 11317, walked: 1.589, unwalked: 0.042, outside: 92.594, lastWakeup: 0},
		})
	})
}
