package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

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
