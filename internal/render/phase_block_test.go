package render

import (
	"strings"
	"testing"
)

// TestFormatPhaseGroupBlock_RendersCircleEnumeration pins
// commit 43 task 2: the phase-group block uses the same
// circle-marker enumeration as formatSubTopicsBlock so multi-
// phase write-mode runs feel like a sibling of the analyzer
// sub-topic block. Empty list → empty output.
func TestFormatPhaseGroupBlock_RendersCircleEnumeration(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			phases := []PhaseInfo{
				{Index: 0, Goal: "add migration"},
				{Index: 1, Goal: "update ORM"},
				{Index: 2, Goal: "deprecate column"},
			}
			block := formatPhaseGroupBlock(lang, phases)
			if block == "" {
				t.Fatal("expected non-empty block for 3 phases")
			}
			// Must contain circle markers ① ② ③.
			for _, want := range []string{"①", "②", "③"} {
				if !strings.Contains(block, want) {
					t.Errorf("%s: block missing circle marker %q; got:\n%s", lang, want, block)
				}
			}
			// Must include each goal.
			for _, want := range []string{"add migration", "update ORM", "deprecate column"} {
				if !strings.Contains(block, want) {
					t.Errorf("%s: block missing goal %q; got:\n%s", lang, want, block)
				}
			}
			// Header must mention phase count.
			if !strings.Contains(block, "3") {
				t.Errorf("%s: header missing phase count 3; got:\n%s", lang, block)
			}
		})
	}
}

// TestFormatPhaseGroupBlock_EmptyReturnsEmpty pins the
// degraded-call case: no phases → no block (avoids polluting
// the dock with a bare header).
func TestFormatPhaseGroupBlock_EmptyReturnsEmpty(t *testing.T) {
	if got := formatPhaseGroupBlock("en", nil); got != "" {
		t.Errorf("empty list should produce empty output; got %q", got)
	}
}

// TestFormatPhaseProgressLine_IconByKind pins the icon
// selection: start gets ▶, accepted gets ✓, rejected gets ✗.
// Pre-commit-43 these went through formatReasoning's 💭
// thought-bubble path, which was semantically wrong (the
// events represent structural progression, not LLM thinking).
func TestFormatPhaseProgressLine_IconByKind(t *testing.T) {
	cases := []struct {
		kind     PhaseProgressKind
		wantIcon string
	}{
		{PhaseProgressStart, "▶"},
		{PhaseProgressAccepted, "✓"},
		{PhaseProgressRejected, "✗"},
	}
	for _, c := range cases {
		got := formatPhaseProgressLine("en", 1, 3, c.kind, "demo detail")
		if !strings.Contains(got, c.wantIcon) {
			t.Errorf("kind=%v: expected icon %q in line; got %q", c.kind, c.wantIcon, got)
		}
		// Must NOT contain the 💭 thought-bubble icon used by
		// the legacy EventAgentReasoning path.
		if strings.Contains(got, "💭") {
			t.Errorf("kind=%v: line should not carry the 💭 LLM-thinking icon; got %q", c.kind, got)
		}
		// Must include "Phase 2/3" position label (1-based).
		if !strings.Contains(got, "Phase 2/3") {
			t.Errorf("kind=%v: line missing position label 'Phase 2/3'; got %q", c.kind, got)
		}
	}
}

// TestActivityPhrase_AcceptanceReview pins commit 44: the
// new activityAcceptanceReview state renders the dedicated
// "验收审查中" / "acceptance review" word so dock row 1 stops
// lying with "请求模型中" during the synchronous LLM Check
// dispatch.
func TestActivityPhrase_AcceptanceReview(t *testing.T) {
	zh := activityPhrase(activityState{kind: activityAcceptanceReview}, "zh")
	en := activityPhrase(activityState{kind: activityAcceptanceReview}, "en")
	if zh != "验收审查中" {
		t.Errorf("zh phrase drift; got %q", zh)
	}
	if en != "acceptance review" {
		t.Errorf("en phrase drift; got %q", en)
	}
}

// TestFormatPhaseProgressLine_DetailOptional pins that the
// terse accepted-confirmation path produces a clean line
// without the dangling colon-detail when detail is empty.
func TestFormatPhaseProgressLine_DetailOptional(t *testing.T) {
	got := formatPhaseProgressLine("en", 0, 3, PhaseProgressAccepted, "")
	if strings.Contains(got, ": ") {
		t.Errorf("empty detail should not produce ': ' separator; got %q", got)
	}
	got = formatPhaseProgressLine("en", 0, 3, PhaseProgressStart, "phase goal text")
	if !strings.Contains(got, "phase goal text") {
		t.Errorf("non-empty detail should be appended; got %q", got)
	}
}
