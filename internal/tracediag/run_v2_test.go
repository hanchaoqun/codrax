package tracediag

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const runV2PairingTrace = `      waker-10   (   10) [000] .... 0.995000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-41 (41) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 1.002000: block_rq_complete: 8,0 R () 123 + 8 [0]
irq-2 (2) [003] .... 1.003000: block_rq_complete: 8,0 R () 123 + 8 [0]
 io-40 (40) [003] .... 1.010000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.012000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096
`

const runV2Script = `
version: 2
description: "typed 自动窗端到端"
defaults: {window: "0.990..1.020"}
limits: {max_generated_windows: 4, max_expanded_steps: 8, max_report_lines: 1000}
discoveries:
  - label: io_pairing
    strategy: pairing_integrity
    families: [block, storage]
    max_windows: 2
    max_window_ms: 50
    padding_ms: 5
    max_lines: 40
steps:
  - label: static_rows
    view: event_search
    event_types: [sched_wakeup]
    max_lines: 20
  - label: raw_io
    view: event_search
    event_types: [block_rq_issue, block_rq_complete, storage_latency]
    windows_from: {discovery: io_pairing}
    max_lines: 30
`

func writeRunV2Fixtures(t *testing.T, trace, script string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "pairing.systrace")
	scriptPath := filepath.Join(dir, "collect.yaml")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return scriptPath, tracePath
}

func TestRunV2DiscoveryFanoutReportEndToEnd(t *testing.T) {
	scriptPath, tracePath := writeRunV2Fixtures(t, runV2PairingTrace, runV2Script)
	run := func() (int, []byte) {
		var buf bytes.Buffer
		failed, err := Run(context.Background(), Options{
			ScriptPath: scriptPath,
			TracePath:  tracePath,
			Version:    "test-v2",
			BuildTime:  "2026-07-10",
			Now:        fixedNow,
		}, &buf)
		if err != nil {
			t.Fatalf("Run(v2): %v", err)
		}
		return failed, buf.Bytes()
	}
	failed, first := run()
	if failed != 0 {
		t.Fatalf("failed=%d\n%s", failed, first)
	}
	failed, second := run()
	if failed != 0 || !bytes.Equal(first, second) {
		t.Fatalf("v2 fixed-input reports are not byte-identical:\nfirst=%s\nsecond=%s", first, second)
	}
	report := string(first)
	if strings.Contains(report, "tid_override=") {
		t.Fatalf("report without a TID input fabricated TID provenance\n%s", report)
	}
	for _, want := range []string{
		"# codrax tracediag 自动采集报告",
		"source_lock=tracequery_source_universe source_lock_status=validated",
		"version=2 discoveries=1 logical_steps=2 expanded_instances=3",
		"coverage_scope=selected_candidate_windows_only",
		"[自动窗发现 1/1] label=io_pairing strategy=pairing_integrity",
		"candidate rank=1 family=block kind=ambiguous_closed",
		"family=storage kind=schema_probe",
		"[已解析执行计划]",
		"label=raw_io view=event_search fanout=1/2",
		"[执行实例 1/3] logical_step=1 label=static_rows",
		"[执行实例 2/3] logical_step=2 label=raw_io view=event_search instance=1/2",
		"FrameWindowAutoDerived=true",
		"block_rq_issue: 8,0 R 4096 () 123 + 8",
		"scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096",
		"[发现与执行状态摘要]",
		"结论: 全部 1 个 discovery 与 3 个执行实例成功；source_lock_status=validated。",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("v2 report missing %q\n%s", want, report)
		}
	}
	if strings.Index(report, "[已解析执行计划]") > strings.Index(report, "[执行实例 1/3]") {
		t.Fatalf("resolved plan must appear before evidence execution\n%s", report)
	}
}

func TestRunV2TIDBindingDoesNotFilterUnboundRawFanout(t *testing.T) {
	scriptText := strings.Replace(runV2Script,
		"description: \"typed 自动窗端到端\"",
		"description: \"typed TID 自动窗端到端\"\ninputs: {tid: required}", 1)
	scriptText = strings.Replace(scriptText,
		"  - label: static_rows\n    view: event_search",
		"  - label: static_rows\n    view: event_search\n    pid_from: tid", 1)
	scriptPath, tracePath := writeRunV2Fixtures(t, runV2PairingTrace, scriptText)
	var buf bytes.Buffer
	failed, err := Run(context.Background(), Options{
		ScriptPath:  scriptPath,
		TracePath:   tracePath,
		TIDOverride: "20",
		Now:         fixedNow,
	}, &buf)
	if err != nil || failed != 0 {
		t.Fatalf("Run(v2 typed TID): failed=%d err=%v\n%s", failed, err, buf.String())
	}
	report := buf.String()
	for _, want := range []string{
		"tid_override=20 source=cli_flag target=pid_from:tid",
		"[执行实例 1/3] logical_step=1 label=static_rows",
		"参数: pid=20 window=0.990..1.020 event_types=[sched_wakeup]",
		"block_rq_complete: 8,0 R () 123 + 8",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("typed TID fanout report missing %q\n%s", want, report)
		}
	}
	rawStart := strings.Index(report, "[执行实例 2/3] logical_step=2 label=raw_io")
	if rawStart < 0 {
		t.Fatalf("raw fanout instance missing\n%s", report)
	}
	rawTail := report[rawStart:]
	if paramsEnd := strings.Index(rawTail, "\n"); paramsEnd >= 0 {
		rawTail = rawTail[paramsEnd+1:]
	}
	if paramsEnd := strings.Index(rawTail, "\n"); paramsEnd >= 0 && strings.Contains(rawTail[:paramsEnd], "pid=") {
		t.Fatalf("unbound raw fanout inherited CLI TID: %s", rawTail[:paramsEnd])
	}
}

func TestRunV2EmptyDiscoveryBlocksOnlyDependentStep(t *testing.T) {
	trace := `      waker-10   (   10) [000] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
`
	scriptPath, tracePath := writeRunV2Fixtures(t, trace, runV2Script)
	var buf bytes.Buffer
	failed, err := Run(context.Background(), Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf)
	if err != nil {
		t.Fatalf("Run(v2 empty): %v", err)
	}
	if failed != 1 {
		t.Fatalf("failed=%d want dependency failure only\n%s", failed, buf.String())
	}
	report := buf.String()
	for _, want := range []string{
		"generated_windows=0",
		"dependency_empty discovery=io_pairing",
		"未回退到父窗口",
		"label=static_rows",
		"sched_wakeup: comm=app pid=20",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("empty-discovery report missing %q\n%s", want, report)
		}
	}
	if strings.Contains(report, "label=raw_io view=event_search instance=1/2") {
		t.Fatalf("empty dependency fabricated generated instances\n%s", report)
	}
}

func TestRunV2SourceMutationPublishesNoPartialReport(t *testing.T) {
	scriptPath, tracePath := writeRunV2Fixtures(t, runV2PairingTrace, runV2Script)
	oldHook := traceDiagV2AfterDiscoveriesHook
	traceDiagV2AfterDiscoveriesHook = func() {
		if err := os.WriteFile(tracePath, []byte("replacement generation\n"), 0o644); err != nil {
			t.Fatalf("mutate trace: %v", err)
		}
	}
	defer func() { traceDiagV2AfterDiscoveriesHook = oldHook }()
	var buf bytes.Buffer
	failed, err := Run(context.Background(), Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf)
	if err == nil || !strings.Contains(err.Error(), "source lock after discovery") {
		t.Fatalf("source mutation was not fatal: failed=%d err=%v", failed, err)
	}
	if buf.Len() != 0 {
		t.Fatalf("mixed-version partial report was published:\n%s", buf.String())
	}
}

func TestRunV2GeneratedWindowQueryCarriesTypedOrigin(t *testing.T) {
	step := Step{
		View:         "frame_root_cause_bundle",
		windowStart:  1,
		windowEnd:    1.01,
		windowSet:    true,
		windowOrigin: &WindowProvenance{DiscoveryLabel: "d"},
	}
	q := stepQuery(&step, "")
	if !q.FrameWindowAutoDerived || !q.TimeStartSet || !q.TimeEndSet || q.TimeStart != 1 || q.TimeEnd != 1.01 {
		t.Fatalf("generated window lost typed origin/bounds: %+v", q)
	}
}

func TestRunV2OptionCapBelowValidatedWorstFailsBeforeOutput(t *testing.T) {
	scriptPath, tracePath := writeRunV2Fixtures(t, runV2PairingTrace, runV2Script)
	var buf bytes.Buffer
	_, err := Run(context.Background(), Options{ScriptPath: scriptPath, TracePath: tracePath, TotalMaxLines: 100, Now: fixedNow}, &buf)
	if err == nil || !strings.Contains(err.Error(), "below the validated worst-case") {
		t.Fatalf("undersized runtime cap must fail preflight: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("preflight failure wrote a partial report: %q", buf.String())
	}
}

func TestRunV2GeneratedEventCompactionFailsLoud(t *testing.T) {
	script := `
version: 2
defaults: {window: "0.990..1.020"}
limits: {max_generated_windows: 2, max_expanded_steps: 2, max_report_lines: 500}
discoveries:
  - {label: io_pairing, strategy: pairing_integrity, families: [block], max_windows: 1, max_lines: 20}
steps:
  - label: raw_io
    view: event_search
    event_types: [block_rq_issue, block_rq_complete]
    windows_from: {discovery: io_pairing}
    max_lines: 5
`
	scriptPath, tracePath := writeRunV2Fixtures(t, runV2PairingTrace, script)
	var buf bytes.Buffer
	failed, err := Run(context.Background(), Options{ScriptPath: scriptPath, TracePath: tracePath, Now: fixedNow}, &buf)
	if err != nil {
		t.Fatalf("Run(v2 compaction): %v", err)
	}
	if failed != 1 {
		t.Fatalf("compacted generated witness must fail, got failed=%d\n%s", failed, buf.String())
	}
	report := buf.String()
	for _, want := range []string{
		"generated_window_compacted",
		"matched=4 emitted=2",
		"不得把本实例当作 N/N 完整 witness",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("compaction report missing %q\n%s", want, report)
		}
	}
	if got := strings.Count(report, "type=block_rq_"); got != 2 {
		t.Fatalf("every returned raw row must remain visible (want 2), got %d\n%s", got, report)
	}
}
