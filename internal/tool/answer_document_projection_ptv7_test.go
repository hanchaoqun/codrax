package tool

// answer_document_projection_ptv7_test.go — PTV7 (#74, 用户裁定 2026-07-06):
// 内核状态词英文原词化 + 图例中文注解单点. Two mechanical pins:
//
//  1. Display-alias mapping ↔ TSH StateKind universe alignment: the 裁定4
//     wording home runtimeTraceProjStateKindLabel is THE single authoritative
//     display-alias table. Every types.TraceStateKindUniverse member maps to
//     a canonical display word from the closed set {running, runnable, sleep,
//     D-state, iowait} (golden below), and the word is FACE-INVARIANT
//     (zh == en, 双面分叉消除). The action-word lane and the #3 type-token
//     state lane draw from the same set (三 lane 单 token 集).
//
//  2. Legend state-annotation bidirectional completeness (图例中文注解单点):
//     every canonical display word is declared by EXACTLY ONE state-icon
//     legend entry head (`glyph/word` form, ⛓ carries the D-state·iowait
//     pair), and each of those zh entries carries a Chinese gloss — the
//     single point where the Chinese state semantics live. Adding a display
//     word to the mapping without adding its legend annotation (or vice
//     versa) is a test failure, not a review hope (加词不加注即红).

import (
	"strings"
	"testing"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ptv7StateKindDisplayAliasGolden is the ruled display-alias table
// (d_sleep→D-state, io_wait→iowait, s_sleep→sleep; identity elsewhere).
var ptv7StateKindDisplayAliasGolden = map[string]string{
	types.TraceStateKindRunning:              "running",
	types.TraceStateKindRunnable:             "runnable",
	types.TraceStateKindSleep:                "sleep",
	types.TraceStateKindSSleep:               "sleep",
	types.TraceStateKindSleepWait:            "sleep",
	types.TraceStateKindDSleep:               "D-state",
	types.TraceStateKindDState:               "D-state",
	types.TraceStateKindIOWait:               "iowait",
	types.TraceStateKindUninterruptibleSleep: "D-state",
}

func TestPTV7StateKindDisplayAliasUniverseAlignment(t *testing.T) {
	if len(ptv7StateKindDisplayAliasGolden) != len(types.TraceStateKindUniverse) {
		t.Fatalf("alias golden has %d entries, universe has %d — extend BOTH in the same change",
			len(ptv7StateKindDisplayAliasGolden), len(types.TraceStateKindUniverse))
	}
	for _, token := range types.TraceStateKindUniverse {
		want, ok := ptv7StateKindDisplayAliasGolden[token]
		if !ok {
			t.Fatalf("universe token %q has no display-alias golden entry", token)
		}
		zh := runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: token}, true)
		en := runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: token}, false)
		if zh != want {
			t.Errorf("token %q: display word %q, golden %q", token, zh, want)
		}
		if zh != en {
			t.Errorf("token %q: faces diverge (zh %q vs en %q) — PTV7 state words are face-invariant", token, zh, en)
		}
	}
	// The action-word lane draws from the same token set and is face-invariant
	// too (the D family speaks the joint two-sided compound).
	actionUniverse := map[string]bool{"running": true, "runnable": true, "D-state/iowait": true, "": true}
	for _, token := range types.TraceStateKindUniverse {
		zh := runtimeTraceCausalProjectionStateActionWord(token, true)
		en := runtimeTraceCausalProjectionStateActionWord(token, false)
		if zh != en {
			t.Errorf("action word for %q diverges across faces (zh %q vs en %q)", token, zh, en)
		}
		if !actionUniverse[zh] {
			t.Errorf("action word %q for token %q is outside the canonical token set", zh, token)
		}
	}
	// #3 type-token state lane: the ambiguous producer compound and the
	// single-state classes all speak canonical words on both faces.
	for class, want := range map[string]string{
		"d_state_or_io_wait": "D-state/iowait",
		"runnable":           "runnable",
		"s_sleep":            "sleep",
		"running":            "running",
		"io_wait":            "iowait",
	} {
		zh := runtimeTraceCausalProjectionTypeTokenStateWord(class, true)
		en := runtimeTraceCausalProjectionTypeTokenStateWord(class, false)
		if zh != want || en != want {
			t.Errorf("type-token state word for class %q = zh %q / en %q, want %q", class, zh, en, want)
		}
	}
}

// ptv7LegendStateEntryHead extracts the `glyph/word[·word]` head of a legend
// entry line ("- `⧖/runnable` = …") and returns the declared display words.
func ptv7LegendStateEntryHead(t *testing.T, line string) []string {
	t.Helper()
	start := strings.Index(line, "`")
	end := strings.Index(line[start+1:], "`")
	if start < 0 || end < 0 {
		t.Fatalf("state legend entry lost its `glyph/word` head: %q", line)
	}
	head := line[start+1 : start+1+end]
	slash := strings.Index(head, "/")
	if slash < 0 {
		t.Fatalf("state legend head %q carries no /word declaration (加词不加注即红)", head)
	}
	return strings.Split(head[slash+1:], "·")
}

func TestPTV7LegendStateAnnotationBidirectional(t *testing.T) {
	// Canonical display-word set, derived from the live mapping (not the
	// golden) so a mapping edit without a legend edit trips this pin.
	canonical := map[string]bool{}
	for _, token := range types.TraceStateKindUniverse {
		canonical[runtimeTraceProjStateKindLabel(types.TraceCausalProjectionNode{StateKind: token}, true)] = true
	}
	stateIconMarks := map[runtimeTraceProjMark]bool{
		runtimeTraceProjMarkIconSleep:    true,
		runtimeTraceProjMarkIconRunnable: true,
		runtimeTraceProjMarkIconRunning:  true,
		runtimeTraceProjMarkIconDState:   true,
	}
	declared := map[string]int{}
	seenEntries := 0
	for _, entry := range runtimeTraceProjLegendCatalog() {
		if !stateIconMarks[entry.Mark] {
			continue
		}
		seenEntries++
		words := ptv7LegendStateEntryHead(t, entry.ZH)
		wordsEN := ptv7LegendStateEntryHead(t, entry.EN)
		if strings.Join(words, "·") != strings.Join(wordsEN, "·") {
			t.Errorf("state legend heads diverge across faces: zh %v vs en %v", words, wordsEN)
		}
		for _, word := range words {
			declared[word]++
		}
		// 中文注解单点: the zh entry must actually gloss the word in Chinese.
		gloss := entry.ZH[strings.Index(entry.ZH, "=")+1:]
		hasCJK := false
		for _, r := range gloss {
			if unicode.Is(unicode.Han, r) {
				hasCJK = true
				break
			}
		}
		if !hasCJK {
			t.Errorf("state legend entry carries no Chinese annotation (中文注解单点): %q", entry.ZH)
		}
	}
	if seenEntries != len(stateIconMarks) {
		t.Fatalf("state-icon legend entries missing from the catalog: saw %d, want %d", seenEntries, len(stateIconMarks))
	}
	// Bidirectional set equality, exactly-once.
	for word, n := range declared {
		if !canonical[word] {
			t.Errorf("legend declares state word %q that is not a canonical display word", word)
		}
		if n != 1 {
			t.Errorf("state word %q declared by %d legend entries, want exactly one (单点承载)", word, n)
		}
	}
	for word := range canonical {
		if declared[word] == 0 {
			t.Errorf("canonical display word %q has no legend annotation entry (加词不加注即红)", word)
		}
	}
}
