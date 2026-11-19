package tracequery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func hmc377F3Assert(t *testing.T, rank RootCauseRankResult, chain ChainResult, wants []hmc377F3Want) {
	t.Helper()
	near := func(a, b float64) bool { return a-b <= 0.0015 && b-a <= 0.0015 }
	pool := append(append([]RootCauseRankItem(nil), rank.Items...), rank.preTruncationItems...)
	for _, w := range wants {
		var remainder, inside, clipped *RootCauseRankItem
		for i := range pool {
			item := &pool[i]
			if item.Thread.PID != w.pid || item.DominantState != string(StateRunnable) || !strings.HasPrefix(item.Source, "window_stats") {
				continue
			}
			switch {
			case item.ChainAnchorRemainderSeat && item.Type == "runnable_wait":
				remainder = item
			case item.OnChainBasis == RootCauseOnChainBasisHostWakeupEdgeState && item.Type == "runnable_wait":
				inside = item
			case item.ChainAnchorFullMs > 0 && item.Type == "runnable_wait" && rootCauseItemIsOnChain(*item):
				clipped = item
			}
		}
		full := w.walked + w.unwalked + w.outside
		if remainder == nil || !near(remainder.RunnableMs, w.outside) || !near(remainder.ChainAnchoredMs, w.walked+w.unwalked) || !near(remainder.ChainAnchorFullMs, full) {
			t.Fatalf("pid %d: the remainder must be the raw %.3fms outside every dependency window (anchored %.3f of %.3f): %+v", w.pid, w.outside, w.walked+w.unwalked, full, remainder)
		}
		if w.oneSeat {
			if clipped == nil || !near(clipped.RunnableMs, w.walked+w.unwalked) || inside != nil {
				t.Fatalf("pid %d: one on-chain seat carries walked + unwalked %.3fms: clipped=%+v inside=%+v", w.pid, w.walked+w.unwalked, clipped, inside)
			}
		} else if inside == nil || !near(inside.RunnableMs, w.unwalked) || inside.HostWakeupEdgeAnchorTs <= 0 ||
			(w.lastWakeup > 0 && !near(inside.HostWakeupEdgeAnchorTs, w.lastWakeup)) || !rootCauseItemIsOnChain(*inside) {
			t.Fatalf("pid %d: the raw %.3fms inside the unwalked waits its wakeup ended is priced on the chain (ended %.6f): %+v", w.pid, w.unwalked, w.lastWakeup, inside)
		}
	}
	// Structural, every chain member: a ready-time remainder never holds time
	// inside the walked chain windows or inside a wait of the analysed thread
	// that the thread's own wakeup ended.
	walked := chainAnchorWindowsByPID(chain)
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
		if total, _ := foldIntervalsLengthMs(merged); !near(total, item.ChainAnchorFullMs) {
			continue // a seat whose inventory is not its account keeps the chain windows' partition (fail closed)
		}
		if !near(outside, item.RunnableMs) {
			t.Fatalf("%s: remainder %.3fms but only %.3fms of its account lies outside its dependency windows", threadLabel(item.Thread), item.RunnableMs, outside)
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
