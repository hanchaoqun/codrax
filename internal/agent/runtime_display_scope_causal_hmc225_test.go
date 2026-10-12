package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC225PublicComparisonCausalRulersStaySourceOwned(t *testing.T) {
	for _, sameWindow := range []bool{false, true} {
		name := "different_windows"
		if sameWindow {
			name = "same_window_independent_originals"
		}
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_source_windows/baseline.systrace")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			a, b := filepath.Join(dir, "a.systrace"), filepath.Join(dir, "b.systrace")
			for _, path := range []string{a, b} {
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			startA, endA, startB, endB := 1.0, 1.01, 4.0, 4.02
			if sameWindow {
				startB, endB = startA, endA
			}
			left := types.RuntimeArtifactComparisonSide{Path: a, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{TimeStart: &startA, TimeEnd: &endA, SourceQuote: "a interval"}}
			right := types.RuntimeArtifactComparisonSide{Path: b, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{TimeStart: &startB, TimeEnd: &endB, SourceQuote: "b interval"}}
			profile := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, Comparison: &types.RuntimeArtifactComparisonScope{Baseline: left, Current: right}, TimeWindows: []types.RuntimeArtifactTimeWindow{left.RuntimeArtifactTimeWindow, right.RuntimeArtifactTimeWindow}}
			ctx := &types.AgentContext{RepoRoot: dir, WorkDir: t.TempDir(), Mutable: types.NewMutableState("causal scope"), AgentName: types.AgentFinalizer, Stage: types.StageFinalize, TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()}), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: profile, RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Source: "user_explicit", Confidence: 1}}}}}
			var results []types.ToolResult
			for _, path := range []string{a, b} {
				for _, window := range [][2]float64{{1, 1.01}, {4, 4.02}} {
					for _, view := range []string{"thread_timeline", "wakeup_chain"} {
						raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 41, "time_start": window[0], "time_end": window[1]})
						result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
						if err != nil || !result.Success {
							t.Fatalf("native %s: %v %+v", view, err, result)
						}
						results = append(results, result)
					}
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			ledger := answerDocObservationLedger(ctx)
			before, _ := json.Marshal(ledger.Records)
			set := types.CompileTraceCausalProjectionSet(ledger)
			principalSources := map[string]bool{}
			for _, p := range set.Projections {
				if p.QuerySourceRef == nil {
					t.Fatal("comparison projection lost exact result source")
				}
				if p.WindowScope.Role == types.TraceQueryWindowScopeRequestedPrincipal {
					want := startA
					if filepath.Base(p.QuerySourceRef.Path) == filepath.Base(b) {
						want = startB
					}
					if p.WindowStartTs != want {
						t.Fatalf("peer-window projection became principal: %+v", p.WindowScope)
					}
					principalSources[p.QuerySourceRef.Path] = true
				}
			}
			if len(principalSources) != 2 {
				t.Fatalf("source principal projections=%v", principalSources)
			}
			states := types.BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
			principalStates := 0
			for _, state := range states {
				if state.WindowScope.Role == types.TraceQueryWindowScopeRequestedPrincipal {
					principalStates++
					if state.TotalMS <= 0 {
						t.Fatal("lost measured state account")
					}
					if state.WindowStartTs == 1 && (state.RunningMS < 3.999 || state.RunningMS > 4.001 || state.SleepMS < 1.999 || state.SleepMS > 2.001) {
						t.Fatalf("own-window account lost its out-of-window closing witness: %+v", state)
					}
				}
			}
			if principalStates < 2 {
				t.Fatalf("both independent state rulers required, got %d: %+v", principalStates, states)
			}
			waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
			principalWaits := 0
			for _, wait := range waits {
				if wait.WindowScope.Role == types.TraceQueryWindowScopeRequestedPrincipal {
					principalWaits++
				}
			}
			if principalWaits < 2 {
				t.Fatalf("both independent wait rulers required, got %d: %+v", principalWaits, waits)
			}
			after, _ := json.Marshal(ledger.Records)
			if string(before) != string(after) {
				t.Fatal("causal scope changed original chain/closing witnesses")
			}
			ctx.Mutable.ResetTurnAArtifacts()
			for _, state := range types.BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger) {
				if state.WindowScope.Role == types.TraceQueryWindowScopeRequestedPrincipal {
					t.Fatal("stale original gained requested state authority")
				}
			}
			for _, wait := range types.BuildTraceTargetWaitSummaryAuthorities(ledger, &ctx.AnalysisIR.RequestModel) {
				if wait.WindowScope.Role == types.TraceQueryWindowScopeRequestedPrincipal {
					t.Fatal("stale original gained requested wait authority")
				}
			}
		})
	}
}
