package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// B1619-P1a: a path is not an occurrence envelope. Its producer knows the
// query window and must publish that ruler for the shared scope formatter.
func TestB1619WakeupPathScopeActualExecute(t *testing.T) {
	dir := t.TempDir()
	body := strings.Join([]string{
		`app-20 (20) [001] .... 1.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`waker-10 (10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.060000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 1.070000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`other-11 (11) [002] .... 1.090000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.095000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "scope.systrace"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	params, err := json.Marshal(map[string]any{
		"source": "path", "path": "scope.systrace", "view": "wakeup_chain",
		"pid": 20, "time_start": 1.0, "time_end": 1.1,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("actual trace query failed: result=%+v err=%v", result, err)
	}
	paths := b1619WakeupPathRecords(result.Observations)
	if len(paths) < 2 {
		t.Fatalf("real two-wakeup fixture must publish multiple paths, got %d", len(paths))
	}
	for _, row := range paths {
		b1619AssertPathQueryScope(t, row, 1, 1.1, types.TraceQueryWindowScopeRequestedPrincipal)
		// Same measured query, different explicit request: only supporting
		// exploration, without changing the published path or its numbers.
		start, end, _ := types.TraceCausalProjectionSelectedWindowNote(row.RichNotes)
		wider := types.ResolveTraceQueryWindowScope(b1619PathRequestedWindow(1, 1.2), start, end)
		if !wider.IsSupportingExploration() {
			t.Fatalf("path query must not substitute for a larger requested window: %+v", wider)
		}
	}
	b1619AssertPathNotesDoNotChangeProjection(t, result.Observations, b1619PathRequestedWindow(1, 1.1))
}

func TestB1619WakeupPathScopeProducerKnownAndUnknownWindows(t *testing.T) {
	cases := []struct {
		name   string
		window tracequery.TimeWindow
		known  bool
	}{
		{"positive", tracequery.TimeWindow{StartTs: 2, EndTs: 2.02}, true},
		{"explicit_zero", tracequery.TimeWindow{StartTs: 0, EndTs: .02, StartSet: true}, true},
		{"unknown_zero", tracequery.TimeWindow{StartTs: 0, EndTs: .02}, false},
		{"absent", tracequery.TimeWindow{}, false},
		{"empty", tracequery.TimeWindow{StartTs: 2, EndTs: 2}, false},
		{"reversed", tracequery.TimeWindow{StartTs: 2, EndTs: 1}, false},
	}
	for _, branchMode := range []bool{false, true} {
		name := "legacy"
		if branchMode {
			name = "multiple_branches"
		}
		t.Run(name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					chain := huadongShapeChain()
					chain.Window = tc.window
					if !branchMode {
						chain = *winflagChainResult(tc.window)
					}
					result := tracequery.Result{View: "wakeup_chain", SourcePath: "/capture/same.systrace", WakeupChain: &chain}
					rows := traceQueryTypedObservations(result, "same.systrace", "/payload/query.json", "/raw/query.txt", "", time.Unix(10, 0))
					paths := b1619WakeupPathRecords(rows)
					wantCount := 1
					if branchMode {
						wantCount = 8
					}
					if len(paths) != wantCount {
						t.Fatalf("paths=%d want=%d", len(paths), wantCount)
					}
					for _, row := range paths {
						if row.SourceRef.Path != result.SourcePath || row.Object == "" {
							t.Fatalf("source/path identity lost: %+v", row)
						}
						start, end, known := types.TraceCausalProjectionSelectedWindowNote(row.RichNotes)
						if known != tc.known {
							t.Fatalf("query window known=%t want=%t: %v", known, tc.known, row.RichNotes)
						}
						if tc.known {
							b1619AssertPathQueryScope(t, row, tc.window.StartTs, tc.window.EndTs, types.TraceQueryWindowScopeRequestedPrincipal)
						} else {
							if strings.Contains(strings.Join(row.RichNotes, "\n"), types.TraceNoteKeySelectedWindow+"=") {
								t.Fatalf("invalid query window must not mint a selected-window note: %v", row.RichNotes)
							}
							scope := types.ResolveTraceQueryWindowScope(b1619PathRequestedWindow(2, 2.02), start, end)
							if scope.Role != types.TraceQueryWindowScopeUnknownQueryWindow {
								t.Fatalf("unknown query must not borrow occurrence/path windows: %+v", scope)
							}
						}
					}
					b1619AssertPathNotesDoNotChangeProjection(t, rows, b1619PathRequestedWindow(2, 2.02))
				})
			}
		})
	}
}

func b1619WakeupPathRecords(rows []types.ObservationRecord) []types.ObservationRecord {
	var paths []types.ObservationRecord
	for _, row := range rows {
		if row.Predicate == "wakeup_chain" {
			paths = append(paths, row)
		}
	}
	return paths
}

func b1619PathRequestedWindow(start, end float64) *types.RuntimeArtifactScopeProfile {
	return &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start, TimeEnd: &end, SourceQuote: "explicit query interval", Confidence: 1,
	}
}

func b1619AssertPathQueryScope(t *testing.T, row types.ObservationRecord, start, end float64, role types.TraceQueryWindowScopeRole) {
	t.Helper()
	s, e, ok := types.TraceCausalProjectionSelectedWindowNote(row.RichNotes)
	if !ok || s != start || e != end {
		t.Fatalf("published path lost the query window %v..%v: %v", start, end, row.RichNotes)
	}
	scope := types.ResolveTraceQueryWindowScope(b1619PathRequestedWindow(start, end), s, e)
	if scope.Role != role {
		t.Fatalf("wrong path query scope: %+v", scope)
	}
	for _, lang := range []string{"zh", "en"} {
		if got := scope.Format(lang); got == "" || strings.Contains(got, "未明确") || strings.Contains(got, "unknown") {
			t.Fatalf("known path query incorrectly rendered as unknown: %q", got)
		}
	}
}

func b1619AssertPathNotesDoNotChangeProjection(t *testing.T, rows []types.ObservationRecord, requested *types.RuntimeArtifactScopeProfile) {
	t.Helper()
	previous := append([]types.ObservationRecord(nil), rows...)
	for i, row := range previous {
		if row.Predicate != "wakeup_chain" {
			continue
		}
		previous[i].RichNotes = nil
		for _, note := range row.RichNotes {
			if !strings.HasPrefix(note, types.TraceNoteKeySelectedWindow+"=") {
				previous[i].RichNotes = append(previous[i].RichNotes, note)
			}
		}
	}
	before := types.CompileTraceCausalProjection(types.ObservationLedger{Records: previous, RuntimeArtifactScopeProfile: requested})
	after := types.CompileTraceCausalProjection(types.ObservationLedger{Records: rows, RuntimeArtifactScopeProfile: requested})
	// A single-query projection is otherwise identical. The elected path's
	// query identity and the existing display-only query roster recover the
	// same producer-owned ruler; neither can invent a different window.
	var knownWindow *types.TraceCausalProjectionQueryWindow
	for _, path := range b1619WakeupPathRecords(rows) {
		if start, end, ok := types.TraceCausalProjectionSelectedWindowNote(path.RichNotes); ok {
			window := types.TraceCausalProjectionQueryWindow{StartTs: start, EndTs: end}
			if knownWindow != nil && *knownWindow != window {
				t.Fatal("single-query parity fixture contains multiple query windows")
			}
			knownWindow = &window
		}
	}
	if knownWindow != nil {
		if after.WakeupPathQueryWindowStartTs != knownWindow.StartTs || after.WakeupPathQueryWindowEndTs != knownWindow.EndTs {
			t.Fatalf("elected path did not preserve its producer ruler: %.6f..%.6f", after.WakeupPathQueryWindowStartTs, after.WakeupPathQueryWindowEndTs)
		}
		if len(before.QueryWindows) == 0 {
			want := []types.TraceCausalProjectionQueryWindow{*knownWindow}
			if !reflect.DeepEqual(after.QueryWindows, want) {
				t.Fatalf("path-only query roster must contain exactly its producer window: %+v", after.QueryWindows)
			}
			before.QueryWindows = want
		}
		before.WakeupPathQueryWindowStartTs = after.WakeupPathQueryWindowStartTs
		before.WakeupPathQueryWindowEndTs = after.WakeupPathQueryWindowEndTs
	}
	if !reflect.DeepEqual(before, after) {
		bv, av := reflect.ValueOf(before), reflect.ValueOf(after)
		for i := 0; i < bv.NumField(); i++ {
			if !reflect.DeepEqual(bv.Field(i).Interface(), av.Field(i).Interface()) {
				old, _ := json.Marshal(bv.Field(i).Interface())
				current, _ := json.Marshal(av.Field(i).Interface())
				t.Logf("changed %s: %.500s => %.500s", bv.Type().Field(i).Name, old, current)
			}
		}
		t.Fatal("path-only query scope metadata changed the causal projection")
	}
}

func TestB1619WakeupPathScopeRestoresExistingExactWindowElection(t *testing.T) {
	full := b1619ProducedPathRows(tracequery.TimeWindow{StartTs: 10, EndTs: 10.1}, "main-worker", 21)
	probe := b1619ProducedPathRows(tracequery.TimeWindow{StartTs: 10.02, EndTs: 10.07}, "probe-worker", 22)
	for _, order := range []struct {
		name string
		rows []types.ObservationRecord
	}{
		{"probe_first", append(append([]types.ObservationRecord(nil), probe...), full...)},
		{"full_first", append(append([]types.ObservationRecord(nil), full...), probe...)},
	} {
		t.Run(order.name, func(t *testing.T) {
			before, err := json.Marshal(order.rows)
			if err != nil {
				t.Fatal(err)
			}
			projection := types.CompileTraceCausalProjection(types.ObservationLedger{
				Records: order.rows, RuntimeArtifactScopeProfile: b1619PathRequestedWindow(10, 10.1),
				AnchorUserEntities: []types.AnchorUserEntity{{Value: "20", TypedLane: true}},
			})
			if !reflect.DeepEqual(projection.WakeupPath, []string{"main-worker-21", "app-20"}) ||
				projection.WakeupPathQueryWindowStartTs != 10 || projection.WakeupPathQueryWindowEndTs != 10.1 {
				t.Fatalf("existing exact-window path rule was not supplied its producer identity: %+v", projection.WakeupPath)
			}
			after, err := json.Marshal(order.rows)
			if err != nil || string(before) != string(after) {
				t.Fatal("scope election must not rewrite or remove either query's observations")
			}
		})
	}
	// Without an exact causal carrier, the old exploratory query still wins;
	// user intent is not itself a new window, path, or accounting witness.
	projection := types.CompileTraceCausalProjection(types.ObservationLedger{
		Records: probe, RuntimeArtifactScopeProfile: b1619PathRequestedWindow(10, 10.1),
		AnchorUserEntities: []types.AnchorUserEntity{{Value: "20", TypedLane: true}},
	})
	if !reflect.DeepEqual(projection.WakeupPath, []string{"probe-worker-22", "app-20"}) ||
		projection.WindowStartTs != 10.02 || projection.WindowEndTs != 10.07 || !projection.WindowScope.IsSupportingExploration() {
		t.Fatalf("lack of exact coverage must preserve exploration: path=%v scope=%+v", projection.WakeupPath, projection.WindowScope)
	}
}

func TestB1619WakeupPathScopeFeedsExistingPublicationDomain(t *testing.T) {
	rows := b1619ProducedPathRows(tracequery.TimeWindow{StartTs: 100, EndTs: 100.2}, "main-worker", 21)
	published := types.CompileTraceCausalProjection(types.ObservationLedger{Records: rows})
	if published.WakeupPathQueryWindowStartTs != 100 || published.WakeupPathQueryWindowEndTs != 100.2 {
		t.Fatalf("producer query ruler not carried by elected path: %+v", published.WakeupPath)
	}
	// Exercise the existing publication join with the actual producer-owned
	// ruler. The same per-query branch ordinal cannot join a different window.
	fixture := gapbWindowedTrunkProjection()
	fixture.WakeupPathQueryWindowStartTs = published.WakeupPathQueryWindowStartTs
	fixture.WakeupPathQueryWindowEndTs = published.WakeupPathQueryWindowEndTs
	model := buildRuntimeTraceProjTreeModel(fixture, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	seenSame, seenOther := false, false
	for _, row := range model.TreeRows {
		switch row.Node.Subject {
		case "same-win-666":
			seenSame = true
			if row.Kind != runtimeTraceProjTreeRowChain || row.Parent != "OS_mmi_EventHdr-43103" || row.Node.ImpactMS != 2 {
				t.Fatalf("same-window relationship/value must survive: %+v", row)
			}
		case "hmfs_discard-777":
			seenOther = true
			if row.Kind != runtimeTraceProjTreeRowDepthless || row.Node.ImpactMS != 3.2 {
				t.Fatalf("foreign-window value remains visible without an invented attachment: %+v", row)
			}
		}
	}
	if !seenSame || !seenOther {
		t.Fatal("publication must retain both query-domain observations")
	}
}

func b1619ProducedPathRows(window tracequery.TimeWindow, wakerName string, pid int) []types.ObservationRecord {
	chain := winflagChainResult(window)
	chain.Nodes[0].Branch, chain.Nodes[1].Branch = 1, 1
	chain.Nodes[1].Thread = tracequery.ThreadRef{Comm: wakerName, PID: pid}
	chain.Edges[0].Waker = chain.Nodes[1].Thread
	chain.Edges[0].From, chain.Edges[0].To, chain.Edges[0].Branch = "n2", "n1", 1
	chain.Edges[0].WakeupTs = window.StartTs + .01
	chain.CausalImpacts = []tracequery.WakeupCausalImpact{{
		Thread: chain.Nodes[1].Thread, Window: tracequery.TimeWindow{StartTs: window.StartTs, EndTs: window.StartTs + .005},
		ChainDepth: 1, ChainBranch: 1, OnChain: true, DominantState: string(tracequery.StateSSleep),
		DominantImpactMs: 5, ProjectedImpactMs: 5, TotalMs: 5, ProjectedTotalMs: 5, SleepMs: 5,
		LineStart: 10, LineEnd: 12,
	}}
	return traceQueryTypedObservations(tracequery.Result{
		View: "wakeup_chain", SourcePath: "/capture/same.systrace", WakeupChain: chain,
	}, "same.systrace", "/payload/"+wakerName+".json", "/raw/"+wakerName+".txt", "", time.Unix(10, 0))
}
