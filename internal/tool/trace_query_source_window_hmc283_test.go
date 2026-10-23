package tool

import (
	"context"
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

func hmc283EventContext(t *testing.T) *types.BusContext {
	t.Helper()
	dir := t.TempDir()
	body := "writer-101 (100) [001] .... 1.000000: print: C|100|value|1\n" +
		"writer-101 (100) [001] .... 2.000000: print: C|100|value|2\n" +
		"writer-101 (100) [001] .... 3.000000: print: C|100|value|3\n" +
		"writer-101 (100) [001] .... 4.000000: print: C|100|value|4\n"
	if err := os.WriteFile(filepath.Join(dir, "capture.systrace"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{RepoRoot: dir, WorkDir: dir}
}

func hmc283EventQuery(t *testing.T, ctx *types.BusContext, extra string) types.ToolResult {
	t.Helper()
	r, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"source":"path","path":"capture.systrace","view":"event_search"`+extra+`}`))
	if err != nil || !r.Success {
		t.Fatalf("native event query failed: %v %s", err, r.Summary)
	}
	return r
}

func TestHMC283EventSearchSourceSeparatesExecutedSelectorFromMemberEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name, extra              string
		lines                    []int
		total                    int
		startApplied, endApplied bool
		start, end               float64
		lineStart, lineEnd       int
	}{
		{name: "unbounded", lines: []int{1, 2, 3, 4}, total: 4},
		{name: "preview", extra: `,"limit":2`, lines: []int{1, 2}, total: 4},
		{name: "zero_matches", extra: `,"pattern":"absent"`},
		{name: "explicit_closed", extra: `,"time_start":2,"time_end":3`, lines: []int{2, 3}, total: 2, startApplied: true, endApplied: true, start: 2, end: 3},
		{name: "empty_explicit_window", extra: `,"time_start":8,"time_end":9`, startApplied: true, endApplied: true, start: 8, end: 9},
		{name: "exact_point", extra: `,"time_start":2,"time_end":2`, lines: []int{2}, total: 1, startApplied: true, endApplied: true, start: 2, end: 2},
		{name: "lower_only", extra: `,"time_start":3`, lines: []int{3, 4}, total: 2, startApplied: true, start: 3},
		{name: "upper_only", extra: `,"time_end":2`, lines: []int{1, 2}, total: 2, endApplied: true, end: 2},
		{name: "line_only", extra: `,"line_start":2,"line_end":3`, lines: []int{2, 3}, total: 2, lineStart: 2, lineEnd: 3},
		{name: "line_overrides_time", extra: `,"line_start":2,"line_end":3,"time_start":8,"time_end":9`, lines: []int{2, 3}, total: 2, lineStart: 2, lineEnd: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := hmc283EventContext(t)
			r := hmc283EventQuery(t, ctx, tc.extra)
			o := requireEventSearchInventory(t, r)
			i, s := o.EventSearchInventory, o.EventSearchInventory.Coverage.ScanScope
			canonicalPath, err := filepath.EvalSymlinks(filepath.Join(ctx.RepoRoot, "capture.systrace"))
			if err != nil {
				t.Fatal(err)
			}
			start, end, startApplied, endApplied := tc.start, tc.end, tc.startApplied, tc.endApplied
			// A no-match lookup may use the indexed fallback. Its scan really
			// applies normalized index endpoints, unlike the streaming lane;
			// neither lane may turn those endpoints into measurement authority.
			if s != nil && s.ObservedBasis == "parsed_events" && !startApplied && !endApplied && tc.lineStart == 0 && tc.lineEnd == 0 {
				start, end, startApplied, endApplied = 1, 4, true, true
			}
			if s == nil || s.TimeStartApplied != startApplied || s.TimeEndApplied != endApplied || s.TimeStart != start || s.TimeEnd != end || s.LineStart != tc.lineStart || s.LineEnd != tc.lineEnd {
				t.Fatalf("executed selector changed: %+v", s)
			}
			var lines []int
			for _, row := range i.Rows {
				lines = append(lines, row.Line)
				if row.SourcePath != canonicalPath || row.LocalLine != row.Line || row.TraceTimeSeconds != float64(row.Line) || row.EmitterTID != 101 {
					t.Fatalf("native coordinate or identity changed: %+v", row)
				}
			}
			if !reflect.DeepEqual(lines, tc.lines) || i.Coverage.MatchedTotal != tc.total || i.Coverage.Emitted != len(tc.lines) || !i.Coverage.EnumerationComplete {
				t.Fatalf("query membership/count changed: lines=%v coverage=%+v", lines, i.Coverage)
			}
			if o.SourceRef.QueryWindowKnown || o.SourceRef.QueryWindowStartTs != 0 || o.SourceRef.QueryWindowEndTs != 0 {
				t.Fatalf("lookup/member envelope acquired continuous measurement authority: %+v", o.SourceRef)
			}
			if o.SourceRef.QueryScopeID == "" || o.SourceRef.QueryLineStart != tc.lineStart || o.SourceRef.QueryLineEnd != tc.lineEnd {
				t.Fatalf("query identity or line selector lost: %+v", o.SourceRef)
			}
			p, ok := types.DecodeRuntimeMeasurementPublication(o)
			if !ok || len(p.Tables) != 1 || p.Tables[0].EventData == nil || !reflect.DeepEqual(p.Source, o.SourceRef) {
				t.Fatalf("native event display lost: ok=%v publication=%+v", ok, p)
			}
		})
	}
}

func TestHMC283IndexedEventSearchDoesNotPromoteNormalizedIndexEnvelope(t *testing.T) {
	ctx := hmc283EventContext(t)
	idx, err := tracequery.BuildIndex(context.Background(), filepath.Join(ctx.RepoRoot, "capture.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	q := tracequery.Query{View: "event_search", Limit: 2}
	r := tracequery.Run(idx, q)
	observations := traceQueryTypedObservations(r, "path", "/result.json", "/result.json", "", time.Now(), q)
	o := requireEventSearchInventory(t, types.ToolResult{Observations: observations})
	if o.SourceRef.QueryWindowKnown || r.TimeStart != 1 || r.TimeEnd != 4 || o.EventSearchInventory.Coverage.MatchedTotal != 4 || len(o.EventSearchInventory.Rows) != 2 {
		t.Fatalf("indexed normalization changed search or became continuous authority: source=%+v coverage=%+v", o.SourceRef, o.EventSearchInventory.Coverage)
	}
}

func TestHMC283LookupToleranceRemainsSeparateFromMeasurementWindow(t *testing.T) {
	ctx := hmc283EventContext(t)
	r := hmc283EventQuery(t, ctx, `,"time_start":"2.000s","time_end":"2.000s"`)
	o := requireEventSearchInventory(t, r)
	s := o.EventSearchInventory.Coverage.ScanScope
	if s == nil || s.TimeStart != 1.9995 || s.TimeEnd != 2.0005 || !s.TimeStartApplied || !s.TimeEndApplied || o.SourceRef.QueryWindowKnown || len(o.EventSearchInventory.Rows) != 1 {
		t.Fatalf("lookup tolerance became metric bounds or lost its hit: source=%+v selector=%+v", o.SourceRef, s)
	}
	notes := strings.Join(o.RichNotes, "\n")
	if strings.Contains(notes, "metric_denominator_window=requested_window") {
		t.Fatal("lookup notes still unconditionally authorize a measurement denominator")
	}
	for _, want := range []string{"requested_window=2.000000..2.000000", "matching_window=1.999500..2.000500", "metric_denominator_window=not_applicable_to_event_lookup"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("lost lookup annotation %q", want)
		}
	}
	if !strings.Contains(r.Summary, "event lookup has no statistical denominator. Separate causal/statistical calculations require a verified requested measurement window, never the lookup-tolerance expansion.") {
		t.Fatal("lookup summary lost the conditional measurement boundary")
	}
}

func TestHMC283MeasuredAndCausalViewsKeepEffectiveResultWindows(t *testing.T) {
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "frame_root_cause_bundle", "perf_sampling_tree", "resource_stack", "cpu_state_frequency", "measurements"} {
		ref := types.ObservationSourceRef{QueryScopeID: "q"}
		traceQueryStampParentQuerySource(&ref, tracequery.Result{View: view, TimeStart: 1, TimeEnd: 4}, tracequery.Query{View: view, PID: 101})
		if !ref.QueryWindowKnown || ref.QueryWindowStartTs != 1 || ref.QueryWindowEndTs != 4 || ref.QueryTargetPID != 101 {
			t.Fatalf("effective %s window/target lost without explicit caller endpoints: %+v", view, ref)
		}
	}
	ctx := suppCoreContext(t)
	r, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"wakeup_chain","time_start":3,"time_end":3.2}`))
	if err != nil || !r.Success {
		t.Fatalf("native automatic target resolution regressed: %v %s", err, r.Summary)
	}
	known := 0
	for _, o := range r.Observations {
		if o.SourceRef.QueryWindowKnown {
			known++
			if o.SourceRef.QueryWindowStartTs != 3 || o.SourceRef.QueryWindowEndTs != 3.2 || o.SourceRef.QueryTargetPID != 200 {
				t.Fatalf("native causal result lost exact automatic target/window: %+v", o.SourceRef)
			}
		}
	}
	if known == 0 {
		t.Fatal("native causal query published no determined window")
	}
}

func TestHMC283EventLookupKeepsInheritedBoundsAndFiniteAnswerProjection(t *testing.T) {
	fixture := hmc283EventContext(t)
	path := filepath.Join(fixture.RepoRoot, "capture.systrace")
	ctx, _, _ := hmc17NamedPathContext(t)
	rm := types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeArtifactScopeProfile: requestWindowTestProfile(2, 3),
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}},
	}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	ctx.Mutable.SetRequestModel(rm)
	ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "file"}}}
	for _, explicitProbe := range []bool{false, true} {
		args := map[string]any{"source": "path", "path": path, "view": "event_search"}
		if explicitProbe {
			args["time_start"], args["time_end"] = 1, 4
		}
		r := hmc17NamedQuery(t, ctx, args)
		o := requireEventSearchInventory(t, r)
		if o.SourceRef.QueryWindowKnown {
			t.Fatal("event lookup acquired a continuous measurement window")
		}
		wantMatched := 2
		if explicitProbe {
			wantMatched = 4
		} else if !strings.Contains(r.Summary, "trace_query_request_window_inherited=") {
			t.Fatal("request-window automatic completion was lost")
		}
		if o.EventSearchInventory.Coverage.MatchedTotal != wantMatched {
			t.Fatalf("lookup query membership changed: %+v", o.EventSearchInventory.Coverage)
		}
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
		view := types.BuildAnswerSemanticViewForBusContext(ctx)
		found := false
		for _, table := range view.RuntimeMeasurementContract.Choices() {
			if table.EventData == nil || table.ObservationID != o.ID {
				continue
			}
			found = true
			if len(table.Rows) != 1 || table.Rows[0][0] != "2" || table.EventData.Coverage.MatchedTotal != wantMatched || strings.Contains(table.Label, "time scope unverified") {
				t.Fatalf("finite half-open report projection lost its point/parent census or scope qualification: rows=%v label=%s coverage=%+v", table.Rows, table.Label, table.EventData.Coverage)
			}
		}
		if !found {
			t.Fatal("native event table disappeared from public answer contract")
		}
	}
}
