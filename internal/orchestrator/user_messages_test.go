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
			if strings.HasPrefix(got, "·") == false {
				t.Errorf("stall must start with the · info-class symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "证据") {
				t.Errorf("zh: stall must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "evidence") {
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
			if strings.HasPrefix(got, "·") == false {
				t.Errorf("investigation-ready must start with the · symbol, got %q", got)
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

			got = softFinalizingMessage(c.lang)
			if strings.HasPrefix(got, "⟳") == false {
				t.Errorf("finalizing must start with the ⟳ soft symbol, got %q", got)
			}
			if c.zh && !strings.Contains(got, "生成") {
				t.Errorf("zh: finalizing must be Chinese, got %q", got)
			}
			if !c.zh && !strings.Contains(got, "Composing") {
				t.Errorf("en: finalizing must be English, got %q", got)
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
		softFinalizingMessage("en"),
		softFinalizingMessage("zh"),
		// Audit followup (2026-05-02) — Block 1+2+3 dock surfaces.
		softFallbackTargetMessage("en", FallbackFinalizerOnly),
		softFallbackTargetMessage("zh", FallbackFinalizerOnly),
		softFallbackTargetMessage("en", FallbackBackToExtract),
		softFallbackTargetMessage("zh", FallbackBackToExtract),
		softFallbackTargetMessage("en", FallbackBackToExplore),
		softFallbackTargetMessage("zh", FallbackBackToExplore),
		softFallbackTargetMessage("en", FallbackFailLoud),
		softFallbackTargetMessage("zh", FallbackFailLoud),
		softUpstreamFallbackCapMessage("en", 2, 2),
		softUpstreamFallbackCapMessage("zh", 2, 2),
		softYieldKillMessage("en"),
		softYieldKillMessage("zh"),
		softPlanCriticReviewMessage("en", 3),
		softPlanCriticReviewMessage("zh", 0),
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

// TestSoftFallbackTargetMessage_DistinctPerTarget pins the audit
// followup contract (2026-05-02): each fallback target produces a
// DIFFERENT user-visible line so the dock conveys which layer is
// being re-run. Pre-this-fix every target rendered the same generic
// "answer needs another pass", hiding scope.
func TestSoftFallbackTargetMessage_DistinctPerTarget(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		seen := map[string]FallbackTarget{}
		for _, tgt := range []FallbackTarget{
			FallbackFailLoud, FallbackFinalizerOnly,
			FallbackBackToExtract, FallbackBackToExplore,
		} {
			msg := softFallbackTargetMessage(lang, tgt)
			if msg == "" {
				t.Errorf("lang=%s target=%s produced empty message", lang, tgt)
				continue
			}
			if prev, dup := seen[msg]; dup {
				t.Errorf("lang=%s targets %s and %s share message %q (must be distinct)", lang, prev, tgt, msg)
			}
			seen[msg] = tgt
		}
	}
}

// TestSoftPlanCriticReviewMessage_HonoursCount pins the per-count
// branch: 0 risks renders cleanly without a count, ≥1 renders the
// count plus a /plan show pointer so the user knows where to read
// the full critique.
func TestSoftPlanCriticReviewMessage_HonoursCount(t *testing.T) {
	zero := softPlanCriticReviewMessage("en", 0)
	three := softPlanCriticReviewMessage("en", 3)
	if zero == three {
		t.Errorf("zero-risk and 3-risk messages must differ")
	}
	if !strings.Contains(three, "3 risk") {
		t.Errorf("3-risk message must surface the count: %q", three)
	}
	if !strings.Contains(three, "/plan show") {
		t.Errorf("≥1 risk message must reference /plan show as the read-more pointer: %q", three)
	}
	if strings.Contains(zero, "/plan show") {
		t.Errorf("0-risk message must NOT reference /plan show (nothing to read): %q", zero)
	}
}

// TestSoftMessages_NoVisualShockSymbols pins the operator-feedback
// red line (2026-05-02): triangles / heavy alert glyphs are too
// strong for the dock cadence — soft '·' / '⟳' / '–' / '›' only.
func TestSoftMessages_NoVisualShockSymbols(t *testing.T) {
	visualShock := []string{"⚠", "❗", "‼", "⛔", "🚨", "🔴", "✘", "✗", "❌"}
	messages := []string{
		softFallbackTargetMessage("en", FallbackFailLoud),
		softFallbackTargetMessage("zh", FallbackFailLoud),
		softFallbackTargetMessage("en", FallbackFinalizerOnly),
		softFallbackTargetMessage("en", FallbackBackToExtract),
		softFallbackTargetMessage("en", FallbackBackToExplore),
		softUpstreamFallbackCapMessage("en", 2, 2),
		softUpstreamFallbackCapMessage("zh", 2, 2),
		softYieldKillMessage("en"),
		softYieldKillMessage("zh"),
		softPlanCriticReviewMessage("en", 3),
		softPlanCriticReviewMessage("en", 0),
	}
	for _, m := range messages {
		for _, glyph := range visualShock {
			if strings.Contains(m, glyph) {
				t.Errorf("message %q contains visual-shock glyph %q (use soft '·' / '⟳' / '–' / '›' only)", m, glyph)
			}
		}
	}
}


// =====================================================================
// Phase 4 / 6 (2026-05-08) multi-repo scan progress messages
// =====================================================================

// TestMultiRepoScanMessages_NoticeStyle pins the scan-progress
// notice strings to the canonical glyph semantics (operator
// feedback, 2026-05-08 audit):
//
//	⟳ <prose>   RETRY only          (planner regenerate, apply re-run, …)
//	· <prose>   in-progress         (canonical glyphPending)
//	✓ <prose>   milestone complete  (canonical glyphSuccess)
//	✗ <prose>   failure             (canonical glyphFatal)
//
// First-time scan is in-progress (NOT a retry) → · for start, ✓
// for ok end, ✗ for fail. ⟳ MUST NOT appear here — that would
// false-signal "we are retrying the scan" to the operator.
//
// Localisation parity check across zh / en.
func TestMultiRepoScanMessages_NoticeStyle(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		start := multiRepoScanStartMessage(lang, "repo-x")
		endOK := multiRepoScanEndMessage(lang, "repo-x", 1234, true)
		endFail := multiRepoScanEndMessage(lang, "repo-x", 1234, false)
		// Glyph contract.
		if !strings.HasPrefix(start, "· ") {
			t.Errorf("%s: scan-start %q must start with \"· \" (in-progress glyph, NOT ⟳ which is retry-only)", lang, start)
		}
		if !strings.HasPrefix(endOK, "✓ ") {
			t.Errorf("%s: scan-end-ok %q must start with \"✓ \" (canonical success rune)", lang, endOK)
		}
		if !strings.HasPrefix(endFail, "✗ ") {
			t.Errorf("%s: scan-end-fail %q must start with \"✗ \" (canonical fatal rune)", lang, endFail)
		}
		// Forbid the retry glyph anywhere in the scan messages —
		// scans are first-time data loads, not retries.
		for _, m := range []string{start, endOK, endFail} {
			if strings.Contains(m, "⟳") {
				t.Errorf("%s: %q leaks the retry glyph ⟳; first-time scans must not signal retry to the operator", lang, m)
			}
		}
		// Sub-repo path wrapped in backticks (consistent with
		// abandonForcedReadMessage and other path-bearing notices).
		for _, m := range []string{start, endOK, endFail} {
			if !strings.Contains(m, "`repo-x`") {
				t.Errorf("%s: %q must wrap the sub-repo name in backticks", lang, m)
			}
		}
		// Forbid the banner-style "multi-repo:" topic prefix — the
		// soft-notice family does not use a topic prefix; the
		// glyph alone classifies the row.
		for _, m := range []string{start, endOK, endFail} {
			if strings.Contains(m, "multi-repo:") {
				t.Errorf("%s: %q leaks the banner-style \"multi-repo:\" prefix; soft-notice family does not use a topic prefix", lang, m)
			}
		}
		// Localisation parity — zh / en messages MUST differ once
		// the leading glyph is removed (otherwise the lang switch
		// is a no-op and a zh user reads English).
		strip := func(s string) string {
			runes := []rune(s)
			if len(runes) <= 2 {
				return s
			}
			return strings.TrimSpace(string(runes[1:]))
		}
		if lang == "zh" {
			zhStart := strip(start)
			enStart := strip(multiRepoScanStartMessage("en", "repo-x"))
			if zhStart == enStart {
				t.Errorf("zh start localisation is identical to en (no-op switch): %q vs %q", zhStart, enStart)
			}
		}
	}
}

func TestMultiRepoScanMessages_ElapsedFormat(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{0, "<1ms"},
		{500, "500ms"},
		{1500, "1.5s"},
		{12345, "12.3s"},
	}
	for _, c := range cases {
		if got := formatScanElapsed(c.ms); got != c.want {
			t.Errorf("formatScanElapsed(%d) = %q, want %q", c.ms, got, c.want)
		}
	}
}

