package types

import (
	"fmt"
	"testing"
)

// aggregateTestRecord builds a trace_query hard-grounded observation with a
// line span — the shape the aggregation rules key on.
func aggregateTestRecord(id, predicate, claimKey, subject, object, value string, impact float64, lineStart, lineEnd int, notes ...string) ObservationRecord {
	base := []string{fmt.Sprintf("impact_ms=%.3f", impact), fmt.Sprintf("cumulative_impact_ms=%.3f", impact)}
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		Predicate:       predicate,
		ClaimKey:        claimKey,
		Subject:         subject,
		Object:          object,
		Value:           value,
		Unit:            "ms",
		Confidence:      0.8,
		SupportRefs:     []string{"obs:" + id},
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		RichNotes:       append(base, notes...),
	}
}

// TestTraceCausalProjectionAggregateRealCustomerShape pins the presentation-v3
// pre-render aggregation against the exact duplication shapes of the real
// customer render (aweme.lite): R1 cross-predicate same-fact twins, the
// primary/hop dual view, R2 tiny same-kind flooding, and R3 unknown-thread
// background flooding.
func TestTraceCausalProjectionAggregateRealCustomerShape(t *testing.T) {
	records := []ObservationRecord{
		// Dual view of the SAME running fact: primary carries chain-cumulative
		// 112.103 with per-layer projection 58.919; the hop twin carries
		// 58.919/58.919 over the SAME line range -> R1 merges, cum keeps max.
		{
			ID: "E1", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: ClaimGroundingHard, Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:running",
			Subject: "#RxComputationT-16816", Object: "running", Value: "112.103", Unit: "ms", Confidence: 0.9,
			SupportRefs: []string{"obs:E1"},
			Span:        ObservationSpan{LineStart: 45689, LineEnd: 79000},
			RichNotes: []string{
				"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
				"impact_ms=58.919", "cumulative_impact_ms=112.103", "actual_impact_ms=59.050", "dominant_state=running",
			},
		},
		{
			ID: "E12", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: ClaimGroundingHard, Predicate: "wakeup_causal_impact", ClaimKey: "wakeup_causal_impact:running",
			Subject: "#RxComputationT-16816", Object: "running", Value: "58.919", Unit: "ms", Confidence: 0.78,
			SupportRefs: []string{"obs:E12"},
			Span:        ObservationSpan{LineStart: 45689, LineEnd: 79000},
			RichNotes: []string{
				"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
				"impact_ms=58.919", "cumulative_impact_ms=58.919", "actual_impact_ms=59.050", "dominant_state=running",
			},
		},
		// R1 cross-predicate twins: an io_latency primary row and the same-interval
		// critical_blocking row naming the udk-irq peer (identical ms + range).
		aggregateTestRecord("E3", "root_cause_primary", "root_cause_primary:io1", "#RxComputationT-16816", "io_latency", "0.568", 0.568, 59938, 60100,
			"rank=5", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		aggregateTestRecord("E14", "critical_blocking", "critical_blocking:io1", "#RxComputationT-16816", "udk-irq-10-90", "0.568", 0.568, 59938, 60100,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		// Two more io_latency instances (distinct ranges) so R2 sees >=3 repeats
		// of (subject, io_latency) AFTER R1 collapsed the twins.
		aggregateTestRecord("E4", "root_cause_primary", "root_cause_primary:io2", "#RxComputationT-16816", "io_latency", "0.500", 0.500, 63943, 64100,
			"rank=6", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		aggregateTestRecord("E15", "critical_blocking", "critical_blocking:io2", "#RxComputationT-16816", "udk-irq-2-77", "0.500", 0.500, 63943, 64100,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		aggregateTestRecord("E5", "root_cause_primary", "root_cause_primary:io3", "#RxComputationT-16816", "io_latency", "0.499", 0.499, 59809, 59900,
			"rank=7", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		// Background flooding: five rows blocked on the unknown-thread sentinel ->
		// keep top-2, fold the rest into one subjectless aggregate.
		aggregateTestRecord("E21", "root_cause_background", "root_cause_background:b1", "isplogcat-1764", "unknown-thread", "117.928", 117.928, 45524, 45524,
			"chain_relevance=background", "causality=background"),
		aggregateTestRecord("E23", "root_cause_background", "root_cause_background:b2", "VSyncGenerator-2290", "unknown-thread", "98.501", 98.501, 53244, 81000,
			"chain_relevance=background", "causality=background"),
		aggregateTestRecord("E24", "root_cause_background", "root_cause_background:b3", "#tp-io-2036-16781", "unknown-thread", "12.689", 12.689, 46055, 50000,
			"chain_relevance=background", "causality=background"),
		aggregateTestRecord("E25", "root_cause_background", "root_cause_background:b4", "CodecLooper-17604", "unknown-thread", "6.886", 6.886, 60560, 65000,
			"chain_relevance=background", "causality=background"),
		aggregateTestRecord("E26", "root_cause_background", "root_cause_background:b5", "ProcessEvent_t-17599", "unknown-thread", "4.355", 4.355, 79382, 81000,
			"chain_relevance=background", "causality=background"),
		{
			ID: "path", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Object: "#RxComputationT-16816 -> .ugc.aweme.lite-16547",
		},
	}

	got := TraceCausalProjectionFromObservationRecords(records)

	// R1 dual view: exactly one running node in primaries, cum=max, per-layer
	// projection kept, hop evidence absorbed.
	var running []TraceCausalProjectionNode
	for _, node := range got.PrimaryRootCauses {
		if node.Object == "running" {
			running = append(running, node)
		}
	}
	if len(running) != 1 {
		t.Fatalf("dual-view running fact should survive exactly once in primaries: %+v", got.PrimaryRootCauses)
	}
	if running[0].CumulativeImpactMS != 112.103 || running[0].ImpactMS != 58.919 {
		t.Fatalf("dual-view merge must keep chain cum (max) and per-layer projection: %+v", running[0])
	}
	if len(running[0].MergedEvidenceIDs) != 1 || running[0].MergedEvidenceIDs[0] != "E12" {
		t.Fatalf("hop twin evidence must be absorbed: %+v", running[0].MergedEvidenceIDs)
	}
	// The hop twin row itself is gone from the hop bucket.
	for _, hop := range got.SupportingHops {
		if hop.EvidenceID == "E12" {
			t.Fatalf("merged hop twin must not survive as its own row: %+v", got.SupportingHops)
		}
	}

	// R1+R2: the io_latency instances collapse into one xN aggregate carrying
	// the twins' evidence and the udk-irq peers as secondary objects.
	var ioAgg *TraceCausalProjectionNode
	for i, node := range got.PrimaryRootCauses {
		if node.Object == "io_latency" {
			if node.MergedCount == 0 {
				t.Fatalf("io_latency rows should aggregate, found bare row: %+v", node)
			}
			if ioAgg != nil {
				t.Fatalf("io_latency should aggregate to ONE row: %+v", got.PrimaryRootCauses)
			}
			ioAgg = &got.PrimaryRootCauses[i]
		}
	}
	if ioAgg == nil {
		t.Fatalf("io_latency aggregate row missing: %+v", got.PrimaryRootCauses)
	}
	if ioAgg.MergedCount != 3 {
		t.Fatalf("io_latency aggregate should count 3 instances: %+v", ioAgg)
	}
	if diff := ioAgg.ImpactMS - 1.567; diff < -0.0005 || diff > 0.0005 {
		t.Fatalf("io_latency aggregate should SUM instances (0.568+0.500+0.499): %+v", ioAgg)
	}
	if ioAgg.MergedMinMS != 0.499 || ioAgg.MergedMaxMS != 0.568 {
		t.Fatalf("io_latency aggregate min-max range lost: %+v", ioAgg)
	}
	wantIDs := map[string]bool{"E14": true, "E15": true, "E4": true, "E5": true}
	for _, id := range ioAgg.MergedEvidenceIDs {
		delete(wantIDs, id)
	}
	if len(wantIDs) != 0 {
		t.Fatalf("aggregate must retain every merged instance id, missing %v: %+v", wantIDs, ioAgg.MergedEvidenceIDs)
	}
	if len(ioAgg.SecondaryObjects) == 0 {
		t.Fatalf("cross-predicate merge should keep the udk-irq peer as secondary object: %+v", ioAgg)
	}

	// R3: background keeps top-2 named rows + one fold row for the rest.
	if len(got.BackgroundCauses) != 3 {
		t.Fatalf("background should fold to top-2 + aggregate, got %d: %+v", len(got.BackgroundCauses), got.BackgroundCauses)
	}
	var fold *TraceCausalProjectionNode
	for i, node := range got.BackgroundCauses {
		if node.MergedCount > 1 {
			fold = &got.BackgroundCauses[i]
		}
	}
	if fold == nil || fold.Subject != "" || fold.MergedCount != 3 {
		t.Fatalf("unknown-thread background fold row missing/incorrect: %+v", got.BackgroundCauses)
	}
	// Pin updated 2026-07-03 (V3, customer revisit): the fold members are
	// DIFFERENT threads, so their wall clocks never sum — the old 23.930ms
	// (12.689+6.886+4.355) pin published a cross-thread sum (real customer: six
	// whole-window 101ms threads rendered 606ms/600%). The fold now publishes
	// the member MAX; the per-member range stays lossless in MergedMin/MaxMS.
	if fold.ImpactMS != 12.689 || fold.CumulativeImpactMS != 12.689 {
		t.Fatalf("background fold must publish the member max, never a cross-thread sum: %+v", fold)
	}
	if fold.MergedMinMS != 4.355 || fold.MergedMaxMS != 12.689 {
		t.Fatalf("background fold min-max range lost: %+v", fold)
	}
	keptSubjects := map[string]bool{}
	for _, node := range got.BackgroundCauses {
		if node.MergedCount <= 1 {
			keptSubjects[node.Subject] = true
		}
	}
	if !keptSubjects["isplogcat-1764"] || !keptSubjects["VSyncGenerator-2290"] {
		t.Fatalf("background fold must keep the top-2 rows: %+v", got.BackgroundCauses)
	}
}

// TestTraceCausalProjectionAggregateStrictGuards pins the STRICT tolerance the
// user adjudicated: rows that differ in ms (even by microseconds) or in line
// range are NEVER merged, and a survivor's own cross-bucket copy is preserved.
func TestTraceCausalProjectionAggregateStrictGuards(t *testing.T) {
	records := []ObservationRecord{
		// Same subject, ms differs by 9µs at the 3rd decimal -> distinct facts.
		aggregateTestRecord("E9", "critical_blocking", "critical_blocking:w1", ".ugc.aweme.lite-16547", "unknown-thread", "112.223", 112.223, 45696, 79000,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=2"),
		aggregateTestRecord("E10", "critical_blocking", "critical_blocking:w2", ".ugc.aweme.lite-16547", "unknown-thread", "112.214", 112.214, 45697, 79000,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=2"),
		// Same subject + same ms but DIFFERENT line range -> distinct facts.
		aggregateTestRecord("E30", "root_cause_primary", "root_cause_primary:r1", "worker-1", "io_wait", "5.000", 5.0, 100, 200,
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
		aggregateTestRecord("E31", "wakeup_causal_impact", "wakeup_causal_impact:r1", "worker-1", "io_wait", "5.000", 5.0, 300, 400,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)

	blocking := 0
	for _, node := range got.OnChainCauses {
		if node.EvidenceID == "E9" || node.EvidenceID == "E10" {
			blocking++
			if node.MergedCount > 1 || len(node.MergedEvidenceIDs) > 0 {
				t.Fatalf("9µs-apart rows must stay separate under strict tolerance: %+v", node)
			}
		}
	}
	if blocking != 2 {
		t.Fatalf("both near-identical (but distinct) blocking rows must survive: %+v", got.OnChainCauses)
	}

	// E30 survives in primaries; E31 (different range) must NOT be absorbed.
	e31Alive := false
	for _, node := range got.SupportingHops {
		if node.EvidenceID == "E31" {
			e31Alive = true
		}
	}
	for _, node := range got.OnChainCauses {
		if node.EvidenceID == "E31" {
			e31Alive = true
		}
	}
	if !e31Alive {
		t.Fatalf("same-ms different-range row must not be merged away: hops=%+v onchain=%+v", got.SupportingHops, got.OnChainCauses)
	}
	// The primary's own on-chain copy (same EvidenceID) is preserved — bucket
	// overlap semantics stay intact for downstream consumers.
	e30InOnChain := false
	for _, node := range got.OnChainCauses {
		if node.EvidenceID == "E30" {
			e30InOnChain = true
		}
	}
	if !e30InOnChain {
		t.Fatalf("survivor's own cross-bucket copy must be preserved: %+v", got.OnChainCauses)
	}
}

// TestTraceCausalProjectionWindowFields pins B2: the requested-window anchor
// populates projection-level WindowStartTs/EndTs, and its absence leaves them
// zero (renderer falls back, never fabricates a window).
func TestTraceCausalProjectionWindowFields(t *testing.T) {
	anchor := ObservationRecord{
		ID: "anchor", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard, Predicate: "frame_target_resolution", ClaimKey: "frame_target_resolution:f1",
		Subject: "app-1", Object: "frame", Span: ObservationSpan{StartTs: 100.5, EndTs: 100.564},
		RichNotes: []string{"window_source=query_window"},
	}
	root := aggregateTestRecord("E1", "root_cause_primary", "root_cause_primary:r", "app-1", "running", "10.000", 10.0, 10, 20,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1")

	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{anchor, root})
	if got.WindowStartTs != 100.5 || got.WindowEndTs != 100.564 {
		t.Fatalf("anchor window should populate projection window fields: %+v", got)
	}
	if ms := got.WindowDurationMS(); ms < 63.999 || ms > 64.001 {
		t.Fatalf("WindowDurationMS should derive 64ms: %f", ms)
	}

	noAnchor := TraceCausalProjectionFromObservationRecords([]ObservationRecord{root})
	if noAnchor.WindowStartTs != 0 || noAnchor.WindowEndTs != 0 || noAnchor.WindowDurationMS() != 0 {
		t.Fatalf("missing anchor must leave window fields zero (fallback mode): %+v", noAnchor)
	}
}
