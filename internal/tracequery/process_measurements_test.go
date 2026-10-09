package tracequery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func processMeasureTestInt(v int64) tracewire.ProcessMeasureScalar {
	return tracewire.ProcessMeasureScalar{Status: "known", StorageClass: "integer", Value: fmt.Sprint(v)}
}
func processMeasureTestRecord(id, ts, dur, value int64, pid int) tracewire.ProcessMeasureInterval {
	return tracewire.ProcessMeasureInterval{RowID: id, StartNS: processMeasureTestInt(ts), DurationNS: processMeasureTestInt(dur), Value: processMeasureTestInt(value), FilterID: processMeasureTestInt(1), IPID: processMeasureTestInt(int64(pid)), Name: "same_metric", NameKnown: true, PID: &pid, ProcessName: "same", OwnerStatus: "known"}
}
func processMeasureTestText(t *testing.T, rows ...tracewire.ProcessMeasureInterval) string {
	t.Helper()
	var b strings.Builder
	for _, r := range rows {
		line, err := tracewire.FormatProcessMeasureInterval(r)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
func processMeasureTestRun(t *testing.T, idx *Index, q Query) *ProcessMeasurementsResult {
	t.Helper()
	q.View = ViewProcessMeasurements
	got := Run(idx, q)
	if got.ProcessMeasurements == nil || !ValidProcessMeasurements(*got.ProcessMeasurements) {
		t.Fatalf("invalid process result: %+v", got)
	}
	if got.RootCauseRank != nil || got.CPUStateFrequency != nil || got.Timeline != nil || got.WindowStats != nil {
		t.Fatal("process measurement acquired unrelated authority")
	}
	return got.ProcessMeasurements
}

func TestProcessMeasurementsExactWindowAndIdentity(t *testing.T) {
	unknown := processMeasureTestRecord(5, 15, 5, 7, 202)
	unknown.DurationNS = tracewire.ProcessMeasureScalar{Status: "null", StorageClass: "null"}
	unknown.Value = unknown.DurationNS
	badTS := processMeasureTestRecord(6, 0, 1, 0, 101)
	badTS.StartNS = tracewire.ProcessMeasureScalar{Status: "invalid_storage", StorageClass: "text", Value: "12"}
	idx := renderingFixture(t, processMeasureTestText(t,
		processMeasureTestRecord(1, -10, 20, 9223372036854775807, 101),
		processMeasureTestRecord(2, 0, 0, 0, 101),
		processMeasureTestRecord(3, 10, 15, 2, 202),
		processMeasureTestRecord(4, 20, 5, 3, 101), unknown, badTS))
	p := processMeasureTestRun(t, idx, Query{TimeStartSet: true, TimeEnd: .00000002})
	if p.Status != "available" || p.TotalRows != 4 || p.UnpositionedRows != 1 || p.TargetScope != "process" {
		t.Fatalf("wrong census/window: %+v", p)
	}
	if p.Rows[0].Record.StartNS.Value != "-10" || p.Rows[0].Record.DurationNS.Value != "20" || *p.Rows[0].ClippedStartNS != 0 || *p.Rows[0].ClippedEndNS != 10 || p.Rows[0].Record.Value.Value != "9223372036854775807" {
		t.Fatal("carry-in raw or clipped coordinates lost", p.Rows[0])
	}
	if p.Rows[1].Selection != "point" || p.Rows[1].Record.Value.Value != "0" || p.Rows[3].Selection != "unknown_duration" || p.Rows[3].ClippedEndNS != nil {
		t.Fatal("zero/NULL interval fabricated", p.Rows)
	}
	for _, q := range []Query{{PID: 101}, {PID: 202}} {
		q.TimeStartSet = true
		q.TimeEnd = .00000002
		got := processMeasureTestRun(t, idx, q)
		if got.TotalRows != 2 {
			t.Fatal("same name process leakage", got)
		}
	}
	for _, q := range []Query{{Thread: "same"}, {Thread: "same-101"}, {PID: 101, TargetScope: "thread"}, {Pattern: "same_metric"}} {
		q.TimeStartSet = true
		q.TimeEnd = .00000002
		if got := processMeasureTestRun(t, idx, q); got.Status != "unavailable" {
			t.Fatalf("unsupported selector got authority: %+v", got)
		}
	}
	for _, ev := range idx.Events {
		coords := ProjectTraceEventInventoryCoordinates(ev)
		if coords.CPUKnown || coords.EmitterTIDKnown || coords.EmitterTGIDKnown {
			t.Fatal("process forged emitter or CPU", coords)
		}
		semantic := ProjectTraceEventSemantics(ev)
		if semantic == nil || !types.ValidateTraceEventSemantics(semantic) {
			t.Fatalf("invalid readable source semantics: %+v", semantic)
		}
		expectEventSemanticValue(t, semantic, "source.table", "process_measure")
		expectEventSemanticValue(t, semantic, "plugin.metric", "same_metric")
		expectEventSemanticValue(t, semantic, "source.subject_role", "process_measurement_not_thread_execution")
		if ev.PluginFields.ProcessMeasure.RowID == 1 {
			expectEventSemanticValue(t, semantic, "source.start_ns", "-10")
			expectEventSemanticValue(t, semantic, "plugin.value", "9223372036854775807")
		}
		b, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		var round Event
		if err = json.Unmarshal(b, &round); err != nil || !reflect.DeepEqual(ev.PluginFields, round.PluginFields) {
			t.Fatal("JSON cache lost typed source", err)
		}
	}
}

func TestProcessMeasurementsNoFillOverlapAndTamper(t *testing.T) {
	idx := renderingFixture(t, processMeasureTestText(t, processMeasureTestRecord(1, 0, 10, 7, 101), processMeasureTestRecord(2, 5, 10, 8, 101), processMeasureTestRecord(3, 30, -1, 9, 101)))
	p := processMeasureTestRun(t, idx, Query{TimeStartSet: true, TimeEnd: .00000004})
	if p.TotalRows != 3 || *p.Rows[0].ClippedEndNS != 10 || *p.Rows[1].ClippedStartNS != 5 || p.Rows[2].Selection != "unknown_duration" {
		t.Fatal("overlap merged/gap filled", p)
	}
	for _, mutate := range []func(*ProcessMeasurementsResult){func(p *ProcessMeasurementsResult) { p.Rows[0].Unit = "bytes" }, func(p *ProcessMeasurementsResult) { p.TotalRows++ }, func(p *ProcessMeasurementsResult) { v := int64(11); p.Rows[0].ClippedEndNS = &v }, func(p *ProcessMeasurementsResult) { p.Rows[1].Record.RowID = p.Rows[0].Record.RowID }, func(p *ProcessMeasurementsResult) { p.TargetPID = 202 }} {
		b, _ := json.Marshal(p)
		var bad ProcessMeasurementsResult
		_ = json.Unmarshal(b, &bad)
		mutate(&bad)
		if ValidProcessMeasurements(bad) {
			t.Fatal("invalid display bound accepted", bad)
		}
	}
	idx.Windowed = true
	if processMeasureTestRun(t, idx, Query{TimeStartSet: true, TimeEnd: .00000004}).Status != "unavailable" {
		t.Fatal("windowed index claimed complete carry-in")
	}
}

func TestProcessMeasurementsStreamCompleteAndBounded(t *testing.T) {
	var rows []tracewire.ProcessMeasureInterval
	for i := int64(1); i <= 100; i++ {
		rows = append(rows, processMeasureTestRecord(i, i, 100, 7, 101))
	}
	body := processMeasureTestText(t, rows...)
	idx := renderingFixture(t, body)
	q := Query{View: ViewProcessMeasurements, TimeStart: .00000005, TimeEnd: .00000006, Limit: 64}
	p := processMeasureTestRun(t, idx, q)
	got, err := StreamProcessMeasurements(context.Background(), idx.Path, q)
	if err != nil || !reflect.DeepEqual(got.ProcessMeasurements, p) || p.TotalRows != 59 || p.OmittedRows != 0 {
		t.Fatalf("stream lost carry-in %v %+v", err, got.ProcessMeasurements)
	}
	p = processMeasureTestRun(t, idx, Query{TimeStartSet: true, TimeEnd: .0000002, Limit: 2})
	if p.TotalRows != 100 || p.OmittedRows != 98 || len(p.Rows) != 2 {
		t.Fatal("limit claimed completeness", p)
	}
	for _, limits := range [][2]int{{1, 1 << 20}, {1000, 10}} {
		got, err := streamProcessMeasurements(context.Background(), idx.Path, q, limits[0], limits[1])
		if err != nil || got.ProcessMeasurements.Status != "unavailable" || len(got.ProcessMeasurements.Rows) != 0 || got.ScannedLineCount != idx.LineCount {
			t.Fatal("retention cap published partial", got, err)
		}
	}
	for _, tail := range []string{strings.Split(body, "\n")[0] + "\n", tracewire.ProcessMeasureIntervalPrefix + " record=broken\n"} {
		bad := renderingFixture(t, body+strings.Repeat("# padding\n", 200)+tail)
		got, err := StreamProcessMeasurements(context.Background(), bad.Path, q)
		if err != nil || got.ProcessMeasurements.Status != "unavailable" || len(got.ProcessMeasurements.Rows) > 0 {
			t.Fatal("bad tail ignored", got, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := StreamProcessMeasurements(ctx, idx.Path, q); !errors.Is(err, context.Canceled) || got.ProcessMeasurements != nil {
		t.Fatal("cancel promoted", got, err)
	}
}

func TestProcessMeasurementsBundleIdentity(t *testing.T) {
	for _, kind := range []string{"identity", "multi", "affine", "stale"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			child, bundle := filepath.Join(dir, "events.systrace"), filepath.Join(dir, "capture.tracebundle.json")
			body := processMeasureTestText(t, processMeasureTestRecord(1, -10, 30, 1, 101))
			if err := os.WriteFile(child, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			manifest := `{"systrace":"events.systrace"}`
			if kind == "multi" {
				if err := os.WriteFile(filepath.Join(dir, "other.systrace"), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				manifest = `{"systrace":"events.systrace","artifacts":[{"type":"systrace","path":"other.systrace"}]}`
			}
			if kind == "affine" {
				manifest = `{"systrace":"events.systrace","perf_clock_alignments":[{"artifact_path":"events.systrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","offset_sec":1,"slope":1,"calibrated":true}]}`
			}
			writeTraceBundleV2ForTest(t, bundle, []byte(manifest))
			if kind == "stale" {
				_ = os.WriteFile(child, []byte(body+"# changed\n"), 0600)
			}
			got, err := StreamProcessMeasurements(context.Background(), bundle, Query{View: ViewProcessMeasurements, TimeStartSet: true, TimeEnd: .00000001})
			if kind != "identity" {
				if err == nil || got.ProcessMeasurements != nil {
					t.Fatal("unverified source joined", got, err)
				}
				return
			}
			if err != nil || got.ProcessMeasurements.TotalRows != 1 || got.ProcessMeasurements.Rows[0].SourcePath != canonicalTraceIndexPath(child) {
				t.Fatal("verified single source rejected", got, err)
			}
		})
	}
}

func TestProcessMeasurementsEventDiscoveryUsesSourceTimeAndNames(t *testing.T) {
	unknown := processMeasureTestRecord(4, 0, 10, 1, 101)
	unknown.StartNS = tracewire.ProcessMeasureScalar{Status: "null", StorageClass: "null"}
	idx := renderingFixture(t, processMeasureTestText(t, processMeasureTestRecord(1, -1, 10, 1, 101), processMeasureTestRecord(2, 0, 10, 2, 101), processMeasureTestRecord(3, 10, 10, 3, 101), unknown))
	for _, q := range []Query{{View: "event_search", Pattern: "same_metric"}, {View: "event_search", Pattern: "no_such_metric"}, {View: "event_search", Pattern: "same_metric", TimeStartSet: true, TimeEnd: .00000001}} {
		indexed := Run(idx, q)
		stream, err := StreamEventSearch(context.Background(), idx.Path, q)
		if err != nil {
			t.Fatal(err)
		}
		want := 4
		if q.Pattern == "no_such_metric" {
			want = 0
		}
		if q.TimeEnd > 0 {
			want = 1
		}
		if len(indexed.Events) != want || len(stream.Events) != want {
			t.Fatalf("point discovery wrong: q=%+v indexed=%+v stream=%+v", q, indexed.Events, stream.Events)
		}
		if want == 1 && indexed.Events[0].PluginFields.ProcessMeasure.RowID != 2 {
			t.Fatal("sort zero granted source-time authority")
		}
	}
	for _, ev := range idx.Events {
		ts, known := ProcessMeasurementSourceTimestamp(ev)
		r := ev.PluginFields.ProcessMeasure
		if r.RowID == 1 && (!known || ts != -1) || r.RowID == 4 && known {
			t.Fatal("source coordinate erased", r, ts, known)
		}
	}
}
