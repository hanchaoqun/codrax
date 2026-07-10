package types

// Marker-stripping class root fix pins (audit 2026-07-10, MARKER batch).
//
// AnswerBlock.SystemGeneratedKind is json:"-" — the model must never
// author authority — so every system-side JSON snapshot of a persisted
// document (FRCAP draft-ledger DocJSON, RetryState.PrevEmitJSON) loses
// the marker on the round trip. CaptureSystemGeneratedBlockKinds /
// ReauthenticateSystemSnapshotBlockKinds are the single shared mechanism
// that preserves the authority out-of-band and restores it on decode.
//
// The pins below hold BOTH directions:
//   - restore: exactly the captured authority comes back after a
//     marshal/unmarshal round trip;
//   - forgery: a block that was NOT marked at capture time can never
//     gain authority from the helper — not by reserved-ID spelling, not
//     by riding the same snapshot, not by a crafted map with empty kind.

import (
	"encoding/json"
	"testing"
)

func reauthDocFixture() *AnswerDocumentV2 {
	return &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{
		{ID: "summary", Kind: BlockSummary, SurfaceRole: SurfacePrincipal,
			Text: "聚合影响达 46.821ms。"},
		{ID: "runtime_trace_metric_snapshot", Kind: BlockSection,
			Title: "关键指标核对", Text: "聚合影响 46.821ms",
			SystemGeneratedKind: AnswerSystemGeneratedRuntimeTrace},
		// Model-authored lookalike that (hypothetically) bypassed the
		// persist choke: exact reserved spelling, NO live marker.
		{ID: "runtime_trace_facts", Kind: BlockSection,
			Title: "模型仿冒块", Text: "伪造 99.999ms"},
	}}
}

// TestCaptureSystemGeneratedBlockKinds — the capture face records ONLY
// authority that is live in memory: marked blocks by exact id; unmarked
// blocks (even with reserved-ID spellings) are absent; empty docs and
// unmarked docs return nil so the common non-trace path pays nothing.
func TestCaptureSystemGeneratedBlockKinds(t *testing.T) {
	got := CaptureSystemGeneratedBlockKinds(reauthDocFixture())
	if len(got) != 1 {
		t.Fatalf("capture must record exactly the one marked block, got %v", got)
	}
	if got["runtime_trace_metric_snapshot"] != AnswerSystemGeneratedRuntimeTrace {
		t.Fatalf("captured kind mismatch: %v", got)
	}
	if _, ok := got["runtime_trace_facts"]; ok {
		t.Fatalf("an UNMARKED reserved-ID block must never be captured (forgery face): %v", got)
	}
	if CaptureSystemGeneratedBlockKinds(nil) != nil {
		t.Fatalf("nil doc must capture nil")
	}
	if CaptureSystemGeneratedBlockKinds(&AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "summary"}}}) != nil {
		t.Fatalf("unmarked doc must capture nil")
	}
}

// TestReauthenticateSystemSnapshotBlockKinds_RoundTrip — the defect this
// mechanism roots out: json.Marshal strips the json:"-" marker (baseline
// asserted), and the re-stamp restores EXACTLY the captured authority —
// the genuine system block regains its kind, while the unmarked
// reserved-ID lookalike in the same snapshot stays model grade.
func TestReauthenticateSystemSnapshotBlockKinds_RoundTrip(t *testing.T) {
	src := reauthDocFixture()
	kinds := CaptureSystemGeneratedBlockKinds(src)

	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded AnswerDocumentV2
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Baseline: the round trip really strips the marker (why the helper
	// exists at all). If this ever turns green without the helper, the
	// field became JSON-visible — a forgery-lane regression to audit.
	if decoded.Blocks[1].SystemGeneratedKind != AnswerSystemGeneratedBlockUnknown {
		t.Fatalf("json round trip no longer strips SystemGeneratedKind — json:\"-\" contract broken")
	}

	if restamped := ReauthenticateSystemSnapshotBlockKinds(&decoded, kinds); restamped != 1 {
		t.Fatalf("restamped = %d, want exactly 1", restamped)
	}
	if !decoded.Blocks[1].SystemGeneratedKind.IsRuntimeTraceSupplement() {
		t.Fatalf("genuine system block must regain its authority marker")
	}
	if decoded.Blocks[0].SystemGeneratedKind != AnswerSystemGeneratedBlockUnknown {
		t.Fatalf("plain model block must stay unmarked")
	}
	if decoded.Blocks[2].SystemGeneratedKind != AnswerSystemGeneratedBlockUnknown {
		t.Fatalf("FORGERY: unmarked reserved-ID lookalike gained authority from the snapshot re-stamp")
	}

	// Idempotence: a second application changes nothing.
	if restamped := ReauthenticateSystemSnapshotBlockKinds(&decoded, kinds); restamped != 0 {
		t.Fatalf("second re-stamp must be a no-op, restamped %d", restamped)
	}
}

// TestReauthenticateSystemSnapshotBlockKinds_NeverInvents — hostile-map
// faces: an empty/nil map is a no-op; an id absent from the doc stamps
// nothing; an empty kind in the map stamps nothing; a non-empty kind
// already on the block is never overwritten.
func TestReauthenticateSystemSnapshotBlockKinds_NeverInvents(t *testing.T) {
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{
		{ID: "a", Kind: BlockSummary},
		{ID: "b", Kind: BlockSection, SystemGeneratedKind: AnswerSystemGeneratedPrincipalEnumerationRows},
	}}
	if got := ReauthenticateSystemSnapshotBlockKinds(doc, nil); got != 0 {
		t.Fatalf("nil map must be a no-op, got %d", got)
	}
	if got := ReauthenticateSystemSnapshotBlockKinds(nil, map[string]AnswerSystemGeneratedBlockKind{"a": AnswerSystemGeneratedRuntimeTrace}); got != 0 {
		t.Fatalf("nil doc must be a no-op, got %d", got)
	}
	if got := ReauthenticateSystemSnapshotBlockKinds(doc, map[string]AnswerSystemGeneratedBlockKind{
		"missing": AnswerSystemGeneratedRuntimeTrace,
		"a":       AnswerSystemGeneratedBlockUnknown, // empty kind never stamps
	}); got != 0 {
		t.Fatalf("absent ids / empty kinds must stamp nothing, got %d", got)
	}
	if doc.Blocks[0].SystemGeneratedKind != AnswerSystemGeneratedBlockUnknown {
		t.Fatalf("block a must stay unmarked")
	}
	if got := ReauthenticateSystemSnapshotBlockKinds(doc, map[string]AnswerSystemGeneratedBlockKind{
		"b": AnswerSystemGeneratedRuntimeTrace,
	}); got != 0 || doc.Blocks[1].SystemGeneratedKind != AnswerSystemGeneratedPrincipalEnumerationRows {
		t.Fatalf("existing non-empty kind must never be overwritten: got=%d kind=%q", got, doc.Blocks[1].SystemGeneratedKind)
	}
}
