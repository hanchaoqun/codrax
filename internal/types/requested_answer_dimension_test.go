package types

import "testing"

func TestNormalizeRequestedAnswerDimensionProfile_PreservesAnchoredDimensions(t *testing.T) {
	raw := "请说明每次提交的 diff 线索、当前关键代码、作用和影响，不要只给 commit id。"
	profile, warnings := NormalizeRequestedAnswerDimensionProfile(raw, &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Confidence:          0.8,
		Dimensions: []RequestedAnswerDimension{
			{Label: "当前关键代码", Role: RequestedAnswerDimensionCurrentKeyCode, Required: true, Index: 2},
			{Label: "diff 线索", Role: RequestedAnswerDimensionDiffClue, Required: true, Index: 1},
			{Label: "diff 线索", Role: RequestedAnswerDimensionDiffClue, Required: true, Index: 9},
			{Label: "invented axis", Role: RequestedAnswerDimensionImpact, Required: true, Index: 3},
			{Label: "影响", Role: RequestedAnswerDimensionImpact, Required: true, Index: 4},
		},
	})
	if profile == nil || !profile.Active() {
		t.Fatalf("profile should survive: profile=%+v warnings=%v", profile, warnings)
	}
	if len(profile.Dimensions) != 3 {
		t.Fatalf("dimensions=%d want 3: %+v warnings=%v", len(profile.Dimensions), profile.Dimensions, warnings)
	}
	if profile.Dimensions[0].Label != "diff 线索" || profile.Dimensions[1].Label != "当前关键代码" || profile.Dimensions[2].Label != "影响" {
		t.Fatalf("order/labels not preserved: %+v", profile.Dimensions)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning for unanchored dimension")
	}
}

func TestNormalizeRequestedAnswerDimensionProfile_PreservesExactMixedScriptQuoteAcrossWhitespace(t *testing.T) {
	raw := "它运行的 CPU 频率有没有受到限制，证据是什么？"
	profile, warnings := NormalizeRequestedAnswerDimensionProfile(raw, &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Confidence:          0.9,
		Dimensions: []RequestedAnswerDimension{{
			Label:       "CPU频率限制判定",
			Role:        RequestedAnswerDimensionCausalAttribution,
			SourceQuote: "它运行的CPU频率有没有受到限制，证据是什么",
			Required:    true,
			Index:       1,
		}},
	})
	if profile == nil || !profile.Active() || len(profile.Dimensions) != 1 {
		t.Fatalf("whitespace-only surface variance must preserve the dimension: profile=%+v warnings=%v", profile, warnings)
	}
	if profile.Dimensions[0].Role != RequestedAnswerDimensionCausalAttribution {
		t.Fatalf("typed role changed: %+v", profile.Dimensions[0])
	}
	if len(warnings) != 0 {
		t.Fatalf("whitespace-only surface variance emitted warning: %v", warnings)
	}
}

func TestNormalizeRequestedAnswerDimensionProfile_DoesNotAnchorParaphraseOrPunctuationChange(t *testing.T) {
	raw := "它运行的 CPU 频率有没有受到限制，证据是什么？"
	for _, quote := range []string{
		"CPU 是否被节流",
		"证据是什么，它运行的 CPU 频率有没有受到限制",
		"它运行的 CPU 频率有没有受到限制、证据是什么",
	} {
		profile, _ := NormalizeRequestedAnswerDimensionProfile(raw, &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Confidence:          0.9,
			Dimensions: []RequestedAnswerDimension{{
				Label:       "invented dimension",
				Role:        RequestedAnswerDimensionCausalAttribution,
				SourceQuote: quote,
				Required:    true,
				Index:       1,
			}},
		})
		if profile != nil {
			t.Fatalf("non-verbatim quote %q must not gain typed dimension authority: %+v", quote, profile)
		}
	}
}

func TestCurrentSourceObligationSignalsFromRequestedDimensions_RecordsDroppedSourceRoles(t *testing.T) {
	// §29.166 OBLSWEEP-1: the dropped obligation dims must carry a precise
	// current-source anchor (path suffix / file:line) to keep minting; the
	// preserved lane below pins that real anchored obligations dropped by
	// presentation normalization are not lost.
	raw := []RequestedAnswerDimension{
		{Label: "parse rules", SourceQuote: "internal/parse/rules.go:42", Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true, Index: 1},
		{Label: "current threshold code", SourceQuote: "internal/threshold/limits.go", Role: RequestedAnswerDimensionCurrentKeyCode, Required: true, Index: 2},
		{Label: "evidence boundary", Role: RequestedAnswerDimensionBoundary, Required: true, Index: 3},
	}
	normalized := &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []RequestedAnswerDimension{{
			Label:    "evidence boundary",
			Role:     RequestedAnswerDimensionBoundary,
			Required: true,
			Index:    3,
		}},
		Confidence: 0.9,
	}
	signals := CurrentSourceObligationSignalsFromRequestedDimensions(raw, normalized)
	if len(signals) != 2 {
		t.Fatalf("signals=%d want 2: %+v", len(signals), signals)
	}
	if signals[0].Kind != CurrentSourceObligationSignalDroppedRequestedDimension ||
		signals[0].Role != RequestedAnswerDimensionFunctionOrPurpose ||
		signals[1].Role != RequestedAnswerDimensionCurrentKeyCode {
		t.Fatalf("unexpected signals: %+v", signals)
	}
}

// §29.166 OBLSWEEP-1 mint-site pin: a dropped Required∧Role dimension whose
// quote/label is prose-only (no code/config path suffix, no file:line surface)
// mints NO obligation signal. Pre-fix, this word-face — which had already
// FAILED current-request provenance anchoring — minted a carrier that armed
// four hard faces (verification anchor, tier-1 floor, completion landing,
// source-audit debt) with no precise-anchor check, i.e. strictly weaker gating
// than the survived lane demoted by §29.146 件3 / §29.151 件3+件4.
func TestCurrentSourceObligationSignalsFromRequestedDimensions_ProseOnlyDroppedDimensionMintsNothing(t *testing.T) {
	raw := []RequestedAnswerDimension{
		// The §29.146 witness family shape: an analyzer-authored prose bullet
		// mislabeled current_key_code that is not verbatim in the request.
		{Label: "链上主要原因", SourceQuote: "主要原因", Role: RequestedAnswerDimensionCurrentKeyCode, Required: true, Index: 1},
		{Label: "mechanism narrative", Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true, Index: 2},
		// Degenerate zero-face: empty label and quote.
		{Label: "", Role: RequestedAnswerDimensionCurrentKeyCode, Required: true, Index: 3},
	}
	// Whole profile failed provenance validation (normalized == nil): the
	// pre-fix worst case where every required obligation-role dim minted.
	if signals := CurrentSourceObligationSignalsFromRequestedDimensions(raw, nil); len(signals) != 0 {
		t.Fatalf("prose-only dropped dimensions must not mint obligation signals: %+v", signals)
	}
	// Same with a survivor profile present for an unrelated dimension.
	normalized := &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []RequestedAnswerDimension{{
			Label:    "evidence boundary",
			Role:     RequestedAnswerDimensionBoundary,
			Required: true,
			Index:    4,
		}},
		Confidence: 0.9,
	}
	if signals := CurrentSourceObligationSignalsFromRequestedDimensions(raw, normalized); len(signals) != 0 {
		t.Fatalf("prose-only dropped dimensions must not mint obligation signals: %+v", signals)
	}
	// Precise-anchor arm on the identical shape keeps minting.
	raw[0].SourceQuote = "internal/render/frame.go:128"
	signals := CurrentSourceObligationSignalsFromRequestedDimensions(raw, normalized)
	if len(signals) != 1 || signals[0].Role != RequestedAnswerDimensionCurrentKeyCode {
		t.Fatalf("file:line-anchored dropped dimension must keep minting exactly its signal: %+v", signals)
	}
}

func TestCurrentSourceObligationSignalsFromRequestedDimensions_SkipsSurvivedSourceRoles(t *testing.T) {
	raw := []RequestedAnswerDimension{{
		Label:    "current key code",
		Role:     RequestedAnswerDimensionCurrentKeyCode,
		Required: true,
		Index:    2,
	}}
	normalized := &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []RequestedAnswerDimension{{
			Label:    "current key code",
			Role:     RequestedAnswerDimensionCurrentKeyCode,
			Required: true,
			Index:    2,
		}},
		Confidence: 0.9,
	}
	if signals := CurrentSourceObligationSignalsFromRequestedDimensions(raw, normalized); len(signals) != 0 {
		t.Fatalf("survived dimension should not produce signal: %+v", signals)
	}
}

func TestCompileAnswerPresentationContract_CarriesRequestedDimensions(t *testing.T) {
	profile := &RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []RequestedAnswerDimension{
			{Label: "diff 线索", Role: RequestedAnswerDimensionDiffClue, Index: 1},
			{Label: "当前关键代码", Role: RequestedAnswerDimensionCurrentKeyCode, Index: 2},
		},
	}
	ir := &AnalysisIR{RequestModel: RequestModel{RequestedAnswerDimensions: profile}}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view is nil")
	}
	if len(view.Presentation.RequestedDimensions) != 2 {
		t.Fatalf("presentation dimensions=%d want 2: %+v", len(view.Presentation.RequestedDimensions), view.Presentation.RequestedDimensions)
	}
}
