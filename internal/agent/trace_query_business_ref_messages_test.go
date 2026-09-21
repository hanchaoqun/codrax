package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Capture the actual explorer-to-adapter request: a property present only in
// the public tool schema is not discoverable if dispatch projection drops it.
func TestTraceQueryBusinessRefActualExplorerSchema(t *testing.T) {
	ctx := traceTeachingRuntimeContext(types.StageExplore)
	reg := toolpkg.NewRegistry()
	query := &toolpkg.TraceQuery{}
	reg.Register(query)
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured business-reference schema")}
	explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
	_, err := explorer.Execute(ctx, traceTeachingSkill(t, "explore-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("did not capture exactly the initial adapter request: calls=%d err=%v", capture.calls, err)
	}

	type property struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	}
	type schema struct {
		Properties map[string]property `json:"properties"`
		Required   []string            `json:"required"`
	}
	var public schema
	if err := json.Unmarshal(query.Parameters(), &public); err != nil {
		t.Fatal(err)
	}
	for _, offered := range capture.tools {
		if offered.Name != query.Name() {
			continue
		}
		var actual schema
		if err := json.Unmarshal(offered.Parameters, &actual); err != nil {
			t.Fatal(err)
		}
		ref, ok := actual.Properties["business_span_ref"]
		if !ok || ref.Type != "string" || ref.Description == "" {
			t.Fatalf("actual explorer schema lost the optional instance reference: %+v", ref)
		}
		if ref != public.Properties["business_span_ref"] {
			t.Fatalf("actual adapter schema changed the public reference contract: actual=%+v public=%+v", ref, public.Properties["business_span_ref"])
		}
		for _, required := range actual.Required {
			if required == "business_span_ref" {
				t.Fatal("navigation shortcut became mandatory for ordinary explicit queries")
			}
		}
		for _, want := range []string{
			"this run", "never pick the first/longest", "original physical capture generation",
			"scheduler TID", "complete paired time interval",
			"coordinate fields are assertions and must exactly match that same current physical instance",
			"they never override it or apply additional filters", "Conflicting fields are rejected, not silently removed",
			"navigation only", "does not accept a completion focus or prove a root cause",
			"Explicit requested time windows remain authoritative",
			"ordinary explicit parameters for the requested clipped window",
			"Async/track rows without an executing-thread proof", "composite captures", "stale/replayed references",
		} {
			if !strings.Contains(ref.Description, want) {
				t.Errorf("actual adapter reference teaching lost boundary %q", want)
			}
		}
		assertTraceBusinessRefCompletionBridge(t, ref.Description)
		return
	}
	t.Fatal("actual explorer request did not offer trace_query")
}

type traceBusinessRefRoundTripLLM struct {
	traceTeachingCaptureLLM
	discovery llm.ToolCall
}

func (l *traceBusinessRefRoundTripLLM) Chat(_ context.Context, messages []llm.Message, offered []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		return llm.Response{ToolCalls: []llm.ToolCall{l.discovery}, StopReason: "tool_use"}, nil
	}
	l.messages = append([]llm.Message(nil), messages...)
	l.tools = append([]llm.ToolSchema(nil), offered...)
	return llm.Response{}, l.stop
}

// Exercise the production explorer dispatch and its next adapter request,
// not a prebuilt ToolResult: debug-log truncation must not hide a published
// navigation receipt from the model's actual tool message.
func TestTraceQueryBusinessRefActualExplorerToolMessage(t *testing.T) {
	for _, view := range []string{"span_window", "root_cause_rank", "trace_perf_bundle"} {
		t.Run(view, func(t *testing.T) { traceQueryBusinessRefActualExplorerToolMessage(t, view) })
	}
}

func traceQueryBusinessRefActualExplorerToolMessage(t *testing.T, view string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "business.systrace")
	trace := "# tracer: nop\n" +
		"worker-200 (100) [001] .... 1.000000: tracing_mark_write: B|100|LoadReport\n" +
		"worker-200 (100) [001] .... 1.050000: tracing_mark_write: E|100\n"
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := traceTeachingRuntimeContext(types.StageExplore)
	ctx.Objective = "Locate the complete LoadReport instance in the attached trace."
	ctx.Mutable = types.NewMutableState(ctx.Objective)
	ctx.RepoRoot, ctx.WorkDir, ctx.AttachedHitrace = root, root, path
	p := map[string]any{"view": view, "source": "path", "path": path}
	if view == "span_window" {
		p["span_name"] = "LoadReport"
	} else {
		// No span-name side output: the native rank/bundle stats alone
		// must carry the complete pair into the actual next model message.
		p["time_start"], p["time_end"] = 1, 1.051
	}
	params, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	capture := &traceBusinessRefRoundTripLLM{
		traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured post-discovery model request")},
		discovery:               llm.ToolCall{ID: "business-instance-discovery", Name: "trace_query", Params: params},
	}
	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.TraceQuery{})
	explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 2})
	_, err = explorer.Execute(ctx, traceTeachingSkill(t, "explore-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 2 {
		t.Fatalf("did not capture the adapter request after real discovery: calls=%d err=%v", capture.calls, err)
	}
	var content string
	for _, message := range capture.messages {
		if message.Role == "tool" && message.ToolCallID == capture.discovery.ID {
			if content != "" {
				t.Fatal("discovery unexpectedly produced duplicate tool messages")
			}
			content = message.Content
		}
	}
	match := regexp.MustCompile(`business_span_ref="(business-span:[0-9a-f]+)"`).FindStringSubmatch(content)
	if len(match) != 2 {
		t.Fatalf("actual next model request lost the native business reference: %s", content)
	}
	for _, want := range []string{
		`work="LoadReport"`, `thread="worker"`, "tid=200", fmt.Sprintf("source=%q", physical),
		"lines=2-3", "complete_window=1.000000000..1.050000000 seconds",
		"not a complete inventory, a causal proof or an accepted completion focus",
		"not the first or longest", "omit copied source, thread and window fields",
		"Explicit requested windows still govern",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("actual model tool message lost exact instance field or boundary %q", want)
		}
	}
	assertTraceBusinessRefCompletionBridge(t, content)
	ref, ok := ctx.Mutable.ResolveTraceBusinessSpanRef(match[1])
	if !ok {
		t.Fatal("reference shown to the model was not registered for this run")
	}
	d := ref.Data()
	if d.Path != physical || d.Name != "LoadReport" || d.Thread != "worker" || d.TID != 200 || d.Kind != "sync" ||
		d.StartLine != 2 || d.EndLine != 3 || d.StartTs != 1 || d.EndTs != 1.05 {
		t.Fatalf("model-visible token resolves to a different physical instance: %+v", d)
	}
	t.Logf("actual tool message bytes=%d; native reference byte offset=%d; registered tuple preserved", len(content), strings.Index(content, match[0]))
}

func assertTraceBusinessRefCompletionBridge(t *testing.T, surface string) {
	t.Helper()
	const want = "Using this reference in trace_query is navigation only; it does not accept a completion focus or prove a root cause. To select that exact instance for automatic supplementation, separately copy the published token into the optional top-level emit_investigation_complete.business_span_ref field. Selection takes effect only after the completion is accepted and its exploration dispatch succeeds."
	if got := strings.Count(surface, want); got != 1 {
		t.Errorf("actual adapter surface must carry the shared query-to-completion teaching exactly once; got %d", got)
	}
}

type traceBusinessRefAssertionsLLM struct{ traceTeachingCaptureLLM }

func (l *traceBusinessRefAssertionsLLM) Chat(_ context.Context, messages []llm.Message, offered []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "discover-instance", Name: "trace_query", Params: json.RawMessage(`{"source":"attached_trace","view":"span_window","span_name":"OpenDocument"}`)}}, StopReason: "tool_use"}, nil
	}
	if l.calls == 2 {
		var content string
		for _, m := range messages {
			if m.Role == "tool" && m.ToolCallID == "discover-instance" {
				content = m.Content
			}
		}
		matches := regexp.MustCompile(`business_span_ref="(business-span:[0-9a-f]+)"`).FindAllStringSubmatch(content, -1)
		if len(matches) != 1 {
			return llm.Response{}, fmt.Errorf("fixture needs exactly one published instance, got %d", len(matches))
		}
		params, _ := json.Marshal(map[string]any{"view": "window_stats", "business_span_ref": matches[0][1], "source": "attached_trace", "thread": "app-main", "pid": 100})
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "measure-instance", Name: "trace_query", Params: params}}, StopReason: "tool_use"}, nil
	}
	l.messages = append([]llm.Message(nil), messages...)
	l.tools = append([]llm.ToolSchema(nil), offered...)
	return llm.Response{}, l.stop
}

func TestTraceQueryBusinessRefAssertionsActualExplorerDispatch(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	ctx := traceTeachingRuntimeContext(types.StageExplore)
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	ctx.AttachedTraceMaterial, ctx.AttachedHitrace = m, m.Preview()
	ctx.Mutable = types.NewMutableState("measure selected business operation")
	capture := &traceBusinessRefAssertionsLLM{traceTeachingCaptureLLM{stop: errors.New("captured post-assertion query message")}}
	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.TraceQuery{})
	explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 3})
	_, err = explorer.Execute(ctx, traceTeachingSkill(t, "explore-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 3 {
		t.Fatalf("did not reach post-query adapter: %v calls=%d", err, capture.calls)
	}
	var content string
	for _, message := range capture.messages {
		if message.Role == "tool" && message.ToolCallID == "measure-instance" {
			content = message.Content
		}
	}
	if content == "" || strings.Contains(content, "coordinate assertions conflict") {
		t.Fatalf("actual adapter got a rejection: %s", content)
	}
	measured := false
	for _, result := range ctx.Mutable.DispatchToolResults() {
		if !result.Success {
			t.Fatalf("unexpected dispatch rejection: %s", result.Summary)
		}
		for _, obs := range result.Observations {
			if obs.SourceRef.PayloadRef == "" {
				continue
			}
			data, err := os.ReadFile(obs.SourceRef.PayloadRef)
			if err != nil {
				t.Fatal(err)
			}
			var native tracequery.Result
			if err := json.Unmarshal(data, &native); err != nil {
				t.Fatal(err)
			}
			if native.WindowStats != nil {
				measured = true
				if native.WindowStats.Window.StartTs != 1 || native.WindowStats.Window.EndTs != 1.05 || len(native.WindowStats.IOLatencies) != 2 {
					t.Fatalf("public dispatch lost exact instance account: %+v", native.WindowStats)
				}
			}
		}
	}
	if !measured {
		t.Fatal("no native measured account reached dispatch ledger")
	}
	if status, ref := ctx.Mutable.AcceptedTraceBusinessFocus(); status == types.TraceBusinessFocusSelected || ref.Token() != "" {
		t.Fatal("successful query accepted a completion focus")
	}
}
