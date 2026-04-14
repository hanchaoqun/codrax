package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This file used to host the linear-pipeline orchestrator tests
// (TestRun_SimplePipeline, TestRun_MultiTaskExecution,
// TestRun_ExplorerFactSourceFlowsToFinalizer,
// TestDecideNextStage_*, TestDetermineActivePolicy_*, etc). All of
// them exercised the legacy implementation pipeline (plan →
// design_review → implement → code_review → verify) which was
// deleted in the 2026-04-14 simplification when codrax collapsed to
// a read-only analysis tool. The orchestrator is now tested via the
// DAG-path tests in orchestrator_dag_test.go + two_turn_e2e_test.go
// + answer_document_e2e_test.go.
//
// Test helpers that the DAG tests still depend on stay here.

// ---------------------------------------------------------------------------
// Mock agent
// ---------------------------------------------------------------------------

type mockAgent struct {
	name   types.AgentName
	execFn func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error)
}

func (m *mockAgent) Name() types.AgentName { return m.name }

func (m *mockAgent) Execute(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sk)
	}
	return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
}

func newMockAgent(name types.AgentName, fn func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error)) *mockAgent {
	return &mockAgent{name: name, execFn: fn}
}

// defaultResolvedConfig builds a read-only pipeline config:
// analyze → explore → extract → finalize. The write stages
// (plan / design_review / implement / code_review / verify) were
// deleted in the 2026-04-14 simplification.
func defaultResolvedConfig() *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Stages: map[types.PipelineStage]*types.StageConfig{
			types.StageAnalyze: {
				Name: types.StageAnalyze, DefaultAgent: types.AgentAnalyzer,
				DefaultSkill: "analyze-skill",
			},
			types.StageExplore: {
				Name: types.StageExplore, DefaultAgent: types.AgentExplorer,
				DefaultSkill: "explore-skill",
			},
			types.StageExtract: {
				Name: types.StageExtract, DefaultAgent: types.AgentExtractor,
				DefaultSkill: "extract-skill",
			},
			types.StageFinalize: {
				Name: types.StageFinalize, DefaultAgent: types.AgentFinalizer,
				DefaultSkill: "finalize-skill", Terminal: true,
			},
		},
		PipelineSettings: types.PipelineSettings{},
		Agents:           map[types.AgentName]*types.AgentConfig{},
		Skills:           map[string]*types.SkillConfigYAML{},
	}
}

// buildRegistries creates agent and skill registries with mock
// entries matching the read-only pipeline.
func buildRegistries(agentFns map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error)) (*agent.Registry, *skill.Registry, *agent.SubAgentRegistry) {
	ar := agent.NewRegistry()
	names := []types.AgentName{
		types.AgentAnalyzer,
		types.AgentExplorer,
		types.AgentExtractor,
		types.AgentFinalizer,
	}
	for _, n := range names {
		var fn func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error)
		if agentFns != nil {
			fn = agentFns[n]
		}
		ar.Register(newMockAgent(n, fn))
	}

	sr := skill.NewRegistry()
	skillNames := []string{
		"analyze-skill",
		"explore-skill",
		"extract-skill",
		"finalize-skill",
		"answer-document-skill",
	}
	for _, s := range skillNames {
		sr.Register(&skill.Config{Name: s, Goal: s + " goal"})
	}

	sar := agent.NewSubAgentRegistry()

	return ar, sr, sar
}
