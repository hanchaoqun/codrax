package tracequery

import "testing"

func TestRankZeroDisclosureCapacityReservesBothTypedLanes(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	items := []RootCauseRankItem{
		{Type: "sleep_wait", Thread: target, SubjectIsAnalysisTarget: true},
		{Type: "binder_wait", Thread: target, SubjectIsAnalysisTarget: true},
		{Type: "missing_wakeup", Thread: target, SubjectIsAnalysisTarget: true},
		{Type: "blocking_span", Thread: target, SubjectIsAnalysisTarget: true},
		{Type: "fragmented_sleep_wait", Thread: target, SubjectIsAnalysisTarget: true},
		{Type: "trace_gap", Thread: ThreadRef{Comm: "peer-a", PID: 201}},
		{Type: "trace_gap", Thread: ThreadRef{Comm: "peer-b", PID: 202}},
		{Type: "trace_gap", Thread: ThreadRef{Comm: "peer-c", PID: 203}},
		{Type: "cpu_pressure", Thread: ThreadRef{Comm: "worker", PID: 300}},
	}

	got, candidateTotal, candidateEmitted, sideTotal, sideEmitted := truncateRootCauseRankCandidatesAndSideRows(items, 1)
	if candidateTotal != 1 || candidateEmitted != 1 || sideTotal != 8 || sideEmitted != rootCauseRankZeroSeatDisclosureCap {
		t.Fatalf("unexpected capacity census candidates=%d/%d side=%d/%d rows=%+v", candidateEmitted, candidateTotal, sideEmitted, sideTotal, got)
	}
	var targetRows, gapRows int
	for _, item := range got {
		if item.Type == "trace_gap" {
			gapRows++
		} else if rootCauseRankItemIsZeroSeatDisclosure(item) {
			targetRows++
		}
	}
	if targetRows != rootCauseRankZeroSeatPerLaneReservedCap || gapRows != rootCauseRankZeroSeatPerLaneReservedCap {
		t.Fatalf("both populated rank-0 lanes must retain reserved seats, target=%d gap=%d rows=%+v", targetRows, gapRows, got)
	}
}

func TestRankZeroDisclosureCapacityLoansUnusedLaneSeats(t *testing.T) {
	items := make([]RootCauseRankItem, 0, 6)
	for i := 0; i < 6; i++ {
		items = append(items, RootCauseRankItem{Type: "trace_gap", Thread: ThreadRef{PID: 200 + i}})
	}
	got, _, _, sideTotal, sideEmitted := truncateRootCauseRankCandidatesAndSideRows(items, 1)
	if sideTotal != 6 || sideEmitted != rootCauseRankZeroSeatDisclosureCap || len(got) != rootCauseRankZeroSeatDisclosureCap {
		t.Fatalf("an unopposed rank-0 lane should use the full disclosure cap, total=%d emitted=%d rows=%+v", sideTotal, sideEmitted, got)
	}
}
