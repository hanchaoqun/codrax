package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func cacheTestIndex(events int) *Index {
	return &Index{Path: "cache.trace", Events: make([]Event, events)}
}

func cacheTestKey(name string) parseCacheKey {
	return parseCacheKey{path: name, size: 1, modUnix: 1, version: ParserVersion}
}

func TestTraceIndexCache_HitReturnsSameIndex(t *testing.T) {
	c := newTraceIndexCache(10 * eventSizeBytes)
	idx := cacheTestIndex(2)
	c.Store(cacheTestKey("a"), idx)
	got, ok := c.Load(cacheTestKey("a"))
	if !ok || got != idx {
		t.Fatalf("hit must return the identical cached index, got %p want %p (ok=%v)", got, idx, ok)
	}
	if _, ok := c.Load(cacheTestKey("missing")); ok {
		t.Fatal("unknown key must miss")
	}
}

func TestTraceIndexCache_OverBudgetInsertEvictsOldest(t *testing.T) {
	c := newTraceIndexCache(2 * eventSizeBytes)
	c.Store(cacheTestKey("a"), cacheTestIndex(1))
	c.Store(cacheTestKey("b"), cacheTestIndex(1))
	c.Store(cacheTestKey("c"), cacheTestIndex(1))
	if _, ok := c.Load(cacheTestKey("a")); ok {
		t.Fatal("oldest entry must be evicted on over-budget insert")
	}
	for _, key := range []string{"b", "c"} {
		if _, ok := c.Load(cacheTestKey(key)); !ok {
			t.Fatalf("entry %q must survive eviction", key)
		}
	}
	if c.used != 2*eventSizeBytes {
		t.Fatalf("byte accounting drifted: used=%d want=%d", c.used, 2*eventSizeBytes)
	}
}

func TestTraceIndexCache_LoadRefreshesRecency(t *testing.T) {
	c := newTraceIndexCache(2 * eventSizeBytes)
	c.Store(cacheTestKey("a"), cacheTestIndex(1))
	c.Store(cacheTestKey("b"), cacheTestIndex(1))
	if _, ok := c.Load(cacheTestKey("a")); !ok {
		t.Fatal("expected hit on a")
	}
	c.Store(cacheTestKey("c"), cacheTestIndex(1))
	if _, ok := c.Load(cacheTestKey("b")); ok {
		t.Fatal("least-recently-used entry b must be evicted, not the touched a")
	}
	if _, ok := c.Load(cacheTestKey("a")); !ok {
		t.Fatal("touch-on-hit must protect a from eviction")
	}
}

func TestTraceIndexCache_SingleOversizedEntryStaysUsable(t *testing.T) {
	c := newTraceIndexCache(1 * eventSizeBytes)
	big := cacheTestIndex(5)
	c.Store(cacheTestKey("big"), big)
	if got, ok := c.Load(cacheTestKey("big")); !ok || got != big {
		t.Fatal("a just-built index larger than the budget must still serve repeat calls")
	}
	c.Store(cacheTestKey("next"), cacheTestIndex(1))
	if _, ok := c.Load(cacheTestKey("big")); ok {
		t.Fatal("the oversized entry must be evicted once a newer entry lands")
	}
}

func TestBuildIndex_CacheHitReturnsSameIndex(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cache_hit.trace")
	content := "          <idle>-0     (-----) [000] d..3  100.000000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=10 next_prio=100\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := BuildIndex(t.Context(), p)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	second, err := BuildIndex(t.Context(), p)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if first != second {
		t.Fatalf("repeat build of a cacheable trace must hit the LRU, got %p then %p", first, second)
	}
}

func TestBuildIndexSingleflight_JoinerWaitsAndSharesResult(t *testing.T) {
	key := cacheTestKey("/nonexistent/singleflight.trace")
	call := &indexBuildCall{done: make(chan struct{})}
	indexBuildMu.Lock()
	indexBuilds[key] = call
	indexBuildMu.Unlock()

	want := &Index{Path: "singleflight"}
	got := make(chan *Index, 1)
	go func() {
		idx, _ := buildIndexSingleflight(context.Background(), key, key.path, key.size, key.modUnix, BuildOptions{}, false)
		got <- idx
	}()
	select {
	case idx := <-got:
		t.Fatalf("joiner must block on the in-flight build, returned %p early", idx)
	case <-time.After(20 * time.Millisecond):
	}

	call.idx = want
	indexBuildMu.Lock()
	delete(indexBuilds, key)
	close(call.done)
	indexBuildMu.Unlock()

	select {
	case idx := <-got:
		if idx != want {
			t.Fatalf("joiner must share the in-flight build's index, got %p want %p", idx, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("joiner never returned after the in-flight build completed")
	}
}
