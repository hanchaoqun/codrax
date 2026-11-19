package tracequery

import (
	"strings"
	"testing"
)

// HMC377 E2-R5: the wait clause names every wait of the analysed thread that
// the host's own wakeup ended — the set a remainder lies outside of — not only
// the waits holding a priced piece (Douyin reboot5 android.bg-7812: "1
// wait(s), 0.245ms" for 2 waits / 0.252 ms in raw).
func TestHMC377WaitClauseCountsEveryWaitTheHostEnded(t *testing.T) {
	windows := []targetWaitDependencyWindow{
		{startTs: 1.000, endTs: 1.007, wakeupTs: 1.007},
		{startTs: 1.010, endTs: 1.245, wakeupTs: 1.245},
	}
	// The host was ready only inside the second wait.
	split := splitFoldIntervalsByDependencyWindows([]foldInterval{{start: 1.020, end: 1.100}, {start: 1.300, end: 1.400}}, windows)
	clause := stateEdgeWaitClause(split)
	if !strings.Contains(clause, "(2 wait(s), 242.000ms of waiting;") {
		t.Fatalf("the clause must count every wait this thread's wakeup ended: %q", clause)
	}
	if !strings.Contains(clause, "the latest one holding part of this account was ended at 1.245000") {
		t.Fatalf("the timestamp names the wakeup that ended the latest wait holding a priced piece: %q", clause)
	}
}
