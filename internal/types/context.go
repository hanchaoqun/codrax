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
// as before. The internal RWMutex protects against data races for
// top-level agents that may run concurrent tool dispatches in
// future refactors; today's single-agent loop does not exercise it.
//
// SubAgents do NOT share this region. SubAgentRuntime spawns
// isolated workers whose AgentContext is built by
// BuildSubAgentContext, which deliberately leaves Mutable nil. Any
// tool that requires Mutable (e.g. todo_write) will reject calls
// from a sub-agent with a clear error. Sub-agents return their
// findings via SubAgentResult and the reducer merges them back at
// the orchestrator boundary — that is the single point at which
// sub-agent output re-enters the parent's working state.
//
// Callers go through TaskList() / SetTaskList() / UpdateTaskStatus /
// UpdateTaskResult / SetCurrentTask instead of touching fields
// directly, so locking stays correct.
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

// UpdateTaskStatus marks a task by ID with the given status. No-op
// if the task is missing. Used by the orchestrator to transition
// individual tasks through the per-task execution loop.
func (m *MutableState) UpdateTaskStatus(id string, status TaskStatus) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.taskList.Tasks {
		if m.taskList.Tasks[i].ID == id {
			m.taskList.Tasks[i].Status = status
			return
		}
	}
}

// UpdateTaskResult records the per-task final answer (and a final
// status). Used by the orchestrator after a per-task finalize stage
// runs, so each task's contribution is preserved on the task itself
// rather than overwriting a single global FinalAnswer.
//
// Returns the actual task ID that was updated, which differs from
// the supplied id ONLY when the supplied id is stale and the
// fallback path below runs. Empty return means the task list was
// empty (nothing could be updated).
//
// Task identity fallback (S1): if the supplied ID is not found in
// the current task list, the result is written to the first in-
// progress task, failing that the first pending task, failing that
// the first task overall. This prevents silent data loss when the
// task list was replaced mid-pipeline by a second todo_write call
// (see project_S1_S2_S3_three_layer_fixes). Without the fallback,
// the finalizer's output is dropped and the CLI renders "(no
// result)" — df3 run 3 on eval/results/df3-20260412-100207 is the
// exact case that surfaced this bug. Callers can compare the
// returned ID against the supplied id to detect and log fallback.
func (m *MutableState) UpdateTaskResult(id, result string, status TaskStatus) string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.taskList.Tasks {
		if m.taskList.Tasks[i].ID == id {
			m.taskList.Tasks[i].Result = result
			m.taskList.Tasks[i].Status = status
			return id
		}
	}
	// Fallback: supplied ID is stale. Find a sensible target task to
	// avoid silent data loss. Preference: in_progress → pending → first.
	idx := -1
	for i := range m.taskList.Tasks {
		if m.taskList.Tasks[i].Status == TaskInProgress {
			idx = i
			break
		}
	}
	if idx < 0 {
		for i := range m.taskList.Tasks {
			if m.taskList.Tasks[i].Status == TaskPending {
				idx = i
				break
			}
		}
	}
	if idx < 0 && len(m.taskList.Tasks) > 0 {
		idx = 0
	}
	if idx >= 0 {
		m.taskList.Tasks[idx].Result = result
		m.taskList.Tasks[idx].Status = status
		return m.taskList.Tasks[idx].ID
	}
	return ""
}

// SetCurrentTask updates which task drives routing. The orchestrator
// calls this when it advances to the next task in the per-task loop.
func (m *MutableState) SetCurrentTask(id string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.taskList.CurrentTaskID = id
}

// StageReport is the synthesized narrative an agent leaves behind
// at the end of its ReAct loop. It carries the LLM's own summary of
// what it discovered or decided so downstream stages can read prior
// reasoning instead of reverse-engineering it from raw tool dumps.
//
// Reports are append-only and accumulate across the whole pipeline
// run. Each stage dispatch produces at most one report (the last
// non-empty assistant message of that ReAct loop).
type StageReport struct {
	Stage    PipelineStage `json:"stage"`
	Agent    AgentName     `json:"agent"`
	Findings string        `json:"findings"`
}

// RepoFact is a single discovered fact about the repository.
type RepoFact struct {
	Key         string  `json:"key"`
	Value       string  `json:"value"`
	Source      string  `json:"source"`
	EvidenceRef string  `json:"evidence_ref,omitempty"`
	Confidence  float64 `json:"confidence"`
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
	DesignReviewPassed bool   `json:"design_review_passed"`
	CodeReviewPassed   bool   `json:"code_review_passed"`
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

	// WorkDir is a per-trace temporary directory used by tools to
	// offload large outputs to disk (see internal/tool/blob.go). The
	// orchestrator creates and tears it down around Run(). When empty
	// (e.g. unit tests with a zero-value BusContext) tools degrade to
	// inline previews without persisting full content.
	WorkDir string `json:"work_dir,omitempty"`

	RepoFacts     []RepoFact          `json:"repo_facts,omitempty"`
	EvidenceItems []EvidenceItem      `json:"evidence_items,omitempty"`
	FlowFindings  []FlowFindingDigest `json:"flow_findings,omitempty"`
	AnswerChains  []string            `json:"answer_chains,omitempty"`  // deterministic chains that directly answer the question
	AnswerSymbols []AnswerSymbol      `json:"answer_symbols,omitempty"` // L0-2: structured terminal symbols extracted from AnswerChains
	ToolResults   []ToolResult        `json:"tool_results,omitempty"`
	MCPResponses  []MCPResponse       `json:"mcp_responses,omitempty"`
	StageReports  []StageReport       `json:"stage_reports,omitempty"`
	AnalysisIR    *AnalysisIR         `json:"analysis_ir,omitempty"`

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

	Objective               string   `json:"objective"`
	CurrentTaskID           string   `json:"current_task_id"`
	CurrentTask             string   `json:"current_task"`
	CurrentTaskDescription  string   `json:"current_task_description,omitempty"`
	CurrentTaskWriting      bool     `json:"current_task_writing"`
	CurrentTaskHighRisk     bool     `json:"current_task_high_risk"`
	CurrentTaskComplexity   string   `json:"current_task_complexity,omitempty"`
	CurrentTaskKeywords     []string `json:"current_task_keywords,omitempty"`
	CurrentTaskEntities     []string `json:"current_task_entities,omitempty"`
	CurrentTaskQuestionKind string   `json:"current_task_question_kind,omitempty"`
	CurrentTaskAnswerShape  string   `json:"current_task_answer_shape,omitempty"`

	RelevantFacts         []string            `json:"relevant_facts,omitempty"`
	RelevantFiles         []string            `json:"relevant_files,omitempty"`
	EvidenceItems         []EvidenceItem      `json:"evidence_items,omitempty"`
	FlowFindings          []FlowFindingDigest `json:"flow_findings,omitempty"`
	AnswerChains          []string            `json:"answer_chains,omitempty"`
	AnswerSymbols         []AnswerSymbol      `json:"answer_symbols,omitempty"`
	RelevantToolSummaries []string            `json:"relevant_tool_summaries,omitempty"`
	RelevantMCPNotes      []string            `json:"relevant_mcp_notes,omitempty"`
	PriorReports          []StageReport       `json:"prior_reports,omitempty"`

	PlanSummary         string `json:"plan_summary,omitempty"`
	PatchSummary        string `json:"patch_summary,omitempty"`
	ReviewSummary       string `json:"review_summary,omitempty"`
	VerificationSummary string `json:"verification_summary,omitempty"`

	Constraints []string `json:"constraints,omitempty"`
	Preferences []string `json:"preferences,omitempty"`

	MissingPiece MissingPiece `json:"missing_piece"`

	// RetryHint is propagated from TaskState.RetryHint when the
	// previous dispatch of this same stage flagged itself as
	// insufficient. The prompt builder renders it as the most
	// prominent user section to override the agent's instinct to
	// repeat the same approach.
	RetryHint string `json:"retry_hint,omitempty"`

	RepoRoot string `json:"repo_root"`
	Branch   string `json:"branch"`
	Commit   string `json:"commit"`
	WorkDir  string `json:"work_dir,omitempty"`

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
