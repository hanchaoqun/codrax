package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSubExplorerFactoryDoesNotStoreSharedBaseAgent(t *testing.T) {
	sub := NewSubExplorer(&Dependencies{})
	if sub == nil || sub.deps == nil {
		t.Fatal("NewSubExplorer should keep dependencies for per-run BaseAgent construction")
	}
	if _, ok := reflect.TypeOf(*sub).FieldByName("base"); ok {
		t.Fatal("SubExplorer must not store a shared BaseAgent; Run needs a fresh evaluator per request")
	}
}

func TestSubExplorerSkillExposesRepoMapNavigationOnly(t *testing.T) {
	sk := subExplorerSkill(&types.SubAgentRequest{
		Scope: []string{"src/service"},
	})
	got := map[string]bool{}
	for _, name := range sk.ToolSuggestions {
		got[name] = true
	}
	for _, want := range []string{"repo_map", "read_file", "list_files", "grep"} {
		if !got[want] {
			t.Fatalf("sub_explorer ToolSuggestions missing %q: %v", want, sk.ToolSuggestions)
		}
	}
	for _, forbidden := range []string{
		"emit_evidence",
		"emit_investigation_complete",
		"emit_analysis",
		"emit_answer_document",
		"exec_command",
		"propose_sub_agents",
	} {
		if got[forbidden] {
			t.Fatalf("sub_explorer ToolSuggestions must not expose %q: %v", forbidden, sk.ToolSuggestions)
		}
	}
	workflow := strings.Join(sk.Workflow, "\n")
	for _, want := range []string{`source_inventory`, `relation_map`} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("sub_explorer workflow should teach repo_map %s navigation: %v", want, sk.Workflow)
		}
	}
}

// TestSubExplorer_CrossRunResetOnObjectiveChange locks the F2-style
// evaluator reset behavior for callers that exercise the evaluator directly. See
// memory/project_followups_from_repl_audit.md item #1 and
// memory/project_repl_equivalence_audit.md for the original explorer
// fix this mirrors. Each SubAgentRequest is an independent scoped
// investigation, so state from a previous Run() must not leak into
// the next one's notes / evidence. The old test also checked that
// idleStreak / lastToolCount counters were reset on cross-run —
// those counters moved to LoopPolicy and are rebuilt per dispatch,
// so the evaluator no longer owns them.
func TestSubExplorer_CrossRunResetOnObjectiveChange(t *testing.T) {
	eval := &subExplorerEvaluator{
		objective:          "find registration points",
		investigationNotes: []string{"[DIRECT] stale note from prior run"},
		structuredEvidence: []types.EvidenceItem{{Summary: "stale"}},
		flowFindings:       []types.FlowFindingDigest{{ID: "stale-flow"}},
	}

	ctx := &types.AgentContext{
		Objective:   "trace config loader",
		Constraints: []string{"internal/config"},
	}
	prompt := eval.BuildInitialInstruction(ctx, nil)

	if len(eval.investigationNotes) != 0 {
		t.Errorf("investigationNotes not reset: %v", eval.investigationNotes)
	}
	if len(eval.structuredEvidence) != 0 {
		t.Errorf("structuredEvidence not reset: %v", eval.structuredEvidence)
	}
	if len(eval.flowFindings) != 0 {
		t.Errorf("flowFindings not reset: %v", eval.flowFindings)
	}
	if eval.objective != "trace config loader" {
		t.Errorf("objective not updated: %q", eval.objective)
	}
	if !strings.Contains(prompt, "trace config loader") {
		t.Errorf("prompt missing new objective: %q", prompt)
	}
}

func TestSubExplorerInitialInstructionTeachesRepoMapWithoutUnavailableTools(t *testing.T) {
	eval := &subExplorerEvaluator{}
	prompt := eval.BuildInitialInstruction(&types.AgentContext{
		Objective:   "inventory service routes",
		Constraints: []string{"src/service"},
	}, nil)
	for _, want := range []string{
		"`repo_map` as a scoped navigation index",
		`view="source_inventory"`,
		"include_attributes=false",
		"add `attribute_roles` only after choosing a narrow scope/member",
		`view="relation_map"`,
		"relation_kinds",
		"Verify selected navigation rows with `read_file` or targeted `grep",
		"verify candidate scopes/files/symbols/counts",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("sub_explorer prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"emit_evidence", "emit_investigation_complete", "exec_command", "propose_sub_agents"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("sub_explorer prompt mentions unavailable tool %q:\n%s", forbidden, prompt)
		}
	}
}

// TestSubExplorer_SameObjectiveKeepsState verifies that when the same
// objective is seen twice in a row by one evaluator instance, accumulated
// notes are NOT wiped. Run() now creates a fresh evaluator per request, but the
// evaluator contract stays useful for direct tests and any future scoped
// single-investigation reuse. structuredEvidence and flowFindings are still
// cleared unconditionally (pre-existing behaviour — they're rebuilt from tool
// results each Execute).
func TestSubExplorer_SameObjectiveKeepsState(t *testing.T) {
	eval := &subExplorerEvaluator{
		objective:          "trace config loader",
		investigationNotes: []string{"[DIRECT] keep me"},
	}

	ctx := &types.AgentContext{
		Objective:   "trace config loader",
		Constraints: []string{"internal/config"},
	}
	_ = eval.BuildInitialInstruction(ctx, nil)

	if len(eval.investigationNotes) != 1 {
		t.Errorf("notes wiped on same-objective call: %v", eval.investigationNotes)
	}
}

func TestSubExplorerAdvisoryNotes_BoundsAndLabelsNoToolProse(t *testing.T) {
	req := &types.SubAgentRequest{
		ID:        "trace-st-1",
		SubAgent:  "explorer",
		Objective: "inventory scoped package",
		Scope:     []string{"pkg/a", "pkg/b"},
	}
	long := strings.Repeat("x", subExplorerAdvisoryMaxNoteBytes+32)
	got := subExplorerAdvisoryNotes(req, []string{
		"old note",
		"rich JSON inventory",
		long,
	}, "rich JSON inventory", "")

	if len(got) != 2 {
		t.Fatalf("notes len=%d, want 2 after recent-window dedupe: %+v", len(got), got)
	}
	if !strings.Contains(got[0], "Sub-agent advisory result (not a citation)") ||
		!strings.Contains(got[0], "request_id=trace-st-1") ||
		!strings.Contains(got[0], "rich JSON inventory") {
		t.Fatalf("note missing advisory metadata/content:\n%s", got[0])
	}
	if strings.Count(strings.Join(got, "\n"), "rich JSON inventory") != 1 {
		t.Fatalf("duplicate stage report was not deduped: %+v", got)
	}
	if !strings.Contains(got[len(got)-1], "[advisory note truncated]") {
		t.Fatalf("long advisory note was not explicitly truncated")
	}
}

func TestSubExplorerAdvisoryNotes_PersistsLongArtifactWhenWorkDirAvailable(t *testing.T) {
	req := &types.SubAgentRequest{
		ID:        "trace-st-1",
		SubAgent:  "explorer",
		Objective: "inventory scoped package",
		Scope:     []string{"pkg/a"},
	}
	workDir := t.TempDir()
	long := "diagram header\n" + strings.Repeat("x", subExplorerAdvisoryMaxNoteBytes+64)

	got := subExplorerAdvisoryNotes(req, []string{long}, "", workDir)
	if len(got) != 1 {
		t.Fatalf("notes len=%d, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0], "full text saved at ") {
		t.Fatalf("long advisory note should mention persisted artifact:\n%s", got[0])
	}
	files, err := os.ReadDir(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("workDir files=%d, want 1", len(files))
	}
	data, err := os.ReadFile(filepath.Join(workDir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != long {
		t.Fatal("persisted advisory artifact does not match original note")
	}
}

func TestSubAgentReducer_CarriesBoundedAdvisoryNotes(t *testing.T) {
	results := []*types.SubAgentResult{
		{RequestID: "a", InvestigationNotes: []string{"note a", "note b"}},
		{RequestID: "b", InvestigationNotes: []string{"note b", "note c"}},
	}

	out := (&SubAgentReducer{}).Reduce(results)
	if got, want := len(out.InvestigationNotes), 3; got != want {
		t.Fatalf("InvestigationNotes len=%d, want %d: %+v", got, want, out.InvestigationNotes)
	}
	if strings.Join(out.InvestigationNotes, "|") != "note a|note b|note c" {
		t.Fatalf("InvestigationNotes order/dedupe mismatch: %+v", out.InvestigationNotes)
	}
}
