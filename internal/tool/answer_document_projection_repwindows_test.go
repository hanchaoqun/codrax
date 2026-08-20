package tool

// Typed representative-window publication pins.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTraceCausalProjectionRepresentativeWindowsBlockUsesTypedRankedSeats(t *testing.T) {
	rank1 := types.TraceCausalProjectionNode{
		Subject:        "worker-a-101",
		Predicate:      "root_cause_primary",
		Object:         "priority_inversion_candidate",
		Rank:           1,
		Tier:           "primary",
		Causality:      "on_wakeup_chain",
		ChainRelevance: "on_chain",
		StartTs:        10.125,
		EndTs:          10.130,
	}
	rank2 := types.TraceCausalProjectionNode{
		Subject:        "worker-b-102",
		Predicate:      "root_cause_secondary",
		Object:         "io_wait",
		Rank:           2,
		Tier:           "secondary",
		Causality:      "on_wakeup_chain",
		ChainRelevance: "on_chain",
		StartTs:        10.140,
		EndTs:          10.149,
	}
	projection := types.TraceCausalProjection{
		PrimaryRootCause: &rank1,
		RankedSeats: []types.TraceCausalProjectionNode{
			rank2,
			rank1,
			rank1, // exact duplicate must not consume a visible row
			{
				Subject:   "invalid-zero-start",
				Predicate: "root_cause_tertiary",
				Rank:      3,
				StartTs:   0,
				EndTs:     0,
			},
		},
		WindowStartTs: 10,
		WindowEndTs:   10.2,
		WakeupPath:    []string{"worker-a-101", "target-200"},
	}

	block := runtimeTraceCausalProjectionRepresentativeWindowsBlock(
		projection,
		true,
		runtimeTraceCausalProjectionBlockIDBase,
		"",
		[]types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}},
		[]string{"observed_artifact_fact"},
	)
	if block == nil {
		t.Fatal("typed ranked-seat intervals must mint a representative-window block")
	}
	if block.ID != "runtime_trace_causal_projection_representative_windows" ||
		block.Title != "代表性时间窗" ||
		block.Kind != types.BlockTable {
		t.Fatalf("unexpected block identity: %+v", block)
	}
	if len(block.Items) != 2 {
		t.Fatalf("valid exact windows should dedupe and retain two rows, got %+v", block.Items)
	}
	if got := block.Items[0].Cells; len(got) != 4 ||
		got[0] != "#1" ||
		!strings.Contains(got[1], "worker-a-101") ||
		got[2] != "10.125000..10.130000" ||
		!strings.Contains(got[3], "不能把它当作此单窗时长") {
		t.Fatalf("rank-1 representative row lost identity/window/caliber: %+v", got)
	}
	if got := block.Items[1].Cells; len(got) != 4 ||
		got[0] != "#2" ||
		!strings.Contains(got[1], "worker-b-102") ||
		got[2] != "10.140000..10.149000" {
		t.Fatalf("rank-2 representative row is not rank ordered: %+v", got)
	}
	for _, id := range []string{
		block.ID,
		"runtime_trace_causal_projection_a2_representative_windows",
	} {
		if !RuntimeTraceSystemBlockID(id) {
			t.Fatalf("representative-window id %q must belong to the exact reserved system family", id)
		}
	}
}

func TestRuntimeTraceCausalProjectionRepresentativeWindowsBlockOmitsInvalidIntervals(t *testing.T) {
	projection := types.TraceCausalProjection{
		RankedSeats: []types.TraceCausalProjectionNode{
			{Subject: "closed", Rank: 1, StartTs: 4, EndTs: 4},
			{Subject: "reversed", Rank: 2, StartTs: 5, EndTs: 4},
			{Subject: "negative", Rank: 3, StartTs: -1, EndTs: 1},
		},
	}
	if got := runtimeTraceCausalProjectionRepresentativeWindowsBlock(
		projection, false, runtimeTraceCausalProjectionBlockIDBase, "", nil, nil,
	); got != nil {
		t.Fatalf("invalid or zero intervals must not manufacture representative windows: %+v", got)
	}
}

func TestRuntimeTraceCausalProjectionRepresentativeWindowsExcludeExpandedQueryBoard(t *testing.T) {
	exact := types.TraceCausalProjectionNode{
		EvidenceID: "exact", Subject: "worker", Rank: 1,
		StartTs: 1.001, EndTs: 1.009,
		RankQueryWindowStartTs: 1, RankQueryWindowEndTs: 1.010,
	}
	expanded := types.TraceCausalProjectionNode{
		EvidenceID: "expanded", Subject: "target", Rank: 2,
		StartTs: 1.010, EndTs: 1.010020,
		RankQueryWindowStartTs: 1, RankQueryWindowEndTs: 1.011,
	}
	projection := types.TraceCausalProjection{
		WindowStartTs: 1, WindowEndTs: 1.010,
		RankedSeats: []types.TraceCausalProjectionNode{exact, expanded},
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{
			Subject: "target", WindowStartTs: 1, WindowEndTs: 1.010, TotalMS: 10,
		},
	}
	block := runtimeTraceCausalProjectionRepresentativeWindowsBlock(
		projection, true, runtimeTraceCausalProjectionBlockIDBase, "", nil, nil,
	)
	if block == nil || len(block.Items) != 1 || block.Items[0].Cells[0] != "#1" ||
		strings.Contains(block.Items[0].Cells[1], "target") {
		t.Fatalf("expanded query board leaked into selected-window representative rows: %+v", block)
	}
}
