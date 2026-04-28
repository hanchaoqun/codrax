package worktree

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeSessionDir creates a session-shaped directory under base
// with the given mtime. Returns the full path.
func makeSessionDir(t *testing.T, base, name string, mtime time.Time) string {
	t.Helper()
	full := filepath.Join(base, name)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chtimes(full, mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return full
}

// TestPruneByAgeAndQuota_TTLRemovesOlderThan covers the age-stage
// happy path: 2 dirs, one within TTL (10 minutes ago), one beyond
// (5 days ago). With ttl=1h, only the 5-day one is reaped.
func TestPruneByAgeAndQuota_TTLRemovesOlderThan(t *testing.T) {
	base := t.TempDir()
	young := makeSessionDir(t, base, "trace-1234567890-99999", time.Now().Add(-10*time.Minute))
	old := makeSessionDir(t, base, "trace-1234567891-99999", time.Now().Add(-5*24*time.Hour))

	reaped := PruneByAgeAndQuota(base, "", 1*time.Hour, 0)
	if reaped != 1 {
		t.Errorf("ttl reap count = %d, want 1", reaped)
	}
	if _, err := os.Stat(young); err != nil {
		t.Errorf("young dir was wrongly reaped: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old dir should be gone; stat err = %v", err)
	}
}

// TestPruneByAgeAndQuota_QuotaEvictsLRU covers the quota stage:
// 5 dirs survive TTL, max=3 → 2 oldest get evicted, 3 newest stay.
func TestPruneByAgeAndQuota_QuotaEvictsLRU(t *testing.T) {
	base := t.TempDir()
	now := time.Now()
	dirs := []string{
		makeSessionDir(t, base, "trace-100-99999", now.Add(-50*time.Minute)),
		makeSessionDir(t, base, "trace-200-99999", now.Add(-40*time.Minute)),
		makeSessionDir(t, base, "trace-300-99999", now.Add(-30*time.Minute)),
		makeSessionDir(t, base, "trace-400-99999", now.Add(-20*time.Minute)),
		makeSessionDir(t, base, "trace-500-99999", now.Add(-10*time.Minute)),
	}
	reaped := PruneByAgeAndQuota(base, "", 0, 3)
	if reaped != 2 {
		t.Errorf("quota reap count = %d, want 2", reaped)
	}
	// 2 oldest gone, 3 newest stay.
	if _, err := os.Stat(dirs[0]); !os.IsNotExist(err) {
		t.Errorf("oldest (-50min) should be gone")
	}
	if _, err := os.Stat(dirs[1]); !os.IsNotExist(err) {
		t.Errorf("second-oldest (-40min) should be gone")
	}
	for i := 2; i < 5; i++ {
		if _, err := os.Stat(dirs[i]); err != nil {
			t.Errorf("dir[%d] survived but stat err: %v", i, err)
		}
	}
}

// TestPruneByAgeAndQuota_BothGatesDisabledNoop pins the opt-out:
// ttl=0 AND maxCount=0 → no-op, returns 0, no dirs touched.
func TestPruneByAgeAndQuota_BothGatesDisabledNoop(t *testing.T) {
	base := t.TempDir()
	dir := makeSessionDir(t, base, "trace-100-99999", time.Now().Add(-1*time.Hour))
	reaped := PruneByAgeAndQuota(base, "", 0, 0)
	if reaped != 0 {
		t.Errorf("disabled both gates should reap 0, got %d", reaped)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("dir wrongly reaped: %v", err)
	}
}

// TestPruneByAgeAndQuota_NonSessionDirsIgnored guards against
// damaging operator-laid directories: only session-shaped names
// get inspected.
func TestPruneByAgeAndQuota_NonSessionDirsIgnored(t *testing.T) {
	base := t.TempDir()
	keep := filepath.Join(base, "operator-laid-files")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chtimes(keep, time.Now().Add(-100*24*time.Hour), time.Now().Add(-100*24*time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	reaped := PruneByAgeAndQuota(base, "", 1*time.Hour, 1)
	if reaped != 0 {
		t.Errorf("non-session dir should be untouched; reaped=%d", reaped)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("operator-laid dir was wrongly reaped: %v", err)
	}
}

// TestPruneByAgeAndQuota_MissingBaseDir is a robustness check: a
// non-existent base just returns 0 silently.
func TestPruneByAgeAndQuota_MissingBaseDir(t *testing.T) {
	reaped := PruneByAgeAndQuota("/nonexistent/path/codrax/worktrees", "", 1*time.Hour, 1)
	if reaped != 0 {
		t.Errorf("missing baseDir should reap 0; got %d", reaped)
	}
}

// TestPruneByAgeAndQuota_ActiveSessionSkipped guards the in-use
// invariant: a dir registered in activeSessions (a Run is using
// it RIGHT NOW) is never reaped, even if its mtime exceeds TTL.
func TestPruneByAgeAndQuota_ActiveSessionSkipped(t *testing.T) {
	base := t.TempDir()
	active := makeSessionDir(t, base, "trace-aaa-99999", time.Now().Add(-5*24*time.Hour))
	idle := makeSessionDir(t, base, "trace-idle-99999", time.Now().Add(-5*24*time.Hour))

	// Register active dir in the activeSessions map so the prune
	// logic sees it as in-use. cleanupKey is the mainRoot value;
	// any non-empty string works for the test.
	activeSessions.Store(active, "/repo")
	defer activeSessions.Delete(active)

	reaped := PruneByAgeAndQuota(base, "", 1*time.Hour, 0)
	if reaped != 1 {
		t.Errorf("expected 1 reap (idle only); got %d", reaped)
	}
	if _, err := os.Stat(active); err != nil {
		t.Errorf("active dir wrongly reaped: %v", err)
	}
	if _, err := os.Stat(idle); !os.IsNotExist(err) {
		t.Errorf("idle dir should be gone")
	}
}
