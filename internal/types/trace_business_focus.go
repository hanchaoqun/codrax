package types

import "strings"

// TraceBusinessFocusStatus is a read-only status, never an authority receipt.
// None/Cleared have no selected instance and may use the legacy no-reference
// supplement lane. Conflict/Invalid must not fall back to an arbitrary explored
// window. Only Selected is accompanied by a current private receipt.
type TraceBusinessFocusStatus string

const (
	TraceBusinessFocusNone     TraceBusinessFocusStatus = "none"
	TraceBusinessFocusCleared  TraceBusinessFocusStatus = "cleared"
	TraceBusinessFocusSelected TraceBusinessFocusStatus = "selected"
	TraceBusinessFocusConflict TraceBusinessFocusStatus = "conflict"
	TraceBusinessFocusInvalid  TraceBusinessFocusStatus = "invalid"
)

// TraceBusinessFocusDispatch is an opaque in-process dispatch ticket. Its
// identity cannot be reconstructed from JSON or an accepted closure mirror.
type TraceBusinessFocusDispatch struct {
	token *traceBusinessFocusDispatchToken
}

// Non-zero-sized so independent epochs always have distinct pointer identity.
type traceBusinessFocusEpoch struct{ marker byte }
type traceBusinessFocusDispatchToken struct {
	epoch  *traceBusinessFocusEpoch
	parent *traceBusinessFocusDispatchToken
}
type traceBusinessFocusDecision struct {
	dispatch             *traceBusinessFocusDispatchToken
	completionGeneration uint64
	ref                  *TraceBusinessSpanRef
}
type traceBusinessFocusState struct {
	epoch   *traceBusinessFocusEpoch
	lineage *traceBusinessFocusDispatchToken
	ticket  *traceBusinessFocusDispatchToken
	pending *traceBusinessFocusDecision
	active  *traceBusinessFocusDecision
	// settled retains only dispatch lineage/choice identity for failed-choice
	// suppression at the parent. It is never returned as execution authority.
	settled          *traceBusinessFocusDecision
	settledSucceeded bool
	status           TraceBusinessFocusStatus
}

func (m *MutableState) BeginTraceBusinessFocusDispatch() TraceBusinessFocusDispatch {
	if m == nil {
		return TraceBusinessFocusDispatch{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := &m.traceBusinessFocus
	if s.epoch == nil {
		s.epoch = &traceBusinessFocusEpoch{marker: 1}
	}
	if s.pending != nil || s.active != nil || s.status == TraceBusinessFocusSelected {
		s.status = TraceBusinessFocusCleared
	}
	s.pending, s.active, s.settled = nil, nil, nil
	s.settledSucceeded = false
	s.ticket = &traceBusinessFocusDispatchToken{epoch: s.epoch, parent: s.lineage}
	return TraceBusinessFocusDispatch{token: s.ticket}
}

// AcceptInvestigationCompleteWithBusinessSpanRef is the model-tool acceptance
// tail, not a dispatch-success signal. It atomically records the ordinary
// completion generation and a private pending choice. nil is an explicit
// no-choice decision. Failed validation writes neither completion nor focus.
func (m *MutableState) AcceptInvestigationCompleteWithBusinessSpanRef(reason string, ref *TraceBusinessSpanRef) bool {
	if m == nil || (ref != nil && !m.TraceBusinessSpanRefCurrent(*ref)) {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ref != nil && !m.traceBusinessSpanRefRegisteredLocked(*ref) {
		return false
	}
	reason = strings.TrimSpace(reason)
	m.investigationComplete = true
	m.investigationCompleteReason = reason
	if reason != "" {
		m.retainedInvestigationCompleteReason = reason
	}
	m.investigationCompleteGeneration++
	decision := &traceBusinessFocusDecision{dispatch: m.traceBusinessFocus.ticket, completionGeneration: m.investigationCompleteGeneration}
	if ref != nil {
		copied := *ref
		decision.ref = &copied
	}
	if m.traceBusinessFocus.active != nil {
		m.traceBusinessFocus.status = TraceBusinessFocusCleared
	}
	m.traceBusinessFocus.active = nil
	m.traceBusinessFocus.settled = nil
	m.traceBusinessFocus.settledSucceeded = false
	m.traceBusinessFocus.pending = decision
	m.bumpAnswerSurfaceRevisionLocked()
	return true
}

// SettleTraceBusinessFocusDispatch is called only after the worker returns.
// A failure consumes the ticket too: a later replay cannot revive its pending
// choice. The caller supplies exact worker/error/output/cancellation checks.
func (m *MutableState) SettleTraceBusinessFocusDispatch(ticket TraceBusinessFocusDispatch, succeeded bool) {
	if m == nil || ticket.token == nil {
		return
	}
	m.mu.RLock()
	pending := m.traceBusinessFocus.pending
	m.mu.RUnlock()
	current := pending == nil || pending.ref == nil || m.TraceBusinessSpanRefCurrent(*pending.ref)
	m.mu.Lock()
	defer m.mu.Unlock()
	s := &m.traceBusinessFocus
	if !m.traceBusinessFocusTicketCurrentLocked(ticket) || s.pending != pending {
		return
	}
	s.ticket = nil
	s.pending = nil
	s.active = nil
	s.settled = nil
	s.settledSucceeded = false
	if pending == nil || pending.dispatch != ticket.token || pending.completionGeneration != m.investigationCompleteGeneration {
		return
	}
	s.settled = pending
	if !succeeded || !current || (pending.ref != nil && !m.traceBusinessSpanRefRegisteredLocked(*pending.ref)) {
		if pending.ref != nil {
			s.status = TraceBusinessFocusInvalid
		} else {
			s.status = TraceBusinessFocusCleared
		}
		return
	}
	s.active = pending
	s.settledSucceeded = true
	if pending.ref == nil {
		s.status = TraceBusinessFocusCleared
	} else {
		s.status = TraceBusinessFocusSelected
	}
}

// SettleParallelTraceBusinessFocusDispatch keeps old closure merges separate
// from the new execution choice. Only eligible (winner/required) workers can
// grant a choice. Other successfully completed workers can veto a conflicting
// choice, but canceled/failed workers and mere published tools cannot grant or
// veto. Distinct tokens for the same original physical instance agree.
func (m *MutableState) SettleParallelTraceBusinessFocusDispatch(ticket TraceBusinessFocusDispatch, eligible, successful []*MutableState, succeeded bool) {
	if m == nil || ticket.token == nil {
		return
	}
	var decisions []*traceBusinessFocusDecision
	var selected *traceBusinessFocusDecision
	invalidEligible, selectedEligible := false, false
	eligibleSet := make(map[*MutableState]bool, len(eligible))
	for _, fork := range eligible {
		eligibleSet[fork] = true
	}
	seen := make(map[*MutableState]bool)
	for _, fork := range append(append([]*MutableState(nil), successful...), eligible...) {
		if fork == nil || seen[fork] {
			continue
		}
		seen[fork] = true
		fork.mu.RLock()
		decision := fork.traceBusinessFocus.settled
		status := fork.traceBusinessFocus.status
		workerSucceeded := fork.traceBusinessFocus.settledSucceeded
		forkEpoch := fork.traceBusinessFocus.epoch
		fork.mu.RUnlock()
		if decision == nil || decision.dispatch == nil || decision.dispatch.parent != ticket.token ||
			forkEpoch != ticket.token.epoch || (status != TraceBusinessFocusSelected && status != TraceBusinessFocusCleared && status != TraceBusinessFocusInvalid) {
			continue
		}
		if eligibleSet[fork] && decision.ref != nil {
			selectedEligible = true
		}
		if !workerSucceeded || status == TraceBusinessFocusInvalid || (decision.ref != nil && !fork.TraceBusinessSpanRefCurrent(*decision.ref)) {
			if eligibleSet[fork] && decision.ref != nil {
				invalidEligible = true
			}
			continue
		}
		decisions = append(decisions, decision)
		if eligibleSet[fork] && selected == nil {
			selected = decision
		}
	}
	conflict := false
	for _, decision := range decisions {
		if selected != nil && !sameTraceBusinessFocusChoice(selected, decision) {
			conflict = true
		}
	}
	current := selected == nil || selected.ref == nil || m.TraceBusinessSpanRefCurrent(*selected.ref)
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.traceBusinessFocusTicketCurrentLocked(ticket) {
		return
	}
	s := &m.traceBusinessFocus
	s.ticket, s.pending, s.active, s.settled = nil, nil, nil, nil
	s.settledSucceeded = false
	if !succeeded {
		if selectedEligible || invalidEligible {
			s.status = TraceBusinessFocusInvalid
		}
		return
	}
	if invalidEligible {
		s.status = TraceBusinessFocusInvalid
		return
	}
	if selected == nil {
		return
	}
	if conflict {
		s.status = TraceBusinessFocusConflict
		return
	}
	if !current || (selected.ref != nil && !m.traceBusinessSpanRefRegisteredLocked(*selected.ref)) {
		s.status = TraceBusinessFocusInvalid
		return
	}
	s.active = selected
	s.settled = selected
	s.settledSucceeded = true
	if selected.ref == nil {
		s.status = TraceBusinessFocusCleared
	} else {
		s.status = TraceBusinessFocusSelected
	}
}

// AcceptedTraceBusinessFocus returns only a worker-confirmed choice and
// rechecks its original physical publication. It never reads retained closure
// reasons, TurnA display mirrors, candidate order, or serialized tokens.
func (m *MutableState) AcceptedTraceBusinessFocus() (TraceBusinessFocusStatus, TraceBusinessSpanRef) {
	if m == nil {
		return TraceBusinessFocusNone, TraceBusinessSpanRef{}
	}
	m.mu.RLock()
	status, active := m.traceBusinessFocus.status, m.traceBusinessFocus.active
	m.mu.RUnlock()
	if status == "" {
		status = TraceBusinessFocusNone
	}
	if status != TraceBusinessFocusSelected || active == nil || active.ref == nil {
		return status, TraceBusinessSpanRef{}
	}
	if !m.TraceBusinessSpanRefCurrent(*active.ref) {
		return TraceBusinessFocusInvalid, TraceBusinessSpanRef{}
	}
	// Recheck that a reset or new completion did not retire the choice while
	// the original physical receipt was being checked outside the state lock.
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.traceBusinessFocus.active != active || m.traceBusinessFocus.status != TraceBusinessFocusSelected {
		status = m.traceBusinessFocus.status
		if status == "" {
			status = TraceBusinessFocusNone
		}
		if status == TraceBusinessFocusSelected {
			status = TraceBusinessFocusInvalid
		}
		return status, TraceBusinessSpanRef{}
	}
	return status, *active.ref
}

func (m *MutableState) traceBusinessFocusTicketCurrentLocked(ticket TraceBusinessFocusDispatch) bool {
	return ticket.token != nil && m.traceBusinessFocus.ticket == ticket.token && m.traceBusinessFocus.epoch == ticket.token.epoch
}

func (m *MutableState) traceBusinessSpanRefRegisteredLocked(ref TraceBusinessSpanRef) bool {
	registered, ok := m.traceBusinessSpanRefs[ref.token]
	return ok && ref.source.generation != nil && ref.source.generation == m.traceSourceReadGeneration && sameTraceBusinessSpanRef(ref, registered)
}

func sameTraceBusinessFocusChoice(a, b *traceBusinessFocusDecision) bool {
	if a.ref == nil || b.ref == nil {
		return a.ref == nil && b.ref == nil
	}
	x, y := *a.ref, *b.ref
	return x.data == y.data && x.source.generation == y.source.generation && x.source.path == y.source.path && x.source.identity.SameVersion(y.source.identity)
}

// resetTraceBusinessFocusLocked retires the dispatch identity, not just its
// presentation. Forks from the old epoch cannot write a choice back later.
func (m *MutableState) resetTraceBusinessFocusLocked() {
	status := m.traceBusinessFocus.status
	if status != "" && status != TraceBusinessFocusNone || m.traceBusinessFocus.pending != nil || m.traceBusinessFocus.active != nil {
		status = TraceBusinessFocusCleared
	}
	m.traceBusinessFocus = traceBusinessFocusState{epoch: &traceBusinessFocusEpoch{marker: 1}, status: status}
}

// TraceBusinessSpanResultSourceMatches requires the result's private original
// publication receipt, not a current stat of its public path alone.
func TraceBusinessSpanResultSourceMatches(ref TraceBusinessSpanRef, result ToolResult) bool {
	source := result.TraceQuerySourceRead
	return traceQueryPublishesSource(result, source) && source.generation != nil && source.generation == ref.source.generation &&
		source.path == ref.source.path && source.identity.SameVersion(ref.source.identity) &&
		TraceBusinessSpanQuerySourceMatches(ref, source, result)
}
