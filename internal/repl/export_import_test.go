package repl

import (
	"encoding/json"
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

	rOut.handleExportCmd("/export")
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
	rOut.handleExportCmd("/export")
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

// TestExportTurnByID pins the by-id lane: a persisted historical turn
// exports request+answer with NO attachment files (sticky lanes belong
// to the live session), and an unknown id fails loud.
func TestExportTurnByID(t *testing.T) {
	r, _, out := newApprovalREPL(t, "", &writeCapableRunner{})
	r.runtimeAnchor = t.TempDir()
	r.attachedLog = "live-session log that must NOT ride a historical bundle\n"
	r.recordTurn("老问题", "老问题", "老答案", "pipeline")
	turnID := r.store.Recent()[len(r.store.Recent())-1].ID

	r.handleExportCmd("/export " + turnID)
	entries, err := os.ReadDir(filepath.Join(r.runtimeAnchor, "exports"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("bundle dir: %v %v", entries, err)
	}
	bundle := filepath.Join(r.runtimeAnchor, "exports", entries[0].Name())
	for _, want := range []string{"manifest.json", "request.txt", "answer.md"} {
		if _, err := os.Stat(filepath.Join(bundle, want)); err != nil {
			t.Fatalf("bundle missing %s", want)
		}
	}
	if _, err := os.Stat(filepath.Join(bundle, "log.txt")); err == nil {
		t.Fatal("by-id bundle must NOT include live-session attachments")
	}

	r.handleExportCmd("/export turn-does-not-exist")
	if !strings.Contains(out.String(), "turn-does-not-exist") {
		t.Fatalf("unknown id must fail loud naming the id; out=%q", out.String())
	}
}

// TestImport_VerifiesEveryPayloadBeforeAttaching pins REGFIX-D #17: the
// advertised "a tampered bundle attaches nothing" contract must hold
// even when the tampering is in the SECOND payload, and a manifest with
// its hashes stripped must not waive verification.
func TestImport_VerifiesEveryPayloadBeforeAttaching(t *testing.T) {
	build := func(t *testing.T) (*REPL, string) {
		t.Helper()
		rOut, _, _ := newApprovalREPL(t, "", &writeCapableRunner{})
		rOut.runtimeAnchor = t.TempDir()
		rOut.attachedLog = "good log payload\n"
		rOut.attachedHitrace = "good trace payload\n"
		rOut.attachedHitraceSource = "harmony_hitrace"
		rOut.recordTurn("q", "q", "a", "pipeline")
		rOut.handleExportCmd("/export")
		entries, err := os.ReadDir(filepath.Join(rOut.runtimeAnchor, "exports"))
		if err != nil || len(entries) != 1 {
			t.Fatalf("bundle: %v %v", entries, err)
		}
		return rOut, filepath.Join(rOut.runtimeAnchor, "exports", entries[0].Name())
	}

	// Tamper the SECOND payload: the log must NOT be attached either.
	_, bundle := build(t)
	if err := os.WriteFile(filepath.Join(bundle, "trace.txt"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rIn, _, out := newApprovalREPL(t, "", &writeCapableRunner{})
	rIn.runtimeAnchor = t.TempDir()
	rIn.handleImportCmd("/import " + bundle)
	if rIn.attachedLog != "" || rIn.attachedHitrace != "" {
		t.Fatalf("tampered trace must leave BOTH lanes untouched; log=%q trace=%q", rIn.attachedLog, rIn.attachedHitrace)
	}
	if !strings.Contains(out.String(), "sha256 mismatch") {
		t.Errorf("tamper must fail loud; out=%q", out.String())
	}

	// Strip the hashes: unverifiable payloads must be refused, not waived.
	_, bundle2 := build(t)
	manifestPath := filepath.Join(bundle2, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "log_sha256")
	delete(m, "trace_sha256")
	stripped, _ := json.Marshal(m)
	if err := os.WriteFile(manifestPath, stripped, 0o600); err != nil {
		t.Fatal(err)
	}
	rIn2, _, out2 := newApprovalREPL(t, "", &writeCapableRunner{})
	rIn2.runtimeAnchor = t.TempDir()
	rIn2.handleImportCmd("/import " + bundle2)
	if rIn2.attachedLog != "" || rIn2.attachedHitrace != "" {
		t.Fatal("hash-stripped manifest must not attach unverifiable payloads")
	}
	if !strings.Contains(out2.String(), "no sha256") {
		t.Errorf("missing-hash refusal must be explicit; out=%q", out2.String())
	}
}

// TestImport_MissingHashedPayloadFailsLoud pins SWEEPFIX S17 (the
// mirror tamper arm): a manifest that declares a payload hash while the
// payload file was deleted must refuse the WHOLE bundle loudly.
func TestImport_MissingHashedPayloadFailsLoud(t *testing.T) {
	rOut, _, _ := newApprovalREPL(t, "", &writeCapableRunner{})
	rOut.runtimeAnchor = t.TempDir()
	rOut.attachedLog = "log payload\n"
	rOut.attachedHitrace = "trace payload\n"
	rOut.attachedHitraceSource = "harmony_hitrace"
	rOut.recordTurn("q", "q", "a", "pipeline")
	rOut.handleExportCmd("/export")
	entries, err := os.ReadDir(filepath.Join(rOut.runtimeAnchor, "exports"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("bundle: %v %v", entries, err)
	}
	bundle := filepath.Join(rOut.runtimeAnchor, "exports", entries[0].Name())
	if err := os.Remove(filepath.Join(bundle, "log.txt")); err != nil {
		t.Fatal(err)
	}

	rIn, _, out := newApprovalREPL(t, "", &writeCapableRunner{})
	rIn.runtimeAnchor = t.TempDir()
	rIn.handleImportCmd("/import " + bundle)
	if rIn.attachedLog != "" || rIn.attachedHitrace != "" {
		t.Fatalf("deleted-but-hashed payload must attach NOTHING; log=%q trace=%q", rIn.attachedLog, rIn.attachedHitrace)
	}
	if !strings.Contains(out.String(), "missing, empty, or unreadable") {
		t.Errorf("refusal must be loud and specific; out=%q", out.String())
	}
}
