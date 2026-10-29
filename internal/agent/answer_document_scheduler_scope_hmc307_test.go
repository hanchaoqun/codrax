package agent

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The full business causal question has a bounded background preview, not a
// promise to expose every supporting row. Keep that boundary and its originals.
func TestHMC307SchedulerScopeBusinessBackgroundBudget(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal(ledger)
	filtered, pretriage := answerDocFinalizerObservationRecords(ctx, ledger.Records)
	filtered, separateIO, outsideIO := answerDocIOWindowObservationRecords(ctx, filtered)
	filtered, outsideSelected := answerDocSelectedWindowObservationRecords(ctx, filtered)
	filteredLedger := ledger
	filteredLedger.Records = filtered
	candidates := answerDocObservationPresentationCandidates(ctx, filteredLedger)
	selected := answerDocObservationPromptRecords(ctx, filtered, answerDocObservationLedgerPromptLimit)
	ranked := types.PrioritizeObservationRecords(candidates, &ctx.AnalysisIR.RequestModel, &ctx.AnalysisIR.AnswerContract, len(candidates))
	var native types.ObservationRecord
	for _, record := range ledger.Records {
		if record.Predicate == "runnable_context" && record.Subject == "app-main-100" {
			native = record
		}
	}
	if native.ID == "" || native.SourceRef.QueryScopeID == "" || native.SourceRef.PayloadRef == "" || native.Object != "insufficient_signal" {
		t.Fatalf("native background lost receipt or acquired local pressure: %+v", native)
	}
	rank := -1
	for i, record := range ranked {
		if record.ID == native.ID {
			rank = i + 1
		}
	}
	for _, row := range selected {
		if row.ID == native.ID {
			t.Fatal("fixture no longer exercises legitimate background budget omission")
		}
	}
	if rank <= answerDocObservationLedgerPromptLimit || len(selected) != answerDocObservationLedgerPromptLimit {
		t.Fatalf("row was lost before bounded ranking: rank=%d selected=%d", rank, len(selected))
	}
	t.Logf("ledger=%d after_filters=%d pretriage=%d separate_io=%d outside_io=%d outside_selected=%d candidates=%d selected=%d native_rank=%d native_id=%s", len(ledger.Records), len(filtered), pretriage, len(separateIO), outsideIO, outsideSelected, len(candidates), len(selected), rank, native.ID)
	for _, want := range []string{"query_window=0.998000..1.052000", "wait_interval=[1.045000,1.046000)", "query_window_other_cpu_idle="} {
		if !strings.Contains(native.Summary, want) {
			t.Fatalf("original accepted fact lost %q: %s", want, native.Summary)
		}
	}
	// Stored observation summaries are bounded. Full scope metadata belongs to
	// source-bound JSON; displayed values retain their own range annotations.
	originalFound := false
	for _, result := range ctx.Mutable.DispatchToolResults() {
		for _, observation := range result.Observations {
			if observation.ID == native.ID {
				originalFound = strings.Contains(strings.Join(observation.RichNotes, "\n"), "other_cpu_idle=37.500cpu_ms (query-window background 0.998000..1.052000; not wait-interval)")
			}
		}
	}
	payloadBefore, err := os.ReadFile(native.SourceRef.PayloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(payloadBefore, &payload); err != nil {
		t.Fatal(err)
	}
	scopeFound := false
	if payload.WindowStats != nil {
		for _, context := range payload.WindowStats.RunnableContext {
			if context.Thread.PID == 100 && context.ContextScope != nil {
				scope := context.ContextScope
				scopeFound = scope.QueryWindowKnown && math.Abs(scope.QueryWindow.StartTs-.998) < 1e-9 && math.Abs(scope.QueryWindow.EndTs-1.052) < 1e-9 &&
					math.Abs(scope.WaitInterval.StartTs-1.045) < 1e-9 && math.Abs(scope.WaitInterval.EndTs-1.046) < 1e-9 &&
					len(scope.QueryWindowFields) == 5 && scope.OtherCPUIdleUnit == "cpu_ms"
			}
		}
	}
	if !originalFound || !scopeFound {
		t.Fatalf("bounded display lost the complete source annotation=%t or native payload=%t", originalFound, scopeFound)
	}
	for _, language := range []string{"zh", "en"} {
		ctx.Language, ctx.AnalysisIR.RequestModel.Language = language, language
		prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
		if !strings.Contains(prompt, "showing 32 prioritized record(s)") || !strings.Contains(prompt, "not absent facts") {
			t.Fatalf("%s legitimate bounded preview omitted its boundary", language)
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("finalizer handoff mutated accepted native facts")
	}
	payloadAfter, err := os.ReadFile(native.SourceRef.PayloadRef)
	if err != nil || string(payloadBefore) != string(payloadAfter) {
		t.Fatal("finalizer handoff mutated original native payload")
	}
}

// A single, publicly queried target leaves enough room for the native context.
// No accepted observation is removed or promoted to manufacture selection.
func TestHMC307SchedulerScopeActualFinalizerHandoff(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh",
		AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Describe the queried scheduler measurements and their scopes"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: "zh", PerfTrace: &types.PerfBundle{},
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		}},
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 100,
		"time_start": .999, "time_end": 1.051, "trace_flavor": "harmony_hitrace"})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("public target query failed: %v %s", err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	ledger := answerDocObservationLedger(ctx)
	selected := answerDocObservationPromptRecords(ctx, ledger.Records, answerDocObservationLedgerPromptLimit)
	var row types.ObservationPromptRecord
	for _, candidate := range selected {
		if strings.Contains(candidate.ID, "#runnable_context:") {
			row = candidate
		}
	}
	t.Logf("single_target_ledger=%d selected=%d runnable_context=%+v", len(ledger.Records), len(selected), row)
	if row.ID == "" {
		t.Fatal("precise public query did not naturally select its native context")
	}
	for _, language := range []string{"zh", "en"} {
		ctx.Language, ctx.AnalysisIR.RequestModel.Language = language, language
		prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
		t.Logf("language=%s actual_initial_instruction:\n%s", language, prompt)
		for _, want := range []string{row.ID, "same_cpu_busy=7.000ms (query-window background 0.999000..1.051000; not wait-interval)", "same_cpu_idle=45.000ms (query-window background 0.999000..1.051000; not wait-interval)", "other_cpu_idle=37.500cpu_ms (query-window background 0.999000..1.051000; not wait-interval)", "high_prio_running=7.000ms (query-window background 0.999000..1.051000; not wait-interval)", "system_or_kernel_running=0.000ms (query-window background 0.999000..1.051000; not wait-interval)"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s selected native context lost %q", language, want)
			}
		}
	}
}
