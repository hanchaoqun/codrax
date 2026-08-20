package tracequery

// evalcase_dhm_watch_pin_test.go — DHMINE-1 batch, piggyback-collection pins
// (搭车采集三件, dhmine1 spec 阶段一; mining ledger scratchpad
// ledger §29.172 收账节(候选表收录)与 roster DHMINE-1 盘点块。
// VERDICTS as long-term watch pins on the two committed real traces — each is
// a 现状 pin (fail-open forms stay fail-open; ruling changes go through the
// ledger, never through a test edit).
//
// Cases:
//
//	DHM-C1   §29.163.1 C1 burst 硬门准入裁定 watch: ZERO contrast-bearing
//	         co-emission bursts exist in either committed trace — the
//	         conditional-enablement judge (对比见证升级) has NO local live
//	         specimen; A(仅披露)stands on local data.
//	DHM-P3M  §29.171 P3MEASURE stage-two data watch on a NEW combo: the
//	         disposition census, the all-quiet family-① middle band
//	         (invalid==0 everywhere), and the two-dimension separation live
//	         specimen.
//	DHM-EPS  §29.104.17 ⑥ ε-overlap 门 watch: a NATURAL ≪10% partial-
//	         intersection keep-chain specimen exists locally (0.10%); the
//	         ruled fail-open (不加门) is pinned in place together with the
//	         µs-exact anchored+remainder split identity (roster A1 三级精确
//	         区间 family).
//
// Fixture red line: real captures — every number is a measured pin.

import (
	"fmt"
	"sort"
	"testing"
)

// DHM-C1 — §29.163.1 (verbatim): 「启用判据=comove_floor_single_burst token
// 自然采集(客户回访/eval 窗)显示「floor 拦住且 burst 携对比」活体在场且量可观
// 时开批,旗舰双复核;活体全为齐停形则 A 即终态。」
//
// CLUSTERSTREAM-1 随改论证 (§29.193.1, 2026-07-21): the witness criterion
// replaced the trimmed whole-sequence merge, but this census is UNMOVED by
// construction — it recomputes bursts independently from the raw per-CPU
// sample timelines (production carriers), and the derivation it consults only
// for clusterOf still yields the identical three donghu clusters / one tieba
// cluster (TestClusterStreamDonghuWitnessBaseline / TestR6TiebaCluster-
// Derivation). The single multi-value burst stays the 齐动 co-movement form:
// under the witness scanner it mints PRO pairs per value (the DHMINE
// not-con arm in cluster_freq_share_cap3_test.go), so the §29.163.1
// enablement judge still reads zero contrast live specimens.
// Phase-one census verdict (REPORTED PROMINENTLY, gates CONTRAST-1): across
// BOTH committed traces, ZERO 15µs co-emission bursts carry a boundary
// contrast (two different frequency points on two sides of a cluster
// boundary at the same instant). donghu holds exactly ONE multi-value burst
// (ts≈13762.858645, cpu12+cpu13) and it is the 齐动 transition form — BOTH
// members witness BOTH values inside one burst (co-movement, the opposite of
// a boundary split); tieba holds ZERO multi-value bursts. Independent
// recomputation arm: the walk below re-derives the census from the raw
// per-CPU sample timelines (production carriers), not from the derivation's
// own verdict.
func TestEvalcaseDHMC1ContrastBurstCensusZero(t *testing.T) {
	type burstFacts struct{ bursts, multiValue, contrast int }
	censusOf := func(idx *Index) burstFacts {
		timelines := indexFreqSampleTimelines(idx)
		domains := deriveClusterFreqDomains(timelines)
		clusterOf := func(cpu int) string {
			for label, members := range domains.members {
				for _, m := range members {
					if m == cpu {
						return label
					}
				}
			}
			return "?"
		}
		type sample struct {
			ts  float64
			cpu int
			khz int64
		}
		var all []sample
		for cpu, tl := range timelines {
			for _, s := range tl {
				all = append(all, sample{s.ts, cpu, s.khz})
			}
		}
		sort.Slice(all, func(i, j int) bool { return all[i].ts < all[j].ts })
		var facts burstFacts
		i := 0
		for i < len(all) {
			j := i + 1
			for j < len(all) && all[j].ts-all[j-1].ts <= clusterFreqDeriveMaxSkewSec {
				j++
			}
			burst := all[i:j]
			facts.bursts++
			values := map[int64]map[int]bool{}
			cpus := map[int]bool{}
			for _, s := range burst {
				if values[s.khz] == nil {
					values[s.khz] = map[int]bool{}
				}
				values[s.khz][s.cpu] = true
				cpus[s.cpu] = true
			}
			if len(cpus) >= 2 && len(values) >= 2 {
				facts.multiValue++
				// Contrast form = the values split across a cluster boundary:
				// two clusters, and the union of values across clusters is
				// not a single value (boundary two sides at different freq
				// points simultaneously).
				perCluster := map[string]map[int64]bool{}
				for khz, cs := range values {
					for cpu := range cs {
						l := clusterOf(cpu)
						if perCluster[l] == nil {
							perCluster[l] = map[int64]bool{}
						}
						perCluster[l][khz] = true
					}
				}
				if len(perCluster) >= 2 {
					distinct := map[int64]bool{}
					for _, vs := range perCluster {
						for k := range vs {
							distinct[k] = true
						}
					}
					if len(distinct) >= 2 {
						facts.contrast++
					}
				}
			}
			i = j
		}
		return facts
	}
	donghu := censusOf(evalcaseIndex(t, evalcaseDonghuFixture))
	tieba := censusOf(evalcaseIndex(t, evalcaseTiebaFixture))
	// Measured pins (phase-one probe, c1_burst_census.txt).
	if donghu.bursts != 182 || donghu.multiValue != 1 || donghu.contrast != 0 {
		t.Fatalf("DHM-C1: donghu burst census drifted: %+v (want 182/1/0) — if a CONTRAST burst appeared, take it to the §29.163.1 enablement judge, never re-base silently", donghu)
	}
	if tieba.bursts != 30 || tieba.multiValue != 0 || tieba.contrast != 0 {
		t.Fatalf("DHM-C1: tieba burst census drifted: %+v (want 30/0/0) — if a CONTRAST burst appeared, take it to the §29.163.1 enablement judge, never re-base silently", tieba)
	}
	// The verdict the §29.163.1 judge reads: zero contrast live specimens —
	// a non-zero count here is NOT a failure of the engine; it is the
	// enablement evidence appearing. Route it to the ledger (open the
	// CONTRAST-1 讨论 per §29.163.1), do not paper over the red.
	if donghu.contrast+tieba.contrast != 0 {
		t.Fatal("DHM-C1: a contrast-bearing burst appeared — take it to the §29.163.1 enablement judge")
	}
}

// DHM-P3M — §29.171 (verbatim): 「阶段二数据门就位:量测自此在每份报告静默积累;
// 中间带活体可观时按 §29.169 词面纪律(见证下界禁比值)议披露,消费者缺席 pin
// 的红=届时复审面。」
// Phase-one distribution facts on a NEW combo (tieba Chrome_IOThread-60560,
// full window): the closed-set disposition census (7 self_ruled / 5
// edge_terminated / 2 segment_join / 7 off-wire), the ALL-QUIET family-①
// middle band (counterfactual invalid == 0 on every seat — across all 16
// probe boards not one periodic-pinned conviction exists locally), and the
// two-dimension separation live specimen: Compositor-61238's running seat
// (值 0.175ms) measures counterfactually valid 65.514ms across its anchor
// windows while its structural edge-witness is 0.077ms ≤ 席值 — the §29.169
// coexistence (counterfactual lane rules, structural lane stays honest).
// Display-only red line intact: these fields feed NO gate/lane/ordinal.
func TestEvalcaseDHMP3MDistributionNewCombo(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	rank := BuildRootCauseRank(idx, Query{PID: 60560, TimeStart: 34579.450627, TimeEnd: 34579.595184,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	census := map[string]int{}
	var dispositionRoster []string
	for _, it := range rank.Items {
		census[it.P3MDisposition]++
		dispositionRoster = append(dispositionRoster, fmt.Sprintf("%s:%s-%d:%s", it.P3MDisposition, it.Thread.Comm, it.Thread.PID, it.Type))
		if it.P3MCounterfactualInvalidMs != 0 {
			t.Fatalf("DHM-P3M: family-① conviction appeared on %s/%d (invalid=%.3f) — stage-two middle-band data, record it in the ledger",
				it.Type, it.Thread.PID, it.P3MCounterfactualInvalidMs)
		}
	}
	want := map[string]int{
		p3mDispositionSelfRuled: 7,
		// B1260: ThreadPoolForeg-60559's D/IO-dominant dependency now keeps
		// its exact scheduling sub-seat, carrying the same typed disposition
		// as the underlying occurrence instead of disappearing at rank mint.
		p3mDispositionEdgeTerminatedWindow: 5,
		p3mDispositionSegmentJoin:          2,
		"":                                 7,
	}
	for disp, n := range want {
		if census[disp] != n {
			t.Fatalf("DHM-P3M: disposition census drifted: %q=%d want %d (full: %v roster=%v)", disp, census[disp], n, census, dispositionRoster)
		}
	}
	for disp := range census {
		if _, ok := want[disp]; !ok {
			t.Fatalf("DHM-P3M: unexpected disposition %q on this board (full: %v)", disp, census)
		}
	}
	sep := evalcaseDHMFindItem(rank.Items, "running", 61238)
	if sep == nil {
		t.Fatal("DHM-P3M: separation specimen Compositor-61238 missing")
	}
	if sep.P3MDisposition != p3mDispositionEdgeTerminatedWindow {
		t.Fatalf("DHM-P3M: specimen disposition drifted: %q", sep.P3MDisposition)
	}
	if !near(sep.ImpactMs, 0.175, 0.001) || !near(sep.P3MCounterfactualValidMs, 65.514, 0.001) ||
		!near(sep.P3MEdgeWitnessedMs, 0.077, 0.001) {
		t.Fatalf("DHM-P3M: specimen values drifted: 席值=%.3f valid=%.3f edge=%.3f",
			sep.ImpactMs, sep.P3MCounterfactualValidMs, sep.P3MEdgeWitnessedMs)
	}
	if sep.P3MEdgeWitnessedMs > sep.ImpactMs+0.001 {
		t.Fatalf("DHM-P3M: edge_witnessed %.3f exceeds 席值 %.3f", sep.P3MEdgeWitnessedMs, sep.ImpactMs)
	}
}

// DHM-EPS — §29.104.17 ⑥ (verbatim): 「**⑥ε-overlap 门**(用户:「按推荐的来」):
// **不加门**——部分相交保链 fail-open 维持,合成 pin 看守;客户复放出现自然
// ≪10% 形再启用。零代码,账本记结即闭。」
// Phase-one live finding: a NATURAL ≪10% form EXISTS locally — donghu
// VSyncGenerator-2179's board carries JankManager-9655's runnable family with
// anchored share 0.031ms of a 31.191ms full account (0.10%). The ruled
// fail-open keeps the split published (◇ remainder seat in place, values
// exact); this pin freezes the specimen so the ε-door discussion has a
// standing local witness — enablement itself remains a user ruling (账本),
// never a test edit. Bonus arm (roster A1 三级精确区间 family, acceptance
// verbatim 「链上 runnable 族 Σ ≤ 该线程全窗 runnable…修后恰一全额席,其余席
// 互指降道」): anchored + remainder == full µs-exactly — the split never
// mints a double count.
func TestEvalcaseDHMEPSNaturalSubTenPercentAnchorSpecimen(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	rank := BuildRootCauseRank(idx, Query{PID: 2179, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		TraceFlavorHint: TraceFlavorHarmonyHitrace})
	seat := evalcaseDHMFindItem(rank.Items, "runnable_wait", 9655)
	if seat == nil {
		t.Fatal("DHM-EPS: 9655 runnable remainder seat missing")
	}
	if !seat.ChainAnchorRemainderSeat {
		t.Fatal("DHM-EPS: specimen lost its ◇ remainder marker")
	}
	if !near(seat.ChainAnchoredMs, 0.031, 0.001) || !near(seat.ChainAnchorFullMs, 31.191, 0.001) ||
		!near(seat.ImpactMs, 31.160, 0.001) {
		t.Fatalf("DHM-EPS: specimen values drifted: anchored=%.3f full=%.3f remainder=%.3f",
			seat.ChainAnchoredMs, seat.ChainAnchorFullMs, seat.ImpactMs)
	}
	// µs identity: anchored + remainder == full (三级精确区间, never a hull).
	if !near(seat.ChainAnchoredMs+seat.ImpactMs, seat.ChainAnchorFullMs, 0.0011) {
		t.Fatalf("DHM-EPS: split identity broken: %.6f + %.6f != %.6f",
			seat.ChainAnchoredMs, seat.ImpactMs, seat.ChainAnchorFullMs)
	}
	// The natural ≪10% fraction (0.10%) — the form §29.104.17 ⑥ waits for.
	frac := seat.ChainAnchoredMs / seat.ChainAnchorFullMs
	if frac >= 0.10 {
		t.Fatalf("DHM-EPS: specimen fraction drifted out of the ≪10%% family: %.4f", frac)
	}
}
