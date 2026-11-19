package tool

import (
	"strings"
	"testing"
)

// Batch-owner ruling R-377-2 (2026-11-19): the business-clue promise word is
// 「自身阶段: 合计/单次/自身最长 TOP8 · 依赖线程: 单次∪合计最长 TOP8」 (and its
// English equivalent), applied consistently on the tree ◈ head, the ◎ ◈ zone
// head and the ◈ legend entry from one word source. The face selects in two
// tiers since HMC377 E3; a promise naming only the dependency tier's rule
// under-described the own-phase tier on every face.
func TestHMC377BusinessCluePromiseWordIsOneSourceOnThreeFaces(t *testing.T) {
	want := map[bool]string{
		true:  "自身阶段: 合计/单次/自身最长 TOP8 · 依赖线程: 单次∪合计最长 TOP8",
		false: "own phases: total/max-single/own-time TOP8 · dependency threads: max-single ∪ total TOP8",
	}
	var legend runtimeTraceProjLegendEntry
	for _, entry := range runtimeTraceProjLegendCatalog() {
		if entry.Mark == runtimeTraceProjMarkBusinessSpanMention {
			legend = entry
		}
	}
	for _, zh := range []bool{true, false} {
		word := runtimeTraceProjBusinessSpanSelectionRuleWord(zh)
		if word != want[zh] {
			t.Fatalf("zh=%v promise word: got %q, want %q", zh, word, want[zh])
		}
		model := buildRuntimeTraceProjTreeModel(spanvisMentionProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		tree := spanvisSquashFence(runtimeTraceProjTreeFence(model, zh))
		board := elimBoardProjection()
		board.BusinessSpanMentions = spanvisMentions()
		board.BusinessSpanMentionOmitted = 2
		_, elim := elimRenderOverview(t, board, zh)
		overview := small3SquashSpaces(spanvisSquashFence(small3UnfoldElim(elim)))
		entry := legend.EN
		if zh {
			entry = legend.ZH
		}
		for face, text := range map[string]string{
			"tree ◈ head": tree,
			"◎ ◈ head":    overview,
			"◈ legend":    entry,
		} {
			probe := word
			if face == "◎ ◈ head" {
				probe = small3SquashSpaces(word)
			}
			if got := strings.Count(text, probe); got != 1 {
				t.Fatalf("zh=%v %s must carry the promise word exactly once, got %d:\n%s", zh, face, got, text)
			}
		}
		// The entry lists the credential words the rows actually wear.
		for _, basis := range []string{"self", "chain_member", "host_wakeup_edge"} {
			word, _ := runtimeTraceProjBusinessSpanMentionBasisWord(basis, zh)
			if !strings.Contains(entry, word) {
				t.Fatalf("zh=%v ◈ legend must list the row credential word %q:\n%s", zh, word, entry)
			}
		}
		if zh && strings.Contains(legend.ZH, "唤醒边凭证") {
			t.Fatalf("the ◈ legend must not list the retired host credential word")
		}
		if zh && (strings.Contains(tree, "单次最长∪合计最长 TOP8") || strings.Contains(overview, small3SquashSpaces("单次最长∪合计最长 TOP8")) || strings.Contains(legend.ZH, "单次最长∪合计最长 TOP8")) {
			t.Fatalf("the retired one-tier promise word must not render on any face")
		}
	}
}
