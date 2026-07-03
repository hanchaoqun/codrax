package types

import (
	"strings"
	"testing"
)

// F-round pins (adversarial review of the V1–V5 fixes, 2026-07-03):
//   F2 — an R2 ×N SUM row NEVER carries DuplicatePublications: the typed
//        contract "dup>0 ⇒ the row's value is ONE republished measurement"
//        cannot hold on a SUM, and inheriting the group-first survivor's count
//        once rendered the mutually-exclusive ×2同值合并(重复发布) and ×3合并
//        labels on one row. Member dup provenance stays lossless through
//        MergedEvidenceIDs — no second counter exists.
//   F3 support — the R3 unknown-background fold propagates the members' typed
//        StateKind only under strict unanimity, so the renderer's wait-family
//        idle gate keeps working on uniform whole-window sleeper folds without
//        ever fabricating a state on mixed/stateless folds.

// --- F2: R2 SUM clears DuplicatePublications ---------------------------------------

func fRoundDupRecord(id string, impact float64, lineStart, lineEnd int) ObservationRecord {
	return aggregateTestRecord(id, "root_cause_context", "root_cause_context:"+id,
		"irq_handler-100", "irq_activity", "5.000", impact, lineStart, lineEnd,
		"chain_relevance=adjacent", "causality=adjacent_to_chain")
}

// Review probe shape (group-first survivor): three duplicate publications of
// one 5.0ms measurement (dedup-folds to a dup=3 survivor BEFORE R2) plus two
// genuinely distinct bursts of the same (subject, object) — R2 then legally
// sums the three surviving rows, and the SUM row must NOT keep the survivor's
// duplicate-publication count.
func TestTraceCausalProjectionR2SumClearsInheritedDuplicatePublications(t *testing.T) {
	records := []ObservationRecord{
		fRoundDupRecord("D1", 5.0, 100, 200),
		fRoundDupRecord("D2", 5.0, 105, 205), // duplicate publication of D1
		fRoundDupRecord("D3", 5.0, 110, 210), // duplicate publication of D1
		fRoundDupRecord("B1", 3.0, 300, 400), // distinct burst
		fRoundDupRecord("B2", 4.0, 500, 600), // distinct burst
	}
	got := TraceCausalProjectionFromObservationRecords(records)
	if len(got.AdjacentCauses) != 1 {
		t.Fatalf("dedup(3→1) + R2(×3) must leave ONE aggregate row, got %d: %+v",
			len(got.AdjacentCauses), got.AdjacentCauses)
	}
	agg := got.AdjacentCauses[0]
	if agg.MergedCount != 3 {
		t.Fatalf("the three post-dedup survivors legally SUM as ×3: %+v", agg)
	}
	if diff := agg.ImpactMS - 12.0; diff < -0.0005 || diff > 0.0005 {
		t.Fatalf("R2 sum of the survivors is 5+3+4=12.0: %+v", agg)
	}
	if agg.DuplicatePublications != 0 {
		t.Fatalf("a SUM row must never carry DuplicatePublications (dup>0 ⇒ single measurement): %+v", agg)
	}
	// Provenance stays lossless: the dup twins AND the burst members all remain
	// auditable from the aggregate's evidence roster.
	ids := strings.Join(agg.MergedEvidenceIDs, ",")
	for _, want := range []string{"D2", "D3", "B1", "B2"} {
		if !strings.Contains(ids, want) {
			t.Fatalf("member/dup evidence id %s lost from the SUM row roster: %+v", want, agg)
		}
	}
}

// Review probe shape (survivor NOT group-first): the dup-fold survivor joins
// an R2 group behind a larger row — its count must be cleared on the SUM row
// exactly like the inherited case (no silent half-state), with its evidence
// ids retained.
func TestTraceCausalProjectionR2SumClearsNonFirstMemberDuplicatePublications(t *testing.T) {
	nodes := []TraceCausalProjectionNode{
		{EvidenceID: "A", Subject: "worker-1", Object: "io_latency",
			ImpactMS: 10.0, CumulativeImpactMS: 10.0, LineStart: 100, LineEnd: 110},
		{EvidenceID: "B", Subject: "worker-1", Object: "io_latency",
			ImpactMS: 5.0, CumulativeImpactMS: 5.0, LineStart: 200, LineEnd: 210,
			DuplicatePublications: 2, MergedEvidenceIDs: []string{"B2"}},
		{EvidenceID: "C", Subject: "worker-1", Object: "io_latency",
			ImpactMS: 3.0, CumulativeImpactMS: 3.0, LineStart: 300, LineEnd: 310},
	}
	out := traceCausalProjectionAggregateSameKind(nodes)
	if len(out) != 1 || out[0].MergedCount != 3 {
		t.Fatalf("expected one ×3 SUM row: %+v", out)
	}
	agg := out[0]
	if agg.DuplicatePublications != 0 {
		t.Fatalf("a non-first member's dup count must not survive onto the SUM row: %+v", agg)
	}
	ids := strings.Join(agg.MergedEvidenceIDs, ",")
	for _, want := range []string{"B", "B2", "C"} {
		if !strings.Contains(ids, want) {
			t.Fatalf("member evidence id %s lost from the SUM row roster: %+v", want, agg)
		}
	}
}

// --- F3 support: R3 fold StateKind propagation (strict unanimity) ------------------

func fRoundBGNode(id, subject, state string) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		EvidenceID: id, Subject: subject, Object: "unknown-thread",
		ImpactMS: 101.0, CumulativeImpactMS: 101.0,
		ChainRelevance: "background", StateKind: state,
	}
}

func TestTraceCausalProjectionUnknownBackgroundFoldStateKindUnanimity(t *testing.T) {
	// Keep-2 + fold-4; every folded member is a whole-window sleeper → the fold
	// row keeps the unanimous typed state (the renderer's idle gate reads it).
	uniform := []TraceCausalProjectionNode{
		fRoundBGNode("K1", "keep-1", "s_sleep"),
		fRoundBGNode("K2", "keep-2", "s_sleep"),
		fRoundBGNode("F1", "bg-1", "s_sleep"),
		fRoundBGNode("F2", "bg-2", "s_sleep"),
		fRoundBGNode("F3", "bg-3", "s_sleep"),
		fRoundBGNode("F4", "bg-4", "s_sleep"),
	}
	out := traceCausalProjectionFoldUnknownBackground(uniform)
	if len(out) != 3 {
		t.Fatalf("expected keep-2 + one fold row: %+v", out)
	}
	fold := out[len(out)-1]
	if fold.MergedCount != 4 || fold.StateKind != "s_sleep" {
		t.Fatalf("uniform sleeper fold must keep the unanimous StateKind: %+v", fold)
	}
	// One running member breaks unanimity → the fold row is stateless (never
	// fabricate a single state for mixed members).
	mixed := []TraceCausalProjectionNode{
		fRoundBGNode("K1", "keep-1", "s_sleep"),
		fRoundBGNode("K2", "keep-2", "s_sleep"),
		fRoundBGNode("F1", "bg-1", "s_sleep"),
		fRoundBGNode("F2", "bg-2", "running"),
		fRoundBGNode("F3", "bg-3", "s_sleep"),
		fRoundBGNode("F4", "bg-4", "s_sleep"),
	}
	out = traceCausalProjectionFoldUnknownBackground(mixed)
	fold = out[len(out)-1]
	if fold.MergedCount != 4 || fold.StateKind != "" {
		t.Fatalf("mixed-state fold must stay stateless: %+v", fold)
	}
	// Stateless members stay stateless (empty ≠ a state; unanimity of "" keeps "").
	stateless := []TraceCausalProjectionNode{
		fRoundBGNode("K1", "keep-1", ""),
		fRoundBGNode("K2", "keep-2", ""),
		fRoundBGNode("F1", "bg-1", ""),
		fRoundBGNode("F2", "bg-2", ""),
		fRoundBGNode("F3", "bg-3", ""),
		fRoundBGNode("F4", "bg-4", ""),
	}
	out = traceCausalProjectionFoldUnknownBackground(stateless)
	fold = out[len(out)-1]
	if fold.StateKind != "" {
		t.Fatalf("stateless members must not fabricate a fold state: %+v", fold)
	}
}
