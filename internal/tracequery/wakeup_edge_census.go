package tracequery

// wakeup_edge_census.go — WAKE-CENSUS (立案 §29.58, 实施 2026-07-13, ledger
// docs/design/real_trace_campaign_20260705.md): the per-(waker → wakee)
// wakeup-edge census, minted by the ENGINE on the FULL edge set of one
// wakeup_chain result.
//
// 病根 (PRC-F1 witness「OS_IPC_14_34911 ×4」三重假): the publication layer
// bounds the per-edge observation rows with a typed per-family row cap, so a
// consumer that re-counts the published rows holds a silently TRUNCATED
// inventory — and the model face, given no complete per-waker account,
// invented counts for pairs it never saw (the raw trace held exactly ONE
// edge for that pair, in the OPPOSITE direction). Census discipline follows
// the blocked_reason_census precedent (§29.57 件1):
//
//   - counts fold over the FULL pre-publication accumulator, never over a
//     capped/truncated view (禁截断库存二次聚合 — this defect class was hit
//     twice in this campaign window);
//   - the wire face stays bounded: a pair cap plus EXPLICIT overflow counts
//     (pairs and edges beyond the listed rows);
//   - the order is deterministic: count desc, then typed tie keys
//     (waker comm, waker pid, wakee comm, wakee pid — DET 纪律, no map
//     iteration order ever reaches the wire).

import "sort"

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

// buildWakeupEdgeCensus folds the FULL edge set into the bounded census view.
// Deduplication: two branch expansions can observe the SAME physical
// sched_wakeup row (same pair + wakeup ts + line); one physical row is one
// observation and counts once. target is the chain's anchor thread — pairs
// waking it are immune to the pair cap (件5 同款). Returns the capped rows
// plus the explicit overflow accounting (distinct pairs beyond the cap, and
// the deduplicated edges those pairs carry).
func buildWakeupEdgeCensus(edges []WakeupEdge, target ThreadRef, pairCap int) ([]WakeupEdgeCensusPair, int, int) {
	if len(edges) == 0 {
		return nil, 0, 0
	}
	type physicalKey struct {
		pair wakeupEdgeCensusKey
		ts   float64
		line int
	}
	seen := map[physicalKey]bool{}
	fold := map[wakeupEdgeCensusKey]*WakeupEdgeCensusPair{}
	var order []wakeupEdgeCensusKey
	for i := range edges {
		e := &edges[i]
		key := wakeupEdgeCensusKey{
			wakerComm: e.Waker.Comm, wakerPID: e.Waker.PID,
			wakeeComm: e.Wakee.Comm, wakeePID: e.Wakee.PID,
		}
		if key.wakerComm == "" && key.wakerPID == 0 && key.wakeeComm == "" && key.wakeePID == 0 {
			continue
		}
		phys := physicalKey{pair: key, ts: e.WakeupTs, line: e.WakeupLine}
		if seen[phys] {
			continue
		}
		seen[phys] = true
		pair, ok := fold[key]
		if !ok {
			pair = &WakeupEdgeCensusPair{Waker: e.Waker, Wakee: e.Wakee, FirstTs: e.WakeupTs, LastTs: e.WakeupTs}
			fold[key] = pair
			order = append(order, key)
		}
		pair.Count++
		if e.WakeupTs < pair.FirstTs {
			pair.FirstTs = e.WakeupTs
		}
		if e.WakeupTs > pair.LastTs {
			pair.LastTs = e.WakeupTs
		}
	}
	rows := make([]WakeupEdgeCensusPair, 0, len(order))
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
	overflowPairs, overflowEdges := 0, 0
	if pairCap > 0 && len(rows) > pairCap {
		// Target-wakee immunity (件5 同款纪律: the question's own thread never
		// falls to a cap): pairs that wake the chain TARGET are the answer to
		// the waker question, yet as ×1 ties with late lexicographic keys they
		// are exactly the rows a blind trim evicts first (donghu witness: the
		// direct tppmgr-idle/hilogcat/binder wakers of CompThread_0-2955 fell
		// to overflow while chain-intermediate pairs stayed listed). Selection:
		// target-wakee rows survive unconditionally — ALL of them, even past
		// the cap (their population is bounded by the branch expansion) — and
		// the remaining seats fill in the deterministic order above. Kept rows
		// preserve that order; evicted rows fold into the explicit overflow.
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
	return rows, overflowPairs, overflowEdges
}

// attachWakeupEdgeCensus mints the census onto a finished chain result. It
// must run AFTER every edge-producing pass (branch expansion, via-immune
// expansion with its rollbacks) so the source is the whole surviving edge
// set — never a partial or truncated one.
func attachWakeupEdgeCensus(res *ChainResult) {
	res.WakeupEdgeCensus, res.WakeupEdgeCensusOverflowPairs, res.WakeupEdgeCensusOverflowEdges =
		buildWakeupEdgeCensus(res.Edges, res.Target, wakeupEdgeCensusPairCap)
}
