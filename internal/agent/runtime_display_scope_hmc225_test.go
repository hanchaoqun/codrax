package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc225DisplayContext(t *testing.T) (*types.AgentContext, []types.ToolResult, map[string]bool) {
	t.Helper()
	cpu, err := os.ReadFile("../../eval/fixtures/hmosperf_cpu_state_frequency/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	business, err := os.ReadFile("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	first := string(cpu) + strings.ReplaceAll(string(business), "5.", "1.")
	content := first + strings.ReplaceAll(first, "1.", "2.")
	dir := t.TempDir()
	a, b := filepath.Join(dir, "old.systrace"), filepath.Join(dir, "new.systrace")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	left, right, endA, endB := 1.0, 2.0, 1.04, 2.04
	tsa := types.RuntimeArtifactComparisonSide{Path: a, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{TimeStart: &left, TimeEnd: &endA, SourceQuote: "old 1..1.04"}}
	tb := types.RuntimeArtifactComparisonSide{Path: b, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{TimeStart: &right, TimeEnd: &endB, SourceQuote: "new 2..2.04"}}
	scope := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, Comparison: &types.RuntimeArtifactComparisonScope{Baseline: tsa, Current: tb}, TimeWindows: []types.RuntimeArtifactTimeWindow{tsa.RuntimeArtifactTimeWindow, tb.RuntimeArtifactTimeWindow}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: t.TempDir(), Mutable: types.NewMutableState("comparison"), AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()}),
		AnalysisIR:         &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{}, RuntimeArtifactScopeProfile: scope}}}
	var results []types.ToolResult
	wanted := map[string]bool{}
	for _, path := range []string{a, b} {
		for _, start := range []float64{1, 2} {
			for _, view := range []string{"cpu_state_frequency", "window_stats", "event_search", "process_profile", "thread_timeline"} {
				args := map[string]any{"source": "path", "path": path, "view": view, "time_start": start, "time_end": start + .04}
				if view == "thread_timeline" || view == "process_profile" {
					args["pid"] = 700
				}
				if view == "event_search" {
					args["event_names"] = []string{"sched_wakeup"}
				}
				raw, _ := json.Marshal(args)
				result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
				if err != nil || !result.Success {
					t.Fatalf("%s: %v %+v", view, err, result)
				}
				results = append(results, result)
				if path == a && start == 1 || path == b && start == 2 {
					for _, r := range result.Observations {
						wanted[r.ID] = true
					}
				}
			}
		}
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx, results, wanted
}

func TestHMC225PublicComparisonDisplayDoesNotBorrowPeerWindow(t *testing.T) {
	ctx, _, wanted := hmc225DisplayContext(t)
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal(ledger.Records)
	selected, _ := answerDocSelectedWindowObservationRecords(ctx, ledger.Records)
	if len(selected) == 0 {
		t.Fatal("lost all healthy source-owned rows")
	}
	for _, r := range selected {
		if types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) && !wanted[r.ID] {
			t.Errorf("principal row borrowed peer source window: %s source=%s window=%g", r.ID, r.SourceRef.Path, r.SourceRef.QueryWindowStartTs)
		}
	}
	for _, r := range answerDocPublishedNativeFacts(ctx) {
		if !wanted[r.ID] {
			t.Errorf("native fact borrowed peer: %s", r.ID)
		}
	}
	if rows, _ := answerDocSchedulerConcurrencyRows(ledger, &ctx.AnalysisIR.RequestModel); len(rows) == 0 {
		t.Fatal("lost healthy scheduler rows")
	} else {
		for _, r := range rows {
			if !wanted[r.ID] {
				t.Errorf("scheduler borrowed peer: %s", r.ID)
			}
		}
	}
	cpu := renderAnswerDocCPUStateFrequency(ctx, ledger)
	for _, r := range ledger.Records {
		if r.Predicate == tool.TraceCPUStateFrequencyPredicate && strings.Contains(cpu, r.ID) != wanted[r.ID] {
			t.Errorf("CPU source scope mismatch: %s wanted=%v", r.ID, wanted[r.ID])
		}
	}
	for _, event := range types.RuntimeWakeupDiagramEvents(ledger, &ctx.AnalysisIR.RequestModel) {
		if !wanted[event.Record.ID] {
			t.Errorf("wakeup support borrowed peer: %s", event.Record.ID)
		}
	}
	for _, r := range ledger.Records {
		if _, ok := tool.DecodeTraceBusinessTreeFact(r); ok && !wanted[r.ID] {
			for _, relation := range tool.RuntimeDiagramRelations(ledger, &ctx.AnalysisIR.RequestModel) {
				for _, ref := range r.SupportRefs {
					for _, used := range relation.SupportRefs {
						if ref == used {
							t.Errorf("business diagram borrowed peer: %s", r.ID)
						}
					}
				}
			}
		}
	}
	after, _ := json.Marshal(ledger.Records)
	if string(before) != string(after) {
		t.Fatal("display mutated full evidence ledger")
	}
}
