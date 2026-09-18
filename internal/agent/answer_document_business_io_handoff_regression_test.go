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

// Replays the four public queries actually used by the business/IO eval.
// This is a deterministic producer-to-finalizer test, not an LLM replay.
func hmosBusinessIOFinalizerContext(t *testing.T) *types.AgentContext {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh",
		AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Explain the observed business response and its dependency waits"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: "zh", Intent: types.IntentRootCause,
			PerfTrace:     &types.PerfBundle{},
			AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
				Scope: types.RuntimeQuestionScopeCausalDiagnosis, RuntimeWorkRelationRequested: true,
			},
		}},
	}
	for _, query := range []struct{ view, thread string }{
		{"thread_timeline", "app-main"}, {"thread_timeline", "document-worker"},
		{"wakeup_chain", "app-main"}, {"window_stats", ""},
	} {
		params, err := json.Marshal(map[string]any{
			"source": "path", "path": path, "view": query.view, "thread": query.thread,
			"time_start": 0.998, "time_end": 1.052, "trace_flavor": "harmony_hitrace",
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !result.Success {
			t.Fatalf("public query %s failed: %v %s", query.view, err, result.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(result)
		if query.view != "window_stats" {
			continue
		}
		data, err := os.ReadFile(result.RawRef)
		if err != nil {
			t.Fatal(err)
		}
		var payload tracequery.Result
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.WindowStats == nil || len(payload.WindowStats.TraceSpans) != 2 {
			t.Fatalf("producer must already have both business spans: %+v", payload.WindowStats)
		}
		closed := false
		for _, io := range payload.WindowStats.IOLatencies {
			if io.IssueThread.PID == 200 && io.CompletionWokeIssuer &&
				math.Abs(io.DurationMs-35) < 1e-6 && math.Abs(io.IssuerBlockedMs-31) < 1e-6 {
				closed = true
			}
		}
		if !closed {
			t.Fatal("producer must already have the distinct request and completion-closed wait rulers")
		}
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	return ctx
}

func TestHmosBusinessIOExistingSpansReachFinalizerWorkSelection(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	input := types.ObservationLedgerInputFromAgentContext(ctx, 64)
	contract := types.BuildRuntimeWorkRelationContract(input, true)
	if contract == nil {
		t.Fatal("ordinary measured business spans were present in public query results but the work relation contract reports no observations")
	}
	for _, want := range []struct {
		name string
		ms   float64
	}{{"OpenDocument", 50}, {"LoadDocumentIndex", 40}} {
		found := false
		for _, row := range contract.Rows {
			if row.WorkLabel == want.name && math.Abs(row.MeasuredDurationMS-want.ms) < 1e-6 {
				found = true
			}
		}
		if !found {
			t.Errorf("measured business span %s %.3fms lost before finalizer: %+v", want.name, want.ms, contract.Rows)
		}
	}
}

func TestHmosBusinessIOExistingRequestRulersReachCausalFinalizer(t *testing.T) {
	ctx := hmosBusinessIOFinalizerContext(t)
	ledger := answerDocObservationLedger(ctx)
	closed := false
	for _, row := range ledger.Records {
		if row.Predicate == "io_latency" && row.Subject == "document-worker-200" && row.Value == "35.000" &&
			strings.Contains(strings.Join(row.RichNotes, "\n"), "issuer_blocked=31.000") {
			closed = true
		}
	}
	if !closed {
		t.Fatal("premise failed: exact request/wait pair must already exist on the typed ledger")
	}
	// Allow either a dedicated measurement handoff or the existing compact
	// ledger to carry the facts. Do not require a particular prompt section,
	// new root-cause election, prose normalizer, or additional query.
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, facts := range [][]string{
		{"document-worker-200", "request_residence=`35.000`", "issuer_blocked=`31.000`", "completion_woke_issuer=`true`", "trace_query:"},
		{"backup-900", "request_residence=`47.000`", "owner_scope=`selected_window_context`", "trace_query:"},
	} {
		found := false
		for _, line := range strings.Split(prompt, "\n") {
			matches := true
			for _, fact := range facts {
				matches = matches && strings.Contains(line, fact)
			}
			found = found || matches
		}
		if !found {
			t.Errorf("causal finalizer lost a source-bound IO measurement row %q", facts)
		}
	}
}
