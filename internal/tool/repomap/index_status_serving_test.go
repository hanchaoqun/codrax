package repomap

import (
	"os"
	"path/filepath"
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestIndexStatusStampedAcrossScanModes drives the real
// buildOrLoadGraph flow through its three disk branches on a temp
// repo and asserts each serves a graph with a non-empty, correct
// Source — the structural guarantee behind the banner's "never
// fabricate, never blank" contract (design doc §6.2). The in-memory
// reuse and projection stamps have their own unit pins
// (TestIndexStatusBanner_CloneStampsReuse,
// TestScopedProjectionStampsSource).
func TestIndexStatusStampedAcrossScanModes(t *testing.T) {
	repo := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 5 files so a single change stays under the >30% full-rescan
	// threshold and the incremental branch is reachable.
	write("a.go", "package main\nfunc A() {}\n")
	write("b.go", "package main\nfunc B() { A() }\n")
	write("c.go", "package main\nfunc C() {}\n")
	write("d.go", "package main\nfunc D() {}\n")
	write("e.go", "package main\nfunc E() {}\n")

	// Branch 1: no cache → full scan.
	g1, err := BuildOrLoadGraph(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := g1.Metadata.IndexStatus; got.Source != rmtypes.IndexSourceFullScan || got.Freshness != rmtypes.IndexFreshnessFresh {
		t.Fatalf("fresh repo: Source=%q Freshness=%q, want full_scan/fresh", got.Source, got.Freshness)
	}

	// Branch 2: nothing changed → cache hit.
	g2, err := BuildOrLoadGraph(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := g2.Metadata.IndexStatus; got.Source != rmtypes.IndexSourceCacheHit || got.Freshness != rmtypes.IndexFreshnessFresh {
		t.Fatalf("unchanged repo: Source=%q Freshness=%q, want cache_hit/fresh", got.Source, got.Freshness)
	}
	if g2.Metadata.IndexStatus.CacheWrittenAt == "" {
		t.Errorf("cache hit must carry the manifest written_at for the banner's timestamp split")
	}

	// Branch 3: one file changed → incremental.
	write("a.go", "package main\nfunc A() {}\nfunc A2() {}\n")
	g3, err := BuildOrLoadGraph(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := g3.Metadata.IndexStatus; got.Source != rmtypes.IndexSourceIncremental || got.Freshness != rmtypes.IndexFreshnessFresh {
		t.Fatalf("one changed file: Source=%q Freshness=%q, want incremental/fresh", got.Source, got.Freshness)
	}
}

// TestScopedProjectionStampsSource pins the projection-serving stamp:
// projections are in-memory state and must say so.
func TestScopedProjectionStampsSource(t *testing.T) {
	repo := t.TempDir()
	sub := filepath.Join(repo, "svc")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "s.go"), []byte("package svc\nfunc S() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := BuildOrLoadGraph(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectGraphToRoot(base, repo, sub)
	if err != nil {
		t.Fatal(err)
	}
	if projected == nil {
		t.Fatal("projection unexpectedly empty")
	}
	if got := projected.Metadata.IndexStatus; got.Source != rmtypes.IndexSourceScopedProjection || got.Freshness != rmtypes.IndexFreshnessReused {
		t.Fatalf("projection: Source=%q Freshness=%q, want scoped_projection/reused", got.Source, got.Freshness)
	}
	served := serveScopedProjection(projected, "S")
	if got := served.Metadata.IndexStatus.Source; got != rmtypes.IndexSourceScopedProjection {
		t.Fatalf("served projection clone lost the stamp: %q", got)
	}
}
