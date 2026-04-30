package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDetectStageToolCapabilityQuery_AnalyzerReadFile(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "Explorer stage 之前的 analyzer stage 里是否允许调用 read_file？",
	}
	got := detectStageToolCapabilityQuery(rm)
	if got == nil {
		t.Fatal("expected capability query to be detected")
	}
	if got.Binding.Stage != types.StageAnalyze {
		t.Fatalf("stage = %s, want %s", got.Binding.Stage, types.StageAnalyze)
	}
	if got.Binding.Agent != types.AgentAnalyzer {
		t.Fatalf("agent = %s, want %s", got.Binding.Agent, types.AgentAnalyzer)
	}
	if got.Binding.Skill != "analysis-skill" {
		t.Fatalf("skill = %q, want analysis-skill", got.Binding.Skill)
	}
	if got.Tool != "read_file" {
		t.Fatalf("tool = %q, want read_file", got.Tool)
	}
}

func TestDetectStageToolCapabilityQuery_SkipsScalarRoleLocate(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "负责校验 analyzer stage 不能调用 read_file 的那个函数叫什么？",
		Complexity: types.ComplexitySimple,
		Intent:     types.IntentReturnValue,
		AnswerSubject: types.AnswerSubject{
			Kind: types.SubjectFunctionName,
		},
		AnalyzerHints: types.AnalyzerHints{
			Kind: "return_value",
		},
		PredicateAxis: types.AxisReturn,
	}
	if got := detectStageToolCapabilityQuery(rm); got != nil {
		t.Fatalf("scalar role-locate query should not be forced into capability lane, got %+v", got)
	}
}

func TestReconcileStageToolCapabilitySurface_ForcesGenericSubjectAndClearsExactTargets(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "Explorer stage 之前的 analyzer stage 里是否允许调用 read_file？",
		Scenario:   types.ScenarioArchitectureExplain,
		AnswerSubject: types.AnswerSubject{
			Kind: types.SubjectConfigKey,
		},
		AnalyzerHints: types.AnalyzerHints{
			Entities:          []string{"read_file", "Explorer stage", "analyzer stage"},
			ExactTargets:      []string{"read_file"},
			ExactContextTerms: []string{"helper"},
			Keywords:          []string{"read_file"},
		},
	}
	resolved, q, reason := reconcileStageToolCapabilitySurface(rm)
	if q == nil {
		t.Fatal("expected reconcile to detect capability query")
	}
	if resolved.AnswerSubject.Kind != types.SubjectGeneric {
		t.Fatalf("subject kind = %s, want generic", resolved.AnswerSubject.Kind)
	}
	if resolved.Scenario != types.ScenarioGeneric {
		t.Fatalf("scenario = %s, want generic", resolved.Scenario)
	}
	if len(resolved.AnalyzerHints.ExactTargets) != 0 || len(resolved.AnalyzerHints.ExactContextTerms) != 0 {
		t.Fatalf("exact-resolution hints should be cleared, got %+v", resolved.AnalyzerHints)
	}
	for _, want := range []string{"read_file", "analysis-skill", "ToolSuggestions", "buildToolSchemas"} {
		if !strings.Contains(strings.Join(resolved.AnalyzerHints.Keywords, " "), want) {
			t.Fatalf("keywords missing %q: %v", want, resolved.AnalyzerHints.Keywords)
		}
	}
	if !strings.Contains(reason, "ToolSuggestions") {
		t.Fatalf("reason missing authority surface, got %q", reason)
	}
}

func TestRenderCapabilityAuthoritySection(t *testing.T) {
	q := &stageToolCapabilityQuery{
		Binding: types.StageBinding{
			Stage: types.StageAnalyze,
			Agent: types.AgentAnalyzer,
			Skill: "analysis-skill",
		},
		Tool: "read_file",
	}
	text := renderCapabilityAuthoritySection(q, "Capability Surface Authority")
	for _, want := range []string{
		"## Capability Surface Authority",
		"`read_file`",
		"`analysis-skill` skill",
		"`ToolSuggestions`",
		"`buildToolSchemas`",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("section missing %q:\n%s", want, text)
		}
	}
}
