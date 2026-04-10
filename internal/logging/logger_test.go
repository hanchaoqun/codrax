package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	// Format: codrax-YYYYMMDD-HHMMSS-mmm-<pid>.log. The structural
	// check is "the embedded PID equals our PID"; that proves both
	// the suffix is present and that pidFromLogFilename can recover
	// it, which is the property the prune sweeper relies on.
	if got := pidFromLogFilename(name); got != os.Getpid() {
		t.Errorf("pidFromLogFilename(%s) = %d, want %d", name, got, os.Getpid())
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

func TestRestartAlwaysCreatesNewFile(t *testing.T) {
	// With per-PID log filenames, every NewFromFlags call opens a
	// fresh file rather than appending to an existing one. This is
	// what makes the multi-instance scenario safe (no two writers
	// share an fd) and is the documented replacement for the old
	// "resume into existing file" behavior. Both files must contain
	// only their own session's records, and listLogFiles must show
	// both, so a tail-of-history reader can still recover everything.
	dir := t.TempDir()

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

	// Sleep just long enough for the millisecond-resolution timestamp
	// in the filename to advance, so the two sessions don't collide
	// on the (timestamp, pid) tuple. Within one process the same PID
	// is reused, so without a timestamp gap openNew would fall through
	// to its numeric collision-counter suffix — which is correct, but
	// makes the assertion below noisier than it needs to be.
	time.Sleep(2 * time.Millisecond)

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
	if len(filesAfterSecond) != 2 {
		t.Fatalf("expected 2 files after restart, got %d (%v)", len(filesAfterSecond), filesAfterSecond)
	}

	// Each file holds exactly its own session's records — no leakage.
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first: %v", err)
	}
	if strings.Contains(string(first), "second session") {
		t.Errorf("first file contains second-session content")
	}
	for i := 0; i < 5; i++ {
		if !strings.Contains(string(first), fmt.Sprintf("first session line %d", i)) {
			t.Errorf("missing first-session record %d in first file", i)
		}
	}

	// The other file (whichever it is) must hold the second session.
	var secondPath string
	for _, p := range filesAfterSecond {
		if p != firstPath {
			secondPath = p
			break
		}
	}
	if secondPath == "" {
		t.Fatalf("could not identify second-session file in %v", filesAfterSecond)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second: %v", err)
	}
	if strings.Contains(string(second), "first session") {
		t.Errorf("second file contains first-session content")
	}
	for i := 0; i < 5; i++ {
		if !strings.Contains(string(second), fmt.Sprintf("second session line %d", i)) {
			t.Errorf("missing second-session record %d in second file", i)
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

func TestConcurrentLoggersDoNotShareFile(t *testing.T) {
	// The whole point of per-PID filenames is that two writers
	// pointed at the same dir never touch the same file. We can't
	// fork a real process from a unit test, so we simulate by
	// constructing two Loggers with the same PID-of-this-process
	// (which is what would happen on the rare millisecond-collision
	// path) and verifying the openNew collision counter still gives
	// each writer its own file. The records of one must not appear
	// in the file of the other.
	dir := t.TempDir()
	lg1, err := NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatalf("lg1: %v", err)
	}
	lg2, err := NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatalf("lg2: %v", err)
	}
	for i := 0; i < 20; i++ {
		lg1.Info("from-one %d", i)
		lg2.Info("from-two %d", i)
	}
	lg1.Close()
	lg2.Close()

	files, _ := listLogFiles(dir)
	if len(files) != 2 {
		t.Fatalf("expected 2 distinct files, got %d (%v)", len(files), files)
	}
	for _, f := range files {
		data, _ := os.ReadFile(f)
		hasOne := strings.Contains(string(data), "from-one")
		hasTwo := strings.Contains(string(data), "from-two")
		if hasOne && hasTwo {
			t.Errorf("file %s mixes records from both writers", filepath.Base(f))
		}
		if !hasOne && !hasTwo {
			t.Errorf("file %s contains records from neither writer", filepath.Base(f))
		}
	}
}

func TestPruneSkipsLivePeerFiles(t *testing.T) {
	// pruneOldFiles must not delete a file whose embedded PID still
	// belongs to a running process — that would be a multi-instance
	// rotation race tearing a peer's active log out from under it.
	// We seed the dir with eight files: seven owned by an obviously
	// dead PID (1, init/systemd, but the bytes are stale fakes) and
	// one owned by THIS test process. After prune the live-owner file
	// must survive even though it's not the newest by name.
	dir := t.TempDir()
	mkfile := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		return path
	}
	// Use a PID we are confident is dead: a deliberately huge number
	// that no kernel will hand out for years. isPidAlive returns false.
	deadPid := 9999999
	for i := 0; i < 7; i++ {
		mkfile(fmt.Sprintf("codrax-2020010%d-000000-000-%d.log", i, deadPid))
	}
	live := mkfile(fmt.Sprintf("codrax-20200101-000000-000-%d.log", os.Getpid()))

	pruneOldFiles(dir)

	files, _ := listLogFiles(dir)
	if len(files) > maxTotalFiles {
		t.Errorf("after prune expected at most %d files, got %d", maxTotalFiles, len(files))
	}
	stillThere := false
	for _, f := range files {
		if f == live {
			stillThere = true
			break
		}
	}
	if !stillThere {
		t.Errorf("live-owner file was incorrectly pruned: %s", live)
	}
}

func TestPidFromLogFilename(t *testing.T) {
	cases := map[string]int{
		"codrax-20260410-024410-000-12345.log":    12345,
		"codrax-20260410-024410-000-12345-1.log":  12345, // collision counter
		"codrax-20200101-000000-000.log":          0,     // legacy, no PID
		"unrelated.log":                           0,
		"codrax-20260410-024410-000-notapid.log":  0,
	}
	for name, want := range cases {
		if got := pidFromLogFilename(name); got != want {
			t.Errorf("pidFromLogFilename(%q) = %d, want %d", name, got, want)
		}
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
