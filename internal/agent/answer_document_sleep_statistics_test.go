package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSleepStatisticsPublicBoundedFinalContext(t *testing.T) {
	for _, directWaker := range []bool{false, true} {
		t.Run(map[bool]string{false: "actual_state_count_profile", true: "with_direct_waker"}[directWaker], func(t *testing.T) {
			testSleepStatisticsPublicBoundedFinalContext(t, directWaker)
		})
	}
}

func testSleepStatisticsPublicBoundedFinalContext(t *testing.T, directWaker bool, eventSearch ...bool) {
	t.Helper()
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_summary/events.systrace")
	start, end := 10.001, 10.012
	families := []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences, types.RuntimeQuestionFactCountOrDuration}
	if directWaker {
		families = append(families, types.RuntimeQuestionFactDirectWaker)
	}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("sleep"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Language: "zh", PerfTrace: &types.PerfBundle{},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "target-41", Source: "user_explicit", Confidence: .95}},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: families},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10.001..10.012"},
	}}}
	var observations []types.ObservationRecord
	// Match the live timeline -> dependency query sequence as well as its
	// explicit-window profile; a profile-free one-query test missed this gap.
	dependencyView := "wakeup_chain"
	if len(eventSearch) > 0 && eventSearch[0] {
		dependencyView = "event_search"
	}
	for _, view := range []string{"thread_timeline", dependencyView} {
		args := map[string]any{"source": "path", "path": path, "view": view, "pid": 41, "time_start": start, "time_end": end}
		if view == "event_search" {
			delete(args, "pid")
			args["event_types"] = []string{"sched_wakeup", "sched_blocked_reason"}
		}
		params, _ := json.Marshal(args)
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !r.Success {
			t.Fatalf("query: %v %+v", err, r)
		}
		// A scheduler-only trace need not contain any business marker. Keep
		// the real event records but remove the optional business-tree carrier.
		var kept []types.ObservationRecord
		for _, record := range r.Observations {
			if record.Predicate != types.TraceBusinessTreePredicate {
				kept = append(kept, record)
			}
		}
		r.Observations = kept
		ctx.Mutable.AppendDispatchToolResult(r)
		observations = append(observations, r.Observations...)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	relations := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
	if len(relations) == 0 {
		t.Fatal("missing native wakeup relation")
	}
	if dependencyView == "event_search" && len(relations) != 2 {
		t.Fatalf("expected both row-local wake events, got %+v", relations)
	}
	for _, relation := range relations {
		if relation.Kind != types.DiagramRelWakeup {
			continue
		}
		if !strings.Contains(prompt, relation.FromIdentity) || !strings.Contains(prompt, relation.ToIdentity) {
			t.Fatal("actual finalizer instruction lost the copyable event recipe")
		}
	}
	for _, want := range []string{"2 intervals; clipped sum=4ms, mean=2ms, max=3ms", "1 intervals; clipped sum=3ms, mean=3ms, max=3ms", "not proof of cause or completion"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("final context lost full-population statistic %q", want)
		}
	}
	for _, record := range observations {
		if record.Predicate != "target_sleep_state_summary" {
			continue
		}
		if len(answerDocScopeProjectedObservationRecords(ctx, []types.ObservationRecord{record})) != 1 {
			t.Fatal("own summary lost")
		}
		peer := record
		peer.Subject = "worker-3"
		if len(answerDocScopeProjectedObservationRecords(ctx, []types.ObservationRecord{peer})) != 0 {
			t.Fatal("sleep summary family laundered an unrelated thread")
		}
	}
}

func TestSleepStatisticsPublicEventSearchFinalContext(t *testing.T) {
	testSleepStatisticsPublicBoundedFinalContext(t, false, true)
}
