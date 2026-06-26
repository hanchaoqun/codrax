package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func scalarDoc(text string, form types.ClaimForm, citationRef int) *types.AnswerDocumentV2 {
	blk := types.AnswerBlock{
		ID:          "s1",
		Kind:        types.BlockScalar,
		SurfaceRole: types.SurfacePrincipal,
		Text:        text,
	}
	if form != "" {
		blk.ClaimUses = []types.RenderedClaimUse{{ClaimForm: form}}
	}
	if citationRef >= 0 {
		blk.Items = []types.AnswerBlockItem{{ID: "x", CitationRef: citationRef}}
	}
	return &types.AnswerDocumentV2{
		Blocks:    []types.AnswerBlock{blk},
		Citations: []types.Citation{{File: "internal/a.go", Line: 12}},
	}
}

func scalarMut(snippet string) *types.MutableState {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID: "ev-1", Subject: "MaxRetries", Source: "internal/a.go",
		LineStart: 10, LineEnd: 14, Snippet: snippet,
		GroundingStatus: types.GroundingGrounded,
	}})
	return mut
}

// A principal scalar literal contradicting its own cited evidence
// fires; the same literal present in the snippet passes.
func TestScalarValueGroundingFiresOnUnsupportedLiteral(t *testing.T) {
	mut := scalarMut("const MaxRetries = 5")
	v := validateScalarValueGrounding(scalarDoc("7", types.ClaimAssignmentFact, 0), mut)
	if len(v) != 1 || v[0].Kind != types.ViolScalarValueUngrounded {
		t.Fatalf("unsupported literal must fire, got %v", v)
	}
	if !strings.Contains(v[0].Detail, `"7"`) {
		t.Fatalf("detail must carry the literal: %q", v[0].Detail)
	}

	if v := validateScalarValueGrounding(scalarDoc("5", types.ClaimAssignmentFact, 0), mut); v != nil {
		t.Fatalf("snippet-supported literal must pass, got %v", v)
	}
}

// Aggregate facts are a legitimate support surface.
func TestScalarValueGroundingAggregateSupport(t *testing.T) {
	mut := scalarMut("unrelated snippet")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{Kind: types.AnswerAggregateTotalCount, Label: "matches", Value: "7"},
	})
	mut.SetInvestigationComplete("done")
	mut.RetainInvestigationAggregateFacts()
	if v := validateScalarValueGrounding(scalarDoc("7", types.ClaimAssignmentFact, 0), mut); v != nil {
		t.Fatalf("aggregate-backed literal must pass, got %v", v)
	}
}

func TestScalarValueGroundingDoesNotUseSummaryUnlessLoadBearing(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "ev-summary-only",
		Subject:         "MaxRetries",
		Object:          "configured retry cap",
		Source:          "internal/a.go",
		LineStart:       10,
		LineEnd:         14,
		Snippet:         "const MaxRetries = 5",
		Summary:         "operator-facing note mentions 7",
		GroundingStatus: types.GroundingGrounded,
	}})

	v := validateScalarValueGrounding(scalarDoc("7", types.ClaimAssignmentFact, 0), mut)
	if len(v) != 1 || v[0].Kind != types.ViolScalarValueUngrounded {
		t.Fatalf("summary-only scalar support must not suppress grounding violation, got %v", v)
	}

	mut = types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:                 "ev-load-bearing-summary",
		Subject:            "MaxRetries",
		Object:             "configured retry cap",
		Source:             "internal/a.go",
		LineStart:          10,
		LineEnd:            14,
		Snippet:            "const MaxRetries = fallbackValue",
		Summary:            "7",
		LoadBearingSummary: true,
		GroundingStatus:    types.GroundingGrounded,
	}})
	if v := validateScalarValueGrounding(scalarDoc("7", types.ClaimAssignmentFact, 0), mut); v != nil {
		t.Fatalf("explicit load-bearing summary scalar must remain usable, got %v", v)
	}
}

// The typed escapes: undeclared claim form never gates;
// external_observation routes to the artifact lane (skip); empty
// support pool is set-side noise; multi-token prose is skipped.
func TestScalarValueGroundingEscapes(t *testing.T) {
	mut := scalarMut("const MaxRetries = 5")
	if v := validateScalarValueGrounding(scalarDoc("7", "", 0), mut); v != nil {
		t.Fatalf("undeclared form must not gate, got %v", v)
	}
	if v := validateScalarValueGrounding(scalarDoc("7", types.ClaimExternalObservation, 0), mut); v != nil {
		t.Fatalf("external_observation must skip the repo pool, got %v", v)
	}
	if v := validateScalarValueGrounding(scalarDoc("7", types.ClaimAssignmentFact, -1), types.NewMutableState("q")); v != nil {
		t.Fatalf("empty support pool must no-op, got %v", v)
	}
	if v := validateScalarValueGrounding(scalarDoc("the cap is 7 retries", types.ClaimAssignmentFact, 0), mut); v != nil {
		t.Fatalf("multi-token prose must be conservatively skipped, got %v", v)
	}
}

// Numeric literals demand digit boundaries: "7" is never vouched by
// "17" or "v7.1".
func TestScalarValueGroundingNumericBoundary(t *testing.T) {
	mut := scalarMut("const MaxRetries = 17 // see v7.1 notes")
	v := validateScalarValueGrounding(scalarDoc("7", types.ClaimAssignmentFact, 0), mut)
	if len(v) != 1 {
		t.Fatalf("embedded digits must not vouch for the literal, got %v", v)
	}
}

// Identifier-shaped literals compare through the shared canonicalizer.
func TestScalarValueGroundingIdentifierForm(t *testing.T) {
	mut := scalarMut("policy := ExponentialBackoff{}")
	if v := validateScalarValueGrounding(scalarDoc("`exponential_backoff`", types.ClaimDefinitionFact, 0), mut); v != nil {
		t.Fatalf("NormalizeCodeKey-equal identifier must pass, got %v", v)
	}
}

// Registry contract mirrors the s11b shape.
func TestScalarValueUngroundedRegistrySpec(t *testing.T) {
	spec, ok := types.ViolKindSpecFor(types.ViolScalarValueUngrounded)
	if !ok {
		t.Fatalf("kind not registered")
	}
	if !spec.SoftByDefault || !spec.Promotable || spec.FallbackLocus != types.LocusFinalizer {
		t.Fatalf("spec drifted: %+v", spec)
	}
}
