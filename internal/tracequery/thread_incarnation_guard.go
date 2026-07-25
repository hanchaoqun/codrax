package tracequery

import "fmt"

// threadIncarnationConflict is an exact, lifecycle-backed proof that one
// numeric TID denotes more than one task inside the selected duration domain.
// We intentionally do not infer generations from comm drift or inactivity.
type threadIncarnationConflict struct {
	PID           int
	PreviousLine  int
	PreviousTs    float64
	BoundaryLine  int
	BoundaryTs    float64
	Signal        string
	PriorDead     bool
	PriorDeadTs   float64
	PriorDeadLine int
	SourcePath    string
	// Local*Line: artifact-local physical lines, set at composite rebase
	// time (audit #36); zero when the published lines are already physical.
	LocalPreviousLine int
	LocalBoundaryLine int
}

// queryPIDIdentityFilter is the per-query, per-contributor authority used by
// PID-keyed scheduler value channels. The global conflict probe remains the
// conservative authority for process/resource composites; this filter only
// prevents one conflicting TID from erasing unrelated clean thread accounts.
//
// A false verdict always means "withhold this PID", never "pick a generation":
// a query crossing one of the PID's lifecycle boundaries still drops the
// whole PID account. A capped lifecycle audit drops every PID because the
// omitted boundary owner is unknowable.
type queryPIDIdentityFilter struct {
	idx        *Index
	q          Query
	global     *threadIncarnationConflict
	verdicts   map[int]bool
	suppressed map[int]bool
}

func newQueryPIDIdentityFilter(idx *Index, q Query, global *threadIncarnationConflict) *queryPIDIdentityFilter {
	return &queryPIDIdentityFilter{
		idx:        idx,
		q:          q,
		global:     global,
		verdicts:   map[int]bool{},
		suppressed: map[int]bool{},
	}
}

func (f *queryPIDIdentityFilter) allows(pid int) bool {
	// FILTERORDER (P2-1, 2026-07-25): the no-conflict early return comes
	// FIRST — on a clean trace this filter rejects nothing (§12.7 promise;
	// pid<=0 comm-only/malformed rows flowed before the narrowing batch and
	// must keep flowing). An identity-less row is withheld only while a
	// conflict is in scope: with no PID there is no way to prove it does not
	// belong to the conflicted task.
	if f == nil || f.global == nil {
		return true
	}
	if pid <= 0 {
		return false
	}
	if safe, ok := f.verdicts[pid]; ok {
		return safe
	}
	safe := false
	if f.global.Signal != "lifecycle_audit_truncated" {
		safe = threadGenerationScopeForQuery(f.idx, pid, f.q).known
	}
	f.verdicts[pid] = safe
	if !safe {
		f.suppressed[pid] = true
	}
	return safe
}

func (f *queryPIDIdentityFilter) suppressedPIDs() []int {
	if f == nil || len(f.suppressed) == 0 {
		return nil
	}
	return sortedIntSet(f.suppressed)
}

func (c *threadIncarnationConflict) reason() string {
	if c == nil {
		return ""
	}
	reason := fmt.Sprintf("thread_incarnation_conflict tid=%d signal=%s previous_line=%d boundary_line=%d boundary_ts=%.6f", c.PID, c.Signal, c.PreviousLine, c.BoundaryLine, c.BoundaryTs)
	if c.SourcePath != "" {
		reason += " source=" + c.SourcePath
		if c.LocalBoundaryLine > 0 && c.LocalBoundaryLine != c.BoundaryLine {
			reason += fmt.Sprintf(" local_previous_line=%d local_boundary_line=%d", c.LocalPreviousLine, c.LocalBoundaryLine)
		}
	}
	return reason
}

// threadIncarnationTracker recognizes only two hard lifecycle signals:
// sched_wakeup_new for an already-seen TID, and any trustworthy ftrace-header
// appearance after that TID was observed exiting in X/Z state. Header PID is a
// precise identity observation even when the payload is not a scheduler event;
// excluding it would let perf/resource rows bridge task generations. The
// tracker is streaming-safe.
type threadIncarnationTracker struct {
	seen              map[int]threadLifecyclePoint
	dead              map[int]threadLifecyclePoint
	generation        map[int]int
	generationEnabled bool
}

type threadLifecyclePoint struct {
	ts   float64
	line int
}

const threadIncarnationFailureCap = 64

func newThreadIncarnationTracker() *threadIncarnationTracker {
	return &threadIncarnationTracker{
		seen: map[int]threadLifecyclePoint{},
		dead: map[int]threadLifecyclePoint{},
	}
}

func newPerfGenerationTracker() *threadIncarnationTracker {
	tracker := newThreadIncarnationTracker()
	tracker.generationEnabled = true
	tracker.generation = map[int]int{}
	return tracker
}

func (t *threadIncarnationTracker) clone() *threadIncarnationTracker {
	copy := newThreadIncarnationTracker()
	if t == nil {
		return copy
	}
	if t.generationEnabled {
		copy.generationEnabled = true
		copy.generation = map[int]int{}
	}
	for pid, point := range t.seen {
		copy.seen[pid] = point
	}
	for pid, point := range t.dead {
		copy.dead[pid] = point
	}
	if copy.generationEnabled {
		for pid, generation := range t.generation {
			copy.generation[pid] = generation
		}
	}
	return copy
}

func (t *threadIncarnationTracker) cloneForPIDs(pids map[int]bool) *threadIncarnationTracker {
	copy := newThreadIncarnationTracker()
	if t == nil || len(pids) == 0 {
		return copy
	}
	if t.generationEnabled {
		copy.generationEnabled = true
		copy.generation = map[int]int{}
	}
	for pid := range pids {
		if point, ok := t.seen[pid]; ok {
			copy.seen[pid] = point
		}
		if point, ok := t.dead[pid]; ok {
			copy.dead[pid] = point
		}
		if copy.generationEnabled {
			generation := t.generation[pid]
			if generation <= 0 {
				continue
			}
			copy.generation[pid] = generation
		}
	}
	return copy
}

func (t *threadIncarnationTracker) generationForPID(pid int) int {
	if t == nil || pid <= 0 {
		return 0
	}
	if generation := t.generation[pid]; generation > 0 {
		return generation
	}
	if _, seen := t.seen[pid]; seen {
		return 1
	}
	return 0
}

func schedulerEventRolePIDs(ev Event) []int {
	var values []int
	add := func(pid int) {
		if pid <= 0 {
			return
		}
		for _, existing := range values {
			if existing == pid {
				return
			}
		}
		values = append(values, pid)
	}
	// Event.PID is parsed from the anchored ftrace row header. It is not a
	// comm/TGID heuristic, so it is valid hard evidence that this numeric TID
	// existed at this physical point for every recognized event family.
	add(ev.PID)
	switch ev.Type {
	case EventSchedSwitch:
		add(ev.PrevPID)
		add(ev.NextPID)
	case EventSchedWakeup, EventSchedWaking:
		add(ev.PID)
		add(ev.WakeePID)
	case EventSchedBlockedReason:
		add(ev.PID)
		add(ev.WakeePID)
	case EventCPUConstraint:
		if cf := ev.ConstraintFields; cf != nil && cf.Kind == "sched_migrate_task" {
			add(cf.PID)
		}
	}
	return values
}

func schedulerLifecycleResetPID(ev Event) (int, bool) {
	if schedWakeupStartsNewIncarnation(ev) && ev.WakeePID > 0 {
		return ev.WakeePID, true
	}
	if ev.Type == EventSchedSwitch && ev.PrevPID > 0 && stateFromPrevState(ev.PrevState) == StateDead {
		return ev.PrevPID, true
	}
	return 0, false
}

// NOTE (ENG audit #42, §29.25 处置委托 2026-07-10): there is deliberately no
// first-conflict-only observe() helper. observeAll permanently consumes the
// lifecycle evidence (delete(t.dead, pid)) for EVERY conflict it emits, so a
// consumer that keeps only conflicts[0] silently discards siblings whose
// window relevance may differ — the discarded conflict can never regenerate.
// Every consumer must iterate the full observeAll slice and apply its own
// relevance predicate per conflict.
func (t *threadIncarnationTracker) observeAll(ev Event, onlyPID int) []threadIncarnationConflict {
	return t.observeAllEligible(ev, func(pid int) bool {
		return pid > 0 && (onlyPID <= 0 || pid == onlyPID)
	})
}

func (t *threadIncarnationTracker) observeAllForPIDSet(ev Event, pids map[int]bool) []threadIncarnationConflict {
	if len(pids) == 0 {
		return nil
	}
	return t.observeAllEligible(ev, func(pid int) bool { return pid > 0 && pids[pid] })
}

func (t *threadIncarnationTracker) observeAllEligible(ev Event, eligible func(int) bool) []threadIncarnationConflict {
	if t == nil {
		return nil
	}
	roles := schedulerEventRolePIDs(ev)
	if len(roles) == 0 {
		return nil
	}
	var conflicts []threadIncarnationConflict
	addConflict := func(conflict threadIncarnationConflict) {
		for _, existing := range conflicts {
			if existing.PID == conflict.PID && existing.Signal == conflict.Signal && existing.BoundaryLine == conflict.BoundaryLine {
				return
			}
		}
		conflicts = append(conflicts, conflict)
		if t.generationEnabled {
			generation := t.generation[conflict.PID]
			if generation <= 0 {
				generation = 1
			}
			t.generation[conflict.PID] = generation + 1
		}
	}

	// A creation edge starts the wakee's new generation. It is a conflict only
	// when the numeric TID already had exact row-header/scheduler identity in
	// this scan.
	if schedWakeupStartsNewIncarnation(ev) && eligible(ev.WakeePID) {
		if previous := t.seen[ev.WakeePID]; previous.line > 0 {
			dead, priorDead := t.dead[ev.WakeePID]
			addConflict(threadIncarnationConflict{PID: ev.WakeePID, PreviousLine: previous.line, PreviousTs: previous.ts, BoundaryLine: ev.Line, BoundaryTs: ev.Ts, Signal: "sched_wakeup_new", PriorDead: priorDead, PriorDeadTs: dead.ts, PriorDeadLine: dead.line})
		}
		delete(t.dead, ev.WakeePID)
	}

	// A later appearance after an exact X/Z exit is a new generation even if
	// the capture omitted sched_wakeup_new. Do not compare the PID against the
	// same switch row that marks it dead; deadLine is installed below.
	for _, pid := range roles {
		if !eligible(pid) {
			continue
		}
		if dead, ok := t.dead[pid]; ok && dead.line > 0 {
			delete(t.dead, pid)
			addConflict(threadIncarnationConflict{PID: pid, PreviousLine: dead.line, PreviousTs: dead.ts, BoundaryLine: ev.Line, BoundaryTs: ev.Ts, Signal: "reappeared_after_dead", PriorDead: true, PriorDeadTs: dead.ts, PriorDeadLine: dead.line})
		}
	}

	for _, pid := range roles {
		if eligible(pid) {
			if t.generationEnabled && t.generation[pid] <= 0 {
				t.generation[pid] = 1
			}
			t.seen[pid] = threadLifecyclePoint{ts: ev.Ts, line: ev.Line}
		}
	}
	if ev.Type == EventSchedSwitch && eligible(ev.PrevPID) && stateFromPrevState(ev.PrevState) == StateDead {
		t.dead[ev.PrevPID] = threadLifecyclePoint{ts: ev.Ts, line: ev.Line}
	}
	if schedWakeupStartsNewIncarnation(ev) && eligible(ev.WakeePID) {
		delete(t.dead, ev.WakeePID)
	}
	return conflicts
}

func incarnationBoundaryInsideQuery(c *threadIncarnationConflict, q Query) bool {
	if c == nil {
		return false
	}
	if q.LineStart > 0 || q.LineEnd > 0 {
		if c.PriorDead && q.LineStart > 0 && c.PriorDeadLine < q.LineStart {
			return false
		}
		if q.LineStart > 0 && c.BoundaryLine < q.LineStart {
			return false
		}
		if q.LineStart > 0 && c.BoundaryLine == q.LineStart && c.PreviousLine < q.LineStart {
			return false
		}
		if q.LineEnd > 0 && c.BoundaryLine > q.LineEnd {
			return false
		}
		return true
	}
	if c.PriorDead && q.TimeStart > 0 && c.PriorDeadTs < q.TimeStart {
		return false
	}
	if q.TimeStart > 0 && c.BoundaryTs < q.TimeStart {
		return false
	}
	// Time windows are inclusive. A lifecycle cut exactly at time_start is
	// new-only only when every scheduler observation of the prior occupant is
	// before the boundary. If the predecessor row has the same timestamp, both
	// generations are selected and must fail closed; timestamp alone cannot
	// erase their authoritative physical-line order.
	if q.TimeStart > 0 && c.BoundaryTs == q.TimeStart && c.PreviousTs < q.TimeStart {
		return false
	}
	if q.TimeEnd > 0 && c.BoundaryTs > q.TimeEnd {
		return false
	}
	return true
}

// threadIncarnationConflictForQuery scans the lifecycle prefix needed by the
// selected window, but returns a conflict only when the generation boundary is
// inside that window. A boundary exactly at the left edge belongs to the new
// task only when the old occupant has no row at that inclusive timestamp. The
// right edge is inclusive throughout trace_query, so a lifecycle row there is
// a cross-generation identity conflict even if its new state has zero duration.
// threadIncarnationConflictsForQuery collects up to max in-window conflicts
// from the preserved parse-time audit (LIFEMULTI, 2026-07-24): one query
// window can cross several identity boundaries — the no_window.txt customer
// window carried two (50173/50174) — and disclosing only the first hid the
// rest. Preserved-list lane only: an empty return does NOT mean no conflict
// (the caller falls back to threadIncarnationConflictForQuery, whose
// full-scan tracker lane stays the single authority for index shapes without
// the preserved audit). Gate consumers keep the single-conflict probe —
// existence is all they read.
func threadIncarnationConflictsForQuery(idx *Index, q Query, onlyPID, max int) []threadIncarnationConflict {
	if idx == nil || max <= 0 {
		return nil
	}
	var out []threadIncarnationConflict
	for i := range idx.threadIncarnationFailures {
		preserved := &idx.threadIncarnationFailures[i]
		if (onlyPID <= 0 || preserved.PID == onlyPID) && incarnationBoundaryInsideQuery(preserved, q) {
			out = append(out, *preserved)
			if len(out) >= max {
				return out
			}
		}
	}
	if len(out) == 0 && idx.threadIncarnationFailuresCapped {
		return []threadIncarnationConflict{{PID: onlyPID, Signal: "lifecycle_audit_truncated"}}
	}
	return out
}

func threadIncarnationConflictForQuery(idx *Index, q Query, onlyPID int) *threadIncarnationConflict {
	if idx == nil {
		return nil
	}
	for i := range idx.threadIncarnationFailures {
		preserved := &idx.threadIncarnationFailures[i]
		if (onlyPID <= 0 || preserved.PID == onlyPID) && incarnationBoundaryInsideQuery(preserved, q) {
			copy := *preserved
			return &copy
		}
	}
	if idx.threadIncarnationFailuresCapped {
		return &threadIncarnationConflict{PID: onlyPID, Signal: "lifecycle_audit_truncated"}
	}
	// LT-HYG CANCEL-TAIL (§29.82 立案, 2026-07-14): this fallback lifecycle
	// audit is a full unsampled event scan, re-run by several builders of one
	// canceled Run (stats entry, timeline entry, latency entry, IPC entry) —
	// it dominated the rank/bundle post-fire tail. Post-fire the probe's only
	// consumers are faces the attach gates discard whole, so the early-exit
	// gate and the in-loop tick return nil without ever minting a truncated
	// verdict into a published face (禁半账 preserved by the attach gates;
	// nil/armed-live carriers read false and stay byte-identical).
	if q.runCancel.fired() {
		return nil
	}
	tracker := newThreadIncarnationTracker()
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			return nil
		}
		// Lifecycle identity needs the prefix before line_start/time_start to
		// prove that a creation/reappearance inside the window reused a TID.
		// Only the upper bound limits the audit; the boundary predicate below
		// decides whether a discovered generation cut is in the selected domain.
		if q.LineEnd > 0 && ev.Line > q.LineEnd {
			continue
		}
		if q.LineStart == 0 && q.LineEnd == 0 && q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			continue
		}
		// ENG audit #42 (§29.25 处置委托 2026-07-10): mirror the ForPIDSet twin —
		// one physical row can emit several conflicts whose window relevance
		// diverges (PriorDeadTs differs per conflict while the boundary is
		// row-shared), and observeAll consumes the lifecycle state for all of
		// them. Testing only the first conflict masked the relevant sibling
		// forever and let PID-keyed aggregates merge two task incarnations
		// with zero caveat.
		for _, conflict := range tracker.observeAll(ev, onlyPID) {
			if incarnationBoundaryInsideQuery(&conflict, q) {
				copy := conflict
				return &copy
			}
		}
	}
	return nil
}

// threadIncarnationConflictForPIDSet is the contributor-scoped twin of
// threadIncarnationConflictForQuery. It is for views whose complete numeric
// TID dependency set is known before aggregation (for example perf_timeline).
// An empty set has no numeric identity dependency and is therefore safe. A
// capped lifecycle audit remains fail-closed for every non-empty set because
// the omitted conflicts may include one of its contributors.
func threadIncarnationConflictForPIDSet(idx *Index, q Query, pids map[int]bool) *threadIncarnationConflict {
	if idx == nil || len(pids) == 0 {
		return nil
	}
	for i := range idx.threadIncarnationFailures {
		preserved := &idx.threadIncarnationFailures[i]
		if pids[preserved.PID] && incarnationBoundaryInsideQuery(preserved, q) {
			copy := *preserved
			return &copy
		}
	}
	if idx.threadIncarnationFailuresCapped {
		return &threadIncarnationConflict{Signal: "lifecycle_audit_truncated"}
	}
	// LT-HYG §29.84 残留④ (2026-07-14): same cooperative-cancel contract as
	// the ForQuery twin above — this fallback lifecycle audit is a full
	// unsampled event scan re-run by the perf_timeline entry and the stats
	// resource/plugin family gates of one canceled Run. Post-fire the probe's
	// only consumers are faces the attach gates discard whole (perf_timeline
	// case gate; window_stats case gates), so the early-exit gate and the
	// in-loop tick return nil without ever minting a truncated verdict into a
	// published face (禁半账 preserved by the attach gates; nil/armed-live
	// carriers read false and stay byte-identical).
	if q.runCancel.fired() {
		return nil
	}
	tracker := newThreadIncarnationTracker()
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			return nil
		}
		if q.LineEnd > 0 && ev.Line > q.LineEnd {
			continue
		}
		if q.LineStart == 0 && q.LineEnd == 0 && q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			continue
		}
		for _, conflict := range tracker.observeAll(ev, 0) {
			if pids[conflict.PID] && incarnationBoundaryInsideQuery(&conflict, q) {
				copy := conflict
				return &copy
			}
		}
	}
	return nil
}

// mergeThreadIncarnationFailures preserves a bounded, deterministic union of
// physical child/file-order proofs and canonical merged-timeline proofs. The
// former catches poison that timestamp sorting would erase; the latter catches
// a generation cut whose old and new rows live in different artifacts.
func mergeThreadIncarnationFailures(dst []threadIncarnationConflict, dstCapped bool, src []threadIncarnationConflict, srcCapped bool, limit int) ([]threadIncarnationConflict, bool) {
	if limit <= 0 || limit > threadIncarnationFailureCap {
		limit = threadIncarnationFailureCap
	}
	capped := dstCapped || srcCapped
	for _, candidate := range src {
		duplicate := -1
		for i := range dst {
			existing := &dst[i]
			if existing.PID == candidate.PID && existing.Signal == candidate.Signal && existing.PreviousLine == candidate.PreviousLine && existing.BoundaryLine == candidate.BoundaryLine && existing.BoundaryTs == candidate.BoundaryTs {
				duplicate = i
				break
			}
		}
		if duplicate >= 0 {
			existing := &dst[duplicate]
			if existing.PreviousTs == 0 && candidate.PreviousTs != 0 {
				existing.PreviousTs = candidate.PreviousTs
			}
			if existing.SourcePath == "" {
				existing.SourcePath = candidate.SourcePath
				existing.LocalPreviousLine = candidate.LocalPreviousLine
				existing.LocalBoundaryLine = candidate.LocalBoundaryLine
			}
			if !existing.PriorDead && candidate.PriorDead {
				existing.PriorDead = true
				existing.PriorDeadTs = candidate.PriorDeadTs
				existing.PriorDeadLine = candidate.PriorDeadLine
			}
			continue
		}
		if len(dst) >= limit {
			capped = true
			continue
		}
		dst = append(dst, candidate)
	}
	return dst, capped
}

func threadIncarnationConflictsFromEvents(events []Event, q Query, limit int) ([]threadIncarnationConflict, bool) {
	if limit <= 0 || limit > threadIncarnationFailureCap {
		limit = threadIncarnationFailureCap
	}
	tracker := newThreadIncarnationTracker()
	out := make([]threadIncarnationConflict, 0, min(limit, 8))
	truncated := false
	for _, ev := range events {
		if q.LineEnd > 0 && ev.Line > q.LineEnd {
			continue
		}
		if q.LineStart == 0 && q.LineEnd == 0 && q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			continue
		}
		for _, conflict := range tracker.observeAll(ev, 0) {
			if !incarnationBoundaryInsideQuery(&conflict, q) {
				continue
			}
			if len(out) == limit {
				truncated = true
				continue
			}
			out = append(out, conflict)
		}
	}
	return out, truncated
}
