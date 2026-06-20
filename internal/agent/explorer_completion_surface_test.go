package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The completion-obligation dispatch narrows the explorer's tool
// schema to the emit-only completion surface, with the standard
// fail-open contract when those tools are absent from the skill set.
func TestExplorerFilterToolSchemasCompletionOnly(t *testing.T) {
	e := &explorerEvaluator{}
	schemas := []llm.ToolSchema{
		{Name: "grep"}, {Name: "read_file"},
		{Name: "emit_evidence"}, {Name: "emit_investigation_complete"},
	}
	ctx := &types.AgentContext{Stage: types.StageExplore, CompletionOnlySurface: true}
	got := e.FilterToolSchemas(ctx, schemas)
	if len(got) != 2 {
		t.Fatalf("expected emit-only surface, got %d schemas", len(got))
	}
	for _, s := range got {
		if !completionProgressToolNames[s.Name] {
			t.Fatalf("non-completion tool leaked through: %s", s.Name)
		}
	}

	// Fail-open: a schema set without the completion tools returns
	// unchanged rather than stranding the dispatch.
	bare := []llm.ToolSchema{{Name: "grep"}}
	if got := e.FilterToolSchemas(ctx, bare); len(got) != 1 || got[0].Name != "grep" {
		t.Fatalf("fail-open violated: %v", got)
	}

	// Without the flag the surface is untouched (stable scenarios
	// byte-identical).
	plain := &types.AgentContext{Stage: types.StageExplore}
	if got := e.FilterToolSchemas(plain, schemas); len(got) != 4 {
		t.Fatalf("flag-off dispatch must keep the full surface, got %d", len(got))
	}
}

func TestExplorerFilterToolSchemasCompletionOnlyAllowsTypedRepairTools(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:  types.RepairStructuredHandoff,
		Tools: []string{"repo_map"},
		Stage: string(types.StageExplore),
	})
	ctx := &types.AgentContext{Stage: types.StageExplore, CompletionOnlySurface: true, Mutable: mut}
	schemas := []llm.ToolSchema{
		{Name: "grep"}, {Name: "read_file"}, {Name: "repo_map"},
		{Name: "emit_evidence"}, {Name: "emit_investigation_complete"},
	}

	got := (&explorerEvaluator{}).FilterToolSchemas(ctx, schemas)
	names := map[string]bool{}
	for _, schema := range got {
		names[schema.Name] = true
	}
	for _, want := range []string{"repo_map", "emit_evidence", "emit_investigation_complete"} {
		if !names[want] {
			t.Fatalf("completion repair surface missing %q: %+v", want, got)
		}
	}
	for _, blocked := range []string{"grep", "read_file"} {
		if names[blocked] {
			t.Fatalf("completion repair surface should not reopen broad tool %q: %+v", blocked, got)
		}
	}
}

func TestExplorerFilterToolSchemasNoEmitEscalationAllowsTypedRepairTools(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.EvidenceClosure().AddRepair(types.RepairDirective{
		Kind:  types.RepairStructuredHandoff,
		Tools: []string{"repo_map"},
		Stage: string(types.StageExplore),
	})
	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: mut}
	schemas := []llm.ToolSchema{
		{Name: "grep"}, {Name: "read_file"}, {Name: "repo_map"},
		{Name: "emit_evidence"}, {Name: "emit_investigation_complete"},
	}
	eval := &explorerEvaluator{
		midLoopNoEmitPushSent:  true,
		midLoopNoEmitEscalated: true,
	}

	got := eval.FilterToolSchemas(ctx, schemas)
	names := map[string]bool{}
	for _, schema := range got {
		names[schema.Name] = true
	}
	for _, want := range []string{"repo_map", "emit_evidence", "emit_investigation_complete"} {
		if !names[want] {
			t.Fatalf("no-emit repair surface missing %q: %+v", want, got)
		}
	}
	for _, blocked := range []string{"grep", "read_file"} {
		if names[blocked] {
			t.Fatalf("no-emit repair surface should not reopen broad tool %q: %+v", blocked, got)
		}
	}
}
