package tracequery

import (
	"strings"
	"testing"
)

func semanticProjectionSpan(thread ThreadRef, name string, start, end float64, line int) TraceSpanSummary {
	return TraceSpanSummary{
		Thread: thread, Kind: "sync", Name: name, SemanticClass: "class_verification",
		StartTs: start, EndTs: end, DurationMs: (end - start) * 1000,
		StartLine: line, EndLine: line + 1,
	}
}

func TestSemanticFamilyOnChainRankUsesExactIntersectionUnion(t *testing.T) {
	worker := ThreadRef{Comm: "worker", PID: 200}
	chain := &ChainResult{
		Target: worker,
		Nodes: []ChainNode{{
			Thread: worker, Window: TimeWindow{StartTs: 5.002, EndTs: 5.008},
			Dominant: StateRunning, Depth: 1, Branch: 1,
		}, {
			Thread: worker, Window: TimeWindow{StartTs: 5.006, EndTs: 5.012},
			Dominant: StateRunnable, Depth: 2, Branch: 2,
		}, {
			Thread: ThreadRef{Comm: "other", PID: 300},
			Window: TimeWindow{StartTs: 5.000, EndTs: 5.015}, Dominant: StateRunning,
		}},
		// Duplicate/overlapping impact faces are deliberately present. They
		// must enrich provenance, never add the same wall time again.
		CausalImpacts: []WakeupCausalImpact{{
			Thread: worker, Window: TimeWindow{StartTs: 5.004, EndTs: 5.010},
			DominantState: string(StateRunning), ChainDepth: 3, ChainBranch: 3,
		}},
	}
	spans := []TraceSpanSummary{
		semanticProjectionSpan(worker, "VerifyClass A", 5.000, 5.010, 10),
		semanticProjectionSpan(worker, "VerifyClass B", 5.005, 5.015, 12),
	}
	families := FoldSemanticSpanFamilies(chain, spans)
	if len(families) != 1 {
		t.Fatalf("expected one on-chain family, got %+v", families)
	}
	fam := families[0]
	if !fam.OnChain || len(fam.Members) != 2 {
		t.Fatalf("family lane/member identity drifted: %+v", fam)
	}
	// Member union [0,15] = 15ms despite a 20ms member sum. Chain union
	// [2,12] = 10ms despite two overlapping nodes plus a duplicate impact.
	if !near(fam.SumMs, 20, 0.001) || !near(fam.TotalMs, 15, 0.001) {
		t.Fatalf("complete family disclosure must preserve sum=20ms/union=15ms: %+v", fam)
	}
	if !near(fam.ProjectedImpactMs, 10, 0.001) ||
		!near(fam.ProjectedStartTs, 5.002, 0.000001) || !near(fam.ProjectedEndTs, 5.012, 0.000001) {
		t.Fatalf("on-chain projection must be the exact 10ms intersection union: %+v", fam)
	}
	// Running windows union to [2,10]=8ms; runnable contributes [6,12]=6ms.
	// The row may name Running dominant, but may not stamp the cross-state
	// 10ms total into RunningMs.
	if fam.DominantState != string(StateRunning) || !near(fam.DominantStateImpactMs, 8, 0.001) {
		t.Fatalf("dominant-state attribution must remain exact: %+v", fam)
	}

	item, ok := rootCauseItemFromSemanticSpanFamily(Query{}, fam, true)
	if !ok {
		t.Fatalf("on-chain family must mint a rank contender")
	}
	for name, got := range map[string]float64{
		"impact": item.ImpactMs, "projected": item.ProjectedImpactMs,
		"effective": item.EffectiveImpactMs, "overlap": item.OverlapMs,
	} {
		if !near(got, 10, 0.001) {
			t.Fatalf("%s must use only the 10ms chain intersection, got %.6f: %+v", name, got, item)
		}
	}
	if !near(item.CumulativeImpactMs, 15, 0.001) || !near(item.ActualImpactMs, 15, 0.001) {
		t.Fatalf("complete window union/actual disclosure must survive beside projected impact: %+v", item)
	}
	if !near(item.RunningMs, 8, 0.001) || item.RunnableMs != 0 {
		t.Fatalf("dominant-state bucket must not absorb other chain states: %+v", item)
	}
	if !near(item.StartTs, 5.002, 0.000001) || !near(item.EndTs, 5.012, 0.000001) {
		t.Fatalf("rank interval must be the projected intersection envelope: %+v", item)
	}
	if !strings.Contains(item.Summary, "exact self interval intersection") ||
		!strings.Contains(item.Summary, "15.000ms complete selected-window span union") {
		t.Fatalf("summary must distinguish causal attribution from full disclosure: %q", item.Summary)
	}
}

func TestSemanticFamilyOffChainCaliberRemainsCompleteWindowUnion(t *testing.T) {
	worker := ThreadRef{Comm: "worker", PID: 200}
	chain := &ChainResult{Nodes: []ChainNode{{
		Thread: ThreadRef{Comm: "other", PID: 300},
		Window: TimeWindow{StartTs: 5.000, EndTs: 5.020},
	}}}
	families := FoldSemanticSpanFamilies(chain, []TraceSpanSummary{
		semanticProjectionSpan(worker, "VerifyClass A", 5.000, 5.003, 10),
		semanticProjectionSpan(worker, "VerifyClass B", 5.004, 5.006, 12),
	})
	if len(families) != 1 || families[0].OnChain || families[0].ProjectedImpactMs != 0 {
		t.Fatalf("off-chain family lane must remain unchanged: %+v", families)
	}
	item, ok := rootCauseItemFromSemanticSpanFamily(Query{}, families[0], true)
	if !ok || !near(item.ImpactMs, 5, 0.001) || !near(item.EffectiveImpactMs, 5, 0.001) ||
		!near(item.CumulativeImpactMs, 5, 0.001) || item.OverlapMs != 0 {
		t.Fatalf("off-chain participation must remain the complete 5ms union: %+v", item)
	}
}

func TestSemanticSingleSpanAndFamilyProjectionUseSameDeduplicatedIntersection(t *testing.T) {
	worker := ThreadRef{Comm: "worker", PID: 200}
	span := semanticProjectionSpan(worker, "VerifyClass A", 5.000, 5.010, 10)
	chain := ChainResult{Target: worker, Nodes: []ChainNode{{
		Thread: worker, Window: TimeWindow{StartTs: 5.002, EndTs: 5.006}, Dominant: StateRunning,
	}, {
		Thread: worker, Window: TimeWindow{StartTs: 5.004, EndTs: 5.008}, Dominant: StateRunning,
	}}}
	families := FoldSemanticSpanFamilies(&chain, []TraceSpanSummary{span})
	if len(families) != 1 || !near(families[0].ProjectedImpactMs, 6, 0.001) {
		t.Fatalf("degenerate family projection must union overlapping chain windows once: %+v", families)
	}
	item, ok := rootCauseItemFromSemanticTraceSpan(Query{}, chain, span, true)
	if !ok {
		t.Fatalf("single semantic span must mint")
	}
	if !near(item.ProjectedImpactMs, 6, 0.001) || !near(item.EffectiveImpactMs, 6, 0.001) {
		t.Fatalf("single-span path must use the same 6ms exact union, never 4+4=8ms: %+v", item)
	}
	if !near(item.ActualImpactMs, 10, 0.001) {
		t.Fatalf("single-span full extent must remain disclosed: %+v", item)
	}
}
