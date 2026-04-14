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
	mu                       sync.RWMutex
	taskList                 TaskList
	requestModel             *RequestModel
	emittedEvidence          []EvidenceItem
	// emittedAnswerSymbols + emittedAnswerSymbolCompleteness are
	// written as a set via SetEmittedAnswerSymbols and read via
	// EmittedAnswerSymbolSet (P2.1 Phase 9). The two fields are always
	// written together because the completeness claim is a set-level
	// property of the slate — an append-then-tag API would allow a
	// retry call to partially overwrite items without also overwriting
	// the claim, which is the exact foot-gun we want to rule out.
	// Retry semantics: a subsequent Set call REPLACES both fields
	// atomically under the write lock.
	emittedAnswerSymbols         []AnswerSymbol
	emittedAnswerSymbolCompleteness CompletenessClaim
	emittedHypothesisVerdicts []HypothesisVerdict
	turnAArtifacts           *TurnAArtifacts
	// answerDocument is the structured final-answer payload. It is
	// written by the emit_answer_document tool (one atomic set per
	// dispatch) and read by the finalizer's ParseOutput to render the
	// user-visible prose. Set semantics mirror SetEmittedAnswerSymbols:
	// a later call REPLACES any previous document, so a correction
	// retry from the ReAct loop cleanly wins over the prior attempt.
	// Cross-task reset (runTaskGraph) calls ResetAnswerDocument at
	// per-task entry so stale state cannot leak between tasks in a
	// multi-task run.
	answerDocument *AnswerDocument
}

// TurnAArtifacts is the P2.1 handoff payload from Turn A (explorer)
// to Turn B (extractor). It is a snapshot of everything Turn B needs
// to derive structured emit_evidence / emit_answer_symbol /
// emit_hypothesis_verdict items WITHOUT calling read_file or grep
// itself.
//
// Why a struct instead of letting Turn B re-read the BusContext
// directly: the extractor is forbidden from running tools, so it
// cannot re-derive any state that was lost during the handoff. Any
// fact that must reach the answer slate has to be in this struct
// when Turn A's ParseOutput closes. The struct is therefore the
// contract surface the two evaluators both depend on, and the
// session-1 ship pins its shape so session-2 wiring on either side
// cannot silently drop a field.
//
// Lifecycle:
//
//  1. Turn A's ParseOutput populates the struct via
//     MutableState.SetTurnAArtifacts. This happens at end-of-stage
//     after ensureStructuredEvidence + grounding + ranking, so the
//     evidence slice already reflects every deterministic
//     (concrete-value / mechanism / flow) item the explorer has.
//
//  2. The orchestrator dispatches StageExtract; the extractor's
//     BuildInitialPrompt reads the snapshot via TurnAArtifacts() and
//     bakes the relevant pieces into Turn B's prompt.
//
//  3. After Turn B's ParseOutput finishes, ResetTurnAArtifacts()
//     clears the buffer so the next per-task explore→extract cycle
//     starts clean (intra-Run self-loops + REPL turn boundary).
//
// The fields are intentionally minimal — Session 2 will iterate on
// what Turn B actually needs. Anything we omit here can be added
// incrementally as a backwards-compatible struct field; anything we
// include and stop using costs a 5-line removal commit.
type TurnAArtifacts struct {
	// UserQuestion is the original task question, plumbed through so
	// Turn B can quote it back in its prompt without re-deriving from
	// AnalysisIR.RequestModel (which is normalized and may have lost
	// the user's exact phrasing).
	UserQuestion string

	// InvestigationNotes is the sequence of per-iteration assistant
	// content blocks the explorer accumulated. Each entry is one ReAct
	// loop iteration's worth of LLM narrative. Turn B's prompt may
	// include a digest of these to ground its extraction in the same
	// language Turn A used.
	InvestigationNotes []string

	// ReadFiles is the de-duplicated list of repository-relative file
	// paths Turn A fetched via read_file. Used by Turn B to constrain
	// its emit_evidence / emit_answer_symbol Source citations to
	// files that were actually read (a structural defense against the
	// LLM citing a file it never saw).
	ReadFiles []string

	// ToolResults is the raw tool result history from Turn A, in
	// chronological order. Carries grep / read_file / repo_map
	// outputs so Turn B can re-scan them without burning iterations.
	// Subject to pruneToolHistory so the slice is bounded.
	ToolResults []ToolResult

	// EvidenceItems is the deterministic evidence the explorer's
	// ParseOutput already produced (concrete values, flow findings,
	// mechanism scan, grounded markdown items if the legacy channel
	// is still on). Turn B uses these as a starting point and may
	// emit additional items via emit_evidence; the merge happens at
	// drain time via mergeEvidenceItems.
	EvidenceItems []EvidenceItem

	// FlowFindings is the dataflow analysis output from Turn A.
	// Carries pre-extracted source→sink chains that are useful for
	// Turn B's chain rendering.
	FlowFindings []FlowFindingDigest

	// TerminalEvidenceCount is the count of EvidenceItems that Turn A's
	// deterministic extraction pipeline identified as terminal-literal
	// answer candidates (i.e. the items that hasTerminalEvidence /
	// identifyAnswerChains' strictAnswerItems filter would admit). It
	// is the β baseline for Phase 9's cardinality validator: when Turn
	// B's emit_answer_symbol claims CompletenessComplete, the
	// validator checks len(emit items) ≥ max(TerminalEvidenceCount,
	// len(AnalysisIR.AnswerContract.MustInclude)) and downgrades /
	// retries on mismatch.
	//
	// This count is NOT len(EvidenceItems) — most evidence items are
	// not terminal-literal answer candidates (they are [DIRECT] facts,
	// [MECHANISM] steps, etc.). It must be computed by Turn A at
	// handoff time using the same predicate identifyAnswerChains uses,
	// so the two numbers are directly comparable.
	//
	// Zero-value is safe: it means "Turn A produced no terminal-literal
	// candidates" which Phase 9 treats as "no β constraint — the
	// baseline collapses to len(AnswerContract.MustInclude) alone".
	TerminalEvidenceCount int
}

// HypothesisVerdict is the structured verdict the extractor (Turn B)
// emits for a single hypothesis from AnalysisIR.HypothesisSet. It is
// the on-the-wire shape of the emit_hypothesis_verdict tool's items
// and the input shape of MutableState.MarkHypothesis (the D7 carve-out
// API landing in P6).
//
// HypothesisID is matched against AnalysisIR.HypothesisSet[*].ID at
// drain time; unknown IDs are diagnosed but not silently dropped, so
// a typo in the LLM's emission cannot disappear a real hypothesis.
//
// Citation is a single file:line pointer (the same shape downstream
// renderers expect for any cite) that the renderer can use to anchor
// the verdict in the final answer. Empty when the verdict is purely
// inferential — but inferential verdicts must be 'inconclusive', not
// 'confirmed' / 'rejected'.
type HypothesisVerdict struct {
	HypothesisID string           `json:"hypothesis_id"`
	Status       HypothesisStatus `json:"status"`
	Rationale    string           `json:"rationale,omitempty"`
	Citation     string           `json:"citation,omitempty"`
}

// NewMutableState constructs a MutableState seeded with the given
// task list. Use this instead of zero-value literals so the internal
// mutex is paired correctly with its data.
func NewMutableState(tl TaskList) *MutableState {
	return &MutableState{taskList: tl}
}

// RequestModel returns a pointer to the analyzer-emitted RequestModel,
// or nil when no emit_analysis call has landed yet. Read by
// analyzer.ParseOutput after the ReAct loop exits to build AnalysisIR.
// The returned pointer is a snapshot copy — callers must not mutate
// the TermGraph slices in place.
func (m *MutableState) RequestModel() *RequestModel {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.requestModel == nil {
		return nil
	}
	cp := *m.requestModel
	return &cp
}

// SetRequestModel stores the analyzer's v3 RequestModel emitted via
// the emit_analysis tool. Callers pass a fully-populated RequestModel;
// the stored value is a deep-enough copy that the analyzer can read it
// out once the ReAct loop closes.
func (m *MutableState) SetRequestModel(rm RequestModel) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := rm
	m.requestModel = &cp
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

// AppendEvidence appends one or more LLM-emitted evidence items to the
// per-run buffer. Written by the emit_evidence tool; read by the
// explorer's ensureStructuredEvidence after the ReAct loop exits.
//
// P1.1: this is the structured replacement for the markdown-parsed
// evidence channel (parseEvidenceItems).
// Tools fill this buffer instead of asking the LLM to write a markdown
// header that a regex then walks. The two channels are merged in
// ensureStructuredEvidence so the structured and markdown channels run
// simultaneously and dedup on StableEvidenceID.
func (m *MutableState) AppendEvidence(items []EvidenceItem) {
	if m == nil || len(items) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedEvidence = append(m.emittedEvidence, items...)
}

// EmittedEvidence returns a snapshot of the LLM-emitted evidence buffer.
// The returned slice shares its backing array with the internal state —
// callers must not mutate it in place.
func (m *MutableState) EmittedEvidence() []EvidenceItem {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.emittedEvidence) == 0 {
		return nil
	}
	out := make([]EvidenceItem, len(m.emittedEvidence))
	copy(out, m.emittedEvidence)
	return out
}

// ResetEmittedEvidence clears the buffer. Called by the explorer's
// cross-Run reset path so a stage re-dispatch starts from an empty
// emitted-evidence state, matching how investigationNotes is reset.
func (m *MutableState) ResetEmittedEvidence() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedEvidence = nil
}

// SetEmittedAnswerSymbols atomically replaces the answer-symbol
// buffer and the accompanying completeness claim (P2.1 Phase 9
// set-level semantics). The emit_answer_symbol tool calls this on
// every invocation; subsequent calls REPLACE the prior slate,
// matching the "last writer wins" retry contract: on a mismatch
// retry the LLM either raises the list or downgrades the claim, and
// either way the new batch wins.
//
// The claim is validated for IsValid() and coerced to
// CompletenessUnknown on invalid input, so a buggy producer cannot
// corrupt the buffer with an unknown enum value. The items slice is
// defensively copied so a later mutation on the caller's side cannot
// race with reader goroutines.
func (m *MutableState) SetEmittedAnswerSymbols(items []AnswerSymbol, claim CompletenessClaim) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(items) == 0 {
		m.emittedAnswerSymbols = nil
	} else {
		m.emittedAnswerSymbols = append([]AnswerSymbol(nil), items...)
	}
	if !claim.IsValid() {
		claim = CompletenessUnknown
	}
	m.emittedAnswerSymbolCompleteness = claim
}

// EmittedAnswerSymbols returns a snapshot of the LLM-emitted answer
// symbol buffer paired with the set-level completeness claim. Both
// travel together so the reader cannot accidentally use one without
// checking the other. The slice is a defensive copy; the claim is a
// string enum so it is copied by value.
func (m *MutableState) EmittedAnswerSymbols() ([]AnswerSymbol, CompletenessClaim) {
	if m == nil {
		return nil, CompletenessUnknown
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.emittedAnswerSymbols) == 0 {
		return nil, m.emittedAnswerSymbolCompleteness
	}
	out := make([]AnswerSymbol, len(m.emittedAnswerSymbols))
	copy(out, m.emittedAnswerSymbols)
	return out, m.emittedAnswerSymbolCompleteness
}

// ResetEmittedAnswerSymbols clears both the buffer and the claim at
// the start of a new extractor dispatch. Mirror of ResetEmittedEvidence.
func (m *MutableState) ResetEmittedAnswerSymbols() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedAnswerSymbols = nil
	m.emittedAnswerSymbolCompleteness = CompletenessUnknown
}

// AppendEmittedHypothesisVerdicts appends LLM-emitted hypothesis
// verdicts (P2.1 Turn B emit_hypothesis_verdict channel) to the
// per-run buffer. The extractor's ParseOutput drains this buffer at
// end-of-stage and routes the verdicts through MutableState.MarkHypothesis
// (the D7 carve-out API on AnalysisIR). Sister API to AppendEvidence
// and AppendEmittedAnswerSymbols.
func (m *MutableState) AppendEmittedHypothesisVerdicts(items []HypothesisVerdict) {
	if m == nil || len(items) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedHypothesisVerdicts = append(m.emittedHypothesisVerdicts, items...)
}

// EmittedHypothesisVerdicts returns a snapshot of the verdict buffer.
// Returned slice is a copy; safe to retain across subsequent appends.
func (m *MutableState) EmittedHypothesisVerdicts() []HypothesisVerdict {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.emittedHypothesisVerdicts) == 0 {
		return nil
	}
	out := make([]HypothesisVerdict, len(m.emittedHypothesisVerdicts))
	copy(out, m.emittedHypothesisVerdicts)
	return out
}

// ResetEmittedHypothesisVerdicts clears the buffer at the start of a
// new extractor dispatch. Mirror of ResetEmittedEvidence.
func (m *MutableState) ResetEmittedHypothesisVerdicts() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedHypothesisVerdicts = nil
}

// SetAnswerDocument atomically replaces the P2.2 structured answer
// payload. Called by the emit_answer_document tool after the schema
// validator has accepted the LLM's emission. Set-replace semantics:
// a later call fully overwrites any previous document, so a
// correction retry from the finalizer's ReAct loop always wins. A nil
// argument clears the buffer (the same effect as ResetAnswerDocument).
//
// The input is defensively deep-copied so a later mutation on the
// caller's side cannot race with reader goroutines through the
// AnswerDocument accessor.
func (m *MutableState) SetAnswerDocument(doc *AnswerDocument) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerDocument = CloneAnswerDocument(doc)
}

// AnswerDocument returns a defensive deep copy of the buffered
// structured answer payload, or nil when no document has been set on
// this MutableState. The returned pointer is independent of the
// internal state — callers cannot mutate the buffered document in
// place.
func (m *MutableState) AnswerDocument() *AnswerDocument {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return CloneAnswerDocument(m.answerDocument)
}

// ResetAnswerDocument clears the answer payload at the start of a
// fresh per-task dispatch. Mirror of ResetTurnAArtifacts /
// ResetEmittedAnswerSymbols. Called from runTaskGraph so multi-task
// runs do not drag a stale document from task N into task N+1.
func (m *MutableState) ResetAnswerDocument() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerDocument = nil
}

// SetTurnAArtifacts stores the P2.1 handoff snapshot from the
// explorer (Turn A) for the extractor (Turn B) to consume. Called
// from the explorer's ParseOutput at end-of-stage when
// agent.TwoTurnExplorerEnabled() is true. The setter takes a value
// (not a pointer) so the explorer cannot accidentally mutate the
// snapshot after handoff — Turn B always sees a frozen view.
func (m *MutableState) SetTurnAArtifacts(a TurnAArtifacts) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Defensive copy of the slice headers so a later append on the
	// caller side cannot mutate the buffered snapshot in place.
	snap := a
	if a.InvestigationNotes != nil {
		snap.InvestigationNotes = append([]string(nil), a.InvestigationNotes...)
	}
	if a.ReadFiles != nil {
		snap.ReadFiles = append([]string(nil), a.ReadFiles...)
	}
	if a.ToolResults != nil {
		snap.ToolResults = append([]ToolResult(nil), a.ToolResults...)
	}
	if a.EvidenceItems != nil {
		snap.EvidenceItems = append([]EvidenceItem(nil), a.EvidenceItems...)
	}
	if a.FlowFindings != nil {
		snap.FlowFindings = append([]FlowFindingDigest(nil), a.FlowFindings...)
	}
	m.turnAArtifacts = &snap
}

// TurnAArtifacts returns a snapshot of the buffered handoff payload,
// or nil when no Turn A has run yet on this MutableState. The
// returned pointer is to a fresh copy — callers cannot mutate the
// buffered state in place.
func (m *MutableState) TurnAArtifacts() *TurnAArtifacts {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.turnAArtifacts == nil {
		return nil
	}
	out := *m.turnAArtifacts
	if m.turnAArtifacts.InvestigationNotes != nil {
		out.InvestigationNotes = append([]string(nil), m.turnAArtifacts.InvestigationNotes...)
	}
	if m.turnAArtifacts.ReadFiles != nil {
		out.ReadFiles = append([]string(nil), m.turnAArtifacts.ReadFiles...)
	}
	if m.turnAArtifacts.ToolResults != nil {
		out.ToolResults = append([]ToolResult(nil), m.turnAArtifacts.ToolResults...)
	}
	if m.turnAArtifacts.EvidenceItems != nil {
		out.EvidenceItems = append([]EvidenceItem(nil), m.turnAArtifacts.EvidenceItems...)
	}
	if m.turnAArtifacts.FlowFindings != nil {
		out.FlowFindings = append([]FlowFindingDigest(nil), m.turnAArtifacts.FlowFindings...)
	}
	return &out
}

// ResetTurnAArtifacts clears the buffered handoff snapshot. Called
// at the start of a fresh per-task explore→extract cycle (intra-Run
// self-loops + REPL turn boundary) so a stale Turn A from the
// previous task cannot leak into the next extractor dispatch.
func (m *MutableState) ResetTurnAArtifacts() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turnAArtifacts = nil
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

// ExecutionSignals tracks boolean signals produced by agents. After
// the 2026-04-14 simplification only HasEnoughFacts remains — the
// write-pipeline signals (HasPlan, HasPatch, *ReviewPassed,
// VerificationPassed) all gated stages that no longer exist.
type ExecutionSignals struct {
	HasEnoughFacts bool `json:"has_enough_facts"`
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
	AnswerChains  []AnswerChain       `json:"answer_chains,omitempty"` // deterministic answer-relevance envelopes (typed)
	AnswerSymbols []AnswerSymbol      `json:"answer_symbols,omitempty"` // L0-2: structured terminal symbols extracted from AnswerChains
	// AnswerSymbolCompleteness is the P2.1 set-level authority claim
	// attached to AnswerSymbols. It is written by whichever stage
	// populated AnswerSymbols (explorer flag=off path, or extractor
	// flag=on path) and read by context/builder.go §Answer Symbols to
	// pick the correct rendering branch (Translation mode for
	// "complete", softened floor prompt for "lower_bound", drop the
	// section entirely for "unknown"/zero). Zero value is the
	// fail-closed default. See types.CompletenessClaim for the three-
	// level authority ladder.
	AnswerSymbolCompleteness CompletenessClaim `json:"answer_symbol_completeness,omitempty"`
	ToolResults   []ToolResult        `json:"tool_results,omitempty"`
	MCPResponses  []MCPResponse       `json:"mcp_responses,omitempty"`
	StageReports  []StageReport       `json:"stage_reports,omitempty"`

	Signals ExecutionSignals `json:"signals"`

	Constraints []string `json:"constraints,omitempty"`
	Preferences []string `json:"preferences,omitempty"`

	LastTransitionReason string `json:"last_transition_reason,omitempty"`
	TraceID              string `json:"trace_id"`

	// AnalysisIR is the Analyzer v3 structured output. Set once by the
	// analyze stage via StageOutput.AnalysisIR → applyStageOutput and
	// never rewritten thereafter — the analyzer is the sole writer.
	// Downstream stages may write hypothesis status or per-node
	// execution state through dedicated APIs; the top-level pointer
	// itself stays read-only.
	AnalysisIR *AnalysisIR `json:"analysis_ir,omitempty"`
}

// AgentContext provides the narrowed view of BusContext for a single agent.
type AgentContext struct {
	AgentName AgentName     `json:"agent_name"`
	Stage     PipelineStage `json:"stage"`

	Objective              string `json:"objective"`
	CurrentTaskID          string `json:"current_task_id"`
	CurrentTask            string `json:"current_task"`
	CurrentTaskDescription string `json:"current_task_description,omitempty"`

	// AnalysisIR aliases BusContext.AnalysisIR for agents that have
	// opted into the v3 pipeline. Still nil for legacy call paths —
	// consumers MUST nil-check before reading.
	AnalysisIR *AnalysisIR `json:"-"`

	RelevantFacts         []string            `json:"relevant_facts,omitempty"`
	RelevantFiles         []string            `json:"relevant_files,omitempty"`
	EvidenceItems         []EvidenceItem      `json:"evidence_items,omitempty"`
	FlowFindings          []FlowFindingDigest `json:"flow_findings,omitempty"`
	AnswerChains          []AnswerChain       `json:"answer_chains,omitempty"`
	AnswerSymbols         []AnswerSymbol      `json:"answer_symbols,omitempty"`
	// AnswerSymbolCompleteness mirrors BusContext.AnswerSymbolCompleteness
	// for the narrowed agent view. Read by finalize's prompt builder
	// to pick Translation / softened-floor / shape-based rendering.
	AnswerSymbolCompleteness CompletenessClaim `json:"answer_symbol_completeness,omitempty"`
	RelevantToolSummaries []string            `json:"relevant_tool_summaries,omitempty"`
	RelevantMCPNotes      []string            `json:"relevant_mcp_notes,omitempty"`
	PriorReports          []StageReport       `json:"prior_reports,omitempty"`

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
