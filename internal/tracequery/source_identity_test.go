package tracequery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func overwriteTraceSameSizeAndRestoreMtime(t *testing.T, path, replacement string, original os.FileInfo) os.FileInfo {
	t.Helper()
	if int64(len(replacement)) != original.Size() {
		t.Fatalf("replacement fixture changed size: got=%d want=%d", len(replacement), original.Size())
	}
	// Ensure ctime has an observable opportunity to advance even on filesystems
	// whose change clock is coarser than the Go process clock.
	time.Sleep(2 * time.Millisecond)
	if err := os.WriteFile(path, []byte(replacement), original.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, original.ModTime(), original.ModTime()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != original.Size() || info.ModTime().UnixNano() != original.ModTime().UnixNano() {
		t.Skip("filesystem cannot reproduce the same-size/restored-mtime adversarial fixture exactly")
	}
	return info
}

func TestStrongSourceIdentityRejectsSameSizeRestoredMtimeRewrite(t *testing.T) {
	resetAnchorCaches()
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.systrace")
	originalText := " idle-0 (0) [000] .... 100.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=100\n"
	replacementText := strings.Replace(originalText, "next_pid=10", "next_pid=20", 1)
	if err := os.WriteFile(path, []byte(originalText), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.TraceArtifacts) != 1 || !first.TraceArtifacts[0].sourceIdentity.Strong() {
		t.Skip("host filesystem does not expose device+inode+ctime through os.FileInfo")
	}
	if len(first.Events) != 1 || first.Events[0].NextPID != 10 {
		t.Fatalf("unexpected original parse: %+v", first.Events)
	}
	originalInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	originalIdentity := traceFileIdentityFromInfo(originalInfo)
	rewrittenInfo := overwriteTraceSameSizeAndRestoreMtime(t, path, replacementText, originalInfo)
	if originalIdentity.SameVersion(traceFileIdentityFromInfo(rewrittenInfo)) {
		t.Fatal("adversarial fixture did not advance the strong change identity")
	}

	// Raw evidence must not splice bytes from the rewritten artifact into an
	// Index whose typed event came from the original version.
	raw, issues := loadRawArtifactLines(first, []Event{first.Events[0]})
	if _, ok := raw[first.Events[0].Line]; ok {
		t.Fatalf("raw loader admitted bytes from a different artifact version: %+v", raw)
	}
	if got := issues[first.Events[0].Line]; got != "artifact_identity_changed" {
		t.Fatalf("raw loader issue=%q, want artifact_identity_changed", got)
	}

	// Scheduler carry-in scans use the same private ledger and must reject the
	// rewritten descriptor before consulting/storing the head cache.
	if _, err := sourceSchedulerHeadSnapshot(t.Context(), first.TraceArtifacts[0], 101); err == nil || !strings.Contains(err.Error(), "differs from parsed artifact ledger") {
		t.Fatalf("scheduler head accepted same-size/restored-mtime rewrite: %v", err)
	}

	// The main LRU/singleflight key includes the strong identity, so a second
	// build parses the new bytes instead of returning the stale pointer.
	second, err := BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("index cache returned the stale pre-rewrite index")
	}
	if len(second.Events) != 1 || second.Events[0].NextPID != 20 {
		t.Fatalf("replacement parse was not observed: %+v", second.Events)
	}
}

func TestTraceAnchorKeyIncludesStrongChangeIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anchor_identity.systrace")
	originalText := " app-10 (10) [000] .... 1.000000: sched_wakeup: comm=app pid=10 prio=120 target_cpu=000\n"
	replacementText := strings.Replace(originalText, "pid=10", "pid=20", 1)
	if err := os.WriteFile(path, []byte(originalText), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeIdentity := traceFileIdentityFromInfo(before)
	if !beforeIdentity.Strong() {
		t.Skip("host filesystem does not expose device+inode+ctime through os.FileInfo")
	}
	beforeKey := traceAnchorKeyForInfo(path, before)
	after := overwriteTraceSameSizeAndRestoreMtime(t, path, replacementText, before)
	afterKey := traceAnchorKeyForInfo(path, after)
	if beforeKey == afterKey {
		t.Fatal("anchor cache key ignored a same-size/restored-mtime change identity")
	}
}

func TestValidateTraceFileIdentityAfterReadRejectsMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "streaming.trace")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	opened := traceFileIdentityFromInfo(info)
	if err := validateTraceFileIdentityAfterRead(f, opened, "test_stream"); err != nil {
		t.Fatalf("unchanged source rejected: %v", err)
	}
	if err := os.WriteFile(path, []byte("second-version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateTraceFileIdentityAfterRead(f, opened, "test_stream"); err == nil || !strings.Contains(err.Error(), "changed during test_stream") {
		t.Fatalf("streaming source mutation was not rejected: %v", err)
	}
}

func TestValidateTraceFileIdentityAfterReadRejectsAtomicPathReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streaming.trace")
	replacement := filepath.Join(dir, "replacement.trace")
	if err := os.WriteFile(path, []byte("first-version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	opened := traceFileIdentityFromInfo(info)
	if err := os.WriteFile(replacement, []byte("second-versio\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if err := validateTraceFileIdentityAfterRead(f, opened, "atomic_stream"); err == nil ||
		(!strings.Contains(err.Error(), "changed during") && !strings.Contains(err.Error(), "path was replaced")) {
		t.Fatalf("atomic source replacement was not rejected: %v", err)
	}
}

func TestTraceSourceVersionValidatesWholeRunGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run-version.systrace")
	original := " app-10 (10) [000] .... 1.000000: sched_wakeup: comm=app pid=10 prio=120 target_cpu=000\n"
	replacement := strings.Replace(original, "pid=10", "pid=20", 1)
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	version, err := CaptureTraceSourceVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if version.Fingerprint() == "" || version.SourceBytes() != int64(len(original)) {
		t.Fatalf("opaque source version metadata = fingerprint:%q bytes:%d", version.Fingerprint(), version.SourceBytes())
	}
	if err := version.Validate(path); err != nil {
		t.Fatalf("unchanged source rejected: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	identity := traceFileIdentityFromInfo(info)
	if !identity.Strong() {
		t.Skip("host filesystem does not expose device+inode+ctime through os.FileInfo")
	}
	after := overwriteTraceSameSizeAndRestoreMtime(t, path, replacement, info)
	if identity.SameVersion(traceFileIdentityFromInfo(after)) {
		t.Fatal("adversarial fixture did not change the strong identity")
	}
	if err := version.Validate(path); err == nil || !strings.Contains(err.Error(), "source universe changed") {
		t.Fatalf("same-size/restored-mtime rewrite passed run-level lock: %v", err)
	}
}

func TestTraceSourceVersionRejectsAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run-version.systrace")
	replacement := filepath.Join(dir, "replacement.systrace")
	if err := os.WriteFile(path, []byte("first-version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	version, err := CaptureTraceSourceVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, []byte("second-version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if err := version.Validate(path); err == nil || !strings.Contains(err.Error(), "source universe changed") {
		t.Fatalf("atomic replacement passed run-level lock: %v", err)
	}
}
