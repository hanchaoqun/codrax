package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRunAuthorityOverreachCheck_NilMutableSafe defends nil-input
// safety on the producer.
func TestRunAuthorityOverreachCheck_NilMutableSafe(t *testing.T) {
	if got := runAuthorityOverreachCheck(nil, "anything"); got != nil {
		t.Errorf("nil mut: got %d violations; want nil", len(got))
	}
}

// TestRunAuthorityOverreachCheck_EmptyEvidenceNoEmit asserts the
// producer is silent when there is no evidence to ground a verdict.
func TestRunAuthorityOverreachCheck_EmptyEvidenceNoEmit(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	if got := runAuthorityOverreachCheck(mut, "anything"); got != nil {
		t.Errorf("empty evidence: got %d violations; want nil", len(got))
	}
}

// TestRunAuthorityOverreachCheck_FactualOnlyNoEmit: cited anchors
// all factual → producer silent.
func TestRunAuthorityOverreachCheck_FactualOnlyNoEmit(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityFactual},
	})
	if got := runAuthorityOverreachCheck(mut, "X is at x.go:10."); got != nil {
		t.Errorf("factual-only: got %d violations; want nil", len(got))
	}
}

// TestRunAuthorityOverreachCheck_ConditionalCitedButNoHedgeFires:
// the load-bearing case — a cited anchor's underlying evidence is
// conditional, but the rendered prose lacks [hedged]. Producer
// must surface a single ViolAuthorityOverreach.
func TestRunAuthorityOverreachCheck_ConditionalCitedButNoHedgeFires(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	})
	viols := runAuthorityOverreachCheck(mut, "X directly causes Y; see x.go:10.")
	if len(viols) != 1 {
		t.Fatalf("got %d violations; want 1", len(viols))
	}
	if viols[0].Kind != types.ViolAuthorityOverreach {
		t.Errorf("Kind = %q; want authority_overreach", viols[0].Kind)
	}
	if !strings.Contains(viols[0].Detail, "drift-bounded evidence") {
		t.Errorf("Detail missing canonical 'drift-bounded evidence' wording: %q",
			viols[0].Detail)
	}
}

// TestRunAuthorityOverreachCheck_DocCaveatIsRequiredAndSufficient:
// the gate is anchored to the doc-level Authority caveat (the
// system's primary user-visible hedging signal). When the caveat
// is present (recognised by the system-private tag, round-4),
// the check passes regardless of per-shape sentinels.
func TestRunAuthorityOverreachCheck_DocCaveatIsRequiredAndSufficient(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityConditional},
	})
	// Prose without per-shape sentinel but WITH a system-tagged
	// caveat — system signaled drift via the canonical channel.
	text := "Current code at x.go:10 reads X.\n\n" +
		render.AuthorityCaveatPrefix + render.AuthorityCaveatTag() +
		"1 evidence item from drifted log."
	if got := runAuthorityOverreachCheck(mut, text); got != nil {
		t.Errorf("doc-caveat present: got %d violations; want nil", len(got))
	}
}

// TestRunAuthorityOverreachCheck_DocLevelCaveatIsHedgeProof: when
// the rendered prose contains the doc-level "Authority: ..." caveat
// (rendered from doc.Caveats[] when ApplyAuthorityHedging detected
// drifted evidence), the gate must NOT fire even if per-shape
// sentinels are absent. The caveat is itself the hedging signal —
// downstream consumers and the user already see it. Without this
// escape, shapes whose body fields don't get per-sentinel hedges
// would force unsatisfiable retries.
func TestRunAuthorityOverreachCheck_DocLevelCaveatIsHedgeProof(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityHistorical},
	})
	// Prose lacks per-shape sentinels but DOES carry the system-
	// tagged caveat (round-4: tag is the canonical hedge signal,
	// not the public prefix).
	text := "X happened in legacy code; see x.go:10.\n\n" +
		render.AuthorityCaveatPrefix + render.AuthorityCaveatTag() +
		"1 historical observation; strong claims hedged."
	if got := runAuthorityOverreachCheck(mut, text); got != nil {
		t.Errorf("doc-level caveat should be hedge proof; got %d violations", len(got))
	}
}

// TestRunAuthorityOverreachCheck_FiresWhenCaveatStripped: the
// failure mode the gate exists to catch — drift-bounded evidence
// AND no doc-level caveat in rendered text (renderer was bypassed
// or some downstream transformation stripped the caveat).
func TestRunAuthorityOverreachCheck_FiresWhenCaveatStripped(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Shape:     types.ShapeExplanation,
		Citations: []types.Citation{{File: "x.go", Line: 10}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{Source: "x.go", LineStart: 10, Authority: types.AuthorityHistorical},
	})
	// Per-shape sentinel present but doc-level caveat ABSENT —
	// renderer was bypassed (raw-prose fallback) or caveat stripped.
	text := render.HedgeMarkerConditional + " X happened in legacy code."
	viols := runAuthorityOverreachCheck(mut, text)
	if len(viols) != 1 {
		t.Fatalf("got %d violations; want 1", len(viols))
	}
	if !strings.Contains(viols[0].Detail, "Authority disclosure is missing") {
		t.Errorf("Detail should describe the missing disclosure in user-facing terms: %q",
			viols[0].Detail)
	}
}
