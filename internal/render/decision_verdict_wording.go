package render

import "github.com/hanchaoqun/codrax/internal/types"

// decision_verdict_wording.go — SPR #72 (RTC ledger §8.3, 2026-07-06).
//
// SINGLE display-wording home for decision-block verdict enums. The
// renderer previously forwarded raw internal enum tokens (`still_present`,
// `per_item_rejection`, …) straight into user-facing prose; in zh answers
// that is leaked jargon (same de-jargon direction as 3a6673ef). English
// answers keep the backtick token (the enum IS readable English); Chinese
// answers get the mapped wording.
//
// Pin contract (decision_verdict_wording_test.go): every member of BOTH
// verdict enum families (AllCurrentStatusVerdicts +
// AllErrorGranularityVerdicts) MUST have a zh wording here, wordings for
// distinct tokens MUST be distinct (token↔wording round-trip stays
// invertible), and no wording may contain its raw token. Adding an enum
// value without a mapping, or removing a mapping, turns the pin red.

// decisionVerdictWordingZH maps raw verdict enum tokens (both the
// current_status_verdict and error_granularity_verdict families) to
// user-facing Chinese wording. Keyed by raw token string so a token shared
// across families (`not_enough_evidence`) has exactly one wording.
var decisionVerdictWordingZH = map[string]string{
	// current_status_verdict family
	string(types.CurrentStatusStillPresent):      "仍然存在",
	string(types.CurrentStatusFixed):             "已修复",
	string(types.CurrentStatusNotEnoughEvidence): "证据不足",
	// error_granularity_verdict family (not_enough_evidence shared above)
	string(types.ErrorGranularityPerItemRejection): "逐条拒绝",
	string(types.ErrorGranularityWholeBatch):       "整批失败",
	string(types.ErrorGranularityPartialSuccess):   "部分成功",
	string(types.ErrorGranularityFailFast):         "遇错即停",
	string(types.ErrorGranularityCollectErrors):    "汇总报错",
}

// decisionVerdictDisplay renders a decision verdict enum for the given
// answer language. en keeps the raw token in code font (byte-identical to
// the pre-SPR surface); zh uses the mapped wording. An unmapped token
// falls back to the code-font raw token — and the family pin test makes
// that fallback unreachable for declared enum members.
func decisionVerdictDisplay(raw string, lang answerDocLang) string {
	if lang == answerDocLangZH {
		if wording, ok := decisionVerdictWordingZH[raw]; ok {
			return wording
		}
	}
	return "`" + raw + "`"
}

// decisionVerdictDowngradeDisplay renders the caveat form for a
// current-status verdict whose run-level evidence downgrade is stamped on
// the document (zero current_source evidence in the origin-lane ledger).
// The original verdict token is disclosed verbatim as the audit position;
// it is deliberately NOT rendered through the wording map so the surface
// cannot read as an asserted conclusion.
func decisionVerdictDowngradeDisplay(raw string, lang answerDocLang) string {
	if lang == answerDocLangZH {
		return "未评估：本轮无源码证据（原始判定 `" + raw + "` 仅留档，未消费）"
	}
	return "Not evaluated: no current-source evidence in this run (original verdict `" + raw + "` retained for audit only)"
}
