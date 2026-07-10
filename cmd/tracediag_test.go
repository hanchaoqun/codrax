package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tracediagCmdTestTrace = `      waker-10   (   10) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=10 next_prio=20
      waker-10   (   10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
        app-20   (   20) [001] .... 1.100000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
        app-20   (   20) [001] .... 1.200000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

const berlinV2CLITestTrace = `        app-20 (20) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      waker-10 (10) [000] .... 1.010000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
        app-20 (20) [001] .... 1.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
        app-20 (20) [001] .... 1.030000: tracing_mark_write: B|20|sync-work
        app-20 (20) [001] .... 1.031000: tracing_mark_write: E|20
        app-20 (20) [001] .... 1.032000: tracing_mark_write: C|20|counter|1|S|20|
        app-20 (20) [001] .... 1.033000: tracing_mark_write: S|20|async-work|7
      worker-30 (30) [002] .... 1.034000: tracing_mark_write: F|20|async-work|7
        app-20 (20) [001] .... 1.035000: tracing_mark_write: G|20|track|track-work|9
      worker-30 (30) [002] .... 1.036000: tracing_mark_write: H|20|track|9
        app-20 (20) [001] .... 1.037000: tracing_mark_write: N|20|track|track-point
        app-20 (20) [001] .... 1.038000: tracing_mark_write: I|20|process-point
          irq-2 (2) [003] .... 1.040000: irq_handler_entry: irq=7 name=test_irq
          irq-2 (2) [003] .... 1.041000: irq_handler_exit: irq=7 ret=handled
          irq-2 (2) [003] .... 1.042000: softirq_entry: vec=1 [action=TIMER]
          irq-2 (2) [003] .... 1.043000: softirq_exit: vec=1 [action=TIMER]
          irq-2 (2) [003] .... 1.044000: ipi_entry: (Rescheduling interrupts)
          irq-2 (2) [003] .... 1.045000: ipi_exit: (Rescheduling interrupts)
          io-40 (40) [003] .... 1.050000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
          irq-2 (2) [003] .... 1.051000: block_rq_complete: 8,0 R () 123 + 8 [0]
        app-20 (20) [001] .... 1.060000: vendor_magic: opaque_key=opaque_value
`

const tracediagCmdTestScript = `
version: 1
description: "cmd wiring test"
steps:
  - {label: rows, view: event_search, thread: "app-20", window: "1.0..1.3"}
`

func withTraceDiagFlags(t *testing.T, script, trace, out, flavor string) {
	t.Helper()
	oldScript, oldTrace, oldOut, oldFlavor := flagTraceDiag, flagTraceDiagTrace, flagTraceDiagOut, flagTraceDiagFlavor
	oldWindow := flagTraceDiagWindow
	oldTID := flagTraceDiagTID
	t.Cleanup(func() {
		flagTraceDiag, flagTraceDiagTrace, flagTraceDiagOut, flagTraceDiagFlavor = oldScript, oldTrace, oldOut, oldFlavor
		flagTraceDiagWindow = oldWindow
		flagTraceDiagTID = oldTID
	})
	flagTraceDiag, flagTraceDiagTrace, flagTraceDiagOut, flagTraceDiagFlavor = script, trace, out, flavor
	flagTraceDiagWindow = ""
	flagTraceDiagTID = ""
}

func writeTraceDiagCmdFixtures(t *testing.T) (scriptPath, tracePath, dir string) {
	t.Helper()
	dir = t.TempDir()
	tracePath = filepath.Join(dir, "t.systrace")
	if err := os.WriteFile(tracePath, []byte(tracediagCmdTestTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath = filepath.Join(dir, "s.yaml")
	if err := os.WriteFile(scriptPath, []byte(tracediagCmdTestScript), 0o644); err != nil {
		t.Fatal(err)
	}
	return scriptPath, tracePath, dir
}

func TestRunTraceDiagCLIWritesReportToStdout(t *testing.T) {
	scriptPath, tracePath, _ := writeTraceDiagCmdFixtures(t)
	withTraceDiagFlags(t, scriptPath, tracePath, "", "auto")
	var buf bytes.Buffer
	if err := runTraceDiagCLI(nil, &buf, nil); err != nil {
		t.Fatalf("runTraceDiagCLI: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"# codrax tracediag 采集报告",
		"[步骤 1/1] label=rows view=event_search",
		"结论: 全部 1 步骤成功。",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout report missing %q\n%s", want, out)
		}
	}
}

func TestRunTraceDiagCLIWritesReportToOutFile(t *testing.T) {
	scriptPath, tracePath, dir := writeTraceDiagCmdFixtures(t)
	outPath := filepath.Join(dir, "report.txt")
	withTraceDiagFlags(t, scriptPath, tracePath, outPath, "auto")
	var buf bytes.Buffer
	if err := runTraceDiagCLI(nil, &buf, nil); err != nil {
		t.Fatalf("runTraceDiagCLI: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("out file: %v", err)
	}
	if !strings.Contains(string(data), "# codrax tracediag 采集报告") {
		t.Fatalf("out file missing report header\n%s", string(data))
	}
	if !strings.Contains(buf.String(), "report written to") {
		t.Errorf("stdout must note the out path, got %q", buf.String())
	}
}

func TestRunTraceDiagCLIWindowOverrideAvoidsTemplateEditing(t *testing.T) {
	scriptPath, tracePath, _ := writeTraceDiagCmdFixtures(t)
	script := `
version: 1
defaults: {window: "0.0..0.1"}
steps:
  - {label: rows, view: event_search, thread: "app-20", event_types: [sched_switch], max_lines: 20}
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	withTraceDiagFlags(t, scriptPath, tracePath, "", "auto")
	flagTraceDiagWindow = "1.000..1.300"
	var buf bytes.Buffer
	if err := runTraceDiagCLI(nil, &buf, nil); err != nil {
		t.Fatalf("runTraceDiagCLI override: %v", err)
	}
	report := buf.String()
	for _, want := range []string{
		"window_override=1.000..1.300 source=cli_flag target=defaults.window",
		"window=1.000..1.300",
		"next_comm=app next_pid=20",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("override report missing %q\n%s", want, report)
		}
	}
}

func TestRunTraceDiagCLITIDOverrideUsesTypedBinding(t *testing.T) {
	scriptPath, tracePath, _ := writeTraceDiagCmdFixtures(t)
	script := `
version: 2
inputs: {window: required, tid: required}
limits: {max_generated_windows: 1, max_expanded_steps: 2, max_report_lines: 300}
steps:
  - {label: target_rows, view: event_search, pid_from: tid, event_types: [sched_switch], max_lines: 20}
  - {label: raw_rows, view: event_search, event_types: [sched_wakeup], max_lines: 20}
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	withTraceDiagFlags(t, scriptPath, tracePath, "", "auto")
	flagTraceDiagWindow = "1.000..1.300"
	flagTraceDiagTID = "00020"
	var buf bytes.Buffer
	if err := runTraceDiagCLI(nil, &buf, nil); err != nil {
		t.Fatalf("runTraceDiagCLI typed TID: %v", err)
	}
	report := buf.String()
	for _, want := range []string{
		"tid_override=20 source=cli_flag target=pid_from:tid",
		"label=target_rows view=event_search",
		"参数: pid=20 window=1.000..1.300",
		"label=raw_rows view=event_search",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("typed TID report missing %q\n%s", want, report)
		}
	}
}

func TestRunTraceDiagCLIBerlinV2SingleFileAtomicEndToEnd(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "berlin-mini.systrace")
	if err := os.WriteFile(tracePath, []byte(berlinV2CLITestTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "berlin_pairing_witness.txt")
	if err := os.WriteFile(outPath, []byte("STALE REPORT MUST BE REPLACED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join("..", "examples", "tracediag", "collect_berlin_pairing_witness.yaml")
	withTraceDiagFlags(t, scriptPath, tracePath, outPath, "auto")
	flagTraceDiagWindow = "0.990..1.100"
	flagTraceDiagTID = "00020"
	var stdout bytes.Buffer
	if err := runTraceDiagCLI(nil, &stdout, nil); err != nil {
		t.Fatalf("Berlin v2 CLI: %v\nstdout=%s", err, stdout.String())
	}
	if strings.Contains(stdout.String(), "# codrax tracediag") || !strings.Contains(stdout.String(), "report written to "+outPath) {
		t.Fatalf("--out must publish one file and only a path receipt on stdout: %q", stdout.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	report := string(data)
	if strings.Contains(report, "%!") {
		t.Fatalf("Berlin report contains a formatting failure\n%s", report)
	}
	for _, want := range []string{
		"# codrax tracediag 自动采集报告",
		"script=collect_berlin_pairing_witness.yaml version=2 discoveries=2 logical_steps=11 expanded_instances=12",
		"window_override=0.990..1.100 source=cli_flag target=defaults.window",
		"tid_override=20 source=cli_flag target=pid_from:tid",
		"validated_worst_report_lines=996",
		"[自动窗发现 1/2] label=io_pairing_windows strategy=pairing_integrity",
		"[自动窗发现 2/2] label=trace_mark_carry_windows strategy=trace_mark_carry",
		"pairing_status=complete_exact carry_class=inside_pair",
		"start_endpoint=action:",
		"family=block kind=schema_probe",
		"block_rq_complete: 8,0 R () 123 + 8",
		"结论: 全部 2 个 discovery 与 12 个执行实例成功",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("Berlin report missing %q\n%s", want, report)
		}
	}
	if strings.Contains(report, "STALE REPORT") || strings.Contains(report, "pattern=") {
		t.Fatalf("Berlin output retained stale/manual-pattern state\n%s", report)
	}
	target := traceDiagExecutionInstance(report, "target_window_stats")
	if !strings.Contains(traceDiagParamsLine(target), "pid=20 window=0.990..1.100") {
		t.Fatalf("target stats did not consume the typed TID/window: %s", traceDiagParamsLine(target))
	}
	wantActions := map[string]string{
		"raw_sync_marks":     "trace_mark_actions=[B,E]",
		"raw_counter_marks":  "trace_mark_actions=[C]",
		"raw_async_starts":   "trace_mark_actions=[S]",
		"raw_async_finishes": "trace_mark_actions=[F]",
		"raw_track_marks":    "trace_mark_actions=[G,H]",
		"raw_instant_marks":  "trace_mark_actions=[N,I]",
	}
	for label, actionEcho := range wantActions {
		section := traceDiagExecutionInstance(report, label)
		params := traceDiagParamsLine(section)
		if strings.Contains(params, "pid=") || strings.Contains(params, "thread=") || !strings.Contains(params, actionEcho) {
			t.Errorf("raw marker lane %s selector/action echo = %q", label, params)
		}
	}
	asyncSection := traceDiagExecutionInstance(report, "raw_async_starts")
	if !strings.Contains(asyncSection, "S|20|async-work|7") || strings.Contains(asyncSection, "C|20|counter|1|S|20|") {
		t.Fatalf("exact S action lane admitted a C payload substring impostor\n%s", asyncSection)
	}
	for _, label := range []string{"raw_interrupt_endpoints", "raw_io_pairing_rows", "raw_trace_mark_carry_endpoints", "raw_unknown_events"} {
		params := traceDiagParamsLine(traceDiagExecutionInstance(report, label))
		if strings.Contains(params, "pid=") || strings.Contains(params, "thread=") {
			t.Errorf("global/raw lane %s inherited target selector: %q", label, params)
		}
	}
	markEndpointParams := traceDiagParamsLine(traceDiagExecutionInstance(report, "raw_trace_mark_carry_endpoints"))
	if !strings.Contains(markEndpointParams, "trace_mark_actions=[B,E,S,F,G,H]") {
		t.Fatalf("generated marker endpoint lane lost its exact action filter: %q", markEndpointParams)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("atomic Berlin output left temp residue %s", entry.Name())
		}
	}
}

func traceDiagExecutionInstance(report, label string) string {
	needle := "label=" + label + " view="
	searchFrom := 0
	for {
		relative := strings.Index(report[searchFrom:], needle)
		if relative < 0 {
			return ""
		}
		start := searchFrom + relative
		lineStart := strings.LastIndex(report[:start], "\n") + 1
		lineEnd := strings.Index(report[start:], "\n")
		if lineEnd < 0 {
			lineEnd = len(report) - start
		}
		if strings.Contains(report[lineStart:start+lineEnd], "[执行实例 ") {
			sectionEnd := strings.Index(report[start+lineEnd:], "\n================================================================================\n[执行实例 ")
			if sectionEnd < 0 {
				sectionEnd = len(report) - (start + lineEnd)
			}
			return report[lineStart : start+lineEnd+sectionEnd]
		}
		searchFrom = start + len(needle)
	}
}

func traceDiagParamsLine(section string) string {
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "参数: ") {
			return line
		}
	}
	return ""
}

func TestRunTraceDiagCLITIDRejectedByV1AndPreservesExistingOut(t *testing.T) {
	scriptPath, tracePath, dir := writeTraceDiagCmdFixtures(t)
	outPath := filepath.Join(dir, "report.txt")
	previous := []byte("PREVIOUS VALUABLE REPORT\n")
	if err := os.WriteFile(outPath, previous, 0o644); err != nil {
		t.Fatal(err)
	}
	withTraceDiagFlags(t, scriptPath, tracePath, outPath, "auto")
	flagTraceDiagTID = "20"
	var buf bytes.Buffer
	err := runTraceDiagCLI(nil, &buf, nil)
	if err == nil || !strings.Contains(err.Error(), "version: 2") {
		t.Fatalf("v1 --trace-tid must fail loud, got %v", err)
	}
	got, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, previous) {
		t.Fatalf("failed TID binding changed previous report: got %q want %q", got, previous)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("failed TID binding left temp residue %s", entry.Name())
		}
	}
}

func TestRunTraceDiagCLIRequiresTraceFlag(t *testing.T) {
	scriptPath, _, _ := writeTraceDiagCmdFixtures(t)
	withTraceDiagFlags(t, scriptPath, "", "", "auto")
	var buf bytes.Buffer
	err := runTraceDiagCLI(nil, &buf, nil)
	if err == nil || !strings.Contains(err.Error(), "--trace") {
		t.Fatalf("missing --trace must fail loud, got %v", err)
	}
}

func TestRunTraceDiagCLIStrictFlavorEnum(t *testing.T) {
	scriptPath, tracePath, _ := writeTraceDiagCmdFixtures(t)
	withTraceDiagFlags(t, scriptPath, tracePath, "", "harmonyos")
	var buf bytes.Buffer
	err := runTraceDiagCLI(nil, &buf, nil)
	if err == nil || !strings.Contains(err.Error(), "harmony_hitrace") {
		t.Fatalf("unknown flavor must fail loud with the enum, got %v", err)
	}
}

// Exit-code pins (自设最强突变: swallowing the failed-step count and
// returning nil reddens here).
func TestTraceDiagExitErrorContract(t *testing.T) {
	if err := traceDiagExitError(0); err != nil {
		t.Fatalf("0 failed steps must exit clean, got %v", err)
	}
	err := traceDiagExitError(2)
	if err == nil || !strings.Contains(err.Error(), "2 step(s) failed") {
		t.Fatalf("failed steps must yield a nonzero exit naming the count, got %v", err)
	}
}

// An invalid script is also a nonzero exit (fail-loud before any step runs).
func TestRunTraceDiagCLIInvalidScriptYieldsError(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "t.systrace")
	if err := os.WriteFile(tracePath, []byte(tracediagCmdTestTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(scriptPath, []byte("version: 1\nsteps:\n  - {label: a, view: not_a_view}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withTraceDiagFlags(t, scriptPath, tracePath, "", "auto")
	var buf bytes.Buffer
	if err := runTraceDiagCLI(nil, &buf, nil); err == nil {
		t.Fatal("invalid script must exit nonzero")
	}
}

// P2-1 pin (对抗复核 2026-07-09, 自设最强突变: reverting to Create-before-
// validate reddens here): a FAILED run (bad script) with --out pointing at an
// existing report leaves that file byte-identical and no temp residue behind.
func TestRunTraceDiagCLIFailedRunPreservesExistingOutFile(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "t.systrace")
	if err := os.WriteFile(tracePath, []byte(tracediagCmdTestTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	badScript := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(badScript, []byte("version: 1\nsteps:\n  - {label: a, view: not_a_view}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "report.txt")
	previous := []byte("PREVIOUS VALUABLE REPORT\n")
	if err := os.WriteFile(outPath, previous, 0o644); err != nil {
		t.Fatal(err)
	}
	withTraceDiagFlags(t, badScript, tracePath, outPath, "auto")
	var buf bytes.Buffer
	if err := runTraceDiagCLI(nil, &buf, nil); err == nil {
		t.Fatal("bad script must fail the run")
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, previous) {
		t.Fatalf("existing --out file was clobbered by a failed run:\n%s", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp report residue left behind: %s", e.Name())
		}
	}
}

// A successful run still lands on --out through the temp+rename lane.
func TestRunTraceDiagCLIRenameLaneWritesReport(t *testing.T) {
	scriptPath, tracePath, dir := writeTraceDiagCmdFixtures(t)
	outPath := filepath.Join(dir, "fresh.txt")
	withTraceDiagFlags(t, scriptPath, tracePath, outPath, "auto")
	var buf bytes.Buffer
	if err := runTraceDiagCLI(nil, &buf, nil); err != nil {
		t.Fatalf("runTraceDiagCLI: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# codrax tracediag 采集报告") {
		t.Fatalf("renamed report missing header\n%s", data)
	}
}

// P2-3 pin (对抗复核 2026-07-09): pipeline-mode flags combined with
// --tracediag fail loud naming the conflicts instead of being silently
// ignored; the inert logging flags are disclosed, not conflicts.
func TestRunTraceDiagCLIConflictingFlagsFailLoud(t *testing.T) {
	scriptPath, tracePath, _ := writeTraceDiagCmdFixtures(t)
	withTraceDiagFlags(t, scriptPath, tracePath, "", "auto")
	oldRequest, oldMode := flagRequest, flagMode
	t.Cleanup(func() { flagRequest, flagMode = oldRequest, oldMode })
	flagRequest, flagMode = "analyse something", "write"
	var buf bytes.Buffer
	err := runTraceDiagCLI(nil, &buf, []string{"positional"})
	if err == nil {
		t.Fatal("conflicting flags must fail loud")
	}
	for _, want := range []string{"--request", "--mode", "positional request argument"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("conflict error must name %q, got %v", want, err)
		}
	}
}

func TestTraceDiagInertLogFlagsDetection(t *testing.T) {
	oldLevel, oldStdout := flagLogLevel, flagLogStdout
	t.Cleanup(func() { flagLogLevel, flagLogStdout = oldLevel, oldStdout })
	flagLogLevel, flagLogStdout = defaultLogLevel, false
	if got := traceDiagInertLogFlags(); len(got) != 0 {
		t.Fatalf("defaults must report no inert flags, got %v", got)
	}
	flagLogLevel, flagLogStdout = "info", true
	got := traceDiagInertLogFlags()
	if len(got) != 2 || got[0] != "--log-level" || got[1] != "--log-stdout" {
		t.Fatalf("inert flags = %v", got)
	}
}

// The pure-read pre-run contract: --tracediag must bypass initApp entirely
// (no providers.yaml lookup, no log/memory dirs). rootPreRun returning nil
// without touching the app struct is that contract's precise witness.
func TestRootPreRunSkipsInitAppForTraceDiag(t *testing.T) {
	withTraceDiagFlags(t, "some-script.yaml", "some.trace", "", "auto")
	if err := rootPreRun(rootCmd, nil); err != nil {
		t.Fatalf("rootPreRun must skip initApp and return nil, got %v", err)
	}
}

func TestRootPreRunRejectsTraceWindowWithoutTraceDiag(t *testing.T) {
	withTraceDiagFlags(t, "", "", "", "auto")
	flagTraceDiagWindow = "1.0..2.0"
	if err := rootPreRun(rootCmd, nil); err == nil || !strings.Contains(err.Error(), "--trace-window requires --tracediag") {
		t.Fatalf("orphan --trace-window must fail before initApp, got %v", err)
	}
}

func TestRootPreRunRejectsTraceTIDWithoutTraceDiag(t *testing.T) {
	withTraceDiagFlags(t, "", "", "", "auto")
	flagTraceDiagTID = "20"
	if err := rootPreRun(rootCmd, nil); err == nil || !strings.Contains(err.Error(), "--trace-tid requires --tracediag") {
		t.Fatalf("orphan --trace-tid must fail before initApp, got %v", err)
	}
}
