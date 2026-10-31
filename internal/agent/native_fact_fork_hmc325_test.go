package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc325HasQuery(ctx *types.AgentContext, result types.ToolResult) bool {
	wanted := result.Observations[0].SourceRef
	for _, candidate := range types.ObservationLedgerInputFromAgentContext(ctx, 0).ToolResults {
		for _, row := range candidate.Observations {
			if row.SourceRef.Path == wanted.Path && row.SourceRef.QueryScopeID == wanted.QueryScopeID && row.SourceRef.PayloadRef == wanted.PayloadRef {
				return true
			}
		}
	}
	return false
}

func hmc325Fork(ctx *types.AgentContext) *types.AgentContext {
	child := ctx.ShallowClone()
	child.Mutable = ctx.Mutable.ForkForExploreDispatch()
	child.Stage, child.AgentName = types.StageExplore, types.AgentExplorer
	return child
}

func TestHMC325NativeFactsForkVisibilityAndBothMerges(t *testing.T) {
	for _, dataOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("data_only_%t", dataOnly), func(t *testing.T) {
			parent, path := hmc323BusinessBudgetContext(t)
			full := hmc325PublicBusinessQuery(t, parent, path, 5.2, 5.312)
			parent.Mutable.ResetDispatchToolResults()
			child := hmc325Fork(parent)
			sibling := hmc325Fork(parent)
			clip := hmc325PublicBusinessQuery(t, child, path, 5.201, 5.2069)
			child.Mutable.ResetDispatchToolResults()
			if !hmc325HasQuery(child, full) || !hmc325HasQuery(child, clip) || !hmc325HasQuery(parent, full) {
				t.Fatal("fork lost inherited or locally published native facts")
			}
			if hmc325HasQuery(parent, clip) || hmc325HasQuery(sibling, clip) {
				t.Fatal("shared pool exposed an unmerged sibling publication")
			}
			// There is no Turn A positional suffix or dispatch result left. The
			// explicit merge must still union the independently published handles.
			if child.Mutable.TurnAArtifacts() != nil || len(child.Mutable.DispatchToolResults()) != 0 {
				t.Fatal("fixture unexpectedly supplies an alternate history carrier")
			}
			if dataOnly {
				if added := parent.Mutable.MergeExploreForkPublishedTools(child.Mutable); len(added) != 0 {
					t.Fatal("data-only fixture should not claim a raw ToolResult suffix")
				}
			} else {
				parent.Mutable.MergeExploreFork(child.Mutable)
			}
			if !hmc325HasQuery(parent, full) || !hmc325HasQuery(parent, clip) || hmc325HasQuery(sibling, clip) {
				t.Fatal("explicit merge lost a window or retroactively mutated a sibling's visibility")
			}
			before := parent.Mutable.NativeFactRetentionStatus()
			parent.Mutable.MergeExploreFork(child.Mutable)
			parent.Mutable.MergeExploreForkPublishedTools(child.Mutable)
			if after := parent.Mutable.NativeFactRetentionStatus(); before != after {
				t.Fatalf("repeated merges duplicated capacity: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestHMC325NativeFactsResetOwnerAndBranchIsolation(t *testing.T) {
	t.Run("child_reset_keeps_parent_and_sibling", func(t *testing.T) {
		parent, path := hmc323BusinessBudgetContext(t)
		full := hmc325PublicBusinessQuery(t, parent, path, 5.2, 5.312)
		parent.Mutable.ResetDispatchToolResults()
		child, sibling := hmc325Fork(parent), hmc325Fork(parent)
		child.Mutable.ResetTurnAArtifacts()
		clip := hmc325PublicBusinessQuery(t, child, path, 5.201, 5.2069)
		child.Mutable.ResetDispatchToolResults()
		parent.Mutable.MergeExploreFork(child.Mutable)
		if !hmc325HasQuery(parent, full) || !hmc325HasQuery(sibling, full) || hmc325HasQuery(parent, clip) {
			t.Fatal("branch-local reset revoked the parent or leaked the new branch epoch")
		}
	})
	t.Run("parent_reset_revokes_old_branch", func(t *testing.T) {
		parent, path := hmc323BusinessBudgetContext(t)
		full := hmc325PublicBusinessQuery(t, parent, path, 5.2, 5.312)
		parent.Mutable.ResetDispatchToolResults()
		child := hmc325Fork(parent)
		parent.Mutable.ResetTurnAArtifacts()
		hmc325PublicBusinessQuery(t, child, path, 5.201, 5.2069)
		child.Mutable.ResetDispatchToolResults()
		parent.Mutable.MergeExploreFork(child.Mutable)
		parent.Mutable.MergeExploreForkPublishedTools(child.Mutable)
		if hmc325HasQuery(parent, full) || parent.Mutable.NativeFactRetentionStatus().RetainedQueries != 0 || child.Mutable.NativeFactRetentionStatus().RetainedQueries != 0 {
			t.Fatal("retired investigation facts or late old-branch publication survived reset")
		}
		current := hmc325PublicBusinessQuery(t, parent, path, 5.2, 5.312)
		parent.Mutable.ResetDispatchToolResults()
		if !hmc325HasQuery(parent, current) {
			t.Fatal("retiring old branches prevented legitimate current-epoch acquisition")
		}
	})
	t.Run("zero_value_reset_establishes_new_epoch", func(t *testing.T) {
		ctx, path := hmc323BusinessBudgetContext(t)
		ctx.Mutable = &types.MutableState{}
		if ctx.Mutable.NativeFactRetentionStatus().RetainedQueries != 0 {
			t.Fatal("zero-value state manufactured retained facts")
		}
		ctx.Mutable.ResetTurnAArtifacts()
		full := hmc325PublicBusinessQuery(t, ctx, path, 5.2, 5.312)
		ctx.Mutable.ResetDispatchToolResults()
		if !hmc325HasQuery(ctx, full) {
			t.Fatal("zero-value reset failed to establish a usable source epoch")
		}
	})
}

func TestHMC325NativeFactsExactDuplicateAndSharedCapacity(t *testing.T) {
	parent, path := hmc323BusinessBudgetContext(t)
	first := hmc325PublicBusinessQuery(t, parent, path, 5.201, 5.2069)
	parent.Mutable.ResetDispatchToolResults()
	before := parent.Mutable.NativeFactRetentionStatus()
	hmc325PublicBusinessQuery(t, parent, path, 5.201, 5.2069)
	parent.Mutable.ResetDispatchToolResults()
	if after := parent.Mutable.NativeFactRetentionStatus(); before != after || after.RetainedQueries == 0 {
		t.Fatalf("same native query failed exact population deduplication: before=%+v after=%+v", before, after)
	}
	left, right := hmc325Fork(parent), hmc325Fork(parent)
	var omitted types.ToolResult
	// Distinct actual query endpoints, not fabricated summary pressure. Both
	// forks draw from the same fixed capacity while keeping separate handles.
	for i := 1; i <= before.QueryLimit; i++ {
		ctx := left
		if i%2 == 0 {
			ctx = right
		}
		omitted = hmc325PublicBusinessQuery(t, ctx, path, 5.201, 5.2069+float64(i)*0.000001)
		ctx.Mutable.ResetDispatchToolResults()
	}
	parent.Mutable.MergeExploreFork(left.Mutable)
	parent.Mutable.MergeExploreForkPublishedTools(right.Mutable)
	status := parent.Mutable.NativeFactRetentionStatus()
	if status.RetainedQueries != status.QueryLimit || !status.CapacityOmitted || status.EncodedBytes > status.EncodedByteLimit ||
		status.LastSource != omitted.Observations[0].SourceRef.Path || status.LastQuery != omitted.Observations[0].SourceRef.QueryScopeID {
		t.Fatalf("shared budget did not publish the bounded omission receipt: %+v", status)
	}
	if !hmc325HasQuery(parent, first) || hmc325HasQuery(parent, omitted) {
		t.Fatal("capacity eviction replaced a valid early account or misrepresented the omitted query as retained")
	}
	boundary := renderAnswerDocNativeFactRetentionBoundary(parent)
	if boundary == "" || !strings.Contains(boundary, status.LastQuery) {
		t.Fatal("capacity omission is invisible to final answer context")
	}
	t.Logf("shared retention capacity=%+v", status)
}
