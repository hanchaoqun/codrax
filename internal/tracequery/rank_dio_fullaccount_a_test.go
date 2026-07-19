package tracequery

// rank_dio_fullaccount_a_test.go — 修复轮二 件A engine pins (v5 P1 批,
// 2026-07-13; 冷读核账① tieba Δ2.818 witness): the formal D/IO family seats
// mint from the FULL pre-cap census (全量账铸席, ENG-1 runnableCensus 补完),
// while the top-8 board face stays capped and DISCLOSES its per-lane
// overflow (groups + total ms).
//
// EVOLUTION RECORD (Σ 分区恒等 pin 迁全量基): the §29.50.5 partition-sum
// identity (cause seats + remainder == the thread account) previously held
// on the CAPPED basis by construction (members = top-8 rows; tieba witness:
// 15.317 seat basis vs 18.135 window truth, Δ2.818 evicted). It now holds on
// the FULL census basis — the tieba shape becomes 18.135 = fscache 7.386 +
// hmfs_get_dnode 0.171 + hmfs_read 0.145 + 余数 10.433 (see the skip-gated
// witness pin below); the synthetic cap-pressure pin enforces the identity
// hermetically.
//
// MUTATION self-checks:
//   - reverting the family mint to the capped lists (stats.DStateTop) reds
//     TestDIOFullAccountSeatUnderCapPressure (seat 31.0→30.0, member 2→1);
//   - dropping the overflow disclosure reds the same test's caveat/field
//     assertions;
//   - breaking the shared-comparator parity (census mint ordered differently
//     from the display list) reds TestDIOFullAccountNoOverflowParity's
//     member-SEQUENCE assertion (修复轮三 F2 勘正: the sum alone is
//     order-blind — the roster/interval sequence is the user-visible wire
//     order, so the pin asserts the per-member (interval,value) sequence
//     equals the display list's own order).

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
)

// dioCapPressureTrace mints fillers+worker D episodes back-to-back: 8 filler
// threads (20..27ms, one group each) own most of the top-8 D board; worker-200
// holds one large group (30ms, in-top) and one tiny group (1ms, evicted along
// with the smallest filler).
func dioCapPressureTrace(fillers int) string {
	var b strings.Builder
	ts := 3.000000
	row := func(format string, a ...interface{}) {
		fmt.Fprintf(&b, format+"\n", a...)
	}
	row("        app-100 (100) [000] .... %.6f: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120", ts)
	episode := func(comm string, pid, cpu int, durMs float64) {
		ts += 0.001
		row("     %s-%d (%d) [%03d] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=%s next_pid=%d next_prio=20", comm, pid, pid, cpu, ts, comm, pid)
		ts += 0.001
		row("     %s-%d (%d) [%03d] .... %.6f: sched_switch: prev_comm=%s prev_pid=%d prev_prio=20 prev_state=D ==> next_comm=idle next_pid=0 next_prio=120", comm, pid, pid, cpu, ts, comm, pid)
		ts += durMs / 1000
		row("       waker-999 (999) [009] .... %.6f: sched_wakeup: comm=%s pid=%d prio=20 target_cpu=%03d", ts, comm, pid, cpu)
		ts += 0.0005
		row("     %s-%d (%d) [%03d] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=%s next_pid=%d next_prio=20", comm, pid, pid, cpu, ts, comm, pid)
		ts += 0.0005
		row("     %s-%d (%d) [%03d] .... %.6f: sched_switch: prev_comm=%s prev_pid=%d prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120", comm, pid, pid, cpu, ts, comm, pid)
	}
	for i := 0; i < fillers; i++ {
		episode("filler", 901+i, 1+i, 20.0+float64(i))
	}
	episode("worker", 200, 3, 30.0)
	episode("worker", 200, 9, 1.0)
	ts += 0.001
	row("        app-100 (100) [000] .... %.6f: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52", ts)
	return b.String()
}

func dioFullAccountStats(t *testing.T, fillers int) WindowStats {
	t.Helper()
	idx := buildTraceIndex(t, "dio_fullaccount.systrace", dioCapPressureTrace(fillers))
	return ComputeWindowStats(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.5})
}

func dioFullAccountWorkerSeat(t *testing.T, stats WindowStats) RootCauseRankItem {
	t.Helper()
	items := rootCauseDIOStateFamilyItems(Query{TimeStart: 3.0, TimeEnd: 3.5}, stats, nil, false, true)
	for _, item := range items {
		if item.Thread.PID == 200 {
			return item
		}
	}
	t.Fatalf("no worker-200 D/IO seat minted: %+v", items)
	return RootCauseRankItem{}
}

// TestDIOFullAccountSeatUnderCapPressure — 10 census groups vs the top-8
// board: the two smallest groups (filler 20ms + worker 1ms) sit beyond the
// cap; the worker seat must still carry its FULL account (30+1), and the
// board face must disclose the eviction per lane.
func TestDIOFullAccountSeatUnderCapPressure(t *testing.T) {
	stats := dioFullAccountStats(t, 8)
	if len(stats.DStateTop) != 8 || len(stats.dstateCensus) != 10 {
		t.Fatalf("fixture drifted: top=%d census=%d", len(stats.DStateTop), len(stats.dstateCensus))
	}
	if stats.DStateTopOverflowGroups != 2 {
		t.Fatalf("overflow disclosure must count the two evicted groups, got %d", stats.DStateTopOverflowGroups)
	}
	if math.Abs(stats.DStateTopOverflowMs-21.0) > 1e-6 {
		t.Fatalf("overflow total must sum the evicted groups' own floats (20+1), got %.6f", stats.DStateTopOverflowMs)
	}
	caveatSeen := false
	for _, cav := range stats.Caveats {
		if strings.Contains(cav, "top_d_state shows 8 of 10") && strings.Contains(cav, "2 group(s)") {
			caveatSeen = true
		}
	}
	if !caveatSeen {
		t.Fatalf("the per-lane cap-overflow caveat must disclose honestly: %v", stats.Caveats)
	}
	seat := dioFullAccountWorkerSeat(t, stats)
	if math.Abs(seat.CumulativeImpactMs-31.0) > 1e-6 {
		t.Fatalf("the family seat must carry the FULL census account (30+1), got %.6f", seat.CumulativeImpactMs)
	}
	if seat.MemberCount != 2 {
		t.Fatalf("the evicted group must still be a seat member, got member_count=%d", seat.MemberCount)
	}
	// Σ 全量基恒等: the seat account equals the census sum for the thread.
	censusSum := 0.0
	for _, td := range stats.dstateCensus {
		if td.Thread.PID == 200 {
			censusSum += td.DurationMs
		}
	}
	if math.Abs(seat.CumulativeImpactMs-censusSum) > 1e-9 {
		t.Fatalf("seat/census single-source identity drifted: seat=%.9f census=%.9f", seat.CumulativeImpactMs, censusSum)
	}
}

// TestDIOFullAccountNoOverflowParity — with ≤8 groups nothing is evicted: the
// overflow face stays silent and the seat members are EXACTLY the display
// list's rows in the display list's own order (shared comparator = byte
// parity with the pre-census mint).
func TestDIOFullAccountNoOverflowParity(t *testing.T) {
	stats := dioFullAccountStats(t, 3) // 3 fillers + 2 worker groups = 5 ≤ 8
	if stats.DStateTopOverflowGroups != 0 || stats.DStateTopOverflowMs != 0 {
		t.Fatalf("no eviction must disclose nothing, got %d/%.3f", stats.DStateTopOverflowGroups, stats.DStateTopOverflowMs)
	}
	for _, cav := range stats.Caveats {
		if strings.Contains(cav, "top_d_state shows") {
			t.Fatalf("no eviction must mint no cap caveat: %q", cav)
		}
	}
	seat := dioFullAccountWorkerSeat(t, stats)
	fromTop := 0.0
	var wantSeq []foldInterval
	for _, td := range stats.DStateTop {
		if td.Thread.PID == 200 {
			fromTop += td.DurationMs
			wantSeq = append(wantSeq, foldInterval{start: td.StartTs, end: td.EndTs, valueMs: td.DurationMs})
		}
	}
	if math.Abs(seat.CumulativeImpactMs-fromTop) > 1e-9 {
		t.Fatalf("no-overflow seat must equal the capped-basis account byte-for-byte, got %.9f vs %.9f", seat.CumulativeImpactMs, fromTop)
	}
	// 修复轮三 F2: the member SEQUENCE is the parity claim's real substance —
	// per-member (interval, value) in the display list's own order (a sum
	// assertion alone is order-blind; the roster is user-visible wire order).
	if len(seat.familyMemberIntervals) != len(wantSeq) {
		t.Fatalf("member sequence length drifted: got %d want %d", len(seat.familyMemberIntervals), len(wantSeq))
	}
	for k, iv := range seat.familyMemberIntervals {
		if iv != wantSeq[k] {
			t.Fatalf("member %d drifted from the display order: got %+v want %+v", k, iv, wantSeq[k])
		}
	}
}

// TestDIOFullAccountTiebaWitness — the 冷读核账① witness (skip-gated on the
// out-of-repo customer trace): pid=60555 window partition on the FULL basis
// 18.135 = fscache 7.386 + hmfs_get_dnode 0.171 + hmfs_read 0.145 + 余数
// 10.433 (previously 15.317 capped basis, Δ2.818 evicted).
//
// EVOLUTION RECORD (RSPA §29.61.10, 2026-07-14): the per-cause sums below now
// span the re-anchoring bipartition — a migrated cause seat may publish as
// ⛓ anchored + ◇ remainder halves, and the Σ-per-cause pin holds BY the
// bipartition identity (同源二分可相加还原; anchored + remainder == the full
// cause account exactly). The pinned values are therefore unchanged whether
// or not the seat migrated on this capture; the added sweep asserts the
// identity on any decomposed row.
func TestDIOFullAccountTiebaWitness(t *testing.T) {
	const trace = "/Users/han/opt/customlogs/xxx_all.systrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("customer witness trace not present: %v", err)
	}
	idx, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	bundle := BuildFrameRootCauseBundle(idx, Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805})
	want := map[string]float64{
		"fscache_page_wait_o": 7.386,
		"hmfs_get_dnode":      0.171,
		"hmfs_read":           0.145,
		"":                    10.433,
	}
	got := map[string]float64{}
	sum := 0.0
	for _, item := range bundle.RootCauseRank.Items {
		if item.Thread.PID == 60555 && (item.Type == "d_state_or_io_wait" || item.Type == "io_wait") &&
			strings.HasPrefix(item.Source, "window_stats") {
			got[item.BlockedReasonCaller] += item.CumulativeImpactMs
			sum += item.CumulativeImpactMs
			// RSPA bipartition identity on any decomposed half.
			if item.ChainAnchorFullMs > 0 && item.ChainAnchorRemainderSeat {
				if math.Abs(item.ChainAnchoredMs+item.CumulativeImpactMs-item.ChainAnchorFullMs) > 0.002 {
					t.Fatalf("RSPA ◇ identity broken on cause %q: %+v", item.BlockedReasonCaller, item)
				}
			}
		}
	}
	// EVOLUTION RECORD (ONCHAIN-3c, 2026-07-19): the bare-census-edge
	// state-seat arm seats T7@ZeusThreadPo-61839 (SCAN-3 sentinel host) on
	// the chain tier (io_wait 3.550 + runnable 0.370⛓/0.075◇), and the
	// candidate cap then displaces the zero-anchored hmfs dust seats
	// (Σ 0.316, adjacent) off the PUBLISHED board. Their accounts survive
	// losslessly in the pre-truncation pool — the partition value/Σ pins now
	// read published-first with a pool fallback (population conservation;
	// first occurrence wins, the pool is the build+enrich union).
	for _, item := range bundle.RootCauseRank.preTruncationItems {
		if item.Thread.PID != 60555 || (item.Type != "d_state_or_io_wait" && item.Type != "io_wait") ||
			!strings.HasPrefix(item.Source, "window_stats") {
			continue
		}
		if _, seen := got[item.BlockedReasonCaller]; seen {
			continue
		}
		got[item.BlockedReasonCaller] = item.CumulativeImpactMs
		sum += item.CumulativeImpactMs
	}
	for cause, ms := range want {
		if math.Abs(got[cause]-ms) > 0.001 {
			t.Fatalf("tieba 60555 %q seat must carry the full account %.3f, got %.3f (all=%v)", cause, ms, got[cause], got)
		}
	}
	if math.Abs(sum-18.135) > 0.001 {
		t.Fatalf("partition Σ must equal the full window truth 18.135, got %.3f", sum)
	}
}
