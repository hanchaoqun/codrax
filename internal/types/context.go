package types

import (
	"sync"
	"time"
)

// MutableState is the tool-mutable region of pipeline state. Tools
// invoked during the ReAct loop receive a *BusContext whose Mutable
// pointer aliases the orchestrator's, so updates to the contained
// task list are visible to subsequent tool calls and to the next
// stage's prompt rebuild without going through applyStageOutput.
//
// Everything outside MutableState in BusContext remains agent-output
// only — mutations are funneled through StageOutput → applyStageOutput
// as before. Concurrent access (e.g. parallel SubAgent runtime) is
// guarded by an internal RWMutex; callers go through TaskList() and
// SetTaskList() instead of touching fields directly.
type MutableState struct {
	mu       sync.RWMutex
	taskList TaskList
}

// NewMutableState constructs a MutableState seeded with the given
// task list. Use this instead of zero-value literals so the internal
// mutex is paired correctly with its data.
func NewMutableState(tl TaskList) *MutableState {
	return &MutableState{taskList: tl}
}

// TaskList returns a snapshot of the current task list. The returned
// value shares its slice headers with the underlying state — callers
// must not append to or otherwise mutate the slices in place.
func (m *MutableState) TaskList() TaskList {
	if m == nil {
		return TaskList{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.taskList
}

// SetTaskList atomically replaces the task list.
func (m *MutableState) SetTaskList(tl TaskList) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.taskList = tl
}

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
	HasEnoughFacts       bool   `json:"has_enough_facts"`
	HasPlan              bool   `json:"has_plan"`
	HasPatch             bool   `json:"has_patch"`
	DesignReviewPassed   bool   `json:"design_review_passed"`
	CodeReviewPassed     bool   `json:"code_review_passed"`
	VerificationPassed   bool   `json:"verification_passed"`
	LastStageFailed      bool   `json:"last_stage_failed"`
	LastFailureReason    string `json:"last_failure_reason,omitempty"`
	RetryCount           int    `json:"retry_count"`
}

// PolicyContext holds policy flags governing pipeline behavior.
type PolicyContext struct {
	AllowWrite          bool `json:"allow_write"`
	RequireReview       bool `json:"require_review"`
	RequireVerification bool `json:"require_verification"`
	MaxRetriesPerStage  int  `json:"max_retries_per_stage"`
}

// BusContext is the central data structure passed through the pipeline.
//
// The Mutable region is the only part of BusContext that tools may
// write to during the ReAct loop. Everything else is mutated only
// via Orchestrator.applyStageOutput so the orchestrator stays the
// single point of stage-level state changes.
type BusContext struct {
	// Mutable holds the tool-writable region (currently the working
	// task list). Tools see this pointer through the narrowed busCtx
	// constructed in BaseAgent.executeTool, so direct mutations are
	// visible immediately to subsequent tool calls and prompt rebuilds.
	Mutable *MutableState `json:"mutable,omitempty"`

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

	// FinalAnswer is the user-facing answer produced by the finalizer
	// agent. The orchestrator populates it from StageOutput.FinalAnswer
	// during applyStageOutput. Empty means "no answer was produced".
	FinalAnswer string `json:"final_answer,omitempty"`
}

// AgentContext provides the narrowed view of BusContext for a single agent.
type AgentContext struct {
	AgentName AgentName     `json:"agent_name"`
	Stage     PipelineStage `json:"stage"`

	Objective          string `json:"objective"`
	CurrentTaskID      string `json:"current_task_id"`
	CurrentTask        string `json:"current_task"`
	CurrentTaskWriting bool   `json:"current_task_writing"`
	CurrentTaskHighRisk bool  `json:"current_task_high_risk"`

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

	// Mutable aliases the orchestrator's BusContext.Mutable so that
	// tools dispatched from this agent (via BaseAgent.executeTool)
	// can write to the shared region. This breaks the strict
	// "AgentContext is a value-only narrow view" rule for one specific
	// pointer field by design — see MutableState's doc.
	Mutable *MutableState `json:"-"`
}

// PromptSection is a titled block of content used in prompt construction.
type PromptSection struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// PromptContext holds the assembled prompt for an agent invocation.
type PromptContext struct {
	SystemSections []PromptSection `json:"system_sections"`
	UserSections   []PromptSection `json:"user_sections"`

	EnabledTools []string `json:"enabled_tools"`

	AgentName AgentName     `json:"agent_name"`
	Stage     PipelineStage `json:"stage"`
	SkillName string        `json:"skill_name"`
}
