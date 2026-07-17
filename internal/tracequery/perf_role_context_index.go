package tracequery

import (
	"sort"
	"strings"
)

// perfRoleContextScanProbe is test-only query plumbing. It pins the resource
// shape without package globals: one root-rank attachment may build one full
// event index regardless of candidate-seat count or caller-supplied limit.
type perfRoleContextScanProbe struct {
	FullEventScans         int
	FullEventsVisited      int
	RoleLookups            int
	LifecycleCandidateTIDs int
}

// perfRoleContextIndex is a per-root-rank immutable selection index. It
// replaces the former role × Events and role × lifecycle replays with one
// cooperative pass, then aggregates only ascending candidate ordinals.
// PerfContext values are deliberately NOT cached: each publication face gets
// fresh pointers and slices from computePerfContextForOrdinalsWithCaveats.
type perfRoleContextIndex struct {
	idx    *Index
	q      Query
	ledger *perfIdentityLedger

	byKey       map[perfThreadKey][]int
	byCPU       map[int][]int
	keysByTID   map[int][]perfThreadKey
	keysByAlias map[string][]perfThreadKey
	truncated   []perfThreadKey

	conflicts          map[int][]threadIncarnationConflict
	conflictsCapped    map[int]bool
	allConflictsCapped bool
	complete           bool
}

func newPerfRoleContextIndex(idx *Index, q Query) *perfRoleContextIndex {
	r := &perfRoleContextIndex{
		idx: idx, q: q,
		byKey: map[perfThreadKey][]int{}, byCPU: map[int][]int{},
		keysByTID: map[int][]perfThreadKey{}, keysByAlias: map[string][]perfThreadKey{},
		conflicts: map[int][]threadIncarnationConflict{}, conflictsCapped: map[int]bool{},
	}
	if idx == nil || q.runCancel.sample() {
		return r
	}
	r.ledger = ensurePerfIdentityLedger(idx)
	if q.runCancel.sample() {
		return r
	}
	for i := range r.ledger.records {
		if q.runCancel.tick() {
			return r
		}
		record := &r.ledger.records[i]
		key := record.key
		if record.identity.TID > 0 {
			// Ledger records are unique and sorted by key. Direct append keeps
			// a common TID/alias population linear instead of doing a growing
			// duplicate search for every generation.
			r.keysByTID[record.identity.TID] = append(r.keysByTID[record.identity.TID], key)
		}
		if record.aliasTruncated {
			r.truncated = append(r.truncated, key)
			continue
		}
		for _, alias := range record.selectorAliases {
			normalized := perfRoleAliasKey(alias)
			if normalized != "" {
				keys := r.keysByAlias[normalized]
				if len(keys) == 0 || keys[len(keys)-1] != key {
					r.keysByAlias[normalized] = append(keys, key)
				}
			}
		}
	}
	// Lifecycle replay is contributor-scoped to numeric TIDs that can enter
	// a perf selector (proved records plus identity-withheld candidates).
	// Unrelated scheduler churn must not grow tracker seen/dead state.
	candidateTIDs := make(map[int]bool, len(r.keysByTID)+len(r.ledger.candidates))
	for tid := range r.keysByTID {
		candidateTIDs[tid] = true
	}
	for _, candidate := range r.ledger.candidates {
		if q.runCancel.tick() {
			return r
		}
		if candidate.tid > 0 {
			candidateTIDs[candidate.tid] = true
		}
	}
	if q.perfRoleContextScanProbe != nil {
		q.perfRoleContextScanProbe.LifecycleCandidateTIDs = len(candidateTIDs)
	}
	for _, conflict := range idx.threadIncarnationFailures {
		if q.runCancel.tick() {
			return r
		}
		if candidateTIDs[conflict.PID] {
			r.addConflict(conflict)
		}
	}
	r.allConflictsCapped = idx.threadIncarnationFailuresCapped
	tracker := newThreadIncarnationTracker()
	if q.perfRoleContextScanProbe != nil {
		q.perfRoleContextScanProbe.FullEventScans++
	}
	for ordinal := range idx.Events {
		if q.runCancel.tick() {
			return r
		}
		if q.perfRoleContextScanProbe != nil {
			q.perfRoleContextScanProbe.FullEventsVisited++
		}
		ev := idx.Events[ordinal]
		for _, conflict := range tracker.observeAllForPIDSet(ev, candidateTIDs) {
			r.addConflict(conflict)
		}
		if ev.Type != EventPerfSample {
			continue
		}
		key, _, ok := r.ledger.identityForEventOrdinalBorrowed(ordinal)
		if ok {
			r.byKey[key] = append(r.byKey[key], ordinal)
		}
		if cpu, ok := perfSampleOnCPUExecutionCPU(ev); ok {
			r.byCPU[cpu] = append(r.byCPU[cpu], ordinal)
		}
	}
	r.complete = true
	return r
}

func appendPerfRoleKeyUnique(keys []perfThreadKey, key perfThreadKey) []perfThreadKey {
	for _, existing := range keys {
		if existing == key {
			return keys
		}
	}
	return append(keys, key)
}

func perfRoleAliasKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (r *perfRoleContextIndex) addConflict(conflict threadIncarnationConflict) {
	if conflict.PID <= 0 {
		return
	}
	items := r.conflicts[conflict.PID]
	for i := range items {
		if items[i].Signal == conflict.Signal && items[i].BoundaryLine == conflict.BoundaryLine {
			if !items[i].PriorDead && conflict.PriorDead {
				items[i].PriorDead = true
				items[i].PriorDeadTs = conflict.PriorDeadTs
				items[i].PriorDeadLine = conflict.PriorDeadLine
				r.conflicts[conflict.PID] = items
			}
			return
		}
	}
	if len(items) >= threadIncarnationFailureCap {
		r.conflictsCapped[conflict.PID] = true
		return
	}
	r.conflicts[conflict.PID] = append(items, conflict)
}

func (r *perfRoleContextIndex) hasLifecycleConflict(tid int, q Query) bool {
	if tid <= 0 {
		return false
	}
	if r.allConflictsCapped || r.conflictsCapped[tid] {
		return true
	}
	for i := range r.conflicts[tid] {
		if incarnationBoundaryInsideQuery(&r.conflicts[tid][i], q) {
			return true
		}
	}
	return false
}

func perfRoleOrdinalInQuery(idx *Index, ordinal int, q Query, executionOnly bool) bool {
	if idx == nil || ordinal < 0 || ordinal >= len(idx.Events) {
		return false
	}
	ev := idx.Events[ordinal]
	if ev.Type != EventPerfSample || !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
		return false
	}
	if !executionOnly {
		return true
	}
	_, ok := perfSampleOnCPUExecutionCPU(ev)
	return ok
}

func (r *perfRoleContextIndex) keyHasSelectedOrdinal(key perfThreadKey, q Query, executionOnly bool) bool {
	for _, ordinal := range r.byKey[key] {
		if r.q.runCancel.tick() {
			return false
		}
		if perfRoleOrdinalInQuery(r.idx, ordinal, q, executionOnly) {
			return true
		}
	}
	return false
}

func (r *perfRoleContextIndex) unknownCandidateWithheld(role ThreadRef, q Query, executionOnly bool) bool {
	selector := threadSelector{Name: strings.TrimSpace(role.Comm)}
	if role.PID > 0 {
		selector.HasPID, selector.PID = true, role.PID
	}
	for _, candidate := range r.ledger.candidates {
		if r.q.runCancel.tick() {
			return false
		}
		if !perfRoleOrdinalInQuery(r.idx, candidate.ordinal, q, executionOnly) {
			continue
		}
		if selector.HasPID {
			if candidate.tid == selector.PID {
				return true
			}
			continue
		}
		if strings.TrimSpace(selector.Name) == "" || threadSelectorMatchesName(selector, candidate.aliasA) || threadSelectorMatchesName(selector, candidate.aliasB) {
			return true
		}
	}
	return false
}

func (r *perfRoleContextIndex) roleOrdinals(role ThreadRef, q Query, executionOnly bool) ([]int, []string) {
	if r == nil || !r.complete {
		return nil, nil
	}
	if r.q.perfRoleContextScanProbe != nil {
		r.q.perfRoleContextScanProbe.RoleLookups++
	}
	withheld := r.unknownCandidateWithheld(role, q, executionOnly)
	var candidateKeys []perfThreadKey
	if role.PID > 0 {
		candidateKeys = r.keysByTID[role.PID]
	} else {
		candidateKeys = r.keysByAlias[perfRoleAliasKey(role.Comm)]
		// The historical selector verdict withholds every comm-only lookup
		// when an in-window record overflowed its complete alias authority.
		for _, key := range r.truncated {
			if r.q.runCancel.tick() {
				return nil, nil
			}
			if r.keyHasSelectedOrdinal(key, q, executionOnly) {
				withheld = true
				break
			}
		}
	}
	var selected []perfThreadKey
	for _, key := range candidateKeys {
		if r.q.runCancel.tick() {
			return nil, nil
		}
		if r.keyHasSelectedOrdinal(key, q, executionOnly) {
			selected = appendPerfRoleKeyUnique(selected, key)
		}
	}
	if withheld {
		return nil, []string{perfThreadSelectorWithheldCaveat(role.PID > 0)}
	}
	if len(selected) != 1 {
		return nil, nil
	}
	key := selected[0]
	if r.hasLifecycleConflict(key.TID, q) {
		return nil, nil
	}
	ordinals := make([]int, 0, len(r.byKey[key]))
	for _, ordinal := range r.byKey[key] {
		if r.q.runCancel.tick() {
			return nil, nil
		}
		if perfRoleOrdinalInQuery(r.idx, ordinal, q, executionOnly) {
			ordinals = append(ordinals, ordinal)
		}
	}
	return ordinals, nil
}

func (r *perfRoleContextIndex) contextForThread(thread ThreadRef, start, end float64, max int, executionOnly bool) *PerfContext {
	if r == nil || (thread.PID <= 0 && strings.TrimSpace(thread.Comm) == "") {
		return nil
	}
	sub := queryForPerfContextWindow(r.q, start, end)
	ordinals, caveats := r.roleOrdinals(thread, sub, executionOnly)
	if ordinals == nil {
		ordinals = make([]int, 0)
	}
	return computePerfContextForOrdinalsWithCaveats(r.idx, sub, max, nil, caveats, ordinals)
}

func (r *perfRoleContextIndex) contextForCPU(cpu int, start, end float64, max int) *PerfContext {
	if r == nil || !r.complete || cpu < 0 {
		return nil
	}
	sub := queryForPerfContextWindow(r.q, start, end)
	ordinals := r.byCPU[cpu]
	if ordinals == nil {
		ordinals = make([]int, 0)
	}
	return computePerfContextForOrdinalsWithCaveats(r.idx, sub, max, nil, nil, ordinals)
}

func (r *perfRoleContextIndex) contextForThreads(threads map[int]ThreadRef, max int, executionOnly bool) *PerfContext {
	roles := perfRoleThreads(threads)
	if r == nil || !r.complete || len(roles) == 0 {
		return nil
	}
	var ordinals []int
	var caveats []string
	for _, role := range roles {
		if r.q.runCancel.tick() {
			return nil
		}
		selected, localCaveats := r.roleOrdinals(role, r.q, executionOnly)
		for _, caveat := range localCaveats {
			caveats = appendUniquePerfCaveat(caveats, caveat)
		}
		for _, ordinal := range selected {
			ordinals = append(ordinals, ordinal)
		}
	}
	sort.Ints(ordinals)
	ordinals = compactSortedPerfRoleOrdinals(ordinals)
	if ordinals == nil {
		ordinals = make([]int, 0)
	}
	return computePerfContextForOrdinalsWithCaveats(r.idx, r.q, max, nil, caveats, ordinals)
}

func (r *perfRoleContextIndex) contextForCPUs(cpus map[int]bool, max int) *PerfContext {
	if r == nil || !r.complete || len(cpus) == 0 {
		return nil
	}
	var ordinals []int
	for cpu := range cpus {
		if r.q.runCancel.tick() {
			return nil
		}
		for _, ordinal := range r.byCPU[cpu] {
			if r.q.runCancel.tick() {
				return nil
			}
			ordinals = append(ordinals, ordinal)
		}
	}
	sort.Ints(ordinals)
	if ordinals == nil {
		ordinals = make([]int, 0)
	}
	return computePerfContextForOrdinalsWithCaveats(r.idx, r.q, max, nil, nil, ordinals)
}

func compactSortedPerfRoleOrdinals(ordinals []int) []int {
	if len(ordinals) < 2 {
		return ordinals
	}
	write := 1
	for read := 1; read < len(ordinals); read++ {
		if ordinals[read] == ordinals[write-1] {
			continue
		}
		ordinals[write] = ordinals[read]
		write++
	}
	return ordinals[:write]
}
