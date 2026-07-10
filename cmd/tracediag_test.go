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
	t.Cleanup(func() {
		flagTraceDiag, flagTraceDiagTrace, flagTraceDiagOut, flagTraceDiagFlavor = oldScript, oldTrace, oldOut, oldFlavor
		flagTraceDiagWindow = oldWindow
	})
	flagTraceDiag, flagTraceDiagTrace, flagTraceDiagOut, flagTraceDiagFlavor = script, trace, out, flavor
	flagTraceDiagWindow = ""
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
