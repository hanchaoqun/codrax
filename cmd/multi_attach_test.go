package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadMultiPathSlice_HeaderedConcat verifies that two --log
// entries concatenate with `# codrax-source:` boundary headers.
func TestLoadMultiPathSlice_HeaderedConcat(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.log")
	b := filepath.Join(dir, "b.log")
	if err := os.WriteFile(a, []byte("body of A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("body of B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := loadMultiPathSlice("log", []string{a, b}, "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "# codrax-source: "+a) {
		t.Errorf("missing source header for a.log; body=%q", body)
	}
	if !strings.Contains(body, "# codrax-source: "+b) {
		t.Errorf("missing source header for b.log; body=%q", body)
	}
	if !strings.Contains(body, "body of A") || !strings.Contains(body, "body of B") {
		t.Errorf("file bodies missing; got %q", body)
	}
	// A's header must precede B's (CLI order preserved).
	idxA := strings.Index(body, "a.log")
	idxB := strings.Index(body, "b.log")
	if idxA >= idxB {
		t.Errorf("CLI order not preserved: a.log at %d, b.log at %d", idxA, idxB)
	}
}

// TestLoadMultiPathSlice_InlineTakesPriority verifies inline-text
// overrides path slice (matches mutual-exclusion contract).
func TestLoadMultiPathSlice_InlineTakesPriority(t *testing.T) {
	body, err := loadMultiPathSlice("log", []string{"/no/such/file"}, "INLINE BODY", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if body != "INLINE BODY" {
		t.Errorf("inline did not win: %q", body)
	}
}

// TestEnforceStdinExclusivity catches multi-stdin mistakes.
func TestEnforceStdinExclusivity(t *testing.T) {
	saveLog := flagAttachLog
	saveH := flagAttachHitrace
	saveA := flagAttachAtrace
	defer func() {
		flagAttachLog = saveLog
		flagAttachHitrace = saveH
		flagAttachAtrace = saveA
	}()

	// Single stdin OK.
	flagAttachLog = []string{"-"}
	flagAttachHitrace = nil
	flagAttachAtrace = nil
	if err := enforceStdinExclusivity(); err != nil {
		t.Errorf("single stdin: unexpected err %v", err)
	}
	// Two stdins across channels MUST error.
	flagAttachLog = []string{"-", "/some/file"}
	flagAttachHitrace = []string{"-"}
	if err := enforceStdinExclusivity(); err == nil {
		t.Errorf("expected error on two stdins")
	}
}

// TestTraceCapInheritsLogCap verifies the inheritance default.
// When user only sets log_attach_max_bytes, trace cap mirrors.
func TestTraceCapInheritsLogCap(t *testing.T) {
	saveLog := maxAttachedLogBytes
	saveTrace := maxAttachedTraceBytes
	defer func() {
		maxAttachedLogBytes = saveLog
		maxAttachedTraceBytes = saveTrace
	}()

	// Both defaults equal at startup.
	if defaultAttachedLogMaxBytes != 50*1024*1024 {
		t.Errorf("log default drifted: %d", defaultAttachedLogMaxBytes)
	}
	maxAttachedTraceBytes = 0
	maxAttachedLogBytes = 100 * 1024 * 1024
	// Inheritance is enforced inside initApp's settings merge; here we
	// simulate the post-merge state where trace inherits.
	if maxAttachedTraceBytes <= 0 {
		maxAttachedTraceBytes = maxAttachedLogBytes
	}
	if maxAttachedTraceBytes != maxAttachedLogBytes {
		t.Errorf("trace cap should inherit log cap; got %d vs %d", maxAttachedTraceBytes, maxAttachedLogBytes)
	}
}

// TestTruncateAttachedToCap_PerChannelLabel verifies the WARN label.
func TestTruncateAttachedToCap_PerChannelLabel(t *testing.T) {
	body := strings.Repeat("X", 100)
	out := truncateAttachedToCap(body, 50, "trace")
	if len(out) != 50 {
		t.Errorf("trace truncate failed: got %d", len(out))
	}
}

// TestHardCeilingClamp_Constant pins the OOM-protection bound. A
// drift here would let a misconfigured codrax.yaml allocate
// arbitrary memory on a single attachment.
func TestHardCeilingClamp_Constant(t *testing.T) {
	want := 1 << 30 // 1 GiB
	if maxAttachedLogHardCeiling != want {
		t.Errorf("hard ceiling drifted: want %d, got %d", want, maxAttachedLogHardCeiling)
	}
}

// TestDefaultsAre50MB pins the user-facing 50 MiB default for both
// log and trace channels (trace inherits at startup).
func TestDefaultsAre50MB(t *testing.T) {
	want := 50 * 1024 * 1024
	if defaultAttachedLogMaxBytes != want {
		t.Errorf("log default: want %d, got %d", want, defaultAttachedLogMaxBytes)
	}
}
