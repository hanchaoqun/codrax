package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PIB-4 (ledger docs/design/pi_borrow_analysis_20260729.md §7.8):
// /export bundles the last turn + sticky attachments; /import restores
// them with manifest-verified integrity — the customer-revisit
// half-loop.

func TestExportImport_Roundtrip(t *testing.T) {
	// Exporter session: one completed turn + a sticky log + trace.
	rOut, store, _ := newApprovalREPL(t, "", &writeCapableRunner{})
	rOut.runtimeAnchor = t.TempDir()
	rOut.attachedLog = "# codrax-source: /tmp/panic.txt\npanic: nil deref at handler.go:42\n"
	rOut.attachedHitrace = "# codrax-source: /tmp/app.htrace\ntracing marker payload\n"
	rOut.attachedHitraceSource = "harmony_hitrace"
	rOut.recordTurn("为什么 handler 崩了?", "为什么 handler 崩了?", "根因: nil deref …", "pipeline")
	_ = store

	rOut.handleExportCmd()
	exportsDir := filepath.Join(rOut.runtimeAnchor, "exports")
	entries, err := os.ReadDir(exportsDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected exactly one bundle dir, got %v (%v)", entries, err)
	}
	bundle := filepath.Join(exportsDir, entries[0].Name())
	for _, want := range []string{"manifest.json", "request.txt", "answer.md", "log.txt", "trace.txt"} {
		if _, err := os.Stat(filepath.Join(bundle, want)); err != nil {
			t.Fatalf("bundle missing %s: %v", want, err)
		}
	}

	// Importer session: fresh REPL, nothing attached.
	rIn, _, _ := newApprovalREPL(t, "", &writeCapableRunner{})
	rIn.runtimeAnchor = t.TempDir()
	rIn.handleImportCmd("/import " + bundle)

	if !strings.Contains(rIn.attachedLog, "panic: nil deref") {
		t.Errorf("log attachment not restored: %q", rIn.attachedLog)
	}
	if !strings.Contains(rIn.attachedHitrace, "tracing marker") || rIn.attachedHitraceSource != "harmony_hitrace" {
		t.Errorf("trace attachment/source not restored: %q / %q", rIn.attachedHitrace, rIn.attachedHitraceSource)
	}
	recent := rIn.store.Recent()
	if len(recent) == 0 {
		t.Fatal("imported turn must land in memory")
	}
	last := recent[len(recent)-1]
	if !strings.Contains(last.Request, "handler 崩") || !strings.Contains(last.Response, "根因") {
		t.Errorf("imported turn content wrong: %+v", last)
	}
}

func TestImport_TamperedAttachmentFailsLoud(t *testing.T) {
	rOut, _, _ := newApprovalREPL(t, "", &writeCapableRunner{})
	rOut.runtimeAnchor = t.TempDir()
	rOut.attachedLog = "original payload\n"
	rOut.recordTurn("q", "q", "a", "pipeline")
	rOut.handleExportCmd()
	entries, _ := os.ReadDir(filepath.Join(rOut.runtimeAnchor, "exports"))
	bundle := filepath.Join(rOut.runtimeAnchor, "exports", entries[0].Name())
	if err := os.WriteFile(filepath.Join(bundle, "log.txt"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rIn, _, out := newApprovalREPL(t, "", &writeCapableRunner{})
	rIn.runtimeAnchor = t.TempDir()
	rIn.handleImportCmd("/import " + bundle)
	if rIn.attachedLog != "" {
		t.Errorf("tampered log must not attach; got %q", rIn.attachedLog)
	}
	if !strings.Contains(out.String(), "sha256 mismatch") {
		t.Errorf("tamper must fail loud; output: %q", out.String())
	}
}
