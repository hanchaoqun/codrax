package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SPR #72 pins (RTC ledger §8.3). Mutation contract:
//   - removing a wording-map entry, or adding a verdict enum member
//     without wording, turns the full-family round-trip red;
//   - removing the renderer's mapping call leaks raw tokens into zh
//     prose and turns the per-family render pins red;
//   - removing the downgrade branch drops the caveat form and turns the
//     zero-evidence pins red.

func decisionVerdictFamilyTokens() []string {
	var out []string
	for _, v := range types.AllCurrentStatusVerdicts() {
		out = append(out, string(v))
	}
	for _, v := range types.AllErrorGranularityVerdicts() {
		out = append(out, string(v))
	}
	return out
}

func TestDecisionVerdictWording_FullFamilyRoundTrip(t *testing.T) {
	seen := map[string]string{}
	for _, token := range decisionVerdictFamilyTokens() {
		wording, ok := decisionVerdictWordingZH[token]
		if !ok || strings.TrimSpace(wording) == "" {
			t.Fatalf("verdict enum %q has no zh wording; every family member must be mapped in the single wording home", token)
		}
		if strings.Contains(wording, token) || strings.Contains(wording, "_") {
			t.Fatalf("zh wording for %q must not leak the raw token shape, got %q", token, wording)
		}
		if prior, dup := seen[wording]; dup && prior != token {
			t.Fatalf("wording %q maps two distinct tokens (%q, %q); token↔wording round-trip must stay invertible", wording, prior, token)
		}
		seen[wording] = token
	}
	// The map must not carry orphan entries outside the declared families.
	family := map[string]bool{}
	for _, token := range decisionVerdictFamilyTokens() {
		family[token] = true
	}
	for token := range decisionVerdictWordingZH {
		if !family[token] {
			t.Fatalf("wording map entry %q has no declared enum member; the map's single home mirrors the enum families exactly", token)
		}
	}
}

func TestDecisionVerdictDisplay_ZHWordingENToken(t *testing.T) {
	for _, token := range decisionVerdictFamilyTokens() {
		zh := decisionVerdictDisplay(token, answerDocLangZH)
		if strings.Contains(zh, token) {
			t.Fatalf("zh display for %q must use mapped wording, got %q", token, zh)
		}
		en := decisionVerdictDisplay(token, answerDocLangEN)
		if en != "`"+token+"`" {
			t.Fatalf("en display must keep the raw token in code font (pre-SPR byte behavior), got %q", en)
		}
	}
	// Unmapped future token: fall back to code-font raw (and the family
	// pin above forces adding a mapping for declared members).
	if got := decisionVerdictDisplay("some_future_verdict", answerDocLangZH); got != "`some_future_verdict`" {
		t.Fatalf("unmapped token must fall back to code-font raw, got %q", got)
	}
}

func TestRenderV2BlockDecision_ZHCurrentStatusMappedWording(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:                   "d1",
			Kind:                 types.BlockDecision,
			Text:                 "still_present。当前代码仍缺少该锁保护。",
			CurrentStatusVerdict: types.CurrentStatusStillPresent,
		}},
	}
	out := RenderAnswerDocument(doc, "zh")
	if strings.Contains(out, "still_present") {
		t.Fatalf("zh decision surface must not leak the raw enum token, got:\n%s", out)
	}
	if !strings.Contains(out, "**结论：** 仍然存在 — 当前代码仍缺少该锁保护。") {
		t.Fatalf("zh decision must render mapped wording with preserved prose, got:\n%s", out)
	}
}

func TestRenderV2BlockDecision_ENKeepsRawTokenByteBehavior(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:                   "d1",
			Kind:                 types.BlockDecision,
			Text:                 "The guard is still missing.",
			CurrentStatusVerdict: types.CurrentStatusStillPresent,
		}},
	}
	out := RenderAnswerDocument(doc, "en")
	if !strings.Contains(out, "**Decision:** `still_present` — The guard is still missing.") {
		t.Fatalf("en decision surface must keep the raw token (pre-SPR byte behavior), got:\n%s", out)
	}
}

func newZeroEvidenceDowngradeDoc() *types.AnswerDocumentV2 {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:                   "d1",
			Kind:                 types.BlockDecision,
			SurfaceRole:          types.SurfacePrincipal,
			Text:                 "是。无法判断该优先级反转问题在最新代码中是否已修复。",
			CurrentStatusVerdict: types.CurrentStatusStillPresent,
		}},
	}
	doc.CurrentStatusVerdictDowngrade = &types.CurrentStatusVerdictDowngrade{
		BlockID:         "d1",
		OriginalVerdict: types.CurrentStatusStillPresent,
		Reason:          types.CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence,
	}
	return doc
}

func TestRenderV2BlockDecision_ZeroEvidenceDowngradeCaveatFormZH(t *testing.T) {
	out := RenderAnswerDocument(newZeroEvidenceDowngradeDoc(), "zh")
	if !strings.Contains(out, "未评估：本轮无源码证据") {
		t.Fatalf("zero-evidence downgrade must render the caveat form, got:\n%s", out)
	}
	if !strings.Contains(out, "原始判定 `still_present` 仅留档，未消费") {
		t.Fatalf("original verdict must stay visible in the audit position, got:\n%s", out)
	}
	if !strings.Contains(out, "是。无法判断该优先级反转问题在最新代码中是否已修复。") {
		t.Fatalf("model prose must be preserved verbatim (downgrade is disclosure, not rewrite), got:\n%s", out)
	}
	if strings.Contains(out, "仍然存在") {
		t.Fatalf("downgraded verdict must not render as an asserted conclusion wording, got:\n%s", out)
	}
}

func TestRenderV2BlockDecision_ZeroEvidenceDowngradeCaveatFormEN(t *testing.T) {
	out := RenderAnswerDocument(newZeroEvidenceDowngradeDoc(), "en")
	if !strings.Contains(out, "Not evaluated: no current-source evidence in this run") ||
		!strings.Contains(out, "original verdict `still_present` retained for audit only") {
		t.Fatalf("en zero-evidence downgrade must render caveat + audit disclosure, got:\n%s", out)
	}
	if strings.Contains(out, "**Decision:** `still_present` — ") {
		t.Fatalf("en downgraded verdict must not render as the plain asserted form, got:\n%s", out)
	}
}

func TestRenderV2BlockDecision_NoStampKeepsAssertedForm(t *testing.T) {
	doc := newZeroEvidenceDowngradeDoc()
	doc.CurrentStatusVerdictDowngrade = nil
	out := RenderAnswerDocument(doc, "zh")
	if strings.Contains(out, "未评估：本轮无源码证据") {
		t.Fatalf("evidence runs (no stamp) must not render the caveat form, got:\n%s", out)
	}
	if !strings.Contains(out, "仍然存在") {
		t.Fatalf("evidence runs keep the asserted wording surface, got:\n%s", out)
	}
}
