package tracequery

// causal_token_registry_gcpause_pin_test.go — 审计 #64 registry pin (§29.25
// 处置委托 + §29.26 待主会话落账, 2026-07-10): gc_pause, the SIXTH semantic
// span class (extension pattern = the §28.1 texture_upload "FIFTH class" user
// ruling), pinned on all four registry dimensions plus its family-fold lane
// and its distinctness from the aggregate memory_gc count family. The lane
// adjudication (read §7.2.1 + ledger §7.4/§7.5 first) is recorded on the
// registry entry: the ruling-locked demand/supply split is untouched — a GC
// pause is a measured per-thread wall-clock span interval on the CPU-work
// home shared by the semantic classes, never a runnable backlog (调度压力
// wording) and never a delivery-side aggregate (算力 wording).

import "testing"

func TestGCPauseRegistryFourDimensionPin(t *testing.T) {
	spec, ok := CausalTokenSpecFor("gc_pause")
	if !ok {
		t.Fatalf("gc_pause must be registered in the causal token registry")
	}
	// Dimension 1 — lane (demand/supply adjudication): the CPU-work home of
	// the semantic span classes; NEVER the ruling-locked scheduling_demand or
	// compute_delivery lanes (§7.4 需求/供给分离).
	if spec.Lane != CausalLaneCPUWork {
		t.Fatalf("gc_pause lane must be cpu_work (semantic-class home), got %q", spec.Lane)
	}
	if spec.Lane == CausalLaneSchedulingDemand || spec.Lane == CausalLaneComputeDelivery {
		t.Fatalf("gc_pause must never enter a §7.4 ruling-locked lane: %q", spec.Lane)
	}
	// Dimension 2 — additivity: a paired-span measured interval is per-thread
	// wall clock (never cross-thread cpu·ms, never a count-derived scalar).
	if spec.Additivity != CausalAdditivityWallClockPerThread {
		t.Fatalf("gc_pause additivity must be wall_clock_per_thread, got %q", spec.Additivity)
	}
	// Dimension 3 — subject kind: the span lives on one concrete thread.
	if spec.Subject != CausalSubjectPerThread {
		t.Fatalf("gc_pause subject must be per_thread, got %q", spec.Subject)
	}
	// Dimension 4 — row token + zh label ownership (identical treatment to the
	// five sibling semantic classes).
	if !spec.RowToken {
		t.Fatalf("gc_pause must be a rank-row token (semantic-class treatment)")
	}
	if spec.LabelZhRef != CausalZhLabelRefRootCauseType {
		t.Fatalf("gc_pause zh label must ride runtimeTraceRootCauseTypeZHLabel, got %q", spec.LabelZhRef)
	}
	// Family-fold lane: the sixth semantic class folds exactly like its five
	// siblings (§24.10 span-grain fold), and the six-class set is exact.
	if CausalTokenFamilyFoldLane("gc_pause") != CausalFamilyFoldSemanticClass {
		t.Fatalf("gc_pause must family-fold on the semantic_class lane")
	}
	semanticClasses := map[string]bool{}
	for token, lane := range causalTokenFamilyFoldLanes {
		if lane == CausalFamilyFoldSemanticClass {
			semanticClasses[token] = true
		}
	}
	want := []string{"jit_compile", "class_verification", "shader_compile", "runtime_compile", "texture_upload", "gc_pause"}
	if len(semanticClasses) != len(want) {
		t.Fatalf("the semantic class census must stay exactly six (a seventh class needs its own ruling): %v", semanticClasses)
	}
	for _, token := range want {
		if !semanticClasses[token] {
			t.Fatalf("semantic class %q missing from the family-fold lane", token)
		}
	}
	// Distinctness: gc_pause (measured span interval) must never share the
	// aggregate memory_gc COUNT family's dimensions.
	memoryGC, ok := CausalTokenSpecFor("memory_gc")
	if !ok {
		t.Fatalf("memory_gc must be registered")
	}
	if memoryGC.Lane == spec.Lane || memoryGC.Additivity == spec.Additivity || memoryGC.Subject == spec.Subject {
		t.Fatalf("gc_pause must stay dimensionally distinct from the aggregate memory_gc count family: gc_pause=%+v memory_gc=%+v", spec, memoryGC)
	}
}
