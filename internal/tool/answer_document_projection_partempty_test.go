package tool

// PARTEMPTY (2026-07-24) — the empty-cluster publication branch must not
// swallow the partition boundary caveat (NW-03 同型第三处窄门:「只在某形
// 发布」). Mutation self-check: deleting the partition arm inside
// runtimeTraceProjEmptyClusterBoundaryBlocks reds the first case; deleting
// the helper call site reds the existing empty-branch coverage pins (the
// coverage block then never publishes).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTraceProjEmptyClusterBoundaryKeepsPartitionCaveat(t *testing.T) {
	set := types.TraceCausalProjectionSet{
		Projections:                  []types.TraceCausalProjection{{}},
		UnattributedObservationCount: 2,
	}
	coverage := &types.AnswerBlock{ID: "cov", Kind: types.BlockCaveat, Text: "coverage-boundary"}
	blocks := runtimeTraceProjEmptyClusterBoundaryBlocks(set, coverage, true)
	if len(blocks) != 2 {
		t.Fatalf("partition counters + coverage must publish two boundary blocks, got %d: %+v", len(blocks), blocks)
	}
	if !strings.Contains(blocks[0].Text, "无法归属") {
		t.Fatalf("partition caveat must lead the empty-cluster boundary: %q", blocks[0].Text)
	}
	if blocks[1].Text != "coverage-boundary" {
		t.Fatalf("coverage block must follow the partition caveat: %q", blocks[1].Text)
	}
	// Partition counters alone (no coverage boundary) still publish.
	if got := runtimeTraceProjEmptyClusterBoundaryBlocks(set, nil, true); len(got) != 1 ||
		!strings.Contains(got[0].Text, "无法归属") {
		t.Fatalf("partition-only boundary must still publish: %+v", got)
	}
	// Zero counters → coverage only; nothing → nothing (byte-identity lanes).
	if got := runtimeTraceProjEmptyClusterBoundaryBlocks(types.TraceCausalProjectionSet{}, coverage, true); len(got) != 1 ||
		got[0].Text != "coverage-boundary" {
		t.Fatalf("counter-free set must publish coverage only: %+v", got)
	}
	if got := runtimeTraceProjEmptyClusterBoundaryBlocks(types.TraceCausalProjectionSet{}, nil, true); len(got) != 0 {
		t.Fatalf("no boundary content must publish nothing: %+v", got)
	}
}
