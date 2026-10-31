package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc326Merge(parent, child *types.AgentContext, dataOnly bool) {
	if dataOnly {
		parent.Mutable.MergeExploreForkPublishedTools(child.Mutable)
	} else {
		parent.Mutable.MergeExploreFork(child.Mutable)
	}
}

// This uses the existing public producer/setter/fork APIs. The HMC325 full
// suite supplies the actual pre-fix replacement failures; this new case adds
// coverage for both raw-tool and retained-handle resurrection paths.
func TestHMC326ExplicitReplacementRejectsOldForks(t *testing.T) {
	for _, dataOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("data_only_%t", dataOnly), func(t *testing.T) {
			parent, path := hmc323BusinessBudgetContext(t)
			full := hmc325PublicBusinessQuery(t, parent, path, 5.2, 5.312)
			parent.Mutable.ResetDispatchToolResults()
			child := hmc325Fork(parent)
			clipped := hmc325PublicBusinessQuery(t, child, path, 5.201, 5.2069)
			child.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
				ToolResults: []types.ToolResult{full, clipped}, InvestigationNotes: []string{"old fork note"},
			})
			child.Mutable.SetInvestigationComplete("old fork completion")
			parent.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{UserQuestion: "replacement"})
			parent.Mutable.ResetDispatchToolResults()
			before := parent.Mutable.TurnAArtifacts()
			beforeStatus := parent.Mutable.NativeFactRetentionStatus()
			hmc326Merge(parent, child, dataOnly)
			if !reflect.DeepEqual(before, parent.Mutable.TurnAArtifacts()) || beforeStatus != parent.Mutable.NativeFactRetentionStatus() ||
				len(hmc325BoundaryInput(parent).ToolResults) != 0 || parent.Mutable.IsInvestigationComplete() {
				t.Fatal("old fork revived tools, native handles, notes, or completion after explicit replacement")
			}
			// A new branch and a real retry remain legal within the same source
			// epoch; replacement is not a bad-source terminal or a run reset.
			fresh := hmc325Fork(parent)
			retry := hmc325PublicBusinessQuery(t, fresh, path, 5.2, 5.312)
			fresh.Mutable.ResetDispatchToolResults()
			hmc326Merge(parent, fresh, dataOnly)
			if !hmc325HasQuery(parent, retry) || hmc325HasQuery(parent, clipped) {
				t.Fatal("replacement blocked a real new retry or restored the old fork's other window")
			}
		})
	}
}

func TestHMC326TypedPublicationKeepsNativeFactsAndLegalRetry(t *testing.T) {
	for _, kind := range []types.TurnAHandoffPublicationKind{
		types.TurnAHandoffHistoryCompacted, types.TurnAHandoffSourceEvidenceAugmented, types.TurnAHandoffAdvisoryUpdated,
	} {
		for _, dataOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("kind_%d/data_only_%t", kind, dataOnly), func(t *testing.T) {
				parent, path := hmc323BusinessBudgetContext(t)
				full := hmc325PublicBusinessQuery(t, parent, path, 5.2, 5.312)
				parent.Mutable.ResetDispatchToolResults()
				child := hmc325Fork(parent)
				clipped := hmc325PublicBusinessQuery(t, child, path, 5.201, 5.2069)
				child.Mutable.ResetDispatchToolResults()
				if !parent.Mutable.PublishTurnAArtifacts(types.TurnAArtifacts{InvestigationNotes: []string{"current advisory"}}, kind) ||
					!child.Mutable.PublishTurnAArtifacts(types.TurnAArtifacts{}, types.TurnAHandoffHistoryCompacted) {
					t.Fatal("valid typed publication was rejected")
				}
				// A memo or a failed later attempt cannot manufacture a query, but
				// neither removes an independently admitted real prior publication.
				memo, failed := clipped, clipped
				memo.ReusedFromRunMemo = true
				failed.Success = false
				child.Mutable.AppendDispatchToolResult(memo)
				child.Mutable.AppendDispatchToolResult(failed)
				if !hmc325HasQuery(child, full) || !hmc325HasQuery(child, clipped) {
					t.Fatal("memo/failed attempt masked a retained real query")
				}
				child.Mutable.ResetDispatchToolResults()
				hmc326Merge(parent, child, dataOnly)
				if !hmc325HasQuery(parent, full) || !hmc325HasQuery(parent, clipped) {
					t.Fatal("normal publication retired a valid fork or mixed its independent query windows")
				}
				before := parent.Mutable.NativeFactRetentionStatus()
				hmc325PublicBusinessQuery(t, parent, path, 5.201, 5.2069)
				if got := parent.Mutable.NativeFactRetentionStatus(); got.RetainedQueries != before.RetainedQueries || got.EncodedBytes != before.EncodedBytes {
					t.Fatal("legal same-query retry duplicated retained capacity")
				}
			})
		}
	}
}

func TestHMC326ChildReplacementKeepsParentAndSibling(t *testing.T) {
	for _, dataOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("data_only_%t", dataOnly), func(t *testing.T) {
			parent, path := hmc323BusinessBudgetContext(t)
			full := hmc325PublicBusinessQuery(t, parent, path, 5.2, 5.312)
			parent.Mutable.ResetDispatchToolResults()
			child, sibling := hmc325Fork(parent), hmc325Fork(parent)
			clipped := hmc325PublicBusinessQuery(t, child, path, 5.201, 5.2069)
			child.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{clipped}})
			child.Mutable.ResetDispatchToolResults()
			if hmc325HasQuery(child, full) || !hmc325HasQuery(child, clipped) || !hmc325HasQuery(parent, full) || !hmc325HasQuery(sibling, full) {
				t.Fatal("branch-local replacement changed another branch or failed to withdraw its own absent query")
			}
			hmc326Merge(parent, child, dataOnly)
			if !hmc325HasQuery(parent, full) || !hmc325HasQuery(parent, clipped) || hmc325HasQuery(sibling, clipped) {
				t.Fatal("current child merge lost a legal query or changed an unmerged sibling")
			}
		})
	}
}

func TestHMC326ReplacementOnlyIntersectsExistingNativeRegistrations(t *testing.T) {
	ctx, path := hmc323BusinessBudgetContext(t)
	full := hmc325PublicBusinessQuery(t, ctx, path, 5.2, 5.312)
	ctx.Mutable.ResetDispatchToolResults()
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 700, "time_start": 5.201, "time_end": 5.2069})
	unregistered, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !unregistered.Success || len(unregistered.Observations) == 0 {
		t.Fatalf("public unregistered query failed: %v", err)
	}
	before := ctx.Mutable.NativeFactRetentionStatus()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{full, unregistered}})
	if after := ctx.Mutable.NativeFactRetentionStatus(); after.RetainedQueries != before.RetainedQueries || after.EncodedBytes != before.EncodedBytes {
		t.Fatal("replacement admitted a query that never passed dispatch admission")
	}
	ctx.Mutable.PublishTurnAArtifacts(types.TurnAArtifacts{}, types.TurnAHandoffHistoryCompacted)
	if !hmc325HasQuery(ctx, full) || hmc325HasQuery(ctx, unregistered) {
		t.Fatal("replacement intersection lost its original or newly admitted the other query")
	}
	var restored types.ToolResult
	if err := json.Unmarshal([]byte(hmc325BoundaryJSON(t, full)), &restored); err != nil {
		t.Fatal(err)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{restored}})
	if got := ctx.Mutable.NativeFactRetentionStatus(); got.RetainedQueries != 0 {
		t.Fatal("restored JSON kept a native registration after explicit replacement")
	}
	beforeArtifacts := ctx.Mutable.TurnAArtifacts()
	if ctx.Mutable.PublishTurnAArtifacts(types.TurnAArtifacts{}, types.TurnAHandoffPublicationKind(255)) ||
		!reflect.DeepEqual(beforeArtifacts, ctx.Mutable.TurnAArtifacts()) {
		t.Fatal("invalid publication kind mutated the handoff")
	}
}
