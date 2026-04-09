package types

// PipelineStage represents a stage in the orchestration pipeline.
type PipelineStage string

const (
	StageAnalyze      PipelineStage = "analyze"
	StageExplore      PipelineStage = "explore"
	StagePlan         PipelineStage = "plan"
	StageDesignReview PipelineStage = "design_review"
	StageImplement    PipelineStage = "implement"
	StageCodeReview   PipelineStage = "code_review"
	StageVerify       PipelineStage = "verify"
	StageFinalize     PipelineStage = "finalize"
)

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

// TaskType represents the category of a task.
type TaskType string

const (
	TaskTypeUnknown        TaskType = "unknown"
	TaskTypeAnalysis       TaskType = "analysis"
	TaskTypePlanning       TaskType = "planning"
	TaskTypeImplementation TaskType = "implementation"
	TaskTypeReview         TaskType = "review"
	TaskTypeVerification   TaskType = "verification"
)

// String returns the string representation of the TaskType.
func (s TaskType) String() string {
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
