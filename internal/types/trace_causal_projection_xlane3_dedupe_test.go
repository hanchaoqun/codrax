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
