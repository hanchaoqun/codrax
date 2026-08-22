package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderRetryPrevEmitPublishesExactSplitCompanionDisposition(t *testing.T) {
	rs := &types.RetryState{PrevEmitSummary: types.RetryStateSummary{
		BlockSummaries: []types.RetryBlockSummary{
			{ID: "call-chain-diagram", Kind: types.BlockSection},
			{ID: "call-chain-diagram_diagram", Kind: types.BlockDiagram},
		},
		BlockCompanionLineages: []types.AnswerBlockCompanionLineage{{
			Kind:           types.AnswerBlockCompanionLineageFusedDiagramSplit,
			VisibleBlockID: "call-chain-diagram",
			DiagramBlockID: "call-chain-diagram_diagram",
		}},
	}}
	got := renderRetryPrevEmit(rs)
	for _, want := range []string{
		"Compatibility split companions",
		`visible="call-chain-diagram"`,
		`diagram="call-chain-diagram_diagram"`,
		"explicitly retain, replace, or remove the other",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("retry prompt missing %q:\n%s", want, got)
		}
	}
}
