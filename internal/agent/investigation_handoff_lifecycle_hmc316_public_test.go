package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The native table and row seal come from public query/completion tools. A
// retained completion must carry the same ownership as its accepted decision,
// without completing the parent's current retry window.
func TestHMC316PublicDecidedResetForkPreservesCurrentNativeHandoff(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	beforeProjection := string(hmc218CausalProjection(t, ctx))
	fork := ctx.ShallowClone()
	fork.Mutable = ctx.Mutable.ForkForExploreDispatch()
	r := hmc303Complete(t, fork, selected, map[string]any{
		"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6},
	})
	if !r.Success {
		t.Fatal(r.Summary)
	}
	want := fork.Mutable.AcceptedInvestigationNativeFactHandoff()
	if want == nil {
		t.Fatal("public completion did not accept the native handoff")
	}
	reason := fork.Mutable.StableInvestigationCompleteReason()
	fork.Mutable.ResetInvestigationComplete()
	if !reflect.DeepEqual(want, fork.Mutable.AcceptedInvestigationNativeFactHandoff()) {
		t.Fatal("same-investigation reset lost the fork's accepted handoff")
	}
	ctx.Mutable.MergeExploreFork(fork.Mutable)
	if ctx.Mutable.IsInvestigationComplete() || ctx.Mutable.StableInvestigationCompleteReason() != reason {
		t.Fatal("retained merge changed the current window flag or lost its reason")
	}
	if got := ctx.Mutable.AcceptedInvestigationNativeFactHandoff(); !reflect.DeepEqual(got, want) {
		t.Fatalf("retained reason arrived without its own current handoff: got=%+v want=%+v", got, want)
	}
	for name, prompt := range map[string]string{
		"extract":  (&extractorEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil),
		"finalize": dependencyObservationMessages(t, ctx.ShallowClone()),
	} {
		if !strings.Contains(prompt, "native_reference_status=current_exact_rows") {
			t.Fatalf("%s lost current native row status after retained merge", name)
		}
	}
	if beforeProjection != string(hmc218CausalProjection(t, ctx)) {
		t.Fatal("handoff lifecycle changed the causal projection")
	}
}

func TestHMC316PublicDecidedResetLegacyForkRetiresPriorNativeHandoff(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	r := hmc303Complete(t, ctx, selected, map[string]any{
		"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6},
	})
	if !r.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
		t.Fatal("public typed precondition was not accepted: " + r.Summary)
	}
	beforeProjection := string(hmc218CausalProjection(t, ctx))
	reason := ctx.Mutable.StableInvestigationCompleteReason()
	ctx.Mutable.ResetInvestigationComplete()
	fork := ctx.ShallowClone()
	fork.Mutable = ctx.Mutable.ForkForExploreDispatch()
	// Reusing the text is legitimate; matching text cannot transfer the prior
	// completion's private acceptance into this distinct legacy decision.
	raw, _ := json.Marshal(map[string]any{"reason": reason, "confidence": "high", "result_kind": "resolved"})
	r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(fork, types.AgentExplorer), raw)
	if err != nil || !r.Success {
		t.Fatalf("public legacy completion: %v %s", err, r.Summary)
	}
	if fork.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("legacy decision did not retire its own inherited handoff")
	}
	fork.Mutable.ResetInvestigationComplete()
	ctx.Mutable.MergeExploreFork(fork.Mutable)
	if ctx.Mutable.IsInvestigationComplete() || ctx.Mutable.StableInvestigationCompleteReason() != reason {
		t.Fatal("retained legacy merge changed the live flag or reason")
	}
	if ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("retained legacy merge left the prior same-reason handoff current")
	}
	if beforeProjection != string(hmc218CausalProjection(t, ctx)) {
		t.Fatal("retiring narrative ownership removed or elevated native causal facts")
	}
}

func hmc316CompleteNative(t *testing.T, ctx *types.AgentContext, selected types.AnswerRuntimeMeasurementReceipt, reason string) *types.InvestigationNativeFactHandoff {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"reason": reason, "confidence": "high", "result_kind": "resolved",
		"native_fact_handoff": map[string]any{
			"runtime_measurements": []types.AnswerRuntimeMeasurementReceipt{selected},
			"interpretation_role":  "independent_analysis",
			"interpretation_units": []types.InvestigationInterpretationUnit{{
				Text: reason + ": independent analysis remains advisory.",
				NativeRefs: []types.InvestigationNativeRowReference{{
					ObservationID: selected.ObservationID, View: selected.View, RowIndices: []int{6},
				}},
			}},
		},
	})
	r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !r.Success {
		t.Fatalf("public typed completion: %v %s", err, r.Summary)
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	if h == nil {
		t.Fatal("accepted public completion has no current handoff")
	}
	return h
}

func TestHMC316PublicRetainedHandoffDoesNotBorrowDifferentLiveReason(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	hmc316CompleteNative(t, ctx, selected, "parent live decision")
	fork := ctx.ShallowClone()
	fork.Mutable = ctx.Mutable.ForkForExploreDispatch()
	want := hmc316CompleteNative(t, fork, selected, "fork retained decision")
	fork.Mutable.ResetInvestigationComplete()
	ctx.Mutable.MergeExploreFork(fork.Mutable)
	if !ctx.Mutable.IsInvestigationComplete() || ctx.Mutable.InvestigationCompleteReason() != "parent live decision" {
		t.Fatal("retained merge rewrote the parent's separate live window")
	}
	if ctx.Mutable.InvestigationCompleteGeneration() != 2 {
		t.Fatal("the retained accepted decision was not counted exactly once")
	}
	if ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("retained handoff borrowed a different live reason")
	}
	ctx.Mutable.ResetInvestigationComplete()
	if ctx.Mutable.StableInvestigationCompleteReason() != "fork retained decision" || !reflect.DeepEqual(want, ctx.Mutable.AcceptedInvestigationNativeFactHandoff()) {
		t.Fatal("normal reset failed to expose the matching retained decision")
	}
	if !types.InvestigationNativeRowReferencesCurrent(ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract) {
		t.Fatal("retained row seal was not carried intact")
	}
}

func TestHMC316PublicNestedResetForkCountsOwnDecisions(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	beforeProjection := string(hmc218CausalProjection(t, ctx))
	fork := ctx.ShallowClone()
	fork.Mutable = ctx.Mutable.ForkForExploreDispatch()
	nested := fork.ShallowClone()
	nested.Mutable = fork.Mutable.ForkForExploreDispatch()
	hmc316CompleteNative(t, nested, selected, "first nested decision")
	want := hmc316CompleteNative(t, nested, selected, "second nested decision")
	nested.Mutable.ResetInvestigationComplete()
	fork.Mutable.MergeExploreFork(nested.Mutable)
	if fork.Mutable.InvestigationCompleteGeneration() != 2 || fork.Mutable.IsInvestigationComplete() {
		t.Fatal("nested retained merge lost or manufactured completion decisions")
	}
	ctx.Mutable.MergeExploreFork(fork.Mutable)
	if ctx.Mutable.InvestigationCompleteGeneration() != 2 || ctx.Mutable.IsInvestigationComplete() || !reflect.DeepEqual(want, ctx.Mutable.AcceptedInvestigationNativeFactHandoff()) {
		t.Fatal("nested acceptance was not carried with its exact generation delta")
	}
	copy := nested.Mutable.AcceptedInvestigationNativeFactHandoff()
	copy.InterpretationUnits[0].Text = "mutated copy"
	nested.Mutable.ResetTurnAArtifacts()
	if !reflect.DeepEqual(want, ctx.Mutable.AcceptedInvestigationNativeFactHandoff()) || !reflect.DeepEqual(want, fork.Mutable.AcceptedInvestigationNativeFactHandoff()) {
		t.Fatal("merged handoff shares mutable ownership with its nested fork")
	}
	if beforeProjection != string(hmc218CausalProjection(t, ctx)) {
		t.Fatal("nested narrative handoff changed native causal permissions")
	}
}

func TestHMC316PublicUndecidedForkCannotReplaceCurrentHandoff(t *testing.T) {
	for _, mode := range []string{"inherited", "reset_only", "rejected"} {
		t.Run(mode, func(t *testing.T) {
			ctx, selected := hmc303BusinessReference(t)
			hmc316CompleteNative(t, ctx, selected, "initial decision")
			fork := ctx.ShallowClone()
			fork.Mutable = ctx.Mutable.ForkForExploreDispatch()
			if mode == "reset_only" {
				fork.Mutable.ResetInvestigationComplete()
			}
			if mode == "rejected" {
				raw, _ := json.Marshal(map[string]any{
					"reason": "invalid selection", "confidence": "high", "result_kind": "resolved",
					"native_fact_handoff": map[string]any{
						"runtime_measurements": []types.AnswerRuntimeMeasurementReceipt{{ObservationID: "not-published", View: types.RuntimeMeasurementSummary}},
						"interpretation":       "not accepted", "interpretation_role": "independent_analysis",
					},
				})
				r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(fork, types.AgentExplorer), raw)
				if err != nil || r.Success {
					t.Fatalf("invalid public selection did not fail normally: %v %+v", err, r)
				}
			}
			want := hmc316CompleteNative(t, ctx, selected, "new parent decision")
			generation := ctx.Mutable.InvestigationCompleteGeneration()
			ctx.Mutable.MergeExploreFork(fork.Mutable)
			if ctx.Mutable.InvestigationCompleteGeneration() != generation || ctx.Mutable.StableInvestigationCompleteReason() != "new parent decision" || !reflect.DeepEqual(want, ctx.Mutable.AcceptedInvestigationNativeFactHandoff()) {
				t.Fatal("a fork without its own accepted decision replaced current ownership")
			}
		})
	}
}

func TestHMC316PublicSerializedIntentCannotAcquireRetainedAcceptance(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	h := hmc316CompleteNative(t, ctx, selected, "same text after JSON")
	beforeProjection := string(hmc218CausalProjection(t, ctx))
	wire, err := json.Marshal(types.TurnAArtifacts{AcceptedClosureReason: ctx.Mutable.StableInvestigationCompleteReason(), AcceptedNativeFactHandoff: h})
	if err != nil {
		t.Fatal(err)
	}
	var restored types.TurnAArtifacts
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if types.InvestigationNativeRowReferencesCurrent(restored.AcceptedNativeFactHandoff, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract) {
		t.Fatal("JSON restored the private native row seal")
	}
	// Use a real fork so observations remain available. Only the accepted
	// completion is replaced by an ordinary system/legacy decision.
	ctx.Mutable.ResetInvestigationComplete()
	fork := ctx.ShallowClone()
	fork.Mutable = ctx.Mutable.ForkForExploreDispatch()
	fork.Mutable.SetTurnAArtifacts(restored)
	fork.Mutable.SetInvestigationComplete(restored.AcceptedClosureReason)
	if fork.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("system decision borrowed the inherited or serialized credential")
	}
	fork.Mutable.ResetInvestigationComplete()
	ctx.Mutable.MergeExploreFork(fork.Mutable)
	if ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("retained merge restored JSON or a retired parent acceptance")
	}
	if beforeProjection != string(hmc218CausalProjection(t, ctx)) {
		t.Fatal("audit-only JSON changed existing causal observations")
	}
}

func TestHMC316PublicNestedRetainedCompletionDoesNotBecomeInheritedLiveDecision(t *testing.T) {
	for _, tc := range []struct {
		name         string
		nestedReason string
		newerParent  bool
	}{
		{"retained_across_two_merges", "nested retained decision", false},
		{"newer_parent_stays_live", "nested retained decision", true},
		{"same_reason_is_not_live_ownership", "parent live decision", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, selected := hmc303BusinessReference(t)
			hmc316CompleteNative(t, ctx, selected, "parent live decision")
			beforeProjection := string(hmc218CausalProjection(t, ctx))
			fork := ctx.ShallowClone()
			fork.Mutable = ctx.Mutable.ForkForExploreDispatch()
			nested := fork.ShallowClone()
			nested.Mutable = fork.Mutable.ForkForExploreDispatch()
			want := hmc316CompleteNative(t, nested, selected, tc.nestedReason)
			nested.Mutable.ResetInvestigationComplete()
			fork.Mutable.MergeExploreFork(nested.Mutable)
			if fork.Mutable.InvestigationCompleteReason() != "parent live decision" || fork.Mutable.StableInvestigationCompleteReason() != "parent live decision" {
				t.Fatal("first retained merge changed the inherited live window")
			}
			if tc.nestedReason != "parent live decision" && fork.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
				t.Fatal("retained ownership borrowed a different live reason")
			}
			liveReason := "parent live decision"
			if tc.newerParent {
				liveReason = "newer parent live decision"
				hmc316CompleteNative(t, ctx, selected, liveReason)
			}
			generation := ctx.Mutable.InvestigationCompleteGeneration()
			ctx.Mutable.MergeExploreFork(fork.Mutable)
			if !ctx.Mutable.IsInvestigationComplete() || ctx.Mutable.InvestigationCompleteReason() != liveReason {
				t.Fatalf("inherited live completion was treated as an own decision: got=%q want=%q", ctx.Mutable.InvestigationCompleteReason(), liveReason)
			}
			if ctx.Mutable.InvestigationCompleteGeneration() != generation+1 {
				t.Fatal("nested accepted generation was not transported exactly once")
			}
			ctx.Mutable.ResetInvestigationComplete()
			if ctx.Mutable.StableInvestigationCompleteReason() != tc.nestedReason || !reflect.DeepEqual(ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), want) {
				t.Fatal("current retained acceptance was mistaken for a retirement during nested transport")
			}
			if beforeProjection != string(hmc218CausalProjection(t, ctx)) {
				t.Fatal("nested completion ownership changed causal evidence")
			}
		})
	}
}

func TestHMC316PublicSameReasonDoesNotLendLiveAcceptance(t *testing.T) {
	for _, staleAccept := range []bool{false, true} {
		t.Run(map[bool]string{false: "new_parent_same_reason", true: "stale_accept_cannot_reseal"}[staleAccept], func(t *testing.T) {
			ctx, selected := hmc303BusinessReference(t)
			complete := func(c *types.AgentContext, row int) *types.InvestigationNativeFactHandoff {
				r := hmc303Complete(t, c, selected, map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{row}})
				if !r.Success {
					t.Fatal(r.Summary)
				}
				return c.Mutable.AcceptedInvestigationNativeFactHandoff()
			}
			prior := complete(ctx, 6)
			fork := ctx.ShallowClone()
			fork.Mutable = ctx.Mutable.ForkForExploreDispatch()
			nested := fork.ShallowClone()
			nested.Mutable = fork.Mutable.ForkForExploreDispatch()
			want := complete(nested, 8)
			nested.Mutable.ResetInvestigationComplete()
			fork.Mutable.MergeExploreFork(nested.Mutable)
			if staleAccept {
				fork.Mutable.AcceptInvestigationNativeFactHandoff(prior)
			}
			if fork.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
				t.Fatal("same text lent an inherited live decision to a different accepted generation")
			}
			if !staleAccept {
				complete(ctx, 9)
			}
			ctx.Mutable.MergeExploreFork(fork.Mutable)
			if ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
				t.Fatal("retained ownership acquired the separate parent live decision")
			}
			ctx.Mutable.ResetInvestigationComplete()
			if !reflect.DeepEqual(ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), want) {
				t.Fatal("a stale acceptance call or nested transport replaced the actual retained decision")
			}
		})
	}
}

func TestHMC316PublicOwnLiveThenRetainedDecisionKeepsSeparateGenerations(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	hmc316CompleteNative(t, ctx, selected, "initial parent")
	fork := ctx.ShallowClone()
	fork.Mutable = ctx.Mutable.ForkForExploreDispatch()
	hmc316CompleteNative(t, fork, selected, "fork own live")
	nested := fork.ShallowClone()
	nested.Mutable = fork.Mutable.ForkForExploreDispatch()
	want := hmc316CompleteNative(t, nested, selected, "nested retained")
	nested.Mutable.ResetInvestigationComplete()
	fork.Mutable.MergeExploreFork(nested.Mutable)
	hmc316CompleteNative(t, ctx, selected, "parent progressed independently")
	ctx.Mutable.MergeExploreFork(fork.Mutable)
	if !ctx.Mutable.IsInvestigationComplete() || ctx.Mutable.InvestigationCompleteReason() != "fork own live" || ctx.Mutable.InvestigationCompleteGeneration() != 4 {
		t.Fatal("the fork's actual live decision or exact two-decision delta was lost")
	}
	if ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("newer retained decision borrowed the fork's separate live decision")
	}
	ctx.Mutable.ResetInvestigationComplete()
	if ctx.Mutable.StableInvestigationCompleteReason() != "nested retained" || !reflect.DeepEqual(ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), want) {
		t.Fatal("retained acceptance did not survive rebasing both generation lanes")
	}
}
