package render

import "testing"

// TestLocalizeRetryReason pins the bilingual mapping that lifts the
// raw English short-phrase reasons internal/llm.retryReasonForError
// emits ("rate limit", "quota", "stream first-byte timeout", etc.)
// to user-facing prose in the operator's configured language.
//
// Pre-2026-05-02 the dock surfaced these phrases verbatim, mixing
// English jargon into Chinese dock prose ("已重试 (第 2 次,等 4s) ·
// rate limit"). Operators reading zh locale should see Chinese
// throughout the dock surface.
//
// Unknown reasons fall through verbatim — better to show an
// unrecognised label than silently drop the cause classification.
func TestLocalizeRetryReason(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		lang   string
		want   string
	}{
		{name: "zh transient", reason: "transient", lang: "zh", want: "临时网络抖动"},
		{name: "zh first-byte", reason: "stream first-byte timeout", lang: "zh", want: "首字节超时"},
		{name: "zh quota", reason: "quota", lang: "zh", want: "被临时限流"},
		{name: "zh rate limit", reason: "rate limit", lang: "zh", want: "限流(请求过频)"},
		{name: "zh server 502", reason: "server 502", lang: "zh", want: "服务端错误 (502)"},
		{name: "zh server 503", reason: "server 503", lang: "zh", want: "服务端错误 (503)"},
		{name: "en passthrough rate limit", reason: "rate limit", lang: "en", want: "rate limit"},
		{name: "en passthrough quota", reason: "quota", lang: "en", want: "quota"},
		{name: "zh empty", reason: "", lang: "zh", want: ""},
		{name: "zh unknown passthrough", reason: "novel error label", lang: "zh", want: "novel error label"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := localizeRetryReason(tc.reason, tc.lang)
			if got != tc.want {
				t.Errorf("localizeRetryReason(%q, %q) = %q, want %q", tc.reason, tc.lang, got, tc.want)
			}
		})
	}
}

// TestActivityRetryingPhrase_NoInternalJargon locks the user-facing
// retry phrase contract: the dock activity row 1 string MUST
// explicitly name what is being retried (model request) so the
// operator does not have to guess between LLM-call retry, tool-
// call retry, or stage retry. It MUST NOT leak internal layer
// names ("L1") or issue-tracker syntax ("#N").
func TestActivityRetryingPhrase_NoInternalJargon(t *testing.T) {
	state := activityState{kind: activityRetrying, retryAttempt: 2, retryDelaySec: 4}
	for _, lang := range []string{"zh", "en"} {
		got := activityPhrase(state, lang)
		bannedJargon := []string{"L1", "#2", "#1"}
		for _, banned := range bannedJargon {
			if contains(got, banned) {
				t.Errorf("%s: retry phrase leaked internal jargon %q: %q", lang, banned, got)
			}
		}
		if !contains(got, "2") {
			t.Errorf("%s: retry phrase must surface the attempt count; got %q", lang, got)
		}
	}
	zh := activityPhrase(state, "zh")
	if !contains(zh, "重新请求模型") {
		t.Errorf("zh retry phrase must spell out '重新请求模型' (the LLM call); got %q", zh)
	}
	en := activityPhrase(state, "en")
	if !contains(en, "model request") {
		t.Errorf("en retry phrase must spell out 'model request' (the LLM call); got %q", en)
	}
}

// contains is a thin testing helper kept local so test files in this
// package don't have to import strings just for one substring check.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
