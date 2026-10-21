package context

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC276FactProjectionPreservesSourceLanes(t *testing.T) {
	results := []types.ToolResult{
		{ToolName: "exec_command", Success: true, Summary: "independent calculation 23", RawRef: "/audit/calc-a.txt"},
		{ToolName: "exec_command", Success: true, Summary: "independent calculation 23", RawRef: "/audit/calc-b.txt"},
		{ToolName: "exec_command", Success: true, Summary: "7", CommandMeasurement: &types.ToolCommandMeasurement{Kind: "count", Value: 7}},
		{ToolName: "git_log", Success: true, Summary: "abc123 Fix accounting", VCSHistory: &types.ToolVCSHistory{Kind: types.ToolVCSHistoryKindGitLog, Commits: []string{"abc123"}}},
		{ToolName: "read_file", Success: true, Summary: "const Limit = 17", ReadCoverage: &types.ToolReadCoverage{Path: "a.go", LineStart: 3, LineEnd: 3}},
		{ToolName: "read_file", Success: true, Summary: "request_id=42 failed", RuntimeArtifactRead: &types.ToolRuntimeArtifactRead{RequestedPath: "log.txt", Kind: "log", LineStart: 1, LineEnd: 1}},
	}
	var facts []types.RepoFact
	for _, result := range results {
		facts = append(facts, types.NewToolResultRepoFact(result, "recorded-locator", .8))
	}
	facts = append(facts, types.RepoFact{Key: "exec_command", Value: results[0].Summary, Source: "recorded-locator", EvidenceRef: results[0].RawRef, Confidence: .8})
	bus := &types.BusContext{Mutable: types.NewMutableState("ordinary investigation"), RepoFacts: facts}
	fork := *bus
	fork.Mutable = bus.Mutable.ForkForExploreDispatch()
	fork.RepoFacts = append([]types.RepoFact(nil), bus.RepoFacts...)
	contexts := []*types.AgentContext{
		BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize),
		BuildAgentContext(&fork, types.AgentFinalizer, types.StageFinalize),
		BuildSubAgentContext(bus, &types.SubAgentRequest{}),
	}
	for i, ac := range contexts {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			text := strings.Join(ac.RelevantFacts, "\n")
			for _, want := range []string{"independent calculation 23", "/audit/calc-a.txt", "/audit/calc-b.txt", "command_measurement", "vcs_metadata", "current_source", "runtime_artifact", "const Limit = 17", "request_id=42 failed", "legacy_unlinked_note", "evidence_ref=unavailable"} {
				if !strings.Contains(text, want) {
					t.Errorf("lost %q: %s", want, text)
				}
			}
			if strings.Count(text, "recorded_tool_output=") != len(results) || strings.Count(text, "legacy_unlinked_note") != 1 {
				t.Fatalf("same label/value guessed association or typed sources lost: %s", text)
			}
			if strings.Contains(text, "confidence:") {
				t.Fatal("tool score returned as truth probability")
			}
			if strings.Contains(text, "typed_grounding_policies=") {
				t.Fatal("typed record policy was promoted beside the independent summary")
			}
		})
	}
	encoded, err := json.Marshal(bus.RepoFacts)
	if err != nil {
		t.Fatal(err)
	}
	var restored []types.RepoFact
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	bus.RepoFacts = restored
	text := strings.Join(BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize).RelevantFacts, "\n")
	if strings.Contains(text, "recorded_tool_output=") || strings.Count(text, "legacy_unlinked_note") != len(facts) || !strings.Contains(text, "/audit/calc-b.txt") {
		t.Fatalf("recovery inferred private linkage or removed audit bytes: %s", text)
	}
}

func TestHMC276FactPreviewKeepsExactLocatorAndValueBudget(t *testing.T) {
	value := strings.Repeat("界", 400)
	ref := "/audit/" + strings.Repeat("source-", 100) + ".json"
	f := types.NewToolResultRepoFact(types.ToolResult{ToolName: "custom_reader", Success: true, Summary: value, RawRef: ref}, "runtime-source", 1)
	text := formatRelevantFact(0, f)
	if !strings.Contains(text, ref) || !strings.Contains(text, "[truncated]") || strings.Contains(text, value) || !strings.Contains(text, "typed_records=0") {
		t.Fatalf("preview lost locator, enlarged value budget or invented typed evidence: %s", text)
	}
}
