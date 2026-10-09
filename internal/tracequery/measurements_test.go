package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func measurementTestInt(n int64) tracewire.MeasureScalar {
	return tracewire.MeasureScalar{StorageClass: "integer", Value: fmt.Sprint(n)}
}
func measurementTestRecord(id, ts, dur int64) tracewire.MeasureInterval {
	return tracewire.MeasureInterval{RowID: id, StartNS: measurementTestInt(ts), DurationNS: measurementTestInt(dur), Value: measurementTestInt(9007199254740993), FilterID: measurementTestInt(-5), MeasureType: tracewire.MeasureScalar{StorageClass: "text", Value: "measure"}, FilterStatus: "observed_unique", Filter: &tracewire.MeasureFilter{ID: measurementTestInt(-5), Name: tracewire.MeasureScalar{StorageClass: "text", Value: "gpufreq"}, Type: tracewire.MeasureScalar{StorageClass: "text", Value: "measure_filter"}, SourceArgSetID: measurementTestInt(99)}}
}
func measurementTestText(t *testing.T, rows ...tracewire.MeasureInterval) string {
	t.Helper()
	var b strings.Builder
	for _, r := range rows {
		line, err := tracewire.FormatMeasureInterval(r)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
func measurementsTestRun(t *testing.T, idx *Index, q Query) *MeasurementsResult {
	t.Helper()
	q.View = ViewMeasurements
	result := Run(idx, q)
	if result.Measurements == nil || !ValidMeasurements(*result.Measurements) {
		t.Fatalf("invalid result: %+v", result)
	}
	if result.RootCauseRank != nil || result.CPUStateFrequency != nil || result.WindowStats != nil || result.Timeline != nil {
		t.Fatal("raw measurement acquired unrelated authority")
	}
	return result.Measurements
}
func TestMeasurementsExactIntervalsUnknownsAndLimit(t *testing.T) {
	unknown := measurementTestRecord(6, 12, 0)
	unknown.StartNS = tracewire.MeasureScalar{StorageClass: "text", Value: "12"}
	idx := renderingFixture(t, measurementTestText(t, measurementTestRecord(1, -5, 10), measurementTestRecord(2, 0, 0), measurementTestRecord(3, 5, 10), measurementTestRecord(4, 20, 2), measurementTestRecord(5, 15, -1), unknown))
	q := Query{TimeStartSet: true, TimeEnd: .00000002, Limit: 64}
	p := measurementsTestRun(t, idx, q)
	if p.Status != "available" || p.TotalRows != 4 || p.UnpositionedRows != 1 || len(p.Rows) != 4 {
		t.Fatal(p)
	}
	if *p.Rows[0].ClippedStartNS != 0 || *p.Rows[0].ClippedEndNS != 5 || p.Rows[0].Record.StartNS.Value != "-5" || p.Rows[1].Selection != "point" || p.Rows[3].ClippedEndNS != nil {
		t.Fatal(p.Rows)
	}
	q.Limit = 1
	limited := measurementsTestRun(t, idx, q)
	if limited.TotalRows != 4 || limited.OmittedRows != 3 || len(limited.Rows) != 1 || limited.UnpositionedRows != 1 {
		t.Fatal(limited)
	}
	for _, m := range []func(*MeasurementsResult){func(p *MeasurementsResult) { p.TotalRows++ }, func(p *MeasurementsResult) { p.Rows[0].Unit = "Hz" }, func(p *MeasurementsResult) { p.Rows[0].Record.Filter.ID = measurementTestInt(99) }, func(p *MeasurementsResult) { v := int64(6); p.Rows[0].ClippedEndNS = &v }} {
		b, _ := json.Marshal(p)
		var bad MeasurementsResult
		_ = json.Unmarshal(b, &bad)
		m(&bad)
		if ValidMeasurements(bad) {
			t.Fatal("tampered measurement admitted")
		}
	}
	for _, badQ := range []Query{{PID: 99}, {Thread: "worker"}, {TargetScope: TargetScopeProcess}, {Pattern: "freq"}, {Patterns: []string{"freq"}}, {SpanName: "frame"}} {
		badQ.TimeStartSet = true
		badQ.TimeEnd = .00000002
		if measurementsTestRun(t, idx, badQ).Status != "unavailable" {
			t.Fatal("owner or filter silently ignored", badQ)
		}
	}
	idx.Windowed = true
	if measurementsTestRun(t, idx, q).Status != "unavailable" {
		t.Fatal("partial index granted completeness")
	}
}

func TestMeasurementsScanMemoAndSemanticInventory(t *testing.T) {
	r := measurementTestRecord(1, -5, 10)
	line := strings.TrimSpace(measurementTestText(t, r))
	var scan lineScan
	scan.reset(1, line)
	_, ok := scan.timestamp()
	if !ok || scan.measure == nil {
		t.Fatal("timestamp failed")
	}
	ptr := scan.measure
	ev, ok := parseLineScan(&scan, newStringInterner())
	if !ok || ev.PluginFields.Measure != ptr || scan.measureInterval() != ptr {
		t.Fatal("carrier decoded more than once")
	}
	if ns, known := MeasurementSourceTimestamp(ev); !known || ns != -5 {
		t.Fatal(ns, known)
	}
	coords := ProjectTraceEventInventoryCoordinates(ev)
	if coords.CPUKnown || coords.EmitterTIDKnown || coords.EmitterTGIDKnown {
		t.Fatal("borrowed raw ID as CPU/thread", coords)
	}
	sem := ProjectTraceEventSemantics(ev)
	if !types.ValidateTraceEventSemantics(sem) {
		t.Fatal(sem)
	}
	expectEventSemanticValue(t, sem, "source.start_ns", "-5")
	expectEventSemanticValue(t, sem, "plugin.metric", "gpufreq")
	if !eventMatchesPattern(ev, "gpufreq") || !streamEventSearchRawCandidate(line, 1, Query{Pattern: "gpufreq"}) {
		t.Fatal("encoded metadata not discoverable")
	}
	b, _ := json.Marshal(ev)
	var decoded Event
	if json.Unmarshal(b, &decoded) != nil || !reflect.DeepEqual(ev.PluginFields, decoded.PluginFields) {
		t.Fatal("JSON cache lost typed scalar")
	}
	scan.reset(2, "ordinary non-event")
	if scan.measureInterval() != nil {
		t.Fatal("memo leaked across lines")
	}
	if allocs := testing.AllocsPerRun(100, func() { scan.reset(2, "ordinary non-event"); _ = scan.measureInterval() }); allocs != 0 {
		t.Fatal("ordinary line carrier probe allocates", allocs)
	}
}

func TestMeasurementsStreamCompletenessAndMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "m.systrace")
	body := measurementTestText(t, measurementTestRecord(1, 0, 10), measurementTestRecord(2, 10, 10), measurementTestRecord(3, 100, 10))
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	q := Query{View: ViewMeasurements, TimeStartSet: true, TimeEnd: .00000002, Limit: 64}
	got, err := StreamMeasurements(context.Background(), path, q)
	if err != nil || got.Measurements == nil || got.Measurements.TotalRows != 2 {
		t.Fatal(got, err)
	}
	limited, err := streamMeasurements(context.Background(), path, q, 2, 1<<20)
	if err != nil || limited.Measurements.Status != "unavailable" || limited.Measurements.TotalRows != 0 {
		t.Fatal("retention prefix published", limited, err)
	}
	dup := body + measurementTestText(t, measurementTestRecord(1, 200, 10))
	if err := os.WriteFile(path, []byte(dup), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = StreamMeasurements(context.Background(), path, q)
	if err != nil || got.Measurements.Status != "unavailable" {
		t.Fatal("outside-window duplicate ignored", got, err)
	}
	if err := os.WriteFile(path, []byte(body+"# codrax_measure_interval/v1 record=broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = StreamMeasurements(context.Background(), path, q)
	if err != nil || got.Measurements.Status != "unavailable" {
		t.Fatal("malformed source erased", got, err)
	}
}
