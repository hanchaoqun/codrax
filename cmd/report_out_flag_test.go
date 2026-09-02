package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/orchestrator"
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
	rootFlag := rootCmd.PersistentFlags().Lookup("root-causes-out")
	if rootFlag == nil || rootFlag.DefValue != "" {
		t.Fatalf("--root-causes-out must be registered and default off: %+v", rootFlag)
	}
	for _, want := range []string{"guaranteed-delivery", "unavailable", "output_dump_enabled=false", "command fail"} {
		if !strings.Contains(rootFlag.Usage, want) {
			t.Fatalf("--root-causes-out usage must mention %q; got: %s", want, rootFlag.Usage)
		}
	}
	if _, ok := compatLongFlagNames["root-causes-out"]; !ok {
		t.Fatalf("root-causes-out missing from compatLongFlagNames")
	}
}

func TestResolveExplicitReportOutputsInactiveWithoutFlags(t *testing.T) {
	reg, err := resolveExplicitReportOutputs("some request", "", "", "", "/tmp/.codrax/output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.active {
		t.Fatalf("registration must stay inactive without flags: %+v", reg)
	}
}

func TestResolveExplicitReportOutputsRejectsREPL(t *testing.T) {
	_, err := resolveExplicitReportOutputs("", "/tmp/report.md", "", "", "/tmp/.codrax/output")
	if err == nil {
		t.Fatalf("REPL launch (empty request) with --report-md must fail loud")
	}
	if !strings.Contains(err.Error(), "REPL") {
		t.Fatalf("error must point at the REPL restriction: %v", err)
	}
}

func TestResolveExplicitReportOutputsEnabledDumpKeepsDefault(t *testing.T) {
	reg, err := resolveExplicitReportOutputs("q", "/tmp/a/report.md", "/tmp/b/report.html", "", "/tmp/.codrax/output")
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
	reg, err := resolveExplicitReportOutputs("q", "/tmp/a/report.md", "", "", "")
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
	reg, err := resolveExplicitReportOutputs("q", "", "/tmp/b/report.html", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.forceDumpDir != "/tmp/b" {
		t.Fatalf("html-only registration must still arm the hook, got %q", reg.forceDumpDir)
	}
}

func TestResolveExplicitReportOutputsDisabledDumpRootCausesOnly(t *testing.T) {
	reg, err := resolveExplicitReportOutputs("q", "", "", "/tmp/c/root-causes.json", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.forceDumpDir != "/tmp/c" || reg.report.RootCauseJSONPath != "/tmp/c/root-causes.json" || !reg.report.SuppressDefaultDir {
		t.Fatalf("root-causes-only registration must arm the hook and suppress default dump: %+v", reg)
	}
}

func TestResolveExplicitReportOutputsRejectsSamePath(t *testing.T) {
	_, err := resolveExplicitReportOutputs("q", "/tmp/x/same.out", "/tmp/x/same.out", "", "/tmp/.codrax/output")
	if err == nil {
		t.Fatalf("identical --report-md/--report-html paths must be rejected")
	}
}

func TestResolveExplicitReportOutputsRejectsRootCausePathCollision(t *testing.T) {
	_, err := resolveExplicitReportOutputs("q", "/tmp/x/same.out", "", "/tmp/x/same.out", "/tmp/.codrax/output")
	if err == nil || !strings.Contains(err.Error(), "--root-causes-out") {
		t.Fatalf("root-cause output path collision must fail loud: %v", err)
	}
}

func TestResolveExplicitReportOutputsAbsolutizesRelativePaths(t *testing.T) {
	reg, err := resolveExplicitReportOutputs("q", "rel/report.md", "", "", "/tmp/.codrax/output")
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
	oldAudit, oldMD, oldHTML, oldRoot := flagWriteAudit, flagReportMD, flagReportHTML, flagRootCausesOut
	t.Cleanup(func() {
		flagWriteAudit, flagReportMD, flagReportHTML, flagRootCausesOut = oldAudit, oldMD, oldHTML, oldRoot
	})
	flagWriteAudit, flagReportMD, flagReportHTML, flagRootCausesOut = "some-final.json", "r.md", "r.html", "roots.json"
	err := rootRun(rootCmd, nil)
	if err == nil {
		t.Fatal("--report-md/--report-html under --write-audit must fail loud")
	}
	for _, want := range []string{"--write-audit", "--report-md", "--report-html", "--root-causes-out", "cannot be combined"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("conflict error must name %q, got %v", want, err)
		}
	}
}

func TestExplicitReportFlagConflictsValueBased(t *testing.T) {
	oldMD, oldHTML, oldRoot := flagReportMD, flagReportHTML, flagRootCausesOut
	t.Cleanup(func() { flagReportMD, flagReportHTML, flagRootCausesOut = oldMD, oldHTML, oldRoot })
	flagReportMD, flagReportHTML, flagRootCausesOut = "", "", ""
	if got := explicitReportFlagConflicts(); len(got) != 0 {
		t.Fatalf("defaults must report no conflicts, got %v", got)
	}
	flagReportMD, flagReportHTML, flagRootCausesOut = "a.md", "b.html", "roots.json"
	got := explicitReportFlagConflicts()
	if len(got) != 3 || got[0] != "--report-md" || got[1] != "--report-html" || got[2] != "--root-causes-out" {
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
	oldMD, oldHTML, oldRoot, oldDir, oldMax := flagReportMD, flagReportHTML, flagRootCausesOut, app.outputDumpDir, app.outputDumpMax
	t.Cleanup(func() {
		flagReportMD, flagReportHTML, flagRootCausesOut, app.outputDumpDir, app.outputDumpMax = oldMD, oldHTML, oldRoot, oldDir, oldMax
		outputdump.SetExplicitReport(outputdump.ExplicitReport{})
	})
	flagReportMD, flagReportHTML, flagRootCausesOut = mdPath, "", ""
	app.outputDumpDir, app.outputDumpMax = "", 10 // output_dump_enabled=false shape

	if err := configureExplicitReportOutputs("some request"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	outputdump.WriteResult(outputdump.Args{Request: "q", Answer: "a"})
	if _, err := os.Stat(mdPath); err != nil {
		t.Fatalf("configured sink must write the explicit copy on WriteResult: %v", err)
	}
}

func TestConfigureExplicitRootCauseOutputWritesTypedUnavailableArtifact(t *testing.T) {
	tmp := t.TempDir()
	rootPath := filepath.Join(tmp, "api", "root-causes.json")
	oldMD, oldHTML, oldRoot, oldDir, oldMax := flagReportMD, flagReportHTML, flagRootCausesOut, app.outputDumpDir, app.outputDumpMax
	t.Cleanup(func() {
		flagReportMD, flagReportHTML, flagRootCausesOut, app.outputDumpDir, app.outputDumpMax = oldMD, oldHTML, oldRoot, oldDir, oldMax
		outputdump.SetExplicitReport(outputdump.ExplicitReport{})
	})
	flagReportMD, flagReportHTML, flagRootCausesOut = "", "", rootPath
	app.outputDumpDir, app.outputDumpMax = "", 10

	if err := configureExplicitReportOutputs("trace request"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	result := outputdump.WriteResult(outputdump.Args{
		Request: "trace request", Answer: "answer",
		RootCauseUnavailableReason: "valid_model_root_cause_selection_unavailable",
	})
	if result.RootCauseJSONPath != rootPath {
		t.Fatalf("guaranteed path not returned: %+v", result)
	}
	data, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatalf("read guaranteed artifact: %v", err)
	}
	var artifact outputdump.ExplicitRootCauseArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode guaranteed artifact: %v", err)
	}
	if artifact.Status != outputdump.ExplicitRootCauseStatusUnavailable || artifact.TraceRootCauses != nil {
		t.Fatalf("missing model selection must stay unavailable, not become an empty conclusion: %+v", artifact)
	}
}

func TestPrintExplicitReportStatusReturnsRootCauseWriteFailure(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(blocker, "root-causes.json")
	oldMD, oldHTML, oldRoot := flagReportMD, flagReportHTML, flagRootCausesOut
	t.Cleanup(func() {
		flagReportMD, flagReportHTML, flagRootCausesOut = oldMD, oldHTML, oldRoot
		outputdump.SetExplicitReport(outputdump.ExplicitReport{})
	})
	flagReportMD, flagReportHTML, flagRootCausesOut = "", "", badPath
	outputdump.SetExplicitReport(outputdump.ExplicitReport{RootCauseJSONPath: badPath, SuppressDefaultDir: true})

	err := printExplicitReportStatus()
	if err == nil || !strings.Contains(err.Error(), "--root-causes-out write failed") {
		t.Fatalf("explicit root-cause write failure must affect command status: %v", err)
	}
}

func TestPrintReportStatusReturnsDefaultRootCauseWriteFailureWithoutFlags(t *testing.T) {
	oldOrch, oldMD, oldHTML, oldRoot := app.orch, flagReportMD, flagReportHTML, flagRootCausesOut
	t.Cleanup(func() { app.orch, flagReportMD, flagReportHTML, flagRootCausesOut = oldOrch, oldMD, oldHTML, oldRoot })
	flagReportMD, flagReportHTML, flagRootCausesOut = "", "", ""
	app.orch = &orchestrator.Orchestrator{}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.orch.SetOutputDump(blocker, 10)
	app.orch.SetAttachedHitrace("binary\x00trace")
	_, _ = app.orch.Run("trace", t.TempDir(), "main")
	if err := printExplicitReportStatus(); err == nil || !strings.Contains(err.Error(), "root-cause output directory") {
		t.Fatalf("post-answer CLI boundary must return default delivery failure: %v", err)
	}
}
