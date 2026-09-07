package repomap

import (
	"encoding/json"
	"strings"
	"testing"

	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

func TestB1587FileRoleRefinementFollowsProducerStage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		stage     ctypes.PipelineStage
		agent     ctypes.AgentName
		nilCtx    bool
		recursive string
	}{
		{"classification", ctypes.StageAnalyze, ctypes.AgentAnalyzer, false, "false"},
		{"classification_stage_only", ctypes.StageAnalyze, "", false, "false"},
		{"classification_agent_only", "", ctypes.AgentAnalyzer, false, "false"},
		{"exploration", ctypes.StageExplore, ctypes.AgentExplorer, false, "true"},
		{"unknown_stage", "", "", false, "true"},
		{"no_context", "", "", true, "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &ctypes.BusContext{PipelineStage: tc.stage, ActiveAgent: tc.agent, Mutable: ctypes.NewMutableState("file discovery")}
			if tc.nilCtx {
				ctx = nil
			}
			res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":"src","view":"source_inventory","roles":["file"]}`))
			if err != nil || res.Success || res.Repair == nil || res.Refinement == nil {
				t.Fatalf("expected pre-index typed file-role refusal: result=%+v err=%v", res, err)
			}
			if res.Refinement.PreferredNextTool != "list_files" || res.Refinement.PreferredParams["path"] != "src" || res.Refinement.PreferredParams["recursive"] != tc.recursive {
				t.Errorf("same-stage recommendation is not executable within recursion policy: %+v", res.Refinement)
			}
			if res.Repair.Metadata["preferred_next_tool"] != res.Refinement.PreferredNextTool {
				t.Errorf("repair and refinement disagree on target tool: %+v / %+v", res.Repair, res.Refinement)
			}
			for surface, text := range map[string]string{"summary": res.Summary, "repair": res.Repair.Hint} {
				if !strings.Contains(text, "recursive="+tc.recursive) {
					t.Errorf("%s did not teach the typed parameter recommendation recursive=%s: %s", surface, tc.recursive, text)
				}
				if tc.recursive == "false" && strings.Contains(text, "recursive=true") {
					t.Errorf("classification %s must not recommend a forbidden recursive retry: %s", surface, text)
				}
			}
			if ctx != nil && ctx.Mutable.SearchGraph() != nil {
				t.Fatal("a rejected parameter shape must not build an index")
			}
		})
	}
}

func TestB1587FileRoleSchemaNamesClassificationAndExplorationBoundaries(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&RepoMapV2{}).Parameters(), &schema); err != nil {
		t.Fatalf("repo-map schema is not valid JSON: %v", err)
	}
	description := schema.Properties["roles"].Description
	for _, want := range []string{"classification", "recursive=false", "exploration", "recursive=true", "available"} {
		if !strings.Contains(description, want) {
			t.Errorf("roles schema lacks stage-scoped path-discovery guidance %q: %s", want, description)
		}
	}
}

func TestB1587FileRoleTypedSemanticRolesKeepPriorityInEveryStage(t *testing.T) {
	for _, stage := range []ctypes.PipelineStage{ctypes.StageAnalyze, ctypes.StageExplore, ""} {
		t.Run(string(stage), func(t *testing.T) {
			mut := ctypes.NewMutableState("typed declarations")
			mut.SetRequestModel(ctypes.RequestModel{SourceInventoryProfile: &ctypes.SourceInventoryProfile{
				IsSourceInventory: true, TargetRoles: []ctypes.AnswerCandidateRole{ctypes.AnswerCandidateRoleType},
			}})
			res, err := (&RepoMapV2{}).Execute(&ctypes.BusContext{PipelineStage: stage, Mutable: mut}, json.RawMessage(`{"path":".","view":"source_inventory","roles":["file"],"attribute_roles":["function"]}`))
			if err != nil || res.Success || res.Refinement == nil || res.Repair == nil {
				t.Fatalf("expected semantic-role repair before index work: result=%+v err=%v", res, err)
			}
			if res.Refinement.PreferredNextTool != "repo_map" || res.Refinement.PreferredParams["roles"] != "function,type" || res.Repair.Metadata["preferred_next_tool"] != "repo_map" {
				t.Errorf("stage guidance replaced already-typed semantic repair: %+v / %+v", res.Repair, res.Refinement)
			}
			if _, ok := res.Refinement.PreferredParams["recursive"]; ok {
				t.Fatal("same-tool semantic refinement must not gain list-files parameters")
			}
			if mut.SearchGraph() != nil {
				t.Fatal("semantic-role refusal must remain before index work")
			}
		})
	}
}
