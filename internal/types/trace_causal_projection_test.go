package types

import (
	"fmt"
	"strconv"
	"testing"
)

func TestTraceCausalProjectionPrefersAttributableChainRootCause(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		traceProjectionTestRoot("root-app", "app-100", "compute_supply", "0.020", 0.02, 0.90, 1),
		traceProjectionTestRoot("root-pressure", "unknown-thread", "io_pressure", "16.000", 16.0, 0.95, 1),
		traceProjectionTestRoot("root-threadpool", "threadpool-400", "io_wait", "11.000", 11.0, 0.86, 4),
		{
			ID:              "path",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_chain",
			ClaimKey:        "wakeup_chain:path",
			Object:          "threadpool-400 -> network-300 -> cookie-200 -> app-100",
		},
		{
			ID:              "hop",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_causal_impact",
			Subject:         "threadpool-400",
			Object:          "io_wait",
			Value:           "11.000",
			Unit:            "ms",
			RichNotes:       []string{"causality=on_wakeup_chain", "chain_depth=1", "impact=11.000ms"},
		},
		{
			ID:              "hop-duplicate",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_causal_impact",
			Subject:         "threadpool-400",
			Object:          "io_wait",
			Value:           "11.000",
			Unit:            "ms",
			RichNotes:       []string{"causality=on_wakeup_chain", "chain_depth=1", "impact=11.000ms"},
		},
	}}

	got := CompileTraceCausalProjection(ledger)
	if got.PrimaryRootCause == nil {
		t.Fatal("expected a primary root cause")
	}
	if got.PrimaryRootCause.Subject != "threadpool-400" ||
		got.PrimaryRootCause.Object != "io_wait" ||
		got.PrimaryRootCause.CumulativeImpactMS != 11.0 {
		t.Fatalf("primary root cause should prefer attributable wakeup-chain nodes over aggregate sentinels, got %+v", got.PrimaryRootCause)
	}
	if len(got.WakeupPath) != 4 || got.WakeupPath[0] != "threadpool-400" || got.WakeupPath[3] != "app-100" {
		t.Fatalf("wakeup path not projected: %+v", got.WakeupPath)
	}
	if len(got.SupportingHops) != 1 || got.SupportingHops[0].Subject != "threadpool-400" {
		t.Fatalf("supporting causal hops should be projected once: %+v", got.SupportingHops)
	}
}

func TestTraceCausalProjectionPreservesMultiLayerChainRelevance(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		traceProjectionTestRootWithNotes("root-on-chain", "binder-100", "runnable_delay", "9.000", 9.0, 0.93, 1, []string{
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
		}),
		traceProjectionTestRootWithNotes("root-adjacent", "io-200", "io_wait", "7.000", 7.0, 0.90, 2, []string{
			"chain_relevance=adjacent",
			"causality=adjacent_to_wakeup_chain",
		}),
		{
			ID:              "root-background",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "root_cause_background",
			ClaimKey:        "root_cause_background",
			Subject:         "disk-300",
			Object:          "background_io_pressure",
			Value:           "18.000",
			Unit:            "ms",
			RichNotes: []string{
				"rank=3",
				"tier=supporting",
				"chain_relevance=background",
				"causality=background",
				"impact_ms=18.000",
				"cumulative_impact_ms=18.000",
			},
			Confidence: 0.88,
		},
		{
			ID:              "path",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_chain",
			ClaimKey:        "wakeup_chain:path",
			Object:          "binder-100 -> target-400",
		},
	}}

	got := CompileTraceCausalProjection(ledger)
	if got.PrimaryRootCause == nil || got.PrimaryRootCause.Subject != "binder-100" {
		t.Fatalf("expected on-chain primary to remain first, got %+v", got.PrimaryRootCause)
	}
	if len(got.PrimaryRootCauses) != 2 {
		t.Fatalf("expected both primary root-cause layers to be retained, got %+v", got.PrimaryRootCauses)
	}
	if got.PrimaryRootCauses[0].ChainRelevance != "on_chain" ||
		got.PrimaryRootCauses[1].ChainRelevance != "adjacent" {
		t.Fatalf("primary layers should preserve typed chain relevance: %+v", got.PrimaryRootCauses)
	}
	if len(got.OnChainCauses) != 1 || got.OnChainCauses[0].Subject != "binder-100" {
		t.Fatalf("on-chain cause projection missing: %+v", got.OnChainCauses)
	}
	if len(got.AdjacentCauses) != 1 || got.AdjacentCauses[0].Subject != "io-200" {
		t.Fatalf("adjacent cause projection missing: %+v", got.AdjacentCauses)
	}
	if len(got.BackgroundCauses) != 1 || got.BackgroundCauses[0].Subject != "disk-300" {
		t.Fatalf("background cause projection missing: %+v", got.BackgroundCauses)
	}
}

func TestTraceCausalProjectionKeepsSemanticSpanOptimizationPoints(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		traceProjectionTestRootWithNotes("root-on-chain", "worker-200", "sleep_wait", "28.000", 28.0, 0.90, 1, []string{
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=1",
		}),
		{
			ID:              "semantic-on-chain",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "trace_semantic_span",
			ClaimKey:        "trace_semantic_span:class_verification",
			Subject:         "worker-200",
			Object:          "class_verification",
			Value:           "2.000",
			Unit:            "ms",
			RichNotes: []string{
				"span_name=VerifyClass com.example.Foo",
				"span_kind=sync",
				"span_category=runtime_verification",
				"span_subcategory=class_verification",
				"semantic_class=class_verification",
				"chain_relevance=on_chain",
				"causality=on_wakeup_chain",
				"chain_depth=1",
			},
			Confidence: 0.82,
		},
		{
			ID:              "semantic-background",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "trace_semantic_span",
			ClaimKey:        "trace_semantic_span:shader_compile",
			Subject:         "render-900",
			Object:          "shader_compile",
			Value:           "12.000",
			Unit:            "ms",
			RichNotes: []string{
				"span_name=ShaderCompile pipeline warmup",
				"semantic_class=shader_compile",
				"chain_relevance=background",
				"causality=background",
			},
			Confidence: 0.62,
		},
	}}

	got := CompileTraceCausalProjection(ledger)
	if len(got.SemanticSpans) != 2 {
		t.Fatalf("semantic span bucket should preserve on-chain and background optimization points, got %+v", got.SemanticSpans)
	}
	if got.SemanticSpans[0].Subject != "worker-200" ||
		got.SemanticSpans[0].SemanticClass != "class_verification" ||
		got.SemanticSpans[0].SpanName != "VerifyClass com.example.Foo" ||
		got.SemanticSpans[0].ChainRelevance != "on_chain" ||
		got.SemanticSpans[0].ChainDepth != 1 {
		t.Fatalf("on-chain semantic span should be first and fully typed: %+v", got.SemanticSpans)
	}
	if len(got.OnChainCauses) < 2 {
		t.Fatalf("semantic span should also appear in on-chain context bucket: %+v", got.OnChainCauses)
	}
	if len(got.BackgroundCauses) != 1 || got.BackgroundCauses[0].SemanticClass != "shader_compile" {
		t.Fatalf("background semantic span should remain supporting/background: %+v", got.BackgroundCauses)
	}
}

func TestTraceCausalProjectionExpandsSemanticSpanBucket(t *testing.T) {
	records := []ObservationRecord{
		traceProjectionTestRootWithNotes("root-on-chain", "worker-200", "sleep_wait", "28.000", 28.0, 0.90, 1, []string{
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=1",
		}),
	}
	semanticClasses := []string{"jit_compile", "class_verification", "shader_compile", "runtime_compile"}
	for i := 1; i <= 14; i++ {
		class := semanticClasses[(i-1)%len(semanticClasses)]
		records = append(records, ObservationRecord{
			ID:              fmt.Sprintf("semantic-%02d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "trace_semantic_span",
			ClaimKey:        "trace_semantic_span:" + class,
			Subject:         fmt.Sprintf("dep-%02d", i),
			Object:          class,
			Value:           fmt.Sprintf("%d.000", 30-i),
			Unit:            "ms",
			RichNotes: []string{
				fmt.Sprintf("span_name=%s-%02d", class, i),
				"semantic_class=" + class,
				"chain_relevance=on_chain",
				"causality=on_wakeup_chain",
				fmt.Sprintf("chain_depth=%d", i),
			},
			Confidence: 0.82,
		})
	}

	got := CompileTraceCausalProjection(ObservationLedger{Records: records})
	if len(got.SemanticSpans) != 12 {
		t.Fatalf("semantic span bucket should keep the expanded bounded surface, got %d: %+v", len(got.SemanticSpans), got.SemanticSpans)
	}
	if got.SemanticSpans[0].Subject != "dep-01" || got.SemanticSpans[11].Subject != "dep-12" {
		t.Fatalf("semantic span ordering should remain typed and deterministic, got %+v", got.SemanticSpans)
	}
	if len(got.OnChainCauses) < 12 {
		t.Fatalf("expanded on-chain bucket should retain semantic optimization context, got %+v", got.OnChainCauses)
	}
}

func TestTraceCausalProjectionKeepsOnChainPrimaryAheadOfLargerBackground(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		traceProjectionTestRootWithNotes("root-background", "logger-900", "io_pressure", "80.000", 80.0, 0.95, 1, []string{
			"chain_relevance=background",
			"causality=background",
		}),
		traceProjectionTestRootWithNotes("root-on-chain", "worker-200", "sleep_wait", "12.000", 12.0, 0.86, 2, []string{
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=1",
		}),
		{
			ID:              "path",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_chain",
			ClaimKey:        "wakeup_chain:path",
			Object:          "worker-200 -> app-100",
		},
	}}

	got := CompileTraceCausalProjection(ledger)
	if got.PrimaryRootCause == nil || got.PrimaryRootCause.Subject != "worker-200" {
		t.Fatalf("on-chain root cause must stay primary even when background impact is larger, got %+v", got.PrimaryRootCause)
	}
	if len(got.PrimaryRootCauses) < 2 ||
		got.PrimaryRootCauses[0].Subject != "worker-200" ||
		got.PrimaryRootCauses[1].Subject != "logger-900" {
		t.Fatalf("primary ordering should preserve on-chain before background: %+v", got.PrimaryRootCauses)
	}
	if len(got.OnChainCauses) != 1 || got.OnChainCauses[0].Subject != "worker-200" {
		t.Fatalf("on-chain bucket missing selected cause: %+v", got.OnChainCauses)
	}
	if len(got.BackgroundCauses) != 1 || got.BackgroundCauses[0].Subject != "logger-900" {
		t.Fatalf("background pressure should remain visible as context, got %+v", got.BackgroundCauses)
	}
}

func TestTraceCausalProjectionPreservesMultiHopPathWithRunningWaker(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		traceProjectionTestRootWithNotes("root-io", "threadpool-400", "io_burst_episode", "119.000", 119.0, 0.88, 1, []string{
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=1",
		}),
		traceProjectionTestRootWithNotes("root-running", "worker-200", "running", "118.000", 118.0, 0.86, 2, []string{
			"chain_relevance=on_chain",
			"causality=on_wakeup_chain",
			"chain_depth=2",
		}),
		{
			ID:              "path",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_chain",
			ClaimKey:        "wakeup_chain:path",
			Object:          "threadpool-400 -> worker-200 -> app-100",
		},
	}}

	got := CompileTraceCausalProjection(ledger)
	if len(got.PrimaryRootCauses) != 2 {
		t.Fatalf("projection should preserve co-primary on-chain layers, got %+v", got.PrimaryRootCauses)
	}
	if got.PrimaryRootCauses[0].Subject != "threadpool-400" || got.PrimaryRootCauses[1].Subject != "worker-200" {
		t.Fatalf("projection should keep ordered on-chain root layers, got %+v", got.PrimaryRootCauses)
	}
	if len(got.WakeupPath) != 3 || got.WakeupPath[0] != "threadpool-400" || got.WakeupPath[1] != "worker-200" || got.WakeupPath[2] != "app-100" {
		t.Fatalf("multi-hop wakeup path must be preserved for answer rendering, got %+v", got.WakeupPath)
	}
}

func TestTraceCausalProjectionKeepsDefaultDepthSupportingHops(t *testing.T) {
	records := []ObservationRecord{{
		ID:              "path",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "wakeup_chain",
		ClaimKey:        "wakeup_chain:path",
		Object:          "dep-1 -> dep-2 -> dep-3 -> dep-4 -> dep-5 -> dep-6 -> dep-7 -> dep-8 -> dep-9 -> dep-10 -> dep-11 -> dep-12 -> app-100",
	}}
	for depth := 1; depth <= 12; depth++ {
		records = append(records, ObservationRecord{
			ID:              fmt.Sprintf("hop-%02d", depth),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_causal_impact",
			ClaimKey:        fmt.Sprintf("wakeup_causal_impact:dep-%d", depth),
			Subject:         fmt.Sprintf("dep-%d", depth),
			Object:          "sleep_wait",
			Value:           "1.000",
			Unit:            "ms",
			RichNotes: []string{
				"causality=on_wakeup_chain",
				fmt.Sprintf("depth=%d", depth),
				"impact=1.000ms",
			},
			Confidence: 0.80,
		})
	}

	got := CompileTraceCausalProjection(ObservationLedger{Records: records})
	if len(got.SupportingHops) != 10 {
		t.Fatalf("supporting hops should align with trace_query default max_depth=10, got %d: %+v", len(got.SupportingHops), got.SupportingHops)
	}
	if got.SupportingHops[0].Subject != "dep-1" || got.SupportingHops[0].ChainDepth != 1 ||
		got.SupportingHops[9].Subject != "dep-10" || got.SupportingHops[9].ChainDepth != 10 {
		t.Fatalf("supporting hops should preserve ordered depth alias values, got %+v", got.SupportingHops)
	}
	if len(got.OnChainCauses) < 10 || got.OnChainCauses[9].Subject != "dep-10" {
		t.Fatalf("on-chain bucket should preserve the default-depth causal surface, got %+v", got.OnChainCauses)
	}
	for _, hop := range got.SupportingHops {
		if hop.Subject == "dep-11" || hop.Subject == "dep-12" {
			t.Fatalf("projection should stay bounded at default depth 10, got %+v", got.SupportingHops)
		}
	}
}

func traceProjectionTestRoot(id, subject, object, value string, cumulative, confidence float64, rank int) ObservationRecord {
	return traceProjectionTestRootWithNotes(id, subject, object, value, cumulative, confidence, rank, []string{
		"causality=on_wakeup_chain",
	})
}

func traceProjectionTestRootWithNotes(id, subject, object, value string, cumulative, confidence float64, rank int, extraNotes []string) ObservationRecord {
	notes := []string{
		"rank=" + strconv.Itoa(rank),
		"tier=primary",
		"impact_ms=" + value,
		"cumulative_impact_ms=" + fmt.Sprintf("%.3f", cumulative),
	}
	notes = append(notes, extraNotes...)
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary",
		Subject:         subject,
		Object:          object,
		Value:           value,
		Unit:            "ms",
		RichNotes:       notes,
		Confidence:      confidence,
	}
}
