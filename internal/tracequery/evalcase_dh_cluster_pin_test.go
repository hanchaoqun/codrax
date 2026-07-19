package tracequery

// evalcase_dh_cluster_pin_test.go — EVALCASE-DH batch, cluster/thermal family
// engine pins on the two committed REAL fixtures (mining ledger:
// scratchpad evalcase_donghu_mining.md + docs/design/cluster_audit_trace_20260718.md;
// expectations re-collected at HEAD 1ada2c49f, hand-cross-checked against raw
// events).
//
// Cases:
//
//	DH-F1  三簇跨档对齐正例 + 行窗错类负例 — donghu full-file R6 derivation
//	       yields the three-tier truth; a physical L1..5000 line carve
//	       derives TWO domains, misclasses the middle cluster as "big"
//	       (cap 2.53 impersonation) and underestimates the global fmax
//	       2.15× — the customer 「经常判断失败」 mechanical-cause family,
//	       pinned as the documented trap shape.
//	DH-T1  limits 热帽 — cpu0 cluster fmax comes from the LIMITS lane
//	       (1720000) sitting ABOVE the highest observed sample (1530000);
//	       the in-window limits face reports the governing 1530000 max and
//	       the oscillation count without rewriting observed samples.
//	DH-T2  thermal 镜像 Tier-2 空转 — thermal_inte1/2 (the only rails that
//	       truly mirror cluster frequencies) are excluded by rail gate ⑥,
//	       the l3c family has <2 members, so rail adoption is NONE with an
//	       EMPTY rejected set (never a fabricated rail fallback).
//
// Fixture red line: the traces are REAL captures — every number below is a
// measured pin, never an edit target.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const evalcaseDonghuFixture = "../../eval/fixtures/real_traces/donghu.ftrace"
const evalcaseTiebaFixture = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"

func evalcaseIndex(t *testing.T, path string) *Index {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("real fixture not present: %v", err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("BuildIndex(%s): %v", path, err)
	}
	return idx
}

// DH-F1 正例: the full-file basis derives the three-tier ground truth with
// the default capability table usable (零降级词 at the derivation face) and
// the ladder caps 1 / 2.3 / 2.53.
func TestEvalcaseDHF1FullFileThreeTierAlignment(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	capability := indexDerivedCoreCapability(idx)
	if !capability.usable() || capability.source != "default_table" {
		t.Fatalf("DH-F1 正例: full-file capability must be usable default_table, got usable=%v source=%q", capability.usable(), capability.source)
	}
	wantFmax := map[string]int64{"small": 1720000, "middle": 2270000, "big": 2750000}
	wantCap := map[string]float64{"small": coreCapabilityDefaultSmall, "middle": coreCapabilityDefaultMiddle, "big": coreCapabilityDefaultBig}
	for class, fmax := range wantFmax {
		label, ok := capability.classClusterLabel(class)
		if !ok {
			t.Fatalf("DH-F1 正例: class %s missing: %+v", class, capability.classByCluster)
		}
		if got := capability.fmaxByCluster[label]; got != fmax {
			t.Fatalf("DH-F1 正例: class %s fmax=%d want %d", class, got, fmax)
		}
		if got := coreCapabilityDefaultByClass[class]; got != wantCap[class] {
			t.Fatalf("DH-F1 正例: class %s cap=%v want %v", class, got, wantCap[class])
		}
	}
}

// DH-F1 负例: a physical carve of the first 5000 lines (13762.791708..
// 13762.842408 — the customer's "short window" build shape) derives only TWO
// domains: the big cluster (cpu12/13) has no samples yet and vanishes, the
// REAL middle cluster [4..11] is misclassed "big" (cap 2.53 impersonation),
// and the carve-global fmax tops out at 1280000 — 2.15× below the trace
// truth 2750000. Documented trap shape: any consumer folding on a line-window
// basis inherits exactly this misclass family.
func TestEvalcaseDHF1LineCarveMisclassNegative(t *testing.T) {
	body, err := os.ReadFile(evalcaseDonghuFixture)
	if err != nil {
		t.Skipf("real fixture not present: %v", err)
	}
	all := strings.SplitAfter(string(body), "\n")
	if len(all) < 5000 {
		t.Fatalf("fixture drifted: fewer than 5000 lines (%d)", len(all))
	}
	carved := filepath.Join(t.TempDir(), "donghu_l1_5000.ftrace")
	if err := os.WriteFile(carved, []byte(strings.Join(all[:5000], "")), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), carved)
	if err != nil {
		t.Fatal(err)
	}
	d := deriveClusterFreqDomains(indexFreqSampleTimelines(idx))
	if d.groupCount != 2 {
		t.Fatalf("DH-F1 负例: carve must derive TWO domains (big cluster unsampled), got %d (%+v)", d.groupCount, d.members)
	}
	classes := indexDerivedCoreClassByCPU(idx)
	for _, cpu := range []int{4, 5, 6, 7, 8, 9, 10, 11} {
		if classes[cpu] != "big" {
			t.Fatalf("DH-F1 负例: carve misclass drifted — cpu%d classed %q (trap shape says the middle cluster impersonates big)", cpu, classes[cpu])
		}
	}
	if _, ok := classes[12]; ok {
		t.Fatalf("DH-F1 负例: cpu12 must be absent from the carve derivation (no samples in L1..5000)")
	}
	capability := indexDerivedCoreCapability(idx)
	var carveMax int64
	for _, fmax := range capability.fmaxByCluster {
		if fmax > carveMax {
			carveMax = fmax
		}
	}
	if carveMax != 1280000 {
		t.Fatalf("DH-F1 负例: carve-global fmax=%d want 1280000 (2.15× underestimate of 2750000)", carveMax)
	}
	if ratio := 2750000.0 / float64(carveMax); ratio < 2.14 || ratio > 2.16 {
		t.Fatalf("DH-F1 负例: underestimate ratio drifted: %.3f want ≈2.148", ratio)
	}
}

// DH-T1: the small cluster's trace-global fmax is the LIMITS-lane ceiling
// 1720000 — strictly above its highest observed cpu_frequency sample lane
// (the in-window governing limits row says max=1530000) — and the ceiling
// face declares Source="limit" while the middle/big ceilings in the same
// window stay "observed". Limits oscillation must never be read as measured
// frequency samples.
func TestEvalcaseDHT1LimitsLiftedFmax(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	q := normalizeQuery(idx, Query{PID: 17267, TimeStart: 13762.860, TimeEnd: 13762.870})
	stats := ComputeWindowStats(idx, q)
	// In-window limits face: the cpu0 lane oscillates (count=6 rows in this
	// 10ms window) with the governing window max 1530000 / min 418000.
	var cpu0 *CPUFrequencyLimit
	for i := range stats.CPUFrequencyLimits {
		if stats.CPUFrequencyLimits[i].CPU == 0 {
			cpu0 = &stats.CPUFrequencyLimits[i]
		}
	}
	if cpu0 == nil {
		t.Fatalf("DH-T1: cpu0 limits lane missing from the window face: %+v", stats.CPUFrequencyLimits)
	}
	if cpu0.MaxFrequency != 1530000 || cpu0.MinFrequency != 418000 || cpu0.Count != 6 {
		t.Fatalf("DH-T1: cpu0 in-window limits drifted: max=%d min=%d count=%d (want 1530000/418000/6)", cpu0.MaxFrequency, cpu0.MinFrequency, cpu0.Count)
	}
	// Ceiling face: small=1720000 sourced from the limits lane (the thermal
	// cap lift above every observed sample), middle/big observed.
	bySource := map[string]ClusterFrequencyCeiling{}
	for _, c := range stats.ClusterFrequencyCeilings {
		bySource[c.CoreClass] = c
	}
	small, ok := bySource["small"]
	if !ok || small.FmaxKHz != 1720000 || small.Source != SupplyFoldFmaxSourceLimit {
		t.Fatalf("DH-T1: small ceiling must be 1720000 Source=limit, got %+v", bySource)
	}
	for _, class := range []string{"middle", "big"} {
		c, ok := bySource[class]
		if !ok || c.Source != SupplyFoldFmaxSourceObserved {
			t.Fatalf("DH-T1: %s ceiling must stay observed in this window, got %+v", class, c)
		}
	}
	// Supply signal in this window carries no low-frequency word (the target
	// runs on middle/big cores at busy frequencies).
	if stats.SupplyPressureSummary == nil || stats.SupplyPressureSummary.Signal != "cpu_pressure" {
		t.Fatalf("DH-T1: supply signal must be plain cpu_pressure, got %+v", stats.SupplyPressureSummary)
	}
	if len(stats.SupplyPressureSummary.LowFrequencyCPUs) != 0 {
		t.Fatalf("DH-T1: no low_frequency_cpus expected in this window, got %v", stats.SupplyPressureSummary.LowFrequencyCPUs)
	}
	// Target running account (hand-recomputable from raw sched_switch):
	// 4.174ms@cpu8(middle) + 3.840ms@cpu12(big); the third fragment
	// (0.172ms@cpu4) sits below the top-8 display cap.
	got := map[int]float64{}
	for _, r := range stats.TopRunning {
		if r.Thread.PID == 17267 {
			got[r.CPU] = r.DurationMs
		}
	}
	if !near(got[8], 4.174, 0.001) || !near(got[12], 3.840, 0.001) {
		t.Fatalf("DH-T1: 17267 per-cpu running drifted: %v (want cpu8=4.174 cpu12=3.840)", got)
	}
}

// DH-T2: rail adoption is NONE on donghu — the thermal mirrors (the only
// name families that truly carry cluster frequencies) are excluded by the
// rail gate ⑥ vocabulary, and the surviving l3c family has fewer than two
// members so it never becomes a candidate (empty rejected set, no reason
// entry). Honest Tier-2 idleness: absolutely no rail fallback is fabricated.
func TestEvalcaseDHT2RailAdoptionNone(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	cache := newChainQueryCache(idx, nil)
	scan := scanClusterRailEvidence(idx.Events, cache.schedObservedCPUs())
	if scan.adoption != nil {
		t.Fatalf("DH-T2: rail adoption must be NONE, got %+v", scan.adoption)
	}
	if len(scan.rejected) != 0 {
		t.Fatalf("DH-T2: rejected set must be EMPTY (l3c family <2 members never enters candidacy), got %v", scan.rejected)
	}
	for _, name := range []string{"thermal_inte1", "thermal_inte2"} {
		if !clusterRailNameExcluded(name) {
			t.Fatalf("DH-T2: %s must be excluded by rail gate ⑥ (thermal vocabulary)", name)
		}
	}
	if clusterRailNameExcluded("l3c_cluster2_freq") {
		t.Fatalf("DH-T2: l3c_cluster2_freq must NOT be vocabulary-excluded (it fails the ≥2-member family gate instead)")
	}
	// The supply face proves the mirrors never leak into the pressure buckets:
	// thermal=0 while ddr/l3 count their own lanes (J1 window witness).
	q := normalizeQuery(idx, Query{PID: 17267, TimeStart: 13762.9374, TimeEnd: 13762.9736})
	stats := ComputeWindowStats(idx, q)
	if s := stats.SupplyPressureSummary; s == nil || s.ThermalEventCount != 0 || s.DDREventCount != 57 || s.L3EventCount != 19 {
		t.Fatalf("DH-T2: mirror buckets drifted (want thermal=0 ddr=57 l3=19): %+v", s)
	}
}
