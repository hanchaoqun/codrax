package types

// trace_causal_projection_chainguard_test.go — CHAINGUARD-1 types pins
// (§29.204.1, 2026-07-22): the chain_credential_census strict parse (件2 wire
// → Node mirror) and the §29.208 P3 记档① ▒ 成对臂端点相接负 pin — two
// same-thread same-kind background accounts whose intervals merely TOUCH at
// an endpoint share no wall clock: the same-segment-mirror caliber must NOT
// engage (they are physically consecutive, not twin accounts of one state)
// and the ×N merge keeps the additive SUM.

import "testing"

// TestChainguardCensusNoteStrictParse — the single strict parser mirrors the
// census note into Node.ChainCredentialCensus; absence stays "".
func TestChainguardCensusNoteStrictParse(t *testing.T) {
	record := ObservationRecord{
		ID: "cg-parse-1", Subject: "worker-200", Predicate: "root_cause_primary",
		ClaimKey:  "root_cause_primary:worker-200",
		RichNotes: []string{"rank=1", "chain_relevance=on_chain", "chain_credential_census=member_inherited"},
	}
	node := traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, record)
	if node.ChainCredentialCensus != "member_inherited" {
		t.Fatalf("census note must strict-parse into the Node mirror, got %q", node.ChainCredentialCensus)
	}
	record.RichNotes = []string{"rank=1", "chain_relevance=on_chain"}
	node = traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, record)
	if node.ChainCredentialCensus != "" {
		t.Fatalf("absent census note must stay empty (渐进兼容), got %q", node.ChainCredentialCensus)
	}
}

// TestChainguardMirrorEndpointTouchingPairStaysSum — §29.208 P3 记档①: the
// ISPGAP F-B 同段镜像 caliber engages ONLY on genuine positive-length interval
// overlap. Two ▒ members whose windows touch at one endpoint ([1.0,1.5] and
// [1.5,2.0]) are consecutive physical accounts — no shared wall clock, no
// mirror, the SUM stays the honest additive value. The strictly-overlapping
// twin arm rides beside it as the positive contrast.
func TestChainguardMirrorEndpointTouchingPairStaysSum(t *testing.T) {
	touching := []TraceCausalProjectionNode{
		{Subject: "isplogd-1300", ImpactMS: 500, CumulativeImpactMS: 500, StartTs: 1.0, EndTs: 1.5},
		{Subject: "isplogd-1300", ImpactMS: 500, CumulativeImpactMS: 500, StartTs: 1.5, EndTs: 2.0},
	}
	if out := traceCausalProjectionSameSegmentMirrorValue(touching, []int{0, 1}); out.engaged {
		t.Fatalf("端点相接负臂: endpoint-touching members must NOT engage the mirror caliber: %+v", out)
	}
	if TraceCausalProjectionIntervalsOverlap(1.0, 1.5, 1.5, 2.0) {
		t.Fatal("端点相接负臂: the shared overlap predicate must stay strict (touching ≠ overlap)")
	}
	overlapping := []TraceCausalProjectionNode{
		{Subject: "isplogd-1300", ImpactMS: 500, CumulativeImpactMS: 500, StartTs: 1.0, EndTs: 1.5},
		{Subject: "isplogd-1300", ImpactMS: 400, CumulativeImpactMS: 400, StartTs: 1.1, EndTs: 1.5},
	}
	out := traceCausalProjectionSameSegmentMirrorValue(overlapping, []int{0, 1})
	if !out.engaged || !out.exact {
		t.Fatalf("正臂: genuinely overlapping twin accounts must engage the mirror deduction: %+v", out)
	}
	if out.valueMS > 500.0+1e-6 {
		t.Fatalf("正臂: the deduplicated value must never exceed the union (≤500ms), got %.3f", out.valueMS)
	}
}
