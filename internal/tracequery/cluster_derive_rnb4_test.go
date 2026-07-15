package tracequery

// cluster_derive_rnb4_test.go — RNB-4 acceptance pins (docs/design/
// real_trace_campaign_20260705.md §29.88.9 R6 / §29.88.3+§29.88.12 R5 /
// §29.88.4+§29.88.7 R5a-R5b; user rulings 2026-07-14/15):
//
//	件1 CLUSTER-DERIVE — the four-rule derivation against the two committed
//	     real fixtures' ground truth (donghu 三簇真值 by user 勘正;tieba by
//	     rules 1+3), plus the rule-4 full-file basis surviving a windowed
//	     carve.
//	件2 R5 单基准 — the global max-core peak-frequency basis on the real
//	     fixtures, and 计入/缺口 same-number identity (§29.88.12 witness).
//	件3 R5a 按核档 — the binding-excludes-bigger-tier proof pair on the real
//	     donghu mask=ffb seat, and the tieba double-negative (禁无中生有).
//
// Fixture red line: the traces are REAL captures — every number below is a
// measured pin, never an edit target.

import (
	"context"
	"testing"
)

func rnb4DonghuIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatalf("BuildIndex(donghu): %v", err)
	}
	return idx
}

func rnb4TiebaIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatalf("BuildIndex(tieba): %v", err)
	}
	return idx
}

// 件1 pin ①: donghu 地面真值 [0,1,2,3]=小 / [4..11]=中 / [12,13]=大 (user
// 二次勘正 2026-07-14). The retired positional-thirds inference misclassified
// cpu9/10/11 into big (§29.88.8 scan) — the R6 co-movement derivation over
// the full-file curves restores the truth, and the class mapping orders by
// the full-trace fmax ladder (limits cpu0=1720000 / cpu4=2270000; big
// observed 2750000).
func TestR6DonghuClusterGroundTruth(t *testing.T) {
	idx := rnb4DonghuIndex(t)
	capability := indexDerivedCoreCapability(idx)
	if !capability.usable() {
		t.Fatalf("donghu must judge (three clusters), got source %q", capability.source)
	}
	wantClass := map[string][]int{
		"small":  {0, 1, 2, 3},
		"middle": {4, 5, 6, 7, 8, 9, 10, 11},
		"big":    {12, 13},
	}
	wantFmax := map[string]int{"small": 1720000, "middle": 2270000, "big": 2750000}
	for class, cpus := range wantClass {
		label, ok := capability.classClusterLabel(class)
		if !ok {
			t.Fatalf("class %s missing: %+v", class, capability.classByCluster)
		}
		members := capability.domains.members[label]
		if len(members) != len(cpus) {
			t.Fatalf("class %s members = %v, want %v", class, members, cpus)
		}
		for i, cpu := range cpus {
			if members[i] != cpu {
				t.Fatalf("class %s members = %v, want %v", class, members, cpus)
			}
		}
		if got := capability.fmaxByCluster[label]; got != wantFmax[class] {
			t.Fatalf("class %s fmax = %d, want %d", class, got, wantFmax[class])
		}
	}
	// The window-face inference authority answers the SAME classes (the
	// positional-thirds retirement pin: cpu9/10/11 are middle, not big).
	classes := indexDerivedCoreClassByCPU(idx)
	for _, cpu := range []int{9, 10, 11} {
		if classes[cpu] != "middle" {
			t.Fatalf("cpu%d must class middle under R6 (thirds inference minted big), got %q", cpu, classes[cpu])
		}
	}
	for _, cpu := range []int{12, 13} {
		if classes[cpu] != "big" {
			t.Fatalf("cpu%d must class big, got %q", cpu, classes[cpu])
		}
	}
	// 件2 pin: the R5 basis is the global max core's peak frequency point —
	// cpu12/13 @2750000 (observed, no limits row on the big cluster), cap
	// 2.53, trace-global.
	cache := newChainQueryCache(idx, nil)
	fm, refCap, refClass := cache.supplyFoldGlobalMaxBasis(cache.coreCapability(""))
	if fm.khz != 2750000 || fm.source != SupplyFoldFmaxSourceObserved || refClass != "big" || refCap != coreCapabilityDefaultBig {
		t.Fatalf("donghu R5 basis must be (2750000 observed, big, 2.53), got %d/%s/%s/%v", fm.khz, fm.source, refClass, refCap)
	}
}

// 件1 pin ②: tieba by rules 1+3 — cpu0/1/2 carry ZERO cpu_frequency samples
// and the first sampled core is cpu3 (§29.88.8 structure fact); 规则1 closes
// the leading cores into the first cluster, so the single derived domain is
// [0..5]. One cluster < 2 ⇒ the class judgment stays honestly freq_only and
// the R5 basis is the global peak frequency point 2189000 at cap 1.
func TestR6TiebaClusterDerivation(t *testing.T) {
	idx := rnb4TiebaIndex(t)
	tls := indexFreqSampleTimelines(idx)
	for _, cpu := range []int{0, 1, 2} {
		if len(tls[cpu]) != 0 {
			t.Fatalf("fixture fact drifted: cpu%d must have zero samples", cpu)
		}
	}
	if len(tls[3]) == 0 {
		t.Fatalf("fixture fact drifted: cpu3 must be the first sampled core")
	}
	d := deriveClusterFreqDomains(tls)
	if d.groupCount != 1 {
		t.Fatalf("tieba must derive ONE cluster, got %d (%+v)", d.groupCount, d.members)
	}
	members := d.members[d.byCPU[3]]
	want := []int{0, 1, 2, 3, 4, 5}
	if len(members) != len(want) {
		t.Fatalf("规则1 leading closure: members = %v, want %v", members, want)
	}
	for i := range want {
		if members[i] != want[i] {
			t.Fatalf("规则1 leading closure: members = %v, want %v", members, want)
		}
	}
	capability := indexDerivedCoreCapability(idx)
	if capability.usable() {
		t.Fatalf("one cluster must stay freq_only (no class fabrication), got %q", capability.source)
	}
	if classes := indexDerivedCoreClassByCPU(idx); len(classes) != 0 {
		t.Fatalf("freq_only must mint NO class words (thirds inference fabricated three), got %v", classes)
	}
	cache := newChainQueryCache(idx, nil)
	fm, refCap, refClass := cache.supplyFoldGlobalMaxBasis(cache.coreCapability(""))
	if fm.khz != 2189000 || refCap != 1 || refClass != "" {
		t.Fatalf("tieba R5 basis must be (2189000, cap 1, class-less), got %d/%v/%q", fm.khz, refCap, refClass)
	}
}

// 件1 pin ③ (规则4 全文件扫描): a WINDOWED build over donghu — the padded
// window gate skips most of the file's lines from event admission — still
// derives the SAME three clusters and the SAME 2750000 global basis, because
// the frequency curves ride the full-file side collection, never the carve.
// (The narrow window sits in the file head where cpu12/13 have no samples at
// all — a window-cropped basis could not even see the big cluster.)
func TestR6FullFileCurvesSurviveWindowedCarve(t *testing.T) {
	idx, err := BuildIndexWithOptions(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace", BuildOptions{
		TimeStart: 13762.800000, TimeEnd: 13762.810000, TimeStartSet: true, TimeEndSet: true,
		TimePaddingBefore: 0.001, TimePaddingAfter: 0.001, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Windowed {
		t.Fatalf("fixture setup: build must be windowed")
	}
	capability := indexDerivedCoreCapability(idx)
	if !capability.usable() {
		t.Fatalf("windowed carve must not crop the topology judgment, got %q", capability.source)
	}
	if label, ok := capability.classClusterLabel("big"); !ok || len(capability.domains.members[label]) != 2 || capability.fmaxByCluster[label] != 2750000 {
		t.Fatalf("windowed carve lost the big cluster: %+v / %+v", capability.classByCluster, capability.fmaxByCluster)
	}
	cache := newChainQueryCache(idx, nil)
	fm, _, refClass := cache.supplyFoldGlobalMaxBasis(cache.coreCapability(""))
	if fm.khz != 2750000 || refClass != "big" {
		t.Fatalf("R5 basis must stay the full-file 2750000 under a windowed carve, got %d/%q", fm.khz, refClass)
	}
}

// 件2 pin (§29.88.12 同源可互推, engine-real): on the donghu flagship window
// the §29.88.12 witness seat (CompThread_0-2955 inversion) publishes ONE
// conversion number — GatedRunningDeficitMs == SupplyFoldDeficitMs exactly
// (the pre-R5 report showed 6.972「按下游消费核」beside 7.296「按大核满频」).
func TestR5UnifiedConversionSingleNumberDonghuWitness(t *testing.T) {
	idx := rnb4DonghuIndex(t)
	q := Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	chain := BuildWakeupChain(idx, q)
	found := false
	for i := range chain.CausalImpacts {
		impact := &chain.CausalImpacts[i]
		if impact.Thread.PID != 2955 || impact.GatedRunningDeficitMs <= 0 {
			continue
		}
		found = true
		if impact.SupplyFoldBasis == nil {
			continue // gated-only seats carry the same fold number by construction
		}
		if impact.GatedRunningDeficitMs != impact.SupplyFoldDeficitMs {
			t.Fatalf("单基准单算法: gated %.6f must equal fold deficit %.6f on one seat",
				impact.GatedRunningDeficitMs, impact.SupplyFoldDeficitMs)
		}
		if impact.SupplyFoldBasis.FmaxKHz != 2750000 {
			t.Fatalf("witness fold basis must be the global 2750000, got %+v", impact.SupplyFoldBasis)
		}
	}
	if !found {
		t.Fatalf("witness seat missing (fixture drifted): %+v", chain.CausalImpacts)
	}
}

// 件3 pin ① (R5a 场景② positive, engine-real): the donghu JankManager-9655
// window's affinity seat (mask=ffb → allowed [0,1,3..11], excludes cpu2 and
// the whole 2750000 tier cpu12/13) MUST mint the 按核档 proof pair
// (2270000 < 2750000). The core-CLASS face cannot express this exclusion
// (§29.88.8 B锚点) — the tier ints are the mention's precise inputs.
func TestR5ADonghuBindingExcludesBiggerTier(t *testing.T) {
	idx := rnb4DonghuIndex(t)
	q := Query{PID: 9655, TimeStart: 13762.934161, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	rank := BuildRootCauseRank(idx, q)
	found := false
	for _, item := range rank.Items {
		if item.Type != "cpu_affinity_or_cpuset" || item.Thread.PID != 9655 {
			continue
		}
		found = true
		if item.CPUConstraintAllowedMaxTierKHz != 2270000 || item.CPUConstraintGlobalMaxTierKHz != 2750000 {
			t.Fatalf("mask=ffb seat must mint the tier pair 2270000<2750000, got %d/%d",
				item.CPUConstraintAllowedMaxTierKHz, item.CPUConstraintGlobalMaxTierKHz)
		}
	}
	if !found {
		t.Fatalf("affinity seat missing (fixture drifted): %+v", rank.Items)
	}
}

// 件3 pin ② (R5a negative — 禁无中生有): tieba carries no affinity/cpuset
// carrier at all in its flagship window (§29.88.8 ③ 如实负record), and its
// single-cluster shape could never prove a bigger-tier exclusion anyway — no
// rank item may carry the tier pair.
func TestR5ATiebaNoBindingMentionFabricated(t *testing.T) {
	idx := rnb4TiebaIndex(t)
	q := Query{PID: 59566, TimeStart: 34579.450627, TimeEnd: 34579.595184,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	rank := BuildRootCauseRank(idx, q)
	for _, item := range rank.Items {
		if item.CPUConstraintAllowedMaxTierKHz != 0 || item.CPUConstraintGlobalMaxTierKHz != 0 {
			t.Fatalf("tieba must mint NO tier-exclusion pair (negative arm), got %+v", item)
		}
	}
	// The unit predicate agrees: a single-cluster capability map proves
	// nothing (fail open), whatever the allowed set.
	if a, g, ok := cpuConstraintTierExclusion(indexDerivedCoreCapability(idx), []int{0, 1}); ok {
		t.Fatalf("single-cluster shape must not prove exclusion, got %d/%d", a, g)
	}
}

// R5a unit arms (判据准确性): binding that INCLUDES the top tier → no claim;
// unknown-tier member → no claim (exclusion unprovable); strict subset below
// the top tier → the pair.
func TestR5ATierExclusionPredicateArms(t *testing.T) {
	idx := rnb4DonghuIndex(t)
	capability := indexDerivedCoreCapability(idx)
	if _, _, ok := cpuConstraintTierExclusion(capability, []int{0, 12}); ok {
		t.Fatalf("a binding including the top tier (cpu12) must not claim exclusion")
	}
	if _, _, ok := cpuConstraintTierExclusion(capability, []int{0, 99}); ok {
		t.Fatalf("an unknown-tier member must fail open (exclusion unprovable)")
	}
	if a, g, ok := cpuConstraintTierExclusion(capability, []int{0, 1, 2, 3}); !ok || a != 1720000 || g != 2750000 {
		t.Fatalf("small-only binding must prove 1720000<2750000, got %d/%d/%v", a, g, ok)
	}
}

// 件1 pin ④ (规则3 区间闭包, synthetic — the two real fixtures carry no
// enclosed sample-less core): sampled cpu1/cpu3 co-move (one cluster), cpu5
// is a distinct cluster; the enclosed sample-less cpu2 joins the {1,3}
// cluster's interval, while the cross-cluster gap core cpu4 stays honestly
// unassigned (R6 retired the 向下继承 arm) and cpu6+ stays out (向上不外推).
func TestR6IntervalClosureEnclosedCore(t *testing.T) {
	a := []freqSample{{ts: 100.0, khz: 1000000}, {ts: 100.010, khz: 1200000}}
	b := []freqSample{{ts: 100.0, khz: 1000000}, {ts: 100.010, khz: 1200000}}
	c := []freqSample{{ts: 100.0, khz: 2000000}, {ts: 100.012, khz: 2100000}}
	d := deriveClusterFreqDomains(map[int][]freqSample{1: a, 3: b, 5: c})
	if d.groupCount != 2 {
		t.Fatalf("two co-move groups expected, got %+v", d)
	}
	if d.byCPU[2] == "" || d.byCPU[2] != d.byCPU[1] || d.byCPU[2] != d.byCPU[3] {
		t.Fatalf("规则3: enclosed cpu2 must join the {1,3} cluster, got %+v", d)
	}
	// Leading cpu0 joins the first cluster (规则1) beside the closure.
	if d.byCPU[0] != d.byCPU[1] {
		t.Fatalf("规则1: leading cpu0 must join the first cluster, got %+v", d)
	}
	if label, ok := d.byCPU[4]; ok {
		t.Fatalf("cross-cluster gap cpu4 must stay unassigned under R6, got %q", label)
	}
	if label, ok := d.byCPU[6]; ok {
		t.Fatalf("向上不外推: cpu6 must stay unassigned, got %q", label)
	}
}
