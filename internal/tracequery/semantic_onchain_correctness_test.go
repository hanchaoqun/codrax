package tracequery

import (
	"strings"
	"testing"
)

func TestSemanticOnChainUsesOrdinaryElectionOffChainUsesBackgroundOnly(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "jit_compile", ImpactMs: 9, EffectiveImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "shader_compile", ImpactMs: 7, EffectiveImpactMs: 7, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "gc_pause", ImpactMs: 5, EffectiveImpactMs: 5, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "class_verification", ImpactMs: 4, EffectiveImpactMs: 4, ChainRelevance: "background", Causality: "background"},
	}
	sortRootCauseRankItems(items, true)
	assignRootCauseRankOrdinalsAndTiers(items)
	for i, want := range []string{"primary", "secondary", "tertiary"} {
		if items[i].Tier != want || items[i].Rank != i+1 {
			t.Fatalf("on-chain semantic row %d must use the ordinary ladder: got %+v want tier=%s rank=%d", i, items[i], want, i+1)
		}
		if items[i].BackgroundRank != 0 {
			t.Fatalf("on-chain semantic row must never carry background_rank: %+v", items[i])
		}
		if items[i].Tier == RootCauseTierDeterministicOptimization {
			t.Fatalf("current engine must not mint the legacy semantic tier: %+v", items[i])
		}
	}
	if items[3].Tier != "tertiary" || items[3].BackgroundRank != 1 {
		t.Fatalf("off-chain semantic row must stay background-only: %+v", items[3])
	}
}

func TestGCPauseSemanticClassifierIsExplicitAndConservative(t *testing.T) {
	positives := []string{
		"GC",
		"GC pause",
		"GC: Young collection",
		"H:GC concurrent mark",
		"Garbage Collection",
		"garbage_collection full",
		"SuspendAllForGC",
		"WaitForGcToComplete",
		"CollectGarbage",
	}
	for _, name := range positives {
		work, ok := traceSpanSemanticWorkClass(name)
		if !ok || work.SemanticClass != "gc_pause" || work.RootCauseType != "gc_pause" {
			t.Fatalf("explicit GC span %q must classify as gc_pause: work=%+v ok=%v", name, work, ok)
		}
	}
	for _, name := range []string{
		"GCLockerMetrics",
		"gc_cache lookup",
		"GC metrics flush",
		"ImageGCrop",
		"magicgcvalue",
		"garbage collector metrics",
		"H:gc_cache",
	} {
		if work, ok := traceSpanSemanticWorkClass(name); ok {
			t.Fatalf("unrelated GC-like name %q must not classify: %+v", name, work)
		}
	}
	spec, ok := CausalTokenSpecFor("gc_pause")
	if !ok || spec.Lane != CausalLaneCPUWork || spec.Additivity != CausalAdditivityWallClockPerThread || !spec.RowToken {
		t.Fatalf("gc_pause registry shape is incomplete: %+v ok=%v", spec, ok)
	}
	if CausalTokenFamilyFoldLane("gc_pause") != CausalFamilyFoldSemanticClass || !rootCauseTypeCanBeDirectOnChain("gc_pause") {
		t.Fatalf("gc_pause must share semantic family-fold and on-chain admission")
	}
}

func TestGCPauseEndToEndRanksOnChainAndStaysOptimizationObservable(t *testing.T) {
	idx := buildTraceIndex(t, "gc_pause_onchain.systrace", `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.000400: tracing_mark_write: B|200|H:GC pause young
     worker-200 (100) [002] .... 5.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (100) [002] .... 5.005800: tracing_mark_write: E|200
     worker-200 (100) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006500: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	q := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.007, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	stats := ComputeWindowStats(idx, q)
	if len(stats.TraceSpans) == 0 || stats.TraceSpans[0].SemanticClass != "gc_pause" {
		t.Fatalf("window stats must preserve the gc_pause semantic class: %+v", stats.TraceSpans)
	}
	rank := BuildRootCauseRank(idx, q)
	found := false
	for _, item := range rank.Items {
		if item.Type != "gc_pause" {
			continue
		}
		found = true
		if item.ChainRelevance != "on_chain" || item.OnChainBasis != RootCauseOnChainBasisSemanticChainIntervalRelation ||
			item.Tier != RootCauseTierContextOnly || item.EffectiveImpactMs != 0 || item.BackgroundRank != 0 {
			t.Fatalf("non-target GC pause must remain a relation-only on-chain business clue: %+v", item)
		}
		if item.SemanticClass != "gc_pause" || !strings.Contains(item.Summary, "GC pause") {
			t.Fatalf("GC typed identity/label was lost: %+v", item)
		}
	}
	if !found {
		t.Fatalf("expected an on-chain gc_pause rank row: %+v", rank.Items)
	}
}

func TestSemanticSpanBoundKeepsOnePerFamilyAndCountsOverflow(t *testing.T) {
	classes := []string{"jit_compile", "class_verification", "shader_compile", "runtime_compile", "texture_upload", "gc_pause"}
	nameFor := map[string]string{
		"jit_compile": "JIT compile foo", "class_verification": "VerifyClass Foo",
		"shader_compile": "ShaderCompile pipeline", "runtime_compile": "Ark runtime compile",
		"texture_upload": "Texture upload", "gc_pause": "GC pause",
	}
	var spans []TraceSpanSummary
	// A large JIT family appears first by duration; every other class must
	// still redeem one family seat before JIT consumes the spare seats.
	for i := 0; i < 20; i++ {
		class := classes[0]
		if i >= 15 {
			class = classes[i-14]
		}
		spans = append(spans, TraceSpanSummary{
			Thread: ThreadRef{Comm: "worker", PID: 200}, Name: nameFor[class], SemanticClass: class,
			StartTs: 5 + float64(i)*0.001, EndTs: 5 + float64(i)*0.001 + 0.0005,
			DurationMs: float64(30 - i), StartLine: i + 1, EndLine: i + 1,
		})
	}
	bounded, info := boundTraceMarkSpansWithInfo(spans, 8)
	seen := map[string]bool{}
	for _, span := range bounded {
		seen[span.SemanticClass] = true
	}
	for _, class := range classes {
		if !seen[class] {
			t.Fatalf("family-aware cap lost class %s: %+v", class, bounded)
		}
	}
	if caveat := info.caveat(); caveat == "" || !strings.Contains(caveat, "kept 16/20") || !strings.Contains(caveat, "lower bounds") {
		t.Fatalf("semantic cap overflow must be counted and fail loud: %q info=%+v", caveat, info)
	}
}
