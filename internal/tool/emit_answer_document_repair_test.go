package tool

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEmitAnswerDocument_DroppedCitation_QueuesRepairDirective is the
// CGEC D1 regression. The LLM emits a value-shape AnswerDocument with
// a citation pointing at a file Turn A did NOT read. The grounder
// drops the citation AND must queue a RepairReadFile directive on
// the closure so the orchestrator's renderWindowHint can surface it
// in the next explore round's Forced Read List.
func TestEmitAnswerDocument_DroppedCitation_QueuesRepairDirective(t *testing.T) {
	mut := types.NewMutableState("test")
	// Pre-populate Turn A read set with one file; the citation
	// will reference a DIFFERENT file → expected drop.
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/skill/defaults.go"},
	})

	bus := &types.BusContext{Mutable: mut, RepoRoot: t.TempDir()}

	tool := &EmitAnswerDocument{}
	params, _ := json.Marshal(map[string]any{
		"shape":   "value",
		"summary": "lead in",
		"value":   map[string]any{"literal": "explore-skill", "citation_ref": 0},
		"citations": []map[string]any{
			{
				"file":  "internal/agent/subagent.go", // NOT in ReadFiles
				"line":  63,
				"quote": "",
			},
		},
	})

	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	// Tool either fails (no kept citation for value.citation_ref=0)
	// or accepts but with citation pool size 0; either way the repair
	// must be queued.
	_ = res
	_ = time.Now() // keep import live for any future timing assertion

	repairs := mut.EvidenceClosure().PendingRepairs()
	var found bool
	for _, r := range repairs {
		if r.Kind == types.RepairReadFile {
			for _, f := range r.Files {
				if f == "internal/agent/subagent.go" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("expected RepairReadFile directive for internal/agent/subagent.go, got %v", repairs)
	}
}

// When all citations are inside ReadFiles, no repair is queued.
func TestEmitAnswerDocument_AllCitationsInReadSet_NoRepairs(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/skill/defaults.go"},
	})
	bus := &types.BusContext{Mutable: mut, RepoRoot: t.TempDir()}

	tool := &EmitAnswerDocument{}
	params, _ := json.Marshal(map[string]any{
		"shape":   "value",
		"summary": "lead in",
		"value":   map[string]any{"literal": "explore-skill", "citation_ref": 0},
		"citations": []map[string]any{
			{
				"file":  "internal/skill/defaults.go", // IN ReadFiles
				"line":  14,
				"quote": "",
			},
		},
	})
	_, _ = tool.Execute(bus, params)

	repairs := mut.EvidenceClosure().PendingRepairs()
	for _, r := range repairs {
		if r.Kind == types.RepairReadFile {
			t.Errorf("unexpected RepairReadFile when citation was in ReadFiles: %v", r)
		}
	}
}
