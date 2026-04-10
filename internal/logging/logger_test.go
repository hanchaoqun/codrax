package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readAllLogs returns the concatenated contents of every codrax-*.log
// file under dir, in chronological order. Tests use this instead of
// reading a single fixed filename because rotation now produces a new
// timestamped file each time.
func readAllLogs(t *testing.T, dir string) string {
	t.Helper()
	files, err := listLogFiles(dir)
	if err != nil {
		t.Fatalf("list log files: %v", err)
	}
	var b strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		b.Write(data)
	}
	return b.String()
}

func countLogFiles(t *testing.T, dir string) int {
	t.Helper()
	files, err := listLogFiles(dir)
	if err != nil {
		t.Fatalf("list log files: %v", err)
	}
	return len(files)
}

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

	out := readAllLogs(t, dir)
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

func TestFilenameFormat(t *testing.T) {
	dir := t.TempDir()
	lg, err := NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatalf("NewFromFlags: %v", err)
	}
	lg.Info("hello")
	lg.Close()

	files, err := listLogFiles(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(files))
	}
	name := filepath.Base(files[0])
	if !strings.HasPrefix(name, "codrax-") || !strings.HasSuffix(name, ".log") {
		t.Errorf("unexpected filename format: %s", name)
	}
	// codrax-YYYYMMDD-HHMMSS-mmm.log → 7 + 19 + 4 = 30 chars exactly.
	if len(name) != len("codrax-")+len("20060102-150405-000")+len(".log") {
		t.Errorf("filename %s does not match codrax-YYYYMMDD-HHMMSS-mmm.log", name)
	}
}

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	lg, err := NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatalf("NewFromFlags: %v", err)
	}
	defer lg.Close()

	// Write enough to trigger several rotations. Each call writes
	// roughly 1KB; 4MB / 1KB ≈ 4096 records per file.
	payload := strings.Repeat("x", 1024)
	for i := 0; i < 30000; i++ {
		lg.Info("%s", payload)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files, err := listLogFiles(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// 30000 * ~1KB ≈ 30MB → at least 7 files would have been created,
	// but retention caps the directory at 7.
	if len(files) < 2 {
		t.Errorf("expected multiple rotated files, got %d", len(files))
	}
	if len(files) > maxTotalFiles {
		t.Errorf("expected at most %d files, got %d", maxTotalFiles, len(files))
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

	filesAfterFirst, _ := listLogFiles(dir)
	if len(filesAfterFirst) != 1 {
		t.Fatalf("expected 1 file after first session, got %d", len(filesAfterFirst))
	}
	firstPath := filesAfterFirst[0]
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read after first session: %v", err)
	}
	firstLen := len(first)
	if firstLen == 0 {
		t.Fatalf("first session wrote nothing")
	}

	// Second session: must reuse the same file (it's well under 4MB)
	// rather than creating a fresh timestamped one.
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

	filesAfterSecond, _ := listLogFiles(dir)
	if len(filesAfterSecond) != 1 {
		t.Errorf("expected still 1 file after restart, got %d (%v)", len(filesAfterSecond), filesAfterSecond)
	}
	if filesAfterSecond[0] != firstPath {
		t.Errorf("restart should resume into %s, got %s", firstPath, filesAfterSecond[0])
	}

	second, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read after second session: %v", err)
	}
	if len(second) <= firstLen {
		t.Errorf("second session did not append: firstLen=%d, secondLen=%d", firstLen, len(second))
	}
	if !strings.HasPrefix(string(second), string(first)) {
		t.Errorf("second session truncated/overwrote earlier content")
	}
	for i := 0; i < 5; i++ {
		if !strings.Contains(string(second), fmt.Sprintf("first session line %d", i)) {
			t.Errorf("missing first-session record %d after restart", i)
		}
		if !strings.Contains(string(second), fmt.Sprintf("second session line %d", i)) {
			t.Errorf("missing second-session record %d", i)
		}
	}
}

func TestRestartCreatesNewFileWhenExistingIsFull(t *testing.T) {
	dir := t.TempDir()

	// Seed dir with a fake "full" log file.
	fullName := filepath.Join(dir, "codrax-20200101-000000-000.log")
	if err := os.WriteFile(fullName, make([]byte, maxFileBytes+1), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	lg, err := NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatalf("NewFromFlags: %v", err)
	}
	lg.Info("fresh start")
	lg.Close()

	files, _ := listLogFiles(dir)
	if len(files) != 2 {
		t.Errorf("expected 2 files (existing full + new), got %d (%v)", len(files), files)
	}
	// The new file must be different from the seeded one.
	if files[len(files)-1] == fullName {
		t.Errorf("new session reused full file: %s", fullName)
	}
}

func TestPruneRetainsAtMostMax(t *testing.T) {
	dir := t.TempDir()
	// Seed with 10 fake old files.
	for i := 0; i < 10; i++ {
		name := filepath.Join(dir, fmt.Sprintf("codrax-2020010%d-000000-000.log", i))
		_ = os.WriteFile(name, []byte("x"), 0o644)
	}
	pruneOldFiles(dir)
	files, _ := listLogFiles(dir)
	if len(files) != maxTotalFiles {
		t.Errorf("after prune expected %d files, got %d", maxTotalFiles, len(files))
	}
	// The surviving files must be the newest ones (highest digits).
	for _, f := range files {
		base := filepath.Base(f)
		if strings.Contains(base, "20200100") || strings.Contains(base, "20200101") || strings.Contains(base, "20200102") {
			t.Errorf("oldest file %s should have been pruned", base)
		}
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

	if !strings.Contains(readAllLogs(t, dir), "INFO hello via writer") {
		t.Errorf("InfoWriter did not produce expected record")
	}
	if countLogFiles(t, dir) != 1 {
		t.Errorf("expected 1 log file")
	}
}
