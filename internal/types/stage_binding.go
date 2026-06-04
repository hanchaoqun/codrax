package types

// StageBinding is the shared stage -> agent -> skill authority record
// used by the orchestrator and by analysis-time capability queries.
// Keeping this table in types avoids duplicating the same topology in
// multiple packages with slightly different drift risks.
type StageBinding struct {
	Stage    PipelineStage
	Agent    AgentName
	Skill    string
	Terminal bool
}

var builtinStageBindings = []StageBinding{
	{Stage: StageLogTriage, Agent: AgentLogTriager, Skill: "log-triage-skill"},
	{Stage: StagePerfTriage, Agent: AgentPerfTriager, Skill: "perf-triage-skill"},
	{Stage: StageMultiRepoFocus, Agent: AgentMultiRepoFocus, Skill: "multi-repo-focus-skill"},
	{Stage: StageAnalyze, Agent: AgentAnalyzer, Skill: "analysis-skill"},
	{Stage: StageExplore, Agent: AgentExplorer, Skill: "explore-skill"},
	{Stage: StageExtract, Agent: AgentExtractor, Skill: "extract-skill"},
	{Stage: StageFinalize, Agent: AgentFinalizer, Skill: "answer-document-skill", Terminal: true},
	{Stage: StageWriteAnalyze, Agent: AgentWriteAnalyzer, Skill: "write-analysis-skill"},
	{Stage: StagePlan, Agent: AgentPlanner, Skill: "change-plan-skill"},
	{Stage: StageApply, Agent: AgentCoder, Skill: "code-write-skill"},
	{Stage: StageVerify, Agent: AgentVerifier, Skill: "test-execute-skill"},
}

// AllStageBindings returns the canonical built-in stage bindings in
// declaration order.
func AllStageBindings() []StageBinding {
	out := make([]StageBinding, len(builtinStageBindings))
	copy(out, builtinStageBindings)
	return out
}

// StageBindingForStage returns the canonical built-in binding for a
// pipeline stage.
func StageBindingForStage(stage PipelineStage) (StageBinding, bool) {
	for _, binding := range builtinStageBindings {
		if binding.Stage == stage {
			return binding, true
		}
	}
	return StageBinding{}, false
}

// StageBindingForAgent returns the canonical built-in binding for an
// agent name.
func StageBindingForAgent(agent AgentName) (StageBinding, bool) {
	for _, binding := range builtinStageBindings {
		if binding.Agent == agent {
			return binding, true
		}
	}
	return StageBinding{}, false
}
