package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// preCompleteDowngradeConverges must force-complete only after the hard
// threshold of CONSECUTIVE no-progress re-attempts on the same lane+blocker,
// and must reset when the typed blocker genuinely changes. Reads only typed
// closure state (no downgrade prose).
func TestPreCompleteDowngradeConverges_ThresholdAndReset(t *testing.T) {
	SetDowngradeConvergenceHardThreshold(4)
	defer SetDowngradeConvergenceHardThreshold(0) // restore default

	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	closure := mut.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{Origin: "phase1_unread", File: "a.go"})

	lane := types.DowngradeLaneContractChain
	for i := 1; i <= 3; i++ {
		if preCompleteDowngradeConverges(bus, lane) {
			t.Fatalf("attempt %d must NOT converge yet (threshold 4)", i)
		}
	}
	if !preCompleteDowngradeConverges(bus, lane) {
		t.Fatal("4th identical no-progress attempt must converge (force-complete)")
	}
	if cv := closure.CompletionCaveats(); len(cv) != 1 || cv[0].Lane != lane {
		t.Fatalf("convergence must record a typed completion caveat, got %+v", cv)
	}
}

func TestPreCompleteDowngradeConverges_ChangedBlockerResets(t *testing.T) {
	SetDowngradeConvergenceHardThreshold(3)
	defer SetDowngradeConvergenceHardThreshold(0)

	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	closure := mut.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{Origin: "phase1_unread", File: "a.go"})

	lane := types.DowngradeLaneContractChain
	preCompleteDowngradeConverges(bus, lane) // 1
	preCompleteDowngradeConverges(bus, lane) // 2 (same blocker)
	// A genuinely changed blocker (a new pending read this round) changes the
	// typed key -> consecutive count resets -> must NOT converge next attempt.
	closure.AddPendingRead(types.PendingRead{Origin: "primary_anchor", File: "c.go"})
	if preCompleteDowngradeConverges(bus, lane) {
		t.Fatal("a changed blocker must reset convergence, not force-complete")
	}
}

func TestPreCompleteDowngradeConverges_TypedBlockerIgnoresSiblingClosureChurn(t *testing.T) {
	mut := types.NewMutableState("typed-blocker")
	bus := &types.BusContext{Mutable: mut}
	closure := mut.EvidenceClosure()
	lane := types.DowngradeLaneFlowParticipantCoverage
	key := types.ComputeDowngradeTypedIdentifierSetKey(string(lane), []string{"Mutable", "BusContext"})

	if preCompleteDowngradeConvergesWithTypedBlockerKey(bus, lane, key) {
		t.Fatal("the first exact participant blocker must request one focused pass")
	}
	closure.AddPendingRead(types.PendingRead{Origin: "unrelated_navigation", File: "other.go"})
	closure.AddRepair(types.RepairDirective{
		Kind: types.RepairExpandSearch, Subject: "unrelated", DowngradeLane: types.DowngradeLaneContractChain,
	})
	if preCompleteDowngradeConvergesWithTypedBlockerKey(bus, lane, key) {
		t.Fatal("participant coverage needs a second bounded repair turn before convergence")
	}
	if !preCompleteDowngradeConvergesWithTypedBlockerKey(bus, lane, key) {
		t.Fatal("unchanged gate-owned participant set must converge on the third close despite sibling closure churn")
	}
}

func TestPreCompleteDowngradeConverges_TypedBlockerResetsWhenOwnSetChanges(t *testing.T) {
	mut := types.NewMutableState("typed-blocker-reset")
	bus := &types.BusContext{Mutable: mut}
	lane := types.DowngradeLaneFlowParticipantCoverage
	two := types.ComputeDowngradeTypedIdentifierSetKey(string(lane), []string{"Mutable", "BusContext"})
	one := types.ComputeDowngradeTypedIdentifierSetKey(string(lane), []string{"BusContext"})

	if preCompleteDowngradeConvergesWithTypedBlockerKey(bus, lane, two) {
		t.Fatal("the first two-participant blocker must not converge")
	}
	if preCompleteDowngradeConvergesWithTypedBlockerKey(bus, lane, one) {
		t.Fatal("a genuinely smaller participant blocker must reset the focused-pass allowance")
	}
	if preCompleteDowngradeConvergesWithTypedBlockerKey(bus, lane, one) {
		t.Fatal("the repeated smaller participant blocker still has one bounded repair turn")
	}
	if !preCompleteDowngradeConvergesWithTypedBlockerKey(bus, lane, one) {
		t.Fatal("the unchanged smaller blocker must converge on its third attempt")
	}
}

func TestPreCompleteDowngradeConverges_SourceInventoryCompletionDebtForceCompletesAtThreshold(t *testing.T) {
	SetDowngradeConvergenceHardThreshold(3)
	defer SetDowngradeConvergenceHardThreshold(0)

	mut := types.NewMutableState("inventory")
	bus := &types.BusContext{Mutable: mut}
	closure := mut.EvidenceClosure()
	closure.AddRepair(types.RepairDirective{
		Kind: types.RepairStructuredHandoff,
		SourceInventoryCompletionAuthority: types.SourceInventoryCompletionAuthority{
			Active:     true,
			Blocking:   true,
			ReasonCode: types.SourceInventoryCompletionReasonFollowupDebt,
			FollowupDebt: types.SourceInventoryFollowupDebt{
				Active:     true,
				ReasonCode: types.SourceInventoryFollowupDebtPagination,
				Query: types.SourceInventoryLensQuery{
					Path:              ".",
					Scopes:            []string{"."},
					Roles:             []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
					IncludeCounts:     true,
					IncludeAttributes: false,
					Cursor:            "24",
				},
			},
		},
	})

	for i := 1; i <= 2; i++ {
		if preCompleteDowngradeConverges(bus, types.DowngradeLaneSourceInventoryCompletion) {
			t.Fatalf("source-inventory completion debt must wait until threshold, converged on attempt %d", i)
		}
	}
	if !preCompleteDowngradeConverges(bus, types.DowngradeLaneSourceInventoryCompletion) {
		t.Fatal("source-inventory completion debt must force-complete as caveat on the third identical blocker")
	}
	if cv := closure.CompletionCaveats(); len(cv) != 1 || cv[0].Lane != types.DowngradeLaneSourceInventoryCompletion {
		t.Fatalf("source-inventory completion debt must record typed caveat, got %+v", cv)
	}
}

func TestPreCompleteDowngradeConverges_NilSafe(t *testing.T) {
	if preCompleteDowngradeConverges(nil, types.DowngradeLaneContractChain) {
		t.Fatal("nil ctx must not converge")
	}
	if preCompleteDowngradeConverges(&types.BusContext{}, types.DowngradeLaneContractChain) {
		t.Fatal("nil Mutable must not converge")
	}
}
