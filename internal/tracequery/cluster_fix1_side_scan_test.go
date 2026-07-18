package tracequery

// cluster_fix1_side_scan_test.go — CLUSTER-FIX-1 acceptance pins (user ruling
// 2026-07-18: 大 trace 全文件基丢失修根 — a second bounded streaming full-file
// side-scan is ALLOWED, cost-capped and cached per artifact generation;
// freq_side_scan.go).
//
// Disease record (engine-real, measured on main=fec890839 before this batch —
// every number is a MEASURED pre-fix verdict, never an edit target):
//
//	donghu L1..5000    → 2 domains, middle [4-11] crowned big (cap 2.53),
//	                     global fmax 1280000 (true 2750000, 2.15× under),
//	                     ZERO degrade words (silent misprice);
//	donghu L1..1000    → 1 domain → freq_only, R5 basis 920000 (3× under);
//	donghu L3200..7000 → 2 domains, middle crowned big @1880000;
//	donghu L8000..13000→ 3 domains by luck, fmax under (big 2340000);
//	donghu budget window (MaxEvents=4000, padding-truncated) → 2 domains with
//	                     an IN-WINDOW FMAX INVERSION: the small cluster [0-3]
//	                     @1040000 crowned big, the middle cluster [4-11]
//	                     @640000 crowned small — misprice, zero degrade words.
//
// Ground truth (user 勘正 2026-07-14): [0-3]小/[4-11]中/[12,13]大,
// fmax 1720000/2270000/2750000, R5 basis 2750000/big/2.53.
//
// Fixture discipline: each pin copies the committed real capture to a temp
// path — a FRESH file generation, so the per-file anchor record and the
// side-scan cache are provably cold for that pin (no cross-test warm-record
// masking).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clusterFix1CopyFixture(t *testing.T, fixture string) string {
	t.Helper()
	src, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), filepath.Base(fixture))
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func clusterFix1AssertDonghuTruth(t *testing.T, label string, idx *Index) {
	t.Helper()
	if basis, _ := indexClusterSampleBasis(idx); basis != ClusterSampleBasisSideScan {
		t.Fatalf("%s: basis must be the typed side_scan token, got %q", label, basis)
	}
	capability := indexDerivedCoreCapability(idx)
	if !capability.usable() {
		t.Fatalf("%s: donghu must judge three clusters, got %q", label, capability.source)
	}
	wantClass := map[string][]int{
		"small":  {0, 1, 2, 3},
		"middle": {4, 5, 6, 7, 8, 9, 10, 11},
		"big":    {12, 13},
	}
	wantFmax := map[string]int64{"small": 1720000, "middle": 2270000, "big": 2750000}
	for class, cpus := range wantClass {
		clusterLabel, ok := capability.classClusterLabel(class)
		if !ok {
			t.Fatalf("%s: class %s missing: %+v", label, class, capability.classByCluster)
		}
		members := capability.domains.members[clusterLabel]
		if fmt.Sprint(members) != fmt.Sprint(cpus) {
			t.Fatalf("%s: class %s members = %v, want %v", label, class, members, cpus)
		}
		if got := capability.fmaxByCluster[clusterLabel]; got != wantFmax[class] {
			t.Fatalf("%s: class %s fmax = %d, want %d", label, class, got, wantFmax[class])
		}
	}
	cache := newChainQueryCache(idx, nil)
	fm, refCap, refClass := cache.supplyFoldGlobalMaxBasis(cache.coreCapability(""))
	if fm.khz != 2750000 || refClass != "big" || refCap != coreCapabilityDefaultBig {
		t.Fatalf("%s: R5 basis must be (2750000, big, 2.53), got %d/%q/%v", label, fm.khz, refClass, refCap)
	}
}

// Pin ① (行窗病形双向, heal side): the audited L1..5000 disease window — the
// line gate stops the main pass mid-file (collected=false), and the side-scan
// restores the FULL-FILE three-cluster truth under the typed side_scan token.
// The pre-fix verdict on this exact build was the silent misprice recorded in
// the header. CAP-3 red line: the side-scan basis holds cpu12/13's complete
// 85-sample lanes although every one of their physical lines (first burst
// L7384) sits OUTSIDE the line window — the basis is Index-global, never a
// window crop.
func TestClusterFix1LineWindowDiseaseHealed(t *testing.T) {
	path := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		LineStart: 1, LineEnd: 5000, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if idx.fullFreq.collected {
		t.Fatalf("fixture setup: a line-window build must not carry in-pass full curves")
	}
	clusterFix1AssertDonghuTruth(t, "L1..5000", idx)
	tls := indexFreqSampleTimelines(idx)
	if len(tls[12]) != 85 || len(tls[13]) != 85 {
		t.Fatalf("CAP-3: the side-scan basis must hold the full 85-sample big-cluster lanes (lines outside the window), got %d/%d", len(tls[12]), len(tls[13]))
	}
}

// Pin ② (窗扫对照, representative windows): the three measured pre-fix carve
// failure arms — single-cluster freq_only (L1..1000), middle-crowned-big
// misclass (L3200..7000), lucky-3-but-fmax-under (L8000..13000) — all judge
// the same full-file truth after the fix. (The 10ms sweep quantification
// rides the batch report; these are its three distinct failure-arm
// representatives.)
func TestClusterFix1RepresentativeCarveWindowsHealed(t *testing.T) {
	for _, r := range [][2]int{{1, 1000}, {3200, 7000}, {8000, 13000}} {
		path := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace")
		idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
			LineStart: r[0], LineEnd: r[1], AllowWindowedParse: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		clusterFix1AssertDonghuTruth(t, fmt.Sprintf("L%d..%d", r[0], r[1]), idx)
	}
}

// Pin ⑥ (MaxEvents 超限形): the budget-truncated windowed build (the large-
// trace shape where a full-file build is impossible and windows are the only
// entrance) — pre-fix this exact build minted the fmax-INVERSION misprice
// (small [0-3]@1040000 crowned big over middle@640000). The side-scan serves
// the cluster basis regardless of the event budget.
func TestClusterFix1BudgetTruncatedWindowHealed(t *testing.T) {
	path := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 13762.795, TimeEnd: 13762.805, TimeStartSet: true, TimeEndSet: true,
		TimePaddingBefore: 0.5, TimePaddingAfter: 0.5, AllowWindowedParse: true, MaxEvents: 4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.PaddingTruncated {
		t.Fatalf("fixture setup: the event budget must trip in the padding tail")
	}
	if idx.fullFreq.collected {
		t.Fatalf("fixture setup: the truncated pass must not carry in-pass full curves")
	}
	clusterFix1AssertDonghuTruth(t, "budget-window", idx)
}

// Pin ③ (单簇负臂): the tieba systrace form — only cpu3-5 carry
// cpu_frequency (single sampled cluster). The side-scan serves the basis but
// CANNOT and MUST NOT invent structure: the verdict stays the honest
// freq_only, semantically identical to the full-file build's (same single
// domain [0..5], same class-less R5 basis 2189000 at cap 1).
func TestClusterFix1SingleClusterFormStaysHonest(t *testing.T) {
	path := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		LineStart: 1, LineEnd: 4000, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if basis, _ := indexClusterSampleBasis(idx); basis != ClusterSampleBasisSideScan {
		t.Fatalf("tieba line window must ride the side-scan basis, got %q", basis)
	}
	d := deriveClusterFreqDomains(indexFreqSampleTimelines(idx))
	if d.groupCount != 1 {
		t.Fatalf("side-scan must not invent structure: one sampled cluster, got %d (%v)", d.groupCount, d.members)
	}
	if got := fmt.Sprint(d.members[d.byCPU[3]]); got != "[0 1 2 3 4 5]" {
		t.Fatalf("规则1 closure must match the full-file verdict, got %s", got)
	}
	capability := indexDerivedCoreCapability(idx)
	if capability.usable() {
		t.Fatalf("single cluster must stay freq_only (no class fabrication), got %q", capability.source)
	}
	cache := newChainQueryCache(idx, nil)
	fm, refCap, refClass := cache.supplyFoldGlobalMaxBasis(cache.coreCapability(""))
	if fm.khz != 2189000 || refCap != 1 || refClass != "" {
		t.Fatalf("tieba R5 basis must stay (2189000, cap 1, class-less), got %d/%v/%q", fm.khz, refCap, refClass)
	}
}

// Pin ④ (复用): one artifact generation is scanned EXACTLY once — the second
// (different-window) build of the same copy consumes the cached outcome, and
// the reuse is observable on the typed counters (用户裁定重点: 扫描到的内容
// 要能复用,不要反复重复扫描).
func TestClusterFix1SideScanReuseNoRescan(t *testing.T) {
	path := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	build := func(lineEnd int) *Index {
		idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
			LineStart: 1, LineEnd: lineEnd, AllowWindowedParse: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return idx
	}
	scans0, _ := sideScanCache.counters()
	idxA := build(5000)
	clusterFix1AssertDonghuTruth(t, "reuse-A", idxA)
	scans1, hits1 := sideScanCache.counters()
	if scans1-scans0 != 1 {
		t.Fatalf("first consumption must scan exactly once, got %d", scans1-scans0)
	}
	idxB := build(2000)
	clusterFix1AssertDonghuTruth(t, "reuse-B", idxB)
	scans2, hits2 := sideScanCache.counters()
	if scans2 != scans1 {
		t.Fatalf("second build of the same generation must NOT re-scan (scans %d→%d)", scans1, scans2)
	}
	if hits2 <= hits1 {
		t.Fatalf("the reuse must be observable as a cache hit (hits %d→%d)", hits1, hits2)
	}
}

// Pin ⑤ (成本帽 + 超帽披露): the sample cap is enforced (small-cap scan →
// typed overflow verdict, no curve set), the cached verdict is never
// re-scanned, and the degrade reaches the caveat lane honestly — the basis
// stays window_carve, the freq_only degrade words unchanged.
func TestClusterFix1SampleCapOverflowDisclosed(t *testing.T) {
	// The seed key must address the exact canonical spelling the built
	// artifact will record (EvalSymlinks folds /var → /private/var on darwin).
	path := canonicalTraceIndexPath(clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace"))
	identity, err := filegenIdentityForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	// (a) cap enforcement, driven directly with a tiny cap.
	entry, err := streamFreqSideScan(path, identity, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !entry.overflowed || entry.curves.collected {
		t.Fatalf("a tripped cost cap must yield the typed overflow verdict, got %+v", entry)
	}
	// (b) the cached verdict degrades consumption honestly (no rescan): seed
	// the cache with the overflow verdict for this generation, then consume.
	key := traceAnchorKeyForIdentity(path, identity)
	sideScanCache.mu.Lock()
	sideScanCache.storeLocked(key, entry)
	sideScanCache.mu.Unlock()
	scans0, _ := sideScanCache.counters()
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		LineStart: 1, LineEnd: 5000, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	basis, _ := indexClusterSampleBasis(idx)
	if basis != ClusterSampleBasisWindowCarve {
		t.Fatalf("an over-cap side-scan must fall back to window_carve, got %q", basis)
	}
	if idx.sideFreqDegrade != freqSideScanDegradeOverflow {
		t.Fatalf("the degrade token must be the typed overflow, got %q", idx.sideFreqDegrade)
	}
	if scans1, _ := sideScanCache.counters(); scans1 != scans0 {
		t.Fatalf("the cached overflow verdict must not trigger a re-scan (%d→%d)", scans0, scans1)
	}
	caveats := resultCaveats(idx, Query{}, Result{})
	found := false
	for _, caveat := range caveats {
		if strings.Contains(caveat, "cluster_freq_side_scan_degraded=sample_cap_overflow") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the over-cap degrade must be disclosed on the caveat lane, got %v", caveats)
	}
	// The degrade wording stays a disclosure: the capability verdict itself
	// keeps the pre-fix carve behavior (this window judges 2 clusters on the
	// carve — the misprice the DISCLOSURE now makes visible; the ruling's
	// heal lane is the non-overflow side-scan, pinned above).
	if capability := indexDerivedCoreCapability(idx); len(capability.domains.members) == 3 {
		t.Fatalf("overflow degrade must stay on the carve basis (2-domain carve shape), got %+v", capability.domains.members)
	}
}

// Pin ⑦ (既有全量绿形负臂): a complete from-0 build keeps the R6 in-pass
// full_index basis — the side-scan lane never runs, and the fold basis wire
// stays byte-identical (token disclosed by absence).
func TestClusterFix1FullBuildKeepsFullIndexBasis(t *testing.T) {
	path := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	scans0, _ := sideScanCache.counters()
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if basis, dropped := indexClusterSampleBasis(idx); basis != ClusterSampleBasisFullIndex || len(dropped) != 0 {
		t.Fatalf("a complete build must keep the full_index basis with no drops, got %q/%v", basis, dropped)
	}
	if scans1, _ := sideScanCache.counters(); scans1 != scans0 {
		t.Fatalf("the side-scan lane must not run on a full_index build (%d→%d)", scans0, scans1)
	}
	cache := newChainQueryCache(idx, nil)
	q := Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898}
	_, basis := cache.supplyFoldRunningIntervals(q, q.TimeStart, q.TimeEnd, []Interval{
		{State: StateRunning, StartTs: 13762.90, EndTs: 13762.91, DurationMs: 10, CPU: 12, CPUKnown: true},
	})
	if basis.ClusterSampleBasis != "" || len(basis.ClusterFreqIntegrityDroppedCPUs) != 0 {
		t.Fatalf("full_index must be disclosed by ABSENCE on the fold basis (wire bytes preserved), got %q/%v",
			basis.ClusterSampleBasis, basis.ClusterFreqIntegrityDroppedCPUs)
	}
	// The side_scan token DOES ride the fold basis on a carve-recovered build
	// (the positive arm of the same disclosure contract).
	pathB := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	idxB, err := BuildIndexWithOptions(context.Background(), pathB, BuildOptions{
		LineStart: 1, LineEnd: 5000, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	cacheB := newChainQueryCache(idxB, nil)
	_, basisB := cacheB.supplyFoldRunningIntervals(q, q.TimeStart, q.TimeEnd, []Interval{
		{State: StateRunning, StartTs: 13762.90, EndTs: 13762.91, DurationMs: 10, CPU: 12, CPUKnown: true},
	})
	if basisB.ClusterSampleBasis != ClusterSampleBasisSideScan {
		t.Fatalf("the side_scan token must ride the fold basis on a recovered build, got %q", basisB.ClusterSampleBasis)
	}
}

// Pin ⑧ (S4 收披露): a physical same-lane timestamp rollback still drops the
// poisoned cpu_frequency lane (judgment unchanged — the long-standing
// fail-close), and the drop is now DISCLOSED: the typed roster reaches the
// caveat lane and the fold basis, on both the full_index and the carve arms.
func TestClusterFix1IntegrityDropDisclosed(t *testing.T) {
	trace := `
   probe-10  (   10) [000] .... 1.000000: cpu_frequency: state=800000 cpu_id=0
   probe-10  (   10) [000] .... 1.000100: cpu_frequency: state=1000000 cpu_id=1
   probe-10  (   10) [000] .... 2.000000: cpu_frequency: state=900000 cpu_id=0
   probe-10  (   10) [000] .... 2.000100: cpu_frequency: state=1100000 cpu_id=1
   probe-10  (   10) [000] .... 1.500000: cpu_frequency: state=950000 cpu_id=0
   probe-10  (   10) [000] .... 3.000000: sched_switch: prev_comm=probe prev_pid=10 prev_prio=100 prev_state=S ==> next_comm=app next_pid=100 next_prio=100
`
	path := filepath.Join(t.TempDir(), "rollback.systrace")
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !idx.fullFreq.collected {
		t.Fatalf("fixture setup: the complete build must publish full curves")
	}
	basis, dropped := indexClusterSampleBasis(idx)
	if basis != ClusterSampleBasisFullIndex || fmt.Sprint(dropped) != "[0]" {
		t.Fatalf("the poisoned cpu0 lane must be disclosed as dropped on the full_index basis, got %q/%v", basis, dropped)
	}
	tls := indexFreqSampleTimelines(idx)
	if len(tls[0]) != 0 || len(tls[1]) == 0 {
		t.Fatalf("the drop judgment itself must stay unchanged (cpu0 out, cpu1 kept): %v", tls)
	}
	caveats := resultCaveats(idx, Query{}, Result{})
	found := false
	for _, caveat := range caveats {
		if strings.Contains(caveat, "cluster_freq_integrity_dropped_cpus=cpu0") && strings.Contains(caveat, "簇计数可能被低估") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the S4 drop must be disclosed on the caveat lane, got %v", caveats)
	}
	// Carve arm: the same disclosure on an events-basis synthetic index.
	carve := &Index{Events: []Event{
		{Type: EventCPUFrequency, Name: "cpu_frequency", Ts: 1.0, Frequency: 800000, CPUForField: 0, CPUForFieldValid: true},
		{Type: EventCPUFrequency, Name: "cpu_frequency", Ts: 2.0, Frequency: 900000, CPUForField: 0, CPUForFieldValid: true},
		{Type: EventCPUFrequency, Name: "cpu_frequency", Ts: 1.5, Frequency: 950000, CPUForField: 0, CPUForFieldValid: true},
		{Type: EventCPUFrequency, Name: "cpu_frequency", Ts: 1.0, Frequency: 1000000, CPUForField: 1, CPUForFieldValid: true},
	}}
	basisC, droppedC := indexClusterSampleBasis(carve)
	if basisC != ClusterSampleBasisWindowCarve || fmt.Sprint(droppedC) != "[0]" {
		t.Fatalf("the carve arm must disclose the dropped lane too, got %q/%v", basisC, droppedC)
	}
}

// Pin ⑦b (采集等价): the side-scan collection is sample-for-sample the set a
// complete in-pass build collects — same admission predicates, same rollback
// audit, same physical order. The two lanes may differ only in their typed
// basis token, never in a value.
func TestClusterFix1SideScanSampleIdenticalToInPass(t *testing.T) {
	path := clusterFix1CopyFixture(t, "../../eval/fixtures/real_traces/donghu.ftrace")
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !idx.fullFreq.collected {
		t.Fatalf("fixture setup: the complete build must publish in-pass curves")
	}
	identity, err := filegenIdentityForTest(canonicalTraceIndexPath(path))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := streamFreqSideScan(canonicalTraceIndexPath(path), identity, freqSideScanSampleCap)
	if err != nil {
		t.Fatal(err)
	}
	if entry.overflowed || !entry.curves.collected {
		t.Fatalf("side-scan must serve the donghu curves, got %+v", entry)
	}
	if entry.curves.samples != idx.fullFreq.samples {
		t.Fatalf("sample-identity: side-scan %d vs in-pass %d", entry.curves.samples, idx.fullFreq.samples)
	}
	for _, lanes := range []struct {
		name       string
		side, full map[int][]freqSample
	}{
		{"freq", entry.curves.freqByCPU, idx.fullFreq.freqByCPU},
		{"limits", entry.curves.limitByCPU, idx.fullFreq.limitByCPU},
	} {
		if len(lanes.side) != len(lanes.full) {
			t.Fatalf("%s lane cpu set differs: %d vs %d", lanes.name, len(lanes.side), len(lanes.full))
		}
		for cpu, full := range lanes.full {
			side := lanes.side[cpu]
			if len(side) != len(full) {
				t.Fatalf("%s cpu%d length differs: %d vs %d", lanes.name, cpu, len(side), len(full))
			}
			for i := range full {
				if side[i] != full[i] {
					t.Fatalf("%s cpu%d[%d] differs: %+v vs %+v", lanes.name, cpu, i, side[i], full[i])
				}
			}
		}
	}
}

// Pin ⑨ (composite perf parity 负臂): a bundle carrying an admitted perf-kind
// child never publishes in-pass composite curves (parse.go merge rule — the
// perf event set passes a typed admission the side curves cannot), and the
// side-scan must NOT be more permissive: it refuses with the typed
// perf_artifact_present degrade, the basis stays window_carve, and the caveat
// lane discloses the refusal.
func TestClusterFix1CompositePerfChildParityRefusal(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "capture.systrace")
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `
 app-20 (20) [001] .... 10.000000: cpu_frequency: state=1000000 cpu_id=1
 app-20 (20) [001] .... 10.000100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
`)
	writeBundleProvenanceFixture(t, perftrace, `
 app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=9000 event=cpu-cycles symbol=App::draw dso=libapp.so source=test
`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[
    {"type":"systrace","path":"capture.systrace"},
    {"type":"perftrace","path":"capture.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"capture.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}
  ]
}`)
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if idx.fullFreq.collected {
		t.Fatalf("a perf-child composite must not publish in-pass curves")
	}
	if basis, _ := indexClusterSampleBasis(idx); basis != ClusterSampleBasisWindowCarve {
		t.Fatalf("the side-scan must not out-permit the in-pass composite merge, got basis %q", basis)
	}
	if idx.sideFreqDegrade != freqSideScanDegradePerfChild {
		t.Fatalf("the refusal must carry the typed perf parity token, got %q", idx.sideFreqDegrade)
	}
	caveats := resultCaveats(idx, Query{}, Result{})
	found := false
	for _, caveat := range caveats {
		if strings.Contains(caveat, "cluster_freq_side_scan_degraded=perf_artifact_present") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the composite refusal must be disclosed on the caveat lane, got %v", caveats)
	}
}

func filegenIdentityForTest(path string) (traceFileIdentity, error) {
	f, identity, err := openTraceSourceRegular(path)
	if err != nil {
		return traceFileIdentity{}, err
	}
	return identity, f.Close()
}
