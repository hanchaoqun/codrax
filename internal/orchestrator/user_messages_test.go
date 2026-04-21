package orchestrator

import (
	"strings"
	"testing"
)

// TestSoftMessages_LocalizeOnLanguage pins that the forced-read and
// convergence-stall strings render in the user's preferred language
// and never leak internal jargon ("CGEC", "E2", "I4", counter names,
// node IDs). Operator-facing details live in logging.Info lines, not
// in these Reasoning strings.
func TestSoftMessages_LocalizeOnLanguage(t *testing.T) {
	cases := []struct {
		name string
		lang string
		zh   bool
	}{
		{"zh", "zh", true},
		{"zh-CN mixed case", "zh-CN", true},
		{"cn alias", "cn", true},
		{"chinese alias", "chinese", true},
		{"en defaults to english", "en", false},
		{"empty defaults to english", "", false},
		{"unknown defaults to english", "fr", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := softForcedReadMessage(c.lang, 3)
			if strings.HasPrefix(got, "⟳") == false {
				t.Errorf("forced-read must start with the ⟳ soft symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "补充") {
				t.Errorf("zh: forced-read must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Filling") {
				t.Errorf("en: forced-read must be English, got %q", got)
			}

			got = softConvergenceStallMessage(c.lang)
			if strings.HasPrefix(got, "–") == false {
				t.Errorf("stall must start with the – soft symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "线索") {
				t.Errorf("zh: stall must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Finalizing") {
				t.Errorf("en: stall must be English, got %q", got)
			}

			got = softRetryHintMessage(c.lang)
			if strings.HasPrefix(got, "⟳") == false {
				t.Errorf("retry-hint must start with the ⟳ soft symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "证据") {
				t.Errorf("zh: retry-hint must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "evidence") {
				t.Errorf("en: retry-hint must be English, got %q", got)
			}

			got = softInvestigationReadyMessage(c.lang)
			if strings.HasPrefix(got, "›") == false {
				t.Errorf("investigation-ready must start with the › soft symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "调查") {
				t.Errorf("zh: investigation-ready must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Investigation") {
				t.Errorf("en: investigation-ready must be English, got %q", got)
			}

			got = softAnswerCheckRetryMessage(c.lang)
			if strings.HasPrefix(got, "⟳") == false {
				t.Errorf("answer-check must start with the ⟳ soft symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "答案") {
				t.Errorf("zh: answer-check must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Answer") {
				t.Errorf("en: answer-check must be English, got %q", got)
			}
		})
	}
}

// TestSoftRetryHintMessage_DropsInternalPromptMarkup pins the
// session-22 customer-facing fix: the retry-hint event must not
// leak the LLM-directed RetryHint body's internal prompt markup —
// customers reported thinking-feed lines like "## Evidence Gaps",
// "[MISSING]", "(entities: MidLoopHint)" that they could not
// interpret. After the fix, the event carries only the localized
// generic cue; the full body lives in the debug log.
func TestSoftRetryHintMessage_DropsInternalPromptMarkup(t *testing.T) {
	forbidden := []string{
		"## Evidence Gaps",
		"[MISSING]",
		"(entities:",
		"not yet satisfied",
		"return_value",
		"Previous attempt",
	}
	for _, lang := range []string{"en", "zh"} {
		m := softRetryHintMessage(lang)
		for _, f := range forbidden {
			if strings.Contains(m, f) {
				t.Errorf("lang=%q: message %q leaks internal prompt markup %q", lang, m, f)
			}
		}
	}
}

// TestSoftMessages_NoInternalJargon pins that the user-facing strings
// carry no internal identifiers that would leak scheduler mechanics
// into the UI. The message-owner contract says those names (CGEC,
// E2, I4, enforcer counter names, repair kind enums, node IDs) stay
// in logging.* only.
func TestSoftMessages_NoInternalJargon(t *testing.T) {
	forbidden := []string{
		// CGEC enforcer naming (previous contract).
		"CGEC", "E2", "I4", "enforcer", "forced_reads", "chains_demoted", "repair",
		// Scheduler / criterion internals newly surfaced in session 22
		// when we folded four more events into the friendly surface.
		"success criteria", "SuccessCriteria", "envShape", "hypProgress",
		"validate node", "node_id", "criterion",
		// Raw RetryHint prompt markup (would leak if a site accidentally
		// reverts to string-concatenating RetryHint bodies).
		"## Evidence Gaps", "[MISSING]", "(entities:",
		// Answer-contract violation vocabulary rendered by
		// renderViolations — must not land in the user-facing event.
		"answer contract", "AnswerContract", "violation", "pendingViolation",
	}
	messages := []string{
		softForcedReadMessage("en", 3),
		softForcedReadMessage("zh", 3),
		softConvergenceStallMessage("en"),
		softConvergenceStallMessage("zh"),
		softRetryHintMessage("en"),
		softRetryHintMessage("zh"),
		softInvestigationReadyMessage("en"),
		softInvestigationReadyMessage("zh"),
		softAnswerCheckRetryMessage("en"),
		softAnswerCheckRetryMessage("zh"),
	}
	for _, m := range messages {
		lower := strings.ToLower(m)
		for _, f := range forbidden {
			if strings.Contains(lower, strings.ToLower(f)) {
				t.Errorf("message %q leaks internal token %q", m, f)
			}
		}
	}
}
