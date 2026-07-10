package tracequery

// pairingCohortState is the shared, deterministic state machine for elapsed
// endpoint families whose available identity can repeat concurrently.  A
// second start before the first completion makes the whole cohort ambiguous;
// no FIFO order is guessed, and the lane recovers only after its depth returns
// to zero.
//
// The state machine deliberately owns only endpoint topology.  Consumers keep
// their existing Count/Bytes/summary accounting because those wire contracts
// differ between block and generic storage families.
type pairingCohortState struct {
	depth        int
	ambiguous    bool
	cohortStarts int
	first        Event
	start        Event
	last         Event
}

type pairingCohortTransition struct {
	pairReady       bool
	pairStart       Event
	cohortClosed    bool
	ambiguous       bool
	cohortStarts    int
	first           Event
	last            Event
	unpairedDone    bool
	ambiguousOpened bool
}

func (s *pairingCohortState) observeStart(ev Event) pairingCohortTransition {
	if s == nil {
		return pairingCohortTransition{}
	}
	if s.depth == 0 {
		s.depth = 1
		s.cohortStarts = 1
		s.first = ev
		s.start = ev
		s.last = ev
		return pairingCohortTransition{}
	}
	s.depth++
	s.cohortStarts++
	s.last = ev
	opened := !s.ambiguous
	s.ambiguous = true
	s.start = Event{}
	return pairingCohortTransition{ambiguousOpened: opened}
}

func (s *pairingCohortState) observeDone(ev Event) pairingCohortTransition {
	if s == nil || s.depth == 0 {
		return pairingCohortTransition{unpairedDone: true, first: ev, last: ev}
	}
	s.last = ev
	if s.ambiguous {
		s.depth--
		if s.depth > 0 {
			return pairingCohortTransition{}
		}
		out := pairingCohortTransition{
			cohortClosed: true,
			ambiguous:    true,
			cohortStarts: s.cohortStarts,
			first:        s.first,
			last:         ev,
		}
		*s = pairingCohortState{}
		return out
	}
	out := pairingCohortTransition{
		pairReady:    true,
		pairStart:    s.start,
		cohortClosed: true,
		cohortStarts: 1,
		first:        s.first,
		last:         ev,
	}
	*s = pairingCohortState{}
	return out
}

func (s *pairingCohortState) finishEOF() pairingCohortTransition {
	if s == nil || s.depth == 0 {
		return pairingCohortTransition{}
	}
	out := pairingCohortTransition{
		cohortClosed: true,
		ambiguous:    s.ambiguous,
		cohortStarts: s.cohortStarts,
		first:        s.first,
		last:         s.last,
	}
	*s = pairingCohortState{}
	return out
}

// pairingIntervalIntersectsQuery applies the engine's public query-domain
// convention without dropping carry-in/carry-through intervals.  Line bounds
// are authoritative when present; otherwise explicit zero-second time bounds
// remain representable through the Set flags.
func pairingIntervalIntersectsQuery(first, last Event, q Query) bool {
	if q.LineStart > 0 || q.LineEnd > 0 {
		lo, hi := first.Line, last.Line
		if lo > hi {
			lo, hi = hi, lo
		}
		if q.LineStart > 0 && hi < q.LineStart {
			return false
		}
		if q.LineEnd > 0 && lo > q.LineEnd {
			return false
		}
		return true
	}
	lo, hi := first.Ts, last.Ts
	if lo > hi {
		lo, hi = hi, lo
	}
	if queryBoundedTimeStart(q) && hi < q.TimeStart {
		return false
	}
	if queryBoundedTimeEnd(q) && lo > q.TimeEnd {
		return false
	}
	return true
}

func pairingEventInsideQuery(ev Event, q Query) bool {
	if q.LineStart > 0 || q.LineEnd > 0 {
		return eventLineInWindow(ev, q)
	}
	if queryBoundedTimeStart(q) && ev.Ts < q.TimeStart {
		return false
	}
	if queryBoundedTimeEnd(q) && ev.Ts > q.TimeEnd {
		return false
	}
	return true
}
