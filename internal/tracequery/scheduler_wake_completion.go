package tracequery

// State accounting accepts the first wake transition. Some native producers
// publish blocked_reason beside the completed wake, not the earlier waking
// attempt. Keep that optional metadata provenance separate from elapsed time.
// This carrier is local to one query/index, not a persistent source receipt.
type schedulerWakeCompletionScope struct {
	idx                *Index
	start, end         float64
	lineStart, lineEnd int
	selectedPID        int
	eventIDs           []int
	indexed, built     bool
	proofs             map[schedulerWakeCompletionKey]schedulerWakeCompletionProof
}

type schedulerWakeCompletionKey struct {
	pid, line int
	ts        float64
}

type schedulerWakeCompletionProof struct {
	source            string
	first, completion schedulerWakeCompletionKey
	// The extra metadata lane may not cross the next physical scheduler or
	// lifecycle boundary. The legacy direct-wake +5us lane is unchanged.
	barrierTs   float64
	barrierLine int
}

func withSchedulerWakeCompletions(idx *Index, q Query, pid int, ids []int, indexed bool) Query {
	s := q.schedulerWakeCompletions
	if s != nil && s.idx == idx && s.start == q.TimeStart && s.end == q.TimeEnd &&
		s.lineStart == q.LineStart && s.lineEnd == q.LineEnd && (s.selectedPID == 0 || s.selectedPID == pid) {
		return q
	}
	q.schedulerWakeCompletions = &schedulerWakeCompletionScope{idx: idx, start: q.TimeStart, end: q.TimeEnd,
		lineStart: q.LineStart, lineEnd: q.LineEnd, selectedPID: pid, eventIDs: ids, indexed: indexed}
	return q
}

func (s *schedulerWakeCompletionScope) build(q Query) {
	if s == nil || s.built || s.idx == nil {
		return
	}
	s.built = true
	s.proofs = make(map[schedulerWakeCompletionKey]schedulerWakeCompletionProof)
	type laneKey struct {
		source string
		pid    int
	}
	type pendingWake struct {
		first      Event
		completion Event
		count      int
		invalid    bool
	}
	open := make(map[laneKey]pendingWake)
	finish := func(key laneKey, barrier *Event) {
		p, ok := open[key]
		delete(open, key)
		if !ok || p.invalid || p.first.Type != EventSchedWaking || p.count != 1 {
			return
		}
		// Reuse the native per-index lifecycle ledger at the pair itself.
		// A later incarnation elsewhere in a wide query must not invalidate
		// these two historical endpoints; a boundary between them must.
		for _, pid := range []int{key.pid, p.first.PID} {
			if pid == 0 { // Idle is separately bound to its physical CPU below.
				continue
			}
			if !threadGenerationScopeAt(s.idx, pid, p.first.Ts, p.first.Line).contains(p.completion.Ts, p.completion.Line) {
				return
			}
		}
		proof := schedulerWakeCompletionProof{source: key.source,
			first:      schedulerWakeCompletionKey{key.pid, p.first.Line, p.first.Ts},
			completion: schedulerWakeCompletionKey{key.pid, p.completion.Line, p.completion.Ts}}
		if barrier != nil {
			proof.barrierTs, proof.barrierLine = barrier.Ts, barrier.Line
		}
		s.proofs[proof.first] = proof
	}
	visitEventsInTimestampOrder(s.idx, s.eventIDs, s.indexed, q.runCancel, func(ev Event) {
		source, ok := tracePairingSourceIdentity(s.idx, ev)
		if !ok {
			return
		}
		if ev.Type == EventSchedSwitch {
			finish(laneKey{source, ev.PrevPID}, &ev)
			finish(laneKey{source, ev.NextPID}, &ev)
			return
		}
		if ev.Type != EventSchedWaking && ev.Type != EventSchedWakeup || ev.WakeePID <= 0 {
			return
		}
		key := laneKey{source, ev.WakeePID}
		if schedWakeupStartsNewIncarnation(ev) {
			finish(key, &ev)
			// The new incarnation is already runnable, so no later waking in
			// this cycle may masquerade as its first transition.
			open[key] = pendingWake{invalid: true}
			return
		}
		p, exists := open[key]
		if !exists {
			p.first = ev
			p.invalid = ev.Type != EventSchedWaking || ev.Line <= 0 || ev.PID < 0
			open[key] = p
			return
		}
		if ev.Type == EventSchedWaking || ev.PID != p.first.PID ||
			(ev.TGID > 0 && p.first.TGID > 0 && ev.TGID != p.first.TGID) ||
			(ev.PID == 0 && (ev.CPU < 0 || p.first.CPU < 0 || ev.CPU != p.first.CPU)) ||
			ev.Ts < p.first.Ts || ev.Line <= p.first.Line {
			p.invalid = true
		}
		if ev.Type == EventSchedWakeup {
			p.count++
			p.completion = ev
		}
		open[key] = p
	})
	if q.runCancel.fired() {
		s.proofs = nil // Never publish a partially scanned uniqueness verdict.
		return
	}
	for key := range open {
		finish(key, nil)
	}
}

func schedulerWakeCompletionFor(idx *Index, q Query, pid int, end float64, endLine int) *schedulerWakeCompletionProof {
	s := q.schedulerWakeCompletions
	if s == nil || idx == nil || s.idx != idx || endLine <= 0 || s.start != q.TimeStart || s.end != q.TimeEnd ||
		s.lineStart != q.LineStart || s.lineEnd != q.LineEnd || (s.selectedPID != 0 && s.selectedPID != pid) {
		return nil
	}
	s.build(q)
	proof, ok := s.proofs[schedulerWakeCompletionKey{pid, endLine, end}]
	if !ok {
		return nil
	}
	// A whole-index scan is only an index. Both physical endpoints still
	// belong to this precise query, including an explicit line-domain query.
	for _, endpoint := range []schedulerWakeCompletionKey{proof.first, proof.completion} {
		if !timeInWindow(endpoint.ts, q) || !eventLineInWindow(Event{Line: endpoint.line}, q) {
			return nil
		}
	}
	return &proof
}

func (p *schedulerWakeCompletionProof) admits(idx *Index, ev *Event) bool {
	if p == nil || ev == nil || ev.Ts < p.completion.ts || ev.Ts > wakeupClosingUpperBound(p.completion.ts) ||
		ev.Line <= p.completion.line {
		return false
	}
	if p.barrierLine > 0 && (ev.Ts > p.barrierTs || ev.Ts == p.barrierTs && ev.Line >= p.barrierLine) {
		return false
	}
	source, ok := tracePairingSourceIdentity(idx, *ev)
	return ok && source == p.source
}

func blockedReasonRefinementUnavailableWithCompletion(idx *Index, q Query, pid int, start, end float64, physical bool, proof *schedulerWakeCompletionProof) bool {
	if blockedReasonRefinementUnavailableForInterval(idx, q, pid, start, end, physical) {
		return true
	}
	if proof == nil {
		return false
	}
	// The second interval is the existing 5us completion envelope only. Do
	// not include the unmeasured gap between first transition and completion.
	metadataQ := q
	if metadataQ.LineStart <= proof.completion.line {
		metadataQ.LineStart = proof.completion.line + 1
	}
	upper := wakeupClosingUpperBound(proof.completion.ts)
	if proof.barrierLine > 0 {
		if metadataQ.LineEnd == 0 || metadataQ.LineEnd >= proof.barrierLine {
			metadataQ.LineEnd = proof.barrierLine - 1
		}
		if proof.barrierTs < upper {
			upper = proof.barrierTs
		}
	}
	return blockedReasonRefinementUnavailableInDomain(idx, metadataQ, pid, proof.completion.ts, upper, true)
}

func matchBlockedReasonForQueryInterval(idx *Index, q Query, in map[int][]Event, thread ThreadRef, start, end float64, endLine int, physical bool) blockedReasonIntervalMatch {
	proof := schedulerWakeCompletionFor(idx, q, thread.PID, end, endLine)
	if blockedReasonRefinementUnavailableWithCompletion(idx, q, thread.PID, start, end, physical, proof) {
		return blockedReasonIntervalMatch{}
	}
	matcher := newBlockedReasonIntervalMatcherAtClosure(thread, start, end, physical)
	matcher.completion, matcher.completionIndex = proof, idx
	for i := range in[thread.PID] {
		matcher.consider(&in[thread.PID][i])
	}
	return matcher.result()
}
