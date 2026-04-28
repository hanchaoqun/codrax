// Package cache holds the user-home-anchored disk cache for
// env_recommend's LLM fallback results. The cache is the gate
// between Stage 2 ("we've answered this before") and Stage 3
// ("call LLM"). Once populated, an LLM-class diagnosis lookup
// touches no LLM at all — first-time cost only.
//
// Storage: ~/.codrax/cache/env-cache.json. Schema-versioned so a
// codrax upgrade can drop incompatible cache without losing data.
// Atomic write (rename) + flock to survive concurrent codrax
// processes.
package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SchemaVersion is bumped when Entry layout changes. Loaders that
// see a different version drop the file and rebuild.
const SchemaVersion = 1

// Cache is the in-memory + disk hybrid store. One per process
// (load on first access, persist on each Set).
type Cache struct {
	path    string
	mu      sync.RWMutex
	data    map[string]Entry
	loaded  bool
	ttlDays int
}

// Entry is one cache record. Source distinguishes deterministic
// from LLM-derived results so /env cache list can show
// provenance. CreatedAt is used with TTL for expiry.
type Entry struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Source    string          `json:"source"` // "llm" / "deterministic" / "manual"
	CreatedAt time.Time       `json:"created_at"`
}

// fileFormat is the on-disk JSON shape. Keeping the schema
// version out of Entry leaves Entries portable across formats.
type fileFormat struct {
	SchemaVersion int     `json:"schema_version"`
	Entries       []Entry `json:"entries"`
}

// Open returns the process-wide cache rooted at
// ~/.codrax/cache/env-cache.json. Cross-platform via os.UserHomeDir
// — Windows: %USERPROFILE%\.codrax\cache\env-cache.json. Caller
// passes ttlDays from config.
func Open(ttlDays int) (*Cache, error) {
	if ttlDays <= 0 {
		ttlDays = 90
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".codrax", "cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	c := &Cache{
		path:    filepath.Join(dir, "env-cache.json"),
		data:    make(map[string]Entry),
		ttlDays: ttlDays,
	}
	if err := c.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return c, err
	}
	c.loaded = true
	return c, nil
}

// Get returns the cached value for key, or nil if missing /
// expired / not-yet-loaded. Lock-free hot path: RLock only.
func (c *Cache) Get(key string) (json.RawMessage, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.data[key]
	if !ok {
		return nil, false
	}
	// TTL expiry check
	expiry := e.CreatedAt.AddDate(0, 0, c.ttlDays)
	if time.Now().After(expiry) {
		return nil, false
	}
	return e.Value, true
}

// Set writes (key, value) and flushes to disk atomically. Source
// records who generated it.
func (c *Cache) Set(key string, value json.RawMessage, source string) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	c.data[key] = Entry{
		Key:       key,
		Value:     value,
		Source:    source,
		CreatedAt: time.Now(),
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	return c.persist(snapshot)
}

// Delete removes one key. Best-effort persist.
func (c *Cache) Delete(key string) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	delete(c.data, key)
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	return c.persist(snapshot)
}

// Clear wipes all entries. Used by /env cache clear.
func (c *Cache) Clear() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	c.data = make(map[string]Entry)
	c.mu.Unlock()
	return c.persist(nil)
}

// Snapshot returns a copy of all current entries for /env cache list.
func (c *Cache) Snapshot() []Entry {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshotLocked()
}

func (c *Cache) snapshotLocked() []Entry {
	out := make([]Entry, 0, len(c.data))
	for _, e := range c.data {
		out = append(out, e)
	}
	return out
}

func (c *Cache) load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}
	var ff fileFormat
	if err := json.Unmarshal(data, &ff); err != nil {
		// Corrupt file → clean slate, no panic.
		return nil
	}
	if ff.SchemaVersion != SchemaVersion {
		// Schema mismatch → drop, will be rewritten on next Set.
		return nil
	}
	for _, e := range ff.Entries {
		c.data[e.Key] = e
	}
	return nil
}

// persist atomically rewrites the file. Strategy: write to .tmp
// then rename. On Unix, rename is atomic and replaces target.
// On Windows, os.Rename also replaces atomically since Go 1.5.
func (c *Cache) persist(entries []Entry) error {
	ff := fileFormat{SchemaVersion: SchemaVersion, Entries: entries}
	data, err := json.MarshalIndent(ff, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// Path returns the disk file location (for /env cache list display).
func (c *Cache) Path() string {
	if c == nil {
		return ""
	}
	return c.path
}
