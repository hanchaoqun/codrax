package types

// trace_causal_projection_omgclean_test.go — OMGCLEAN-1 件2 pins (§29.175
// 定谳② class-C carriage bug / design G2+G4, 2026-07-20): the ×N same-kind
// merge adopts FixDirection from the rank-supplying member on typed unanimity
// — empty-slot only, conflict keeps the honest empty slot (tail placement),
// and the survivor's own published direction always wins.
//
// MUTATION self-checks (cp-copy recovery only): dropping the adoption block
// in traceCausalProjectionMergeSameKindMembers reds the adoption arm; dropping
// the conflict veto reds the conflict arm (a false single direction would
// mint); re-ordering the empty-slot guard reds the survivor arm.

import (
	"reflect"
	"testing"
)

func omgcleanMergeMember(id string, rank int, direction string, eff float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		EvidenceID:   "E-" + id,
		Subject:      "worker-77",
		Predicate:    "wakeup_causal_impact",
		Object:       "running",
		StateKind:    "running",
		Rank:         rank,
		FixDirection: direction,
		ImpactMS:     eff, CumulativeImpactMS: eff, EffectiveImpactMS: eff,
		ChainRelevance: "on_chain", LineStart: 100, LineEnd: 200,
	}
}

// TestOmgcleanMergeAdoptsRankSupplierDirection — 正臂 (the runnable_2 E9
// carriage shape): a direction-bare census/chain-view group-first survivor
// merges a direction-stamped rank member — the merged row adopts the
// rank-supplying member's direction (the same member whose board/window
// identity the row already wears).
func TestOmgcleanMergeAdoptsRankSupplierDirection(t *testing.T) {
	seed := omgcleanMergeMember("seed", 0, "", 10.0)
	rankSeat := omgcleanMergeMember("rank", 2, "frequency_thermal", 4.0)
	rankSeat.RankBoardTarget = "com.baidu.tieba-59566"
	plain := omgcleanMergeMember("plain", 0, "", 2.684)
	nodes := []TraceCausalProjectionNode{seed, rankSeat, plain}
	merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1, 2})
	if merged.FixDirection != "frequency_thermal" {
		t.Fatalf("件2: the merged row must adopt the rank supplier's direction, got %q", merged.FixDirection)
	}
	if merged.Rank != 2 || merged.RankBoardTarget != "com.baidu.tieba-59566" {
		t.Fatalf("件2: the direction must travel with the SAME rank/board identity (rank=%d board=%q)",
			merged.Rank, merged.RankBoardTarget)
	}
}

// TestOmgcleanMergeConflictKeepsEmptySlot — 冲突负臂 (宁漏勿假指): two rank
// members publishing DIFFERENT directions veto the adoption — the slot stays
// empty and the row keeps the honest tail placement, even though the winning
// rank member has a direction of its own.
func TestOmgcleanMergeConflictKeepsEmptySlot(t *testing.T) {
	seed := omgcleanMergeMember("seed", 0, "", 10.0)
	rankA := omgcleanMergeMember("ra", 2, "scheduling_supply", 4.0)
	rankB := omgcleanMergeMember("rb", 3, "lock_priority", 3.0)
	nodes := []TraceCausalProjectionNode{seed, rankA, rankB}
	merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1, 2})
	if merged.FixDirection != "" {
		t.Fatalf("件2 冲突臂: conflicting rank directions must keep the empty slot, got %q", merged.FixDirection)
	}
}

// TestOmgcleanMergeSurvivorDirectionAlwaysWins — 空位 doctrine: a survivor
// with its OWN published direction keeps it verbatim, whatever the rank
// members say (the two sibling carriages at the R1 absorb / semantic-donor
// fill share the doctrine).
func TestOmgcleanMergeSurvivorDirectionAlwaysWins(t *testing.T) {
	seed := omgcleanMergeMember("seed", 0, "io_dependency", 10.0)
	rankSeat := omgcleanMergeMember("rank", 2, "frequency_thermal", 4.0)
	plain := omgcleanMergeMember("plain", 0, "", 2.0)
	nodes := []TraceCausalProjectionNode{seed, rankSeat, plain}
	merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1, 2})
	if merged.FixDirection != "io_dependency" {
		t.Fatalf("件2 恒胜臂: the survivor's own direction must win, got %q", merged.FixDirection)
	}
}

// TestOmgcleanMergeSupplierBareNoAdoption — 仅取 rank 供席: when the
// rank-supplying member itself publishes NO direction, nothing is adopted —
// a non-supplying member's direction never rides a foreign identity (the
// direction travels only with the board/window identity the row wears).
func TestOmgcleanMergeSupplierBareNoAdoption(t *testing.T) {
	seed := omgcleanMergeMember("seed", 0, "", 10.0)
	winner := omgcleanMergeMember("win", 1, "", 4.0) // supplies Rank, bare direction
	loser := omgcleanMergeMember("lose", 5, "frequency_thermal", 3.0)
	nodes := []TraceCausalProjectionNode{seed, winner, loser}
	merged := traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1, 2})
	if merged.Rank != 1 {
		t.Fatalf("fixture: the bare member must win the rank, got %d", merged.Rank)
	}
	if merged.FixDirection != "" {
		t.Fatalf("件2 供席臂: a bare rank supplier adopts nothing, got %q", merged.FixDirection)
	}
}

// TestOmgcleanMergeValueChannelsUntouched — 硬纪律1 twin. 双复核修复 件11
// (对抗 CR-5, spec 原令 strip-then-DeepEqual, 2026-07-21): the former 4-field
// subset compare could miss a stamp-driven mutation on ANY other node field —
// the guard is now the FULL-node reflect.DeepEqual after stripping exactly
// the adopted attribute slot (FixDirection, the one field the stamp may
// legitimately change).
func TestOmgcleanMergeValueChannelsUntouched(t *testing.T) {
	build := func(direction string) TraceCausalProjectionNode {
		seed := omgcleanMergeMember("seed", 0, "", 10.0)
		rankSeat := omgcleanMergeMember("rank", 2, direction, 4.0)
		plain := omgcleanMergeMember("plain", 0, "", 2.0)
		nodes := []TraceCausalProjectionNode{seed, rankSeat, plain}
		return traceCausalProjectionMergeSameKindMembers(nodes, 0, []int{0, 1, 2})
	}
	with := build("frequency_thermal")
	without := build("")
	with.FixDirection, without.FixDirection = "", ""
	if !reflect.DeepEqual(with, without) {
		t.Fatalf("件2/件11: strip-then-DeepEqual — the direction stamp must touch NO other node field:\nwith=%+v\nwithout=%+v", with, without)
	}
}
