package multigraph

import (
	"fmt"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// makeGraph mints a tiny *rmtypes.Graph with a few files + symbols
// + import edges so the multigraph fan-out / flatten paths have
// something to operate on.
func makeGraph(slug string, files []string, syms map[string]string) *rmtypes.Graph {
	g := &rmtypes.Graph{
		Root:           "/test/" + slug,
		FileIndex:      make(map[string]*rmtypes.FileInfo, len(files)),
		SymbolDefs:     make(map[string][]*rmtypes.Symbol, len(syms)),
		ImportGraph:    make(map[string][]string),
		ReverseImports: make(map[string][]string),
		Scores:         make(map[string]float64),
		QueryScores:    make(map[string]float64),
		Metadata: rmtypes.Metadata{
			ScanTime:     time.Now(),
			FileCount:    len(files),
			Languages:    map[string]int{"go": len(files)},
			SpecialFiles: []string{"go.mod"},
		},
	}
	for _, f := range files {
		fi := &rmtypes.FileInfo{
			RelPath:   f,
			Language:  "go",
			ParseTier: 1,
		}
		g.Files = append(g.Files, fi)
		g.FileIndex[f] = fi
		g.Scores[f] = 0.5
		g.QueryScores[f] = 0.3
	}
	for name, file := range syms {
		g.SymbolDefs[name] = []*rmtypes.Symbol{
			{Name: name, File: file, Line: 10, EndLine: 20},
		}
	}
	if len(files) >= 2 {
		g.ImportGraph[files[0]] = []string{files[1]}
		g.ReverseImports[files[1]] = []string{files[0]}
	}
	return g
}

func mkTopo(parentRoot string, subs []topology.SubRepo) *topology.RepoTopology {
	return &topology.RepoTopology{
		ParentRoot:   parentRoot,
		ParentSlug:   "parent-deadbeef",
		Repos:        subs,
		DiscoveredAt: time.Now(),
	}
}

// === LRU ===

func TestLRU_PutGetEviction(t *testing.T) {
	c := NewLRU(2)
	c.Put("a", makeGraph("a", []string{"a/x.go"}, nil))
	c.Put("b", makeGraph("b", []string{"b/x.go"}, nil))
	if c.Len() != 2 {
		t.Fatalf("Len = %d want 2", c.Len())
	}
	// "a" is now LRU; insert "c" → evicts "a".
	evictedSlug, _, did := c.Put("c", makeGraph("c", []string{"c/x.go"}, nil))
	if !did || evictedSlug != "a" {
		t.Errorf("Put(c) eviction = (%q, did=%v), want (a, true)", evictedSlug, did)
	}
	if _, ok := c.Get("a"); ok {
		t.Errorf("a should have been evicted")
	}
	if _, ok := c.Get("b"); !ok {
		t.Errorf("b should still be resident")
	}
}

func TestLRU_GetPromotesMRU(t *testing.T) {
	c := NewLRU(2)
	c.Put("a", makeGraph("a", []string{"a/x.go"}, nil))
	c.Put("b", makeGraph("b", []string{"b/x.go"}, nil))
	// Touch a (now MRU); insert c → evicts b.
	c.Get("a")
	evicted, _, _ := c.Put("c", makeGraph("c", []string{"c/x.go"}, nil))
	if evicted != "b" {
		t.Errorf("expected b evicted (a was promoted), got %q", evicted)
	}
}

func TestLRU_UpdateNoEvict(t *testing.T) {
	c := NewLRU(2)
	c.Put("a", makeGraph("a", []string{"a/x.go"}, nil))
	c.Put("b", makeGraph("b", []string{"b/x.go"}, nil))
	_, _, did := c.Put("a", makeGraph("a2", []string{"a/y.go"}, nil))
	if did {
		t.Errorf("update of existing slug should NOT evict")
	}
	if c.Len() != 2 {
		t.Errorf("Len = %d want 2 after update", c.Len())
	}
}

// === ThrashingTracker ===

func TestThrashing_TripsOverThreshold(t *testing.T) {
	tt := NewThrashingTracker()
	now := time.Now()
	clock := func() time.Time { return now }
	tt.SetClock(clock)
	for i := 0; i < 5; i++ {
		if tripped := tt.Record(); tripped {
			t.Errorf("trip too early at i=%d", i)
		}
	}
	// 6th event → trips.
	if tripped := tt.Record(); !tripped {
		t.Errorf("expected trip at 6th event")
	}
	if !tt.Tripped() {
		t.Error("Tripped() should be true")
	}
	// Subsequent events: tripped already, returns false.
	if tripped := tt.Record(); tripped {
		t.Errorf("Record after trip should not return true again until ResetTrip")
	}
}

func TestThrashing_OldEventsExpire(t *testing.T) {
	tt := NewThrashingTracker()
	cur := time.Now()
	tt.SetClock(func() time.Time { return cur })
	for i := 0; i < 3; i++ {
		tt.Record()
	}
	cur = cur.Add(2 * time.Minute) // beyond 60s window
	for i := 0; i < 3; i++ {
		tt.Record()
	}
	if got := tt.CountInWindow(); got != 3 {
		t.Errorf("CountInWindow = %d want 3 (older 3 expired)", got)
	}
}

// === MultiGraph: single-repo posture ===

func TestMultiGraph_SinglePosture(t *testing.T) {
	g := makeGraph("only", []string{"main.go", "lib.go"}, map[string]string{"Run": "main.go", "Helper": "lib.go"})
	loaded := atomic.Int32{}
	build := func(root, _ string) (*rmtypes.Graph, error) {
		loaded.Add(1)
		return g, nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "only-aabbccdd", RootAbs: "/parent", RootRel: ".", GitMode: "dir", PrimaryLangs: []string{"go"}, FileCount: 2},
	})
	mg, err := New(Config{Topology: topo, Build: build})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !mg.IsSingle() {
		t.Fatal("expected IsSingle()")
	}
	got, err := mg.Single()
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	if got != g {
		t.Errorf("Single returned wrong graph")
	}
	if loaded.Load() != 1 {
		t.Errorf("expected 1 build call, got %d", loaded.Load())
	}
	// Re-fetch — should not re-build.
	if _, err := mg.Single(); err != nil {
		t.Fatalf("Single second: %v", err)
	}
	if loaded.Load() != 1 {
		t.Errorf("Second Single() should hit cache, got %d total builds", loaded.Load())
	}
	// PartialTypedLane: single mode is never partial.
	if mg.PartialTypedLane() {
		t.Error("single-repo PartialTypedLane should be false")
	}
}

// === MultiGraph: multi-repo flatten + fan-out ===

func TestMultiGraph_MultiRepoFiles(t *testing.T) {
	gA := makeGraph("a", []string{"main.go", "util.go"}, nil)
	gB := makeGraph("b", []string{"src/lib.rs"}, nil)
	build := func(root, _ string) (*rmtypes.Graph, error) {
		switch root {
		case "/parent/repo-a":
			return gA, nil
		case "/parent/repo-b":
			return gB, nil
		}
		return nil, fmt.Errorf("unknown root %q", root)
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
	})
	mg, err := New(Config{Topology: topo, Build: build, Cap: 3})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if mg.IsSingle() {
		t.Fatal("multi-repo posture expected")
	}
	if err := mg.EnsureMany([]string{"repo-a", "repo-b"}); err != nil {
		t.Fatalf("EnsureMany: %v", err)
	}
	// Flatten: 3 files total (2 from a, 1 from b)
	files := mg.Files()
	if len(files) != 3 {
		t.Errorf("Files len = %d want 3", len(files))
	}
	// FileInfoFor("repo-a/main.go") returns the *FileInfo with internal RelPath "main.go".
	fi, sr, hit := mg.FileInfoFor("repo-a/main.go")
	if !hit {
		t.Fatalf("FileInfoFor miss for repo-a/main.go")
	}
	if sr.Slug != "repo-a" {
		t.Errorf("owning slug = %q, want repo-a", sr.Slug)
	}
	if fi.RelPath != "main.go" {
		t.Errorf("internal RelPath = %q, want main.go (sub-repo prefix stripped)", fi.RelPath)
	}
	// Cross-repo lookup: nope.
	if _, _, hit := mg.FileInfoFor("repo-x/main.go"); hit {
		t.Errorf("FileInfoFor for unknown sub-repo should miss")
	}
}

func TestMultiGraph_GraphFor_NotActive(t *testing.T) {
	gA := makeGraph("a", []string{"main.go"}, nil)
	build := func(root, _ string) (*rmtypes.Graph, error) {
		switch root {
		case "/parent/repo-a":
			return gA, nil
		}
		return nil, fmt.Errorf("not loaded")
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	// Don't load anything. GraphFor should return (nil, sr, false).
	g, sr, hit := mg.GraphFor("repo-a/main.go")
	if hit || g != nil {
		t.Errorf("GraphFor before EnsureLoaded should miss, got hit=%v g=%v", hit, g)
	}
	if sr == nil || sr.Slug != "repo-a" {
		t.Errorf("sr should still be returned for known sub-repo, got %v", sr)
	}
}

func TestMultiGraph_EnsureManyLoadsInParallel(t *testing.T) {
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
	})
	startedA := make(chan struct{}, 1)
	startedB := make(chan struct{}, 1)
	release := make(chan struct{})
	build := func(root, _ string) (*rmtypes.Graph, error) {
		switch root {
		case "/parent/repo-a":
			startedA <- struct{}{}
		case "/parent/repo-b":
			startedB <- struct{}{}
		default:
			return nil, fmt.Errorf("unknown root %q", root)
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
			return nil, fmt.Errorf("test timed out waiting for release")
		}
		return makeGraph(root, []string{"x.go"}, nil), nil
	}
	mg, err := New(Config{Topology: topo, Build: build, Cap: 2})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- mg.EnsureMany([]string{"repo-a", "repo-b"})
	}()
	select {
	case <-startedA:
	case <-time.After(time.Second):
		t.Fatal("repo-a build did not start")
	}
	select {
	case <-startedB:
	case <-time.After(time.Second):
		t.Fatal("repo-b build did not start; EnsureMany appears serialized")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("EnsureMany: %v", err)
	}
}

func TestMultiGraph_EnsureLoadedCoalescesSameSlug(t *testing.T) {
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
	})
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	builds := atomic.Int32{}
	build := func(root, _ string) (*rmtypes.Graph, error) {
		builds.Add(1)
		started <- struct{}{}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
			return nil, fmt.Errorf("test timed out waiting for release")
		}
		return makeGraph(root, []string{"x.go"}, nil), nil
	}
	mg, err := New(Config{Topology: topo, Build: build, Cap: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := mg.EnsureLoaded("repo-a")
			done <- err
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("repo-a build did not start")
	}
	select {
	case <-started:
		t.Fatal("duplicate same-slug build started before first completed")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("EnsureLoaded[%d]: %v", i, err)
		}
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("same-slug load should coalesce to 1 build, got %d", got)
	}
}

func TestMultiGraph_EnsureLoadedEmitsWaitNoticeForCoalescedSameSlug(t *testing.T) {
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a", FileCount: 42},
	})
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	build := func(root, _ string) (*rmtypes.Graph, error) {
		started <- struct{}{}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
			return nil, fmt.Errorf("test timed out waiting for release")
		}
		return makeGraph(root, []string{"x.go"}, nil), nil
	}
	waitEvents := make(chan types.RepoMapScanEvent, 2)
	mg, err := New(Config{
		Topology: topo,
		Build:    build,
		Cap:      1,
		WaitNotifier: func(ev types.RepoMapScanEvent) {
			waitEvents <- ev
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := mg.EnsureLoaded("repo-a")
		firstDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("repo-a build did not start")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := mg.EnsureLoaded("repo-a")
		secondDone <- err
	}()
	select {
	case ev := <-waitEvents:
		if ev.Phase != types.RepoMapScanPhaseWait || !ev.Started {
			t.Fatalf("wait event = %+v, want wait/start", ev)
		}
		if ev.DisplayName != "repo-a" || ev.SubRepoRootRel != "repo-a" || ev.TotalFiles != 42 {
			t.Fatalf("wait event should carry sub-repo label and file count, got %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("coalesced same-slug caller did not emit wait notice")
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first EnsureLoaded: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second EnsureLoaded: %v", err)
	}
}

// === Oracle fan-out ===

func TestMultiGraph_OracleFanOut(t *testing.T) {
	gA := makeGraph("a", []string{"main.go"}, map[string]string{"Run": "main.go"})
	gB := makeGraph("b", []string{"src/lib.rs"}, map[string]string{"Helper": "src/lib.rs"})
	build := func(root, _ string) (*rmtypes.Graph, error) {
		switch root {
		case "/parent/repo-a":
			return gA, nil
		case "/parent/repo-b":
			return gB, nil
		}
		return nil, fmt.Errorf("unknown")
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureMany([]string{"repo-a", "repo-b"})

	oracle := mg.Oracle()
	if found, _ := oracle.SymbolExists("Run"); !found {
		t.Error("oracle should find Run in repo-a")
	}
	if found, _ := oracle.SymbolExists("Helper"); !found {
		t.Error("oracle should find Helper in repo-b")
	}
	if found, _ := oracle.SymbolExists("Nonexistent"); found {
		t.Error("oracle should not find Nonexistent")
	}
}

// === Locator fan-out (with sub-repo prefix) ===

func TestMultiGraph_LocatorPrefix(t *testing.T) {
	gA := makeGraph("a", []string{"main.go"}, map[string]string{"Run": "main.go"})
	gB := makeGraph("b", []string{"main.go"}, map[string]string{"Run": "main.go"}) // same name, same internal path!
	build := func(root, _ string) (*rmtypes.Graph, error) {
		switch root {
		case "/parent/repo-a":
			return gA, nil
		case "/parent/repo-b":
			return gB, nil
		}
		return nil, fmt.Errorf("unknown")
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureMany([]string{"repo-a", "repo-b"})

	locs := mg.Locator().LocateSymbol("Run")
	if len(locs) != 2 {
		t.Fatalf("expected 2 locations (one per sub-repo), got %d", len(locs))
	}
	files := []string{locs[0].File, locs[1].File}
	sort.Strings(files)
	want := []string{"repo-a/main.go", "repo-b/main.go"}
	if files[0] != want[0] || files[1] != want[1] {
		t.Errorf("locator files = %v, want %v (prefixed by sub-repo)", files, want)
	}
}

// === PartialTypedLane / PendingSubRepoNames ===

func TestMultiGraph_PartialTypedLane(t *testing.T) {
	gA := makeGraph("a", []string{"main.go"}, nil)
	build := func(root, _ string) (*rmtypes.Graph, error) {
		if root == "/parent/repo-a" {
			return gA, nil
		}
		return nil, fmt.Errorf("not loaded")
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
		{Slug: "repo-c", RootAbs: "/parent/repo-c", RootRel: "repo-c"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	// 0/3 active → partial (R3 conservative: "we haven't consulted
	// every sub-repo, so the typed lane could miss matches in
	// repo-b / repo-c"). The R3 semantic is "are we sure we asked
	// every relevant sub-repo?" — at 0 active the answer is no.
	if !mg.PartialTypedLane() {
		t.Error("expected PartialTypedLane true (0 active < 3 topology — nothing consulted yet)")
	}
	mg.EnsureLoaded("repo-a")
	if !mg.PartialTypedLane() {
		t.Error("expected partial: only 1 of 3 sub-repos active")
	}
	pending := mg.PendingSubRepoNames()
	sort.Strings(pending)
	if len(pending) != 2 || pending[0] != "repo-b" || pending[1] != "repo-c" {
		t.Errorf("pending = %v, want [repo-b repo-c]", pending)
	}
}

// === EnsureMany cap fail-loud ===

func TestMultiGraph_EnsureManyTooMany(t *testing.T) {
	build := func(root, _ string) (*rmtypes.Graph, error) {
		return makeGraph(root, []string{"x.go"}, nil), nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
		{Slug: "repo-c", RootAbs: "/parent/repo-c", RootRel: "repo-c"},
		{Slug: "repo-d", RootAbs: "/parent/repo-d", RootRel: "repo-d"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 2})
	err := mg.EnsureMany([]string{"repo-a", "repo-b", "repo-c"})
	if err == nil {
		t.Fatal("expected ErrTooManyActive — fail-loud R3")
	}
}

// === Metadata aggregation ===

func TestMultiGraph_MetadataAggregate(t *testing.T) {
	gA := makeGraph("a", []string{"main.go"}, nil)
	gB := makeGraph("b", []string{"src/lib.rs", "src/util.rs"}, nil)
	gB.Metadata.Languages = map[string]int{"rust": 2}
	build := func(root, _ string) (*rmtypes.Graph, error) {
		switch root {
		case "/parent/repo-a":
			return gA, nil
		case "/parent/repo-b":
			return gB, nil
		}
		return nil, fmt.Errorf("unknown")
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureMany([]string{"repo-a", "repo-b"})

	md := mg.Metadata()
	if md.FileCount != 3 {
		t.Errorf("FileCount = %d want 3", md.FileCount)
	}
	if md.Languages["go"] != 1 {
		t.Errorf("Languages.go = %d want 1", md.Languages["go"])
	}
	if md.Languages["rust"] != 2 {
		t.Errorf("Languages.rust = %d want 2", md.Languages["rust"])
	}
	// SpecialFiles get sub-repo prefix.
	gotPrefixed := 0
	for _, sf := range md.SpecialFiles {
		if sf == "repo-a/go.mod" || sf == "repo-b/go.mod" {
			gotPrefixed++
		}
	}
	if gotPrefixed != 2 {
		t.Errorf("expected both prefixed go.mod entries, got %v", md.SpecialFiles)
	}
}

// === LookupSymbol / ImplementersOf / LookupSymbolByID / Iter helpers ===

func TestMultiGraph_LookupSymbol_FanOut(t *testing.T) {
	gA := makeGraph("a", []string{"main.go"}, map[string]string{"Run": "main.go"})
	gB := makeGraph("b", []string{"main.go"}, map[string]string{"Run": "main.go"})
	build := func(root, _ string) (*rmtypes.Graph, error) {
		switch root {
		case "/parent/repo-a":
			return gA, nil
		case "/parent/repo-b":
			return gB, nil
		}
		return nil, fmt.Errorf("unknown")
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureMany([]string{"repo-a", "repo-b"})

	hits := mg.LookupSymbol("Run")
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits (one per sub-repo), got %d", len(hits))
	}
	subRels := []string{hits[0].Sub.RootRel, hits[1].Sub.RootRel}
	sort.Strings(subRels)
	if subRels[0] != "repo-a" || subRels[1] != "repo-b" {
		t.Errorf("unexpected sub-repos in hits: %v", subRels)
	}
}

func TestMultiGraph_LookupSymbolByID_OwnerAware(t *testing.T) {
	g := makeGraph("a", []string{"main.go"}, map[string]string{"Run": "main.go"})
	// Inject a synthetic SymbolID so we can look it up.
	id := rmtypes.SymbolID("synthetic-id-aabbccdd")
	g.SymbolByID = map[rmtypes.SymbolID]*rmtypes.Symbol{id: g.SymbolDefs["Run"][0]}
	build := func(root, _ string) (*rmtypes.Graph, error) { return g, nil }
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureLoaded("repo-a")

	sym, sr, hit := mg.LookupSymbolByID(id)
	if !hit || sym == nil {
		t.Fatal("LookupSymbolByID miss")
	}
	if sr.Slug != "repo-a" {
		t.Errorf("owning slug = %q, want repo-a", sr.Slug)
	}
}

func TestMultiGraph_IterateSymbolDefs(t *testing.T) {
	gA := makeGraph("a", []string{"main.go"}, map[string]string{"Run": "main.go", "Helper": "main.go"})
	gB := makeGraph("b", []string{"lib.go"}, map[string]string{"Util": "lib.go"})
	build := func(root, _ string) (*rmtypes.Graph, error) {
		switch root {
		case "/parent/repo-a":
			return gA, nil
		case "/parent/repo-b":
			return gB, nil
		}
		return nil, fmt.Errorf("unknown")
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureMany([]string{"repo-a", "repo-b"})

	visited := map[string]bool{}
	mg.IterateSymbolDefs(func(name string, defs []*rmtypes.Symbol, sub *topology.SubRepo) bool {
		visited[name+"@"+sub.RootRel] = true
		return true
	})
	want := []string{"Run@repo-a", "Helper@repo-a", "Util@repo-b"}
	for _, w := range want {
		if !visited[w] {
			t.Errorf("missing iteration entry %q (got %v)", w, visited)
		}
	}
}

func TestMultiGraph_IterateFileIndex(t *testing.T) {
	gA := makeGraph("a", []string{"main.go", "util.go"}, nil)
	gB := makeGraph("b", []string{"src/lib.rs"}, nil)
	build := func(root, _ string) (*rmtypes.Graph, error) {
		switch root {
		case "/parent/repo-a":
			return gA, nil
		case "/parent/repo-b":
			return gB, nil
		}
		return nil, fmt.Errorf("unknown")
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
		{Slug: "repo-b", RootAbs: "/parent/repo-b", RootRel: "repo-b"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureMany([]string{"repo-a", "repo-b"})

	count := 0
	mg.IterateFileIndex(func(rel string, fi *rmtypes.FileInfo, sub *topology.SubRepo) bool {
		count++
		// Internal relpath should match what's in g.FileIndex (sub-repo-relative,
		// no prefix).
		if rel == "main.go" && sub.RootRel != "repo-a" {
			t.Errorf("main.go owner = %q, want repo-a", sub.RootRel)
		}
		return true
	})
	if count != 3 {
		t.Errorf("IterateFileIndex visited %d entries, want 3 (2+1)", count)
	}
}

// === Routing fold (channels A/B/C/D/E) ===

func TestMultiGraph_RouteActiveSet_Single(t *testing.T) {
	g := makeGraph("only", []string{"main.go"}, nil)
	build := func(_, _ string) (*rmtypes.Graph, error) { return g, nil }
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "only-aa", RootAbs: "/parent", RootRel: ".", FileCount: 100},
	})
	mg, _ := New(Config{Topology: topo, Build: build})
	dec := mg.RouteActiveSet(RoutingInputs{})
	if len(dec.Active) != 1 || dec.Active[0] != "only-aa" {
		t.Errorf("single-repo route should return [only-aa], got %v", dec.Active)
	}
	if dec.Reasons["only-aa"] != "S" {
		t.Errorf("expected reason S (single), got %q", dec.Reasons["only-aa"])
	}
}

func TestMultiGraph_RouteActiveSet_FocusPin(t *testing.T) {
	build := func(root, _ string) (*rmtypes.Graph, error) {
		return makeGraph(root, []string{"x.go"}, nil), nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "a", RootAbs: "/parent/a", RootRel: "a", FileCount: 100},
		{Slug: "b", RootAbs: "/parent/b", RootRel: "b", FileCount: 50},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 1})
	// Focus pin "b" — even though "a" is bigger (channel E preference),
	// channel A (focus) wins.
	dec := mg.RouteActiveSet(RoutingInputs{FocusSlugs: []string{"b"}})
	if len(dec.Active) != 1 || dec.Active[0] != "b" {
		t.Errorf("focus pin should force b active, got %v", dec.Active)
	}
	if dec.Reasons["b"] != "A" {
		t.Errorf("expected reason A (focus), got %q", dec.Reasons["b"])
	}
}

func TestMultiGraph_RouteActiveSet_RequiredFiles(t *testing.T) {
	build := func(root, _ string) (*rmtypes.Graph, error) {
		return makeGraph(root, []string{"x.go"}, nil), nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "a", RootAbs: "/parent/a", RootRel: "a", FileCount: 50},
		{Slug: "b", RootAbs: "/parent/b", RootRel: "b", FileCount: 100},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 1})
	// Channel B: RequiredFiles → owning sub-repo. With cap=1 and "a"
	// matching, "a" wins over "b" (which would have won channel E).
	dec := mg.RouteActiveSet(RoutingInputs{
		RequiredFiles: []string{"a/main.go"},
	})
	if len(dec.Active) != 1 || dec.Active[0] != "a" {
		t.Errorf("RequiredFiles channel B should pick a, got %v", dec.Active)
	}
	if dec.Reasons["a"] != "B" {
		t.Errorf("expected reason B, got %q", dec.Reasons["a"])
	}
}

func TestMultiGraph_RouteActiveSet_LogFrameFiles(t *testing.T) {
	build := func(root, _ string) (*rmtypes.Graph, error) {
		return makeGraph(root, []string{"x.go"}, nil), nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "a", RootAbs: "/parent/a", RootRel: "a"},
		{Slug: "b", RootAbs: "/parent/b", RootRel: "b"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 2})
	dec := mg.RouteActiveSet(RoutingInputs{
		LogFrameFiles: []string{"a/main.go", "b/lib.rs"},
	})
	if len(dec.Active) != 2 {
		t.Errorf("expected 2 active, got %d", len(dec.Active))
	}
	for _, slug := range dec.Active {
		if dec.Reasons[slug] != "C" {
			t.Errorf("expected reason C for %s, got %q", slug, dec.Reasons[slug])
		}
	}
}

func TestMultiGraph_RouteActiveSet_LanguageAffinity(t *testing.T) {
	build := func(root, _ string) (*rmtypes.Graph, error) {
		return makeGraph(root, []string{"x.go"}, nil), nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "a-go", RootAbs: "/parent/a", RootRel: "a", PrimaryLangs: []string{"go"}, FileCount: 50},
		{Slug: "b-rs", RootAbs: "/parent/b", RootRel: "b", PrimaryLangs: []string{"rust"}, FileCount: 50},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 1})
	dec := mg.RouteActiveSet(RoutingInputs{
		QueryLanguages: []string{"rust"},
	})
	// Channel D should pick b-rs (rust matches).
	if len(dec.Active) != 1 || dec.Active[0] != "b-rs" {
		t.Errorf("language affinity should pick b-rs, got %v", dec.Active)
	}
	if dec.Reasons["b-rs"] != "D" {
		t.Errorf("expected reason D, got %q", dec.Reasons["b-rs"])
	}
}

func TestMultiGraph_RouteActiveSet_FallbackBiggest(t *testing.T) {
	build := func(root, _ string) (*rmtypes.Graph, error) {
		return makeGraph(root, []string{"x.go"}, nil), nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "a", RootAbs: "/parent/a", RootRel: "a", FileCount: 50},
		{Slug: "b", RootAbs: "/parent/b", RootRel: "b", FileCount: 200},
		{Slug: "c", RootAbs: "/parent/c", RootRel: "c", FileCount: 100},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 2})
	// No A/B/C/D inputs — channel E (FileCount desc) picks b (200), c (100).
	dec := mg.RouteActiveSet(RoutingInputs{})
	if len(dec.Active) != 2 {
		t.Fatalf("expected 2 active, got %d", len(dec.Active))
	}
	hasB, hasC := false, false
	for _, slug := range dec.Active {
		if slug == "b" {
			hasB = true
		}
		if slug == "c" {
			hasC = true
		}
	}
	if !hasB || !hasC {
		t.Errorf("expected b+c active (biggest), got %v", dec.Active)
	}
}

func TestMultiGraph_RouteActiveSet_OverflowDrop(t *testing.T) {
	build := func(root, _ string) (*rmtypes.Graph, error) {
		return makeGraph(root, []string{"x.go"}, nil), nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "a", RootAbs: "/parent/a", RootRel: "a"},
		{Slug: "b", RootAbs: "/parent/b", RootRel: "b"},
		{Slug: "c", RootAbs: "/parent/c", RootRel: "c"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 1})
	dec := mg.RouteActiveSet(RoutingInputs{
		RequiredFiles: []string{"a/x.go", "b/x.go", "c/x.go"},
	})
	if len(dec.Active) != 1 {
		t.Errorf("cap=1 should yield 1 active, got %d", len(dec.Active))
	}
	if len(dec.OverflowDrop) != 2 {
		t.Errorf("expected 2 overflow dropped slugs, got %v", dec.OverflowDrop)
	}
	if len(dec.Inactive) != 2 {
		t.Errorf("expected 2 inactive, got %v", dec.Inactive)
	}
}

// === ImportEdges flatten ===

func TestMultiGraph_ImportEdgesPrefixed(t *testing.T) {
	gA := makeGraph("a", []string{"main.go", "util.go"}, nil) // creates main.go → util.go edge
	build := func(root, _ string) (*rmtypes.Graph, error) {
		return gA, nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureLoaded("repo-a")

	edges := mg.ImportEdges()
	if v, ok := edges["repo-a/main.go"]; !ok || len(v) != 1 || v[0] != "repo-a/util.go" {
		t.Errorf("ImportEdges = %v, want repo-a/main.go → [repo-a/util.go]", edges)
	}
}

func TestMultiGraph_TypedRelationCandidatesImportExportPrefixed(t *testing.T) {
	gA := makeGraph("a", []string{"main.go", "util.go"}, nil) // creates main.go → util.go edge
	build := func(root, _ string) (*rmtypes.Graph, error) {
		if root == "/parent/repo-a" {
			return gA, nil
		}
		return nil, fmt.Errorf("unknown root %q", root)
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureLoaded("repo-a")

	imports := mg.TypedRelationCandidates(types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationImports},
		Sources: []string{"repo-a/main.go"},
		Purpose: types.TypedRelationPurposeCoverageGate,
	})
	if len(imports) != 1 {
		t.Fatalf("imports candidates = %+v, want one", imports)
	}
	if imports[0].SourceName != "repo-a/main.go" ||
		imports[0].SourceFile != "repo-a/main.go" ||
		imports[0].Member.File != "repo-a/util.go" ||
		imports[0].Carrier != types.TypedRelationCarrierMultiGraph {
		t.Fatalf("import candidate should be path-from-parent prefixed: %+v", imports[0])
	}

	exports := mg.TypedRelationCandidates(types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationExports},
		Sources: []string{"repo-a/util.go"},
		Purpose: types.TypedRelationPurposeCoverageGate,
	})
	if len(exports) != 1 {
		t.Fatalf("exports candidates = %+v, want one", exports)
	}
	if exports[0].SourceName != "repo-a/util.go" ||
		exports[0].SourceFile != "repo-a/main.go" ||
		exports[0].Member.File != "repo-a/main.go" ||
		exports[0].Carrier != types.TypedRelationCarrierMultiGraph {
		t.Fatalf("export candidate should be path-from-parent prefixed: %+v", exports[0])
	}
}

func TestMultiGraph_TypedRelationCandidatesCalledByPrefixed(t *testing.T) {
	build := func(_, _ string) (*rmtypes.Graph, error) {
		targetFile := &rmtypes.FileInfo{RelPath: "lib/target.go", Language: rmtypes.LangGo, Package: "main"}
		targetFile.Symbols = []rmtypes.Symbol{{Name: "Target", Kind: "function", File: "lib/target.go", Line: 7}}
		callerFile := &rmtypes.FileInfo{
			RelPath:  "cmd/main.go",
			Language: rmtypes.LangGo,
			Package:  "main",
			Symbols:  []rmtypes.Symbol{{Name: "Run", Kind: "function", File: "cmd/main.go", Line: 20, EndLine: 24}},
			Relations: []rmtypes.Relation{{
				Kind: "call",
				From: "cmd/main.go",
				To:   "Target",
				File: "cmd/main.go",
				Line: 22,
				ToEP: rmtypes.RelationEndpoint{Name: "Target", File: "cmd/main.go", Line: 22},
			}},
		}
		for _, fi := range []*rmtypes.FileInfo{targetFile, callerFile} {
			for i := range fi.Symbols {
				fi.Symbols[i].ID = rmtypes.DeriveSymbolID(fi, &fi.Symbols[i])
			}
		}
		return &rmtypes.Graph{
			Files:     []*rmtypes.FileInfo{targetFile, callerFile},
			FileIndex: map[string]*rmtypes.FileInfo{targetFile.RelPath: targetFile, callerFile.RelPath: callerFile},
			SymbolDefs: map[string][]*rmtypes.Symbol{
				"Target": {&targetFile.Symbols[0]},
				"Run":    {&callerFile.Symbols[0]},
			},
			SymbolByID: map[rmtypes.SymbolID]*rmtypes.Symbol{
				targetFile.Symbols[0].ID: &targetFile.Symbols[0],
				callerFile.Symbols[0].ID: &callerFile.Symbols[0],
			},
			MethodIndex: map[rmtypes.MethodKey]*rmtypes.Symbol{
				{Pkg: "main", Receiver: "", Name: "Target"}: &targetFile.Symbols[0],
				{Pkg: "main", Receiver: "", Name: "Run"}:    &callerFile.Symbols[0],
			},
			ImportGraph:    map[string][]string{},
			ReverseImports: map[string][]string{},
		}, nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureLoaded("repo-a")

	rows := mg.TypedRelationCandidates(types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationCalledBy},
		Sources: []string{"Target"},
		Purpose: types.TypedRelationPurposeCoverageGate,
	})
	if len(rows) != 1 {
		t.Fatalf("called-by candidates = %+v, want one", rows)
	}
	if rows[0].SourceFile != "repo-a/lib/target.go" ||
		rows[0].Member.File != "repo-a/cmd/main.go" ||
		rows[0].Member.Name != "Run" ||
		rows[0].Carrier != types.TypedRelationCarrierMultiGraph {
		t.Fatalf("called-by candidate should be path-from-parent prefixed: %+v", rows[0])
	}
}

func TestMultiGraph_TypedRelationCandidatesExtendsPrefixed(t *testing.T) {
	build := func(_, _ string) (*rmtypes.Graph, error) {
		baseFile := &rmtypes.FileInfo{
			RelPath:  "lib/base.go",
			Language: rmtypes.LangJava,
			Package:  "sample",
			Symbols:  []rmtypes.Symbol{{Name: "Base", Kind: "class", File: "lib/base.go", Line: 4}},
		}
		childFile := &rmtypes.FileInfo{
			RelPath:  "impl/child.java",
			Language: rmtypes.LangJava,
			Package:  "sample",
			Symbols:  []rmtypes.Symbol{{Name: "Child", Kind: "class", File: "impl/child.java", Line: 11, EndLine: 18}},
			Relations: []rmtypes.Relation{{
				Kind: "inheritance",
				From: "impl/child.java:Child",
				To:   "Base",
				File: "impl/child.java",
				Line: 12,
				ToEP: rmtypes.RelationEndpoint{Name: "Base", File: "impl/child.java", Line: 12},
			}},
		}
		for _, fi := range []*rmtypes.FileInfo{baseFile, childFile} {
			for i := range fi.Symbols {
				fi.Symbols[i].ID = rmtypes.DeriveSymbolID(fi, &fi.Symbols[i])
			}
		}
		return &rmtypes.Graph{
			Files:     []*rmtypes.FileInfo{baseFile, childFile},
			FileIndex: map[string]*rmtypes.FileInfo{baseFile.RelPath: baseFile, childFile.RelPath: childFile},
			SymbolDefs: map[string][]*rmtypes.Symbol{
				"Base":  {&baseFile.Symbols[0]},
				"Child": {&childFile.Symbols[0]},
			},
			SymbolByID: map[rmtypes.SymbolID]*rmtypes.Symbol{
				baseFile.Symbols[0].ID:  &baseFile.Symbols[0],
				childFile.Symbols[0].ID: &childFile.Symbols[0],
			},
			ImportGraph:    map[string][]string{},
			ReverseImports: map[string][]string{},
		}, nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "repo-a", RootAbs: "/parent/repo-a", RootRel: "repo-a"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	mg.EnsureLoaded("repo-a")

	rows := mg.TypedRelationCandidates(types.TypedRelationQuery{
		Kinds:   []types.TypedRelationKind{types.TypedRelationExtends},
		Sources: []string{"Base"},
		Purpose: types.TypedRelationPurposeCoverageGate,
	})
	if len(rows) != 1 {
		t.Fatalf("extends candidates = %+v, want one", rows)
	}
	if rows[0].SourceFile != "repo-a/lib/base.go" ||
		rows[0].Member.File != "repo-a/impl/child.java" ||
		rows[0].Member.Name != "Child" ||
		rows[0].Carrier != types.TypedRelationCarrierMultiGraph ||
		rows[0].Precision != types.TypedRelationPrecisionExactSymbolID {
		t.Fatalf("extends candidate should be path-from-parent prefixed: %+v", rows[0])
	}
}

// TestMultiGraph_SubRepoNames is the F1 companion regression: the
// internal/types package needs a way to discriminate project-name
// tokens from code symbols when building RequiredMechanismAnchor.
// SubRepoNames flattens topology identifiers (Slug + RootRel +
// RootRel basename) into one set so the consumer can membership-test
// without importing the topology package.
func TestMultiGraph_SubRepoNames(t *testing.T) {
	build := func(_, _ string) (*rmtypes.Graph, error) {
		return makeGraph("g", []string{"x.go"}, nil), nil
	}
	topo := mkTopo("/parent", []topology.SubRepo{
		{Slug: "codrax-aabb", RootAbs: "/parent/codrax", RootRel: "codrax"},
		{Slug: "opencode-ccdd", RootAbs: "/parent/opencode", RootRel: "opencode"},
		{Slug: "claude-codrax-eeff", RootAbs: "/parent/claude/codrax", RootRel: "claude/codrax"},
	})
	mg, _ := New(Config{Topology: topo, Build: build, Cap: 3})
	names := mg.SubRepoNames()

	got := make(map[string]bool, len(names))
	for _, n := range names {
		got[n] = true
	}

	for _, want := range []string{
		"codrax-aabb", "codrax", // Slug + RootRel
		"opencode-ccdd", "opencode",
		"claude-codrax-eeff", "claude/codrax", // multi-segment RootRel kept
		// And basename of multi-segment RootRel is also surfaced so a
		// bare "codrax" entity matches the "claude/codrax" sub-repo.
	} {
		if !got[want] {
			t.Errorf("SubRepoNames missing %q, got %v", want, names)
		}
	}
	// Basename of "claude/codrax" === "codrax" already surfaced via
	// the active sub-repo's own RootRel, so dedup leaves a single
	// entry — no assertion needed beyond uniqueness.

	// Nil receiver returns nil.
	var nilmg *MultiGraph
	if got := nilmg.SubRepoNames(); got != nil {
		t.Errorf("nil receiver should return nil, got %v", got)
	}
}
