package context

import (
	stdcontext "context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestPreparedMeasurementNativeNavigationIndependentOfPreview(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(stdcontext.Background(), traceinput.Options{InputPath: path, RuntimeAnchor: t.TempDir(), PreviewBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	got := formatAttachedTrace(m.Preview(), t.TempDir(), attachedTriageProducer, "", attachedTraceRenderOptions{Material: m, PreferTraceQuery: true})
	for _, want := range []string{"Native interval navigation", `"view":"measurements"`, `"source_table":"measure"`, "navigation only", "interval overlap", "unknown"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "For process_measure_interval rows") {
		t.Fatal("generic measure taught as process measure")
	}
	if strings.Count(got, "Native interval navigation") != 1 {
		t.Fatal("duplicate navigation teaching")
	}
}

func TestNativeIntervalManifestNavigationIsBoundedAdvisory(t *testing.T) {
	meta := attachedTraceBundleMetadata{}
	for _, n := range tracequery.NativeIntervalNavigations() {
		meta.TraceDBCoverage = append(meta.TraceDBCoverage, attachedTraceCoverageNavigation{Family: n.CoverageFamily, Table: n.SourceTable, Found: true, RowsEmitted: 1, ArtifactPath: "child.sys"})
	}
	meta.TraceCoverage = append(meta.TraceCoverage, meta.TraceDBCoverage...)
	got := attachedTraceNativeNavigation(meta)
	for _, n := range tracequery.NativeIntervalNavigations() {
		if strings.Count(got, `"view":"`+n.View+`"`) != 1 {
			t.Fatal("missing/duplicate lane", got)
		}
	}
	for _, want := range []string{"navigation only", "Manifest candidates are not proof", `"declared_artifact":"child.sys"`, "same source and explicit window", "do not silently drop", "unknown durations"} {
		if !strings.Contains(got, want) {
			t.Fatal(want, got)
		}
	}
	for _, c := range []attachedTraceCoverageNavigation{
		{Family: "measurement", Table: "measure", Found: false, RowsEmitted: 1},
		{Family: "measurement", Table: "measure", Found: true},
		{Family: "measurement", Table: "measure", Found: true, RowsEmitted: 1, Error: "failed"},
		{Family: "measurement", Table: "measure", Found: true, RowsEmitted: 1, ColumnsMissing: []string{"ts"}},
		{Family: "measurement", Table: "gpufreq", Found: true, RowsEmitted: 1},
		{Family: "counter", Table: "gpu_state", Found: true, RowsEmitted: 1},
	} {
		if attachedTraceNativeNavigation(attachedTraceBundleMetadata{TraceDBCoverage: []attachedTraceCoverageNavigation{c}}) != "" {
			t.Fatal("unsupported metadata created navigation", c)
		}
	}
	meta = attachedTraceBundleMetadata{}
	for i := 0; i < 30; i++ {
		meta.TraceDBCoverage = append(meta.TraceDBCoverage, attachedTraceCoverageNavigation{Family: "measurement", Table: "measure", Found: true, RowsEmitted: 1, ArtifactPath: fmt.Sprintf("child-%d.sys", i)})
	}
	got = attachedTraceNativeNavigation(meta)
	if strings.Count(got, `"view":"measurements"`) != 8 || !strings.Contains(got, "omitted_disclosures=22") || len(got) > 8192 {
		t.Fatal("navigation budget", got)
	}
	meta.TraceDBCoverage[0].ArtifactPath = strings.Repeat("x", 5000)
	got = attachedTraceNativeNavigation(meta)
	if strings.Contains(got, strings.Repeat("x", 1000)) || !strings.Contains(got, "omitted_disclosures=22") {
		t.Fatal("partial long metadata row", got)
	}
}

func TestNativeIntervalVisibleNavigationUsesParsedFamily(t *testing.T) {
	ts, dur, value := int64(-1), int64(2), int64(1)
	line, err := tracewire.FormatCPUMeasureInterval(tracewire.CPUMeasureInterval{RowID: 1, FilterID: 1, CPU: 0, Kind: "idle", Encoding: "native_sql_idle", StartNS: &ts, DurationNS: &dur, Value: &value})
	if err != nil {
		t.Fatal(err)
	}
	got := renderAttachedTraceSemantics(tracePreviewPart{line + "\n", 1, false})
	if !strings.Contains(got, `"view":"cpu_state_frequency"`) || !strings.Contains(got, "parsed visible event families") {
		t.Fatal(got)
	}
	if got := renderAttachedTraceSemanticsWithNavigation(map[tracequery.EventType]bool{tracequery.EventMeasureInterval: true}, tracePreviewPart{line + "\n", 1, false}); !strings.Contains(got, `"view":"cpu_state_frequency"`) {
		t.Fatal("another manifest family suppressed visible CPU navigation")
	}
	if got := renderAttachedTraceSemanticsWithNavigation(map[tracequery.EventType]bool{tracequery.EventCPUMeasureInterval: true}, tracePreviewPart{line + "\n", 1, false}); strings.Contains(got, "Native interval navigation") {
		t.Fatal("same family navigation duplicated")
	}
	for _, raw := range []string{"# gpufreq gpu_state process_measure_interval\n", tracewire.CPUMeasureIntervalPrefix + " record=invalid\n", "a-1 (1) [000] .... 1.000000: tracing_mark_write: C|1|gpufreq|1\n"} {
		if strings.Contains(renderAttachedTraceSemantics(tracePreviewPart{raw, 1, false}), "Native interval navigation") {
			t.Fatal("name acquired navigation", raw)
		}
	}
}
