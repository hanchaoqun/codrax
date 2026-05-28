package repl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestREPL_AttachedLogCap_Default confirms that a zero-value
// Config.AttachedLogMaxBytes seeds the REPL with the 50 MiB default —
// protects callers (tests, legacy wiring) that never set the field.
func TestREPL_AttachedLogCap_Default(t *testing.T) {
	in := strings.NewReader("")
	r := New(Config{In: in, Out: &bytes.Buffer{}})
	if r.attachedLogMaxBytes != DefaultAttachedLogMaxBytes {
		t.Fatalf("default cap: got %d, want %d", r.attachedLogMaxBytes, DefaultAttachedLogMaxBytes)
	}
}

// TestREPL_AttachedLogCap_HonorsConfig sets a smaller cap and
// verifies New propagates it onto the REPL instance.
func TestREPL_AttachedLogCap_HonorsConfig(t *testing.T) {
	in := strings.NewReader("")
	const customCap = 4096
	r := New(Config{In: in, Out: &bytes.Buffer{}, AttachedLogMaxBytes: customCap})
	if r.attachedLogMaxBytes != customCap {
		t.Fatalf("custom cap: got %d, want %d", r.attachedLogMaxBytes, customCap)
	}
}

// TestREPL_AttachedLogCap_NonPositiveFallsBack guards against a
// misconfigured 0 bricking every /log surface. New clamps ≤ 0 up
// to the default rather than storing it verbatim.
func TestREPL_AttachedLogCap_NonPositiveFallsBack(t *testing.T) {
	in := strings.NewReader("")
	// Explicit zero — mirrors an operator who typed `log_attach_max_bytes: 0`.
	r := New(Config{In: in, Out: &bytes.Buffer{}, AttachedLogMaxBytes: 0})
	if r.attachedLogMaxBytes != DefaultAttachedLogMaxBytes {
		t.Fatalf("zero cap should fall back: got %d, want %d",
			r.attachedLogMaxBytes, DefaultAttachedLogMaxBytes)
	}

	r2 := New(Config{In: in, Out: &bytes.Buffer{}, AttachedLogMaxBytes: -1})
	if r2.attachedLogMaxBytes != DefaultAttachedLogMaxBytes {
		t.Fatalf("negative cap should fall back: got %d, want %d",
			r2.attachedLogMaxBytes, DefaultAttachedLogMaxBytes)
	}
}

// TestREPL_HandleLogLoad_HonorsCap is the behavioural test: write a
// 10 KB file, seed the REPL with a 4 KB cap, invoke /log <path>, and
// assert the attachedLog is truncated to exactly 4 KB.
func TestREPL_HandleLogLoad_HonorsCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.log")
	payload := strings.Repeat("x", 10*1024)
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	in := strings.NewReader("")
	out := &bytes.Buffer{}
	const customCap = 4096
	r := New(Config{In: in, Out: out, AttachedLogMaxBytes: customCap})

	r.handleLogLoad(path)

	if len(r.attachedLog) != customCap {
		t.Fatalf("attached log: len=%d, want %d", len(r.attachedLog), customCap)
	}
	if !strings.Contains(out.String(), "log truncated") {
		t.Errorf("expected truncation warning in output, got: %q", out.String())
	}
}

func TestREPL_HandleHitraceLoad_HonorsCapWithSourceHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.systrace")
	payload := strings.Repeat("t", 10*1024)
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out := &bytes.Buffer{}
	const customCap = 4096
	r := New(Config{In: strings.NewReader(""), Out: out, AttachedTraceMaxBytes: customCap})

	r.handleHitraceCmd("/htrace " + path)

	if len(r.attachedHitrace) != customCap {
		t.Fatalf("attached hitrace: len=%d, want %d", len(r.attachedHitrace), customCap)
	}
	if !strings.HasPrefix(r.attachedHitrace, "# codrax-source: "+path+"\n") {
		t.Fatalf("trace header missing: %q", r.attachedHitrace[:min(len(r.attachedHitrace), 80)])
	}
	if !strings.Contains(out.String(), "hitrace truncated") {
		t.Errorf("expected truncation warning in output, got: %q", out.String())
	}
}
