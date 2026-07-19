package tracequery

// evalcase_cmp_mech_pin_test.go — EVALCASE-DH batch, 双 trace 对比机制 pins
// over the two committed real fixtures (mining ledger evalcase_xa_cmp_mining.md
// §2; CMP campaign red lines: CMP-A 窗长归一 / CMP-6 census 折叠防伪 /
// CMP-10 需求供给分离 / 折算全域单基准裁定).
//
// Each case pins the PER-SIDE typed truth that the comparison mechanism must
// consume — the numbers a cross-trace narrative is honest against.
//
//	CMP-XA1 窗长差归一化 — the raw clock_set_rate count ratio (767/323 =
//	        2.37×) is a WINDOW-LENGTH artifact; after dividing by each
//	        side's own span (233.190ms vs 144.557ms) the density ratio is
//	        1.47×. Cross-trace aggregates must divide by window length —
//	        both denominators are pinned on the artifacts themselves.
//	CMP-XA2 单边未采样 — donghu cpu0..3 carry their OWN cpu_frequency
//	        samples; tieba cpu0..2 carry ZERO samples and their fmax is a
//	        same-cluster donor reuse carrying the typed disclosure
//	        (donor_cpu=3 / freq_change_point_derived). Sample absence ≠
//	        low frequency; a donor-reused value must never impersonate an
//	        own-sample measurement in a cross-machine comparison.
//	CMP-XA3 簇 verdict 不对称分侧 — donghu folds on the usable default
//	        table (three classes, basis 2750000/big/2.53); tieba stays
//	        freq_only (classless, basis 2189000/cap 1). The two fold bases
//	        are typed per side and must never silently mix.
//	CMP-XA4 锁形不对称 — donghu carries 形A monitor grammar ×4 + 形B ×19;
//	        tieba is PURE 形B ×84 with zero monitor grammar. The sentinel
//	        vocabulary (uint64-1 / 0 → typed ownerless) means the same
//	        thing on both sides. Raw counts invert under normalization:
//	        84/144.557ms = 0.581/ms vs 23/233.190ms = 0.099/ms.
//	CMP-XA5 需求/供给分离 — the registry keeps runnable/cpu_pressure/
//	        supply_pressure on the scheduling-demand lane (§7.4: demand
//	        despite the name; §7.5 keeps the wire token) and the
//	        compute_supply family on the compute-delivery lane; each side's
//	        balance face carries its OWN window_ms denominator and fold
//	        basis, so no single "CPU 压力 N×" headline is derivable from
//	        the typed faces (the customer 2.18× lesson).

import (
	"strings"
	"testing"
)

func evalcaseCountEvents(idx *Index, typ EventType) int {
	n := 0
	for _, ev := range idx.Events {
		if ev.Type == typ {
			n++
		}
	}
	return n
}

// CMP-XA1.
func TestEvalcaseCMPXA1WindowLengthNormalization(t *testing.T) {
	donghu := evalcaseIndex(t, evalcaseDonghuFixture)
	tieba := evalcaseIndex(t, evalcaseTiebaFixture)
	donghuSpanMs := (donghu.LastTs - donghu.FirstTs) * 1000
	tiebaSpanMs := (tieba.LastTs - tieba.FirstTs) * 1000
	if !near(donghuSpanMs, 233.190, 0.001) || !near(tiebaSpanMs, 144.557, 0.001) {
		t.Fatalf("CMP-XA1: window spans drifted: %.3f / %.3f", donghuSpanMs, tiebaSpanMs)
	}
	donghuClock := evalcaseCountEvents(donghu, EventClockSetRate)
	tiebaClock := evalcaseCountEvents(tieba, EventClockSetRate)
	if donghuClock != 767 || tiebaClock != 323 {
		t.Fatalf("CMP-XA1: clock_set_rate census drifted: %d / %d", donghuClock, tiebaClock)
	}
	rawRatio := float64(donghuClock) / float64(tiebaClock)
	densityRatio := (float64(donghuClock) / donghuSpanMs) / (float64(tiebaClock) / tiebaSpanMs)
	if rawRatio < 2.37 || rawRatio > 2.38 {
		t.Fatalf("CMP-XA1: raw ratio drifted: %.3f", rawRatio)
	}
	if densityRatio < 1.46 || densityRatio > 1.48 {
		t.Fatalf("CMP-XA1: normalized density ratio drifted: %.3f (the honest cross-trace figure)", densityRatio)
	}
	// The trap margin itself: reading the raw ratio as the comparison figure
	// overstates by >60% — the mechanism red line is that the denominator
	// (each side's own window) is pinned ON the artifact, never optional.
	if rawRatio/densityRatio < 1.6 {
		t.Fatalf("CMP-XA1: trap margin drifted: %.3f", rawRatio/densityRatio)
	}
}

// CMP-XA2.
func TestEvalcaseCMPXA2OneSidedUnsampledDonorNoImpersonation(t *testing.T) {
	donghu := evalcaseIndex(t, evalcaseDonghuFixture)
	tieba := evalcaseIndex(t, evalcaseTiebaFixture)
	donghuTLs := indexFreqSampleTimelines(donghu)
	for _, cpu := range []int{0, 1, 2, 3} {
		if len(donghuTLs[cpu]) == 0 {
			t.Fatalf("CMP-XA2: donghu cpu%d must carry its OWN samples", cpu)
		}
	}
	tiebaTLs := indexFreqSampleTimelines(tieba)
	for _, cpu := range []int{0, 1, 2} {
		if len(tiebaTLs[cpu]) != 0 {
			t.Fatalf("CMP-XA2: tieba cpu%d must carry ZERO samples (single-sided absence)", cpu)
		}
	}
	// The tieba side's balance rows for cpu0-2 must be donor-disclosed, and
	// the donghu side's small-cluster rows must NOT carry any donor (their
	// own samples govern) — the typed asymmetry a comparison must surface.
	qT := normalizeQuery(tieba, Query{PID: 59566, TimeStart: evalcaseXAFullStart, TimeEnd: evalcaseXAFullEnd})
	balT := ComputeWindowStats(tieba, qT).ComputeSupplyBalance
	if balT == nil {
		t.Fatal("CMP-XA2: tieba balance missing")
	}
	for _, p := range balT.PerCPU {
		if p.CPU <= 2 {
			if p.FrequencyClusterDonorCPU == nil || *p.FrequencyClusterDonorCPU != 3 || p.FrequencyClusterDonorSource != ClusterFreqSourceDerived {
				t.Fatalf("CMP-XA2: tieba cpu%d donor disclosure missing: %+v", p.CPU, p)
			}
		}
	}
	qD := normalizeQuery(donghu, Query{PID: 17267, TimeStart: 13762.9374, TimeEnd: 13762.9736})
	balD := ComputeWindowStats(donghu, qD).ComputeSupplyBalance
	if balD == nil {
		t.Fatal("CMP-XA2: donghu balance missing")
	}
	for _, p := range balD.PerCPU {
		if p.FrequencyClusterDonorCPU != nil {
			t.Fatalf("CMP-XA2: donghu cpu%d must not carry a donor (own samples): %+v", p.CPU, p)
		}
	}
}

// CMP-XA3.
func TestEvalcaseCMPXA3ClusterVerdictAsymmetricBases(t *testing.T) {
	donghu := evalcaseIndex(t, evalcaseDonghuFixture)
	tieba := evalcaseIndex(t, evalcaseTiebaFixture)
	capD := indexDerivedCoreCapability(donghu)
	capT := indexDerivedCoreCapability(tieba)
	if !capD.usable() || capD.source != "default_table" {
		t.Fatalf("CMP-XA3: donghu side must fold on the usable default table: %q", capD.source)
	}
	if capT.usable() || capT.source != CoreCapabilitySourceFreqOnly {
		t.Fatalf("CMP-XA3: tieba side must stay freq_only: %q", capT.source)
	}
	cacheD := newChainQueryCache(donghu, nil)
	fmD, capValD, classD := cacheD.supplyFoldGlobalMaxBasis(cacheD.coreCapability(""))
	cacheT := newChainQueryCache(tieba, nil)
	fmT, capValT, classT := cacheT.supplyFoldGlobalMaxBasis(cacheT.coreCapability(""))
	if fmD.khz != 2750000 || classD != "big" || capValD != coreCapabilityDefaultBig {
		t.Fatalf("CMP-XA3: donghu basis drifted: %d/%s/%v", fmD.khz, classD, capValD)
	}
	if fmT.khz != 2189000 || classT != "" || capValT != 1 {
		t.Fatalf("CMP-XA3: tieba basis drifted: %d/%q/%v", fmT.khz, classT, capValT)
	}
	// Both bases are observed-sourced but NOT interchangeable: mixing them
	// (pricing one trace's ms on the other's basis) has no typed carrier —
	// the per-side (khz, cap, class) triplets differ in every component.
	if fmD.khz == fmT.khz || capValD == capValT {
		t.Fatalf("CMP-XA3: asymmetry collapsed — bases became interchangeable")
	}
}

// CMP-XA4.
func TestEvalcaseCMPXA4LockShapeAsymmetry(t *testing.T) {
	donghu := evalcaseIndex(t, evalcaseDonghuFixture)
	tieba := evalcaseIndex(t, evalcaseTiebaFixture)
	census := func(idx *Index) (formA, formB int) {
		q := normalizeQuery(idx, Query{TimeStart: idx.FirstTs, TimeEnd: idx.LastTs, MinDurationMs: 0.0001})
		spans, _, _ := computeTraceMarks(idx, q, 20000)
		for _, s := range spans {
			if !strings.Contains(s.Name, "contention") {
				continue
			}
			info, ok := parseLockContentionPayload(s.Name)
			if !ok {
				continue
			}
			if info.Kind == blockingKindMonitorContention && strings.HasPrefix(s.Name, "monitor contention") {
				formA++
			} else {
				formB++
			}
		}
		return
	}
	dA, dB := census(donghu)
	tA, tB := census(tieba)
	if dA != 4 || dB != 19 {
		t.Fatalf("CMP-XA4: donghu lock census drifted: 形A=%d 形B=%d (want 4/19)", dA, dB)
	}
	if tA != 0 || tB != 84 {
		t.Fatalf("CMP-XA4: tieba lock census drifted: 形A=%d 形B=%d (want 0/84 — 无形A证据须如实缺席)", tA, tB)
	}
	// Normalized rates INVERT the naive story direction check: tieba is the
	// lock-heavy side per ms (0.581/ms vs 0.099/ms), and the raw counts must
	// never cross traces without each side's own window denominator.
	dRate := float64(dA+dB) / ((donghu.LastTs - donghu.FirstTs) * 1000)
	tRate := float64(tA+tB) / ((tieba.LastTs - tieba.FirstTs) * 1000)
	if dRate > 0.11 || tRate < 0.55 {
		t.Fatalf("CMP-XA4: normalized lock rates drifted: %.3f/ms vs %.3f/ms", dRate, tRate)
	}
	// Sentinel vocabulary is side-invariant (the same parser, the same typed
	// ownerless verdict) — a comparison must not read tieba's 30 sentinel
	// rows as "unknown ART bug" while donghu's read as no-holder.
	for _, payload := range []string{
		"Lock contention on ClassLinker classes lock (owner tid: 18446744073709551615)",
		"Lock contention on visibly initialized callback lock (owner tid: 0)",
	} {
		info, ok := parseLockContentionPayload(payload)
		if !ok || !info.OwnerAbsent {
			t.Fatalf("CMP-XA4: sentinel semantics drifted for %q: %+v", payload, info)
		}
	}
}

// CMP-XA5.
func TestEvalcaseCMPXA5DemandSupplySeparation(t *testing.T) {
	// Registry lane split (§7.4 ruling-locked; supply_pressure display split
	// §7.5 终局裁定 — the wire token stays, the lane says demand).
	for _, token := range []string{"runnable_wait", "cpu_pressure", "supply_pressure"} {
		spec, ok := causalTokenRegistry[token]
		if !ok || spec.Lane != CausalLaneSchedulingDemand {
			t.Fatalf("CMP-XA5: %s must ride the scheduling-demand lane, got %+v", token, spec)
		}
	}
	for _, token := range []string{"compute_supply", "compute_supply_balance", "low_frequency", "cpu_frequency_limit"} {
		spec, ok := causalTokenRegistry[token]
		if !ok || spec.Lane != CausalLaneComputeDelivery {
			t.Fatalf("CMP-XA5: %s must ride the compute-delivery lane, got %+v", token, spec)
		}
	}
	// Per-side balance faces carry their OWN denominators and saturation
	// shapes: tieba cpu0..2 run near window saturation (≥140ms of 144.557)
	// while the donghu J1 window keeps whole middle-cluster cores idle
	// (cpu8/9/10 running=0) — "哪台更卡" has no single-axis answer, and the
	// typed faces force the demand/supply split narrative.
	tieba := evalcaseIndex(t, evalcaseTiebaFixture)
	qT := normalizeQuery(tieba, Query{PID: 59566, TimeStart: evalcaseXAFullStart, TimeEnd: evalcaseXAFullEnd})
	balT := ComputeWindowStats(tieba, qT).ComputeSupplyBalance
	if balT == nil || !near(balT.WindowMs, 144.557, 0.001) {
		t.Fatalf("CMP-XA5: tieba balance denominator drifted: %+v", balT)
	}
	saturated := 0
	for _, p := range balT.PerCPU {
		if p.CPU <= 2 && p.RunningMs >= 140 {
			saturated++
		}
	}
	if saturated != 3 {
		t.Fatalf("CMP-XA5: tieba cpu0..2 saturation shape drifted: %d of 3", saturated)
	}
	donghu := evalcaseIndex(t, evalcaseDonghuFixture)
	qD := normalizeQuery(donghu, Query{PID: 17267, TimeStart: 13762.9374, TimeEnd: 13762.9736})
	balD := ComputeWindowStats(donghu, qD).ComputeSupplyBalance
	if balD == nil || !near(balD.WindowMs, 36.200, 0.001) {
		t.Fatalf("CMP-XA5: donghu balance denominator drifted: %+v", balD)
	}
	idleMiddle := 0
	for _, p := range balD.PerCPU {
		if p.CoreClass == "middle" && p.RunningMs == 0 {
			idleMiddle++
		}
	}
	if idleMiddle < 3 {
		t.Fatalf("CMP-XA5: donghu idle middle-core shape drifted: %d", idleMiddle)
	}
}
