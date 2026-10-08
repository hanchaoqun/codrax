package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceCatalogDefaultRegistrationAndInitialExplorerSurface(t *testing.T) {
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	catalog, err := registry.Get("trace_catalog")
	if err != nil || catalog.IsWrite() || catalog.Confidence() != 0 {
		t.Fatalf("catalog must be registered and navigation-only: %v %+v", err, catalog)
	}
	for _, attached := range []bool{false, true} {
		ctx := ctxbuilder.BuildAgentContext(traceCapabilitiesDiscoveryBus(t, attached), types.AgentExplorer, types.StageExplore)
		capture := &traceTeachingCaptureLLM{stop: errors.New("captured directory discovery surface")}
		explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
		_, err := explorer.Execute(ctx, traceTeachingSkill(t, "explore-skill"))
		if !errors.Is(err, capture.stop) || capture.calls != 1 {
			t.Fatalf("actual initial request not captured: %v", err)
		}
		count := 0
		for _, schema := range capture.tools {
			if schema.Name == "trace_catalog" {
				count++
				if !json.Valid(schema.Parameters) || strings.Contains(schema.Description, "[high-confidence evidence]") {
					t.Fatalf("catalog schema/evidence boundary: %+v", schema)
				}
			}
		}
		if count != 1 {
			t.Fatalf("attached=%t catalog offered %d times", attached, count)
		}
	}
}

func TestTraceCatalogDoesNotExpandAnalyzerTools(t *testing.T) {
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	ctx := ctxbuilder.BuildAgentContext(traceCapabilitiesDiscoveryBus(t, false), types.AgentAnalyzer, types.StageAnalyze)
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured classifier")}
	analyzer := NewAnalyzerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
	_, err := analyzer.Execute(ctx, traceTeachingSkill(t, "analysis-skill"))
	if !errors.Is(err, capture.stop) {
		t.Fatal(err)
	}
	for _, schema := range capture.tools {
		if schema.Name == "trace_catalog" {
			t.Fatal("discovery expanded classification rights")
		}
	}
}

type traceCatalogSequenceLLM struct {
	traceTeachingCaptureLLM
	ctx          *types.AgentContext
	path         string
	attached     bool
	beforeNative []types.ToolResult
}

func (l *traceCatalogSequenceLLM) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	switch l.calls {
	case 1:
		params, _ := json.Marshal(map[string]any{"action": "discover", "root": "."})
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "discover", Name: "trace_catalog", Params: params}}, StopReason: "tool_use"}, nil
	case 2:
		l.beforeNative = l.ctx.Mutable.DispatchToolResults()
		if l.ctx.Mutable.TraceQueryRuntimeObservationCount() != 0 || traceQueryToolAvailable(l.ctx) != l.attached {
			return llm.Response{}, errors.New("discovery changed native evidence authority")
		}
		visible := false
		for _, schema := range schemas {
			visible = visible || schema.Name == "trace_query"
		}
		if !visible {
			return llm.Response{}, errors.New("directory discovery did not expose native query")
		}
		params, _ := json.Marshal(map[string]any{"source": "path", "path": l.path, "view": "window_stats", "pid": 42, "time_start": 1, "time_end": 1.02})
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "native", Name: "trace_query", Params: params}}, StopReason: "tool_use"}, nil
	default:
		l.messages, l.tools = append([]llm.Message(nil), messages...), append([]llm.ToolSchema(nil), schemas...)
		return llm.Response{}, l.stop
	}
}

func TestTraceCatalogActualDiscoveryThenNativeQuery(t *testing.T) {
	for _, attached := range []bool{false, true} {
		t.Run(map[bool]string{false: "directory_only", true: "attached_probe_duty"}[attached], func(t *testing.T) {
			registry := toolpkg.NewRegistry()
			toolpkg.RegisterDefaults(registry)
			bus := traceCapabilitiesDiscoveryBus(t, attached)
			body := "# tracer: nop\napp-42 (42) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120\napp-42 (42) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
			path := filepath.Join(bus.RepoRoot, "capture.systrace")
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if attached {
				bus.AttachedHitrace = body
			}
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
			capture := &traceCatalogSequenceLLM{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured discovery then query")}, ctx: ctx, path: path, attached: attached}
			explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 3})
			out, err := explorer.Execute(ctx, traceTeachingSkill(t, "explore-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 3 {
				t.Fatalf("actual loop: calls=%d err=%v", capture.calls, err)
			}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: capture.beforeNative})
			if len(ledger.Records) != 0 || len(ctx.Mutable.TraceCatalogs()) != 1 {
				t.Fatalf("discovery evidence/catalog boundary: %+v", ledger)
			}
			found := false
			for _, result := range ctx.Mutable.DispatchToolResults() {
				if result.ToolName == "trace_catalog" && !result.Success {
					t.Fatalf("discover: %+v", result)
				}
				if result.ToolName == "trace_query" {
					found = result.Success && len(result.Observations) > 0
				}
			}
			if !found || ctx.Mutable.TraceQueryRuntimeObservationCount() == 0 {
				t.Fatal("normal native execution did not provide evidence")
			}
			if out != nil {
				for _, fact := range out.NewFacts {
					if fact.Key == "trace_catalog" {
						t.Fatal("discovery became source evidence")
					}
				}
			}
		})
	}
}
