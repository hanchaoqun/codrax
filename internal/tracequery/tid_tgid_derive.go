package tracequery

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// B-3 (§7.11): the hmtrace line template ("{task}-{tid} [{cpu}] d... {ts}:
// {body}") carries no "( tgid )" column, so every Event.TGID parses to 0 and
// the per-process display rollups degrade to one-row-per-thread. The
// trace_mark payload still carries the true process pid on every business
// span — the row header names the emitting tid while `B|{pid}|{name}` names
// the owning process — so a per-index majority vote over (row tid → span pid)
// recovers thread→process attribution.
//
// Discipline (CLAUDE.md 架构红线): the vote is a NOISY signal. It drives only
// SOFT display-layer grouping — the pid→process catalog TGID backfill behind
// buildThreadCatalog plus caveat/marker text. It must never feed matchesPID
// filtering, census/completion gates, or any contract.Check face.
//
// Enablement gate (PRECISE signal): derivation is considered at all only when
// the index parsed at least one event and EVERY event's native TGID is 0 —
// "no TGID column anywhere in this index" is a single integer comparison, not
// a flavor guess (flavor.go scores are noise and must not gate). Any trace
// with a real TGID column — android atrace included, even when most rows show
// the "(-----)" placeholder that parses to 0 — keeps derivation disabled
// wholesale, so the with-TGID path is byte-identical.

// tidTgidDerivedCaveat is the verbatim window-level disclosure attached to
// WindowStats.Caveats and CPUOccupancyStats.Caveats whenever the vote table
// actually grouped threads in the selected window.
//
// Wording is pinned (TestTidTgidCaveatWordingPinned):
//   - it must say that pid= filtering/drilldown does NOT follow the derived
//     grouping — matchesPID and every drilldown face still key on the
//     row-header tid only, so a reader who drills a derived tgid must not
//     expect numbers that reconcile with the grouped process row;
//   - it must NOT claim "reused" tids stay ungrouped: the guard tests
//     row-header comm drift only, a reused tid that keeps the same comm is
//     undetectable, so promising it would over-claim.
const tidTgidDerivedCaveat = "process attribution soft-derived from trace_mark span pids by per-tid majority vote (this trace has no TGID column); derivation is per-window/per-index, ties and renamed tids stay ungrouped; pid= filtering/drilldown still matches row-header tids only and does not follow the derived grouping — 进程归属为软推导(SpanPID 表决),平票/改名线程不归组;pid= 过滤/钻取仍只匹配行头 tid,不追随派生分组"

// tidTgidDerivedRowMarker is appended to the Summary of each process row whose
// grouping came from the vote table, so every derived row is self-explaining
// on the typed lane without a new typed field (R2' six-spot sync avoided by
// design for the first version).
const tidTgidDerivedRowMarker = " attribution=trace_mark_span_vote(soft)"

// tidTgidDerivation is the per-index soft tid→tgid table. nil (or an empty
// table) means "derivation disabled — behave exactly as before".
type tidTgidDerivation struct {
	// tidTgid maps a row-header tid to the span pid that won its majority
	// vote. Ties and tids with drifting row-header names are absent
	// (fail-open: the thread keeps today's per-thread degenerate row).
	tidTgid map[int]int
	// commByTgid is display-only name enrichment for derived process rows:
	// tgid → the process name of the first CONFIRMED self-registration pair
	// (`B|{tgid}|{process_name}` emitted BY tid==tgid and immediately closed
	// by its own E — zero child marks in between; db2systrace.py pins
	// main-thread name == process name and emits the pair back-to-back).
	// A bare self-B is NOT enough: business span names must never be
	// promoted to process names, so an unconfirmed candidate is discarded
	// and the tgid gets no backfill (row keeps the degenerate pid=N label).
	// Never a matching key.
	commByTgid map[int]string
}

func (d *tidTgidDerivation) enabled() bool { return d != nil && len(d.tidTgid) > 0 }

func (d *tidTgidDerivation) tgidFor(tid int) int {
	if d == nil {
		return 0
	}
	return d.tidTgid[tid]
}

func (d *tidTgidDerivation) commFor(tgid int) string {
	if d == nil {
		return ""
	}
	return d.commByTgid[tgid]
}

// derivedTidTgid lazily builds the vote table exactly once per Index (parse
// is already done; one O(N) scan, the same order of work as one
// buildThreadCatalog pass). Windowed builds get their own Index and therefore
// their own table — the anchor-seek window may not see the trace head's
// registration pairs, which is why the caveat says per-window/per-index.
// Safe for concurrent queries against a cached Index: sync.Once publishes an
// immutable table.
func (idx *Index) derivedTidTgid() *tidTgidDerivation {
	if idx == nil {
		return nil
	}
	idx.tidTgidVoteOnce.Do(func() {
		idx.tidTgidVote = buildTidTgidDerivation(idx.Events)
	})
	return idx.tidTgidVote
}

// buildTidTgidDerivation runs the gate and the vote in one pass. Returns nil
// whenever derivation must stay off: any native TGID present, no span votes,
// or no tid survives the tie/name-drift guards.
func buildTidTgidDerivation(events []Event) *tidTgidDerivation {
	if len(events) == 0 {
		return nil
	}
	votes := map[int]map[int]int{}
	names := map[int]map[string]bool{}
	// selfName is the confirmed registration-pair name per tgid; pendingReg
	// is the one-step state machine that confirms the pair SHAPE: a self-B
	// (`B|{tid}|{name}` with span pid == row tid) is only a candidate, and
	// becomes the process name IFF the very next trace_mark emitted by the
	// same tid is its own E close — zero child marks in between, the
	// db2systrace.py registration-pair shape. Any other intervening mark
	// from that tid (child B, C counter, async S/F, an E for another pid)
	// breaks the shape and discards the candidate. Adjacent-closed LEAF
	// business marks are additionally excluded by the "H:" prefix guard:
	// hitrace machine-prefixes every instrumentation mark name with the
	// closed "H:" token while db2systrace.py registration pairs carry raw
	// process names, so an "H:" name can never be a process name. When no
	// pair confirms, the tgid simply gets no display name — fail open to
	// the degenerate pid=N label, never a business span name.
	selfName := map[int]string{}
	pendingReg := map[int]string{}
	for _, ev := range events {
		if ev.TGID > 0 {
			// Precise disable: this index HAS a TGID column. The with-TGID
			// (android/atrace) path must stay byte-identical. Not silent
			// (§7.11 B-3 ruling): a mixed artifact — one stray native-TGID
			// line inside an hmtrace capture — disables derivation wholesale
			// and fails open to ungrouped rows, so leave an operator-visible
			// trail explaining why grouping disappeared.
			logging.Warning("tracequery: native TGID present (row tid=%d tgid=%d); SpanPID derivation disabled for this index", ev.PID, ev.TGID)
			return nil
		}
		if ev.PID > 0 {
			if comm := strings.TrimSpace(ev.Comm); comm != "" && comm != "<...>" {
				set := names[ev.PID]
				if set == nil {
					set = map[string]bool{}
					names[ev.PID] = set
				}
				set[comm] = true
			}
		}
		if ev.Type != EventTraceMark || ev.PID <= 0 {
			continue
		}
		// Registration-pair state machine (display names only, see selfName
		// above). Runs on EVERY mark from the tid — including bare `E` and
		// `B|0|…` rows that carry no span pid — because any mark between a
		// candidate B and its close breaks the zero-child pair shape.
		switch {
		case ev.SpanAction == "B" && ev.SpanPID == ev.PID && ev.SpanName != "" && !strings.HasPrefix(ev.SpanName, "H:"):
			pendingReg[ev.PID] = ev.SpanName
		case ev.SpanAction == "E" && (ev.SpanPID == ev.PID || ev.SpanPID == 0):
			// `E|{tid}|` (db2systrace.py) or bare `E` (plain atrace) closes
			// the candidate: pair confirmed, first confirmed name wins.
			if name := pendingReg[ev.PID]; name != "" {
				if _, ok := selfName[ev.PID]; !ok {
					selfName[ev.PID] = name
				}
			}
			delete(pendingReg, ev.PID)
		default:
			delete(pendingReg, ev.PID)
		}
		if ev.SpanPID <= 0 {
			continue
		}
		switch ev.SpanAction {
		case "B", "C":
			// B carries the owning process pid on every business span and on
			// the thread-registration pair; C (counters) has the same shape.
			// E is deliberately NOT counted: every B|pid registration closes
			// with E|pid and counting both would double-weight registration
			// pairs. S/F (async) stay out of the first version.
			m := votes[ev.PID]
			if m == nil {
				m = map[int]int{}
				votes[ev.PID] = m
			}
			m[ev.SpanPID]++
		}
	}
	if len(votes) == 0 {
		return nil
	}
	out := &tidTgidDerivation{tidTgid: map[int]int{}, commByTgid: selfName}
	for tid, m := range votes {
		if len(names[tid]) > 1 {
			// Precise name-drift signal (same test detectThreadDrifts uses):
			// a renamed or reused tid must not be grouped by a vote whose
			// ballots may span two identities. Fail open.
			continue
		}
		best, bestVotes, tie := 0, 0, false
		for tgid, n := range m {
			switch {
			case n > bestVotes:
				best, bestVotes, tie = tgid, n, false
			case n == bestVotes:
				tie = true
			}
		}
		if tie || best <= 0 {
			// Tie → no derivation for this tid (never first-seen tie-breaks:
			// map iteration order would make grouping non-reproducible).
			continue
		}
		out.tidTgid[tid] = best
	}
	if len(out.tidTgid) == 0 {
		return nil
	}
	return out
}
