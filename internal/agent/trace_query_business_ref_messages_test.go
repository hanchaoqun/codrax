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
			"omit source/path/pid/thread/target_scope/time_start/time_end/line_start/line_end/span_name",
			"navigation only", "not evidence of a root cause or a completion focus",
			"Explicit requested time windows remain authoritative",
			"ordinary explicit parameters for the requested clipped window",
			"Async/track rows without an executing-thread proof", "composite captures", "stale/replayed references",
		} {
			if !strings.Contains(ref.Description, want) {
				t.Errorf("actual adapter reference teaching lost boundary %q", want)
			}
		}
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
	params, err := json.Marshal(map[string]any{
		"view": "span_window", "source": "path", "path": path, "span_name": "LoadReport",
	})
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
	} {
		if !strings.Contains(content, want) {
			t.Errorf("actual model tool message lost exact instance field or boundary %q", want)
		}
	}
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
