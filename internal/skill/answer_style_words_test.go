package skill

import (
	"strings"
	"testing"
)

// STYLE-1 (§29.96.3) pins. The wordlist is a single-point constant:
// the answer-writing skill's style section renders its examples FROM
// AnswerStyleFillerPhrases, and the orchestrator's advisory counter
// reads the same slice. These tests pin (a) the SST wiring, (b) the
// counter's arithmetic, and (c) the soft-guidance / fact-lane framing
// so a future edit cannot silently turn the style section into a hard
// gate or drop the fact-lane fence.

func styleTestAnswerSkillOutputFormat(t *testing.T) string {
	t.Helper()
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	return sk.OutputFormat
}

// (a) SST wiring: every advisory phrase appears in the skill prompt's
// style section (rendered via answerStyleFillerPhraseList), so the
// prompt examples and the advisory counter can never drift apart.
func TestAnswerStyleWordlistWiredIntoAnswerSkill(t *testing.T) {
	of := styleTestAnswerSkillOutputFormat(t)
	if len(AnswerStyleFillerPhrases) == 0 {
		t.Fatal("AnswerStyleFillerPhrases must not be empty")
	}
	for _, p := range AnswerStyleFillerPhrases {
		if !strings.Contains(of, "「"+p+"」") {
			t.Fatalf("answer-document-skill OutputFormat missing advisory phrase 「%s」 — the style section must render from AnswerStyleFillerPhrases (single source)", p)
		}
	}
}

// (b) Counter arithmetic: totals, multiplicity, breakdown shape, and
// the zero-hit fast path.
func TestCountAnswerStyleFillerHits(t *testing.T) {
	if n, b := CountAnswerStyleFillerHits(""); n != 0 || b != "" {
		t.Fatalf("empty text: want (0, \"\"), got (%d, %q)", n, b)
	}
	if n, b := CountAnswerStyleFillerHits("主线程等待 dma_fence 共 36.757ms，排序第一。"); n != 0 || b != "" {
		t.Fatalf("clean engineering prose: want (0, \"\"), got (%d, %q)", n, b)
	}
	text := "值得注意的是，等待很长。综上所述，值得注意的是问题在 IO。"
	n, b := CountAnswerStyleFillerHits(text)
	if n != 3 {
		t.Fatalf("want total 3 hits, got %d (breakdown %q)", n, b)
	}
	if !strings.Contains(b, "「值得注意的是」×2") || !strings.Contains(b, "「综上所述」×1") {
		t.Fatalf("breakdown missing per-phrase counts: %q", b)
	}
}

// (c) Framing pins: the style section stays soft guidance and the
// fact-lane fence stays attached to it.
func TestAnswerStyleSectionStaysSoftWithFactLaneFence(t *testing.T) {
	of := styleTestAnswerSkillOutputFormat(t)
	// Soft-guidance marker: the section must keep declaring itself
	// guidance-only. (The noisy-signal red line: style never becomes
	// a rejection reason.)
	if !strings.Contains(of, "guidance only, never a rejection reason") {
		t.Fatalf("style section lost its guidance-only framing")
	}
	// Fact-lane fence: the verbatim discipline hand-off sentence must
	// survive, including the qualifier-word family examples and the
	// evidence-reference token, and must keep deferring to the
	// evidence sections' existing rules rather than minting a second
	// rule set.
	for _, want := range []string{
		"Fact lane (unchanged",
		"全额 / 折算 / 下界 / 计数当量",
		"[E#] evidence references",
		"verbatim-quote discipline those sections already state",
	} {
		if !strings.Contains(of, want) {
			t.Fatalf("fact-lane fence missing %q in answer-document-skill OutputFormat", want)
		}
	}
}
