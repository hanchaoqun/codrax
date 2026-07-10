package tracequery

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// schedulerHeadSnapshot is the single authority for scheduler state at an
// explicit time-window head. It exists because a bounded Index intentionally
// drops the arbitrarily old sched_switch that may still govern the first
// in-window interval. Consumers must either seed from a Complete snapshot or
// publish an explicit unknown-head caveat; absence from a bounded index is
// never interpreted as zero duration.
type schedulerHeadSnapshot struct {
	BoundaryTs float64
	Complete   bool
	Reason     string
	Threads    map[int]schedulerHeadThread
	CPUs       map[int]schedulerHeadCPU
	// lifecycle is the exact physical-prefix identity state at BoundaryTs.
	// Window parsers that seek to an anchor clone it so sched_wakeup_new inside
	// the retained window can still prove reuse of a TID seen before the seek.
	lifecycle *threadIncarnationTracker
}

// ENG audit #45 (§29.25 处置委托 2026-07-10): the head structs deliberately
// store only the raw Priority. They used to also store a PriorityClass
// classified with a hardcoded TraceFlavorGenericFtrace — dead-but-wrong data
// on Harmony traces (window-head prio 100 stored as raw_scheduler_prio
// instead of ohos_rt) and a booby trap for any future consumer reading the
// stored field. Every consumer classifies from Priority with the query's
// proven flavor at read time (classifyTracePriority(q.TraceFlavor, ...)).
type schedulerHeadThread struct {
	Thread       ThreadRef
	State        ThreadState
	StartTs      float64
	LastEventTs  float64
	Line         int
	CPU          int
	CPUKnown     bool
	Priority     int
	PrevStateRaw string
}

type schedulerHeadCPU struct {
	CPU         int
	Thread      ThreadRef
	StartTs     float64
	LastEventTs float64
	Line        int
	Priority    int
}

// sourceSchedulerHeadCache prevents repeated prefix scans when several trace
// views inspect the same physical artifact and boundary. Entries are immutable
// and identity-keyed by path/size/modtime/parser version plus clock mapping and
// virtual-line base. A byte budget, rather than an entry count alone, bounds
// high-thread-cardinality traces.
const schedulerHeadCacheBudgetBytes int64 = 64 << 20

// Windowed indexes retain their one build-time checkpoint even when it exceeds
// this local memo budget because that checkpoint is part of result correctness;
// traceIndexCacheCost charges its actual bytes to the global LRU. Full indexes
// do not retain per-query checkpoints.
const indexSchedulerHeadMemoBudgetBytes int64 = 16 << 20
const indexSchedulerHeadMemoMaxEntries = 2

type schedulerHeadCacheKey struct {
	path         string
	size         int64
	modUnix      int64
	identity     string
	version      string
	boundaryBits uint64
	lineBase     int
	offsetBits   uint64
	slopeBits    uint64
}

type schedulerHeadCacheEntry struct {
	snapshot *schedulerHeadSnapshot
	cost     int64
}

type schedulerHeadCacheStore struct {
	mu    sync.Mutex
	items map[schedulerHeadCacheKey]schedulerHeadCacheEntry
	order []schedulerHeadCacheKey
	bytes int64
}

var schedulerHeadCache = &schedulerHeadCacheStore{items: map[schedulerHeadCacheKey]schedulerHeadCacheEntry{}}

func (c *schedulerHeadCacheStore) load(key schedulerHeadCacheKey) *schedulerHeadSnapshot {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
		return nil
	}
	return entry.snapshot
}

func (c *schedulerHeadCacheStore) store(key schedulerHeadCacheKey, snapshot *schedulerHeadSnapshot) {
	if c == nil || snapshot == nil {
		return
	}
	cost := schedulerHeadSnapshotCost(snapshot)
	if cost <= 0 || cost > schedulerHeadCacheBudgetBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[key]; exists {
		return
	}
	for len(c.order) > 0 && c.bytes+cost > schedulerHeadCacheBudgetBytes {
		oldest := c.order[0]
		c.order = c.order[1:]
		if entry, ok := c.items[oldest]; ok {
			c.bytes -= entry.cost
			delete(c.items, oldest)
		}
	}
	c.items[key] = schedulerHeadCacheEntry{snapshot: snapshot, cost: cost}
	c.order = append(c.order, key)
	c.bytes += cost
}

func schedulerHeadSnapshotCost(snapshot *schedulerHeadSnapshot) int64 {
	if snapshot == nil {
		return 0
	}
	cost := int64(256)
	for _, state := range snapshot.Threads {
		cost += 160 + int64(len(state.Thread.Comm)+len(state.PrevStateRaw))
	}
	for _, state := range snapshot.CPUs {
		cost += 128 + int64(len(state.Thread.Comm))
	}
	if snapshot.lifecycle != nil {
		cost += int64(len(snapshot.lifecycle.seen))*40 + int64(len(snapshot.lifecycle.dead))*40
	}
	return cost
}

func schedulerHeadBoundaryKey(ts float64) uint64 { return math.Float64bits(ts) }

func (idx *Index) schedulerHeadAt(boundaryTs float64) *schedulerHeadSnapshot {
	if idx == nil || boundaryTs <= 0 {
		return nil
	}
	key := schedulerHeadBoundaryKey(boundaryTs)
	idx.schedulerHeadMu.Lock()
	if snapshot := idx.schedulerHeads[key]; snapshot != nil {
		idx.schedulerHeadMu.Unlock()
		return snapshot
	}
	idx.schedulerHeadMu.Unlock()
	// A full index owns the complete event prefix, so no physical-file rescan
	// is needed. Windowed indexes must be populated during BuildIndexWithOptions
	// while its cancellable context is available.
	if idx.Windowed {
		return nil
	}
	// Full indexes are already retained by the global LRU.  Caching a
	// high-cardinality snapshot inside them would escape that LRU's byte cost
	// and understate memory by O(query-boundaries * threads).  Recompute the
	// single-pass checkpoint instead; bounded indexes retain exactly their one
	// build-time checkpoint above.
	return schedulerHeadFromEvents(idx, boundaryTs)
}

func (idx *Index) setSchedulerHead(snapshot *schedulerHeadSnapshot) {
	if idx == nil || snapshot == nil || snapshot.BoundaryTs <= 0 {
		return
	}
	idx.schedulerHeadMu.Lock()
	defer idx.schedulerHeadMu.Unlock()
	if idx.schedulerHeads == nil {
		idx.schedulerHeads = map[uint64]*schedulerHeadSnapshot{}
	}
	idx.storeSchedulerHeadLocked(schedulerHeadBoundaryKey(snapshot.BoundaryTs), snapshot, true)
}

func (idx *Index) storeSchedulerHeadLocked(key uint64, snapshot *schedulerHeadSnapshot, required bool) {
	if idx == nil || snapshot == nil {
		return
	}
	if idx.schedulerHeads == nil {
		idx.schedulerHeads = map[uint64]*schedulerHeadSnapshot{}
	}
	if old := idx.schedulerHeads[key]; old != nil {
		idx.schedulerHeadBytes -= schedulerHeadSnapshotCost(old)
		for i, existing := range idx.schedulerHeadOrder {
			if existing == key {
				idx.schedulerHeadOrder = append(idx.schedulerHeadOrder[:i], idx.schedulerHeadOrder[i+1:]...)
				break
			}
		}
	}
	cost := schedulerHeadSnapshotCost(snapshot)
	if !required && (cost <= 0 || cost > indexSchedulerHeadMemoBudgetBytes) {
		return
	}
	for len(idx.schedulerHeadOrder) > 0 &&
		(len(idx.schedulerHeadOrder) >= indexSchedulerHeadMemoMaxEntries || (!required && idx.schedulerHeadBytes+cost > indexSchedulerHeadMemoBudgetBytes)) {
		oldest := idx.schedulerHeadOrder[0]
		idx.schedulerHeadOrder = idx.schedulerHeadOrder[1:]
		if old := idx.schedulerHeads[oldest]; old != nil {
			idx.schedulerHeadBytes -= schedulerHeadSnapshotCost(old)
			delete(idx.schedulerHeads, oldest)
		}
	}
	idx.schedulerHeads[key] = snapshot
	idx.schedulerHeadOrder = append(idx.schedulerHeadOrder, key)
	idx.schedulerHeadBytes += cost
}

func schedulerHeadFromEvents(idx *Index, boundaryTs float64) *schedulerHeadSnapshot {
	snapshot := newSchedulerHeadSnapshot(boundaryTs)
	if idx == nil {
		snapshot.Reason = "trace_index_missing"
		return snapshot
	}
	if idx.TimestampOrder == TraceTimestampOrderUnknown && idx.Windowed {
		snapshot.Reason = "timestamp_order_unproven"
		return snapshot
	}
	cpuOrder, pidOrder := newSchedulerOrderTracker(), newSchedulerOrderTracker()
	orderQ := Query{TimeEnd: boundaryTs}
	for _, ev := range idx.Events {
		for _, violation := range auditSchedulerOrderEvent(cpuOrder, pidOrder, ev) {
			if schedulerOrderViolationRelevantToQuery(&violation, orderQ, 0) {
				snapshot.Reason = "scheduler_lane_timestamp_regressed"
				return snapshot
			}
		}
		if ev.Ts >= boundaryTs {
			if idx.TimestampOrder.AllowsTimeEndEarlyStop() {
				break
			}
			continue
		}
		applySchedulerHeadEvent(snapshot, ev)
	}
	snapshot.Complete = true
	return snapshot
}

func newSchedulerHeadSnapshot(boundaryTs float64) *schedulerHeadSnapshot {
	return &schedulerHeadSnapshot{
		BoundaryTs: boundaryTs,
		Threads:    map[int]schedulerHeadThread{},
		CPUs:       map[int]schedulerHeadCPU{},
		lifecycle:  newThreadIncarnationTracker(),
	}
}

func schedulerHeadForQuery(idx *Index, q Query) *schedulerHeadSnapshot {
	if idx == nil || q.TimeStart <= 0 || q.LineStart > 0 || q.LineEnd > 0 {
		return nil
	}
	base := idx.schedulerHeadAt(q.TimeStart)
	if base == nil {
		return nil
	}
	copy := &schedulerHeadSnapshot{
		BoundaryTs: base.BoundaryTs,
		Complete:   base.Complete,
		Reason:     base.Reason,
		Threads:    make(map[int]schedulerHeadThread, len(base.Threads)),
		CPUs:       make(map[int]schedulerHeadCPU, len(base.CPUs)),
	}
	for pid, state := range base.Threads {
		copy.Threads[pid] = state
	}
	for cpu, state := range base.CPUs {
		copy.CPUs[cpu] = state
	}
	if !copy.Complete {
		return copy
	}
	for _, ev := range idx.Events {
		if ev.Ts != q.TimeStart || !eventLineInWindow(ev, q) || !schedWakeupStartsNewIncarnation(ev) || ev.WakeePID <= 0 {
			continue
		}
		// The pre-boundary checkpoint may still name the previous occupant of a
		// reused TID. At the exact inclusive boundary that identity is gone.
		// Remove its thread state and any CPU carry that still points at it; a
		// same-timestamp sched_switch will re-establish CPU state from the actual
		// in-window event, otherwise coverage remains explicitly unknown.
		delete(copy.Threads, ev.WakeePID)
		for cpu, state := range copy.CPUs {
			if state.Thread.PID == ev.WakeePID {
				delete(copy.CPUs, cpu)
			}
		}
	}
	return copy
}

func applySchedulerHeadEvent(snapshot *schedulerHeadSnapshot, ev Event) {
	if snapshot == nil || ev.Ts >= snapshot.BoundaryTs {
		return
	}
	if snapshot.lifecycle != nil {
		snapshot.lifecycle.observeAll(ev, 0)
	}
	switch ev.Type {
	case EventSchedWakeup, EventSchedWaking:
		if ev.WakeePID <= 0 {
			return
		}
		current, exists := snapshot.Threads[ev.WakeePID]
		newIncarnation := ev.Type == EventSchedWakeup && ev.Name == "sched_wakeup_new"
		if !newIncarnation && exists && (current.State == StateRunning || current.State == StateRunnable) {
			return
		}
		tgid := current.Thread.TGID
		if newIncarnation {
			// sched_wakeup_new is a precise lifecycle reset.  Never carry the
			// previous occupant's TGID/state through a reused numeric TID. The
			// old task may have no visible switch-out in a partial capture, so
			// invalidate every CPU checkpoint that still names that numeric TID
			// before publishing the new task as runnable.
			tgid = 0
			current = schedulerHeadThread{}
			for cpu, state := range snapshot.CPUs {
				if state.Thread.PID == ev.WakeePID {
					delete(snapshot.CPUs, cpu)
				}
			}
		}
		targetCPU, targetCPUKnown := eventTargetCPU(ev)
		snapshot.Threads[ev.WakeePID] = schedulerHeadThread{
			Thread:      ThreadRef{Comm: firstNonEmpty(ev.WakeeComm, current.Thread.Comm), PID: ev.WakeePID, TGID: tgid},
			State:       StateRunnable,
			StartTs:     ev.Ts,
			LastEventTs: ev.Ts,
			Line:        ev.Line,
			CPU:         targetCPU,
			CPUKnown:    targetCPUKnown,
			Priority:    ev.WakeePrio,
		}
	case EventSchedBlockedReason:
		if ev.WakeePID <= 0 || ev.IOWait <= 0 {
			return
		}
		current, exists := snapshot.Threads[ev.WakeePID]
		if !exists || current.State != StateDSleep || ev.Ts < current.StartTs {
			return
		}
		current.State = StateIOWait
		current.LastEventTs = ev.Ts
		if ev.Line > 0 {
			current.Line = ev.Line
		}
		snapshot.Threads[ev.WakeePID] = current
	case EventCPUConstraint:
		// sched_migrate_task is an exact CPU-location transition for a
		// runnable task. Preserve the governing runnable interval's state and
		// start time while moving only its CPU attribution. Other constraint
		// rows (affinity/cpuset policy) do not prove an actual migration.
		pid, destCPU, comm, ok := schedMigrationTarget(ev)
		if !ok {
			return
		}
		current, exists := snapshot.Threads[pid]
		if !exists || current.State != StateRunnable {
			return
		}
		current.CPU = destCPU
		current.CPUKnown = true
		current.LastEventTs = ev.Ts
		if ev.Line > 0 {
			current.Line = ev.Line
		}
		if current.Thread.Comm == "" && comm != "" {
			current.Thread.Comm = comm
		}
		snapshot.Threads[pid] = current
	case EventSchedSwitch:
		nextThread := ThreadRef{Comm: ev.NextComm, PID: ev.NextPID}
		if ev.NextPID > 0 {
			nextTGID := snapshot.Threads[ev.NextPID].Thread.TGID
			if ev.PID == ev.NextPID && ev.TGID > 0 {
				nextTGID = ev.TGID
			}
			nextThread.TGID = nextTGID
			snapshot.Threads[ev.NextPID] = schedulerHeadThread{
				Thread:      nextThread,
				State:       StateRunning,
				StartTs:     ev.Ts,
				LastEventTs: ev.Ts,
				Line:        ev.Line,
				CPU:         ev.CPU,
				CPUKnown:    true,
				Priority:    ev.NextPrio,
			}
		}
		// CPU state includes pid 0: the idle lane is just as important as a
		// running task for busy/idle carry-in.
		snapshot.CPUs[ev.CPU] = schedulerHeadCPU{
			CPU:         ev.CPU,
			Thread:      nextThread,
			StartTs:     ev.Ts,
			LastEventTs: ev.Ts,
			Line:        ev.Line,
			Priority:    ev.NextPrio,
		}
		if ev.PrevPID > 0 {
			state := stateFromPrevState(ev.PrevState)
			if state == StateDead {
				// X/Z is a precise terminal state; a dead task has no governing
				// carry-in state in a later window.
				delete(snapshot.Threads, ev.PrevPID)
				return
			}
			prevTGID := snapshot.Threads[ev.PrevPID].Thread.TGID
			if ev.PID == ev.PrevPID && ev.TGID > 0 {
				prevTGID = ev.TGID
			}
			snapshot.Threads[ev.PrevPID] = schedulerHeadThread{
				Thread:       ThreadRef{Comm: ev.PrevComm, PID: ev.PrevPID, TGID: prevTGID},
				State:        state,
				StartTs:      ev.Ts,
				LastEventTs:  ev.Ts,
				Line:         ev.Line,
				CPU:          ev.CPU,
				CPUKnown:     true,
				Priority:     ev.PrevPrio,
				PrevStateRaw: ev.PrevState,
			}
		}
	}
}

// schedMigrationTarget is the single precise admission predicate shared by
// the prefix checkpoint and in-window runnable state machines. Policy-only
// CPU constraints never enter; a destination must be explicitly present and
// a valid non-negative CPU identity.
func schedMigrationTarget(ev Event) (pid, destCPU int, comm string, ok bool) {
	if ev.Type != EventCPUConstraint || ev.ConstraintFields == nil {
		return 0, 0, "", false
	}
	cf := ev.ConstraintFields
	if cf.Kind != "sched_migrate_task" || cf.PID <= 0 || !cf.DestCPUSet || !validTraceCPUIndex(cf.DestCPU) {
		return 0, 0, "", false
	}
	return cf.PID, cf.DestCPU, cf.Comm, true
}

func populateWindowSchedulerHead(ctx context.Context, idx *Index, boundaryTs float64) error {
	if idx == nil || !idx.Windowed || boundaryTs <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sources := idx.TraceArtifacts
	if len(sources) == 0 {
		sources = []TraceArtifactSource{singleTraceArtifactSource(idx.Path, idx.Size, idx.ModTime.UnixNano(), idx.LineCount, len(idx.Events))}
	}
	merged := newSchedulerHeadSnapshot(boundaryTs)
	merged.Complete = true
	var reasons []string
	admitted := 0
	for _, source := range sources {
		if !source.CausalCompatible || strings.EqualFold(source.Kind, "perftrace") {
			continue
		}
		admitted++
		child, err := sourceSchedulerHeadSnapshot(ctx, source, boundaryTs)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			child = newSchedulerHeadSnapshot(boundaryTs)
			child.Reason = "scheduler_head_scan_error=" + err.Error()
		}
		if !child.Complete {
			merged.Complete = false
			reasons = append(reasons, fmt.Sprintf("%s:%s", filepath.Base(source.SourcePath), firstNonEmpty(child.Reason, "incomplete")))
			continue
		}
		mergeSchedulerHeadSnapshot(merged, child)
	}
	if admitted == 0 {
		merged.Complete = false
		reasons = append(reasons, "no_causal_scheduler_artifact")
	}
	if !merged.Complete {
		// Partial maps are never consumed: doing so would turn one source's
		// absence into another source's false zero. Preserve only the reason.
		merged.Threads = map[int]schedulerHeadThread{}
		merged.CPUs = map[int]schedulerHeadCPU{}
		merged.Reason = strings.Join(reasons, ";")
	} else if idx.RelationScoped {
		// Relation-scoped indexes consume head state only for their retained
		// target/waker closure.  Drop unrelated subjects and every CPU-global
		// checkpoint so the snapshot cannot be mistaken for a compatible
		// all-thread aggregate input.
		for pid := range merged.Threads {
			if !idx.relationScopeTIDs[pid] {
				delete(merged.Threads, pid)
			}
		}
		merged.CPUs = map[int]schedulerHeadCPU{}
	}
	idx.setSchedulerHead(merged)
	return nil
}

func mergeSchedulerHeadSnapshot(dst, src *schedulerHeadSnapshot) {
	if dst == nil || src == nil {
		return
	}
	for pid, candidate := range src.Threads {
		current, exists := dst.Threads[pid]
		if !exists || candidate.LastEventTs > current.LastEventTs || (candidate.LastEventTs == current.LastEventTs && candidate.Line > current.Line) {
			dst.Threads[pid] = candidate
		}
	}
	for cpu, candidate := range src.CPUs {
		current, exists := dst.CPUs[cpu]
		if !exists || candidate.LastEventTs > current.LastEventTs || (candidate.LastEventTs == current.LastEventTs && candidate.Line > current.Line) {
			dst.CPUs[cpu] = candidate
		}
	}
}

func sourceSchedulerHeadSnapshot(ctx context.Context, source TraceArtifactSource, canonicalBoundary float64) (*schedulerHeadSnapshot, error) {
	canonicalPath := canonicalTraceIndexPath(source.SourcePath)
	f, err := os.Open(canonicalPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !schedulerHeadSourceIdentityMatches(source, info) {
		return nil, fmt.Errorf("scheduler head source identity differs from parsed artifact ledger")
	}
	offset, slope := traceClockMapValues(source.ClockOffsetSec, source.ClockSlope)
	key := schedulerHeadCacheKey{
		path:         canonicalPath,
		size:         info.Size(),
		modUnix:      info.ModTime().UnixNano(),
		identity:     source.identityToken(info),
		version:      ParserVersion,
		boundaryBits: schedulerHeadBoundaryKey(canonicalBoundary),
		lineBase:     source.VirtualLineBase,
		offsetBits:   math.Float64bits(offset),
		slopeBits:    math.Float64bits(slope),
	}
	if cached := schedulerHeadCache.load(key); cached != nil {
		return cached, nil
	}
	snapshot, err := scanSourceSchedulerHead(ctx, source, f, info, canonicalBoundary)
	if err == nil {
		schedulerHeadCache.store(key, snapshot)
	}
	return snapshot, err
}

func schedulerHeadSourceIdentityMatches(source TraceArtifactSource, info os.FileInfo) bool {
	return source.identityMatchesInfo(info)
}

func scanSourceSchedulerHead(ctx context.Context, source TraceArtifactSource, f *os.File, info os.FileInfo, canonicalBoundary float64) (*schedulerHeadSnapshot, error) {
	snapshot := newSchedulerHeadSnapshot(canonicalBoundary)
	if f == nil || !schedulerHeadSourceIdentityMatches(source, info) {
		return nil, fmt.Errorf("scheduler head scan requires the parsed artifact version")
	}
	rawBoundary, boundaryOK := source.toSourceTsChecked(canonicalBoundary)
	if !boundaryOK {
		snapshot.Reason = "clock_inverse_unsafe"
		return snapshot, nil
	}
	rawScanBoundary := traceClockWidenULPs(rawBoundary, math.Inf(1), traceClockRoundTripMaxULPs)
	canonicalPath := canonicalTraceIndexPath(source.SourcePath)
	proof := TraceTimestampOrderUnknown
	if anchors := anchorCache.load(traceAnchorKeyForInfo(canonicalPath, info)); anchors != nil {
		proof = anchors.TimestampOrder
	}
	reader := bufio.NewReaderSize(f, 256*1024)
	intern := newStringInterner()
	parsedCandidates := 0
	scratch := &Index{}
	lastTs, lastTsSet := 0.0, false
	regressions := 0
	parseFailure := false
	orderCPU, orderPID := newSchedulerOrderTracker(), newSchedulerOrderTracker()
	var schedulerViolation *schedulerOrderViolation
	for lineNo := 1; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			ts, hasTS := parseLineTimestamp(trimmed)
			if hasTS {
				if lastTsSet && ts < lastTs {
					regressions++
				}
				lastTs, lastTsSet = ts, true
				if proof == TraceTimestampOrderMonotonic && ts >= rawScanBoundary {
					break
				}
			}
			if hasTS && schedulerHeadRawCandidate(trimmed) {
				ev, ok := safeParseLine(lineNo, trimmed, intern, scratch)
				if !ok {
					parseFailure = true
				} else {
					for _, violation := range auditSchedulerOrderEvent(orderCPU, orderPID, ev) {
						if schedulerOrderViolationRelevantToQuery(&violation, Query{TimeEnd: rawBoundary}, 0) {
							copy := violation
							schedulerViolation = &copy
							break
						}
					}
					ev.Line += source.VirtualLineBase
					mapped, mapOK := source.toCanonicalTsChecked(ev.Ts)
					if !mapOK {
						parseFailure = true
						continue
					}
					if traceClockRoundTripWithinULPs(canonicalBoundary, mapped) {
						mapped = canonicalBoundary
					}
					ev.Ts = mapped
					if ev.Ts < canonicalBoundary {
						applySchedulerHeadEvent(snapshot, ev)
						parsedCandidates++
					}
					if parsedCandidates%4096 == 0 {
						// Bound transient interner retention on long prefixes.  Strings
						// still referenced by the snapshot remain live independently.
						intern = newStringInterner()
					}
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, readErr
		}
	}
	if schedulerViolation != nil {
		snapshot.Threads = map[int]schedulerHeadThread{}
		snapshot.CPUs = map[int]schedulerHeadCPU{}
		snapshot.Reason = "scheduler_lane_timestamp_regressed"
		return snapshot, nil
	}
	if parseFailure {
		snapshot.Threads = map[int]schedulerHeadThread{}
		snapshot.CPUs = map[int]schedulerHeadCPU{}
		snapshot.Reason = "scheduler_row_parse_incomplete"
		return snapshot, nil
	}
	// Re-check the exact descriptor after the scan. This catches in-place
	// append/truncate/write races; atomic path replacement keeps this old fd
	// coherent and is caught by BuildIndexWithOptions' final universe check.
	finalInfo, statErr := f.Stat()
	if statErr != nil {
		return nil, statErr
	}
	if !traceFileIdentityFromInfo(info).matchesInfo(finalInfo) || !schedulerHeadSourceIdentityMatches(source, finalInfo) {
		return nil, fmt.Errorf("scheduler head source changed while its prefix was being scanned")
	}
	snapshot.Complete = true
	return snapshot, nil
}

func schedulerHeadRawCandidate(line string) bool {
	return strings.Contains(line, "sched_switch:") ||
		strings.Contains(line, "sched_wakeup:") ||
		strings.Contains(line, "sched_wakeup_new:") ||
		strings.Contains(line, "sched_waking:") ||
		strings.Contains(line, "sched_blocked_reason:") ||
		strings.Contains(line, "sched_migrate_task:")
}

func schedulerHeadSortedThreads(snapshot *schedulerHeadSnapshot) []schedulerHeadThread {
	if snapshot == nil || !snapshot.Complete {
		return nil
	}
	out := make([]schedulerHeadThread, 0, len(snapshot.Threads))
	for _, state := range snapshot.Threads {
		out = append(out, state)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Thread.PID != out[j].Thread.PID {
			return out[i].Thread.PID < out[j].Thread.PID
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func schedulerHeadSortedCPUs(snapshot *schedulerHeadSnapshot) []schedulerHeadCPU {
	if snapshot == nil || !snapshot.Complete {
		return nil
	}
	out := make([]schedulerHeadCPU, 0, len(snapshot.CPUs))
	for _, state := range snapshot.CPUs {
		out = append(out, state)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CPU < out[j].CPU })
	return out
}
