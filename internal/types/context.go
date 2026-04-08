package types

import "time"

// RepoFact is a single discovered fact about the repository.
type RepoFact struct {
	Key        string  `json:"key"`
	Value      string  `json:"value"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

// ToolResult records the outcome of a tool invocation.
type ToolResult struct {
	ToolName  string    `json:"tool_name"`
	Summary   string    `json:"summary"`
	RawRef    string    `json:"raw_ref,omitempty"`
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
}

// MCPResponse records a response from an MCP server.
type MCPResponse struct {
	ServerName string    `json:"server_name"`
	Method     string    `json:"method"`
	Summary    string    `json:"summary"`
	RawRef     string    `json:"raw_ref,omitempty"`
	Success    bool      `json:"success"`
	Timestamp  time.Time `json:"timestamp"`
}

// ExecutionSignals tracks boolean signals used for stage transitions.
type ExecutionSignals struct {
	HasEnoughFacts     bool   `json:"has_enough_facts"`
	HasPlan            bool   `json:"has_plan"`
	HasPatch           bool   `json:"has_patch"`
	ReviewPassed       bool   `json:"review_passed"`
	VerificationPassed bool   `json:"verification_passed"`
	LastStageFailed    bool   `json:"last_stage_failed"`
	LastFailureReason  string `json:"last_failure_reason,omitempty"`
	RetryCount         int    `json:"retry_count"`
}

// PolicyContext holds policy flags governing pipeline behavior.
type PolicyContext struct {
	AllowWrite          bool `json:"allow_write"`
	RequireReview       bool `json:"require_review"`
	RequireVerification bool `json:"require_verification"`
	MaxRetriesPerStage  int  `json:"max_retries_per_stage"`
}

// BusContext is the central data structure passed through the pipeline.
type BusContext struct {
	TaskList  TaskList  `json:"task_list"`
	TaskState TaskState `json:"task_state"`

	PipelineStage PipelineStage `json:"pipeline_stage"`
	ActiveAgent   AgentName     `json:"active_agent"`

	RepoRoot  string   `json:"repo_root"`
	Branch    string   `json:"branch"`
	Commit    string   `json:"commit"`
	ModuleMap []string `json:"module_map,omitempty"`

	RepoFacts    []RepoFact    `json:"repo_facts,omitempty"`
	ToolResults  []ToolResult  `json:"tool_results,omitempty"`
	MCPResponses []MCPResponse `json:"mcp_responses,omitempty"`

	Signals ExecutionSignals `json:"signals"`
	Policy  PolicyContext    `json:"policy"`

	Constraints []string `json:"constraints,omitempty"`
	Preferences []string `json:"preferences,omitempty"`

	LastTransitionReason string `json:"last_transition_reason,omitempty"`
	TraceID              string `json:"trace_id"`
}

// AgentContext provides the narrowed view of BusContext for a single agent.
type AgentContext struct {
	AgentName AgentName     `json:"agent_name"`
	Stage     PipelineStage `json:"stage"`

	Objective       string   `json:"objective"`
	CurrentTaskID   string   `json:"current_task_id"`
	CurrentTask     string   `json:"current_task"`
	CurrentTaskType TaskType `json:"current_task_type"`

	RelevantFacts         []string `json:"relevant_facts,omitempty"`
	RelevantFiles         []string `json:"relevant_files,omitempty"`
	RelevantToolSummaries []string `json:"relevant_tool_summaries,omitempty"`
	RelevantMCPNotes      []string `json:"relevant_mcp_notes,omitempty"`

	PlanSummary         string `json:"plan_summary,omitempty"`
	PatchSummary        string `json:"patch_summary,omitempty"`
	ReviewSummary       string `json:"review_summary,omitempty"`
	VerificationSummary string `json:"verification_summary,omitempty"`

	Constraints []string `json:"constraints,omitempty"`
	Preferences []string `json:"preferences,omitempty"`

	MissingPiece MissingPiece `json:"missing_piece"`

	RepoRoot string `json:"repo_root"`
	Branch   string `json:"branch"`
	Commit   string `json:"commit"`
}

// PromptSection is a titled block of content used in prompt construction.
type PromptSection struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// PromptContext holds the assembled prompt for an agent invocation.
type PromptContext struct {
	SystemSections    []PromptSection `json:"system_sections"`
	DeveloperSections []PromptSection `json:"developer_sections"`
	UserSections      []PromptSection `json:"user_sections"`

	EnabledTools []string `json:"enabled_tools"`

	AgentName AgentName     `json:"agent_name"`
	Stage     PipelineStage `json:"stage"`
	SkillName string        `json:"skill_name"`
}
