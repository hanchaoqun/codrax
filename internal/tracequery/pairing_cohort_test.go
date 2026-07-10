package tracequery

import "testing"

func TestPairingCohortStateMachineSuppressesAmbiguityAndRecovers(t *testing.T) {
	e := func(line int) Event { return Event{Line: line, Ts: float64(line)} }
	var state pairingCohortState
	if got := state.observeDone(e(1)); !got.unpairedDone {
		t.Fatalf("done-only transition = %+v", got)
	}
	state.observeStart(e(2))
	pair := state.observeDone(e(3))
	if !pair.pairReady || pair.pairStart.Line != 2 || pair.last.Line != 3 || state.depth != 0 {
		t.Fatalf("single pair transition = %+v state=%+v", pair, state)
	}
	state.observeStart(e(4))
	if got := state.observeStart(e(5)); !got.ambiguousOpened {
		t.Fatalf("second start must open ambiguity: %+v", got)
	}
	if got := state.observeDone(e(6)); got.cohortClosed {
		t.Fatalf("first done closed depth-two cohort: %+v", got)
	}
	ambiguous := state.observeDone(e(7))
	if !ambiguous.cohortClosed || !ambiguous.ambiguous || ambiguous.cohortStarts != 2 || state.depth != 0 {
		t.Fatalf("ambiguous cohort transition = %+v state=%+v", ambiguous, state)
	}
	state.observeStart(e(8))
	recovered := state.observeDone(e(9))
	if !recovered.pairReady || recovered.pairStart.Line != 8 {
		t.Fatalf("lane did not recover after depth returned to zero: %+v", recovered)
	}
}

func TestPairingCohortEOFCountsStartsExactlyOnce(t *testing.T) {
	e := func(line int) Event { return Event{Line: line, Ts: float64(line)} }
	var single pairingCohortState
	single.observeStart(e(1))
	open := single.finishEOF()
	if !open.cohortClosed || open.ambiguous || open.cohortStarts != 1 || single.depth != 0 {
		t.Fatalf("single EOF open = %+v state=%+v", open, single)
	}
	var ambiguous pairingCohortState
	ambiguous.observeStart(e(2))
	ambiguous.observeStart(e(3))
	ambiguous.observeDone(e(4))
	truncated := ambiguous.finishEOF()
	if !truncated.ambiguous || truncated.cohortStarts != 2 || ambiguous.depth != 0 {
		t.Fatalf("ambiguous EOF cohort = %+v state=%+v", truncated, ambiguous)
	}
}
