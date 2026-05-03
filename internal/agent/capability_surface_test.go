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

func TestDetectStageToolCapabilityQuery_PrefersExplicitStageToolAuthorityOverScalarLiteralLane(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "Explorer stage 之前的 analyzer stage 里是否允许调用 read_file？",
		Complexity: types.ComplexitySimple,
		Intent:     types.IntentExplain,
		AnswerSubject: types.AnswerSubject{
			Kind: types.SubjectStringLiteral,
		},
		AnalyzerHints: types.AnalyzerHints{
			Kind:     "return_value",
			Entities: []string{"read_file"},
		},
		Predicates: types.SemanticPredicates{
			IsScalarAnswer: true,
		},
	}
	got := detectStageToolCapabilityQuery(rm)
	if got == nil {
		t.Fatal("expected explicit stage/tool authority question to stay in capability lane")
	}
	if got.Binding.Stage != types.StageAnalyze {
		t.Fatalf("stage = %s, want %s", got.Binding.Stage, types.StageAnalyze)
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
		RawRequest:    "Explorer stage 之前的 analyzer stage 里是否允许调用 read_file？",
		Intent:        types.IntentConfigQuery,
		Scenario:      types.ScenarioArchitectureExplain,
		PredicateAxis: types.AxisConfigure,
		AnswerSubject: types.AnswerSubject{
			Kind: types.SubjectConfigKey,
		},
		AnalyzerHints: types.AnalyzerHints{
			Kind:              "config_mapping",
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
	if resolved.Intent != types.IntentExplain {
		t.Fatalf("intent = %s, want explain", resolved.Intent)
	}
	if resolved.PredicateAxis != types.AxisDefine {
		t.Fatalf("predicate axis = %s, want define", resolved.PredicateAxis)
	}
	if resolved.AnalyzerHints.Kind != string(types.ReqMechanism) {
		t.Fatalf("question kind = %q, want %q", resolved.AnalyzerHints.Kind, types.ReqMechanism)
	}
	if len(resolved.AnalyzerHints.ExactTargets) != 0 || len(resolved.AnalyzerHints.ExactContextTerms) != 0 {
		t.Fatalf("exact-resolution hints should be cleared, got %+v", resolved.AnalyzerHints)
	}
	if resolved.AnalyzerHints.CapabilitySurface == nil {
		t.Fatal("capability surface hint should be compiled onto the request model")
	}
	if resolved.AnalyzerHints.CapabilitySurface.Tool != "read_file" {
		t.Fatalf("compiled capability tool = %q, want read_file", resolved.AnalyzerHints.CapabilitySurface.Tool)
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

func TestBuildAnalysisIR_CapabilitySurfacePreservesGenericScenario(t *testing.T) {
	raw := "analyzer stage 能调用 read_file 吗？"
	mut := types.NewMutableState(raw)
	mut.SetRequestModel(types.RequestModel{
		RawRequest:    raw,
		Intent:        types.IntentConfigQuery,
		Scenario:      types.ScenarioArchitectureExplain,
		PredicateAxis: types.AxisConfigure,
		Complexity:    types.ComplexityModerate,
		AnalyzerHints: types.AnalyzerHints{
			Kind:     "config_mapping",
			Shape:    "config_value",
			Entities: []string{"read_file", "analyzer stage"},
			Keywords: []string{"read_file"},
		},
		Predicates: types.SemanticPredicates{IsScalarAnswer: true},
	})
	ir, err := buildAnalysisIR(&types.AgentContext{
		Stage:     types.StageAnalyze,
		Objective: raw,
		Mutable:   mut,
	})
	if err != nil {
		t.Fatalf("buildAnalysisIR returned error: %v", err)
	}
	if ir == nil {
		t.Fatal("expected non-nil AnalysisIR")
	}
	if ir.RequestModel.Scenario != types.ScenarioGeneric {
		t.Fatalf("scenario = %s, want generic", ir.RequestModel.Scenario)
	}
	if ir.RequestModel.Intent != types.IntentExplain {
		t.Fatalf("intent = %s, want explain", ir.RequestModel.Intent)
	}
	if ir.RequestModel.PredicateAxis != types.AxisDefine {
		t.Fatalf("predicate axis = %s, want define", ir.RequestModel.PredicateAxis)
	}
	if ir.RequestModel.AnalyzerHints.Kind != string(types.ReqMechanism) {
		t.Fatalf("question kind = %q, want %q", ir.RequestModel.AnalyzerHints.Kind, types.ReqMechanism)
	}
	// Commit 61 Batch F.3 (red line "no system hard-cap"): pre-fix
	// reconcileShape's capability-surface rule (0c) forced
	if ir.RequestModel.AnalyzerHints.CapabilitySurface == nil {
		t.Fatal("expected compiled capability surface hint")
	}
	if ir.AnswerContract.ExactResolution != nil {
		t.Fatalf("exact resolution contract should be disabled for capability-surface questions, got %+v", ir.AnswerContract.ExactResolution)
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
