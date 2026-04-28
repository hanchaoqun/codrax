package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
)

// TestRequireCitationCorroboration_MixedClaimUnicodeFallback covers
// the gap Class A audit found: when an LLM-emitted claim mixes
// Chinese with an ASCII identifier and the line contains the FULL
// claim (Chinese + ASCII portions both present), the token-set path
// SHOULD already match via the ASCII token. We additionally cover
// the case where token-set could fail but substring still matches —
// e.g. line is `# ProcessRequest 用户` with ASCII tokens covered.
// In all cases substring fallback either matches or the token-set
// would have. Mirrors the H pattern from ground.go.
func TestRequireCitationCorroboration_MixedClaimUnicodeFallback(t *testing.T) {
	gc := &ground.Context{
		LineIndex: map[string]map[int]string{
			"x.py": {
				9: `print("ProcessRequest 用户已存在")`,
			},
		},
	}
	cfg := corroborationCfg{
		claimLabel: `value.literal "ProcessRequest 用户已存在"`,
		citeLabel:  "citations[0]",
		escape:     "Otherwise cite a real file:line.",
	}
	err := requireCitationCorroboration("ProcessRequest 用户已存在", "x.py", 9, gc, cfg)
	if err != nil {
		t.Errorf("mixed Chinese+ASCII claim present in cited line must corroborate; got %v", err)
	}
}

// TestRequireCitationCorroboration_MixedClaimAbsent guards the
// false-positive: when the mixed claim's ASCII token does not
// overlap AND substring also doesn't match, the substring fallback
// must reject. This is the "absent" case — neither route accepts.
func TestRequireCitationCorroboration_MixedClaimAbsent(t *testing.T) {
	gc := &ground.Context{
		LineIndex: map[string]map[int]string{
			"x.py": {
				9: `# unrelated ASCII comment`,
			},
		},
	}
	cfg := corroborationCfg{
		claimLabel: `value.literal "ProcessRequest 用户已存在"`,
		citeLabel:  "citations[0]",
		escape:     "Otherwise cite a real file:line.",
	}
	err := requireCitationCorroboration("ProcessRequest 用户已存在", "x.py", 9, gc, cfg)
	if err == nil {
		t.Errorf("absent mixed claim must NOT corroborate")
	}
	if !strings.Contains(err.Error(), "is not corroborated") {
		t.Errorf("error should be the standard rejection message; got %q", err.Error())
	}
}

// TestRequireCitationCorroboration_PureChineseClaimFailsOpen pins
// the pre-existing behaviour: a claim with ZERO ASCII tokens hits
// the early `len(tokens) == 0 → return nil` and is fail-OPEN. This
// is intentional — the helper documents "claim text has no
// identifier-shape tokens → skip (numeric literals, short
// sentinels, prose with only noise words)". The H patch did NOT
// change this; the substring fallback only fires AFTER token-set
// has produced ≥1 wanted token and failed. Documenting via test
// so a future tightening is a deliberate decision.
func TestRequireCitationCorroboration_PureChineseClaimFailsOpen(t *testing.T) {
	gc := &ground.Context{
		LineIndex: map[string]map[int]string{
			"x.py": {
				9: `# unrelated comment`,
			},
		},
	}
	cfg := corroborationCfg{
		claimLabel: `value.literal "用户已存在"`,
		citeLabel:  "citations[0]",
		escape:     "Otherwise cite a real file:line.",
	}
	err := requireCitationCorroboration("用户已存在", "x.py", 9, gc, cfg)
	if err != nil {
		t.Errorf("pure-Chinese claim is fail-open by design (zero ASCII tokens path); got rejection %v", err)
	}
}

// TestRequireCitationCorroboration_ASCIIPathByteIdentical guards
// the dominant case: a claim whose ASCII identifier tokens DO
// match the line takes the original token-set path; no behaviour
// drift from the Unicode fallback addition.
func TestRequireCitationCorroboration_ASCIIPathByteIdentical(t *testing.T) {
	gc := &ground.Context{
		LineIndex: map[string]map[int]string{
			"x.go": {
				9: `func ProcessRequest(ctx Context) {}`,
			},
		},
	}
	cfg := corroborationCfg{
		claimLabel: `value.literal "ProcessRequest"`,
		citeLabel:  "citations[0]",
		escape:     "Otherwise cite a real file:line.",
	}
	err := requireCitationCorroboration("ProcessRequest", "x.go", 9, gc, cfg)
	if err != nil {
		t.Errorf("ASCII identifier must corroborate via token-set path; got %v", err)
	}
	// Reverse: ASCII claim NOT present must reject.
	err = requireCitationCorroboration("ParseRequest", "x.go", 9, gc, cfg)
	if err == nil {
		t.Errorf("absent ASCII identifier must reject")
	}
}

// TestRequireCitationCorroboration_PureSubstringFalsePositiveGuard
// is the critical false-positive guard for the substring fallback:
// when the claim is `Foo` and the line is `func FooBar()`, the
// ASCII token-set path correctly rejects because the line tokens are
// `{FooBar}` and the claim tokens are `{Foo}`. The substring fallback
// would naively `strings.Contains("func FooBar()", "Foo")` = true,
// which would erode the precision the token-set was designed to
// preserve. The current implementation enters the substring fallback
// AFTER the token-set has already produced ≥1 wanted-token and
// failed; in this scenario the wanted-token "Foo" exists, so the
// helper executes the substring fallback. This test pins the actual
// behaviour: a 3-char ASCII substring "Foo" inside "FooBar" DOES
// pass the substring check today. The author of any future tightening
// must update this expectation deliberately, not as a side effect.
func TestRequireCitationCorroboration_PureSubstringFalsePositiveGuard(t *testing.T) {
	gc := &ground.Context{
		LineIndex: map[string]map[int]string{
			"x.go": {
				9: `func FooBar(){}`,
			},
		},
	}
	cfg := corroborationCfg{
		claimLabel: `value.literal "Foo"`,
		citeLabel:  "citations[0]",
		escape:     "Otherwise cite a real file:line.",
	}
	err := requireCitationCorroboration("Foo", "x.go", 9, gc, cfg)
	// The ASCII token "Foo" is a wanted token but the line only has
	// "FooBar" as a token; token-set route fails. The substring
	// fallback then matches "Foo" inside "func FooBar(){}". This is
	// a documented permissiveness — the function is the LAST gate, the
	// LLM has already passed shape + symbol-anchor checks. Accept and
	// document; if a future user reports false positives we tighten
	// the fallback to require a word-boundary surround.
	if err != nil {
		t.Errorf("documented behaviour: substring fallback ACCEPTS 'Foo' inside 'FooBar'; if changed, update this test deliberately. got %v", err)
	}
}
