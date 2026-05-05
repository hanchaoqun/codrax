package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// user_panel_redline_test.go — W1.5 (2026-05-05).
//
// Pins the red-line invariant: the user-visible answer surface
// (out.FinalAnswer text) MUST NOT contain orchestration internals.
// Specifically, AppendUserCaveatsToAnswer is the ONLY allowed
// production path for "the system needs to tell the user the
// answer has unresolved gaps" — and it MUST produce natural-
// language output free of internal jargon.
//
// If a future commit reintroduces a header-prepend pattern that
// surfaces ViolKind names / IR field names / confidence numbers
// / yield-kill counts to the user, these tests will fail loud at
// the red-line boundary.

// forbiddenUserPanelTokens lists strings that MUST NEVER appear in
// user-facing answer text. Mix of known-leakage substrings the
// pre-W1 prependFailLoudWarning produced + structural tokens that
// only have meaning to operators.
var forbiddenUserPanelTokens = []string{
	"Pipeline terminated with unresolved violations",
	"yield kill",
	"top suspected IR field",
	"conf=0.",
	"event(s)",
	"Classification may be incorrect",
	// ViolKind name fragments — operator-only registry tokens.
	"block_items_label",
	"diagram_edges",
	"facet_uncovered",
	"answer_authority",
	"answer_richness_facet_coverage",
	// Internal validation header that pre-W1 also fronted the
	// answer (digest line stays for now via applyContractViolations
	// but should remain operator-only at user surface — see
	// settings.ViolationBudget.UserVisibleViolationCaveat default
	// false post-B4).
	"answer-contract validation exhausted:",
}

// TestRedline_AppendCaveats_StripsAllForbiddenTokens — the public
// entrypoint AppendUserCaveatsToAnswer must never produce a string
// containing any forbidden token, regardless of which violations
// fire. Sweeps every registered ViolKind that has a CaveatFamilyID
// to make sure NO template gets accidentally edited to leak
// internals.
func TestRedline_AppendCaveats_StripsAllForbiddenTokens(t *testing.T) {
	specs := types.AllViolKindSpecs()
	violations := make([]types.Violation, 0, len(specs))
	for _, s := range specs {
		violations = append(violations, types.Violation{Kind: s.Kind})
	}
	for _, lang := range []string{"zh", "en"} {
		out := AppendUserCaveatsToAnswer("Original answer body.\n", violations, lang)
		for _, token := range forbiddenUserPanelTokens {
			if strings.Contains(out, token) {
				t.Errorf("[lang=%s] AppendUserCaveatsToAnswer leaked forbidden token %q\nOutput:\n%s",
					lang, token, out)
			}
		}
		// Sanity check: original body must survive (caveats append, not replace).
		if !strings.Contains(out, "Original answer body.") {
			t.Errorf("[lang=%s] original answer body lost: %q", lang, out)
		}
	}
}

// TestRedline_StructurallyEmptyAnswer_NoLeakage — the
// structurallyEmptyAnswerForUser fallback for the degenerate
// "explore returned nothing" path must also be free of internal
// tokens. This path takes a different code branch from the
// caveat materializer and was the most-likely place to silently
// regress.
func TestRedline_StructurallyEmptyAnswer_NoLeakage(t *testing.T) {
	for _, lang := range []string{"zh", "en", ""} {
		out := structurallyEmptyAnswerForUser(lang)
		for _, token := range forbiddenUserPanelTokens {
			if strings.Contains(out, token) {
				t.Errorf("[lang=%q] structurallyEmptyAnswerForUser leaked %q in: %s",
					lang, token, out)
			}
		}
		// Also the "investigation structurally empty" operator
		// phrase must not leak — it belongs on out.Error only.
		if strings.Contains(out, "investigation structurally empty") {
			t.Errorf("[lang=%q] user-facing prose contains operator phrase 'investigation structurally empty': %s",
				lang, out)
		}
	}
}

// TestRedline_NoPrependFailLoudWarning_References — guards
// against a future revert that re-introduces the pre-W1
// "prepend orchestration header to FinalAnswer" pattern. We can't
// grep the repo from inside a test, but we CAN assert the function
// is no longer in the orchestrator package's exported / internal
// surface by attempting a compilation-style reflective probe:
// since the function was unexported, the simplest assertion is
// that no test in this package references it. The Go compiler
// would have failed the build above if it still existed AND was
// referenced. So this test exists as documentation: re-introducing
// prependFailLoudWarning should fail this package's full test
// build by way of one of the materializer/redline assertions
// firing instead.
func TestRedline_DocumentsPrependFailLoudWarningRetirement(t *testing.T) {
	// Intentional no-op test body — its purpose is documentary.
	// The protection is structural: the materializer tests +
	// AppendUserCaveatsToAnswer-redline test above will fail loud
	// if prependFailLoudWarning's leak pattern returns.
	_ = forbiddenUserPanelTokens
}
