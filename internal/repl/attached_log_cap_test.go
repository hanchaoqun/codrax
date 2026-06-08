package repl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestREPL_AttachedLogCap_Default confirms that a zero-value
// Config.AttachedLogMaxBytes seeds the REPL with the 512 MiB default —
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

func TestREPL_HandleLogLoad_RejectsBinaryAttachment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.log")
	if err := os.WriteFile(path, []byte{'l', 'o', 'g', 0, 1, 2}, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out := &bytes.Buffer{}
	r := New(Config{In: strings.NewReader(""), Out: out})

	r.handleLogLoad(path)

	if r.attachedLog != "" {
		t.Fatalf("binary log must not become sticky attachment: %q", r.attachedLog)
	}
	got := out.String()
	for _, want := range []string{"附加 log", "不是可解析文本", "UTF-8"} {
		if !strings.Contains(got, want) {
			t.Fatalf("binary log warning missing %q:\n%s", want, got)
		}
	}
}

func TestREPL_HandleLogAppend_RejectsBinaryWithoutMutatingExistingAttachment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.log")
	if err := os.WriteFile(path, []byte{'b', 'a', 'd', 0, 3}, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out := &bytes.Buffer{}
	r := New(Config{In: strings.NewReader(""), Out: out})
	r.attachedLog = "existing\n"

	r.handleLogAppend(path)

	if r.attachedLog != "existing\n" {
		t.Fatalf("binary append should not mutate existing attachment: %q", r.attachedLog)
	}
	if !strings.Contains(out.String(), "不是可解析文本") {
		t.Fatalf("append rejection should be visible:\n%s", out.String())
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

func TestREPL_HandleHitraceLoad_RejectsBinaryWithConvertHint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.htrace")
	if err := os.WriteFile(path, []byte{'H', 'T', 0, 1, 2, 3}, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out := &bytes.Buffer{}
	r := New(Config{In: strings.NewReader(""), Out: out})

	r.handleHitraceCmd("/htrace " + path)

	if r.attachedHitrace != "" {
		t.Fatalf("binary trace must not become sticky attachment: %q", r.attachedHitrace)
	}
	got := out.String()
	for _, want := range []string{"附加 trace", "/htrace convert", "codrax trace convert"} {
		if !strings.Contains(got, want) {
			t.Fatalf("binary trace guidance missing %q:\n%s", want, got)
		}
	}
}

func TestREPL_HandleAtraceAlias_RejectsBinaryWithConvertHint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.atrace")
	if err := os.WriteFile(path, []byte{'A', 'T', 0, 1, 2, 3}, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out := &bytes.Buffer{}
	r := New(Config{In: strings.NewReader(""), Out: out})
	line := "/atrace " + path
	r.pendingHitraceSource = replTraceSourceHint(line)
	r.handleSlash(types.NormalizeREPLCommandAlias(line))

	if r.attachedHitrace != "" {
		t.Fatalf("binary atrace must not become sticky attachment: %q", r.attachedHitrace)
	}
	if r.pendingHitraceSource != "android_atrace" {
		t.Fatalf("atrace alias should set android source hint, got %q", r.pendingHitraceSource)
	}
	got := out.String()
	for _, want := range []string{"附加 trace", "/htrace convert", "codrax trace convert"} {
		if !strings.Contains(got, want) {
			t.Fatalf("binary atrace guidance missing %q:\n%s", want, got)
		}
	}
}

func TestREPL_HandleHitraceAppend_RejectsBinaryWithoutMutatingExistingAttachment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.htrace")
	if err := os.WriteFile(path, []byte{'H', 0, 4, 5}, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out := &bytes.Buffer{}
	r := New(Config{In: strings.NewReader(""), Out: out})
	r.attachedHitrace = "existing trace\n"

	r.handleHitraceAppend(path)

	if r.attachedHitrace != "existing trace\n" {
		t.Fatalf("binary trace append should not mutate existing attachment: %q", r.attachedHitrace)
	}
	if !strings.Contains(out.String(), "/htrace convert") {
		t.Fatalf("trace append rejection should include convert guidance:\n%s", out.String())
	}
}
