package types

// trace_causal_projection_g10en_test.go — G10-EN 根修 (QH2-A, 2026-07-14;
// ledger real_trace_campaign_20260705.md §27.4-G10 → §28.7 留账) compile-side
// pins: the holder_self_contradiction_* component quintet assembles into the
// typed BlockingHolderContradictionParts node field (per-lane wording
// source), and a PARTIAL quintet parses to nil (absence never guesses — the
// display lanes then fall back to the legacy verbatim string).
//
// MUTATION self-check: dropping the compile read-in (the
// traceCausalProjectionParseHolderSelfContradiction call) reds the positive
// arm; loosening the all-five gate reds the partial arm.

import "testing"

func g10enWithdrawnBlockingRecord(notes ...string) ObservationRecord {
	rich := append([]string{
		"type=blocking_span",
		"blocking_kind=monitor_contention",
		"holder_self_contradiction=推断持有者 ugc.aweme.lite-16547 自身在同一 payload 持有者 tid 42067 上排队 112.223ms(本段共 115.944ms;行 45696-79136)",
	}, notes...)
	return ObservationRecord{
		ID: "E-g10", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard, Predicate: "critical_blocking",
		ClaimKey: "critical_blocking:E-g10", Subject: "LegoHandler-16865",
		Object: "blocking_span", Value: "115.944", Unit: "ms", Confidence: 0.62,
		SupportRefs: []string{"obs:E-g10"},
		Span:        ObservationSpan{LineStart: 45696, LineEnd: 79136},
		RichNotes:   rich,
	}
}

func g10enFindNode(t *testing.T, projection TraceCausalProjection) TraceCausalProjectionNode {
	t.Helper()
	for _, bucket := range [][]TraceCausalProjectionNode{
		projection.PrimaryRootCauses, projection.OnChainCauses,
		projection.AdjacentCauses, projection.BackgroundCauses,
	} {
		for _, node := range bucket {
			if node.EvidenceID == "E-g10" {
				return node
			}
		}
	}
	t.Fatalf("fixture node E-g10 not compiled: %+v", projection)
	return TraceCausalProjectionNode{}
}

func TestG10ENHolderSelfContradictionPartsCompile(t *testing.T) {
	projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		g10enWithdrawnBlockingRecord(
			"holder_self_contradiction_holder=ugc.aweme.lite-16547",
			"holder_self_contradiction_owner_tid=42067",
			"holder_self_contradiction_queued_ms=112.223",
			"holder_self_contradiction_span_ms=115.944",
			"holder_self_contradiction_lines=45696-79136",
		),
	})
	node := g10enFindNode(t, projection)
	parts := node.BlockingHolderContradictionParts
	if parts == nil {
		t.Fatalf("the component quintet must assemble into the typed node field, got %+v", node)
	}
	want := TraceHolderSelfContradictionWitness{
		Holder: "ugc.aweme.lite-16547", OwnerTid: 42067,
		QueuedMs: 112.223, SpanMs: 115.944,
		LineStart: 45696, LineEnd: 79136,
	}
	if *parts != want {
		t.Fatalf("components drifted: got %+v want %+v", *parts, want)
	}
	// Round-trip wording: the components' zh wording is byte-identical to
	// the legacy string note (zero zh display regression by construction).
	if parts.WitnessText(true) != node.BlockingHolderContradiction {
		t.Fatalf("zh wording must round-trip byte-identically:\n%q\n%q",
			parts.WitnessText(true), node.BlockingHolderContradiction)
	}
}

func TestG10ENHolderSelfContradictionPartialQuintetParsesNil(t *testing.T) {
	// Each partial form drops the whole set — the legacy string survives as
	// the both-lane fallback; no component is ever guessed.
	partials := [][]string{
		{ // no holder label
			"holder_self_contradiction_owner_tid=42067",
			"holder_self_contradiction_queued_ms=112.223",
			"holder_self_contradiction_span_ms=115.944",
			"holder_self_contradiction_lines=45696-79136",
		},
		{ // no line range
			"holder_self_contradiction_holder=ugc.aweme.lite-16547",
			"holder_self_contradiction_owner_tid=42067",
			"holder_self_contradiction_queued_ms=112.223",
			"holder_self_contradiction_span_ms=115.944",
		},
		{ // zero durations
			"holder_self_contradiction_holder=ugc.aweme.lite-16547",
			"holder_self_contradiction_owner_tid=42067",
			"holder_self_contradiction_queued_ms=0.000",
			"holder_self_contradiction_span_ms=115.944",
			"holder_self_contradiction_lines=45696-79136",
		},
	}
	for i, notes := range partials {
		projection := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
			g10enWithdrawnBlockingRecord(notes...),
		})
		node := g10enFindNode(t, projection)
		if node.BlockingHolderContradictionParts != nil {
			t.Fatalf("partial form %d must parse to nil, got %+v", i, node.BlockingHolderContradictionParts)
		}
		if node.BlockingHolderContradiction == "" {
			t.Fatalf("partial form %d must keep the legacy verbatim fallback", i)
		}
	}
}
