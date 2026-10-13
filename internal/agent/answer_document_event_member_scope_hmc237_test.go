package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func eventMemberScopeHMC237Context(t *testing.T) (*types.AgentContext, []types.ObservationRecord) {
	t.Helper()
	result := traceInventoryScopePublicQuery(t,
		"main-11 (11) [000] .... 1.000000: tracing_mark_write: B|11|jank_event_sync: start_ts=920000000, end_ts=980000000, jank_frames=3, appid=11\n"+
			"main-22 (22) [001] .... 1.081000: tracing_mark_write: B|22|jank_event_sync: start_ts=980000000, end_ts=1080000000, jank_frames=4, appid=22\n"+
			"main-11 (11) [000] .... 1.161000: tracing_mark_write: B|11|jank_event_sync: start_ts=1100000000, end_ts=1160000000, jank_frames=5, appid=11\n"+
			"main-22 (22) [001] .... 1.421000: tracing_mark_write: B|22|jank_event_sync: start_ts=1320000000, end_ts=1420000000, jank_frames=7, appid=22\n"+
			"main-22 (22) [001] .... 1.499000: tracing_mark_write: B|22|jank_event_sync: start_ts=1430000000, end_ts=1470000000, jank_frames=6, appid=22\n"+
			"main-11 (11) [000] .... 1.500000: tracing_mark_write: B|11|jank_event_sync: start_ts=1440000000, end_ts=1490000000, jank_frames=8, appid=11\n", 1, 1.5, map[string]any{"time_start": "1.000000", "time_end": "1.500000"})
	ctx := traceInventoryScopeContext(result, 1, 1.5)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	ctx.AnalysisIR.RequestModel.Intent = types.IntentReturnValue
	var facts []types.ObservationRecord
	for _, record := range answerDocObservationLedger(ctx).Records {
		if strings.HasPrefix(record.ClaimKey, "evidence_fact:") {
			facts = append(facts, record)
		}
	}
	if len(facts) != 6 {
		t.Fatalf("fixture needs six unmodified native lookup facts, got %d", len(facts))
	}
	return ctx, facts
}

func TestEventMemberScopeHMC237ActualFinalizerUsesOneWindowRoster(t *testing.T) {
	ctx, facts := eventMemberScopeHMC237Context(t)
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || len(views[0].Inventory.Rows) != 5 {
		t.Fatalf("native inventory must already contain five displayed rows: %+v", views)
	}
	for name, text := range map[string]string{"actual_prompt": prompt, "native_fact_support": renderAnswerDocNativeFactSupport(ctx)} {
		if strings.Contains(text, facts[5].ID) {
			t.Errorf("%s reintroduced the excluded right-boundary event via its legacy fact ID", name)
		}
		for _, fact := range facts[:5] {
			if !strings.Contains(text, fact.ID) {
				t.Errorf("%s lost the in-window event %s (including late payload starts)", name, fact.ID)
			}
		}
	}
	carrier := types.ToolHandoffCarrier{Version: types.ToolHandoffCarrierVersion, ToolName: "trace_query"}
	for _, fact := range facts {
		carrier.ObservationRefs = append(carrier.ObservationRefs, types.ToolObservationRef{ID: fact.ID})
	}
	got := answerDocToolHandoffCarriersForFinalizer(ctx, []types.ToolHandoffCarrier{carrier})
	if len(got) != 1 || len(got[0].ObservationRefs) != 5 {
		t.Errorf("typed refs did not share the five-member display roster: %+v", got)
	}
	if after := traceInventoryScopeAuthority(t, ctx); before != after {
		t.Fatal("display projection changed full ledger, source evidence or causal projection")
	}
}

func TestEventMemberScopeHMC237DoesNotBlanketDeleteInvestigationNarrative(t *testing.T) {
	ctx, _ := eventMemberScopeHMC237Context(t)
	const synthesis = "MODEL_RUNTIME_SYNTHESIS_SIX_EVENTS"
	const independent = "Independent source investigation note remains available."
	ctx.Mutable.SetInvestigationComplete(synthesis)
	ta := *ctx.Mutable.TurnAArtifacts()
	ta.InvestigationNotes = []string{synthesis, independent}
	ctx.Mutable.SetTurnAArtifacts(ta)
	if runtimeObservationOnlyForAnswerDoc(ctx) {
		t.Fatal("fixture must reproduce the mixed/default-source route, not the old runtime-only suppression")
	}
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	// The original RED is retained outside the repository. Until an exact
	// budget-after-projection completeness witness exists, this patch does
	// not silently treat native rows as permission to delete mixed notes.
	if !strings.Contains(prompt, synthesis) {
		t.Error("member-only projection silently removed accepted investigation synthesis")
	}
	if !strings.Contains(renderAnswerDocInvestigationNarrativeHandoff(ctx), independent) {
		t.Error("independent investigation notes were suppressed with the runtime synthesis")
	}
	if ctx.Mutable.StableInvestigationCompleteReason() != synthesis || len(ctx.Mutable.TurnAArtifacts().InvestigationNotes) != 2 {
		t.Fatal("display suppression changed accepted investigation artifacts")
	}
}
