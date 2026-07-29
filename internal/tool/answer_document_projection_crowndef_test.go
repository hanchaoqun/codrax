package tool

import (
	"strings"
	"testing"
)

// answer_document_projection_crowndef_test.go — B+ (user ruling 2026-07-28):
// 主根因 becomes a DEFINED term of art. The crowned head line's first
// occurrence carries the short definitional parenthetical, the legend
// carries the full definition (election over credentials + effective
// attribution, never a mechanism verdict), and both LLM word surfaces teach
// it. 「首要可消除项」 is the reserved rename should plan A ever be ruled.

func TestPrimaryCrownDefinitionLegendEntry(t *testing.T) {
	// The definition rides the EXISTING ➊-anchored Badge legend entry (the
	// 词条-图例双向 discipline: the anchor token lives in the fence).
	marks := &runtimeTraceProjMarkSet{}
	marks.mark(runtimeTraceProjMarkBadge)
	zh := strings.Join(runtimeTraceProjLegendGroupLines(marks, true), "\n")
	if !strings.Contains(zh, "主根因=已证链上候选中单项最大可消除量的持有席") ||
		!strings.Contains(zh, "非机理层裁定") {
		t.Fatalf("zh badge legend must carry the crown definition:\n%s", zh)
	}
	en := strings.Join(runtimeTraceProjLegendGroupLines(marks, false), "\n")
	if !strings.Contains(en, "largest single proven on-chain eliminable contribution") ||
		!strings.Contains(en, "never a mechanism-level verdict") {
		t.Fatalf("en badge legend must carry the crown definition:\n%s", en)
	}
}

func TestPrimaryCrownTeachingOnBothLLMFaces(t *testing.T) {
	tq := &TraceQuery{}
	for name, face := range map[string]string{
		"description": tq.Description(),
		"parameters":  string(tq.Parameters()),
	} {
		for _, want := range []string{
			"DEFINED term of art",
			"largest single PROVEN on-chain eliminable contribution",
			"never a mechanism-level verdict",
		} {
			if !strings.Contains(face, want) {
				t.Fatalf("%s must teach the crown definition, missing %q", name, want)
			}
		}
	}
}
