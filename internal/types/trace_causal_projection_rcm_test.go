package types

// trace_causal_projection_rcm_test.go — RCM projection-compile pins (ledger
// §24.7.1/§24.10/§24.12 dimension A, real_trace_campaign_20260705.md,
// 2026-07-08): the engine family-merge accounting re-materializes on the
// ISOLATED FamilyMember* node lane, and the display-side Merged* lane
// (R2/R3/PTS folds, whose lead selector collapses ×N rows to their member
// MAX) never receives it.
//
// Mutation self-check M-d (verified red during development): rerouting the
// member_count parse onto node.MergedCount / member_max_ms onto
// node.MergedMaxMS makes TestTraceCausalProjectionFamilyMemberLaneIsolatedFromMergedLane
// red — the isolation is a structural pin, not a comment.

import (
	"reflect"
	"testing"
)

func rcmFamilyRankRecord(notes []string) ObservationRecord {
	return ObservationRecord{
		ID:        "trace_query:w1#root_cause_rank:1",
		Subject:   "RxComputationT-16816",
		Predicate: "root_cause_primary",
		ClaimKey:  "root_cause_primary",
		Object:    "block_io_by_inode",
		Value:     "1.598",
		Unit:      "ms",
		RichNotes: notes,
	}
}

func TestTraceCausalProjectionNodeParsesFamilyMemberLane(t *testing.T) {
	record := rcmFamilyRankRecord([]string{
		"rank=3",
		"type=block_io_by_inode",
		"member_count=2",
		"member_max_ms=1.136",
		"member_min_ms=0.462",
		"member_fold_caliber=sum_disjoint",
		"member_roster=inode=286395 dev=254:2 1.136ms | inode=300123 dev=254:2 0.462ms",
		"dev=254:2",
	})
	node := traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, record)
	if node.FamilyMemberCount != 2 {
		t.Fatalf("member_count must promote to the typed family lane: %+v", node)
	}
	if node.FamilyMemberMaxMS != 1.136 || node.FamilyMemberMinMS != 0.462 {
		t.Fatalf("member range promotion: %+v", node)
	}
	if node.FamilyFoldCaliber != "sum_disjoint" {
		t.Fatalf("caliber promotion: %+v", node)
	}
	want := []string{"inode=286395 dev=254:2 1.136ms", "inode=300123 dev=254:2 0.462ms"}
	if !reflect.DeepEqual(node.FamilyMemberRoster, want) {
		t.Fatalf("roster splits on the pipe separator (member keys may contain commas): %+v", node.FamilyMemberRoster)
	}
	if node.Dev != "254:2" || node.Inode != "" {
		t.Fatalf("typed inode/dev identity (§24.9-B F3): %+v", node)
	}
}

func TestTraceCausalProjectionNodeParsesInodeDevOnUnmergedRow(t *testing.T) {
	record := rcmFamilyRankRecord([]string{
		"rank=3",
		"type=block_io_by_inode",
		"inode=286395",
		"dev=254:2",
	})
	node := traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, record)
	if node.Inode != "286395" || node.Dev != "254:2" {
		t.Fatalf("unmerged rows carry their typed distinguishing keys too: %+v", node)
	}
	if node.FamilyMemberCount != 0 || node.FamilyFoldCaliber != "" || len(node.FamilyMemberRoster) != 0 {
		t.Fatalf("no family accounting without member_count>1: %+v", node)
	}
}

func TestTraceCausalProjectionFamilyMemberSumDisclosure(t *testing.T) {
	record := rcmFamilyRankRecord([]string{
		"member_count=6",
		"member_max_ms=0.568",
		"member_min_ms=0.443",
		"member_sum_ms=2.946",
		"member_fold_caliber=interval_union",
		"member_roster=a 0.568ms | b 0.500ms",
	})
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record)
	if node.FamilyMemberSumMS != 2.946 || node.FamilyFoldCaliber != "interval_union" {
		t.Fatalf("union < Σ disclosure must survive the compile: %+v", node)
	}
}

func TestTraceCausalProjectionFamilyMemberLaneIsolatedFromMergedLane(t *testing.T) {
	// §24.12 dim-A mandate ① (mutation M-d bites here): the family carriers
	// must NEVER populate the display Merged* lane — the lead selector folds
	// MergedCount>1 rows to MergedMaxMS (×N SUMs never compete), which would
	// collapse the same-thread family total back to its largest member.
	record := rcmFamilyRankRecord([]string{
		"member_count=14",
		"member_max_ms=2.424",
		"member_min_ms=0.040",
		"member_fold_caliber=sum_disjoint",
		"member_roster=VerifyClass a 2.424ms | VerifyClass b 0.040ms",
	})
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleSemanticSpan, record)
	if node.FamilyMemberCount != 14 {
		t.Fatalf("family lane must populate: %+v", node)
	}
	if node.MergedCount != 0 || node.MergedMaxMS != 0 || node.MergedMinMS != 0 || len(node.MergedSubjects) != 0 {
		t.Fatalf("ISOLATION BROKEN: member_* notes leaked into the display Merged* fold lane: %+v", node)
	}
	if node.OnChainOverflowFold {
		t.Fatalf("a family row is not a wire-cap overflow fold: %+v", node)
	}
}

func TestTraceCausalProjectionFoldedLaneDoesNotWriteFamilyLane(t *testing.T) {
	// The reverse isolation: the PTS folded_* wire-cap family keeps writing
	// ONLY the Merged* lane — never the FamilyMember* carriers.
	record := rcmFamilyRankRecord([]string{
		"folded_rows=5",
		"folded_min_ms=0.100",
		"folded_max_ms=1.500",
		"folded_subjects=t1,t2",
	})
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record)
	if node.MergedCount != 5 || node.MergedMaxMS != 1.5 {
		t.Fatalf("folded_* lane unchanged: %+v", node)
	}
	if node.FamilyMemberCount != 0 || node.FamilyFoldCaliber != "" || len(node.FamilyMemberRoster) != 0 {
		t.Fatalf("ISOLATION BROKEN: folded_* notes leaked into the FamilyMember* lane: %+v", node)
	}
}

// TestTraceCausalProjectionR2AggregateClearsFamilyLane (RCM-2 复核 F-1,
// 2026-07-08): the ≥3 same-(subject,object) R2 fold's group-first seed can be
// an engine family contender — inheriting its FamilyMember*/caliber/roster/
// BackgroundRank/Inode/Dev wholesale minted a chimera row carrying BOTH ×N
// lanes (one row: 行1 「×2 合计」 + subordinate 「3次(…)」 R2 tag) and let one
// member's inode/board seat impersonate the merge. The aggregate clears the
// family lane unconditionally (DuplicatePublications/SupplyFold precedent).
// Mutation M-5 (verified red, then reverted): dropping the clear block in
// traceCausalProjectionAggregateSameKind reds this pin.
func TestTraceCausalProjectionR2AggregateClearsFamilyLane(t *testing.T) {
	family := func(id string, impact float64) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Role: TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: "RxComputationT-16816", Object: "block_io_by_inode",
			ImpactMS: impact, CumulativeImpactMS: impact,
			FamilyMemberCount: 2, FamilyMemberMaxMS: 1.136, FamilyMemberMinMS: 0.462,
			FamilyMemberSumMS: 1.598, FamilyFoldCaliber: "sum_disjoint",
			FamilyMemberRoster: []string{"inode=286395 dev=254:2 1.136ms"},
			BackgroundRank:     2, Inode: "286395", Dev: "254:2",
		}
	}
	plain := TraceCausalProjectionNode{
		Role: TraceCausalRoleRootCauseContext, EvidenceID: "w3",
		Subject: "RxComputationT-16816", Object: "block_io_by_inode",
		ImpactMS: 3.0, CumulativeImpactMS: 3.0,
	}
	out := traceCausalProjectionAggregateSameKind([]TraceCausalProjectionNode{
		family("w1", 1.598), family("w2", 2.0), plain,
	})
	if len(out) != 1 || out[0].MergedCount != 3 {
		t.Fatalf("the three same-kind rows must fold into one ×3 row: %+v", out)
	}
	merged := out[0]
	if merged.FamilyMemberCount != 0 || merged.FamilyMemberMaxMS != 0 ||
		merged.FamilyMemberMinMS != 0 || merged.FamilyMemberSumMS != 0 ||
		merged.FamilyFoldCaliber != "" || len(merged.FamilyMemberRoster) != 0 {
		t.Fatalf("CHIMERA: the FamilyMember* lane must be cleared on the R2 fold: %+v", merged)
	}
	if merged.BackgroundRank != 0 || merged.Inode != "" || merged.Dev != "" {
		t.Fatalf("the group-first seat/keys must not impersonate the merge: %+v", merged)
	}
}
