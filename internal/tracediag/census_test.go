package tracediag

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// censusTrace exercises every census face: known sched/power/clock/marker
// rows, 35 distinct unknown event names (over the 30-name unknown cap), a
// prio-301 thread, file kv rows, and two unparsable garbage lines.
func censusTraceText() string {
	var b strings.Builder
	b.WriteString(`      waker-10   (   10) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=10 next_prio=20
      waker-10   (   10) [000] .... 1.010000: sched_wakeup: comm=udk-irq-0 pid=73 prio=301 target_cpu=000
       udk-irq-0-73    (    2) [000] .... 1.020000: sched_switch: prev_comm=waker prev_pid=10 prev_prio=20 prev_state=D ==> next_comm=udk-irq-0 next_pid=73 next_prio=301
        app-20   (   20) [001] .... 1.030000: print: B|20|H:Texture upload(200 KiB)
        app-20   (   20) [001] .... 1.040000: print: E|20|I39
        app-20   (   20) [001] .... 1.050000: print: B|20|Choreographer#doFrame
        app-20   (   20) [001] .... 1.060000: print: E|20
        app-20   (   20) [001] .... 1.070000: print: S|20|animator|7
        app-20   (   20) [001] .... 1.080000: print: F|20|animator|7
        app-20   (   20) [001] .... 1.090000: print: C|20|frame_missed|1
        app-20   (   20) [001] .... 1.100000: clock_set_rate: cpu-cluster.0 state=1400000 cpu_id=0
        app-20   (   20) [001] .... 1.110000: clock_set_rate: cpu-cluster.0 state=1800000 cpu_id=1
        app-20   (   20) [001] .... 1.120000: cpu_frequency: state=1800000 cpu_id=1
        app-20   (   20) [001] .... 1.130000: cpu_frequency: state=2200000 cpu_id=2
        app-20   (   20) [001] .... 1.140000: cpu_idle: state=4294967295 cpu_id=0
        app-20   (   20) [001] .... 1.150000: mm_filemap_add_to_page_cache: dev 260:84 ino 0x60ffe page=0000000000000000 pfn=3062260 ofs=0
        app-20   (   20) [001] .... 1.160000: block_rq_issue: 8,0 R 4096 () 123 + 8 [app]
        app-20   (   20) [001] .... 1.170000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	// 35 distinct unknown event names — over the 30-name unknown-list cap.
	for i := 0; i < 35; i++ {
		fmt.Fprintf(&b, "        app-20   (   20) [001] .... 1.%03d000: zz_widget_evt_%02d: foo=%d\n", 200+i, i, i)
	}
	b.WriteString("total garbage line one\n")
	b.WriteString("=== another unparsable line ===\n")
	return b.String()
}

func writeCensusFixtures(t *testing.T) (scriptPath, tracePath string) {
	t.Helper()
	dir := t.TempDir()
	tracePath = filepath.Join(dir, "census.systrace")
	if err := os.WriteFile(tracePath, []byte(censusTraceText()), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath = filepath.Join(dir, "census.yaml")
	script := `
version: 1
description: "格式盲点普查"
steps:
  - {label: census, view: format_census}
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return scriptPath, tracePath
}

// Census golden: every face present with honest caps and disclosures.
func TestFormatCensusReportGolden(t *testing.T) {
	scriptPath, tracePath := writeCensusFixtures(t)
	var buf bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf)
	if err != nil || failed != 0 {
		t.Fatalf("Run: failed=%d err=%v\n%s", failed, err, buf.String())
	}
	report := buf.String()

	for _, want := range []string{
		// TDIAG B2 (§28.13): the census streams the whole file — the header
		// speaks the streaming caliber, no index event count.
		"格式普查(format_census): 普查范围=全文件 范围内事件=53 (流式解析事件=53",
		// ① full name spectrum + the blind-spot list (the load-bearing face)
		"① 事件名全谱(共",
		"①a 未识别事件名单(引擎分类=unknown;共 35 名,列前 30)——格式盲点清单:",
		"- name=zz_widget_evt_00 count=1 ⚠ 引擎未分类",
		// ② marker forms
		"② 标记形普查: trace_mark 总数=7",
		"- 原始字段形: B|=2 E|=2 S|=1 F|=1 C|=1",
		// ③ clock track census: one track, two cpu_ids, min/max value range
		"③ clock_set_rate 轨谱(共 1 轨,按计数列前 1):",
		"- track=cpu-cluster.0 count=2 distinct_cpu_id=2 cpu_ids={0,1} 值域=[1400000..1800000]",
		// ④ sched domain: microkernel RT and raw >159 buckets stay separate.
		"④ 调度域: sched_switch=",
		"- prio=140..159 (Harmony microkernel RT) 计数=",
		"- prio>159 计数=",
		"- prev_state token 集:",
		// ⑤ FS/IO faces
		"⑤ FS/IO: 事件名前缀谱(共",
		"- prefix=zz_ 引擎分类=unknown count=35",
		"- 文件 kv 覆盖率: 携文件字段事件=",
		"- block 事件: issue=1 remap=0 complete=1",
		// ⑥ power events with CPU sets
		"⑥ 电源事件: cpu_frequency=2 覆盖CPU={1,2} cpu_frequency_limits=0 覆盖CPU={} cpu_idle=1 覆盖CPU={0}",
		// ⑦ span census — annotated with the engine's exported semantic
		// classification (TDIAG B3): the fixture's Texture upload span
		// classifies; the plain frame span carries no annotation.
		"⑦ span 普查(B/S 起始记号;共",
		" semantic_class=texture_upload",
		// ⑧ line-level quality + typed unparsed samples (TDIAG B4)
		"行级质量: unparsed_lines=",
		"- 不可解析行样本 line=",
		"total garbage line one",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("census report missing %q\n----\n%s", want, report)
		}
	}

	// TDIAG B2/B3/B4 (§28.13): the 缺 API honest-disclosure list is fully
	// consumed — the section must be GONE (a reappearing entry means an
	// engine export regressed).
	if strings.Contains(report, "本构建不支持的普查面") {
		t.Errorf("the unsupported-faces disclosure must be empty after the §28.13 exports:\n%s", report)
	}
	// The plain frame span must NOT be annotated (no fabricated class, no
	// noisy near-miss on ordinary spans).
	if regexp.MustCompile(`Choreographer#doFrame count=\d+ (semantic_class|⚠)`).MatchString(report) {
		t.Errorf("plain span must carry no semantic annotation:\n%s", report)
	}

	// prio 301 must land in the >159 bucket with a nonzero count.
	re := regexp.MustCompile(`- prio>159 计数=(\d+)`)
	m := re.FindStringSubmatch(report)
	if m == nil || m[1] == "0" {
		t.Fatalf("prio>159 bucket must count the prio-301 rows, got %v", m)
	}

	// Unknown-list cap honesty (自设最强突变: silently dropping the unknown
	// list, or listing all 35 without the cap disclosure, reddens here).
	if got := strings.Count(report, "⚠ 引擎未分类"); got != 30 {
		t.Fatalf("unknown list must be capped at 30 entries, got %d", got)
	}

	// D-state prev_state token must appear in the token set face.
	if !regexp.MustCompile(`- prev_state token 集: .*D×\d+`).MatchString(report) {
		t.Errorf("prev_state token set must carry the D token\n%s", report)
	}
}

// Scope pin (自设最强突变: dropping the scope.admits filter reddens here): a
// census step with a window must census ONLY in-window events even though a
// small trace gets a full index — the unknown zz_ rows all sit at ts≥1.2 and
// must vanish from a 1.0..1.1 window census, while the whole-file line
// counters stay engine-authoritative.
func TestFormatCensusRespectsStepWindow(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "census.systrace")
	if err := os.WriteFile(tracePath, []byte(censusTraceText()), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "census_window.yaml")
	script := `
version: 1
steps:
  - {label: census_win, view: format_census, window: "1.0..1.1"}
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf); err != nil || failed != 0 {
		t.Fatalf("Run: failed=%d err=%v", failed, err)
	}
	report := buf.String()
	if !strings.Contains(report, "普查范围=window=1.000000..1.100000") {
		t.Fatalf("census must disclose its scope in fixed-point seconds form\n%s", report)
	}
	if strings.Contains(report, "zz_widget_evt_") {
		t.Fatalf("out-of-window unknown events leaked into a window-scoped census\n%s", report)
	}
	if !strings.Contains(report, "共 0 名,列前 0)——格式盲点清单") {
		t.Errorf("window-scoped unknown list must be empty\n%s", report)
	}
	if !strings.Contains(report, "(行级质量计数与不可解析样本为全文件流式口径;①-⑦ 统计面按普查范围过滤)") {
		t.Errorf("bounded census must disclose the two-scope split\n%s", report)
	}
	// TDIAG B4: a window-scoped census now carries unparsed samples too —
	// the former "窗口化索引下…样本略" honest skip is retired.
	if !strings.Contains(report, "- 不可解析行样本 line=") {
		t.Errorf("window-scoped census must still carry typed unparsed samples\n%s", report)
	}
	if strings.Contains(report, "样本略") {
		t.Errorf("the windowed sample-skip disclosure is retired\n%s", report)
	}
}

// EVOLUTION RECORD (TDIAG B4, §28.13, 2026-07-09; renamed per DIAG 复核
// P3-3 — the old name TestFormatCensusOverlongLineScanDisclosed claimed the
// retired scanner-abort disclosure while the body asserts sampling success) —
// supersedes the former TDIAG-P3-3 scanner-abort pin: the sample second read
// (and its bufio.ErrTooLong abort arm) is deleted; samples come typed from
// the parse site, so a >1MiB line is SAMPLED (rune-safe byte-capped) instead
// of aborting the sweep, and the render clamp marks the cut (parse cap 512 >
// render cap 480 — a cut sample can never silently read as whole).
func TestFormatCensusOverlongLineSampledWithTruncationMark(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("      waker-10   (   10) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=10 next_prio=20\n")
	b.WriteString("plain garbage before the monster line\n")
	b.WriteString(strings.Repeat("x", 2<<20)) // 2MiB single line, no newline until here
	b.WriteString("\n")
	tracePath := filepath.Join(dir, "overlong.systrace")
	if err := os.WriteFile(tracePath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "census.yaml")
	if err := os.WriteFile(scriptPath, []byte("version: 1\nsteps:\n  - {label: census, view: format_census}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf); err != nil || failed != 0 {
		t.Fatalf("Run: failed=%d err=%v", failed, err)
	}
	report := buf.String()
	// The monster line is sampled (typed face), byte-capped, and the render
	// marker discloses the cut.
	if !strings.Contains(report, "- 不可解析行样本 line=2 | plain garbage before the monster line") {
		t.Fatalf("the plain garbage line must be sampled whole\n%s", report)
	}
	if !regexp.MustCompile(`- 不可解析行样本 line=3 \| x+…\(截断\)`).MatchString(report) {
		t.Fatalf("the over-long line must be sampled with a truncation marker\n%s", report)
	}
	// No second read remains to abort — the old disclosure wording must be gone.
	if strings.Contains(report, "扫描中止") || strings.Contains(report, "样本可能不完整") {
		t.Fatalf("the second-read abort disclosure is retired\n%s", report)
	}
}

// TDIAG 留观① (§28.13 scope 语义分叉): a census step carrying BOTH a window
// and a line range states the census intersection semantics — the engine
// restatement ("time 界不参与") would be a lie on this step; engine steps
// keep the engine wording byte-identically.
func TestFormatCensusScopeSemanticsHeaderNote(t *testing.T) {
	scriptPath, tracePath := writeCensusFixtures(t)
	script := `
version: 1
steps:
  - {label: census_both, view: format_census, window: "1.0..1.1", line_start: 1, line_end: 20}
  - {label: engine_both, view: event_search, window: "1.0..1.1", line_start: 1, line_end: 20}
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf); err != nil || failed != 0 {
		t.Fatalf("Run: failed=%d err=%v\n%s", failed, err, buf.String())
	}
	report := buf.String()
	if !strings.Contains(report, "注: 本步为普查步,时间窗与行区间取交集过滤(普查步语义;引擎查询步则 line 界恒生效、time 界仅在无 line 界时生效)") {
		t.Fatalf("census step must state its intersection semantics\n%s", report)
	}
	if !strings.Contains(report, "注: 行区间生效时时间窗不参与过滤(引擎语义:line 界恒生效,time 界仅在无 line 界时生效)") {
		t.Fatalf("engine steps keep the engine-semantics note\n%s", report)
	}
}

// TDIAG B2 (§28.13): a format_census step streams the whole file and is NOT
// subject to the index event budget — the same budget that fails an
// index-backed step in the SAME script leaves the census whole (自设最强突变:
// routing the census arm back through buildStepIndex reddens here).
func TestFormatCensusNotSubjectToIndexBudget(t *testing.T) {
	scriptPath, tracePath := writeCensusFixtures(t)
	script := `
version: 1
steps:
  - {label: census, view: format_census}
  - {label: heavy, view: window_stats, pid: 20, window: "1.0..1.6"}
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	old := stepIndexMaxEvents
	stepIndexMaxEvents = 2 // trips every index-backed step
	defer func() { stepIndexMaxEvents = old }()
	var buf bytes.Buffer
	failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	report := buf.String()
	if failed != 1 {
		t.Fatalf("only the index-backed step may fail, failed=%d\n%s", failed, report)
	}
	if !strings.Contains(report, "- 步骤 1 label=census view=format_census 状态=成功") {
		t.Fatalf("census must survive the index budget (streaming path)\n%s", report)
	}
	if !strings.Contains(report, "格式普查(format_census): 普查范围=全文件 范围内事件=53") {
		t.Fatalf("census under budget pressure must still census EVERY event\n%s", report)
	}
	if !strings.Contains(report, "- 步骤 2 label=heavy view=window_stats 状态=失败:") {
		t.Fatalf("the IndexEventLimitError lane must stay with index-backed steps\n%s", report)
	}
}

// The unparsed-sample cap: more unparsed lines than the cap lists only the
// first censusUnparsedSamples with a cap disclosure.
func TestFormatCensusUnparsedSampleCap(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("      waker-10   (   10) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=10 next_prio=20\n")
	for i := 0; i < 9; i++ {
		fmt.Fprintf(&b, "garbage line number %d\n", i)
	}
	tracePath := filepath.Join(dir, "garbage.systrace")
	if err := os.WriteFile(tracePath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "census.yaml")
	if err := os.WriteFile(scriptPath, []byte("version: 1\nsteps:\n  - {label: census, view: format_census}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if failed, err := Run(nil, Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf); err != nil || failed != 0 {
		t.Fatalf("Run: failed=%d err=%v", failed, err)
	}
	report := buf.String()
	if got := strings.Count(report, "- 不可解析行样本 line="); got != censusUnparsedSamples {
		t.Fatalf("unparsed samples = %d, want cap %d\n%s", got, censusUnparsedSamples, report)
	}
	if !strings.Contains(report, fmt.Sprintf("样本帽 %d", censusUnparsedSamples)) {
		t.Errorf("missing sample-cap disclosure\n%s", report)
	}
}

func TestFormatCensusHarmonyMicrokernelPriorityBuckets(t *testing.T) {
	c := newFormatCensusAcc(censusScope{})
	for _, prio := range []int{139, 140, 142, 157, 159, 160, 301} {
		c.countPrio(prio)
	}
	if c.prioMicrokernelRT != 4 {
		t.Fatalf("140/142/157/159 must occupy the microkernel RT bucket, got %d", c.prioMicrokernelRT)
	}
	if c.prioOver159 != 2 {
		t.Fatalf("only 160/301 may occupy the raw >159 bucket, got %d", c.prioOver159)
	}
}
