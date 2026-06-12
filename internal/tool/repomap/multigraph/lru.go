// Package multigraph holds the multi-repo carrier (Z+Y hybrid:
// SymbolOracle/SymbolLocator fan-out + raw-field flatten/owner-aware
// access) consumed by the read pipeline when codrax.yaml ::
// multi_repo_enabled is true (default). See
// docs/design/multi_repo_discovery_and_lazy_load.md for the design.
package multigraph

import (
	"container/list"
	"sync"
	"time"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// LRU is a thread-safe least-recently-used cache mapping sub-repo
// slugs to *types.Graph references. Capped — Put returns the evicted
// (slug, graph) when the cap is exceeded.
//
// Implemented with container/list (Go stdlib) so the multigraph
// package introduces zero external dependencies (CLAUDE.md
// "gopkg.in/yaml.v3 only" rule).
//
// Concurrency: every public method takes the embedded mutex. Get
// promotes the entry to MRU position (a write-in-disguise), so
// concurrent reads of the same slug do not race.
type LRU struct {
	cap   int
	mu    sync.Mutex
	ll    *list.List
	items map[string]*list.Element
}

type lruEntry struct {
	slug  string
	graph *rmtypes.Graph
}

// NewLRU returns an LRU with the given cap. cap <= 0 panics — the
// degenerate single-graph case still uses cap=1, never zero.
func NewLRU(cap int) *LRU {
	if cap <= 0 {
		panic("multigraph.NewLRU: cap must be positive")
	}
	return &LRU{
		cap:   cap,
		ll:    list.New(),
		items: make(map[string]*list.Element, cap),
	}
}

// Cap returns the maximum number of entries.
func (c *LRU) Cap() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cap
}

// SetCap resizes the cap. The new cap applies on the next Put —
// already-resident entries are NOT evicted just because the cap
// shrank. Cap <= 0 is a no-op.
func (c *LRU) SetCap(n int) {
	if n <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cap = n
}

// Len returns the current entry count.
func (c *LRU) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// Get returns the *Graph for slug and promotes it to MRU. Returns
// (nil, false) on miss.
func (c *LRU) Get(slug string) (*rmtypes.Graph, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[slug]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*lruEntry).graph, true
}

// Put inserts or updates slug → g. When inserting causes Len > Cap,
// the LRU entry is evicted; (evictedSlug, evictedGraph, true) is
// returned. On no-eviction Put, returns ("", nil, false).
//
// Updating an existing slug refreshes its position to MRU and never
// triggers eviction.
func (c *LRU) Put(slug string, g *rmtypes.Graph) (string, *rmtypes.Graph, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[slug]; ok {
		c.ll.MoveToFront(el)
		el.Value.(*lruEntry).graph = g
		return "", nil, false
	}
	el := c.ll.PushFront(&lruEntry{slug: slug, graph: g})
	c.items[slug] = el
	if c.ll.Len() > c.cap {
		oldest := c.ll.Back()
		if oldest != nil {
			c.ll.Remove(oldest)
			ent := oldest.Value.(*lruEntry)
			delete(c.items, ent.slug)
			return ent.slug, ent.graph, true
		}
	}
	return "", nil, false
}

// Has reports whether the LRU currently holds slug. Does NOT promote.
func (c *LRU) Has(slug string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.items[slug]
	return ok
}

// Snapshot returns a slug→*Graph map of every resident entry, in
// no particular order (recently-used not exposed). Safe to mutate
// the returned map; the underlying LRU is unaffected.
func (c *LRU) Snapshot() map[string]*rmtypes.Graph {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]*rmtypes.Graph, c.ll.Len())
	for el := c.ll.Front(); el != nil; el = el.Next() {
		ent := el.Value.(*lruEntry)
		out[ent.slug] = ent.graph
	}
	return out
}

// Slugs returns the slugs in MRU→LRU order.
func (c *LRU) Slugs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, c.ll.Len())
	for el := c.ll.Front(); el != nil; el = el.Next() {
		out = append(out, el.Value.(*lruEntry).slug)
	}
	return out
}

// Drop removes slug if present. No-op when missing.
func (c *LRU) Drop(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[slug]; ok {
		c.ll.Remove(el)
		delete(c.items, slug)
	}
}

// ThrashingTracker tracks LRU eviction frequency over a sliding
// window. Crossing the threshold (default >5 evictions per 60s) is
// the signal MultiGraph surfaces to callers via the Thrashing() flag
// — telemetry layer logs a fail-loud Warning + Phase 6 emits an
// operator-facing event so the user can raise multi_repo_max_active.
//
// Concurrency: every method takes the embedded mutex.
type ThrashingTracker struct {
	mu        sync.Mutex
	window    time.Duration
	threshold int
	events    []time.Time
	tripped   bool
	now       func() time.Time // injectable for tests
}

// NewThrashingTracker constructs a tracker with the documented
// window=60s threshold=5 defaults. Injectable now for tests.
func NewThrashingTracker() *ThrashingTracker {
	return &ThrashingTracker{
		window:    60 * time.Second,
		threshold: 5,
		now:       time.Now,
	}
}

// SetClock injects a custom now function (testing only).
func (t *ThrashingTracker) SetClock(now func() time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.now = now
}

// Record an eviction event. Returns true if this event tripped the
// threshold for the first time (transition false→true). Stays
// tripped until ResetTrip is called.
func (t *ThrashingTracker) Record() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	t.events = append(t.events, now)
	cutoff := now.Add(-t.window)
	keep := t.events[:0]
	for _, ev := range t.events {
		if ev.After(cutoff) || ev.Equal(cutoff) {
			keep = append(keep, ev)
		}
	}
	t.events = keep
	if len(t.events) > t.threshold && !t.tripped {
		t.tripped = true
		return true
	}
	return false
}

// Tripped reports whether the threshold has been crossed.
func (t *ThrashingTracker) Tripped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tripped
}

// CountInWindow returns the number of evictions in the current
// window. Useful for telemetry.
func (t *ThrashingTracker) CountInWindow() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.events)
}

// ResetTrip clears the tripped flag (does not clear the event
// history). Call after the operator has been notified so a second
// burst can re-trip.
func (t *ThrashingTracker) ResetTrip() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tripped = false
}

// Clear drops every resident, returning how many were evicted. Used
// by the explicit /repos refresh affordance: a user-requested refresh
// must not keep serving in-memory graphs whose ScanTime predates it.
func (l *LRU) Clear() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(l.items)
	l.ll.Init()
	l.items = make(map[string]*list.Element)
	return n
}
