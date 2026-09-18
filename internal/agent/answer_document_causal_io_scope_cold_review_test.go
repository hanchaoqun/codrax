package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// An overlapping physical request is still witnessed in each independently
// requested query window. Repeated identical queries can be compacted, but a
// finalizer must not silently choose one of two explicit windows for the user.
func TestCausalIOHandoffColdReviewKeepsDistinctExplicitWindows(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	aStart, aEnd, bStart, bEnd := 1.006, 1.020, 1.025, 1.045
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "en",
		AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Explain the IO evidence separately in both selected windows"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: "en", Intent: types.IntentRootCause, PerfTrace: &types.PerfBundle{},
			RuntimeTargets:         []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "app-main", Source: "user_explicit"}},
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeWindows: []types.RuntimeArtifactTimeWindow{
					{TimeStart: &aStart, TimeEnd: &aEnd, SourceQuote: "1.006..1.020"},
					{TimeStart: &bStart, TimeEnd: &bEnd, SourceQuote: "1.025..1.045"},
				},
			},
		}},
	}
	for _, bounds := range [][2]float64{{aStart, aEnd}, {bStart, bEnd}, {aStart, aEnd}, {0.998, 1.052}} {
		params, err := json.Marshal(map[string]any{
			"source": "path", "path": path, "view": "window_stats",
			"time_start": bounds[0], "time_end": bounds[1], "trace_flavor": "harmony_hitrace",
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !result.Success {
			t.Fatalf("public window query %v failed: %v %s", bounds, err, result.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	ledger := answerDocObservationLedger(ctx)
	wantWindows := map[string]int{
		fmt.Sprintf("%.6f..%.6f", aStart, aEnd): 0,
		fmt.Sprintf("%.6f..%.6f", bStart, bEnd): 0,
	}
	for _, row := range ledger.Records {
		if row.Predicate == "io_latency" && row.Subject == "document-worker-200" {
			window := traceQueryObservationSupplementNoteValue(row, types.TraceNoteKeySelectedWindow)
			if _, expected := wantWindows[window]; expected {
				wantWindows[window]++
			}
		}
	}
	for window, count := range wantWindows {
		if count == 0 {
			t.Fatalf("premise failed: public query did not publish request for %s", window)
		}
		wantWindows[window] = 0
	}
	before, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	var lines []string
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.Contains(line, "subject=`document-worker-200`") || !strings.Contains(line, "request_residence=`35.000`") {
			continue
		}
		lines = append(lines, line)
		if !strings.Contains(line, "issuer_blocked=`31.000`") || !strings.Contains(line, "owner_scope=`selected_window_context`") {
			t.Errorf("window-scoped source row lost IO caliber or was promoted to target ownership: %s", line)
		}
		if strings.Contains(line, `selected_window="0.998000..1.052000"`) {
			t.Errorf("out-of-scope broad query reintroduced: %s", line)
		}
		for window := range wantWindows {
			if strings.Contains(line, fmt.Sprintf("selected_window=%q", window)) {
				wantWindows[window]++
			}
		}
	}
	for window, count := range wantWindows {
		if count != 1 {
			t.Errorf("expected one source-bound IO row for explicit window %s; got %d (all rows: %v)", window, count, lines)
		}
	}
	after, err := json.Marshal(answerDocObservationLedger(ctx))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("prompt formatting changed accepted source records")
	}
}
