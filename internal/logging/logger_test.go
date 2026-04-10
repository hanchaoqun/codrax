package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]Level{
		"error":   LevelError,
		"ERROR":   LevelError,
		"warn":    LevelWarning,
		"warning": LevelWarning,
		"info":    LevelInfo,
		"":        LevelInfo,
		"debug":   LevelDebug,
		"junk":    LevelInfo,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	dir := t.TempDir()
	lg, err := NewFromFlags(dir, "warning", false)
	if err != nil {
		t.Fatalf("NewFromFlags: %v", err)
	}
	defer lg.Close()

	lg.Error("e1")
	lg.Warning("w1")
	lg.Info("i1")  // filtered
	lg.Debug("d1") // filtered

	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "codrax.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "ERROR e1") {
		t.Errorf("expected error record; got %q", out)
	}
	if !strings.Contains(out, "WARN w1") {
		t.Errorf("expected warning record; got %q", out)
	}
	if strings.Contains(out, "i1") || strings.Contains(out, "d1") {
		t.Errorf("info/debug should have been filtered; got %q", out)
	}
}

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	lg, err := NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatalf("NewFromFlags: %v", err)
	}
	defer lg.Close()

	// Write enough to trigger at least 2 rotations. Each call writes
	// roughly 200 bytes (timestamp + level + payload + newline).
	payload := strings.Repeat("x", 1024)
	// 4MB / 1KB ≈ 4096 records per file. Write ~10000 to force 2 rotations.
	for i := 0; i < 10000; i++ {
		lg.Info("%s", payload)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Expect codrax.log + at least .1 and .2.
	checkExists := func(name string) {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
	checkExists("codrax.log")
	checkExists("codrax.log.1")
	checkExists("codrax.log.2")

	// And there must be no more than 7 files in total (current + 6).
	entries, _ := os.ReadDir(dir)
	logFiles := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "codrax.log") {
			logFiles++
		}
	}
	if logFiles > 7 {
		t.Errorf("expected at most 7 log files, got %d", logFiles)
	}
}

func TestRestartAppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()

	// First session: write a small amount, well under 4MB.
	lg1, err := NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatalf("first NewFromFlags: %v", err)
	}
	for i := 0; i < 5; i++ {
		lg1.Info("first session line %d", i)
	}
	if err := lg1.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	first, err := os.ReadFile(filepath.Join(dir, "codrax.log"))
	if err != nil {
		t.Fatalf("read after first session: %v", err)
	}
	firstLen := len(first)
	if firstLen == 0 {
		t.Fatalf("first session wrote nothing")
	}

	// Second session: must continue appending to the same file, not
	// truncate it and not rotate (well under 4MB).
	lg2, err := NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatalf("second NewFromFlags: %v", err)
	}
	for i := 0; i < 5; i++ {
		lg2.Info("second session line %d", i)
	}
	if err := lg2.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	second, err := os.ReadFile(filepath.Join(dir, "codrax.log"))
	if err != nil {
		t.Fatalf("read after second session: %v", err)
	}
	if len(second) <= firstLen {
		t.Errorf("second session did not append: firstLen=%d, secondLen=%d", firstLen, len(second))
	}
	if !strings.HasPrefix(string(second), string(first)) {
		t.Errorf("second session truncated/overwrote earlier content")
	}
	// The first session's records must still be present.
	for i := 0; i < 5; i++ {
		if !strings.Contains(string(second), fmt.Sprintf("first session line %d", i)) {
			t.Errorf("missing first-session record %d after restart", i)
		}
	}
	// And the new session's records must be present too.
	for i := 0; i < 5; i++ {
		if !strings.Contains(string(second), fmt.Sprintf("second session line %d", i)) {
			t.Errorf("missing second-session record %d", i)
		}
	}
	// No rotated file should have been produced — we are nowhere near 4MB.
	if _, err := os.Stat(filepath.Join(dir, "codrax.log.1")); !os.IsNotExist(err) {
		t.Errorf("rotation triggered prematurely on restart")
	}
}

func TestInfoWriter(t *testing.T) {
	dir := t.TempDir()
	lg, err := NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatalf("NewFromFlags: %v", err)
	}
	w := lg.InfoWriter()
	if _, err := w.Write([]byte("hello via writer\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	lg.Close()

	data, _ := os.ReadFile(filepath.Join(dir, "codrax.log"))
	if !strings.Contains(string(data), "INFO hello via writer") {
		t.Errorf("InfoWriter did not produce expected record; got %q", string(data))
	}
}
