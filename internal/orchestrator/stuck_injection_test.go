package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestInjectInconclusiveForStuck_OnlyUnknownAffected verifies the
// scheduler's escape hatch for shape-stuck validate loops touches
// ONLY hypotheses in HypUnknown state. Confirmed / rejected /
// inconclusive hypotheses stay as the analyzer or LLM decided them —
// the escape is for advancing past undecidable state, not overriding
// prior verdicts.
func TestInjectInconclusiveForStuck_OnlyUnknownAffected(t *testing.T) {
	bus := &types.BusContext{
		Mutable: types.NewMutableState("stuck test"),
		AnalysisIR: &types.AnalysisIR{
			HypothesisSet: []types.Hypothesis{
				{ID: "h1", Status: types.HypUnknown},
				{ID: "h2", Status: types.HypConfirmed},
				{ID: "h3", Status: types.HypRejected},
				{ID: "h4", Status: types.HypInconclusive},
				{ID: "h5", Status: ""}, // zero-value = Unknown-equivalent
			},
		},
	}
	o := &Orchestrator{busCtx: bus}

	n := o.injectInconclusiveForStuckHypotheses("n2_validate")

	if n != 2 {
		t.Fatalf("injected %d, want 2 (h1 unknown + h5 empty-string)", n)
	}

	verdicts := bus.Mutable.EmittedHypothesisVerdicts()
	got := map[string]types.HypothesisStatus{}
	rationale := map[string]string{}
	for _, v := range verdicts {
		got[v.HypothesisID] = v.Status
		rationale[v.HypothesisID] = v.Rationale
	}
	if got["h1"] != types.HypInconclusive {
		t.Errorf("h1 status = %q, want inconclusive", got["h1"])
	}
	if got["h5"] != types.HypInconclusive {
		t.Errorf("h5 status = %q, want inconclusive", got["h5"])
	}
	// Others must NOT have been touched — they were not originally
	// unknown, so the escape hatch skipped them.
	for _, id := range []string{"h2", "h3", "h4"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s was touched by inject (should be skipped, original status preserved)", id)
		}
	}
	// Rationale should carry the stuck-signal + the node ID for
	// traceability.
	for _, id := range []string{"h1", "h5"} {
		if !strings.Contains(rationale[id], "stuck") {
			t.Errorf("%s rationale missing stuck signal: %q", id, rationale[id])
		}
		if !strings.Contains(rationale[id], "n2_validate") {
			t.Errorf("%s rationale missing validate node ID: %q", id, rationale[id])
		}
	}
}

// TestInjectInconclusiveForStuck_SkipsAlreadyVerdicted verifies the
// method honors existing verdicts emitted by the LLM or prior
// runAutoVerdicts — it does not double-inject over them.
func TestInjectInconclusiveForStuck_SkipsAlreadyVerdicted(t *testing.T) {
	bus := &types.BusContext{
		Mutable: types.NewMutableState("t"),
		AnalysisIR: &types.AnalysisIR{
			HypothesisSet: []types.Hypothesis{
				{ID: "h1", Status: types.HypUnknown},
			},
		},
	}
	// Pre-seed an existing verdict for h1 (as if LLM emitted one).
	bus.Mutable.AppendEmittedHypothesisVerdicts([]types.HypothesisVerdict{
		{HypothesisID: "h1", Status: types.HypConfirmed, Rationale: "found proof"},
	})
	o := &Orchestrator{busCtx: bus}

	n := o.injectInconclusiveForStuckHypotheses("nX")

	if n != 0 {
		t.Errorf("injected %d, want 0 (h1 already has a verdict)", n)
	}
	// Confirm the original verdict stands.
	verdicts := bus.Mutable.EmittedHypothesisVerdicts()
	if len(verdicts) != 1 || verdicts[0].Status != types.HypConfirmed {
		t.Errorf("original verdict lost: %+v", verdicts)
	}
}

// TestInjectInconclusiveForStuck_EmptyHypothesisSet_Noop verifies
// the method is a clean no-op when there are no hypotheses to touch.
func TestInjectInconclusiveForStuck_EmptyHypothesisSet_Noop(t *testing.T) {
	bus := &types.BusContext{
		Mutable:    types.NewMutableState("t"),
		AnalysisIR: &types.AnalysisIR{},
	}
	o := &Orchestrator{busCtx: bus}
	n := o.injectInconclusiveForStuckHypotheses("n2")
	if n != 0 {
		t.Errorf("injected %d, want 0 (empty HypothesisSet)", n)
	}
}

func TestRunAutoVerdicts_SkipsObservationOnlyRuntimeArtifact(t *testing.T) {
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				Func: "Cart.itemAt",
				File: "src/cart/Cart.cj",
				Line: 78,
			}},
		}},
	}
	mut := types.NewMutableState("runtime panic")
	mut.SetLogTriage(logBundle)
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: logBundle,
			},
			HypothesisSet: []types.Hypothesis{{
				ID:                     "h1",
				Status:                 types.HypUnknown,
				FalsificationCondition: types.Criterion{Kind: types.CritNoCallSites, Expr: "Cart.itemAt"},
			}},
		},
	}
	o := &Orchestrator{busCtx: bus}

	o.runAutoVerdicts()

	if got := bus.Mutable.EmittedHypothesisVerdicts(); len(got) != 0 {
		t.Fatalf("observation-only artifact should not get repo-evidence auto-verdicts, got %+v", got)
	}
}

// TestInjectInconclusiveForStuck_NilGuardsDontPanic covers the
// defensive paths — nil bus, nil IR, nil mutable all return 0
// without panicking.
func TestInjectInconclusiveForStuck_NilGuardsDontPanic(t *testing.T) {
	cases := []*Orchestrator{
		{busCtx: nil},
		{busCtx: &types.BusContext{}},
		{busCtx: &types.BusContext{Mutable: types.NewMutableState("t")}},
	}
	for i, o := range cases {
		if got := o.injectInconclusiveForStuckHypotheses("n"); got != 0 {
			t.Errorf("case %d: got %d, want 0", i, got)
		}
	}
}
