package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// === P7 (2026-05-10) — user-facing surface polish + P6 channel correction ===

// TestAppendSystemCaveatString_ChannelDistinct — system caveats land
// under the "**补充说明：**" / "**Additional notes:**" heading, NOT
// the LLM-authored "**说明**：" / "**Caveats:**" channel. Pinning the
// distinction is core to the P6 → P7 channel correction.
func TestAppendSystemCaveatString_ChannelDistinct(t *testing.T) {
	cases := []struct {
		name        string
		lang        string
		wantHeading string
	}{
		{"chinese", "zh", "**补充说明：**"},
		{"english", "en", "**Additional notes:**"},
		{"empty defaults to zh", "", "**补充说明：**"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AppendSystemCaveatString("answer body", "system note", tc.lang)
			if !strings.Contains(got, "answer body") {
				t.Errorf("answer body must be preserved; got %q", got)
			}
			if !strings.Contains(got, tc.wantHeading) {
				t.Errorf("expected heading %q in output; got %q", tc.wantHeading, got)
			}
			if !strings.Contains(got, "- system note") {
				t.Errorf("caveat content must be bulleted under heading; got %q", got)
			}
			// Must NOT use the LLM-authored channel headings.
			for _, banned := range []string{"**说明**：", "**Caveats:**"} {
				if strings.Contains(got, banned) {
					t.Errorf("system channel must NOT use LLM-authored heading %q; got %q",
						banned, got)
				}
			}
		})
	}
}

// TestAppendSystemCaveatString_EmptyInputs — whitespace-only caveat
// returns the answer unchanged (no spurious heading).
func TestAppendSystemCaveatString_EmptyInputs(t *testing.T) {
	cases := []string{"", "   ", "\n\t"}
	for _, c := range cases {
		got := AppendSystemCaveatString("ans", c, "zh")
		if got != "ans" {
			t.Errorf("whitespace-only caveat %q should return answer unchanged; got %q", c, got)
		}
	}
}

func TestAppendSystemCaveatString_ReusesExistingSystemHeading(t *testing.T) {
	first := AppendSystemCaveatString("answer body", "first note", "zh")
	got := AppendSystemCaveatString(first, "second note", "zh")
	if count := strings.Count(got, "**补充说明：**"); count != 1 {
		t.Fatalf("system heading count = %d, want 1:\n%s", count, got)
	}
	if !strings.Contains(got, "- first note") || !strings.Contains(got, "- second note") {
		t.Fatalf("both caveat bullets should be preserved:\n%s", got)
	}
}

// TestSemanticQualityReviewStartMessage_RedlineAudit — pin the
// reviewer-start cue's R6 (no internal vocab) + CN+EN-purity +
// generic shape.
func TestSemanticQualityReviewStartMessage_RedlineAudit(t *testing.T) {
	zh := semanticQualityReviewStartMessage("zh")
	en := semanticQualityReviewStartMessage("en")
	if !strings.Contains(zh, "›") || !strings.Contains(en, "›") {
		t.Errorf("progress glyph › should appear in both branches; got zh=%q en=%q", zh, en)
	}
	for _, msg := range []string{zh, en} {
		for _, banned := range []string{
			"FacetCoverage", "G5", "reviewer", "Reviewer",
			"ViolKind", "AnchoredCount", "DeclaredCount",
			"semantic_quality", "answer-document-skill",
		} {
			if strings.Contains(msg, banned) {
				t.Errorf("must not leak internal token %q; got %q", banned, msg)
			}
		}
	}
}

// TestSystemSubAgentFanoutStartMessage_RedlineAudit pins the
// system-driven explorer fan-out cue (2026-05-17). Same redline contract
// as the two reviewer-start messages: progress glyph (›), no internal
// vocabulary, CN+EN-only, user-vocabulary phrasing.
func TestSystemSubAgentFanoutStartMessage_RedlineAudit(t *testing.T) {
	zh := systemSubAgentFanoutStartMessage("zh", 3)
	en := systemSubAgentFanoutStartMessage("en", 3)
	if !strings.Contains(zh, "›") || !strings.Contains(en, "›") {
		t.Errorf("progress glyph › should appear in both branches; got zh=%q en=%q", zh, en)
	}
	if !strings.Contains(zh, "3") || !strings.Contains(en, "3") {
		t.Errorf("sub-task count must surface in the prose; got zh=%q en=%q", zh, en)
	}
	for _, msg := range []string{zh, en} {
		for _, banned := range []string{
			// Internal pipeline / red-line §6 vocabulary that must not
			// leak to user-facing status lines.
			"sub_topic", "SubTopic", "fan-out", "fanout", "SubAgentRuntime",
			"SubAgent", "sub_explorer", "BaseAgent", "AnalysisIR",
			"propose_sub_agents", "explorer.go", "orchestrator",
		} {
			if strings.Contains(msg, banned) {
				t.Errorf("must not leak internal token %q; got %q", banned, msg)
			}
		}
	}
	// English must read as user-facing prose (no snake_case, no
	// PascalCase, no backticked identifiers).
	if strings.Contains(en, "_") || strings.Contains(en, "`") {
		t.Errorf("English message must avoid snake_case and backticks; got %q", en)
	}
}

// TestPolishedMessages_NoTechnicalJargon — pin the 4 P7 polish
// targets (BackToExplore / completeness gap / answer-check retry /
// generic retry hint). Each must read in user vocabulary, not the
// pre-P7 technical phrasing.
func TestPolishedMessages_NoTechnicalJargon(t *testing.T) {
	// softRetryHintMessage: was "正在补齐调查证据" / "Gathering more evidence"
	zh := softRetryHintMessage("zh")
	if strings.Contains(zh, "调查") {
		t.Errorf("softRetryHintMessage CN should not say '调查' (technical); got %q", zh)
	}

	// softAnswerCheckRetryMessage: was "答案待完善，再跑一轮"
	zh = softAnswerCheckRetryMessage("zh")
	if strings.Contains(zh, "再跑一轮") {
		t.Errorf("softAnswerCheckRetryMessage CN should not say '再跑一轮' (technical); got %q", zh)
	}
	if !strings.Contains(zh, "重新生成") {
		t.Errorf("softAnswerCheckRetryMessage CN should explicitly name the action; got %q", zh)
	}

	// softFallbackTargetMessage(BackToExplore): was "证据不足，继续探索"
	zh = softFallbackTargetMessage("zh", FallbackBackToExplore)
	if strings.Contains(zh, "证据不足") {
		t.Errorf("softFallbackTargetMessage(BackToExplore) CN should not say '证据不足' (易误读); got %q", zh)
	}

	// softCompletenessGapMessage: was "维度可能覆盖不足" / "提示模型补足"
	gap := softCompletenessGapMessage("zh", &agent.CompletenessFailure{Dimension: agent.DimensionScalarCount})
	if strings.Contains(gap, "维度") {
		t.Errorf("softCompletenessGapMessage CN should not say '维度' (technical); got %q", gap)
	}
	if strings.Contains(gap, "提示模型") {
		t.Errorf("softCompletenessGapMessage CN should not surface '模型' (internal); got %q", gap)
	}
}

// TestAnalyzeExhaustedUserHint_BothLanguages — error fallback hint
// stays generic + user-actionable + mono-language per branch.
func TestAnalyzeExhaustedUserHint_BothLanguages(t *testing.T) {
	zh := analyzeExhaustedUserHint(&types.BusContext{Language: "zh"})
	en := analyzeExhaustedUserHint(&types.BusContext{Language: "en"})

	// Each must reference the actionable next step.
	if !strings.Contains(zh, "细化") || !strings.Contains(zh, "日志") {
		t.Errorf("CN hint should suggest narrow + log attach; got %q", zh)
	}
	if !strings.Contains(en, "narrow") || !strings.Contains(en, "log") {
		t.Errorf("EN hint should suggest narrow + log attach; got %q", en)
	}

	// CN purity: no English prose words mid-sentence (excepting
	// the inline `/repos` code identifier reserved for multi-repo
	// branches — this single-repo CN should NOT mention /repos).
	if strings.Contains(zh, "/repos") {
		t.Errorf("single-repo CN must NOT reference /repos (misleading); got %q", zh)
	}
	if strings.Contains(en, "/repos") {
		t.Errorf("single-repo EN must NOT reference /repos (misleading); got %q", en)
	}

	// R6: no internal vocab.
	for _, msg := range []string{zh, en} {
		for _, banned := range []string{
			"analyze stage", "AnalysisIR", "RequestModel",
			"orchestrator", "TaskGraph", "BusContext",
		} {
			if strings.Contains(msg, banned) {
				t.Errorf("must not leak internal token %q; got %q", banned, msg)
			}
		}
	}
}
