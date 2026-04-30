package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBaselineCache_Disabled covers the no-op path when maxEntries
// is 0. Lookup must return nil; Store must not create files.
func TestBaselineCache_Disabled(t *testing.T) {
	dir := t.TempDir()
	c := NewBaselineCache(dir, 0)
	if c.Enabled() {
		t.Fatal("cache with maxEntries=0 must report Enabled()=false")
	}
	if got := c.Lookup("abcdef1234567890abcdef1234567890"); got != nil {
		t.Errorf("disabled cache Lookup returned non-nil: %+v", got)
	}
	c.Store("abcdef1234567890abcdef1234567890", &types.ChangeReport{Passed: true})
	entries, _ := os.ReadDir(filepath.Join(dir, "baselines"))
	if len(entries) != 0 {
		t.Errorf("disabled cache wrote %d files; want 0", len(entries))
	}
}

// TestBaselineCache_RoundTrip covers a happy path: store, then
// lookup the same SHA returns an equivalent report.
func TestBaselineCache_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := NewBaselineCache(dir, 5)
	if !c.Enabled() {
		t.Fatal("cache should be enabled")
	}
	sha := "1234567890abcdef1234567890abcdef12345678"
	report := &types.ChangeReport{
		PlanID: "test-plan",
		Passed: true,
		TestResults: []types.TestResult{
			{AssertionID: "TestFoo", Passed: true},
			{AssertionID: "TestBar", Passed: true},
		},
	}
	c.Store(sha, report)
	got := c.Lookup(sha)
	if got == nil {
		t.Fatal("Lookup returned nil after Store")
	}
	if got.PlanID != report.PlanID || got.Passed != report.Passed || len(got.TestResults) != 2 {
		t.Errorf("round-trip drift: got %+v want %+v", got, report)
	}
}

// TestBaselineCache_LRUEviction covers the LRU cap. Storing N+2
// entries on a cap of N should leave exactly N files surviving,
// and the survivors should be the most-recently-touched.
func TestBaselineCache_LRUEviction(t *testing.T) {
	dir := t.TempDir()
	c := NewBaselineCache(dir, 2)
	shas := []string{
		"aaaa000000000000aaaa000000000000aaaa0000",
		"bbbb000000000000bbbb000000000000bbbb0000",
		"cccc000000000000cccc000000000000cccc0000",
		"dddd000000000000dddd000000000000dddd0000",
	}
	for i, sha := range shas {
		report := &types.ChangeReport{PlanID: "p" + string(rune('0'+i))}
		c.Store(sha, report)
		// Sleep nanoseconds so each entry has a distinct mtime —
		// the LRU compare uses ModTime() which is filesystem-
		// dependent; without a small gap the sort is non-deterministic.
		time.Sleep(10 * time.Millisecond)
	}
	cacheDir := filepath.Join(dir, "baselines")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 surviving entries after LRU cap; got %d", len(entries))
	}
	// Most-recent two are c and d; oldest two (a, b) should be
	// evicted.
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "aaaa") || strings.HasPrefix(name, "bbbb") {
			t.Errorf("oldest entry %s survived LRU eviction", name)
		}
	}
}

// TestBaselineCache_Persistence covers the disk format: a stored
// report should be a valid JSON object the cache can read back
// after being constructed fresh.
func TestBaselineCache_Persistence(t *testing.T) {
	dir := t.TempDir()
	sha := "feedface00000000feedface0000000feedface0"
	c1 := NewBaselineCache(dir, 5)
	c1.Store(sha, &types.ChangeReport{PlanID: "persist-test", Passed: false})
	// Verify the file exists and is valid JSON before reading via cache.
	path := c1.path(sha)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var report types.ChangeReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("stored cache file is not valid JSON: %v\ndata=%s", err, string(data))
	}
	// Fresh cache instance reads the same dir.
	c2 := NewBaselineCache(dir, 5)
	got := c2.Lookup(sha)
	if got == nil {
		t.Fatal("fresh cache instance failed to read stored entry")
	}
	if got.PlanID != "persist-test" {
		t.Errorf("PlanID drift: got %q want %q", got.PlanID, "persist-test")
	}
}

// TestBaselineCache_PeerStoreSkipsRecentMtime pins the cross-
// process race-skip (commit 11 #2): when a sibling cache file
// already exists with mtime in the last 60s, Store treats it as
// "peer just wrote, don't clobber" and bails. Older cache files
// (mtime > 60s ago) still get overwritten — that's the "this is
// the same SHA, but the peer's write is so old we'd rather
// refresh" semantics.
func TestBaselineCache_PeerStoreSkipsRecentMtime(t *testing.T) {
	dir := t.TempDir()
	c := NewBaselineCache(dir, 5)
	sha := "race0000000000000race0000000000000race00"
	// First store succeeds.
	c.Store(sha, &types.ChangeReport{PlanID: "first", Passed: true})
	first := c.Lookup(sha)
	if first == nil || first.PlanID != "first" {
		t.Fatalf("first store failed: %+v", first)
	}
	// Second immediate store from "another process" should skip
	// because the file's mtime is fresh (<60s old).
	c.Store(sha, &types.ChangeReport{PlanID: "second", Passed: false})
	got := c.Lookup(sha)
	if got == nil || got.PlanID != "first" {
		t.Errorf("second store should have been skipped; got PlanID=%q (want 'first')", got.PlanID)
	}
}

// TestBaselineCache_EmptyShaSkips covers the empty-sha guard.
func TestBaselineCache_EmptyShaSkips(t *testing.T) {
	dir := t.TempDir()
	c := NewBaselineCache(dir, 5)
	if got := c.Lookup(""); got != nil {
		t.Errorf("Lookup(\"\") returned non-nil")
	}
	c.Store("", &types.ChangeReport{Passed: true})
	entries, _ := os.ReadDir(filepath.Join(dir, "baselines"))
	if len(entries) != 0 {
		t.Errorf("Store(\"\") wrote %d files; want 0", len(entries))
	}
}
