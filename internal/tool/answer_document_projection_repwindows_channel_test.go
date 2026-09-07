package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func representativeChannelFixture() types.TraceCausalProjection {
	var nodes []types.TraceCausalProjectionNode
	for _, channel := range []string{"adjacent", "background", "", "future_channel", "on_chain"} {
		for rank := 1; rank <= 4; rank++ {
			nodes = append(nodes, types.TraceCausalProjectionNode{
				EvidenceID: fmt.Sprintf("%s-%d", channel, rank),
				Subject:    fmt.Sprintf("%s-worker-%d", channel, rank),
				Predicate:  "root_cause_primary", Object: "runnable_wait", StateKind: "runnable",
				Role: types.TraceCausalRoleRootCauseContext, ChainRelevance: channel,
				Causality: "on_wakeup_chain", Rank: rank, Tier: "primary", ChainDepth: 1,
				StartTs: 10 + float64(rank)/1000, EndTs: 10 + float64(rank+1)/1000,
				ImpactMS: 1, RunnableMS: 1, EffectiveImpactMS: 1, EffectiveImpactPublished: true,
				RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.020,
				RankBoardTarget: "target-99", RankBoardParamsFingerprint: "same-query",
			})
		}
	}
	chain := append([]types.TraceCausalProjectionNode(nil), nodes[len(nodes)-4:]...)
	return types.TraceCausalProjection{
		WindowStartTs: 10, WindowEndTs: 10.020, WakeupPath: []string{"on_chain-worker-1", "target-99"},
		RankedSeats: nodes, PrimaryRootCause: &chain[0], OnChainCauses: chain,
	}
}

func TestRepresentativeWindowPublicationDoesNotPromoteOtherRankChannels(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-reverse-%t", lang, reverse), func(t *testing.T) {
				projection := representativeChannelFixture()
				if reverse {
					for i, j := 0, len(projection.RankedSeats)-1; i < j; i, j = i+1, j-1 {
						projection.RankedSeats[i], projection.RankedSeats[j] = projection.RankedSeats[j], projection.RankedSeats[i]
					}
				}
				before, _ := json.Marshal(projection)
				blocks := runtimeTraceCausalProjectionCluster(projection, lang, runtimeTraceProjUserFocus{})
				var found bool
				for _, block := range blocks {
					if block.ID != runtimeTraceCausalProjectionBlockIDBase+runtimeTraceCausalProjectionRepresentativeSuffix {
						continue
					}
					found = true
					if len(block.Items) != runtimeTraceCausalProjectionRepresentativeWindowLimit {
						t.Fatalf("other channels consumed the three on-chain slots: %+v", block.Items)
					}
					for i, item := range block.Items {
						if item.Cells[0] != fmt.Sprintf("#%d", i+1) || !strings.Contains(item.Cells[1], fmt.Sprintf("on_chain-worker-%d", i+1)) {
							t.Fatalf("context rank was published as an on-chain rank: %+v", item)
						}
					}
				}
				if !found {
					t.Fatal("actual projection publisher lost the representative-window block")
				}
				after, _ := json.Marshal(projection)
				if string(before) != string(after) {
					t.Fatal("display selection mutated the full ranked-seat/context evidence")
				}
			})
		}
	}
}

func TestRepresentativeWindowsRequireExactChainRoleIncludingFallback(t *testing.T) {
	for _, channel := range []string{"on_chain", "adjacent", "background", "", "on_wakeup_chain", "future_channel"} {
		for _, fallback := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-fallback-%t", channel, fallback), func(t *testing.T) {
				projection := representativeChannelFixture()
				node := projection.RankedSeats[0]
				node.ChainRelevance = channel
				// A time window does not require a positive price. In particular,
				// do not add the eliminable-seat selector's separate price gate.
				node.EffectiveImpactMS, node.EffectiveImpactPublished = 0, false
				projection.RankedSeats = []types.TraceCausalProjectionNode{node}
				projection.PrimaryRootCause, projection.OnChainCauses = &node, nil
				projection.PrimaryRootCauses = []types.TraceCausalProjectionNode{node}
				if fallback {
					projection.RankedSeats = nil
				}
				block := runtimeTraceCausalProjectionRepresentativeWindowsBlock(projection, true, "runtime_trace_causal_projection", "", nil, nil)
				if (block != nil) != (channel == "on_chain") {
					t.Fatalf("only exact chain role grants the on-chain caption: channel=%q fallback=%t block=%+v", channel, fallback, block)
				}
			})
		}
	}
}
