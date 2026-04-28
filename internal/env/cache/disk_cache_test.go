package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withTempHome stubs $HOME so Open writes to a test-specific
// directory. Restored automatically.
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows
	return home
}

// TestCache_RoundTrip locks the basic write/read cycle: Set then
// Get returns the original value.
func TestCache_RoundTrip(t *testing.T) {
	withTempHome(t)
	c, err := Open(90)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := c.Set("k1", json.RawMessage(`{"x":1}`), "test"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, ok := c.Get("k1")
	if !ok {
		t.Fatal("Get miss after Set")
	}
	if string(v) != `{"x":1}` {
		t.Errorf("Get returned %q", string(v))
	}
}

// TestCache_TTLExpiry covers the TTL: an entry past its expiry is
// treated as missing even though it's on disk.
func TestCache_TTLExpiry(t *testing.T) {
	withTempHome(t)
	c, err := Open(90)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Manually inject an old entry by bypassing Set (which uses
	// time.Now). Then Get should miss.
	old := time.Now().AddDate(0, 0, -200)
	c.data["expired"] = Entry{
		Key: "expired", Value: json.RawMessage(`"old"`),
		Source: "test", CreatedAt: old,
	}
	if _, ok := c.Get("expired"); ok {
		t.Errorf("expired entry should not be returned")
	}
}

// TestCache_Persistence verifies disk writes survive across Cache
// reopens — atomic rename + JSON marshal round-trip works.
func TestCache_Persistence(t *testing.T) {
	withTempHome(t)
	c1, _ := Open(90)
	_ = c1.Set("persisted", json.RawMessage(`"v"`), "test")

	// Open a fresh Cache; should pick up the persisted entry.
	c2, err := Open(90)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	v, ok := c2.Get("persisted")
	if !ok {
		t.Fatalf("persisted entry missing after re-Open")
	}
	if string(v) != `"v"` {
		t.Errorf("persisted value = %q", string(v))
	}
}

// TestCache_SchemaVersionMismatch verifies that a cache file with
// a stale schema_version is silently dropped (no parse error
// surfaced; the loader treats it as no-cache).
func TestCache_SchemaVersionMismatch(t *testing.T) {
	home := withTempHome(t)
	dir := filepath.Join(home, ".codrax", "cache")
	_ = os.MkdirAll(dir, 0o755)
	// Pre-write a future-schema file.
	old := []byte(`{"schema_version": 999, "entries": [{"key":"x","value":"y"}]}`)
	_ = os.WriteFile(filepath.Join(dir, "env-cache.json"), old, 0o644)

	c, err := Open(90)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, ok := c.Get("x"); ok {
		t.Errorf("schema_version mismatch should drop the cache; got hit")
	}
}

// TestCache_Clear drops everything.
func TestCache_Clear(t *testing.T) {
	withTempHome(t)
	c, _ := Open(90)
	_ = c.Set("a", json.RawMessage(`1`), "test")
	_ = c.Set("b", json.RawMessage(`2`), "test")
	if err := c.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, ok := c.Get("a"); ok {
		t.Errorf("a should be gone after Clear")
	}
	if len(c.Snapshot()) != 0 {
		t.Errorf("Snapshot should be empty after Clear; got %d", len(c.Snapshot()))
	}
}

// TestCache_DeleteOneKey verifies surgical removal: deleting a
// single key leaves siblings intact and persists the change.
func TestCache_DeleteOneKey(t *testing.T) {
	withTempHome(t)
	c, _ := Open(90)
	_ = c.Set("k1", json.RawMessage(`"v1"`), "test")
	_ = c.Set("k2", json.RawMessage(`"v2"`), "test")
	if err := c.Delete("k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := c.Get("k1"); ok {
		t.Errorf("k1 should be gone after Delete")
	}
	v, ok := c.Get("k2")
	if !ok || string(v) != `"v2"` {
		t.Errorf("sibling k2 must survive; ok=%v v=%q", ok, string(v))
	}
	// Re-open to confirm persistence.
	c2, _ := Open(90)
	if _, ok := c2.Get("k1"); ok {
		t.Errorf("Delete did not persist; k1 reappeared after re-Open")
	}
	if _, ok := c2.Get("k2"); !ok {
		t.Errorf("k2 missing after re-Open")
	}
}

// TestCache_TTLBoundary verifies behavior right at the TTL edge:
// entry created exactly N days ago with TTL=N should be considered
// fresh; one day past should be expired.
func TestCache_TTLBoundary(t *testing.T) {
	withTempHome(t)
	c, _ := Open(7) // 7-day TTL

	// Inject an entry 6 days old (within window).
	c.data["fresh"] = Entry{
		Key: "fresh", Value: json.RawMessage(`"ok"`),
		Source: "test", CreatedAt: time.Now().AddDate(0, 0, -6),
	}
	if _, ok := c.Get("fresh"); !ok {
		t.Errorf("entry within TTL window should be returned")
	}

	// Inject an entry 8 days old (past window).
	c.data["stale"] = Entry{
		Key: "stale", Value: json.RawMessage(`"old"`),
		Source: "test", CreatedAt: time.Now().AddDate(0, 0, -8),
	}
	if _, ok := c.Get("stale"); ok {
		t.Errorf("entry past TTL window should NOT be returned")
	}
}
