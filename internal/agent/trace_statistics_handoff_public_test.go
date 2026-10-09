package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the real producer and the data-only sibling handoff. Availability
// must survive value copies without importing the losing model's completion.
func TestTraceStatisticsPublicSiblingHandoffPreservesAvailabilityNotClosure(t *testing.T) {
	bus, path := statisticsReadinessFixture(t, "H:PreferredFrameRate")
	queryInSibling := func(view string) []types.ToolResult {
		t.Helper()
		fork := bus.Mutable.ForkForExploreDispatch()
		child := bus.ShallowClone()
		child.Mutable = fork
		result := statisticsReadinessQuery(t, child, path, view, 1, 2, 0)
		fork.AppendDispatchToolResult(result)
		fork.SetInvestigationComplete("losing model claims the question is answered")
		fork.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: fork.DispatchToolResults()})
		added := bus.Mutable.MergeExploreForkPublishedTools(fork)
		if len(added) != 1 {
			t.Fatalf("%s expected one producer publication after duplicate coalescing: %d", view, len(added))
		}
		if extra := bus.Mutable.MergeExploreForkPublishedTools(fork); len(extra) != 0 {
			t.Fatalf("repeated handoff duplicated producer result: %d", len(extra))
		}
		return added
	}
	raw := queryInSibling("process_measurements")
	if got := bus.Mutable.TraceStatisticsAvailability(raw); !got.HasPending() {
		t.Fatalf("publication copy lost raw availability: %+v", got)
	}
	snapshot := bus.Mutable.TurnAArtifacts()
	if snapshot == nil {
		t.Fatal("missing handoff snapshot")
	}
	e := &explorerEvaluator{mutable: bus.Mutable, turnRouteHint: types.TurnRouteHint{Route: "repo", Source: "external_tool", Confidence: 1, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}}
	sig := e.postExternalObservationSufficiencySignal(LoopObservation{AllToolResults: snapshot.ToolResults})
	if !strings.Contains(sig.Hint, "preferred_frame_rate") || strings.Contains(sig.Hint, "Prefer closing") {
		t.Fatalf("snapshot clone lost optional derived navigation: %+v", sig)
	}
	ctx := parseOutputCtx("", "")
	ctx.Mutable, ctx.TurnRouteHint = bus.Mutable, e.turnRouteHint
	e.analysisIR = ctx.AnalysisIR
	out, err := e.ParseOutput(ctx, nil, snapshot.ToolResults, nil)
	if err != nil || out.SignalUpdates == nil || out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("losing sibling model closure leaked through data handoff: err=%v updates=%+v", err, out.SignalUpdates)
	}
	queryInSibling("preferred_frame_rate")
	snapshot = bus.Mutable.TurnAArtifacts()
	got := bus.Mutable.TraceStatisticsAvailability(snapshot.ToolResults)
	if got.HasPending() || len(got.ComputedViews) != 1 || got.ComputedViews[0] != "preferred_frame_rate" {
		t.Fatalf("second sibling's exact-scope computation did not survive handoff: %+v", got)
	}
}
