package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/outputdump"
)

func TestReportOutFlagsRegistered(t *testing.T) {
	for _, name := range []string{"report-md", "report-html"} {
		flag := rootCmd.PersistentFlags().Lookup(name)
		if flag == nil {
			t.Fatalf("--%s is not registered", name)
		}
		if flag.DefValue != "" {
			t.Fatalf("--%s default must be empty (feature off), got %q", name, flag.DefValue)
		}
		for _, want := range []string{".codrax/output", "output_dump_enabled=false", "overwritten"} {
			if !strings.Contains(flag.Usage, want) {
				t.Fatalf("--%s usage must mention %q; got: %s", name, want, flag.Usage)
			}
		}
	}
	if _, ok := compatLongFlagNames["report-md"]; !ok {
		t.Fatalf("report-md missing from compatLongFlagNames")
	}
	if _, ok := compatLongFlagNames["report-html"]; !ok {
		t.Fatalf("report-html missing from compatLongFlagNames")
	}
}

func TestResolveExplicitReportOutputsInactiveWithoutFlags(t *testing.T) {
	reg, err := resolveExplicitReportOutputs("some request", "", "", "/tmp/.codrax/output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.active {
		t.Fatalf("registration must stay inactive without flags: %+v", reg)
	}
}

func TestResolveExplicitReportOutputsRejectsREPL(t *testing.T) {
	_, err := resolveExplicitReportOutputs("", "/tmp/report.md", "", "/tmp/.codrax/output")
	if err == nil {
		t.Fatalf("REPL launch (empty request) with --report-md must fail loud")
	}
	if !strings.Contains(err.Error(), "REPL") {
		t.Fatalf("error must point at the REPL restriction: %v", err)
	}
}

func TestResolveExplicitReportOutputsEnabledDumpKeepsDefault(t *testing.T) {
	reg, err := resolveExplicitReportOutputs("q", "/tmp/a/report.md", "/tmp/b/report.html", "/tmp/.codrax/output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reg.active {
		t.Fatalf("expected active registration")
	}
	if reg.report.SuppressDefaultDir {
		t.Fatalf("enabled default dump must NOT be suppressed")
	}
	if reg.forceDumpDir != "" {
		t.Fatalf("enabled default dump needs no forced hook dir, got %q", reg.forceDumpDir)
	}
	if reg.report.MarkdownPath != "/tmp/a/report.md" || reg.report.HTMLPath != "/tmp/b/report.html" {
		t.Fatalf("absolute paths must pass through verbatim: %+v", reg.report)
	}
}

func TestResolveExplicitReportOutputsDisabledDumpForcesHook(t *testing.T) {
	reg, err := resolveExplicitReportOutputs("q", "/tmp/a/report.md", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reg.report.SuppressDefaultDir {
		t.Fatalf("disabled default dump must suppress the default dir")
	}
	if reg.forceDumpDir != "/tmp/a" {
		t.Fatalf("hook must be armed with a placeholder dir, got %q", reg.forceDumpDir)
	}
}

func TestResolveExplicitReportOutputsDisabledDumpHTMLOnly(t *testing.T) {
	reg, err := resolveExplicitReportOutputs("q", "", "/tmp/b/report.html", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.forceDumpDir != "/tmp/b" {
		t.Fatalf("html-only registration must still arm the hook, got %q", reg.forceDumpDir)
	}
}

func TestResolveExplicitReportOutputsRejectsSamePath(t *testing.T) {
	_, err := resolveExplicitReportOutputs("q", "/tmp/x/same.out", "/tmp/x/same.out", "/tmp/.codrax/output")
	if err == nil {
		t.Fatalf("identical --report-md/--report-html paths must be rejected")
	}
}

func TestResolveExplicitReportOutputsAbsolutizesRelativePaths(t *testing.T) {
	reg, err := resolveExplicitReportOutputs("q", "rel/report.md", "", "/tmp/.codrax/output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(reg.report.MarkdownPath) {
		t.Fatalf("relative path must be anchored to CWD, got %q", reg.report.MarkdownPath)
	}
	if !strings.HasSuffix(reg.report.MarkdownPath, filepath.Join("rel", "report.md")) {
		t.Fatalf("file name must be used verbatim, got %q", reg.report.MarkdownPath)
	}
}

// ---- OUT-1 修复轮 F2 (2026-07-12): --write-audit refuses the report flags ----

// The audit lane never produces a final-answer transcript, so the requested
// report file would silently never appear — same refuse-the-ambiguity
// contract as the tracediag conflict list. rootRun is exercised directly:
// the conflict check sits BEFORE runWriteAuditCLI, so no audit file is
// needed and no side effects fire.
func TestRootRunWriteAuditRejectsExplicitReportFlags(t *testing.T) {
	oldAudit, oldMD, oldHTML := flagWriteAudit, flagReportMD, flagReportHTML
	t.Cleanup(func() { flagWriteAudit, flagReportMD, flagReportHTML = oldAudit, oldMD, oldHTML })
	flagWriteAudit, flagReportMD, flagReportHTML = "some-final.json", "r.md", "r.html"
	err := rootRun(rootCmd, nil)
	if err == nil {
		t.Fatal("--report-md/--report-html under --write-audit must fail loud")
	}
	for _, want := range []string{"--write-audit", "--report-md", "--report-html", "cannot be combined"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("conflict error must name %q, got %v", want, err)
		}
	}
}

func TestExplicitReportFlagConflictsValueBased(t *testing.T) {
	oldMD, oldHTML := flagReportMD, flagReportHTML
	t.Cleanup(func() { flagReportMD, flagReportHTML = oldMD, oldHTML })
	flagReportMD, flagReportHTML = "", ""
	if got := explicitReportFlagConflicts(); len(got) != 0 {
		t.Fatalf("defaults must report no conflicts, got %v", got)
	}
	flagReportMD, flagReportHTML = "a.md", "b.html"
	got := explicitReportFlagConflicts()
	if len(got) != 2 || got[0] != "--report-md" || got[1] != "--report-html" {
		t.Fatalf("conflicts = %v", got)
	}
}

// ---- OUT-1 修复轮 F3 (2026-07-12): the arming wiring itself is pinned ----

type recordingOutputDumpArmer struct {
	dir   string
	max   int
	calls int
}

func (r *recordingOutputDumpArmer) SetOutputDump(dir string, max int) {
	r.dir, r.max = dir, max
	r.calls++
}

// Deleting either wiring call in armExplicitReportOutputs (the sink
// install or the orchestrator hook arming) turns this red — the
// disabled-dump lane's survival depends on exactly these two calls.
func TestArmExplicitReportOutputsWiresSinkAndHook(t *testing.T) {
	reg := explicitReportRegistration{
		active: true,
		report: outputdump.ExplicitReport{
			MarkdownPath:       "/tmp/x/report.md",
			SuppressDefaultDir: true,
		},
		forceDumpDir: "/tmp/x",
	}
	orch := &recordingOutputDumpArmer{}
	var sink *outputdump.ExplicitReport
	armExplicitReportOutputs(reg, orch, 7, func(r outputdump.ExplicitReport) { sink = &r })
	if sink == nil || sink.MarkdownPath != "/tmp/x/report.md" || !sink.SuppressDefaultDir {
		t.Fatalf("sink must be installed with the resolved registration, got %+v", sink)
	}
	if orch.calls != 1 || orch.dir != "/tmp/x" || orch.max != 7 {
		t.Fatalf("orchestrator dump hook must be armed with the placeholder dir + retention cap, got %+v", orch)
	}
}

func TestArmExplicitReportOutputsEnabledDumpSkipsHook(t *testing.T) {
	reg := explicitReportRegistration{
		active: true,
		report: outputdump.ExplicitReport{MarkdownPath: "/tmp/x/report.md"},
	}
	orch := &recordingOutputDumpArmer{}
	installed := false
	armExplicitReportOutputs(reg, orch, 7, func(outputdump.ExplicitReport) { installed = true })
	if !installed {
		t.Fatal("sink must be installed")
	}
	if orch.calls != 0 {
		t.Fatalf("enabled default dump must NOT re-arm the hook, got %+v", orch)
	}
}

func TestArmExplicitReportOutputsInactiveNoop(t *testing.T) {
	orch := &recordingOutputDumpArmer{}
	installed := false
	armExplicitReportOutputs(explicitReportRegistration{}, orch, 7, func(outputdump.ExplicitReport) { installed = true })
	if installed || orch.calls != 0 {
		t.Fatalf("inactive registration must be a no-op (installed=%t, orch=%+v)", installed, orch)
	}
}

// End-to-end wiring pin through configureExplicitReportOutputs: after
// configuring with live flags, the outputdump package must actually hold
// the sink — WriteResult with an empty Dir (dump disabled) still writes
// the explicit copy. Deleting the armExplicitReportOutputs call (or the
// sink install inside it) turns this red.
func TestConfigureExplicitReportOutputsInstallsSink(t *testing.T) {
	tmp := t.TempDir()
	mdPath := filepath.Join(tmp, "wired.md")
	oldMD, oldHTML, oldDir, oldMax := flagReportMD, flagReportHTML, app.outputDumpDir, app.outputDumpMax
	t.Cleanup(func() {
		flagReportMD, flagReportHTML, app.outputDumpDir, app.outputDumpMax = oldMD, oldHTML, oldDir, oldMax
		outputdump.SetExplicitReport(outputdump.ExplicitReport{})
	})
	flagReportMD, flagReportHTML = mdPath, ""
	app.outputDumpDir, app.outputDumpMax = "", 10 // output_dump_enabled=false shape

	if err := configureExplicitReportOutputs("some request"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	outputdump.WriteResult(outputdump.Args{Request: "q", Answer: "a"})
	if _, err := os.Stat(mdPath); err != nil {
		t.Fatalf("configured sink must write the explicit copy on WriteResult: %v", err)
	}
}
