package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDeniedTokenAnswerCheckSkipsAnswerSurfaceRepairStamps(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "section-public-class",
		Kind: types.BlockSection,
		Text: "The answer mentions Comparable and defaultName as ordinary explanatory prose after inline-code repair.",
	}}}
	denials := types.NewTypedDenialSet()
	denials.Add(types.TypedDenial{Class: types.TypedDenialAnswerSurfaceSymbolUnverified, Token: "Comparable"})
	denials.Add(types.TypedDenial{Class: types.TypedDenialAnswerSurfaceSymbolUnverified, Token: "defaultName"})

	if got := runDeniedTokenAnswerCheck(doc, denials); len(got) != 0 {
		t.Fatalf("answer-surface repair stamps must not become stale final-answer caveats: %+v", got)
	}
}

func TestDeniedTokenAnswerCheckKeepsDurableOracleSymbolDenials(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "The answer still names PhantomService without any boundary disclosure.",
	}}}
	denials := types.NewTypedDenialSet()
	denials.Add(types.TypedDenial{Class: types.TypedDenialOracleSymbolUnverified, Token: "PhantomService"})

	got := runDeniedTokenAnswerCheck(doc, denials)
	if len(got) != 1 || got[0].Kind != types.ViolDeniedTokenUndeclared {
		t.Fatalf("durable oracle symbol denial should remain answer-visible when unresolved: %+v", got)
	}
}
