package tracequery

// wakeup_edge_census.go — WAKE-CENSUS (立案 §29.58, 实施 2026-07-13, ledger
// docs/design/real_trace_campaign_20260705.md) + WAKE-CENSUS-D 2A 换源
// (§29.58.4 立案, RANK-U Stage 1 commit B, 2026-07-13): the per-(waker →
// wakee) wakeup census, minted by the ENGINE as a WINDOW-TOTAL direct count
// of raw sched_wakeup rows for the chain-thread wakee set.
//
// 病根 lineage:
//
//   - PRC-F1 (§29.58 首批): the publication layer bounds per-edge observation
//     rows with a typed row cap, so a consumer re-counting the published rows
//     holds a silently TRUNCATED inventory — and the model face invented
//     counts for pairs it never saw. First fix: fold the FULL pre-cap edge
//     set (禁截断库存二次聚合).
//   - §29.58.4 (WAKE-CENSUS-D): the edge set itself is a STRUCTURALLY partial
//     supply — expandChain mints edges only on the `case StateSSleep:` arm
//     (the D/IO arm reads sched_blocked_reason, never findWakeup), so every
//     D-exit wakeup was invisible to the census (donghu witness: 29 raw
//     in-window sched_wakeup rows waking the target, 12 of them D exits all
//     by gpu-token-id4-2931 — the engine's 28 edges carried ZERO gpu-token
//     pairs), and S-exit wakeups on segments the branch expansion never
//     visited leaked the same way. 2A 换源 (裁定 2026-07-13): the census
//     counts raw sched_wakeup rows DIRECTLY from the indexed inventory —
//     chain-independent counting over a chain-bounded wakee population — so
//     its zero is a real window-total zero. res.Edges is untouched (拒铸边
//     方案存档 §3.1: no D-edge minting, no waker recursion — census counting
//     is measurement-face arithmetic, blocked_reason keeps the D causal lane,
//     via/rank/tree/provenance blast radius stays zero).
//
// Census discipline (blocked_reason_census precedent, §29.57 件1):
//
//   - counts fold over the FULL raw inventory, never over a capped/truncated
//     view (禁截断库存二次聚合 — hit twice in this campaign window);
//   - the wire face stays bounded: a pair cap plus EXPLICIT overflow counts;
//   - the order is deterministic: count desc, then typed tie keys
//     (waker comm, waker pid, wakee comm, wakee pid — DET 纪律, no map
//     iteration order ever reaches the wire).
//
// EVOLUTION RECORD (2A 换源): the §29.62 physical-row dedup arm (same pair +
// wakeup ts + line observed by two branch expansions) is RETIRED — the raw
// inventory is scanned once with per-event-index dedup, so one physical row
// is one observation by construction.

import (
	"fmt"
	"sort"
	"strings"
)

// wakeupEdgeCensusPairCap bounds the census wire face (the COUNTS are always
// whole-inventory; only the listed pair rows are capped). Mirrors the
// blocked_reason census pid cap.
const wakeupEdgeCensusPairCap = 16

// wakeupEdgeCensusKey is the pair fold identity: both endpoints by
// (comm, pid) — the same identity the edge rows publish.
type wakeupEdgeCensusKey struct {
	wakerComm string
	wakerPID  int
	wakeeComm string
	wakeePID  int
}

// wakeupExitStateBucket classifies which scheduler state a wakee LEFT when a
// raw sched_wakeup row hit it, from the wakee's own timeline (§29.58.4 裁定点
// b answered on the measurement face: the exit split is a typed CLASSIFIER
// column on the census pair, never a new edge kind — res.Edges untouched).
// The three buckets partition every counted row exactly (sleep + d + other ==
// count, the 恰N 双加恒等式), with the honest unclassified arm folded into
// other (timeline missing/gapped → absence never guesses).
func wakeupExitStateBucket(intervals []Interval, ts float64) string {
	// The exited state is the interval CONTAINING ts with StartTs strictly
	// before it (a segment starting exactly AT ts is the post-wake state);
	// when ts sits in a gap, the nearest PRECEDING interval speaks.
	best := -1
	for i := range intervals {
		if intervals[i].EndTs <= intervals[i].StartTs {
			continue
		}
		if intervals[i].StartTs < ts && ts <= intervals[i].EndTs {
			best = i
			break
		}
		if intervals[i].EndTs <= ts && (best < 0 || intervals[i].EndTs > intervals[best].EndTs) {
			best = i
		}
	}
	if best < 0 {
		return "other"
	}
	switch intervals[best].State {
	case StateSSleep:
		return "sleep"
	case StateDSleep, StateIOWait:
		return "d"
	default:
		return "other"
	}
}

// buildWakeupEdgeCensus counts raw wakeup rows for the chain-thread wakee set
// (WAKE-CENSUS-D 2A): wakee population = {res.Target} ∪ {res.Nodes[*].Thread}
// — the exact subject set of the waker-question family, chain-BOUNDED for the
// population but chain-INDEPENDENT for the counting (no edge minting, no
// expansion-path dependence; D exits and off-path S exits count alike).
//
// Source rows: EventSchedWakeup in the res.Window time range (incarnation
// starts excluded — sched_wakeup_new creates a task, it does not wake one).
// A trace publishing ONLY sched_waking rows falls back to counting those
// (single ruler per result, disclosed via a caveat) so the census can never
// claim window-total zero beside a sched_waking-minted edge.
//
// Returns the capped rows plus the explicit overflow accounting; target-wakee
// pairs are immune to the pair cap (件5 同款: the question's own thread never
// falls to a cap).
func buildWakeupEdgeCensus(idx *Index, cache *chainQueryCache, q Query, res *ChainResult, pairCap int) (rows []WakeupEdgeCensusPair, overflowPairs, overflowEdges int, sourceCaveat string) {
	if idx == nil || res == nil {
		return nil, 0, 0, ""
	}
	// --- typed wakee population (裁定点 ③): target ∪ chain node threads ----
	population := make([]ThreadRef, 0, len(res.Nodes)+1)
	seenWakee := map[string]bool{}
	addWakee := func(thread ThreadRef) {
		if thread.PID <= 0 && strings.TrimSpace(thread.Comm) == "" {
			return
		}
		key := threadKey(thread)
		if seenWakee[key] {
			return
		}
		seenWakee[key] = true
		population = append(population, thread)
	}
	addWakee(res.Target)
	for _, node := range res.Nodes {
		addWakee(node.Thread)
	}
	if len(population) == 0 {
		return nil, 0, 0, ""
	}
	inWindow := func(ts float64) bool {
		if res.Window.EndTs > res.Window.StartTs {
			return ts >= res.Window.StartTs && ts <= res.Window.EndTs
		}
		return true
	}
	// --- raw scan (pre-cap 全量折叠; per-event-index dedup) ----------------
	fold := map[wakeupEdgeCensusKey]*WakeupEdgeCensusPair{}
	var order []wakeupEdgeCensusKey
	timelines := map[string][]Interval{}
	timelineFor := func(wakee ThreadRef) []Interval {
		key := threadKey(wakee)
		if cached, ok := timelines[key]; ok {
			return cached
		}
		var intervals []Interval
		if cache != nil {
			tq := q
			tq.PID = wakee.PID
			tq.Thread = wakee.Comm
			tq.TimeStart, tq.TimeEnd = res.Window.StartTs, res.Window.EndTs
			tl := cache.timeline(tq, wakee)
			if tl.IntegrityFailure == "" {
				intervals = tl.Intervals
			}
		}
		timelines[key] = intervals
		return intervals
	}
	countRows := func(eventType EventType) int {
		counted := 0
		seenEvent := map[int]bool{}
		for _, wakee := range population {
			visit := func(i int) {
				if i < 0 || i >= len(idx.Events) || seenEvent[i] {
					return
				}
				ev := &idx.Events[i]
				if ev.Type != eventType || schedWakeupStartsNewIncarnation(*ev) {
					return
				}
				if !inWindow(ev.Ts) || !threadMatches(wakee, ev.WakeePID, ev.WakeeComm) {
					return
				}
				seenEvent[i] = true
				counted++
				key := wakeupEdgeCensusKey{
					wakerComm: ev.Comm, wakerPID: ev.PID,
					wakeeComm: ev.WakeeComm, wakeePID: ev.WakeePID,
				}
				if key.wakeeComm == "" && key.wakeePID == 0 {
					// The raw row may carry a pid-only wakee; keep the
					// population thread's identity so the pair stays nameable.
					key.wakeeComm, key.wakeePID = wakee.Comm, wakee.PID
				}
				pair, ok := fold[key]
				if !ok {
					// The wakee identity is the raw row's own (comm,pid) with
					// the matched population thread's TGID backfilled — the raw
					// sched_wakeup row carries no wakee tgid, while the chain
					// population resolved it (identity fidelity kept from the
					// pre-2A edge-fold face).
					wakeeRef := ThreadRef{Comm: key.wakeeComm, PID: key.wakeePID, TGID: wakee.TGID}
					pair = &WakeupEdgeCensusPair{
						Waker:   ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID},
						Wakee:   wakeeRef,
						FirstTs: ev.Ts, LastTs: ev.Ts,
					}
					fold[key] = pair
					order = append(order, key)
				}
				pair.Count++
				switch wakeupExitStateBucket(timelineFor(wakee), ev.Ts) {
				case "sleep":
					pair.SleepExitCount++
				case "d":
					pair.DExitCount++
				default:
					pair.OtherExitCount++
				}
				if ev.Ts < pair.FirstTs {
					pair.FirstTs = ev.Ts
				}
				if ev.Ts > pair.LastTs {
					pair.LastTs = ev.Ts
				}
			}
			if cache != nil && wakee.PID > 0 {
				ids, indexed := cache.wakeupsForThread(wakee)
				if indexed {
					for _, id := range ids {
						visit(id)
					}
					continue
				}
			}
			for i := range idx.Events {
				visit(i)
			}
		}
		return counted
	}
	if countRows(EventSchedWakeup) == 0 {
		// Typed single-ruler fallback: a sched_waking-only trace still mints
		// edges (findWakeup accepts both event types), so a sched_wakeup-only
		// census would claim window-total zero beside real edges. One source
		// per result, disclosed.
		if countRows(EventSchedWaking) > 0 {
			sourceCaveat = "wakeup_edge_census_source=sched_waking; the selected window publishes no sched_wakeup rows for the counted wakee set, so the census counts raw sched_waking rows instead (single source for this result)"
		}
	}
	rows = make([]WakeupEdgeCensusPair, 0, len(order))
	for _, key := range order {
		rows = append(rows, *fold[key])
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		if rows[i].Waker.Comm != rows[j].Waker.Comm {
			return rows[i].Waker.Comm < rows[j].Waker.Comm
		}
		if rows[i].Waker.PID != rows[j].Waker.PID {
			return rows[i].Waker.PID < rows[j].Waker.PID
		}
		if rows[i].Wakee.Comm != rows[j].Wakee.Comm {
			return rows[i].Wakee.Comm < rows[j].Wakee.Comm
		}
		return rows[i].Wakee.PID < rows[j].Wakee.PID
	})
	if pairCap > 0 && len(rows) > pairCap {
		// Target-wakee immunity (件5 同款纪律: the question's own thread never
		// falls to a cap): pairs that wake the chain TARGET are the answer to
		// the waker question, yet as ×1 ties with late lexicographic keys they
		// are exactly the rows a blind trim evicts first (donghu witness: the
		// direct tppmgr-idle/hilogcat/binder wakers of CompThread_0-2955 fell
		// to overflow while chain-intermediate pairs stayed listed). Selection:
		// target-wakee rows survive unconditionally — ALL of them — and the
		// remaining seats fill in the deterministic order above. Kept rows
		// preserve that order; evicted rows fold into the explicit overflow.
		target := res.Target
		isTarget := func(row WakeupEdgeCensusPair) bool {
			if target.PID > 0 {
				return row.Wakee.PID == target.PID
			}
			return target.Comm != "" && row.Wakee.Comm == target.Comm
		}
		targetRows := 0
		for _, row := range rows {
			if isTarget(row) {
				targetRows++
			}
		}
		seats := pairCap
		if targetRows > seats {
			seats = targetRows
		}
		kept := make([]WakeupEdgeCensusPair, 0, seats)
		fillSeats := seats - targetRows
		for _, row := range rows {
			if isTarget(row) {
				kept = append(kept, row)
				continue
			}
			if fillSeats > 0 {
				kept = append(kept, row)
				fillSeats--
				continue
			}
			overflowPairs++
			overflowEdges += row.Count
		}
		rows = kept
	}
	return rows, overflowPairs, overflowEdges, sourceCaveat
}

// attachWakeupEdgeCensus mints the census onto a finished chain result. The
// call sites sit AFTER every expansion pass so the node POPULATION is final;
// the counting itself is window-total raw-inventory arithmetic and never
// reads res.Edges (WAKE-CENSUS-D 2A — a truncated or structurally partial
// edge set can no longer under-supply the counts).
func attachWakeupEdgeCensus(idx *Index, cache *chainQueryCache, q Query, res *ChainResult) {
	var caveat string
	res.WakeupEdgeCensus, res.WakeupEdgeCensusOverflowPairs, res.WakeupEdgeCensusOverflowEdges, caveat =
		buildWakeupEdgeCensus(idx, cache, q, res, wakeupEdgeCensusPairCap)
	if caveat != "" {
		res.Caveats = append(res.Caveats, caveat)
	}
}

// WakeupEdgeCensusExitSplitLabel renders the typed exit split for banner/log
// faces — one renderer, both faces (两面同源). "" when the pair carries no
// split (legacy JSON replays).
func WakeupEdgeCensusExitSplitLabel(pair WakeupEdgeCensusPair) string {
	if pair.SleepExitCount == 0 && pair.DExitCount == 0 && pair.OtherExitCount == 0 {
		return ""
	}
	return fmt.Sprintf("sleep_exit=%d d_exit=%d other_exit=%d", pair.SleepExitCount, pair.DExitCount, pair.OtherExitCount)
}
