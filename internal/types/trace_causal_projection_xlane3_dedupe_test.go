package types

import "testing"

// XLANE-3: rank-board identity is part of exact-node identity whenever the
// producer supplied it. Two boards can measure the same physical coordinates
// under different target/parameter accounts; compile dedupe must preserve both.
func TestTraceCausalProjectionDedupeNodesScopesRankBoards(t *testing.T) {
	base := TraceCausalProjectionNode{
		Role:                       TraceCausalRoleRootCauseContext,
		Subject:                    "logd.writer-9163",
		Predicate:                  "root_cause_tertiary",
		Object:                     "runnable_wait",
		SupportRefs:                []string{"donghu.ftrace:1276-27463"},
		QueryWindowStartTs:         13762.791708,
		QueryWindowEndTs:           13763.024898,
		RankBoardTarget:            "CompThread_0-2955",
		RankBoardParamsFingerprint: "655f120d",
		Rank:                       5,
		EffectiveImpactMS:          0.018,
	}

	assertCount := func(name string, want int, nodes ...TraceCausalProjectionNode) {
		t.Helper()
		if got := len(traceCausalProjectionDedupeNodes(nodes)); got != want {
			t.Fatalf("%s: dedupe count=%d, want %d", name, got, want)
		}
	}

	assertCount("same board duplicate", 1, base, base)

	// LEDGER-MERGE-1 (§40.33): the discarded exact-identity twin's ids ride the
	// survivor's MergedEvidenceIDs (a second query result republishing the
	// same seat), so the seat's evidence set no longer depends on result order.
	first, second := base, base
	first.EvidenceID, second.EvidenceID = "trace_query:p-frame_root_cause_bundle#root_cause_rank:1", "trace_query:p-root_cause_rank#root_cause_rank:1"
	second.MergedEvidenceIDs = []string{"trace_query:p-root_cause_rank#root_cause_rank:9"}
	for name, order := range map[string][]TraceCausalProjectionNode{"bundle first": {first, second}, "rank first": {second, first}} {
		got := traceCausalProjectionDedupeNodes(order)
		if len(got) != 1 || got[0].EvidenceID != order[0].EvidenceID || len(got[0].MergedEvidenceIDs) != 2 {
			t.Fatalf("%s: the survivor must carry the twin's ids: %+v", name, got)
		}
		for _, want := range []string{order[1].EvidenceID, "trace_query:p-root_cause_rank#root_cause_rank:9"} {
			found := false
			for _, id := range got[0].MergedEvidenceIDs {
				found = found || id == want
			}
			if !found {
				t.Fatalf("%s: twin id %q missing from the survivor: %+v", name, want, got[0].MergedEvidenceIDs)
			}
		}
	}

	differentTarget := base
	differentTarget.RankBoardTarget = "logd.writer-9163"
	assertCount("different target board", 2, base, differentTarget)

	differentParams := base
	differentParams.RankBoardParamsFingerprint = "different-params"
	assertCount("different params board", 2, base, differentParams)

	differentWindow := base
	differentWindow.QueryWindowStartTs += 1
	differentWindow.QueryWindowEndTs += 1
	assertCount("different window board", 2, base, differentWindow)

	legacyA, legacyB := base, base
	legacyA.RankBoardTarget, legacyA.RankBoardParamsFingerprint = "", ""
	legacyB.RankBoardTarget, legacyB.RankBoardParamsFingerprint = "", ""
	legacyB.QueryWindowStartTs += 1
	legacyB.QueryWindowEndTs += 1
	assertCount("legacy identity-less byte compatibility", 1, legacyA, legacyB)

	zeroMirror := base
	zeroMirror.RankBoardTarget, zeroMirror.RankBoardParamsFingerprint = "", ""
	zeroMirror.Rank, zeroMirror.EffectiveImpactMS = 0, 0
	assertCount("identity-less zero mirror does not split", 1, zeroMirror, base)
	assertCount("identity-less zero mirror order independent", 1, base, zeroMirror)

	unnamedAccount := zeroMirror
	unnamedAccount.Rank = 5
	unnamedAccount.EffectiveImpactMS = 0.018
	assertCount("identity-less value account remains unnamed board", 2, unnamedAccount, base)
}
