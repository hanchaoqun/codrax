package tracequery

// cluster_fix1_guard_pins_test.go — CLUSTER-FIX-1 load-bearing guard pins
// (修补轮, 2026-07-18; the adversarial-review probes formally adopted into the
// battery, plus the cache-discipline pins of the same round).
//
// The four load-bearing guards of freq_side_scan.go and their pins:
//
//	SameVersion generation pre-check (streamFreqSideScan)
//	    → TestClusterFix1GuardGenerationSwapSameSizeSameMtimeRefused
//	    → TestClusterFix1GuardGenerationReplacedRefused
//	singleflight (sideScanArtifactFreqCurves inflight join)
//	    → TestClusterFix1GuardSingleflightExactlyOneScan
//	cache budget eviction ring (storeLocked)
//	    → TestClusterFix1GuardCacheBudgetBounded
//	post-EOF identity re-validation (validateTraceFileIdentityAfterRead)
//	    → NO deterministic pin (备案): the guarded window opens after
//	      streamFreqSideScan's own open/pre-check and closes at its EOF stat —
//	      a deterministic mid-scan swap needs a scheduling seam the production
//	      path deliberately does not expose. The swap-DETECTION behavior
//	      itself (descriptor rewrite, pathname rebind) is exercised by the
//	      shared helper's eleven other streaming call sites and their
//	      generation tests; the swap-before-scan half is pinned here.
//
// Cache-discipline pins of this round:
//
//	true LRU (hit refreshes recency)   → TestClusterFix1CacheHitRefreshesLRUOrder
//	entry count cap (both lanes)       → TestClusterFix1CacheEntryCountBounded
//	raw-scan-content-only boundary     → TestClusterFix1ArtifactCacheStoresRawScanContentOnly
//	composite bundle success (e2e)     → TestClusterFix1CompositeBundleSideScanServes
//	affine merge-arm mapping           → TestClusterFix1CompositeSingleAffineChildMapped
//	union loop + merged-set 成本帽      → TestClusterFix1CompositeUnionCapRecheckOverflow
//	failure-verdict reuse (件7)        → TestClusterFix1ScanFailureVerdictCachedPerGeneration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

// Guard pin (缓存投毒攻击面): the Index records generation-1 identity, the
// physical file is REPLACED IN PLACE (same path, same byte size, restored
// mtime) before the side-scan lane is first consumed. The scan MUST refuse
// (scan_failed) — serving the new bytes under the old Index would be a
// mixed-generation basis. Only ctime moves; the strong identity half of the
// SameVersion pre-check is what catches it.
func TestClusterFix1GuardGenerationSwapSameSizeSameMtimeRefused(t *testing.T) {
	path := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		LineStart: 1, LineEnd: 5000, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if idx.fullFreq.collected {
		t.Fatalf("setup: line-window build must not carry in-pass curves")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Flip bytes inside the region the L1..5000 parse never retained (the
	// tail) so the swap is invisible to everything but identity discipline.
	for i := len(raw) - 4096; i < len(raw); i++ {
		if raw[i] != '\n' {
			raw[i] = 'X'
		}
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	post, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if post.Size() != info.Size() || !post.ModTime().Equal(info.ModTime()) {
		t.Fatalf("setup: swap must preserve size+mtime (size %d→%d)", info.Size(), post.Size())
	}
	basis, _ := indexClusterSampleBasis(idx)
	if basis == ClusterSampleBasisSideScan {
		t.Fatalf("POISONED: side-scan served curves for a swapped generation (basis %q, degrade %q)", basis, idx.sideFreqDegrade)
	}
	if idx.sideFreqDegrade != freqSideScanDegradeScanFailed {
		t.Fatalf("swap must degrade as scan_failed, got %q (basis %q)", idx.sideFreqDegrade, basis)
	}
}

// Guard pin: plain replacement (different size) must also refuse.
func TestClusterFix1GuardGenerationReplacedRefused(t *testing.T) {
	path := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		LineStart: 1, LineEnd: 5000, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replaced\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	basis, _ := indexClusterSampleBasis(idx)
	if basis == ClusterSampleBasisSideScan {
		t.Fatalf("POISONED: side-scan served curves for a replaced file")
	}
	if idx.sideFreqDegrade != freqSideScanDegradeScanFailed {
		t.Fatalf("replacement must degrade as scan_failed, got %q", idx.sideFreqDegrade)
	}
}

// Guard pin (并发 singleflight): N concurrent first consumers of ONE fresh
// generation must produce exactly ONE physical scan.
func TestClusterFix1GuardSingleflightExactlyOneScan(t *testing.T) {
	path := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	const n = 8
	indices := make([]*Index, n)
	for i := range indices {
		idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
			LineStart: 1, LineEnd: 3000 + 100*i, AllowWindowedParse: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		indices[i] = idx
	}
	scans0, _ := sideScanCache.counters()
	var wg sync.WaitGroup
	basisResults := make([]string, n)
	for i := range indices {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			basisResults[i], _ = indexClusterSampleBasis(indices[i])
		}(i)
	}
	wg.Wait()
	scans1, _ := sideScanCache.counters()
	if scans1-scans0 != 1 {
		t.Fatalf("singleflight: %d concurrent first consumers caused %d scans, want 1", n, scans1-scans0)
	}
	for i, basis := range basisResults {
		if basis != ClusterSampleBasisSideScan {
			t.Fatalf("consumer %d got basis %q", i, basis)
		}
	}
}

// Guard pin (预算有界): entries above the total budget still evict; a stream
// of large entries keeps the resident sample count bounded.
func TestClusterFix1GuardCacheBudgetBounded(t *testing.T) {
	c := &freqSideScanCache{
		items:    map[traceAnchorKey]*freqSideScanArtifact{},
		inflight: map[traceAnchorKey]*freqSideScanFlight{},
		errItems: map[traceAnchorKey]error{},
	}
	mk := func(i, samples int) (traceAnchorKey, *freqSideScanArtifact) {
		return traceAnchorKey{path: fmt.Sprintf("/x/%d", i), size: int64(i)},
			&freqSideScanArtifact{curves: fullFreqCurves{samples: samples}}
	}
	for i := 0; i < 10; i++ {
		k, e := mk(i, freqSideScanCacheBudgetSamples/2+1)
		c.mu.Lock()
		c.storeLocked(k, e)
		c.mu.Unlock()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) != 1 || c.samples > freqSideScanCacheBudgetSamples {
		t.Fatalf("budget must bound residents: items=%d samples=%d", len(c.items), c.samples)
	}
}

// 件2 pin (真 LRU): a cache HIT refreshes the entry's recency — through the
// REAL hit path (arm 1, wiring: the consumed key moves behind the untouched
// one), and under eviction pressure the refreshed entry survives while the
// colder one is evicted (arm 2, outcome).
func TestClusterFix1CacheHitRefreshesLRUOrder(t *testing.T) {
	// Arm 1 — wiring through sideScanArtifactFreqCurves on the global cache.
	dir := t.TempDir()
	mkTrace := func(name string) (string, traceFileIdentity) {
		p := canonicalTraceIndexPath(filepath.Join(dir, name))
		body := " app-20 (20) [000] .... 10.000000: cpu_frequency: state=1000000 cpu_id=0\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		identity, err := filegenIdentityForTest(p)
		if err != nil {
			t.Fatal(err)
		}
		return p, identity
	}
	pathA, idA := mkTrace("a.systrace")
	pathB, idB := mkTrace("b.systrace")
	if _, err := sideScanArtifactFreqCurves(pathA, idA); err != nil {
		t.Fatal(err)
	}
	if _, err := sideScanArtifactFreqCurves(pathB, idB); err != nil {
		t.Fatal(err)
	}
	if _, err := sideScanArtifactFreqCurves(pathA, idA); err != nil { // HIT on A
		t.Fatal(err)
	}
	keyA := traceAnchorKeyForIdentity(pathA, idA)
	keyB := traceAnchorKeyForIdentity(pathB, idB)
	posOf := func(key traceAnchorKey) int {
		sideScanCache.mu.Lock()
		defer sideScanCache.mu.Unlock()
		for i, k := range sideScanCache.order {
			if k == key {
				return i
			}
		}
		return -1
	}
	posA, posB := posOf(keyA), posOf(keyB)
	if posA < 0 || posB < 0 {
		t.Fatalf("setup: both entries must be resident (posA=%d posB=%d)", posA, posB)
	}
	if posA < posB {
		t.Fatalf("a cache hit must refresh LRU recency: consumed A sits at %d, colder B at %d", posA, posB)
	}

	// Arm 2 — eviction outcome on a private cache: with the hit refresh, the
	// hot entry survives eviction pressure and the cold one goes.
	c := &freqSideScanCache{
		items:    map[traceAnchorKey]*freqSideScanArtifact{},
		inflight: map[traceAnchorKey]*freqSideScanFlight{},
		errItems: map[traceAnchorKey]error{},
	}
	third := freqSideScanCacheBudgetSamples/3 + 1 // any two fit, three do not
	mk := func(name string) traceAnchorKey {
		key := traceAnchorKey{path: "/lru/" + name}
		c.mu.Lock()
		c.storeLocked(key, &freqSideScanArtifact{curves: fullFreqCurves{samples: third}})
		c.mu.Unlock()
		return key
	}
	hot := mk("hot")
	cold := mk("cold")
	c.mu.Lock()
	c.touchLocked(hot) // the hit refresh
	c.mu.Unlock()
	evictor := mk("evictor")
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[hot]; !ok {
		t.Fatalf("the refreshed (hottest) entry must survive eviction pressure")
	}
	if _, ok := c.items[cold]; ok {
		t.Fatalf("the coldest entry must be the one evicted")
	}
	if _, ok := c.items[evictor]; !ok {
		t.Fatalf("the newly stored entry must be resident")
	}
}

// 件2 pin (条目数有界): zero-cost verdicts (an overflow verdict retains no
// samples, a failure verdict retains only an error) are invisible to the
// sample budget — the entry count cap bounds both lanes, evicting oldest.
func TestClusterFix1CacheEntryCountBounded(t *testing.T) {
	c := &freqSideScanCache{
		items:    map[traceAnchorKey]*freqSideScanArtifact{},
		inflight: map[traceAnchorKey]*freqSideScanFlight{},
		errItems: map[traceAnchorKey]error{},
	}
	total := freqSideScanCacheMaxEntries + 8
	for i := 0; i < total; i++ {
		key := traceAnchorKey{path: fmt.Sprintf("/count/%d", i)}
		c.mu.Lock()
		c.storeLocked(key, &freqSideScanArtifact{overflowed: true}) // samples=0
		c.storeErrLocked(key, fmt.Errorf("verdict %d", i))
		c.mu.Unlock()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) != freqSideScanCacheMaxEntries || len(c.order) != freqSideScanCacheMaxEntries {
		t.Fatalf("curve lane must be count-bounded: items=%d order=%d cap=%d", len(c.items), len(c.order), freqSideScanCacheMaxEntries)
	}
	if len(c.errItems) != freqSideScanCacheMaxEntries || len(c.errOrder) != freqSideScanCacheMaxEntries {
		t.Fatalf("failure lane must be count-bounded: errItems=%d errOrder=%d cap=%d", len(c.errItems), len(c.errOrder), freqSideScanCacheMaxEntries)
	}
	if _, ok := c.items[traceAnchorKey{path: "/count/0"}]; ok {
		t.Fatalf("over the cap the OLDEST entry must be the one evicted")
	}
	if _, ok := c.items[traceAnchorKey{path: fmt.Sprintf("/count/%d", total-1)}]; !ok {
		t.Fatalf("the newest entry must be resident")
	}
}

// 件3 pin (裁定边界, user ruling 2026-07-18): the side-scan cache stores RAW
// scanned content only — never a derived cluster conclusion. Cross-question
// derived recomputation is per-query local by construction (chainQueryCache).
// A new field on either cached shape is a boundary event: re-affirm the
// raw-content ruling, then extend the census.
func TestClusterFix1ArtifactCacheStoresRawScanContentOnly(t *testing.T) {
	census := func(v interface{}, want []string) {
		typ := reflect.TypeOf(v)
		var got []string
		for i := 0; i < typ.NumField(); i++ {
			got = append(got, typ.Field(i).Name)
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s field census changed: got %v, want %v — the cache stores raw scan content only (user ruling 2026-07-18); derived cluster conclusions (domains, fmax, classes, R5 bases) must stay per-query local in chainQueryCache", typ.Name(), got, want)
		}
	}
	census(freqSideScanArtifact{}, []string{"curves", "overflowed"})
	// Confluence adjudication (2026-07-18, §29.129 merge with the strict CPU
	// scalar authority batch): the poison receipt fields (freqUnsafe /
	// limitUnsafe / freqAll / limitAll + their durationOrderViolation
	// witnesses) are collection-time integrity FACTS about the raw scanned
	// stream — the same nature as droppedFreqCPUs — not derived cluster
	// conclusions, so they are admissible cache content under the user
	// ruling. Any domains/fmax/class/R5 field appearing here must still turn
	// this pin red.
	census(fullFreqCurves{}, []string{
		"collected", "samples", "freqByCPU", "limitByCPU",
		"freqUnsafe", "limitUnsafe", "freqAll", "limitAll",
		"freqPoisonByCPU", "limitPoisonByCPU", "freqAllPoison",
		"limitAllPoison", "freqAllPoisonSet", "limitAllPoisonSet",
		"droppedFreqCPUs",
	})
}

// 件4 pin, arm 1 (end-to-end, the PRODUCTION-reachable composite success
// form): a systrace-only V2 bundle, line-window build — the side-scan must
// recover the full-file basis through the real provenance roster
// (TraceArtifacts + sourceIdentity), identical to the full build's in-pass
// composite curves.
//
// The richer composite forms cannot be reached through the provenance gate:
// two causally-compatible systrace children are force-isolated ("only one
// systrace causal authority", audit #40, provenance.go), and clock-alignment
// records may bind only perf children (tracebundle_provenance_v2.go) — so an
// affine or multi-child ELIGIBLE roster is impossible in a real bundle today.
// Those defensive merge arms are pinned below with fabricated rosters.
func TestClusterFix1CompositeBundleSideScanServes(t *testing.T) {
	// Two SEPARATE fixture copies = two artifact generations: the windowed
	// build must run against a COLD generation (a same-generation full build
	// would stamp the per-file anchor record and legitimately serve
	// full_index — the healthy R6 lane, not the lane under pin).
	mkBundle := func() string {
		dir := t.TempDir()
		systrace := filepath.Join(dir, "capture.systrace")
		bundle := filepath.Join(dir, "capture.tracebundle.json")
		writeBundleProvenanceFixture(t, systrace, `
 app-20 (20) [000] .... 10.000000: cpu_frequency: state=1000000 cpu_id=0
 app-20 (20) [000] .... 10.000100: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
 app-20 (20) [000] .... 10.020000: cpu_frequency: state=1200000 cpu_id=0
 app-20 (20) [001] .... 10.030000: cpu_frequency: state=800000 cpu_id=1
 app-20 (20) [001] .... 10.040000: cpu_frequency: state=900000 cpu_id=1
`)
		writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[
    {"type":"systrace","path":"capture.systrace"}
  ]
}`)
		return bundle
	}
	full, err := BuildIndex(context.Background(), mkBundle())
	if err != nil {
		t.Fatal(err)
	}
	if len(full.TraceArtifacts) != 1 || !full.TraceArtifacts[0].CausalCompatible {
		t.Fatalf("fixture setup: the child must be causally compatible, got %+v", full.TraceArtifacts)
	}
	if !full.fullFreq.collected {
		t.Fatalf("fixture setup: the full build must publish in-pass composite curves")
	}
	win, err := BuildIndexWithOptions(context.Background(), mkBundle(), BuildOptions{
		LineStart: 1, LineEnd: 2, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if win.fullFreq.collected {
		t.Fatalf("fixture setup: the line-window build must not carry in-pass curves")
	}
	if basis, _ := indexClusterSampleBasis(win); basis != ClusterSampleBasisSideScan {
		t.Fatalf("the bundle child must be recovered through the side-scan, got basis %q (degrade %q)", basis, win.sideFreqDegrade)
	}
	tlsWin := indexFreqSampleTimelines(win)
	tlsFull := indexFreqSampleTimelines(full)
	want := map[int][]freqSample{
		0: {{ts: 10.0, khz: 1000000}, {ts: 10.02, khz: 1200000}},
		1: {{ts: 10.03, khz: 800000}, {ts: 10.04, khz: 900000}},
	}
	for _, cmp := range []struct {
		label string
		got   map[int][]freqSample
	}{{"side-scan", tlsWin}, {"in-pass", tlsFull}} {
		if len(cmp.got) != len(want) {
			t.Fatalf("%s cpu set: got %v", cmp.label, cmp.got)
		}
		for cpu, samples := range want {
			if fmt.Sprint(cmp.got[cpu]) != fmt.Sprint(samples) {
				t.Fatalf("%s cpu%d full-file samples wrong: got %v, want %v", cmp.label, cpu, cmp.got[cpu], samples)
			}
		}
	}
}

// clusterFix1FabricatedChild builds one REAL physical trace file and wraps it
// in a fabricated TraceArtifactSource roster entry (件4 arms 2/3: the affine
// and multi-child ELIGIBLE rosters are unreachable through the provenance
// gate today — see TestClusterFix1CompositeBundleSideScanServes — so the
// defensive merge arms are driven with fabricated rosters over real files).
func clusterFix1FabricatedChild(t *testing.T, dir, name, body string, affineOffset *float64) TraceArtifactSource {
	t.Helper()
	p := canonicalTraceIndexPath(filepath.Join(dir, name))
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	identity, err := filegenIdentityForTest(p)
	if err != nil {
		t.Fatal(err)
	}
	source := TraceArtifactSource{
		SourcePath:          p,
		Kind:                "systrace",
		TimeDomain:          "trace_seconds",
		CanonicalTimeDomain: "trace_seconds",
		CausalCompatible:    true,
		ClockAlignment:      TraceClockAlignmentIdentity,
		sourceIdentity:      identity,
	}
	if affineOffset != nil {
		slope := 1.0
		source.ClockAlignment = TraceClockAlignmentAffine
		source.ClockCalibrated = true
		source.ClockOffsetSec = affineOffset
		source.ClockSlope = &slope
	}
	return source
}

// 件4 pin, arm 2 (single AFFINE child — the merge path, not the direct-share
// path): the scan runs in the artifact's OWN clock domain and the samples are
// affine-mapped into the canonical domain at assembly.
func TestClusterFix1CompositeSingleAffineChildMapped(t *testing.T) {
	dir := t.TempDir()
	offset := 7.0
	child := clusterFix1FabricatedChild(t, dir, "affine.systrace",
		" app-20 (20) [000] .... 2.000000: cpu_frequency: state=1000000 cpu_id=0\n"+
			" app-20 (20) [000] .... 2.250000: cpu_frequency: state=1200000 cpu_id=0\n", &offset)
	idx := &Index{TraceArtifacts: []TraceArtifactSource{child}}
	if basis, _ := indexClusterSampleBasis(idx); basis != ClusterSampleBasisSideScan {
		t.Fatalf("the affine child must serve through the side-scan merge arm, got %q (degrade %q)", basis, idx.sideFreqDegrade)
	}
	tls := indexFreqSampleTimelines(idx)
	want := []freqSample{{ts: 9.0, khz: 1000000}, {ts: 9.25, khz: 1200000}}
	if fmt.Sprint(tls[0]) != fmt.Sprint(want) {
		t.Fatalf("affine mapping must move the samples into the canonical domain (+7.0s): got %v, want %v", tls[0], want)
	}
}

// 件4 pin, arm 3 (defensive union loop + 合并后帽复检): the multi-eligible
// merge loop cannot be reached through the provenance gate today (single
// systrace causal authority), so the union Index is fabricated to drive the
// loop directly: two causally-compatible children, the second affine.
// Positive half — the union merges both children with the affine child's
// samples mapped. Cap half — an over-cap MERGED set must be the typed
// OVERFLOW verdict (not clock_unmappable: the merge folds its own over-cap
// detection into collected=false, and before this round the caller
// misreported that as a clock issue).
func TestClusterFix1CompositeUnionCapRecheckOverflow(t *testing.T) {
	dir := t.TempDir()
	offset := 5.0
	childA := clusterFix1FabricatedChild(t, dir, "a.systrace", " app-20 (20) [000] .... 1.000000: cpu_frequency: state=1000000 cpu_id=0\n", nil)
	childB := clusterFix1FabricatedChild(t, dir, "b.systrace", " app-30 (30) [001] .... 1.500000: cpu_frequency: state=800000 cpu_id=1\n", &offset)

	// Positive half: real physical scans of both children, affine-mapped union.
	union := &Index{TraceArtifacts: []TraceArtifactSource{childA, childB}}
	basis, _ := indexClusterSampleBasis(union)
	if basis != ClusterSampleBasisSideScan {
		t.Fatalf("the union side-scan must serve, got basis %q (degrade %q)", basis, union.sideFreqDegrade)
	}
	tls := indexFreqSampleTimelines(union)
	if fmt.Sprint(tls[0]) != fmt.Sprint([]freqSample{{ts: 1.0, khz: 1000000}}) {
		t.Fatalf("identity child samples wrong: %v", tls[0])
	}
	if fmt.Sprint(tls[1]) != fmt.Sprint([]freqSample{{ts: 6.5, khz: 800000}}) {
		t.Fatalf("affine child samples must be mapped (+5.0s): %v", tls[1])
	}

	// Cap half: seed both generations with cached curve sets that merge over
	// the cap — the union must degrade with the typed OVERFLOW verdict and
	// zero additional physical scans.
	overCap := func(name string, cpu int) TraceArtifactSource {
		source := clusterFix1FabricatedChild(t, dir, name, " app-20 (20) [000] .... 1.000000: cpu_frequency: state=1000000 cpu_id=0\n", nil)
		samples := make([]freqSample, freqSideScanSampleCap/2+1)
		for i := range samples {
			samples[i] = freqSample{ts: float64(i), khz: 100000}
		}
		key := traceAnchorKeyForIdentity(source.SourcePath, source.sourceIdentity)
		sideScanCache.mu.Lock()
		sideScanCache.storeLocked(key, &freqSideScanArtifact{curves: fullFreqCurves{
			collected: true,
			samples:   len(samples),
			freqByCPU: map[int][]freqSample{cpu: samples},
		}})
		sideScanCache.mu.Unlock()
		return source
	}
	bigA := overCap("big_a.systrace", 0)
	bigB := overCap("big_b.systrace", 1)
	scans0, _ := sideScanCache.counters()
	overCapUnion := &Index{TraceArtifacts: []TraceArtifactSource{bigA, bigB}}
	basisOver, _ := indexClusterSampleBasis(overCapUnion)
	if basisOver != ClusterSampleBasisWindowCarve {
		t.Fatalf("an over-cap union must fall back to window_carve, got %q", basisOver)
	}
	if overCapUnion.sideFreqDegrade != freqSideScanDegradeOverflow {
		t.Fatalf("合并后帽复检: the over-cap union must carry the typed overflow verdict, got %q", overCapUnion.sideFreqDegrade)
	}
	if scans1, _ := sideScanCache.counters(); scans1 != scans0 {
		t.Fatalf("the seeded cache must serve without physical scans (%d→%d)", scans0, scans1)
	}
}

// 件7 pin (失败判决按代际复用): a post-open scan failure is a per-generation
// verdict — cached, so a second Index of the SAME dead generation consumes
// the refusal without re-streaming the artifact. Plain open failures stay
// uncached (one cheap syscall, transient environmental classes) and ARE
// retried per Index.
func TestClusterFix1ScanFailureVerdictCachedPerGeneration(t *testing.T) {
	dir := t.TempDir()
	body := " app-20 (20) [000] .... 1.000000: cpu_frequency: state=1000000 cpu_id=0\n" +
		" app-20 (20) [000] .... 1.000100: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53\n" +
		" app-20 (20) [000] .... 1.020000: cpu_frequency: state=1200000 cpu_id=0\n" +
		" app-20 (20) [001] .... 1.030000: cpu_frequency: state=800000 cpu_id=1\n"
	path := filepath.Join(dir, "gen.systrace")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(lineEnd int) *Index {
		idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
			LineStart: 1, LineEnd: lineEnd, AllowWindowedParse: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if idx.fullFreq.collected {
			t.Fatalf("setup: the line-window build must not carry in-pass curves")
		}
		return idx
	}
	idxA1, idxA2 := build(2), build(3)
	if err := os.WriteFile(path, []byte("replaced generation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scans0, _ := sideScanCache.counters()
	if basis, _ := indexClusterSampleBasis(idxA1); basis != ClusterSampleBasisWindowCarve || idxA1.sideFreqDegrade != freqSideScanDegradeScanFailed {
		t.Fatalf("dead generation must refuse: basis=%q degrade=%q", basis, idxA1.sideFreqDegrade)
	}
	scans1, hits1 := sideScanCache.counters()
	if scans1-scans0 != 1 {
		t.Fatalf("first consumer must pay exactly one scan attempt, got %d", scans1-scans0)
	}
	if basis, _ := indexClusterSampleBasis(idxA2); basis != ClusterSampleBasisWindowCarve || idxA2.sideFreqDegrade != freqSideScanDegradeScanFailed {
		t.Fatalf("second index of the dead generation must refuse identically: basis=%q degrade=%q", basis, idxA2.sideFreqDegrade)
	}
	scans2, hits2 := sideScanCache.counters()
	if scans2 != scans1 {
		t.Fatalf("the cached failure verdict must not re-stream the artifact (scans %d→%d)", scans1, scans2)
	}
	if hits2 <= hits1 {
		t.Fatalf("failure-verdict reuse must be observable as a hit (hits %d→%d)", hits1, hits2)
	}

	// Open failures are NOT cached: two indexes of a deleted file each pay
	// their own (cheap) attempt.
	pathGone := filepath.Join(dir, "gone.systrace")
	if err := os.WriteFile(pathGone, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idxB1, idxB2 := func() (*Index, *Index) {
		mk := func(lineEnd int) *Index {
			idx, err := BuildIndexWithOptions(context.Background(), pathGone, BuildOptions{
				LineStart: 1, LineEnd: lineEnd, AllowWindowedParse: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			return idx
		}
		return mk(2), mk(3)
	}()
	if err := os.Remove(pathGone); err != nil {
		t.Fatal(err)
	}
	scans3, _ := sideScanCache.counters()
	if basis, _ := indexClusterSampleBasis(idxB1); basis != ClusterSampleBasisWindowCarve || idxB1.sideFreqDegrade != freqSideScanDegradeScanFailed {
		t.Fatalf("deleted artifact must refuse: basis=%q degrade=%q", basis, idxB1.sideFreqDegrade)
	}
	if basis, _ := indexClusterSampleBasis(idxB2); basis != ClusterSampleBasisWindowCarve || idxB2.sideFreqDegrade != freqSideScanDegradeScanFailed {
		t.Fatalf("deleted artifact must refuse for every index: basis=%q degrade=%q", basis, idxB2.sideFreqDegrade)
	}
	if scans4, _ := sideScanCache.counters(); scans4-scans3 != 2 {
		t.Fatalf("open failures must stay uncached (each index retries the cheap attempt), got %d extra scans", scans4-scans3)
	}
}
