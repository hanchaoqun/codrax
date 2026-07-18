package types

// trace_causal_projection_spantop_test.go — SPANTOP-1 件1 compile pins
// (§29.131, 2026-07-18): the member_wall_ms strict all-or-nothing parse and
// the ×N aggregate clear of the per-member carriers.

import "testing"

func TestSpanTopParseMemberWallMSStrict(t *testing.T) {
	// Complete positive list → parsed floats in member order.
	got := traceCausalProjectionParseMemberWallMS("1.781|0.607", 2)
	if len(got) != 2 || got[0] != 1.781 || got[1] != 0.607 {
		t.Fatalf("complete list must parse in order, got %v", got)
	}
	for name, tc := range map[string]struct {
		raw   string
		count int
	}{
		"count mismatch (short)": {"1.781", 2},
		"count mismatch (long)":  {"1.781|0.607|0.100", 2},
		"non-numeric entry":      {"1.781|x", 2},
		"non-positive entry":     {"1.781|0", 2},
		"negative entry":         {"1.781|-0.5", 2},
		"single member":          {"1.781", 1},
		"empty":                  {"", 2},
	} {
		if got := traceCausalProjectionParseMemberWallMS(tc.raw, tc.count); got != nil {
			t.Fatalf("%s: strict parse must drop the WHOLE list, got %v", name, got)
		}
	}
}

func TestSpanTopNodeParsesWallListWithFamily(t *testing.T) {
	record := rcmFamilyRankRecord([]string{
		"rank=5",
		"type=class_verification",
		"semantic_class=class_verification",
		"member_count=3",
		"member_fold_caliber=sum_disjoint",
		"member_roster=a 3.000ms | b 2.000ms | c 1.000ms",
		"member_line_ranges=100..120|130..150|160..180",
		"member_wall_ms=3.000|2.000|1.000",
	})
	node := traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, record)
	if len(node.FamilyMemberWallMS) != 3 || node.FamilyMemberWallMS[0] != 3.000 ||
		node.FamilyMemberWallMS[2] != 1.000 {
		t.Fatalf("the compile must parse the wall list beside the family lane, got %v",
			node.FamilyMemberWallMS)
	}
	if len(node.FamilyMemberLineRanges) != 3 {
		t.Fatalf("line ranges must co-parse, got %v", node.FamilyMemberLineRanges)
	}
	// Strict arm at record grain: a count-mismatched note drops the WHOLE
	// list while the rest of the family lane stays.
	short := rcmFamilyRankRecord([]string{
		"rank=5",
		"type=class_verification",
		"member_count=3",
		"member_fold_caliber=sum_disjoint",
		"member_roster=a 3.000ms | b 2.000ms | c 1.000ms",
		"member_wall_ms=3.000|2.000",
	})
	node2 := traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, short)
	if node2.FamilyMemberWallMS != nil {
		t.Fatalf("a mismatched wall list must drop whole, got %v", node2.FamilyMemberWallMS)
	}
	if node2.FamilyMemberCount != 3 || len(node2.FamilyMemberRoster) != 3 {
		t.Fatalf("the family lane must survive the dropped wall list: %+v", node2)
	}
	// No family (member_count absent) → never parsed at all.
	bare := rcmFamilyRankRecord([]string{
		"rank=5",
		"type=class_verification",
		"member_wall_ms=3.000|2.000|1.000",
	})
	node3 := traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, bare)
	if node3.FamilyMemberWallMS != nil {
		t.Fatalf("no family lane → no wall list, got %v", node3.FamilyMemberWallMS)
	}
}

// The ×N aggregate Σ row sheds the per-member carriers with the rest of the
// family grammar (SPANTOP-1 hygiene beside the RCM-2 F-1 chimera clear).
func TestSpanTopAggregateClearsPerMemberCarriers(t *testing.T) {
	seed := levelmergeTestConstituentNode(10, 100)
	seed.GatedShareConstituentSeat = false
	seed.GatedShareClaimedMS, seed.GatedShareFullMS = 0, 0
	seed.GatedShareClaimSeats = nil
	seed.FamilyMemberCount = 2
	seed.FamilyFoldCaliber = "sum_disjoint"
	seed.FamilyMemberRoster = []string{"a 6.000ms", "b 4.000ms"}
	seed.FamilyMemberWallMS = []float64{6.000, 4.000}
	seed.FamilyMemberLineRanges = [][2]int{{100, 105}, {106, 110}}
	twin := levelmergeTestConstituentNode(8, 200)
	twin.GatedShareConstituentSeat = false
	twin.GatedShareClaimedMS, twin.GatedShareFullMS = 0, 0
	twin.GatedShareClaimSeats = nil
	twin.EvidenceID = "spantop-agg-b"
	merged := TraceCausalProjectionMergeOccurrenceRows([]TraceCausalProjectionNode{seed, twin})
	if merged.FamilyMemberWallMS != nil || merged.FamilyMemberLineRanges != nil {
		t.Fatalf("the ×N Σ row must shed the per-member carriers: wall=%v ranges=%v",
			merged.FamilyMemberWallMS, merged.FamilyMemberLineRanges)
	}
	if merged.FamilyMemberCount != 0 || merged.FamilyMemberRoster != nil {
		t.Fatalf("the family grammar clear must hold beside them: %+v", merged)
	}
}
