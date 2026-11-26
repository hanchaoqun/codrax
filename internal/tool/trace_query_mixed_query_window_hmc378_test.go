package tool

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc378MixedLedgerCapture: main-100 sleeps from 10.010 to 10.200 and is woken
// by HeapTaskDaemon-150, which runs a collection it opened at 9.950, before the
// queried lines. Six more threads of the process record a wait for that
// collection. It returns the capture and the lines of [10.0, 10.31].
func hmc378MixedLedgerCapture(t *testing.T) (path string, lineStart, lineEnd int) {
	t.Helper()
	var c hmc376Trace
	c.run(9.900000, "HeapTaskDaemon", 150, 100, 3)
	c.add(9.950000, "HeapTaskDaemon", 150, 100, 3, "tracing_mark_write: B|100|Background concurrent copying GC")
	c.run(10.000000, "main", 100, 100, 1)
	c.sleep(10.010000, "main", 100, 100, 1)
	for i := 0; i < 6; i++ {
		comm, tid, cpu, shift := fmt.Sprintf("worker%d", i), 200+i, 4+i%3, float64(i)*1e-5
		c.run(10.000200+shift, comm, tid, 100, cpu)
		c.span(10.011000+shift, 10.200300+shift, comm, tid, 100, cpu, "GC: Wait For Completion Alloc")
		c.sleep(10.011500+shift, comm, tid, 100, cpu)
		c.run(10.200400+shift, comm, tid, 100, cpu)
		c.sleep(10.201000+shift, comm, tid, 100, cpu)
	}
	c.add(10.199000, "HeapTaskDaemon", 150, 100, 3, "tracing_mark_write: E|100")
	c.add(10.200000, "HeapTaskDaemon", 150, 100, 3, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.sleep(10.200050, "HeapTaskDaemon", 150, 100, 3)
	c.run(10.200600, "main", 100, 100, 1)
	c.sleep(10.300000, "main", 100, 100, 1)
	path = c.write(t)
	// write sorts the lines and puts one header line before them.
	for i, line := range c.lines {
		if line.ts < 10.0 || line.ts > 10.31 {
			continue
		}
		if lineStart == 0 {
			lineStart = i + 2
		}
		lineEnd = i + 2
	}
	return path, lineStart, lineEnd
}

func hmc378AdjacentBucket(t *testing.T, results ...types.ToolResult) []types.TraceCausalProjectionNode {
	t.Helper()
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
	bus.ToolResults = results
	set := types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
	if len(set.Projections) != 1 {
		t.Fatalf("want one projection, got %d", len(set.Projections))
	}
	return set.Projections[0].AdjacentCauses
}

func hmc378ReaderFace(t *testing.T, results ...types.ToolResult) string {
	t.Helper()
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
	bus.ToolResults = results
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "probe."}}}
	if res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now()); err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
}

func hmc378BucketLabel(rows []types.TraceCausalProjectionNode) string {
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Subject) == "" {
			parts = append(parts, fmt.Sprintf("FOLD[%d:%s]", row.MergedCount, strings.Join(row.MergedSubjects, ",")))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s/%s/%.3f", row.Subject, row.Object, row.ImpactMS))
	}
	return strings.Join(parts, " | ")
}

// A time query and a line query of the same lines in one ledger read the
// collector's collection twice: clipped to the time window, and at its full
// length without a query window. The second reading takes no context seat, so
// every thread the time query alone publishes keeps its own row.
func TestHMC378SecondReadingOfACollectionTakesNoContextSeat(t *testing.T) {
	path, lineStart, lineEnd := hmc378MixedLedgerCapture(t)
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	query := func(extra map[string]any) types.ToolResult {
		params := map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100, "target_scope": "thread"}
		for key, value := range extra {
			params[key] = value
		}
		result := hmc17NamedQuery(t, ctx, params)
		if !result.Success {
			t.Fatalf("query %v failed: %s", extra, result.Summary)
		}
		return result
	}
	timed := query(map[string]any{"time_start": 10.0, "time_end": 10.31})
	lined := query(map[string]any{"line_start": lineStart, "line_end": lineEnd})

	workers := []string{"worker0-200", "worker1-201", "worker2-202", "worker3-203", "worker4-204", "worker5-205"}
	ownRows := func(rows []types.TraceCausalProjectionNode) map[string]bool {
		subjects := map[string]bool{}
		for _, row := range rows {
			subjects[strings.TrimSpace(row.Subject)] = true
		}
		return subjects
	}
	alone := hmc378AdjacentBucket(t, timed)
	for _, worker := range workers {
		if !ownRows(alone)[worker] {
			t.Fatalf("fixture: the time query alone must publish %s on its own row: %s", worker, hmc378BucketLabel(alone))
		}
	}
	for name, results := range map[string][]types.ToolResult{"time-then-line": {timed, lined}, "line-then-time": {lined, timed}} {
		t.Run(name, func(t *testing.T) {
			bucket := hmc378AdjacentBucket(t, results...)
			label := hmc378BucketLabel(bucket)
			clipped, full, lastSeated := -1, -1, -1
			for i, row := range bucket {
				if strings.TrimSpace(row.Subject) == "" {
					t.Fatalf("a row of the time query was folded when the line query joined the ledger: %s", label)
				}
				switch {
				case row.Subject != "HeapTaskDaemon-150" || row.Object != "trace_span":
					lastSeated = i
				case types.TraceCausalProjectionWindowPresent(row.QueryWindowStartTs, row.QueryWindowEndTs):
					clipped = i
				default:
					full = i
				}
			}
			if clipped < 0 || full < 0 {
				t.Fatalf("fixture: want the collection once with and once without a query window: %s", label)
			}
			if !hmc376Near(bucket[full].ImpactMS, 249) || !hmc376Near(bucket[clipped].ImpactMS, 199) {
				t.Fatalf("the two readings changed: %s", label)
			}
			for _, worker := range workers {
				if !ownRows(bucket)[worker] {
					t.Fatalf("%s lost its own row to the second reading of the collection: %s", worker, label)
				}
			}
			if clipped < lastSeated {
				t.Fatalf("the reading inside the longer one is shown after the seated rows: %s", label)
			}
			face := hmc378ReaderFace(t, results...)
			for _, worker := range workers {
				if !strings.Contains(face, worker+" · GC: Wait For Completion") {
					t.Fatalf("the reader face no longer names the wait %s recorded:\n%s", worker, face)
				}
			}
		})
	}
}

// The same rule on a complete original capture (Donghu baseline 105 / NetEase
// Cloud Music / cold start after a background kill, capture 1): the main
// thread's frame holds an animator phase, which the time query alone shows
// beside the frame. With the line query of the same lines first in the ledger
// the frame is seated from the reading without a query window, and the phase
// stays beside it instead of leaving its row for the counted fold. The capture
// is read from CODRAX_HMC376_NETEASE_GCWAIT and is never copied into the repo.
func TestHMC378WholeOriginalMixedLedgerPhaseTakesNoSeat(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC376_NETEASE_GCWAIT",
		"ae52285a00c4afce43e48bae519615110973052370537e7c53f5e4351073f2e0", 106869666)
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	var timed, lined []types.ToolResult
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		for _, reading := range []struct {
			into *[]types.ToolResult
			args map[string]any
		}{
			{&timed, map[string]any{"time_start": 5773.690943, "time_end": 5773.879288}},
			{&lined, map[string]any{"line_start": 427336, "line_end": 439619}},
		} {
			params := map[string]any{"source": "path", "path": path, "view": view, "pid": 34044, "target_scope": "thread"}
			for key, value := range reading.args {
				params[key] = value
			}
			result := hmc17NamedQuery(t, ctx, params)
			if !result.Success {
				t.Fatalf("%s %v failed: %s", view, reading.args, result.Summary)
			}
			*reading.into = append(*reading.into, result)
		}
	}
	const thread = "ease.cloudmusic-34044"
	phaseRow := func(bucket []types.TraceCausalProjectionNode) (own, folded bool) {
		for _, row := range bucket {
			if row.Subject == thread && row.Object == "trace_span" && row.SpanName == "animator:scaleX" && hmc376Near(row.ImpactMS, 147.051) {
				own = true
			}
			if strings.TrimSpace(row.Subject) == "" {
				for _, subject := range row.MergedSubjects {
					folded = folded || subject == thread
				}
			}
		}
		return own, folded
	}
	alone := hmc378AdjacentBucket(t, timed...)
	if own, folded := phaseRow(alone); !own || folded {
		t.Fatalf("fixture: the time query alone must show the animator phase on its own row: %s", hmc378BucketLabel(alone))
	}
	for name, results := range map[string][]types.ToolResult{
		"time-then-line": append(append([]types.ToolResult(nil), timed...), lined...),
		"line-then-time": append(append([]types.ToolResult(nil), lined...), timed...),
	} {
		bucket := hmc378AdjacentBucket(t, results...)
		if own, folded := phaseRow(bucket); !own || folded {
			t.Fatalf("%s: the animator phase left its own row when the line query joined the ledger (own=%v folded=%v): %s", name, own, folded, hmc378BucketLabel(bucket))
		}
	}
}
