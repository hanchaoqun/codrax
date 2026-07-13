package tool

// WF-xn (§29.52.1, docs/design/real_trace_campaign_20260705.md, 2026-07-12)
// pins: the ×N semantic-overload dissolution word family.
//
//	instance/sum   ×N(a~b)        → zh N次(a~b)          / en n=N(a~b)
//	same-value     ×N同值          → zh N次同值            / en n=N same-value
//	thread max     ×N(a~b)取最大   → zh N线程取最大(单项a~b) / en N-thread max(each a~b)
//	window max     ×N(a~b)跨窗取最大 → zh N次跨窗取最大(单项a~b) / en n=N cross-window max(each a~b)
//	union          ×N(a~b)union   → zh N次(a~b)union      / en n=N(a~b)union
//
// plus the tracefence display-table ⑥ wrap atoms: a count and its family
// word are ONE claim — a wrap boundary must never split "4次(" or
// "6线程取最大(" between the digits and the word.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestWFXnFiveFormTokensVerbatim pins the five data-token forms zh+en.
func TestWFXnFiveFormTokensVerbatim(t *testing.T) {
	node := types.TraceCausalProjectionNode{MergedCount: 4, MergedMinMS: 2.197, MergedMaxMS: 3.853}
	if got := runtimeTraceProjMergedSumTagText(node, true); got != "4次(2.197~3.853ms)" {
		t.Fatalf("zh sum form: %q", got)
	}
	if got := runtimeTraceProjMergedSumTagText(node, false); got != "n=4(2.197~3.853ms)" {
		t.Fatalf("en sum form: %q", got)
	}
	if got := runtimeTraceProjMergedMaxTagText(node, true); got != "4线程取最大(单项2.197~3.853ms)" {
		t.Fatalf("zh max form: %q", got)
	}
	if got := runtimeTraceProjMergedMaxTagText(node, false); got != "4-thread max(each 2.197~3.853ms)" {
		t.Fatalf("en max form: %q", got)
	}
	if got := runtimeTraceProjMergedCrossWindowMaxTagText(node, true); got != "4次跨窗取最大(单项2.197~3.853ms)" {
		t.Fatalf("zh cross-window max form: %q", got)
	}
	if got := runtimeTraceProjMergedCrossWindowMaxTagText(node, false); got != "n=4 cross-window max(each 2.197~3.853ms)" {
		t.Fatalf("en cross-window max form: %q", got)
	}
	if got := runtimeTraceProjMergedUnionTagText(node, true); got != "4次(2.197~3.853ms)union" {
		t.Fatalf("zh union form: %q", got)
	}
	if got := runtimeTraceProjMergedUnionTagText(node, false); got != "n=4(2.197~3.853ms)union" {
		t.Fatalf("en union form: %q", got)
	}
	if got := runtimeTraceProjDedupFoldTagText(3, true); got != "3次同值" {
		t.Fatalf("zh same-value form: %q", got)
	}
	if got := runtimeTraceProjDedupFoldTagText(3, false); got != "n=3 same-value" {
		t.Fatalf("en same-value form: %q", got)
	}
	// The retired ×N spellings never resurface on any of the five forms.
	for _, got := range []string{
		runtimeTraceProjMergedSumTagText(node, true), runtimeTraceProjMergedSumTagText(node, false),
		runtimeTraceProjMergedMaxTagText(node, true), runtimeTraceProjMergedMaxTagText(node, false),
		runtimeTraceProjMergedCrossWindowMaxTagText(node, true), runtimeTraceProjMergedCrossWindowMaxTagText(node, false),
		runtimeTraceProjMergedUnionTagText(node, true), runtimeTraceProjMergedUnionTagText(node, false),
		runtimeTraceProjDedupFoldTagText(3, true), runtimeTraceProjDedupFoldTagText(3, false),
	} {
		if strings.Contains(got, "×") {
			t.Fatalf("retired ×N spelling resurfaced: %q", got)
		}
	}
}

// TestWFXnMergeCountChipForms pins the 行1 count chip zh/en fork.
func TestWFXnMergeCountChipForms(t *testing.T) {
	if got := runtimeTraceProjMergeCountChip(14, true); got != " 14次" {
		t.Fatalf("zh chip: %q", got)
	}
	if got := runtimeTraceProjMergeCountChip(14, false); got != " n=14" {
		t.Fatalf("en chip: %q", got)
	}
}

// TestWFXnWrapAtomDigitFusion — tracefence display-table ⑥: the count digits
// and the family word fuse into one wrap atom at EVERY width, so no chunk
// ever ends with the bare count or opens with the bare word.
func TestWFXnWrapAtomDigitFusion(t *testing.T) {
	if len(tracefence.MergeCountWrapAtoms()) < 5 {
		t.Fatalf("display-table ⑥ must carry the five family word heads")
	}
	texts := []string{
		"CompThread_0-2955 · D-state 4次(2.197~3.853ms) 合计36.757ms",
		"folded-rows 6线程取最大(单项99.500~101.000ms) 置信中",
		"page-cache 3次同值 · 14次跨窗取最大(单项1.035~6.357ms)",
	}
	for _, text := range texts {
		for width := 6; width <= 40; width++ {
			chunks := runtimeTraceProjWrapDisplay(text, width)
			if strings.Join(chunks, "") != text {
				t.Fatalf("wrap must be byte-lossless at width %d: %q", width, chunks)
			}
			for i, chunk := range chunks {
				trimmed := strings.TrimRight(chunk, " ")
				if trimmed == "" {
					continue
				}
				last := trimmed[len(trimmed)-1]
				if last >= '0' && last <= '9' && i+1 < len(chunks) {
					next := strings.TrimLeft(chunks[i+1], " ")
					for _, word := range tracefence.MergeCountWrapAtoms() {
						if strings.HasPrefix(next, word) {
							t.Fatalf("width %d split the count from its word: %q | %q", width, chunk, chunks[i+1])
						}
					}
				}
			}
		}
	}
}
