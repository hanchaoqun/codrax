package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

// processMeasurementTablesHMC376 runs the public query and returns the typed
// tables of its one process-measurement publication with the producer record.
func processMeasurementTablesHMC376(t *testing.T, ctx *types.BusContext, path string, start, end float64) ([]types.RuntimeMeasurementTable, types.ObservationRecord) {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_measurements", "time_start": start, "time_end": end})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("default process preparation/query failed: %v %s", err, result.Summary)
	}
	var tables []types.RuntimeMeasurementTable
	var record types.ObservationRecord
	found := 0
	for _, r := range result.Observations {
		if r.Predicate != "process_measurements_observation" {
			continue
		}
		publication, ok := types.DecodeRuntimeMeasurementPublication(r)
		if !ok {
			t.Fatalf("native row publication absent: %+v", r)
		}
		tables, record = publication.Tables, r
		found++
	}
	if found != 1 {
		t.Fatalf("source-bound process measurement publications=%d, want 1", found)
	}
	return tables, record
}

// processMeasurementCellsHMC376 returns one named column of one published
// view. Assertions read these typed cells: the encoded publication also holds
// the prepared path and content ids, so it is never searched for a bare value.
func processMeasurementCellsHMC376(t *testing.T, tables []types.RuntimeMeasurementTable, view types.RuntimeMeasurementView, column string) []string {
	t.Helper()
	for _, table := range tables {
		if table.View != view {
			continue
		}
		for i, name := range table.Columns {
			if name != column {
				continue
			}
			var cells []string
			for _, row := range table.Rows {
				cells = append(cells, row[i])
			}
			return cells
		}
	}
	t.Fatalf("published view %s lost column %q", view, column)
	return nil
}

// processMeasurementBoundaryViolationsHMC376 is the right-boundary oracle. It
// returns the violations instead of failing so the same function is exercised
// by the positive cases and by the widened-window control.
func processMeasurementBoundaryViolationsHMC376(t *testing.T, tables []types.RuntimeMeasurementTable, startNs, endNs int64) []string {
	t.Helper()
	var bad []string
	fail := func(format string, args ...any) { bad = append(bad, fmt.Sprintf(format, args...)) }
	integer := func(what, cell string) (int64, bool) {
		v, err := strconv.ParseInt(cell, 10, 64)
		if err != nil {
			fail("%s is not an integer nanosecond instant: %q", what, cell)
		}
		return v, err == nil
	}
	for _, view := range []types.RuntimeMeasurementView{types.RuntimeMeasurementMembers, types.RuntimeMeasurementTimeline} {
		for i, cell := range processMeasurementCellsHMC376(t, tables, view, "窗口内起点(ns)") {
			if v, ok := integer(string(view)+" in-window start", cell); ok && (v < startNs || v >= endNs) {
				fail("%s row %d is observed at %d ns, outside the half-open query window [%d,%d)", view, i+1, v, startNs, endNs)
			}
		}
	}
	members := processMeasurementCellsHMC376(t, tables, types.RuntimeMeasurementMembers, "原起点(ns)")
	for i, cell := range members {
		if v, ok := integer("member source start", cell); ok && v >= endNs {
			fail("member row %d starts at %d ns, at or after the right boundary %d", i+1, v, endNs)
		}
	}
	// The per-series summary carries the same records as "start→value" pairs.
	counts := processMeasurementCellsHMC376(t, tables, types.RuntimeMeasurementSummary, "保留记录数")
	total := 0
	for i, series := range processMeasurementCellsHMC376(t, tables, types.RuntimeMeasurementSummary, "原起点(ns)→原值（单位未提供）") {
		entries := strings.Split(series, "; ")
		if n, err := strconv.Atoi(counts[i]); err != nil || n != len(entries) {
			fail("summary row %d declares %q records but lists %d", i+1, counts[i], len(entries))
		}
		total += len(entries)
		for _, entry := range entries {
			instant, _, ok := strings.Cut(entry, "→")
			if !ok {
				fail("summary row %d entry %q is not a start→value pair", i+1, entry)
				continue
			}
			if v, ok := integer("summary source start", instant); ok && v >= endNs {
				fail("summary row %d lists a record starting at %d ns, at or after the right boundary %d", i+1, v, endNs)
			}
		}
	}
	if total != len(members) {
		fail("summary lists %d records, members table holds %d", total, len(members))
	}
	return bad
}

// processMeasurementPathNoiseHMC376 names the directory of the path-noise
// control. It spells the raw value of the fixture record that starts exactly
// on the right boundary.
const processMeasurementPathNoiseHMC376 = "run-9900"

var processMeasurementDigitsHMC376 = regexp.MustCompile(`[0-9]+`)

// HMC376 re-base: the former guards searched the encoded publication for the
// owners, two native values and the right-boundary value. That encoding also
// carries the prepared path and content ids, so the right-boundary guard
// failed whenever the temporary path spelled the value. The same claims are
// now read from typed cells, with a path-noise case and a widened-window
// control below.
func TestProcessMeasurementsPublicNativePreparationAndReceipt(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_measurements/capture.data")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, sub string }{{"default_path", ""}, {"path_noise", processMeasurementPathNoiseHMC376}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.sub != "" {
				dir = filepath.Join(dir, tc.sub)
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, _, _ := hmc17NamedPathContextIn(t, dir)
			tables, r := processMeasurementTablesHMC376(t, ctx, path, 1, 2)
			owners := processMeasurementCellsHMC376(t, tables, types.RuntimeMeasurementMembers, "进程")
			values := processMeasurementCellsHMC376(t, tables, types.RuntimeMeasurementMembers, "原始值（单位未提供）")
			has := func(cells []string, match func(string) bool) bool {
				for _, cell := range cells {
					if match(cell) {
						return true
					}
				}
				return false
			}
			for _, want := range []string{"app.alpha", "app.beta"} {
				if !has(owners, func(cell string) bool { return strings.HasPrefix(cell, want+" ") }) {
					t.Errorf("lost native owner %q: %q", want, owners)
				}
			}
			if !has(values, func(cell string) bool { return cell == "9007199254740993" }) {
				t.Errorf("lost exact native value: %q", values)
			}
			if !has(values, func(cell string) bool { return strings.HasPrefix(cell, `"unavailable"`) }) {
				t.Errorf("lost native text value: %q", values)
			}
			for _, v := range processMeasurementBoundaryViolationsHMC376(t, tables, 1_000_000_000, 2_000_000_000) {
				t.Errorf("included right boundary: %s", v)
			}
			if has(values, func(cell string) bool { return cell == "9900" }) {
				t.Fatal("included right boundary")
			}
			// The former search also covered the producer notes. They are
			// read as typed strings; the one note that names the prepared
			// path is identified by its own prefix and left out.
			for _, table := range tables {
				sourceNotes := 0
				for _, note := range table.Notes {
					if strings.HasPrefix(note, "Source: ") {
						sourceNotes++
						continue
					}
					for _, number := range processMeasurementDigitsHMC376.FindAllString(note, -1) {
						if number == "9900" {
							t.Fatalf("included right boundary in a %s note: %q", table.View, note)
						}
					}
				}
				if sourceNotes != 1 {
					t.Fatalf("%s: want exactly one source note, got %d: %q", table.View, sourceNotes, table.Notes)
				}
			}
			if r.SourceRef.QueryTargetThread != "" || r.SourceRef.QueryWindowStartTs != 1 || r.SourceRef.QueryWindowEndTs != 2 {
				t.Fatalf("changed owner/window: %+v", r.SourceRef)
			}
			if tc.sub != "" {
				encoded, _ := json.Marshal(tables)
				if !strings.Contains(string(encoded), tc.sub) {
					t.Fatal("path noise no longer reaches the encoded publication; this control is vacuous")
				}
			}
		})
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("source database mutated")
	}
}

// Strong negative control: a publication that really includes the record
// starting on the requested right boundary must be rejected by the same
// oracle, and accepted once the oracle is told the wider window.
func TestProcessMeasurementsBoundaryOracleRejectsRightBoundaryRecord(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_measurements/capture.data")
	ctx, _, _ := hmc17NamedPathContext(t)
	tables, _ := processMeasurementTablesHMC376(t, ctx, path, 1, 2.1)
	leaked := false
	for _, cell := range processMeasurementCellsHMC376(t, tables, types.RuntimeMeasurementMembers, "原始值（单位未提供）") {
		leaked = leaked || cell == "9900"
	}
	if !leaked {
		t.Fatal("fixture changed: the widened window no longer includes the right-boundary record; re-derive this control")
	}
	got := strings.Join(processMeasurementBoundaryViolationsHMC376(t, tables, 1_000_000_000, 2_000_000_000), "\n")
	for _, want := range []string{
		"members row", "timeline row", "outside the half-open query window",
		"member row", "at or after the right boundary 2000000000",
		"summary row 1 lists a record starting at 2000000000 ns",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("oracle missed %q on a real right-boundary record:\n%s", want, got)
		}
	}
	if bad := processMeasurementBoundaryViolationsHMC376(t, tables, 1_000_000_000, 2_100_000_000); len(bad) != 0 {
		t.Fatalf("oracle rejects a correct wider window, so it proves nothing: %v", bad)
	}
}

func TestProcessMeasurementsPublicOwnerScope(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_measurements/capture.data")
	for _, tc := range []struct {
		name  string
		focus []types.RuntimeTarget
		pid   int
		rows  int
	}{
		{"thread is not process", []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit", Confidence: 1}}, 0, 11},
		{"typed process", []types.RuntimeTarget{{Kind: types.RuntimeTargetKindProcess, PID: 100, Source: "user_explicit", Confidence: 1}}, 100, 7},
		{"multiple owners", []types.RuntimeTarget{{Kind: types.RuntimeTargetKindProcess, PID: 100, Source: "user_explicit", Confidence: 1}, {Kind: types.RuntimeTargetKindProcess, PID: 200, Source: "user_explicit", Confidence: 1}}, 0, 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, _ := hmc17NamedPathContext(t)
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeTargets: tc.focus}}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_measurements", "time_start": 1, "time_end": 2})
			out, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !out.Success {
				t.Fatalf("query: %v %s", err, out.Summary)
			}
			found := false
			for _, r := range out.Observations {
				if r.Predicate != TraceProcessMeasurementsPredicate {
					continue
				}
				p, ok := types.DecodeRuntimeMeasurementPublication(r)
				if !ok || r.SourceRef.QueryTargetPID != tc.pid || r.SourceRef.QueryTargetScope != "process" || r.SourceRef.QueryTargetThread != "" {
					t.Fatalf("bad process authority: %+v", r)
				}
				for _, table := range p.Tables {
					if table.MemberSet != nil {
						t.Fatal("display acquired population-completion authority")
					}
					if table.View == types.RuntimeMeasurementMembers && len(table.Rows) != tc.rows {
						t.Fatalf("rows=%d, want %d", len(table.Rows), tc.rows)
					}
				}
				for _, mutate := range []func(*types.ObservationRecord){
					func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
					func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs = 3 },
					func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 999 },
					func(r *types.ObservationRecord) { r.SourceRef.QueryTargetThread = "worker" },
					func(r *types.ObservationRecord) { r.Producer = "model" },
					func(r *types.ObservationRecord) {
						r.RichNotes = append(append([]string(nil), r.RichNotes...), r.RichNotes[0])
					},
				} {
					bad := r
					mutate(&bad)
					if _, ok := types.DecodeRuntimeMeasurementPublication(bad); ok {
						t.Fatal("accepted rebound/ambiguous display receipt")
					}
				}
				found = true
			}
			if !found {
				t.Fatal("missing native process observation")
			}
		})
	}
	for _, args := range []string{`{"view":"process_measurements","thread":"worker"}`, `{"view":"process_measurements","pid":100,"target_scope":"thread"}`} {
		out, err := (&TraceQuery{}).Execute(nil, json.RawMessage(args))
		if err != nil || out.Success || out.Repair == nil || out.Repair.Code != "trace_query_process_owned_view" || out.RawRef != "" || len(out.Observations) != 0 {
			t.Fatalf("read before selector repair: %v %+v", err, out)
		}
	}
	if !strings.Contains(string((&TraceQuery{}).Parameters()), tracequery.ProcessMeasurementsTeaching) {
		t.Fatal("schema lost single-source teaching")
	}
}

func TestProcessMeasurementsReceiptBudgetAndEmptyStatus(t *testing.T) {
	integer := func(v int64) tracewire.ProcessMeasureScalar {
		return tracewire.ProcessMeasureScalar{Status: "known", StorageClass: "integer", Value: strconv.FormatInt(v, 10)}
	}
	pid := 100
	p := tracequery.ProcessMeasurementsResult{Status: "available", SourcePath: "/capture.systrace", Window: tracequery.ProcessMeasurementsWindow{StartTs: 1, EndTs: 2}, TargetScope: "process"}
	for i := 0; i < 64; i++ {
		left, right := int64(1e9+i), int64(1e9+i+1)
		rec := tracewire.ProcessMeasureInterval{RowID: int64(i + 1), StartNS: integer(left), DurationNS: integer(1), Value: integer(9007199254740993), FilterID: integer(int64(i)), IPID: integer(1), NameKnown: true, Name: strings.Repeat("指标", 500) + strconv.Itoa(i), OwnerStatus: "known", PID: &pid, ProcessName: "app"}
		p.Rows = append(p.Rows, tracequery.ProcessMeasurementRow{SourcePath: p.SourcePath, Line: i + 1, SourceLine: i + 1, Record: rec, ClippedStartNS: &left, ClippedEndNS: &right, Selection: "interval_overlap", Unit: "unknown"})
	}
	p.TotalRows = len(p.Rows)
	before, _ := json.Marshal(p)
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: p.SourcePath, PayloadRef: "payload", QueryScopeID: "scope", QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 2, QueryTargetScope: "process"}
	rows := traceQueryProcessMeasurementsObservations(&p, ref, "scope", "now")
	if len(rows) != 1 || len(rows[0].RichNotes[0]) > (64<<10)+len(types.TraceNoteKeyRuntimeMeasurement)+1 {
		t.Fatal("unbounded publication")
	}
	publication, ok := types.DecodeRuntimeMeasurementPublication(rows[0])
	if !ok {
		t.Fatal("bounded receipt invalid")
	}
	kept := len(publication.Tables[1].Rows)
	if kept <= 0 || kept >= len(p.Rows) || len(publication.Tables[2].Rows) != kept {
		t.Fatal("budget did not omit coupled whole rows")
	}
	for i, row := range publication.Tables[1].Rows {
		if row[2] != p.Rows[i].Record.Name || row[5] != "9007199254740993" {
			t.Fatal("budget truncated individual value")
		}
	}
	after, _ := json.Marshal(p)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("receipt changed original query")
	}
	for _, status := range []string{"available", "unavailable"} {
		p.Status, p.Rows, p.TotalRows = status, nil, 0
		rows := traceQueryProcessMeasurementsObservations(&p, ref, "scope", "now")
		if len(rows) != 1 {
			t.Fatal("empty query lost status")
		}
		publication, ok := types.DecodeRuntimeMeasurementPublication(rows[0])
		if !ok || !strings.Contains(strings.Join(publication.Tables[0].Notes, "\n"), "查询状态："+status) {
			t.Fatal("empty and unavailable conflated")
		}
	}
}
