package authority

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBackfillEvidenceProjector_PinsBypassItems exercises the round-3
// fix: items written via AppendEvidence WITHOUT going through
// emit_evidence's hook (concrete_value extractor / mechanism_scan /
// bridge_literal merge) used to land with Authority="" and
// HighestAuthorityFor would treat them as factual-equivalent. The
// projector now backfills Authority/Origin per item.
func TestBackfillEvidenceProjector_PinsBypassItems(t *testing.T) {
	mut := types.NewMutableState("test")
	// Bypass-shape item: no Origin, no Authority — what mechanism_scan
	// or concrete_value extractor would emit.
	bypass := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "x.go",
		LineStart:       42,
		AnchorKind:      types.AnchorDefinition,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}
	proj := BackfillEvidenceProjector()
	out := proj([]types.EvidenceItem{bypass}, mut)
	if len(out) != 1 {
		t.Fatalf("got %d items; want 1", len(out))
	}
	if out[0].Origin != types.ClaimOriginCurrentRepo {
		t.Errorf("Origin = %q; want current_repo", out[0].Origin)
	}
	if out[0].Authority != types.AuthorityFactual {
		t.Errorf("Authority = %q; want factual (line-shaped current-repo grounded)",
			out[0].Authority)
	}
}

// TestBackfillEvidenceProjector_IsIdempotent: items that already went
// through ComputeForEvidence (non-zero Origin OR Authority) must NOT
// be reprojected. Idempotency guards the dual-write case where some
// emitter calls AppendEvidence repeatedly.
func TestBackfillEvidenceProjector_IsIdempotent(t *testing.T) {
	mut := types.NewMutableState("test")
	already := types.EvidenceItem{
		Source:          "x.go",
		LineStart:       42,
		GroundingStatus: types.GroundingGrounded,
		Origin:          types.ClaimOriginLog,
		Authority:       types.AuthorityHistorical,
		AuthorityReason: "test-set; do not overwrite",
	}
	proj := BackfillEvidenceProjector()
	out := proj([]types.EvidenceItem{already}, mut)
	if out[0].Authority != types.AuthorityHistorical {
		t.Errorf("Authority overwritten: %q; want historical", out[0].Authority)
	}
	if out[0].Origin != types.ClaimOriginLog {
		t.Errorf("Origin overwritten: %q; want log", out[0].Origin)
	}
	if out[0].AuthorityReason != "test-set; do not overwrite" {
		t.Errorf("Reason overwritten: %q", out[0].AuthorityReason)
	}
}

// TestHighestAuthorityFor_SkipsUnknownItems: round-3 fix — Unknown
// items used to win the "Highest" vote because rank(Unknown)=4 ==
// rank(Factual). A bypass item at the same anchor as a Conditional
// LLM-emit would falsely show factual. Now Unknown is skipped so the
// Conditional vote is what surfaces.
func TestHighestAuthorityFor_SkipsUnknownItems(t *testing.T) {
	items := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityUnknown},
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityConditional},
	}
	got := HighestAuthorityFor(items, "a.go", 10)
	if got != types.AuthorityConditional {
		t.Errorf("Unknown+Conditional → got %q; want conditional", got)
	}
}

// TestLowestAuthorityFor_SkipsUnknownItems: symmetric — Unknown
// shouldn't down-rank either. A factual item with an Unknown
// neighbor must stay factual.
func TestLowestAuthorityFor_SkipsUnknownItems(t *testing.T) {
	items := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityUnknown},
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityFactual},
	}
	got := LowestAuthorityFor(items, "a.go", 10)
	if got != types.AuthorityFactual {
		t.Errorf("Unknown+Factual → got %q; want factual", got)
	}
}

// TestHighestAuthorityFor_AllUnknownReturnsUnknown: when every
// matching item is Unknown (no information anywhere), Highest
// returns Unknown — the renderer treats this as factual-equivalent
// (the legacy default for un-projected items).
func TestHighestAuthorityFor_AllUnknownReturnsUnknown(t *testing.T) {
	items := []types.EvidenceItem{
		{Source: "a.go", LineStart: 10, Authority: types.AuthorityUnknown},
	}
	if got := HighestAuthorityFor(items, "a.go", 10); got != types.AuthorityUnknown {
		t.Errorf("all-Unknown: got %q; want Unknown", got)
	}
}
