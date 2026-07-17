package tracequery

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// defaultRelationScopeMaxDepth bounds the transitive waker-closure BFS when the
// caller does not pass an explicit ScopeMaxDepth. It sits one above the
// wakeup-chain query's default MaxDepth (10, see normalizeQuery) so the
// discovered waker set is a superset of what expandChain actually walks —
// pruning a waker's events at exactly the query depth would break the chain.
const defaultRelationScopeMaxDepth = 11

// relationScope is the pass-1 result: the set of thread ids whose events the
// causal-chain views (thread_timeline / wakeup_chain) consume for a
// pid-scoped query — the target process's threads plus their transitive
// scheduler wakers. Everything else in the window is dropped in pass-2, except
// binder rows which are always kept (they are sparse and needed to pair
// transaction↔received edges by id, which no pid predicate can reconstruct).
type relationScope struct {
	relevantTids map[int]struct{}
}

func (s *relationScope) has(pid int) bool {
	if s == nil || pid <= 0 {
		return false
	}
	_, ok := s.relevantTids[pid]
	return ok
}

// keep is the pass-2 retention predicate. It mirrors exactly what
// newChainQueryCache indexes (SchedSwitch→Prev/Next, Wakeup/Waking→Wakee,
// BlockedReason→Wakee) plus threadTimelineForTarget's consumption, so the
// pruned index is complete for those two views. Binder rows are kept
// unconditionally for attachIPCGraphToChain's id-based edge pairing.
//
// Correctness rests on relevantTids being a SUPERSET of the threads the chain
// walks: pass-1 collects every waker that ever woke a relevant thread in the
// window (not just the one expandChain picks per sleep interval) and closes
// transitively, so any sched_switch / wakeup / blocked_reason the chain could
// read for a walked thread is retained.
func (s *relationScope) keep(ev *Event) bool {
	switch ev.Type {
	case EventSchedSwitch:
		return s.has(ev.PrevPID) || s.has(ev.NextPID)
	case EventSchedWakeup, EventSchedWaking, EventSchedBlockedReason:
		return s.has(ev.WakeePID)
	}
	return relationScopeAlwaysKeepEvent(ev.Type)
}

// relationScopeAlwaysKeepEvent lists event types retained regardless of pid.
// Binder rows are kept whole so BuildIPCGraph can pair a target's send with a
// peer's receive by BinderTransactionID (the peer row carries no field equal to
// the target pid, so a pid predicate would drop it and degrade the IPC edge).
func relationScopeAlwaysKeepEvent(t EventType) bool {
	switch t {
	case EventBinderTransaction, EventBinderReceived, EventBinderAllocBuf,
		EventBinderLock, EventBinderLocked, EventBinderUnlock, EventBinderReply:
		return true
	}
	return false
}

// discoverRelationScope streams the window once (a separate, lightweight pass
// that appends no events) to build the relevant-thread set for a pid/thread
// scoped build. It collects tid→tgid (to expand a process id into its threads),
// typed thread-name candidates (when ScopeThread is supplied), and wakee→waker
// edges (to close the waker chain), then runs a bounded BFS. Thread-only
// selectors prune only when they resolve to a single pid/tgid universe; ambiguous
// selectors degrade to an unpruned index with a caveat instead of hard-choosing.
func discoverRelationScope(ctx context.Context, f *os.File, path string, expected traceFileIdentity, opts BuildOptions) (*relationScope, []string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if f == nil {
		return nil, nil, fmt.Errorf("relation discovery requires an opened trace source")
	}
	opened, err := traceFileIdentityFromFile(f)
	if err != nil {
		return nil, nil, err
	}
	if expected.Initialized() && !expected.SameVersion(opened) {
		return nil, nil, fmt.Errorf("relation discovery source differs from the selected artifact ledger")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, nil, fmt.Errorf("rewind trace source for relation discovery: %w", err)
	}

	// Reuse only an EOF-complete timestamp-order proof. Without it the
	// discovery pass must scan through out-of-window future rows because a
	// later physical line may regress back into the selected window.
	var anchorSet *traceAnchorSet
	anchorSet = anchorCache.load(traceAnchorKeyForIdentity(path, opened))
	gate := windowGate{
		lineStart:        paddedLineStart(opts),
		lineEnd:          paddedLineEnd(opts),
		timeStart:        paddedTimeStart(opts),
		timeEnd:          paddedTimeEnd(opts),
		timeStartSet:     opts.TimeStartSet,
		timeEndSet:       opts.TimeEndSet,
		allowTimeEndStop: anchorSet != nil && anchorSet.TimestampOrder.AllowsTimeEndEarlyStop(),
	}

	tidToTgid := map[int]int{}
	wakeeToWakers := map[int]map[int]struct{}{}
	threadCandidates := map[int]struct{}{}
	selector := parseThreadSelector(opts.ScopeThread)

	// P1: the discovery pre-pass consumes the anchor index too (it never
	// records — the main pass owns extension).
	startLine := 1
	if anchorSet != nil && anchorSet.FlavorSet {
		if a, ok := anchorSet.seekAnchorFor(opts.TimeStartSet, paddedTimeStart(opts), paddedLineStart(opts)); ok {
			if _, serr := f.Seek(a.ByteOffset, io.SeekStart); serr == nil {
				startLine = a.LineNo + 1
			}
		}
	}
	frozenSource, err := frozenTraceSectionAtCurrentOffset(f, opened)
	if err != nil {
		return nil, nil, err
	}
	r := bufio.NewReaderSize(frozenSource, 256*1024)
	intern := newStringInterner()
	scratch := &Index{} // throwaway sink for safeParseLineScan's panic bookkeeping
	seenTimeWindow := false
	var scan lineScan
	for lineNo := startLine; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		line, rerr := readStreamScanPhysicalLine(r, attachment.TracePhysicalLineMaxBytes)
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			scan.reset(lineNo, trimmed)
			skip, stop := gate.decide(lineNo, &scan, &seenTimeWindow)
			if stop {
				break
			}
			if !skip {
				if ev, ok := safeParseLineScan(&scan, intern, scratch); ok {
					if ev.PID > 0 && ev.TGID > 0 {
						tidToTgid[ev.PID] = ev.TGID
					}
					collectRelationScopeThreadCandidates(selector, ev, threadCandidates)
					switch ev.Type {
					case EventSchedWakeup, EventSchedWaking:
						// Task creation is a lifecycle edge, not a causal wakeup
						// dependency from creator to the previous TID occupant.
						if !schedWakeupStartsNewIncarnation(ev) && ev.WakeePID > 0 && ev.PID > 0 {
							wakers := wakeeToWakers[ev.WakeePID]
							if wakers == nil {
								wakers = map[int]struct{}{}
								wakeeToWakers[ev.WakeePID] = wakers
							}
							wakers[ev.PID] = struct{}{}
						}
					}
				}
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return nil, nil, traceReadErrorAfterIdentity(f, opened, "relation discovery physical read", rerr)
		}
	}
	if err := validateTraceFileIdentityAfterRead(f, opened, "relation discovery"); err != nil {
		return nil, nil, err
	}

	scopePID, caveat, ok := resolveRelationScopeSeedPID(opts, selector, tidToTgid, threadCandidates)
	if !ok {
		var caveats []string
		if caveat != "" {
			caveats = append(caveats, caveat)
		}
		return nil, caveats, nil
	}
	scope := &relationScope{relevantTids: map[int]struct{}{}}
	// Target set: the scoped pid itself (whether it names a thread or a
	// process) plus every thread whose tgid equals it (process expansion).
	// This process expansion is relation-scope's OWN semantic — the causal
	// chain views need every member thread of a scoped process. The streaming
	// state-cluster face deliberately does NOT share it:
	// streamStateClusterApplyThreadFilter (stream_search.go) filters by exact
	// TID only (stream ThreadRefs carry no TGID), so pid=<process pid> means
	// the whole process here and exactly one thread there (audit #54).
	frontier := []int{scopePID}
	scope.relevantTids[scopePID] = struct{}{}
	for tid, tgid := range tidToTgid {
		if tgid == scopePID {
			if _, seen := scope.relevantTids[tid]; !seen {
				scope.relevantTids[tid] = struct{}{}
				frontier = append(frontier, tid)
			}
		}
	}
	// Transitive waker closure, bounded by ScopeMaxDepth.
	depth := opts.scopeMaxDepth()
	for d := 0; d < depth && len(frontier) > 0; d++ {
		var next []int
		for _, tid := range frontier {
			for waker := range wakeeToWakers[tid] {
				if _, seen := scope.relevantTids[waker]; !seen {
					scope.relevantTids[waker] = struct{}{}
					next = append(next, waker)
				}
			}
		}
		frontier = next
	}
	return scope, nil, nil
}

func collectRelationScopeThreadCandidates(sel threadSelector, ev Event, out map[int]struct{}) {
	if out == nil || strings.TrimSpace(sel.Raw) == "" {
		return
	}
	add := func(pid int, comm string) {
		if pid <= 0 || !threadSelectorMatchesName(sel, comm) {
			return
		}
		if sel.HasPID && pid != sel.PID {
			return
		}
		out[pid] = struct{}{}
	}
	add(ev.PID, ev.Comm)
	add(ev.PrevPID, ev.PrevComm)
	add(ev.NextPID, ev.NextComm)
	add(ev.WakeePID, ev.WakeeComm)
	if ss := ev.SchedStatFields; ss != nil {
		add(ss.PID, ss.Comm)
	}
	if cf := ev.ConstraintFields; cf != nil {
		add(cf.PID, cf.Comm)
	}
	if pf := ev.PerfFields; pf != nil {
		add(pf.PID, pf.Comm)
		add(pf.TID, pf.Comm)
	}
}

func resolveRelationScopeSeedPID(opts BuildOptions, sel threadSelector, tidToTgid map[int]int, candidates map[int]struct{}) (int, string, bool) {
	if opts.ScopePID > 0 {
		return opts.ScopePID, "", true
	}
	if sel.HasPID {
		return sel.PID, "", true
	}
	if strings.TrimSpace(sel.Raw) == "" {
		return 0, "", false
	}
	if len(candidates) == 0 {
		return 0, fmt.Sprintf("relation_scope_thread_unresolved selector=%q", strings.TrimSpace(sel.Raw)), false
	}
	tgids := map[int]struct{}{}
	var pids []int
	for pid := range candidates {
		pids = append(pids, pid)
		if tgid := tidToTgid[pid]; tgid > 0 {
			tgids[tgid] = struct{}{}
		}
	}
	sort.Ints(pids)
	if len(tgids) == 1 {
		for tgid := range tgids {
			return tgid, "", true
		}
	}
	if len(tgids) == 0 && len(pids) == 1 {
		return pids[0], "", true
	}
	return 0, fmt.Sprintf("relation_scope_thread_ambiguous selector=%q candidate_pids=%s candidate_tgids=%s", strings.TrimSpace(sel.Raw), joinIntsForRelationScopeCaveat(pids), joinRelationScopeTgids(tgids)), false
}

func joinRelationScopeTgids(tgids map[int]struct{}) string {
	if len(tgids) == 0 {
		return ""
	}
	items := make([]int, 0, len(tgids))
	for tgid := range tgids {
		items = append(items, tgid)
	}
	sort.Ints(items)
	return joinIntsForRelationScopeCaveat(items)
}

func joinIntsForRelationScopeCaveat(items []int) string {
	if len(items) == 0 {
		return ""
	}
	const maxItems = 8
	if len(items) > maxItems {
		items = items[:maxItems]
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, strconv.Itoa(item))
	}
	return strings.Join(parts, ",")
}
