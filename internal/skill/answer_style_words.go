package skill

import (
	"fmt"
	"strings"
)

// answer_style_words.go — STYLE-1 (§29.96.3, user ruling 2026-07-15):
// the single-point wordlist behind the answer-style ADVISORY lint and
// the answer-writing skill's style-guidance section.
//
// Red-line placement (CLAUDE.md "Architectural principle"): phrase
// counting over model prose is a NOISY signal. It therefore drives
// SOFT guidance only — a skill-prompt style section plus an
// observation-only WARN log line / eval observation column. It MUST
// NEVER gate an emit, enter a verdict, block publication, or be fed
// back into any LLM prompt or retry hint. Anyone wiring this list
// into a hard gate is violating the precise-signals red line.
//
// Single source of truth (SST): the answer-writing skill prompt
// renders its "avoid these fillers" examples FROM this slice (see
// answerStyleFillerPhraseList), and the orchestrator's advisory
// counter reads the same slice — extend the list here and both
// surfaces move together. The entries are generic Chinese
// AI-register fillers / meta-discourse openers (they are not tied to
// any question family or fixture); the lint is phrase-substring
// based, so entries stay Chinese-only by construction — no language
// gate is involved (an explicit !hasHan-style gate was deliberately
// avoided: language gates create smuggling surfaces, wordlist
// membership does not).
var AnswerStyleFillerPhrases = []string{
	// Empty transition / summary-template connectors.
	"值得注意的是",
	"综上所述",
	"总的来说",
	"首先让我们",
	"深入探讨",
	"不难发现",
	// First-person-plural reader-walking.
	"我们可以看到",
	// Empty evaluation filler.
	"这是一个复杂的问题",
	// Self-referential meta-discourse.
	"作为分析工具",
	"根据提供的数据",
}

// answerStyleFillerPhraseList renders the wordlist as a zh-quoted
// inline list (「a」「b」…) for the skill prompt, so the prompt's
// examples can never drift from the advisory counter's list.
func answerStyleFillerPhraseList() string {
	var b strings.Builder
	for _, p := range AnswerStyleFillerPhrases {
		b.WriteString("「")
		b.WriteString(p)
		b.WriteString("」")
	}
	return b.String()
}

// CountAnswerStyleFillerHits counts occurrences of the advisory
// wordlist phrases in a final answer's text. It returns the total hit
// count and a compact per-phrase breakdown ("「值得注意的是」×2 「综上所述」×1")
// for the observation-only WARN log line. Zero hits returns (0, "").
//
// Pure observation: no caller may branch publication, verdicts, or
// prompts on this value (see the red-line note on
// AnswerStyleFillerPhrases).
func CountAnswerStyleFillerHits(text string) (int, string) {
	if text == "" {
		return 0, ""
	}
	total := 0
	var b strings.Builder
	for _, p := range AnswerStyleFillerPhrases {
		n := strings.Count(text, p)
		if n == 0 {
			continue
		}
		total += n
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "「%s」×%d", p, n)
	}
	return total, b.String()
}
