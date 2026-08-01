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
	if !strings.Contains(out, "**结论：** 当前代码仍存在同类风险 — 当前代码仍缺少该锁保护。") {
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
	if strings.Contains(out, "当前代码仍存在同类风险") {
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
	if !strings.Contains(out, "当前代码仍存在同类风险") {
		t.Fatalf("evidence runs keep the asserted wording surface, got:\n%s", out)
	}
}

// G15 pins (§27.4, real_trace_campaign_20260705.md, 2026-07-09): the
// downgrade parenthetical carries a BODY disclosure — the model's preserved
// prose about the current code state is flagged as unverified this run —
// because rewriting the label alone left label and body contradicting each
// other (opendir_79_01: "未评估" beside "该代码路径仍然存在…风险仍然存在").
// Mutation contract: dropping the disclosure clause from either language
// face turns the downgrade pins red; the no-stamp pin keeps evidence runs
// disclosure-free.

func TestRenderV2BlockDecision_DowngradeBodyDisclosureZH(t *testing.T) {
	out := RenderAnswerDocument(newZeroEvidenceDowngradeDoc(), "zh")
	if !strings.Contains(out, "正文中关于当前代码状态的表述未经本轮源码验证，请以趋势性描述解读") {
		t.Fatalf("zh downgrade must disclose that the preserved body is unverified this run, got:\n%s", out)
	}
	// The disclosure is a caveat about the body, never a rewrite of it:
	// the model prose must still be present byte-verbatim.
	if !strings.Contains(out, "是。无法判断该优先级反转问题在最新代码中是否已修复。") {
		t.Fatalf("model prose must survive the disclosure verbatim, got:\n%s", out)
	}
}

func TestRenderV2BlockDecision_DowngradeBodyDisclosureEN(t *testing.T) {
	out := RenderAnswerDocument(newZeroEvidenceDowngradeDoc(), "en")
	if !strings.Contains(out, "body statements about the current code state were not verified against current source this run") {
		t.Fatalf("en downgrade must carry the body disclosure, got:\n%s", out)
	}
}

func TestRenderV2BlockDecision_NoDowngradeNoBodyDisclosure(t *testing.T) {
	doc := newZeroEvidenceDowngradeDoc()
	doc.CurrentStatusVerdictDowngrade = nil
	out := RenderAnswerDocument(doc, "zh")
	if strings.Contains(out, "未经本轮源码验证") {
		t.Fatalf("evidence runs (no stamp) must not carry the body disclosure, got:\n%s", out)
	}
	outEN := RenderAnswerDocument(doc, "en")
	if strings.Contains(outEN, "not verified against current source this run") {
		t.Fatalf("en evidence runs must not carry the body disclosure, got:\n%s", outEN)
	}
}
