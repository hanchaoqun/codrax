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

const (
	ReadModePipelineEnumsFile        = "internal/types/enums.go"
	ReadModePipelineStageBindingFile = "internal/types/stage_binding.go"
	ReadModePipelineTopologyFile     = "internal/orchestrator/topology.go"
)

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

// ReadModeMainStageBindings returns the canonical unconditional
// read-mode pipeline stages in the same order as AllMainStages.
func ReadModeMainStageBindings() []StageBinding {
	stages := AllMainStages()
	out := make([]StageBinding, 0, len(stages))
	for _, stage := range stages {
		if binding, ok := StageBindingForStage(stage); ok {
			out = append(out, binding)
		}
	}
	return out
}

// ReadModeConditionalPreStageBindings returns the canonical
// conditional pre-stages that can run before read-mode analyze.
func ReadModeConditionalPreStageBindings() []StageBinding {
	stages := []PipelineStage{StageLogTriage, StagePerfTriage}
	out := make([]StageBinding, 0, len(stages))
	for _, stage := range stages {
		if binding, ok := StageBindingForStage(stage); ok {
			out = append(out, binding)
		}
	}
	return out
}

// ReadModePipelineAuthorityFiles returns the source files that define
// the read-mode stage namespace and its orchestrator topology.
func ReadModePipelineAuthorityFiles() []string {
	return []string{
		ReadModePipelineEnumsFile,
		ReadModePipelineStageBindingFile,
		ReadModePipelineTopologyFile,
	}
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
