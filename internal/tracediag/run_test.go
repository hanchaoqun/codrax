package tracediag

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// runTestTrace is a synthetic ftrace-format trace: two threads, sched
// switches/wakeups, a trace_mark pair, clock/frequency rows, one unknown
// event name and one unparsable garbage line.
const runTestTrace = `      waker-10   (   10) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=10 next_prio=20
      waker-10   (   10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
      waker-10   (   10) [000] .... 1.060000: sched_switch: prev_comm=waker prev_pid=10 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
        app-20   (   20) [001] .... 1.100000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
        app-20   (   20) [001] .... 1.150000: print: B|20|Choreographer#doFrame
        app-20   (   20) [001] .... 1.200000: print: E|20
        app-20   (   20) [001] .... 1.220000: cpu_frequency: state=1800000 cpu_id=1
        app-20   (   20) [001] .... 1.240000: clock_set_rate: cpu-cluster.0 state=1400000 cpu_id=0
        app-20   (   20) [001] .... 1.250000: zz_widget_evt: foo=1 bar=2
this line is not a trace line at all
        app-20   (   20) [001] .... 1.300000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-10   (   10) [000] .... 1.400000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
        app-20   (   20) [001] .... 1.450000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
        app-20   (   20) [001] .... 1.500000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func writeRunFixtures(t *testing.T, scriptYAML string) (scriptPath, tracePath, dir string) {
	t.Helper()
	dir = t.TempDir()
	tracePath = filepath.Join(dir, "synthetic.systrace")
	if err := os.WriteFile(tracePath, []byte(runTestTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath = filepath.Join(dir, "collect.yaml")
	if err := os.WriteFile(scriptPath, []byte(scriptYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return scriptPath, tracePath, dir
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
}

const runTestScript = `
version: 1
description: "端到端双步采集"
defaults: { window: "1.0..1.6" }
steps:
  - label: raw_rows
    view: event_search
    thread: "app-20"
    event_types: [sched_switch]
    max_lines: 100
  - label: stats
    view: window_stats
    pid: 20
    max_lines: 400
  - label: rank
    view: root_cause_rank
    pid: 20
    max_lines: 200
`

// End-to-end golden: provenance header, step separators, params echo,
// verbatim raw event rows, detail line families, and the status summary.
func TestRunEndToEndReport(t *testing.T) {
	scriptPath, tracePath, _ := writeRunFixtures(t, runTestScript)
	var buf bytes.Buffer
	failed, err := Run(nil, Options{
		ScriptPath: scriptPath,
		TracePath:  tracePath,
		Version:    "test-1.0",
		BuildTime:  "2026-07-09",
		Now:        fixedNow,
	}, &buf)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if failed != 0 {
		t.Fatalf("failed = %d, want 0\n%s", failed, buf.String())
	}
	report := buf.String()
	for _, want := range []string{
		// provenance header (pin: every field present)
		"# codrax tracediag 采集报告",
		"codrax_version=test-1.0 build_time=2026-07-09",
		"generated_at=2026-07-09T12:00:00Z",
		"size_bytes=",
		"trace_flavor_hint=auto",
		"version=1 steps=3",
		"description=端到端双步采集",
		fmt.Sprintf("line_caps: per_step_default=%d per_step_hard=%d report_total=%d", DefaultStepMaxLines, HardStepMaxLines, DefaultTotalMaxLines),
		// step separators + params echo
		"[步骤 1/3] label=raw_rows view=event_search",
		`thread="app-20"`,
		"window=1.0..1.6",
		"event_types=[sched_switch]",
		"[步骤 2/3] label=stats view=window_stats",
		"pid=20",
		// evidence: the raw sched_switch row verbatim with its coordinates
		"line=4",
		"prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53",
		// detail face: engine Summary line families with trace coordinates
		"明细 window_stats:",
		"- window_stats.state_churn[0]:",
		// index-backed rank view runs end to end too
		"[步骤 3/3] label=rank view=root_cause_rank",
		"明细 root_cause_rank:",
		// status summary
		"[步骤状态摘要]",
		"- 步骤 1 label=raw_rows view=event_search 状态=成功",
		"- 步骤 2 label=stats view=window_stats 状态=成功",
		"- 步骤 3 label=rank view=root_cause_rank 状态=成功",
		"结论: 全部 3 步骤成功。",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q\n----\n%s", want, report)
		}
	}
	// The engine's json:"-" fields are "not an observation face" by explicit
	// ruling (CFC: WindowStats.ClusterFrequencyCeilings) — the walker must
	// honor the exclusion instead of minting a new display surface.
	if strings.Contains(report, "ClusterFrequencyCeilings") {
		t.Errorf("json:\"-\" engine-internal field leaked into the report\n%s", report)
	}
}

// Purity pin: a run must not modify the trace file, and must leave no new
// files behind (the only write surface is the caller-chosen out writer).
func TestRunIsPureRead(t *testing.T) {
	scriptPath, tracePath, dir := writeRunFixtures(t, runTestScript)
	statBefore, err := os.Stat(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	hashBefore := sha256.Sum256(mustRead(t, tracePath))
	entriesBefore := listDir(t, dir)

	var buf bytes.Buffer
	if _, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf); err != nil {
		t.Fatalf("Run: %v", err)
	}

	statAfter, err := os.Stat(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	if !statAfter.ModTime().Equal(statBefore.ModTime()) || statAfter.Size() != statBefore.Size() {
		t.Fatalf("trace file changed: %v/%d → %v/%d", statBefore.ModTime(), statBefore.Size(), statAfter.ModTime(), statAfter.Size())
	}
	if sha256.Sum256(mustRead(t, tracePath)) != hashBefore {
		t.Fatal("trace file content changed")
	}
	if got := listDir(t, dir); got != entriesBefore {
		t.Fatalf("directory contents changed:\nbefore=%s\nafter=%s", entriesBefore, got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func listDir(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return strings.Join(names, ",")
}

// Per-step line cap: body truncation must be disclosed with the verbatim
// N/M/X form (自设最强突变: a renderer that silently stops clamping, or
// clamps without disclosure, reddens here).
func TestRunPerStepLineCapDisclosure(t *testing.T) {
	scriptYAML := `
version: 1
steps:
  - label: tiny
    view: window_stats
    pid: 20
    window: "1.0..1.6"
    max_lines: 5
`
	scriptPath, tracePath, _ := writeRunFixtures(t, scriptYAML)
	var buf bytes.Buffer
	if _, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf); err != nil {
		t.Fatalf("Run: %v", err)
	}
	report := buf.String()
	re := regexp.MustCompile(`…共 (\d+) 行,按帽截断至 5,余 (\d+) 行未列`)
	m := re.FindStringSubmatch(report)
	if m == nil {
		t.Fatalf("missing verbatim truncation disclosure\n%s", report)
	}
	// The disclosure arithmetic must be honest: N = 5 + X.
	var total, rest int
	fmt.Sscanf(m[1], "%d", &total)
	fmt.Sscanf(m[2], "%d", &rest)
	if total != 5+rest || rest < 1 {
		t.Fatalf("dishonest disclosure: total=%d rest=%d", total, rest)
	}
	if !strings.Contains(report, "输出行=5/"+m[1]) {
		t.Errorf("status summary must carry shown/total = 5/%s\n%s", m[1], report)
	}
}

// max_lines above the hard cap is clamped WITH a disclosure line.
func TestRunMaxLinesHardCapDisclosure(t *testing.T) {
	scriptYAML := `
version: 1
steps:
  - label: big
    view: event_search
    max_lines: 5000
`
	scriptPath, tracePath, _ := writeRunFixtures(t, scriptYAML)
	var buf bytes.Buffer
	if _, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := fmt.Sprintf("⚠ max_lines=5000 超过硬帽 %d,已夹取为 %d", HardStepMaxLines, HardStepMaxLines)
	if !strings.Contains(buf.String(), want) {
		t.Fatalf("missing hard-cap clamp disclosure %q\n%s", want, buf.String())
	}
}

// event_search max_lines is a REPORT-body cap, not an Events-only cap. Pin
// the static lane that originally returned max_lines raw rows, then lost the
// final three behind result/window/header metadata while still claiming all N
// were visible. The priority face must expose matched/emitted/compaction and
// its caveat inside the cap, and the header count must equal visible raw rows.
func TestRunStaticEventSearchVisibleBudgetAccounting(t *testing.T) {
	var trace strings.Builder
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&trace, "waker-%d ( %d) [000] .... 1.%06d: sched_wakeup: comm=app pid=%d prio=53 target_cpu=001\n",
			10+i, 10+i, 100000+i, 20+i)
	}
	scriptYAML := `
version: 1
steps:
  - label: static_rows
    view: event_search
    window: "1.0..2.0"
    event_types: [sched_wakeup]
    max_lines: 7
`
	scriptPath, tracePath, _ := writeRunFixtures(t, scriptYAML)
	if err := os.WriteFile(tracePath, []byte(trace.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf)
	if err != nil || failed != 0 {
		t.Fatalf("Run(static compacted event_search): failed=%d err=%v\n%s", failed, err, buf.String())
	}
	report := buf.String()
	headerRE := regexp.MustCompile(`匹配事件 (\d+) 行 \(matched=(\d+) emitted=(\d+) compacted=true caveat=report_event_search_compacted=true diagnostics=\d+/\d+ details=\d+/\d+ coverage=\{[^}]+\}\):`)
	header := headerRE.FindStringSubmatch(report)
	if header == nil {
		t.Fatalf("static match accounting is not priority-visible\n%s", report)
	}
	var headerRows, matched, emitted int
	fmt.Sscanf(header[1], "%d", &headerRows)
	fmt.Sscanf(header[2], "%d", &matched)
	fmt.Sscanf(header[3], "%d", &emitted)
	visibleRaw := len(regexp.MustCompile(`(?m)^- line=\d+ .* type=sched_wakeup \|`).FindAllString(report, -1))
	if matched != 8 || emitted != headerRows || headerRows != visibleRaw {
		t.Fatalf("header/raw accounting drifted: matched=%d emitted=%d header=%d visible_raw=%d\n%s",
			matched, emitted, headerRows, visibleRaw, report)
	}
	for _, token := range []string{
		"- 报告截断账目: view=event_search dimension=events matched=8 emitted=",
		"- 报告完整性提示: report_event_search_compacted=true; matched=8 emitted=",
		"- 引擎 caveat 原文: streamed_event_search=true;",
		"输出行=7/",
	} {
		if !strings.Contains(report, token) {
			t.Errorf("static compacted report missing %q\n%s", token, report)
		}
	}
	if strings.Contains(report, "观测记录(引擎 evidence") {
		t.Fatalf("event_search duplicated its raw rows through EvidencePack\n%s", report)
	}
	if strings.Contains(report, "caveats(引擎原文") || strings.Contains(report, "引擎截断记录: view=event_search") {
		t.Fatalf("report-level compaction was mislabeled as engine-originated\n%s", report)
	}
	if strings.Contains(report, "…共 ") {
		t.Fatalf("event_search raw/diagnostic/detail compaction was collapsed into the generic body-line ruler\n%s", report)
	}
	// Window provenance must not change the engine's raw budget; generated
	// and static instances enter the same deterministic limit helper.
	if got := eventSearchEngineRowLimit(7); got != 4 {
		t.Fatalf("event_search engine raw budget=%d, want max_lines-base=4", got)
	}
}

func TestRunTraceMarkActionsEchoExactRowsAndIntegrity(t *testing.T) {
	scriptYAML := `
version: 1
steps:
  - label: async_starts
    view: event_search
    window: "1.0..1.1"
    trace_mark_actions: [S]
    max_lines: 30
`
	scriptPath, tracePath, _ := writeRunFixtures(t, scriptYAML)
	trace := strings.Join([]string{
		` app-10 (10) [000] .... 1.000000: tracing_mark_write: C|10|counter|1|S|10|`,
		` app-10 (10) [000] .... 1.010000: tracing_mark_write: S|10|first-async|7`,
		` app-10 (10) [000] .... 1.020000: tracing_mark_write: S|bad|malformed|8`,
		` app-10 (10) [000] .... 1.030000: tracing_mark_write: S|10|second-async|9`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf)
	if err != nil || failed != 0 {
		t.Fatalf("Run(action filter): failed=%d err=%v\n%s", failed, err, buf.String())
	}
	report := buf.String()
	for _, want := range []string{
		"trace_mark_actions=[S]",
		"matched=2 emitted=2 compacted=false",
		"S|10|first-async|7",
		"S|10|second-async|9",
		"trace_mark_integrity_degraded=true",
		"invalid_payload_pid",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("action report missing %q\n%s", want, report)
		}
	}
	for _, forbidden := range []string{"C|10|counter|1|S|10|", "S|bad|malformed|8"} {
		if strings.Contains(report, forbidden) {
			t.Errorf("action report re-admitted raw-prefix/malformed row %q\n%s", forbidden, report)
		}
	}
}

// Total report cap: when the whole-report budget is exhausted the writer
// discloses ONCE, keeps executing every step, and the status summary always
// lands (自设最强突变: removing the cap or the summary bypass reddens here).
func TestRunTotalLineCap(t *testing.T) {
	scriptPath, tracePath, _ := writeRunFixtures(t, runTestScript)
	var buf bytes.Buffer
	failed, err := Run(nil, Options{
		ScriptPath:    scriptPath,
		TracePath:     tracePath,
		Now:           fixedNow,
		TotalMaxLines: 12,
	}, &buf)
	if err != nil || failed != 0 {
		t.Fatalf("Run: failed=%d err=%v", failed, err)
	}
	report := buf.String()
	if got := strings.Count(report, "报告总行帽 12 已达"); got != 1 {
		t.Fatalf("total-cap disclosure must appear exactly once, got %d\n%s", got, report)
	}
	for _, want := range []string{
		"[步骤状态摘要]",
		"- 步骤 1 label=raw_rows",
		"- 步骤 2 label=stats",
		"- 总行帽截断:",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q under total cap\n%s", want, report)
		}
	}
}

// Step failure discipline: an engine error is rendered verbatim, the run
// CONTINUES through every remaining step, and Run reports the failure count
// for the nonzero exit (自设最强突变: an early return on step error reddens
// here because step 3 would vanish from the report).
func TestRunStepFailureContinuesAndCounts(t *testing.T) {
	scriptYAML := `
version: 1
steps:
  - {label: heavy1, view: window_stats, pid: 20, window: "1.0..1.6"}
  - {label: streaming, view: event_search, thread: "app-20"}
  - {label: heavy2, view: root_cause_rank, pid: 20, window: "1.0..1.6"}
`
	scriptPath, tracePath, _ := writeRunFixtures(t, scriptYAML)
	old := stepIndexMaxEvents
	stepIndexMaxEvents = 2 // trip the engine's typed IndexEventLimitError
	defer func() { stepIndexMaxEvents = old }()

	var buf bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf)
	if err != nil {
		t.Fatalf("Run must complete the report even with failing steps: %v", err)
	}
	if failed != 2 {
		t.Fatalf("failed = %d, want 2\n%s", failed, buf.String())
	}
	report := buf.String()
	for _, want := range []string{
		"[步骤失败] engine error (verbatim):",
		"[步骤 2/3] label=streaming",
		"- 步骤 1 label=heavy1 view=window_stats 状态=失败:",
		"- 步骤 2 label=streaming view=event_search 状态=成功",
		"- 步骤 3 label=heavy2 view=root_cause_rank 状态=失败:",
		"结论: 2/3 步骤失败;报告仍为全量采集(失败步骤错误原文见各节)。",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q\n%s", want, report)
		}
	}
	// The successful streaming step's evidence must still be present AFTER a
	// failed step — the strongest witness that execution continued.
	if !strings.Contains(report, "prev_state=R ==> next_comm=app") {
		t.Errorf("streaming step evidence missing after failed step\n%s", report)
	}
}

// Strict --trace-flavor enum: unknown values fail loud instead of the
// engine's silent →auto normalization.
func TestRunUnknownFlavorFailLoud(t *testing.T) {
	scriptPath, tracePath, _ := writeRunFixtures(t, runTestScript)
	var buf bytes.Buffer
	_, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, FlavorHint: "harmony", Now: fixedNow}, &buf)
	if err == nil || !strings.Contains(err.Error(), "harmony_hitrace") {
		t.Fatalf("unknown flavor must fail loud listing the enum, got %v", err)
	}
}

// P1-2 pin (对抗复核 2026-07-09, 自设最强突变: reverting formatSecondsToken
// to the 'g' form reddens here): berlin-magnitude second coordinates render
// fixed-point (6 decimals, the trace file's own timestamp form) — never
// scientific notation anywhere in the report.
func TestRunBerlinMagnitudeCoordinatesFixedPoint(t *testing.T) {
	trace := `      waker-10   (   10) [000] .... 6793222.100000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=10 next_prio=20
        app-20   (   20) [001] .... 6793224.900123: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
        app-20   (   20) [001] .... 6793225.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
`
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "berlin_like.systrace")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "s.yaml")
	script := `
version: 1
steps:
  - label: rows
    view: event_search
    thread: "app-20"
    window: "6793222.031..6793225.370"
    line_start: 1
    line_end: 3
    max_lines: 50
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf); err != nil || failed != 0 {
		t.Fatalf("Run: failed=%d err=%v\n%s", failed, err, buf.String())
	}
	report := buf.String()
	for _, want := range []string{
		// event row ts in the trace file's own fixed-point form
		"ts=6793224.900123",
		// query window endpoints fixed-point
		"query=[6793222.031000..6793225.370000]",
		// P1-1: window+line coexistence discloses the engine gate semantics
		"注: 行区间生效时时间窗不参与过滤(引擎语义:line 界恒生效,time 界仅在无 line 界时生效)",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q\n%s", want, report)
		}
	}
	if strings.Contains(report, "e+06") || strings.Contains(report, "E+06") {
		t.Errorf("scientific notation leaked into trace coordinates\n%s", report)
	}
}

// P2-2 pin (对抗复核 2026-07-09): two runs over the same inputs with a fixed
// clock produce BYTE-IDENTICAL reports. The script deliberately includes a
// census step and a window_stats step (multi-key maps: event_counts, census
// name/prefix/prio/prev_state maps) — the faces where a dropped sort would
// surface as run-to-run ordering jitter (自设最强突变: removing the
// formatMapTokens sort reddens here).
func TestRunDoubleRunByteIdentical(t *testing.T) {
	scriptYAML := `
version: 1
steps:
  - {label: stats, view: window_stats, pid: 20, window: "1.0..1.6", max_lines: 400}
  - {label: census, view: format_census, max_lines: 400}
`
	scriptPath, tracePath, _ := writeRunFixtures(t, scriptYAML)
	run := func() []byte {
		var buf bytes.Buffer
		failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf)
		if err != nil || failed != 0 {
			t.Fatalf("Run: failed=%d err=%v", failed, err)
		}
		return buf.Bytes()
	}
	first, second := run(), run()
	if !bytes.Equal(first, second) {
		t.Fatalf("two identical runs produced different reports:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

// P3-1 literal pin: the windowed-index mirror constants are hard-coded here
// so a drift on EITHER side (this package or the tool source it mirrors —
// internal/tool/trace_query.go :78 min-bytes, :1599 per-view padding,
// :1510-1512 line padding) turns a test red and forces a re-audit of the
// mirror instead of a silent divergence.
func TestWindowedIndexMirrorLiteralsPinned(t *testing.T) {
	if traceDiagWindowedIndexMinBytes != 64<<20 {
		t.Errorf("min bytes = %d, pinned mirror of tool traceQueryWindowedIndexMinBytes is 64MiB", traceDiagWindowedIndexMinBytes)
	}
	if !traceDiagUseWindowedIndex(filepath.Join(t.TempDir(), "small.tracebundle.json"), 1, true) {
		t.Fatal("explicit composite window incorrectly used tiny manifest size to select a full-child index")
	}
	if traceDiagUseWindowedIndex(filepath.Join(t.TempDir(), "small.tracebundle.json"), 1, false) {
		t.Fatal("unbounded composite query silently acquired windowed-parse semantics")
	}
	if traceDiagWindowedIndexLinePadding != 200 {
		t.Errorf("line padding = %d, pinned mirror of tool traceQueryWindowedIndexOptions is 200", traceDiagWindowedIndexLinePadding)
	}
	for view, want := range map[string]float64{
		"event_search":            0.050,
		"thread_timeline":         0.250,
		"scheduler_latency_stats": 0.250,
		"window_stats":            0.500,
		"root_cause_rank":         0.500,
	} {
		if got := traceDiagWindowedIndexTimePadding(view); got != want {
			t.Errorf("time padding(%s) = %v, pinned mirror of tool traceQueryWindowedIndexTimePadding is %v", view, got, want)
		}
	}
}

func TestRunMissingTraceFailLoud(t *testing.T) {
	scriptPath, _, dir := writeRunFixtures(t, runTestScript)
	var buf bytes.Buffer
	if _, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: filepath.Join(dir, "nope.systrace"), Now: fixedNow}, &buf); err == nil {
		t.Fatal("missing trace must fail loud")
	}
	if _, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: dir, Now: fixedNow}, &buf); err == nil {
		t.Fatal("directory trace path must fail loud")
	}
}
