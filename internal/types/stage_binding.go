package types

// StageBinding is the shared stage -> agent -> skill authority record
// used by the orchestrator and by analysis-time capability queries.
// Keeping this table in types avoids duplicating the same topology in
// multiple packages with slightly different drift risks.
type StageBinding struct {
	Stage            PipelineStage
	Agent            AgentName
	Skill            string
	Terminal         bool
	Responsibility   string
	PrimaryArtifacts []string
}

const (
	ReadModePipelineEnumsFile        = "internal/types/enums.go"
	ReadModePipelineStageBindingFile = "internal/types/stage_binding.go"
	ReadModePipelineTopologyFile     = "internal/orchestrator/topology.go"
)

var builtinStageBindings = []StageBinding{
	{
		Stage:            StageLogTriage,
		Agent:            AgentLogTriager,
		Skill:            "log-triage-skill",
		Responsibility:   "Parse attached runtime logs before analyze and publish non-fatal diagnostic hints.",
		PrimaryArtifacts: []string{"LogBundle", "MutableState.LogTriage"},
	},
	{
		Stage:            StagePerfTriage,
		Agent:            AgentPerfTriager,
		Skill:            "perf-triage-skill",
		Responsibility:   "Parse attached HiTrace or systrace artifacts before analyze and publish non-fatal performance hints.",
		PrimaryArtifacts: []string{"PerfBundle", "MutableState.PerfTrace"},
	},
	{
		Stage:            StageMultiRepoFocus,
		Agent:            AgentMultiRepoFocus,
		Skill:            "multi-repo-focus-skill",
		Responsibility:   "Select the active sub-repository set for a multi-repo read-mode question.",
		PrimaryArtifacts: []string{"MultiRepoFocusDecision", "active sub-repo scope"},
	},
	{
		Stage:          StageAnalyze,
		Agent:          AgentAnalyzer,
		Skill:          "analysis-skill",
		Responsibility: "Classify the request into a RequestModel, compile AnalysisIR, and build the downstream read-mode task and answer contracts.",
		PrimaryArtifacts: []string{
			"AnalysisIR",
			"TaskGraph",
			"EvidencePlan",
			"AnswerContract",
			"HypothesisSet",
			"QualityGate",
		},
	},
	{
		Stage:          StageExplore,
		Agent:          AgentExplorer,
		Skill:          "explore-skill",
		Responsibility: "Execute the AnalysisIR task graph and evidence plan, collect grounded evidence, and close investigation units.",
		PrimaryArtifacts: []string{
			"EvidenceItems",
			"AnswerChains",
			"StageReport",
			"aggregate facts",
		},
	},
	{
		Stage:          StageExtract,
		Agent:          AgentExtractor,
		Skill:          "extract-skill",
		Responsibility: "Distill accepted evidence into answer-ready symbols, hypothesis verdicts, and structured support.",
		PrimaryArtifacts: []string{
			"AnswerSymbols",
			"HypothesisVerdicts",
			"accepted evidence IDs",
			"StageReport",
		},
	},
	{
		Stage:          StageFinalize,
		Agent:          AgentFinalizer,
		Skill:          "answer-document-skill",
		Terminal:       true,
		Responsibility: "Render the AnswerDocumentV2 final answer from the AnswerContract, support plans, and grounded evidence.",
		PrimaryArtifacts: []string{
			"AnswerDocumentV2",
			"FinalAnswer",
			"Citations",
			"AnswerContract validation",
		},
	},
	{
		Stage:            StageWriteAnalyze,
		Agent:            AgentWriteAnalyzer,
		Skill:            "write-analysis-skill",
		Responsibility:   "Compile write-mode task shape, scope, risk, constraints, and expected outcomes.",
		PrimaryArtifacts: []string{"WriteAnalysisIR"},
	},
	{
		Stage:            StagePlan,
		Agent:            AgentPlanner,
		Skill:            "change-plan-skill",
		Responsibility:   "Produce a structured ChangePlan for an approved write-mode task.",
		PrimaryArtifacts: []string{"ChangePlan"},
	},
	{
		Stage:            StageApply,
		Agent:            AgentCoder,
		Skill:            "code-write-skill",
		Responsibility:   "Apply an approved ChangePlan inside the write-mode worktree blast radius.",
		PrimaryArtifacts: []string{"Changed files", "ChangeReport"},
	},
	{
		Stage:            StageVerify,
		Agent:            AgentVerifier,
		Skill:            "test-execute-skill",
		Responsibility:   "Run verification for applied changes and record structured verification results.",
		PrimaryArtifacts: []string{"VerificationReport"},
	},
}

// AllStageBindings returns the canonical built-in stage bindings in
// declaration order.
func AllStageBindings() []StageBinding {
	return cloneStageBindings(builtinStageBindings)
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
			return cloneStageBinding(binding), true
		}
	}
	return StageBinding{}, false
}

// StageBindingForAgent returns the canonical built-in binding for an
// agent name.
func StageBindingForAgent(agent AgentName) (StageBinding, bool) {
	for _, binding := range builtinStageBindings {
		if binding.Agent == agent {
			return cloneStageBinding(binding), true
		}
	}
	return StageBinding{}, false
}

func cloneStageBindings(in []StageBinding) []StageBinding {
	out := make([]StageBinding, len(in))
	for i, binding := range in {
		out[i] = cloneStageBinding(binding)
	}
	return out
}

func cloneStageBinding(in StageBinding) StageBinding {
	out := in
	if len(in.PrimaryArtifacts) > 0 {
		out.PrimaryArtifacts = append([]string(nil), in.PrimaryArtifacts...)
	}
	return out
}
