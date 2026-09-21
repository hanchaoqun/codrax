package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public query/typed publication/projection route from the same
// fixture as the production report, including unrelated thread names.
func TestTraceDependencyWindowPublicOccupancyScope(t *testing.T) {
	caseBytes, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, trace, ok := strings.Cut(string(caseBytes), "HTRACE='")
	if !ok {
		t.Fatal("fixture has no trace")
	}
	trace, _, ok = strings.Cut(trace, "\n'\n")
	if !ok {
		t.Fatal("fixture trace is unterminated")
	}
	for _, rename := range []bool{false, true} {
		t.Run(map[bool]string{false: "original", true: "renamed"}[rename], func(t *testing.T) {
			content, subject := trace, "network-300"
			if rename {
				content = strings.ReplaceAll(content, "network", "transport")
				subject = "transport-300"
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "window.ftrace")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100, "time_start": 2, "time_end": 2.020, "trace_flavor": "harmony_hitrace"})
			result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("query: %v %s", err, result.Summary)
			}
			if !strings.Contains(result.Summary, "not a continuous state interval") {
				t.Error("public tool omits dependency-window measurement meaning")
			}
			var impact, exact *types.ObservationRecord
			for i := range result.Observations {
				r := &result.Observations[i]
				if r.Subject != subject {
					continue
				}
				if r.Predicate == "wakeup_causal_impact" {
					impact = r
				}
				if r.Predicate == "state_drilldown" && r.Object == "s_sleep" {
					exact = r
				}
			}
			if impact == nil || impact.Value != "14.000" || impact.Span.StartTs != 2.001 || impact.Span.EndTs != 2.018 {
				t.Fatalf("dependency measurement changed: %+v", impact)
			}
			if exact == nil || exact.Value != "14.000" || exact.Span.StartTs != 2.002 || exact.Span.EndTs != 2.016 {
				t.Fatalf("exact state occurrence lost: %+v", exact)
			}
			before, _ := json.Marshal(result.Observations)
			projection := types.TraceCausalProjectionFromObservationRecords(result.Observations)
			if projection.WindowStartTs != 2 || projection.WindowEndTs != 2.020 {
				t.Fatalf("query window changed: %v..%v", projection.WindowStartTs, projection.WindowEndTs)
			}
			for _, zh := range []bool{true, false} {
				model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				rows := runtimeTraceOccupancyPathCandidates(model, projection.TargetStateAccount, zh)
				found := false
				for _, row := range rows {
					if !strings.Contains(row.subject, subject) || row.totalMS != 14 {
						continue
					}
					found = true
					if strings.Contains(row.location, "2.001000..2.018000") && !strings.Contains(row.location, "统计范围") && !strings.Contains(row.location, "measurement scope") {
						t.Errorf("14ms is paired with an unqualified 17ms envelope: %+v", row)
					}
				}
				if !found {
					t.Fatal("raw network state occupancy disappeared")
				}
			}
			after, _ := json.Marshal(result.Observations)
			if string(before) != string(after) {
				t.Fatal("display mutated query observations")
			}
		})
	}
}

func TestTraceDependencyWindowMultipleStatesAreNotContinuousIntervals(t *testing.T) {
	for _, state := range []string{"s_sleep", "runnable", "running", "d_sleep", "io_wait"} {
		for _, predicate := range []string{"wakeup_causal_impact", "wakeup_causal_aggregate"} {
			node := types.TraceCausalProjectionNode{Subject: "renamed-78", Predicate: predicate, Object: state, StateKind: state,
				ImpactMS: 4, StartTs: 3, EndTs: 3.020, ActualWindowStartTs: 3.001, ActualWindowEndTs: 3.019, EvidenceID: "multi", MergedCount: 1}
			for _, zh := range []bool{true, false} {
				rows := runtimeTraceOccupancyPathCandidates(runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{{Node: node, Kind: runtimeTraceProjTreeRowSelf, HasData: true}}}, nil, zh)
				if len(rows) != 1 || rows[0].totalMS != 4 {
					t.Fatalf("state %s lost: %+v", state, rows)
				}
				if !strings.Contains(rows[0].location, "统计范围") && !strings.Contains(rows[0].location, "measurement scope") {
					t.Errorf("%s %s lacks scope disclosure: %s", predicate, state, rows[0].location)
				}
			}
		}
	}
	// A real state-drilldown interval stays precise and is not relabelled as
	// the larger dependency envelope merely because the values are equal.
	node := types.TraceCausalProjectionNode{Predicate: "state_drilldown", Object: "s_sleep", StateKind: "s_sleep", StartTs: 2.002, EndTs: 2.016, ImpactMS: 14}
	if got := runtimeTraceOccupancyNodeLocation(node, true); got != "2.002000..2.016000" {
		t.Fatalf("exact occurrence changed: %s", got)
	}
}

func TestTraceDependencyWindowToolTeachingDoesNotClaimSingleState(t *testing.T) {
	description := (&TraceQuery{}).Description()
	if strings.Contains(description, "underlying scheduler state segment that may extend") || !strings.Contains(description, "actual_window is its all-state envelope") {
		t.Fatal("tool JSON teaching must distinguish state-inventory envelopes from one state's occurrence")
	}
}

func TestTraceStateDrilldownPublicDisjointMeasurementScope(t *testing.T) {
	trace, err := os.ReadFile("testdata/state_drilldown_disjoint.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "states.ftrace")
	if err := os.WriteFile(path, trace, 0600); err != nil {
		t.Fatal(err)
	}
	query := func(view string) types.ToolResult {
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": 1, "time_end": 1.020, "trace_flavor": "harmony_hitrace"})
		result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
		if err != nil || !result.Success {
			t.Fatalf("%s: %v %s", view, err, result.Summary)
		}
		return result
	}
	result := query("window_stats")
	before, _ := json.Marshal(result.Observations)
	for _, want := range []struct {
		state, value string
		start, end   float64
	}{{"s_sleep", "8.000", 1, 1.012}, {"running", "8.000", 0, 0}, {"runnable", "4.000", 1.006, 1.018}} {
		found := false
		for _, record := range result.Observations {
			if record.Predicate != "state_drilldown" || record.Object != want.state || record.Subject != "worker-100" {
				continue
			}
			found = true
			if record.Value != want.value || record.Span.StartTs != want.start || record.Span.EndTs != want.end {
				t.Fatalf("cumulative %s or its envelope changed: %+v", want.state, record)
			}
		}
		if !found {
			t.Fatalf("no public state drilldown for %s: %+v", want.state, result.Observations)
		}
	}
	if !strings.Contains(result.Summary, "cumulative state measurement scope") || !strings.Contains(result.Summary, "not a continuous state interval") {
		t.Error("public drilldown handoff does not distinguish cumulative state scopes from occurrences")
	}
	for _, old := range []string{"precise state occurrences come from state_drilldown/thread_timeline", "use the same-source state_drilldown or thread_timeline occurrence"} {
		if strings.Contains((&TraceQuery{}).Description(), old) || strings.Contains(types.TraceDependencyAnalysisWindowGuidance, old) {
			t.Errorf("teaching still treats every drilldown as an occurrence: %q", old)
		}
	}
	// The occurrence authority remains the actual timeline interval, not the
	// drilldown's cumulative 8ms paired with its 12ms hull.
	timeline := query("thread_timeline")
	for _, want := range []string{"s_sleep 1.000000..1.006000 6.000ms", "s_sleep 1.010000..1.012000 2.000ms"} {
		if !strings.Contains(timeline.Summary, want) {
			t.Errorf("exact timeline occurrence changed or absent: %s\n%s", want, timeline.Summary)
		}
	}
	after, _ := json.Marshal(result.Observations)
	if string(before) != string(after) {
		t.Fatal("display changed cumulative observations")
	}
}

func TestTraceStateDrilldownLocationNeverInfersOccurrenceFromEqualWidth(t *testing.T) {
	for _, width := range []float64{0.008, 0.012} {
		node := types.TraceCausalProjectionNode{Subject: "worker-100", Predicate: "state_drilldown", Object: "s_sleep", StateKind: "s_sleep", ImpactMS: 8, StartTs: 1, EndTs: 1 + width}
		for _, zh := range []bool{true, false} {
			got := runtimeTraceOccupancyPathLocation(node, node, false, zh)
			if !strings.Contains(got, "统计范围") && !strings.Contains(got, "measurement scope") {
				t.Errorf("width=%v must not certify a drilldown occurrence: %s", width, got)
			}
		}
	}
	// Exact timeline occurrences are a different typed producer family.
	node := types.TraceCausalProjectionNode{Subject: "worker-100", Predicate: "thread_timeline", Object: "s_sleep", StateKind: "s_sleep", ImpactMS: 6, StartTs: 1, EndTs: 1.006}
	if got := runtimeTraceOccupancyPathLocation(node, node, false, true); got != "1.000000..1.006000" {
		t.Fatalf("exact timeline interval was relabelled: %s", got)
	}
}
