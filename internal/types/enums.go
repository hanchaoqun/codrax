package types

// PipelineStage represents a stage in the orchestration pipeline.
// After the 2026-04-14 simplification codrax is a read-only
// analysis tool: four stages, four agents, one deterministic DAG.
type PipelineStage string

const (
	// StageLogTriage is a conditional pre-stage that runs before
	// analyze when BusContext.AttachedLog is non-empty. It dispatches
	// the log_triager agent to produce a validated LogBundle on
	// MutableState, which downstream stages read as a read-only hint.
	// Failure is non-fatal; the main 4-stage pipeline continues with
	// bus.LogTriage()==nil.
	StageLogTriage PipelineStage = "log_triage"

	// StagePerfTriage is the HarmonyOS-HiTrace / Android-systrace
	// companion to StageLogTriage. Conditional pre-stage that runs
	// before analyze when BusContext.AttachedHitrace is non-empty.
	// Dispatches the perf_triager agent to produce a validated
	// PerfBundle on MutableState (jank spans + main-thread stalls +
	// cold-start timing). Failure is non-fatal; main 4-stage
	// pipeline continues with bus.PerfTrace()==nil.
	StagePerfTriage PipelineStage = "perf_triage"

	StageAnalyze  PipelineStage = "analyze"
	StageExplore  PipelineStage = "explore"
	StageExtract  PipelineStage = "extract"
	StageFinalize PipelineStage = "finalize"

	// Write-mode stages. Only fire when BusContext.Mode is
	// ModePlan / ModeApply / ModeVerify; Run()'s Mode switch
	// dispatches to runPlanPhase / runApplyPhase / runVerifyPhase
	// which set PipelineStage for observability. Each phase calls
	// dispatchStage directly rather than routing through
	// runTaskGraph (write mode bypasses the explore scheduler by
	// design — see the R6a decision in session 33 B0).
	StagePlan   PipelineStage = "plan"
	StageApply  PipelineStage = "apply"
	StageVerify PipelineStage = "verify"
)

// IsTerminal returns true only for the finalize stage.
func (s PipelineStage) IsTerminal() bool {
	return s == StageFinalize
}

// IsWrite reports whether the stage belongs to the write-mode
// pipeline (plan / apply / verify). Load-bearing: the single source
// of truth used by BuildAgentContext to gate propagation of
// read-mode stage artifacts (PriorReports / EvidenceItems /
// AnswerChains / AnswerSymbols / FlowFindings / UnverifiedAnalyzer-
// Findings) into planner/coder/verifier prompts. When those bled
// through untyped (session-35 audit), the analyzer's narrative
// (including <think> preamble like "Let me emit the analysis…")
// caused the planner LLM to pattern-match and skip
// emit_change_plan. Keep this method the only discriminator so
// future drift lights up one test, not a dozen render sites.
//
// StageAnalyze is NOT a write stage even though it runs inside the
// write pipeline as a classifier — its read-mode inputs (AnalysisIR
// assembly, repo_map, etc.) are required for correct classification
// regardless of the outer mode.
func (s PipelineStage) IsWrite() bool {
	return s == StagePlan || s == StageApply || s == StageVerify
}

// String returns the string representation of the PipelineStage.
func (s PipelineStage) String() string {
	return string(s)
}

// AllStages returns all pipeline stages in order, pre-stages first.
// Callers that need only the main pipeline should use AllMainStages.
func AllStages() []PipelineStage {
	return []PipelineStage{
		StageLogTriage,
		StagePerfTriage,
		StageAnalyze,
		StageExplore,
		StageExtract,
		StageFinalize,
	}
}

// AllMainStages returns the unconditional 4-stage pipeline, excluding
// conditional pre-stages. Used by the orchestrator when iterating
// the always-runs chain.
func AllMainStages() []PipelineStage {
	return []PipelineStage{
		StageAnalyze,
		StageExplore,
		StageExtract,
		StageFinalize,
	}
}

// AgentName identifies a named agent in the system.
type AgentName string

const (
	AgentAnalyzer    AgentName = "analyzer"
	AgentExplorer    AgentName = "explorer"
	AgentExtractor   AgentName = "extractor"
	AgentFinalizer   AgentName = "finalizer"
	AgentLogTriager  AgentName = "log_triager"
	AgentPerfTriager AgentName = "perf_triager"

	// Write-mode agents. Each pairs with the matching Stage
	// (StagePlan / StageApply / StageVerify) via pipelineTopology.
	// All three are real LLM-backed agents as of B2: planner emits
	// a structured ChangePlan, coder walks plan.Changes via
	// apply_patch calls inside a git worktree, verifier drives
	// run_tests (4-language parser) and persists a ChangeReport.
	AgentPlanner  AgentName = "planner"
	AgentCoder    AgentName = "coder"
	AgentVerifier AgentName = "verifier"
)

// String returns the string representation of the AgentName.
func (a AgentName) String() string {
	return string(a)
}

// AllAgentNames returns all agent names.
func AllAgentNames() []AgentName {
	return []AgentName{
		AgentAnalyzer,
		AgentExplorer,
		AgentExtractor,
		AgentFinalizer,
		AgentLogTriager,
		AgentPlanner,
		AgentCoder,
		AgentVerifier,
	}
}

// MissingPiece indicates what the pipeline still needs.
type MissingPiece string

const (
	MissingNone          MissingPiece = "none"
	MissingUnderstanding MissingPiece = "understanding"
	MissingFacts         MissingPiece = "facts"
)

// String returns the string representation of the MissingPiece.
func (s MissingPiece) String() string {
	return string(s)
}

// TransportType represents the MCP transport mechanism.
type TransportType string

const (
	TransportStdio TransportType = "stdio"
	TransportSSE   TransportType = "sse"
	TransportHTTP  TransportType = "http"
)
