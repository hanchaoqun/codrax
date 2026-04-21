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
	StageAnalyze   PipelineStage = "analyze"
	StageExplore   PipelineStage = "explore"
	StageExtract   PipelineStage = "extract"
	StageFinalize  PipelineStage = "finalize"
)

// IsTerminal returns true only for the finalize stage.
func (s PipelineStage) IsTerminal() bool {
	return s == StageFinalize
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
	AgentAnalyzer   AgentName = "analyzer"
	AgentExplorer   AgentName = "explorer"
	AgentExtractor  AgentName = "extractor"
	AgentFinalizer  AgentName = "finalizer"
	AgentLogTriager AgentName = "log_triager"
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
