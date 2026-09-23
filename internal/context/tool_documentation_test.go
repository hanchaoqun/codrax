package context

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func documentationTestResult(t *testing.T, id int, padding int) types.ToolResult {
	t.Helper()
	body, err := json.Marshal(map[string]any{"id": id, "padding": strings.Repeat("x", padding), "unit": "ns", "missing": "unknown, never zero"})
	if err != nil {
		t.Fatal(err)
	}
	doc, ok := types.NormalizeToolDocumentation(types.ToolDocumentation{Version: types.ToolDocumentationVersion, Schema: "example/v1", Selection: types.ToolDocumentationSelection{View: fmt.Sprint(id)}, Content: body})
	if !ok {
		t.Fatal("invalid test contract")
	}
	return types.ToolResult{ToolName: "example_catalog", Success: true, Summary: string(body), Handoff: &types.ToolHandoffCarrier{Version: types.ToolHandoffCarrierVersion, ToolName: "example_catalog", Documentation: &doc}}
}

func documentationTestPrompt(t *testing.T, bus *types.BusContext, stage types.PipelineStage) string {
	t.Helper()
	ac := BuildAgentContext(bus, types.AgentFinalizer, stage)
	pc := BuildPromptContext(ac, &skill.Config{Name: "documentation-test"})
	for _, section := range pc.UserSections {
		if section.Title == SectionToolDocumentation {
			return section.Content
		}
	}
	return ""
}

func TestToolDocumentationPromptAdmission(t *testing.T) {
	valid := documentationTestResult(t, 1, 0)
	failed := valid
	failed.Success = false
	untyped := valid
	untyped.Handoff = nil
	for _, tc := range []struct {
		name   string
		result types.ToolResult
		stage  types.PipelineStage
		want   bool
	}{
		{"extract", valid, types.StageExtract, true},
		{"finalize", valid, types.StageFinalize, true},
		{"no_analyzer_expansion", valid, types.StageAnalyze, false},
		{"no_explorer_duplicate", valid, types.StageExplore, false},
		{"failed_producer", failed, types.StageFinalize, false},
		{"summary_is_not_authority", untyped, types.StageFinalize, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := &types.BusContext{ToolResults: []types.ToolResult{tc.result}}
			prompt := documentationTestPrompt(t, bus, tc.stage)
			if (prompt != "") != tc.want {
				t.Fatalf("documentation admitted=%t want=%t", prompt != "", tc.want)
			}
			if tc.want && strings.Count(prompt, valid.Summary) != 1 {
				t.Fatal("complete contract was changed or duplicated")
			}
		})
	}
}

func TestToolDocumentationPromptWholeDocumentBudget(t *testing.T) {
	for _, tc := range []struct {
		name, omitted string
		count, bytes  int
		firstKept     int
	}{
		{"bytes", "2 complete retained document(s) omitted", 5, 40000, 2},
		{"count", "2 complete retained document(s) omitted", 10, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := &types.BusContext{Mutable: types.NewMutableState("capabilities")}
			for i := 0; i < tc.count; i++ {
				bus.ToolResults = append(bus.ToolResults, documentationTestResult(t, i, tc.bytes))
			}
			// Duplicate delivery via the TurnA snapshot must not consume the
			// budget twice or change the actual producer JSON.
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: bus.ToolResults})
			prompt := documentationTestPrompt(t, bus, types.StageFinalize)
			if !strings.Contains(prompt, tc.omitted) {
				t.Fatal("bounded omission was not explicitly reported")
			}
			for i, result := range bus.ToolResults {
				want := 0
				if i >= tc.firstKept {
					want = 1
				}
				if got := strings.Count(prompt, result.Summary); got != want {
					t.Errorf("contract %d whole-document count=%d want=%d", i, got, want)
				}
			}
			if strings.Contains(prompt, "...[trimmed") {
				t.Fatal("contract was split into a misleading excerpt")
			}
		})
	}
}

func TestToolDocumentationDoesNotEnterRawCommandOutput(t *testing.T) {
	doc := documentationTestResult(t, 1, 2000)
	command := types.ToolResult{ToolName: "exec_command", Success: true, Summary: "123 files"}
	rendered := formatRawToolOutputs([]types.ToolResult{doc, command})
	if strings.Contains(rendered, doc.ToolName) || !strings.Contains(rendered, command.Summary) {
		t.Fatal("documentation became command truth or hid the real command")
	}
	mixed := doc
	mixed.Summary = "actual independent measurement: 123 files"
	mixed.Observations = []types.ObservationRecord{{ID: "measured"}}
	if !strings.Contains(formatRawToolOutputs([]types.ToolResult{mixed}), mixed.Summary) {
		t.Fatal("a mixed carrier's independent measurement output was hidden")
	}
}
