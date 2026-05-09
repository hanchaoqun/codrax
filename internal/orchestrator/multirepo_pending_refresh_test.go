package orchestrator

import (
	"os"
	"strings"
	"testing"
)

// TestMultiRepoPendingSubReposRefresh_AlwaysRuns is a structural
// regression pin against the asymmetric-refresh bug that caused
// mr_focus_single to fail on 2026-05-09.
//
// Symptoms: with cap=3 and no focus pin, routing decided all 3
// sub-repos active → decision.Inactive=empty → the original
// `if len(decision.Inactive) > 0` guard SKIPPED the refresh,
// leaving PendingSubRepos in its snapshot-time state ("ALL
// sub-repos" because the LRU was empty at snapshot time). The
// L1 active-set gate then computed
// `active = topology - PendingSubRepos = empty`, refusing every
// tool with "workspace has no active sub-repos".
//
// The fix removes the conditional guard so the refresh ALWAYS
// rewrites PendingSubRepos from the routing decision. This test
// pins the contract by source-substring inspection — the more
// brittle path is easy to reintroduce, so we lock the source
// shape rather than try to simulate the full Run() entry.
func TestMultiRepoPendingSubReposRefresh_AlwaysRuns(t *testing.T) {
	src, err := os.ReadFile("orchestrator.go")
	if err != nil {
		t.Fatalf("read orchestrator.go: %v", err)
	}
	source := string(src)

	// The pre-fix shape was:
	//   if len(decision.Inactive) > 0 {
	//       pendingNames := ... decision.Inactive ...
	//       o.busCtx.PendingSubRepos = pendingNames
	//   }
	// The fix removes the guard. We verify the source no longer
	// contains the guarded-conditional pattern around the refresh.
	if strings.Contains(source, "if len(decision.Inactive) > 0 {") {
		// Could be coincidence — must be checking that variable
		// specifically, not generic Inactive checks in other
		// scheduler paths. Validate by also requiring the
		// PendingSubRepos identifier appears within ~5 lines after
		// the guard.
		idx := strings.Index(source, "if len(decision.Inactive) > 0 {")
		window := source[idx:]
		if len(window) > 500 {
			window = window[:500]
		}
		if strings.Contains(window, "PendingSubRepos") {
			t.Errorf("orchestrator.go contains the pre-fix asymmetric-refresh guard\n" +
				"    `if len(decision.Inactive) > 0 { ... PendingSubRepos = ... }`\n" +
				"This guard caused mr_focus_single failure on 2026-05-09 — when the\n" +
				"routing decision left zero sub-repos inactive (cap≥total + no focus\n" +
				"pin), the refresh was skipped and the snapshot-time stale\n" +
				"PendingSubRepos (ALL sub-repos) carried into the L1 active-set gate,\n" +
				"computing active=empty and refusing every tool. Remove the guard so\n" +
				"the refresh always runs.")
		}
	}

	// Positive assertion: the unconditional refresh body is present.
	// The fix path always rewrites PendingSubRepos from the
	// routing decision's Inactive list.
	if !strings.Contains(source, "o.busCtx.PendingSubRepos = pendingNames") {
		t.Errorf("orchestrator.go missing the unconditional PendingSubRepos refresh\n" +
			"    `o.busCtx.PendingSubRepos = pendingNames`\n" +
			"The routing fold MUST rewrite PendingSubRepos from the routing\n" +
			"decision so the L1 active-set gate sees the correct active = topology\n" +
			"- PendingSubRepos invariant. See orchestrator.go::Run() routing fold.")
	}
}
