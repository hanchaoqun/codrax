package tracequery

import (
	"encoding/json"
	"sort"
	"testing"
)

// Each physical child is parsed by BuildIndex. The constructed composite only
// rebases source coordinates, exercising the defensive multi-source boundary
// even though normal bundle admission currently isolates extra systraces.
func concurrencyParsedComposite(t *testing.T, children ...string) *Index {
	t.Helper()
	idx := &Index{Path: "/synthetic/source-ledger.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic}
	for i, body := range children {
		child := buildTraceIndex(t, string(rune('a'+i))+".systrace", body)
		base := idx.LineCount
		for _, ev := range child.Events {
			ev.Line += base
			idx.Events = append(idx.Events, ev)
		}
		for _, source := range child.TraceArtifacts {
			source.VirtualLineBase += base
			idx.TraceArtifacts = append(idx.TraceArtifacts, source)
		}
		idx.LineCount += child.LineCount
		idx.ScannedLineCount += child.ScannedLineCount
		if child.LastTs > idx.LastTs {
			idx.LastTs = child.LastTs
		}
	}
	sort.SliceStable(idx.Events, func(i, j int) bool { return idx.Events[i].Ts < idx.Events[j].Ts })
	return idx
}

func TestSchedulerConcurrencySourceConflictPreservesHealthyMembers(t *testing.T) {
	for _, sameTID := range []bool{false, true} {
		t.Run(map[bool]string{false: "shared_cpu", true: "shared_tid"}[sameTID], func(t *testing.T) {
			bCPU, bPID := 0, 20
			if sameTID {
				bCPU, bPID = 1, 10
			}
			idx := concurrencyParsedComposite(t,
				concurrencyPublicTrace(concurrencyPublicSwitch(1.001, 0, 0, 10, "S"), concurrencyPublicSwitch(1.004, 0, 10, 0, "S"), concurrencyPublicSwitch(1.001, 2, 0, 30, "S"), concurrencyPublicSwitch(1.003, 2, 30, 0, "S")),
				concurrencyPublicTrace(concurrencyPublicSwitch(1.002, bCPU, 0, bPID, "S"), concurrencyPublicSwitch(1.005, bCPU, bPID, 0, "S")))
			before, _ := json.Marshal(idx.Events)
			res := Run(idx, Query{View: "window_stats", TimeStart: 0, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true})
			stats := res.WindowStats.SchedulerConcurrency
			if stats == nil || stats.Coverage.SourceConflictIntervals != 2 {
				t.Fatalf("source conflicts not disclosed: %+v", stats)
			}
			for _, g := range stats.Groups {
				if g.Values != nil {
					concurrencyPublicValues(t, g, 1, 2.0/1010, 2, 2)
				}
			}
			if stats.Coverage.AcceptedIntervals != 1 {
				t.Fatalf("healthy member lost or conflicting member accepted: %+v", stats)
			}
			after, _ := json.Marshal(idx.Events)
			if string(before) != string(after) {
				t.Fatal("new stats changed source events")
			}
		})
	}
}

func TestSchedulerConcurrencyNonSchedulerSiblingAndMissingSource(t *testing.T) {
	a := concurrencyPublicTrace(concurrencyPublicSwitch(1.001, 0, 0, 10, "S"), concurrencyPublicSwitch(1.004, 0, 10, 0, "S"))
	idx := concurrencyParsedComposite(t, a, "noise-10 (10) [000] .... 1.002000: tracing_mark_write: B|10|background\nnoise-10 (10) [000] .... 1.003000: tracing_mark_write: E|10\n")
	q := Query{View: "window_stats", TimeStart: 0, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true}
	stats := Run(idx, q).WindowStats.SchedulerConcurrency
	g := concurrencyPublicGroup(t, stats, "running")
	concurrencyPublicValues(t, g, 1, 3.0/1010, 3, 3)
	if stats.Coverage.SourceConflictIntervals != 0 {
		t.Fatalf("non-scheduler sibling gained scheduler identity: %+v", stats.Coverage)
	}
	idx.TraceArtifacts[0].SourcePath = ""
	stats = Run(idx, q).WindowStats.SchedulerConcurrency
	if stats == nil || stats.Coverage.UnresolvedSourceIntervals == 0 {
		t.Fatalf("source failure undisclosed: %+v", stats)
	}
	for _, g := range stats.Groups {
		if g.Values != nil {
			t.Fatalf("unresolved provenance regained authority: %+v", g)
		}
	}
}

func TestSchedulerConcurrencyPublicSameTIDUnion(t *testing.T) {
	body := concurrencyPublicTrace(concurrencyPublicSwitch(1.001, 0, 0, 10, "S"), concurrencyPublicSwitch(1.005, 0, 10, 0, "S"), concurrencyPublicSwitch(1.003, 1, 0, 10, "S"), concurrencyPublicSwitch(1.007, 1, 10, 0, "S"))
	g := concurrencyPublicGroup(t, concurrencyPublicRun(t, body, 1, 1.01), "running")
	concurrencyPublicValues(t, g, 1, .6, 6, 6)
	if g.AcceptedIntervalCount != 2 || g.ThreadCount != 1 {
		t.Fatalf("same TID duplicated in population: %+v", g)
	}
}
