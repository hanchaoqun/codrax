package agent

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The first real explorer request must distinguish selecting an accepted
// completion focus from merely navigating with the same published token.
// Public schema tests alone would not catch a dispatch projection dropping
// or rewriting either contract before it reaches the model.
func TestCompletionBusinessFocusActualExplorerSchema(t *testing.T) {
	ctx := traceTeachingRuntimeContext(types.StageExplore)
	completion := &toolpkg.EmitInvestigationComplete{}
	query := &toolpkg.TraceQuery{}
	reg := toolpkg.NewRegistry()
	reg.Register(completion)
	reg.Register(query)
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured initial business-focus teaching")}
	explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
	sk := traceTeachingSkill(t, "explore-skill")
	// cmd/root.go adds the structured emitters to the default explore skill
	// during bootstrap; reproduce that wiring, not a hand-built tool schema.
	sk.ToolSuggestions = append(sk.ToolSuggestions, "emit_evidence", completion.Name())
	_, err := explorer.Execute(ctx, sk)
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
	for _, tc := range []struct {
		name       string
		parameters json.RawMessage
		boundaries []string
	}{
		{
			name: completion.Name(), parameters: completion.Parameters(),
			boundaries: []string{
				"OPTIONAL", "currently published business-span token from trace_query",
				"that exact complete business instance",
				"physical source, thread and full time window travel together",
				"only after this completion and its exploration dispatch succeed",
				"does not prove a causal relation or elect a root cause",
				"explicit user scope and target remain authoritative",
				"this top-level field", "never reconstruct it from prose or repeat its coordinates",
				"Omit when no single instance is selected",
				"a new accepted completion without this field clears any previous selection",
			},
		},
		{
			name: query.Name(), parameters: query.Parameters(),
			boundaries: []string{
				"this run", "never pick the first/longest",
				"original physical capture generation", "scheduler TID", "complete paired time interval",
				"coordinate fields are assertions and must exactly match that same current physical instance",
				"they never override it or apply additional filters", "Conflicting fields are rejected, not silently removed",
				"navigation only", "does not accept a completion focus or prove a root cause",
				"Explicit requested time windows remain authoritative",
				"ordinary explicit parameters for the requested clipped window",
				"Async/track rows without an executing-thread proof", "composite captures", "stale/replayed references",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var public schema
			if err := json.Unmarshal(tc.parameters, &public); err != nil {
				t.Fatal(err)
			}
			var actual schema
			found := false
			for _, offered := range capture.tools {
				if offered.Name != tc.name {
					continue
				}
				if found {
					t.Fatalf("initial adapter request duplicated %s", tc.name)
				}
				found = true
				if err := json.Unmarshal(offered.Parameters, &actual); err != nil {
					t.Fatal(err)
				}
			}
			if !found {
				t.Fatalf("initial explorer request did not offer %s", tc.name)
			}
			ref, ok := actual.Properties["business_span_ref"]
			if !ok || ref.Type != "string" || ref.Description == "" {
				t.Fatalf("actual adapter schema lost optional reference teaching: %+v", ref)
			}
			if ref != public.Properties["business_span_ref"] {
				t.Fatalf("adapter must retain public reference teaching verbatim: actual=%+v public=%+v", ref, public.Properties["business_span_ref"])
			}
			if !reflect.DeepEqual(actual.Required, public.Required) {
				t.Fatalf("dispatch changed required fields: actual=%v public=%v", actual.Required, public.Required)
			}
			for _, required := range actual.Required {
				if required == "business_span_ref" {
					t.Fatal("optional instance selection became mandatory")
				}
			}
			for _, want := range tc.boundaries {
				if !strings.Contains(ref.Description, want) {
					t.Errorf("actual adapter teaching lost boundary %q", want)
				}
			}
		})
	}
}
