package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceCapabilitiesDiscoveryBus(t *testing.T, attached bool) *types.BusContext {
	t.Helper()
	root := t.TempDir()
	bus := &types.BusContext{
		RepoRoot: root, WorkDir: root,
		Mutable: types.NewMutableState("Identify the available analysis capabilities for a trace investigation."),
	}
	if attached {
		runtime := traceTeachingRuntimeContext(types.StageExplore)
		bus.AttachedHitrace = runtime.AttachedHitrace
		bus.AnalysisIR = runtime.AnalysisIR
	}
	return bus
}

type traceCapabilitiesSequenceLLM struct {
	traceTeachingCaptureLLM
	ctx                          *types.AgentContext
	resultsBeforeQuery           []types.ToolResult
	hardQueryDutyBeforeQuery     bool
	runtimeCountBeforeQuery      int
	completionBlockedBeforeQuery bool
}

func (l *traceCapabilitiesSequenceLLM) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	call := llm.ToolCall{}
	switch l.calls {
	case 1:
		call = llm.ToolCall{ID: "catalog", Name: "trace_capabilities", Params: json.RawMessage(`{}`)}
	case 2:
		l.hardQueryDutyBeforeQuery = runtimeSourceNavigationPhaseForExplorer(l.ctx, true).RuntimeProbeHardRequired
		l.runtimeCountBeforeQuery = l.ctx.Mutable.TraceQueryRuntimeObservationCount()
		completion := validateExplorerTraceQueryFirstToolCall(l.ctx, llm.ToolCall{Name: "emit_investigation_complete", Params: json.RawMessage(`{}`)}, true)
		l.completionBlockedBeforeQuery = completion != nil && !completion.Success && completion.Repair != nil && completion.Repair.Code == explorerTraceQueryFirstCode
		call = llm.ToolCall{ID: "other-non-evidence", Name: "list_memory", Params: json.RawMessage(`{}`)}
	case 3:
		l.resultsBeforeQuery = l.ctx.Mutable.DispatchToolResults()
		call = llm.ToolCall{ID: "native-query", Name: "trace_query", Params: json.RawMessage(`{"source":"attached_trace","view":"window_stats","pid":42,"time_start":1,"time_end":1.02}`)}
	default:
		l.messages = append([]llm.Message(nil), messages...)
		l.tools = append([]llm.ToolSchema(nil), schemas...)
		return llm.Response{}, l.stop
	}
	return llm.Response{ToolCalls: []llm.ToolCall{call}, StopReason: "tool_use"}, nil
}

func TestTraceCapabilitiesDiscoveryActualCatalogThenNativeQuery(t *testing.T) {
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	bus := traceCapabilitiesDiscoveryBus(t, true)
	bus.AttachedHitrace = "# tracer: nop\n" +
		"app-42 (42) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120\n" +
		"app-42 (42) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	if !runtimeSourceNavigationPhaseForExplorer(ctx, true).RuntimeProbeHardRequired {
		t.Fatal("fixture must start with a required native trace probe")
	}
	capture := &traceCapabilitiesSequenceLLM{
		traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured catalog then native query")}, ctx: ctx,
	}
	explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 4})
	out, err := explorer.Execute(ctx, traceTeachingSkill(t, "explore-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 4 {
		t.Fatalf("real dispatch did not execute the bounded three-call sequence: calls=%d err=%v", capture.calls, err)
	}
	results := map[string]types.ToolResult{}
	for _, result := range ctx.Mutable.DispatchToolResults() {
		results[result.ToolName] = result
	}
	if catalog := results["trace_capabilities"]; !catalog.Success {
		t.Errorf("non-evidence catalog must execute before the first runtime probe: %+v", catalog)
	}
	other := results["list_memory"]
	if other.Success || other.Repair == nil || other.Repair.Code != explorerTraceQueryFirstCode || !capture.completionBlockedBeforeQuery {
		t.Errorf("catalog must not grant completion or release other non-evidence tools: other=%+v completion_blocked=%t", other, capture.completionBlockedBeforeQuery)
	}
	if !capture.hardQueryDutyBeforeQuery || capture.runtimeCountBeforeQuery != 0 {
		t.Errorf("catalog consumed the native-query duty or invented runtime authority: required=%t count=%d", capture.hardQueryDutyBeforeQuery, capture.runtimeCountBeforeQuery)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: capture.resultsBeforeQuery})
	if len(ledger.Records) != 0 {
		t.Errorf("capability descriptions became observations before native query: %+v", ledger.Records)
	}
	if native := results["trace_query"]; !native.Success || len(native.Observations) == 0 || ctx.Mutable.TraceQueryRuntimeObservationCount() == 0 {
		t.Errorf("real native trace_query did not supply the subsequent evidence: %+v", native)
	}
	if runtimeSourceNavigationPhaseForExplorer(ctx, true).RuntimeProbeHardRequired {
		t.Error("completed native query did not release its own probe obligation")
	}
	if out == nil {
		t.Fatal("actual explorer did not preserve its partial output for audit")
	}
	for _, fact := range out.NewFacts {
		if fact.Key == "trace_capabilities" {
			t.Errorf("non-evidence capability catalog became a repository fact: %+v", fact)
		}
	}
}

func TestTraceCapabilitiesDiscoveryDoesNotBypassTerminalInputRepair(t *testing.T) {
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	ctx := ctxbuilder.BuildAgentContext(traceCapabilitiesDiscoveryBus(t, true), types.AgentExplorer, types.StageExplore)
	armTraceQueryTerminalAdmissionForTest(t, ctx.Mutable, tracequery.TraceInputAdmissionCodeConversionRequired, false)
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: registry}, &stubEvaluator{})
	result, _ := base.executeTool(ctx, llm.ToolCall{Name: "trace_capabilities", Params: json.RawMessage(`{}`)}, map[string]bool{"trace_query": true, "trace_capabilities": true})
	if result == nil || result.Success || result.Repair == nil || result.Repair.Metadata["policy"] != "trace_input_admission_terminal" {
		t.Fatalf("metadata exception must not bypass an armed terminal input repair: %+v", result)
	}
}

func TestTraceCapabilitiesDiscoveryActualExplorerInitialRequest(t *testing.T) {
	for _, attached := range []bool{false, true} {
		t.Run(map[bool]string{false: "without_attachment", true: "with_attachment"}[attached], func(t *testing.T) {
			registry := toolpkg.NewRegistry()
			toolpkg.RegisterDefaults(registry)
			ctx := ctxbuilder.BuildAgentContext(traceCapabilitiesDiscoveryBus(t, attached), types.AgentExplorer, types.StageExplore)
			sk := traceTeachingSkill(t, "explore-skill")
			listed := false
			for _, name := range sk.ToolSuggestions {
				listed = listed || name == "trace_capabilities"
			}
			if !listed {
				t.Error("default exploration skill does not make the capability catalog discoverable")
			}
			capture := &traceTeachingCaptureLLM{stop: errors.New("captured capability discovery request")}
			explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
			_, err := explorer.Execute(ctx, sk)
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("did not capture the actual initial request: calls=%d err=%v", capture.calls, err)
			}
			count := 0
			for _, schema := range capture.tools {
				if schema.Name != "trace_capabilities" {
					continue
				}
				count++
				if !json.Valid(schema.Parameters) || strings.TrimSpace(schema.Description) == "" {
					t.Fatalf("offered catalog lacks usable JSON schema/description: %+v", schema)
				}
				if strings.Contains(schema.Description, "[high-confidence evidence]") {
					t.Fatal("capability catalog was advertised as trace evidence")
				}
			}
			if count != 1 {
				t.Fatalf("actual initial tool schemas expose trace_capabilities %d times, want exactly once", count)
			}
		})
	}
}

func TestTraceCapabilitiesDiscoveryDoesNotExpandAnalyzerSurface(t *testing.T) {
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	bus := traceCapabilitiesDiscoveryBus(t, true)
	bus.AnalysisIR = nil // classification has not emitted its request model yet
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentAnalyzer, types.StageAnalyze)
	sk := traceTeachingSkill(t, "analysis-skill")
	for _, name := range sk.ToolSuggestions {
		if name == "trace_capabilities" {
			t.Fatal("classification skill gained a new exploration tool")
		}
	}
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured classification surface")}
	analyzer := NewAnalyzerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
	_, err := analyzer.Execute(ctx, sk)
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("did not capture the actual analyzer request: calls=%d err=%v", capture.calls, err)
	}
	for _, schema := range capture.tools {
		if schema.Name == "trace_capabilities" {
			t.Fatal("capability discovery expanded the analyzer's actual tool rights")
		}
	}
}

func TestTraceCapabilitiesDiscoveryDefaultRegistrationIsReadOnlyNonEvidence(t *testing.T) {
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	catalog, err := registry.Get("trace_capabilities")
	if err != nil {
		t.Fatalf("catalog is not registered by the real default registry: %v", err)
	}
	if catalog.IsWrite() || catalog.Confidence() != 0 {
		t.Fatalf("catalog must not mutate files or carry evidence authority: write=%t confidence=%v", catalog.IsWrite(), catalog.Confidence())
	}
}
