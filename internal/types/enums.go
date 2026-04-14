package types

// PipelineStage represents a stage in the orchestration pipeline.
type PipelineStage string

const (
	StageAnalyze      PipelineStage = "analyze"
	StageExplore      PipelineStage = "explore"
	StageExtract      PipelineStage = "extract"
	StagePlan         PipelineStage = "plan"
	StageDesignReview PipelineStage = "design_review"
	StageImplement    PipelineStage = "implement"
	StageCodeReview   PipelineStage = "code_review"
	StageVerify       PipelineStage = "verify"
	StageFinalize     PipelineStage = "finalize"
)

// StageExtract is the P2.1 Turn B stage. It runs immediately after the
// merged explore window completes and before finalize. The extractor
// agent (internal/agent/extractor.go) handles it; the agent is
// restricted to emit_evidence / emit_answer_symbol /
// emit_hypothesis_verdict and works exclusively from Turn A's
// transcript snapshot — no read_file / grep / repo_map. The stage is
// only entered when agent.TwoTurnExplorerEnabled() is true; otherwise
// the scheduler skips it and the legacy path runs unchanged.

// IsTerminal returns true only for the finalize stage.
func (s PipelineStage) IsTerminal() bool {
	return s == StageFinalize
}

// String returns the string representation of the PipelineStage.
func (s PipelineStage) String() string {
	return string(s)
}

// AllStages returns all pipeline stages in order.
func AllStages() []PipelineStage {
	return []PipelineStage{
		StageAnalyze,
		StageExplore,
		StageExtract,
		StagePlan,
		StageDesignReview,
		StageImplement,
		StageCodeReview,
		StageVerify,
		StageFinalize,
	}
}

// AgentName identifies a named agent in the system.
type AgentName string

const (
	AgentAnalyzer       AgentName = "analyzer"
	AgentPlanner        AgentName = "planner"
	AgentExplorer       AgentName = "explorer"
	AgentExtractor      AgentName = "extractor"
	AgentDesignReviewer AgentName = "design_reviewer"
	AgentCodeReviewer   AgentName = "code_reviewer"
	AgentImplementer    AgentName = "implementer"
	AgentVerifier       AgentName = "verifier"
	AgentFinalizer      AgentName = "finalizer"
)

// String returns the string representation of the AgentName.
func (a AgentName) String() string {
	return string(a)
}

// AllAgentNames returns all agent names.
func AllAgentNames() []AgentName {
	return []AgentName{
		AgentAnalyzer,
		AgentPlanner,
		AgentExplorer,
		AgentExtractor,
		AgentDesignReviewer,
		AgentCodeReviewer,
		AgentImplementer,
		AgentVerifier,
		AgentFinalizer,
	}
}

// TaskStatus represents the status of a task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone       TaskStatus = "done"
	TaskBlocked    TaskStatus = "blocked"
	TaskFailed     TaskStatus = "failed"
)

// String returns the string representation of the TaskStatus.
func (s TaskStatus) String() string {
	return string(s)
}

// MissingPiece indicates what the pipeline still needs.
type MissingPiece string

const (
	MissingNone          MissingPiece = "none"
	MissingUnderstanding MissingPiece = "understanding"
	MissingFacts         MissingPiece = "facts"
	MissingPlan          MissingPiece = "plan"
	MissingCode          MissingPiece = "code"
	MissingReview        MissingPiece = "review"
	MissingVerification  MissingPiece = "verification"
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
