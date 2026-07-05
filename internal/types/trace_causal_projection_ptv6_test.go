package types

// trace_causal_projection_ptv6_test.go — PTV6 批① (real-trace campaign
// 2026-07-06, 标本归因 #1/#2/#9 + PTS 连带) types-layer pins:
//
//   #1a — SupportingHops admission carries an ON-CHAIN gate (typed signals
//         only: chain_relevance=on_chain / causality=on_wakeup_chain via the
//         single strict parser, root_evidence: family, or the wakeup-chain-
//         by-construction families when relevance is unstated). A background
//         critical_blocking record (the donghu_short specimen shape) stays
//         OUT of the hops lane — its background-stanza seat is its ONLY seat.
//         MUTATION: reverting to predicate-only admission must red.
//   PTS 连带 — the former silent hops[:10] discard folds with a count
//         (post-aggregation truth), EXCEPT overflow members already
//         represented on the on-chain bucket surface (deliberate bucket
//         overlap — folding those would mint a fake 其余 N 项 of rows that
//         render anyway; 复核 P1 教训).

import (
	"fmt"
	"strings"
	"testing"
)

func ptv6HopRecord(id, predicate, claimKey, subject string, impact float64, lineStart, lineEnd int, notes ...string) ObservationRecord {
	return ObservationRecord{
		ID: id, Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: predicate, ClaimKey: claimKey,
		Subject: subject, Object: "runnable_wait",
		Value: fmt.Sprintf("%.3f", impact), Unit: "ms",
		// Real producer records carry per-record line-range SupportRefs — the
		// bucket dedupe keys on them, so fixtures must too.
		SupportRefs: []string{fmt.Sprintf("xxx_all.systrace:%d-%d", lineStart, lineEnd)},
		Span:        ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		RichNotes:   append([]string{fmt.Sprintf("impact_ms=%.3f", impact)}, notes...),
		Confidence:  0.8,
	}
}

func ptv6HopIDs(nodes []TraceCausalProjectionNode) map[string]bool {
	out := map[string]bool{}
	for _, node := range nodes {
		if id := strings.TrimSpace(node.EvidenceID); id != "" {
			out[id] = true
		}
		for _, id := range node.MergedEvidenceIDs {
			out[strings.TrimSpace(id)] = true
		}
	}
	return out
}

func TestTraceCausalProjectionHopAdmissionRequiresOnChainSignal(t *testing.T) {
	records := []ObservationRecord{
		// 标本形态 (donghu_short absolute-20260706-013848): critical_blocking
		// chain_relevance=background — the double-cast that hung └─唤醒─
		// phantom children off the 🎯 target.
		ptv6HopRecord("cb-bg", "critical_blocking", "critical_blocking:io_latency",
			"sysevent_store-47924", 1.383, 2913, 3120,
			"type=io_latency", "chain_relevance=background", "nearest_chain_thread=CookieMonsterCl-59843"),
		// Typed on-chain critical_blocking: admitted.
		ptv6HopRecord("cb-on", "critical_blocking", "critical_blocking:blocking_span",
			"holder-11", 2.100, 100, 120,
			"type=blocking_span", "chain_relevance=on_chain"),
		// causality-only (single strict parser resolves on_chain): admitted.
		ptv6HopRecord("imp-causality", "wakeup_causal_impact", "wakeup_causal_impact:worker-3",
			"worker-3", 4.000, 200, 220,
			"causality=on_wakeup_chain"),
		// root_evidence audit family (no relevance note by design): admitted.
		ptv6HopRecord("root-ev", "root_evidence_probe", "root_evidence:probe",
			"prober-5", 0.900, 300, 320),
		// Micro-probe family with UNSTATED relevance: must state membership.
		ptv6HopRecord("cb-bare", "critical_blocking", "critical_blocking:d_state_or_io_wait",
			"bare-7", 1.100, 400, 420,
			"type=d_state_or_io_wait"),
		// Wakeup-CHAIN-view family with unstated relevance (the REAL producer
		// aggregate shape carries no relevance/causality note): on-chain by
		// construction, admitted.
		ptv6HopRecord("agg-bare", "wakeup_causal_aggregate", "wakeup_causal_aggregate:CookieMonsterCl-59843",
			"CookieMonsterCl-59843", 42.131, 3265, 4500,
			"dominant_state=s_sleep"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	hops := ptv6HopIDs(got.SupportingHops)
	for _, want := range []string{"cb-on", "imp-causality", "root-ev", "agg-bare"} {
		if !hops[want] {
			t.Fatalf("on-chain hop %s must be admitted to SupportingHops: %+v", want, got.SupportingHops)
		}
	}
	// MUTATION (predicate-only admission 必红): the background / unstated
	// micro-probe rows never reach the hops lane.
	for _, banned := range []string{"cb-bg", "cb-bare"} {
		if hops[banned] {
			t.Fatalf("record %s must not double-cast into SupportingHops (predicate-only admission revived?): %+v",
				banned, got.SupportingHops)
		}
	}
	// The background record's classified copy keeps its ONLY seat; [P1 修正轮]
	// the UNDECLARED micro-probe row defaults into the background seat too —
	// zero-seat invisibility is the bug this pins against.
	background := ptv6HopIDs(got.BackgroundCauses)
	for _, want := range []string{"cb-bg", "cb-bare"} {
		if !background[want] {
			t.Fatalf("record %s must keep a background-stanza seat: %+v", want, got.BackgroundCauses)
		}
	}
}

// TestTraceCausalProjectionCriticalBlockingSeatMatrix ([P1 修正轮 2026-07-06])
// pins the three-shape seat matrix for the micro-probe family:
//
//	on_chain 声明   → SupportingHops + OnChainCauses,永不背景席
//	background 声明 → BackgroundCauses only
//	未声明          → BackgroundCauses only(缺省=背景保席;自声明才可能上链)
//
// and the P1 regression itself: a LONE undeclared record keeps the projection
// Active with a ▒ seat instead of vanishing entirely.
func TestTraceCausalProjectionCriticalBlockingSeatMatrix(t *testing.T) {
	records := []ObservationRecord{
		ptv6HopRecord("m-on", "critical_blocking", "critical_blocking:blocking_span",
			"declared-on-1", 3.000, 100, 120, "type=blocking_span", "chain_relevance=on_chain"),
		ptv6HopRecord("m-bg", "critical_blocking", "critical_blocking:io_latency",
			"declared-bg-2", 2.000, 200, 220, "type=io_latency", "chain_relevance=background"),
		ptv6HopRecord("m-bare", "critical_blocking", "critical_blocking:d_state_or_io_wait",
			"undeclared-3", 1.000, 300, 320, "type=d_state_or_io_wait"),
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	hops := ptv6HopIDs(got.SupportingHops)
	onChain := ptv6HopIDs(got.OnChainCauses)
	background := ptv6HopIDs(got.BackgroundCauses)
	matrix := []struct {
		id                        string
		hop, onChain, backgroundS bool
	}{
		{"m-on", true, true, false},
		{"m-bg", false, false, true},
		{"m-bare", false, false, true},
	}
	for _, want := range matrix {
		if hops[want.id] != want.hop || onChain[want.id] != want.onChain || background[want.id] != want.backgroundS {
			t.Fatalf("seat matrix for %s: hops=%v onChain=%v background=%v, want %v/%v/%v",
				want.id, hops[want.id], onChain[want.id], background[want.id],
				want.hop, want.onChain, want.backgroundS)
		}
	}
	// P1 regression: a lone undeclared record must not flip Active() false.
	lone := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		ptv6HopRecord("m-lone", "critical_blocking", "critical_blocking:d_state_or_io_wait",
			"undeclared-lone", 1.500, 400, 420, "type=d_state_or_io_wait"),
	})
	if !lone.Active() {
		t.Fatalf("a lone undeclared micro-probe row must keep the projection active (zero-seat invisibility)")
	}
	if !ptv6HopIDs(lone.BackgroundCauses)["m-lone"] || len(lone.SupportingHops) != 0 || len(lone.OnChainCauses) != 0 {
		t.Fatalf("lone undeclared row must seat in ▒ ONLY: bg=%+v hops=%+v onchain=%+v",
			lone.BackgroundCauses, lone.SupportingHops, lone.OnChainCauses)
	}
}

// TestTraceCausalProjectionNearLaneSubjectSentinelGate ([Med 修正轮 2026-07-06])
// pins the near lane's SUBJECT sentinel gate: an unknown-thread subject pair
// with near (≤3%) values over overlapping ranges must stay two rows — the
// sentinel carries no identity on either leg of the "one republished
// measurement" assertion.
func TestTraceCausalProjectionNearLaneSubjectSentinelGate(t *testing.T) {
	record := func(id, subject string, impact float64, ls, le int) ObservationRecord {
		r := ptv6HopRecord(id, "critical_blocking", "critical_blocking:io_latency",
			subject, impact, ls, le, "type=io_latency", "chain_relevance=background")
		r.Object = "udk-irq-1-63"
		return r
	}
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		record("s1", "unknown-thread", 1.354, 2908, 3094),
		record("s2", "unknown-thread", 1.383, 2913, 3120),
	})
	rows := 0
	for _, node := range got.BackgroundCauses {
		if node.DuplicatePublications > 1 {
			t.Fatalf("sentinel-subject near pair must not fold: %+v", node)
		}
		rows++
	}
	if rows != 2 {
		t.Fatalf("sentinel-subject near pair must stay two rows, got %d: %+v", rows, got.BackgroundCauses)
	}
	// Control arm: the same pair under a REAL subject folds (the band is live).
	folded := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		record("r1", "sysevent_store-47924", 1.354, 2908, 3094),
		record("r2", "sysevent_store-47924", 1.383, 2913, 3120),
	})
	if len(folded.BackgroundCauses) != 1 || folded.BackgroundCauses[0].DuplicatePublications != 2 {
		t.Fatalf("real-subject near pair must fold with a publication count: %+v", folded.BackgroundCauses)
	}
}

func TestTraceCausalProjectionSupportingHopsOverflowFoldsWithCount(t *testing.T) {
	// 14 hops-only rows (root_evidence family: no relevance note → never in a
	// relevance bucket, SupportingHops is their only surface) — the former
	// hops[:10] silently dropped four of them.
	var records []ObservationRecord
	for i := 1; i <= 14; i++ {
		records = append(records, ptv6HopRecord(
			fmt.Sprintf("hop-%02d", i), "root_evidence_probe",
			fmt.Sprintf("root_evidence:probe_%d", i),
			fmt.Sprintf("prober-%d", i), 20.0-float64(i), 100*i, 100*i+10,
			fmt.Sprintf("chain_depth=%d", i)))
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.SupportingHops) != traceCausalProjectionSupportingHopLimit+1 {
		t.Fatalf("overflow must keep %d hops + 1 fold row, got %d: %+v",
			traceCausalProjectionSupportingHopLimit, len(got.SupportingHops), got.SupportingHops)
	}
	fold := got.SupportingHops[len(got.SupportingHops)-1]
	if !fold.OnChainOverflowFold {
		t.Fatalf("the hop overflow must materialize as the typed fold row: %+v", fold)
	}
	if fold.MergedCount != 14-traceCausalProjectionSupportingHopLimit {
		t.Fatalf("fold must COUNT the folded hops: got %d want %d", fold.MergedCount, 14-traceCausalProjectionSupportingHopLimit)
	}
	// 零静默丢弃逐 id 核对: every record's evidence id survives on some hop
	// (kept row or the fold's absorbed evidence).
	seen := ptv6HopIDs(got.SupportingHops)
	for _, record := range records {
		if !seen[record.ID] {
			t.Fatalf("hop %s silently dropped from SupportingHops", record.ID)
		}
	}
}

func TestTraceCausalProjectionSupportingHopsOverflowRepresentedOnChainDoesNotFold(t *testing.T) {
	// 14 on_chain critical_blocking rows: every one ALSO classifies into the
	// (uncapped) on-chain bucket — the hop overflow is the deliberate bucket
	// overlap, NOT a silent drop, so no fold row may mint a fake count (复核
	// P1: no 其余 N 项 made of rows that render anyway).
	var records []ObservationRecord
	for i := 1; i <= 14; i++ {
		records = append(records, ptv6HopRecord(
			fmt.Sprintf("cb-%02d", i), "critical_blocking",
			fmt.Sprintf("critical_blocking:span_%d", i),
			fmt.Sprintf("blocker-%d", i), 20.0-float64(i), 100*i, 100*i+10,
			"type=blocking_span", "chain_relevance=on_chain", fmt.Sprintf("chain_depth=%d", i)))
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.SupportingHops) != traceCausalProjectionSupportingHopLimit {
		t.Fatalf("represented overflow must keep exactly the cap, no fold row: %d", len(got.SupportingHops))
	}
	for _, hop := range got.SupportingHops {
		if hop.OnChainOverflowFold {
			t.Fatalf("no fold row may claim members that render via the on-chain bucket: %+v", hop)
		}
	}
	onChain := ptv6HopIDs(got.OnChainCauses)
	for _, record := range records {
		if !onChain[record.ID] {
			t.Fatalf("overflow hop %s must stay represented on the on-chain bucket surface: %+v",
				record.ID, got.OnChainCauses)
		}
	}
}

func TestTraceCausalProjectionSupportingHopsUnderCapNoFoldRow(t *testing.T) {
	// 突变形态: at/under the cap the hops surface stays fold-free.
	var records []ObservationRecord
	for i := 1; i <= traceCausalProjectionSupportingHopLimit; i++ {
		records = append(records, ptv6HopRecord(
			fmt.Sprintf("hop-%02d", i), "root_evidence_probe",
			fmt.Sprintf("root_evidence:probe_%d", i),
			fmt.Sprintf("prober-%d", i), 20.0-float64(i), 100*i, 100*i+10))
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.SupportingHops) != traceCausalProjectionSupportingHopLimit {
		t.Fatalf("under-cap hops must keep every row: %d", len(got.SupportingHops))
	}
	for _, hop := range got.SupportingHops {
		if hop.OnChainOverflowFold {
			t.Fatalf("under-cap hops must not mint a fold row: %+v", hop)
		}
	}
}
