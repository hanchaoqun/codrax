package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestApplyHedgeMarkerToV2BlockIsIdempotentAndOnlyUpgrades(t *testing.T) {
	blk := types.AnswerBlock{Text: "root cause"}
	applyHedgeMarkerToV2Block(&blk, HedgeMarkerConditional, answerDocLangEN)
	applyHedgeMarkerToV2Block(&blk, HedgeMarkerConditional, answerDocLangEN)
	if got, want := blk.Text, "[hedged] root cause"; got != want {
		t.Fatalf("repeated marker = %q, want %q", got, want)
	}

	applyHedgeMarkerToV2Block(&blk, HedgeMarkerHistorical, answerDocLangEN)
	if got, want := blk.Text, "[historical] root cause"; got != want {
		t.Fatalf("upgraded marker = %q, want %q", got, want)
	}
	applyHedgeMarkerToV2Block(&blk, HedgeMarkerConditional, answerDocLangEN)
	if got, want := blk.Text, "[historical] root cause"; got != want {
		t.Fatalf("weaker replay downgraded marker: got %q, want %q", got, want)
	}
}

func TestAddV2AuthorityCaveatReconcilesPrivateParagraph(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "user_caveat", Kind: types.BlockCaveat, Text: "Keep this model caveat."},
		{ID: "old_system", Kind: types.BlockCaveat, Text: "stale\n\n" + AuthorityCaveatPrefix + AuthorityCaveatTag() + "old"},
	}}
	evidence := []types.EvidenceItem{{Authority: types.AuthorityConditional}}

	addV2AuthorityCaveat(doc, evidence, answerDocLangEN)
	first := doc.Blocks[0].Text
	addV2AuthorityCaveat(doc, evidence, answerDocLangEN)
	if doc.Blocks[0].Text != first {
		t.Fatalf("repeated reconciliation changed canonical caveat:\nfirst=%q\nsecond=%q", first, doc.Blocks[0].Text)
	}
	joined := doc.Blocks[0].Text + "\n" + doc.Blocks[1].Text
	if got := strings.Count(joined, AuthorityCaveatTag()); got != 1 {
		t.Fatalf("private authority paragraph count = %d, want 1: %q", got, joined)
	}
	for _, want := range []string{"Keep this model caveat.", "stale"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("non-system caveat %q was lost: %q", want, joined)
		}
	}
}

func TestAddV2AuthorityCaveatRemovesStaleSystemParagraphWhenNoLongerNeeded(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "caveat",
		Kind: types.BlockCaveat,
		Text: "keep\n\n" + AuthorityCaveatPrefix + AuthorityCaveatTag() + "stale",
	}}}
	addV2AuthorityCaveat(doc, nil, answerDocLangEN)
	if got, want := doc.Blocks[0].Text, "keep"; got != want {
		t.Fatalf("stale generated caveat was not removed: got %q, want %q", got, want)
	}
}
