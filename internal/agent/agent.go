package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/hint"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// StageOutput is the structured result an Agent returns to the Orchestrator.
type StageOutput struct {
	// Raw JSON output produced by the Agent
	Data json.RawMessage `json:"data"`

	// Signals to update in BusContext
	SignalUpdates *types.ExecutionSignals `json:"signal_updates,omitempty"`

	// Facts discovered during execution
	NewFacts []types.RepoFact `json:"new_facts,omitempty"`

	// Structured evidence items discovered during execution.
	EvidenceItems []types.EvidenceItem `json:"evidence_items,omitempty"`

	// Compact dataflow findings discovered during execution.
	FlowFindings []types.FlowFindingDigest `json:"flow_findings,omitempty"`

	// AnswerChains: deterministic answer-relevance envelopes produced
	// by identifyAnswerChains. Each element wraps an EvidenceItem
	// plus the computed Score + StrictOK flag. The finalizer prompt
	// assembler renders them to markdown via the single legal flatten
	// point in context/builder.go.
	AnswerChains []types.AnswerChain `json:"answer_chains,omitempty"`

	// Answer symbols: L0-2 structured terminals extracted from
	// AnswerChains. For registration / call_chain / return_value
	// kinds, these are the canonical names the finalizer must list
	// verbatim (no add, no drop). For other kinds, empty; the
	// finalizer falls back to the legacy prose path.
	AnswerSymbols []types.AnswerSymbol `json:"answer_symbols,omitempty"`

	// AnswerSymbolCompleteness is the set-level authority claim the
	// producer attaches to AnswerSymbols (P2.1). See
	// types.CompletenessClaim for the three-level ladder. Written by
	// the explorer (flag=off legacy path, always "complete" when
	// AnswerSymbols is non-empty) or the extractor (flag=on path,
	// validated against Turn A's TerminalEvidenceCount and
	// AnalysisIR.AnswerContract.MustInclude). Zero value degrades the
	// rendering layer to the shape-based prompt.
	AnswerSymbolCompleteness types.CompletenessClaim `json:"answer_symbol_completeness,omitempty"`

	// Tool results collected during execution
	ToolResults []types.ToolResult `json:"tool_results,omitempty"`

	// MCP responses collected during execution
	MCPResponses []types.MCPResponse `json:"mcp_responses,omitempty"`

	// Repairs is the structured RepairDirective queue produced by
	// CGEC enforcers (citation grounder, pre-complete check, stall
	// detector). applyStageOutput drains this slice into
	// MutableState.EvidenceClosure().AddRepair so the next explore-
	// window prompt can render them via the orchestrator's retry-
	// hint path. Empty for stages that did not raise any structured
	// repairs (i.e. the analyzer / extractor steady state).
	Repairs []types.RepairDirective `json:"repairs,omitempty"`

	// The missing piece after this stage completes
	MissingPiece types.MissingPiece `json:"missing_piece"`

	// Error if the stage failed
	Error string `json:"error,omitempty"`

	// FinalAnswer is the user-facing text the finalizer agent produces.
	// applyStageOutput copies it to BusContext.FinalAnswer when non-empty
	// so the CLI (and any other caller of Run) can render it. Other
	// agents leave this empty.
	FinalAnswer string `json:"final_answer,omitempty"`

	// RetryHint is the agent's own diagnosis of why it could not
	// progress this turn. When the orchestrator self-loops the same
	// stage, it copies this into TaskState.RetryHint so the next
	// dispatch's prompt includes a concrete "do this differently"
	// directive. Forward transitions clear it.
	RetryHint string `json:"retry_hint,omitempty"`

	// StageReport is the LLM's own synthesis of what this stage
	// discovered or decided — typically the last assistant message of
	// the ReAct loop. BaseAgent.Execute auto-populates it from the
	// message history if the evaluator did not set it explicitly, so
	// downstream stages can read prior reasoning instead of trying to
	// reverse-engineer it from raw tool dumps. applyStageOutput
	// appends it to BusContext.StageReports.
	StageReport string `json:"stage_report,omitempty"`

	// AnalysisIR is the Analyzer v3 structured output. Only the
	// analyzer stage sets this; other stages leave it nil.
	// applyStageOutput copies it onto BusContext.AnalysisIR on the
	// first non-nil value and leaves it alone afterwards, so a rogue
	// re-dispatch to the analyzer cannot mutate the IR in place.
	AnalysisIR *types.AnalysisIR `json:"analysis_ir,omitempty"`
}

// Agent defines the interface for all agent types.
type Agent interface {
	// Name returns the agent identifier.
	Name() types.AgentName

	// Execute runs the agent's ReAct loop and returns structured output.
	Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error)
}

// Dependencies bundles the external dependencies an Agent needs.
type Dependencies struct {
	LLM           llm.Adapter
	Tools         *tool.Registry
	MCPServers    *mcp.Registry
	SubAgents     *SubAgentRegistry
	MaxIterations int
	Emit          render.EventEmitter

	// PromptAssembler turns an AgentContext + Skill config into the
	// initial llm.Message slice for the ReAct loop. Optional: a nil
	// value means "use DefaultPromptAssembler()" and is the common
	// production path. Tests and specialized callers set a custom
	// implementation to capture the assembled PromptContext or to
	// render messages in a non-default encoding. See
	// internal/agent/prompt_assembler.go for the contract.
	PromptAssembler PromptAssembler

	// LoopPolicy is the throttling / dedup / budget configuration
	// for the LoopController extension point BaseAgent.Execute
	// consults each iteration. Optional: a zero LoopPolicy is
	// replaced by DefaultLoopPolicy() in NewBaseAgent, matching the
	// historical "fire at most every 3 iters, force-stop after 2
	// idle rounds, cap continuations at 5" behavior of the pre-
	// refactor evaluator implementations.
	LoopPolicy LoopPolicy

	// CancelChecker is the user-driven cancellation probe. The agent
	// loop polls it at the top of each iteration and at every tool
	// dispatch; a non-nil error means the operator hit Ctrl+C / typed
	// `/cancel` in the REPL and the loop should unwind immediately.
	// Optional: nil disables the check entirely (single-shot CLI runs
	// have no operator to interrupt). Wired by cmd/root.go from the
	// Orchestrator's CancelToken so REPL and orchestrator share one
	// source of truth.
	CancelChecker func() error

	// ExploreHeuristics carries the tunable thresholds for the
	// explorer evaluator's mid-loop and soft-stop detection branches.
	// Optional: zero fields are filled from DefaultExploreHeuristics()
	// in cmd/root.go before agent construction.
	ExploreHeuristics types.ExploreHeuristics

	// AgentSettings carries all per-agent tunable limits (iteration
	// caps, tool-history budget, correction retries). Resolved from
	// YAML in cmd/root.go before agent construction.
	AgentSettings types.AgentSettings

	// Skills is the skill registry. Agents that need to switch
	// skill mid-dispatch (currently only log_triager for its two-step
	// fallback: log-triage-skill → log-segmentation-skill → log-triage-skill
	// per segment) read from this handle. Optional: agents that only
	// use the skill the orchestrator passed in leave it nil.
	Skills *skill.Registry

	// ToolParamCompatByAgent carries provider-scoped compatibility policy for
	// schema-aware tool argument normalization. Absent agent key means off.
	// Kept outside llm.Adapter because it lives at the agent/tool boundary:
	// the adapter knows bytes from the provider, while the agent owns the
	// exact tool schema catalog for the current dispatch.
	ToolParamCompatByAgent map[types.AgentName]types.ToolParamCompatConfig
}

// BaseAgent provides the common ReAct loop implementation.
// Concrete agents embed this and customize behavior via the Evaluator interface.
type BaseAgent struct {
	name types.AgentName
	deps *Dependencies
	eval Evaluator
}

// Evaluator allows concrete agents to customize the ReAct loop behavior.
type Evaluator interface {
	// BuildInitialInstruction returns the evaluator's per-dispatch
	// dynamic supplement — a single user-role message BaseAgent
	// appends AFTER the static PromptContext the shared assembler
	// renders. The supplement is strictly additive: it may ONLY
	// carry content that depends on this dispatch's runtime state
	// (Turn A digest, resolved shape, cardinality baseline, ...) and
	// must NEVER restate any piece of the skill config (Goal,
	// Workflow, OutputFormat, Prohibitions) or re-emit a section
	// title the builder already renders — see docs/architecture.md
	// §3.3 for the Skill/Evaluator boundary contract. Returning ""
	// is the correct implementation for a stage whose entire
	// per-dispatch context is already carried by the builder (the
	// analyzer is the minimal case).
	BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string

	// ShouldStop decides if the loop should terminate based on the LLM response.
	ShouldStop(resp llm.Response, iteration int) bool

	// ParseOutput extracts the structured StageOutput from accumulated results.
	ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, mcpResponses []types.MCPResponse) (*StageOutput, error)

	// DetermineMissingPiece decides what's still missing after this stage.
	DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece
}

// ToolSchemaFilter is an OPTIONAL interface evaluators may implement
// to dynamically restrict the agent's tool surface per-iter inside a
// single Execute() dispatch. Without this hook the tool schema set is
// fixed at Execute() entry from the skill's ToolSuggestions; the
// 2026-05-17 T2 patch-first work needs per-iter responsiveness so the
// answer-document evaluator can drop emit_answer_document from the
// schema list after two consecutive full-emit failures, leaving only
// emit_answer_document_patch as the actionable retry path.
//
// BaseAgent.Execute calls FilterToolSchemas BEFORE every LLM.Chat
// turn so a state change in the prior iter's Observe pass takes
// effect immediately. Implementations MUST be idempotent — repeated
// calls with the same context yield the same schema slice — and MUST
// NOT mutate the input slice (return a fresh slice or a re-sliced
// view).
//
// Returning the input unchanged is the correct implementation when
// no filter applies, and is the implicit behaviour for evaluators
// that do not implement this interface.
type ToolSchemaFilter interface {
	FilterToolSchemas(ctx *types.AgentContext, schemas []llm.ToolSchema) []llm.ToolSchema
}

// LoopPhase names the point in the ReAct iteration where
// LoopController.Observe is being consulted. The phase lets a single
// Observe hook handle both the "LLM is still calling tools — should
// we nudge it?" case (PhaseMidLoop, fires after each tool-execution
// batch) and the "LLM soft-stopped with content — should we
// continue?" case (PhaseSoftStop, fires when resp.ToolCalls is empty
// and resp.Content is non-empty).
type LoopPhase int

const (
	// PhaseMidLoop fires after BaseAgent has executed a batch of
	// tool calls for this iteration. The evaluator sees the full
	// allToolResults slice plus a pointer to the last result it
	// appended, and can request a corrective hint.
	PhaseMidLoop LoopPhase = iota
	// PhaseSoftStop fires when the LLM produced content but called
	// no tools. The evaluator can vote to inject a continuation hint
	// (accept the soft-stop into a forced continue) or stay silent
	// (accept the natural termination).
	PhaseSoftStop
)

// String returns a human-readable phase name used in the
// BaseAgent.Execute debug trace. Keeping it short keeps the one-line
// log entries readable.
func (p LoopPhase) String() string {
	switch p {
	case PhaseMidLoop:
		return "mid-loop"
	case PhaseSoftStop:
		return "soft-stop"
	}
	return "unknown"
}

// LoopObservation is the read-only snapshot a LoopController sees
// when BaseAgent asks it to observe one iteration. The policy-owned
// counter fields (IdleStreak, ContinuationsUsed, MidLoopInjectsUsed)
// are snapshots of the LoopPolicy state BEFORE this iteration's
// signal is applied, so branches that used to read the evaluator's
// own counters (explorer.idleStreakInDepth, sub_explorer.idleStreak,
// ...) can migrate to obs.IdleStreak with no semantic change.
type LoopObservation struct {
	// Phase distinguishes the mid-loop call from the soft-stop call.
	// Evaluators should dispatch on this field.
	Phase LoopPhase

	// Iteration is the ReAct loop's zero-based iteration counter,
	// matching the index BaseAgent would log as `iter=N`.
	Iteration int

	// Response is the LLM's latest response — non-empty Content and
	// potentially non-empty ToolCalls. At PhaseSoftStop the ToolCalls
	// slice is guaranteed empty by the BaseAgent dispatch path.
	Response llm.Response

	// LastToolResult points at the most recently executed tool's
	// result inside AllToolResults. Only populated at PhaseMidLoop;
	// nil at PhaseSoftStop. The pointer may be nil mid-loop too if
	// the previous iteration had no successful tool execution.
	LastToolResult *types.ToolResult

	// AllToolResults is every successful tool result collected so
	// far in this Execute call. Evaluators must not mutate this
	// slice — BaseAgent reuses it across observations.
	AllToolResults []types.ToolResult

	// IdleStreak is the number of consecutive Observe calls so far
	// that returned LoopSignal{Progress: false, HintRequested: false}
	// — i.e. the count of "truly idle" rounds. Owned and incremented
	// by LoopPolicy; read by evaluators that want to branch on
	// "N-th idle round" without tracking the count themselves.
	IdleStreak int

	// ContinuationsUsed is the number of soft-stop hints LoopPolicy
	// has already accepted in this dispatch, subject to the
	// MaxContinuations budget.
	ContinuationsUsed int

	// MidLoopInjectsUsed is the number of mid-loop hints LoopPolicy
	// has already accepted in this dispatch, subject to the
	// MaxMidLoopInjects budget.
	MidLoopInjectsUsed int
}

// LoopSignal is the raw detection result a LoopController returns
// from Observe. Every field is a DETECTION signal — throttling,
// dedup, and budget are applied by LoopPolicy against this struct.
// Multiple fields may be set at once: for example, an evaluator
// that detected progress this round AND wants to inject a prompt
// returns Progress=true and HintRequested=true; the policy resets
// the idle counter and considers the hint subject to its other
// rules.
type LoopSignal struct {
	// Progress reports that this iteration made meaningful forward
	// progress toward the question. Resets the LoopPolicy's
	// IdleStreak counter. Leave false if the iteration was a no-op
	// (LLM output content but added no useful tool results, or read
	// the wrong file again).
	Progress bool

	// StopRequested is a hard vote to terminate the loop. The
	// evaluator has the final word on termination — LoopPolicy
	// honors this signal without applying throttle / budget rules.
	// Use for detected terminal conditions (answer complete,
	// structural failure, hard budget exhausted) where the evaluator
	// is certain no further iteration will help.
	StopRequested bool

	// StopReason is a short human-readable label surfaced in the
	// BaseAgent.Execute debug trace when StopRequested is honored.
	StopReason string

	// HintRequested is true when the evaluator has a corrective
	// prompt to inject. LoopPolicy may still drop the hint based on
	// HintKey dedup, MinInjectInterval throttle, or the phase-
	// specific budget cap.
	HintRequested bool

	// Hint is the user-role message body LoopPolicy will append to
	// the ReAct message stream when it accepts the injection.
	Hint string

	// HintKey is a stable identifier used for dedup. When two
	// consecutive accepted hints share the same non-empty HintKey,
	// the second is dropped. Pick a short descriptor of the hint's
	// category ("partial_function", "erm_gap", "parallelize", ...)
	// so the dedup window catches "same problem two iters in a row"
	// without suppressing legitimately different corrections.
	HintKey string

	// BypassThrottle skips the MinInjectInterval spacing rule while
	// still honoring HintKey dedup and the phase-specific inject
	// budgets. Reserve this for urgent corrective hints that must be
	// delivered immediately after a prior accepted hint, such as
	// "repair the evidence you just emitted before widening scope".
	BypassThrottle bool

	// BypassBudget skips the phase-specific hint budget accounting while
	// still honoring HintKey dedup and (unless also set) the throttle.
	// Reserve this for repair / closure / materialization corrections
	// that should not compete with ordinary exploratory nudges for the
	// same finite mid-loop budget.
	BypassBudget bool
}

// LoopController is the unified replacement for ContinuingEvaluator +
// MidLoopEvaluator. One Observe hook is consulted at BOTH the
// mid-loop (after tool execution) and soft-stop (no tool call with
// content) points in BaseAgent.Execute; the LoopPhase field on
// LoopObservation distinguishes the two. Detection-only: the
// evaluator returns a raw LoopSignal, and LoopPolicy decides whether
// to act on it based on throttling, dedup, and budget rules. See
// docs/architecture.md §3.2 for the full loop-control contract.
type LoopController interface {
	Observe(ctx *types.AgentContext, obs LoopObservation) LoopSignal
}

// NewBaseAgent creates a new BaseAgent.
func NewBaseAgent(name types.AgentName, deps *Dependencies, eval Evaluator) *BaseAgent {
	maxIter := deps.MaxIterations
	if maxIter <= 0 {
		maxIter = 20
	}
	emit := deps.Emit
	if emit == nil {
		emit = render.NopEmitter
	}
	// A nil PromptAssembler is the common production path — tests are
	// the only callers that install a custom implementation. The
	// fallback keeps NewBaseAgent zero-config for every existing
	// caller (cmd/root.go, eval harnesses, unit tests) while still
	// letting the new extension point override the default.
	assembler := deps.PromptAssembler
	if assembler == nil {
		assembler = DefaultPromptAssembler()
	}
	// Zero-value LoopPolicy → substitute the historical defaults.
	// A caller that genuinely wants every knob off must build an
	// explicit LoopPolicy (e.g. LoopPolicy{IdleStopThreshold: -1})
	// — but in practice only unit tests do that.
	loopPolicy := deps.LoopPolicy
	if loopPolicy == (LoopPolicy{}) {
		loopPolicy = DefaultLoopPolicy()
	}
	// Copy ALL fields from the input deps via dereference + override.
	// Pre-fix this was an explicit field-by-field assignment list of
	// 8 fields, silently dropping CancelChecker / AgentSettings /
	// ExploreHeuristics / Skills — which broke Ctrl+C interruption
	// (b.deps.CancelChecker stayed nil), made yaml-tuned tool-history
	// + context-pressure overrides ineffective at the BaseAgent layer,
	// and disabled the log_triager / perf_triager two-step
	// segmentation fallback (base.deps.Skills nil). Dereference +
	// override removes the omission risk and auto-picks up any
	// future Dependencies field with no maintenance.
	copied := *deps
	copied.MaxIterations = maxIter
	copied.Emit = emit
	copied.PromptAssembler = assembler
	copied.LoopPolicy = loopPolicy
	return &BaseAgent{
		name: name,
		deps: &copied,
		eval: eval,
	}
}

func (b *BaseAgent) Name() types.AgentName {
	return b.name
}

// truncForLog clips a string for diagnostic logging. Used by the debug
// trace lines in Execute so a single iteration dump never floods the
// log file with multi-megabyte LLM bodies. The "...[truncated N bytes]"
// suffix preserves the total length so the reader can spot truncation.
// looksLikeEmbeddedToolCall detects when the LLM wrote tool-call JSON
// in its text content instead of using the function-calling mechanism.
//
// Trigger conditions (ALL must be true):
//  1. Content contains either an explicit <tool_call> envelope or a
//     JSON code block (```json ... ```) / top-level '{...}' object.
//  2. The explicit envelope carries JSON-looking name + arguments
//     fields, OR the JSON block carries a concrete emit_* tool name
//     as a quoted string value (e.g. "emit_answer_symbol",
//     "emit_answer_document", "emit_evidence").
//
// This avoids false positives from LLM prose that merely discusses
// tool names or JSON structures. Explicit tool_call tags are already
// a tool-call syntax, so they are treated as serialized-but-not-
// executed calls even for non-emit tools such as grep/read_file.
func looksLikeEmbeddedToolCall(content string) bool {
	lower := strings.ToLower(content)
	if (strings.Contains(lower, "<tool_call>") || strings.Contains(lower, "<minimax:tool_call>")) &&
		(strings.Contains(content, `"name"`) || strings.Contains(content, `"tool"`) || strings.Contains(content, `"tool_name"`)) &&
		(strings.Contains(content, `"arguments"`) || strings.Contains(content, `"parameters"`) || strings.Contains(content, `"args"`)) {
		return true
	}
	// Must contain a JSON-like block.
	hasJSONBlock := strings.Contains(content, "```json") ||
		(strings.Contains(content, `{"`) && strings.Contains(content, `"}`))
	if !hasJSONBlock {
		return false
	}
	// Must contain a concrete emit tool name as a JSON string value
	// (quoted, not just mentioned in prose).
	emitTools := []string{
		`"emit_answer_symbol"`,
		`"emit_answer_document"`,
		`"emit_hypothesis_verdict"`,
		`"emit_evidence"`,
		`"emit_investigation_complete"`,
		`"emit_analysis"`,
	}
	for _, t := range emitTools {
		if strings.Contains(content, t) {
			return true
		}
	}
	return false
}

func shouldCompactNoToolAnswerDraftHistory(ctx *types.AgentContext, resp llm.Response) bool {
	return noToolTextShouldStayProtocolOnly(ctx, resp) && looksLikeStructuredAnswerDraft(resp.Content)
}

func contentForNoToolHistory(ctx *types.AgentContext, resp llm.Response) string {
	if !noToolTextShouldStayProtocolOnly(ctx, resp) || !looksLikeStructuredAnswerDraft(resp.Content) {
		return resp.Content
	}
	return compactNoToolAnswerDraftHistory(ctx, resp)
}

func contentForNoToolHistoryWithFinalizerDraftBudget(ctx *types.AgentContext, resp llm.Response, finalizerDraftsPreserved int) (string, int) {
	if ctx != nil &&
		ctx.Stage == types.StageFinalize &&
		len(resp.ToolCalls) == 0 &&
		strings.TrimSpace(resp.Content) != "" &&
		looksLikeStructuredAnswerDraft(resp.Content) {
		if finalizerDraftsPreserved == 0 {
			return resp.Content, 1
		}
		return compactNoToolAnswerDraftHistory(ctx, resp), finalizerDraftsPreserved
	}
	return contentForNoToolHistory(ctx, resp), finalizerDraftsPreserved
}

func compactNoToolAnswerDraftHistory(ctx *types.AgentContext, resp llm.Response) string {
	stage := "unknown"
	if ctx != nil && ctx.Stage != "" {
		stage = string(ctx.Stage)
	}
	return fmt.Sprintf("[protocol-only text omitted: the model returned %d bytes of answer-like prose during stage %s without a tool call. The next user message carries the required tool-call correction.]", len(resp.Content), stage)
}

func noToolTextShouldStayProtocolOnly(ctx *types.AgentContext, resp llm.Response) bool {
	if ctx == nil || len(resp.ToolCalls) != 0 || strings.TrimSpace(resp.Content) == "" {
		return false
	}
	// Finalization deliberately preserves rich drafts on missing
	// emit_answer_document retries so the model can copy its prior
	// answer into the structured tool call. Earlier stages produce
	// protocol handoffs, not user-visible answer bodies; feeding a
	// premature draft back into those loops reinforces stale or
	// unvalidated prose.
	if ctx.Stage == types.StageFinalize {
		return false
	}
	return ctx.Stage == types.StageExplore || toolChoiceForStage(ctx.Stage) == "required"
}

func shouldRouteNoToolThroughStageProtocolController(ctx *types.AgentContext, resp llm.Response, loopCtrl LoopController) bool {
	if loopCtrl == nil || ctx == nil || len(resp.ToolCalls) != 0 || strings.TrimSpace(resp.Content) == "" {
		return false
	}
	// Explore and Extract are intermediate protocol stages: free
	// prose is useful to show in the REPL, but it is never the
	// stage's completion channel. Route no-tool prose through the
	// stage LoopController before any evaluator fallback can accept
	// it, so the existing structured completion contracts decide
	// whether to inject a tool-call correction or stop.
	switch ctx.Stage {
	case types.StageExplore, types.StageExtract:
		return true
	default:
		return false
	}
}

func shouldRouteEmptyNoToolThroughStageProtocolController(ctx *types.AgentContext, resp llm.Response, loopCtrl LoopController) bool {
	if loopCtrl == nil || ctx == nil || len(resp.ToolCalls) != 0 || strings.TrimSpace(resp.Content) != "" {
		return false
	}
	switch ctx.Stage {
	case types.StageAnalyze, types.StageExtract, types.StageFinalize, types.StageLogTriage, types.StagePerfTriage:
		return toolChoiceForStage(ctx.Stage) == "required"
	default:
		return false
	}
}

func looksLikeStructuredAnswerDraft(content string) bool {
	s := strings.TrimSpace(content)
	if s == "" {
		return false
	}
	if len(s) >= 1200 {
		return true
	}
	if strings.Contains(s, "```") {
		return true
	}
	lines := strings.Split(s, "\n")
	structural := 0
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") ||
			strings.HasPrefix(t, "- ") ||
			strings.HasPrefix(t, "* ") ||
			strings.HasPrefix(t, "|") ||
			looksLikeOrderedMarkdownLine(t) {
			structural++
		}
		if structural >= 3 {
			return true
		}
	}
	return false
}

func looksLikeOrderedMarkdownLine(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(s) && s[i] == '.' && s[i+1] == ' '
}

// sanitizeToolCallsForHistory preserves OpenAI-style tool-call pairing
// while preventing a malformed assistant arguments blob from poisoning
// the next LLM request. Streaming gateways can occasionally return a
// truncated function.arguments string (for example `{"items":`). The
// execution path separately repairs or converts malformed params into
// a model-facing failed ToolResult; the conversation history must still
// contain syntactically valid JSON or the provider rejects the next turn
// before the model has a chance to self-correct.
func sanitizeToolCallsForHistory(calls []llm.ToolCall) []llm.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	var out []llm.ToolCall
	for i, tc := range calls {
		if toolCallParamsValidForHistory(tc.Params) {
			if out != nil {
				out[i] = tc
			}
			continue
		}
		if out == nil {
			out = append([]llm.ToolCall(nil), calls...)
		}
		out[i].Params = json.RawMessage(`{}`)
		logging.Warning("[diag] sanitized invalid tool-call arguments for history tool=%s id=%s len=%d",
			tc.Name, tc.ID, len(tc.Params))
	}
	if out != nil {
		return out
	}
	return calls
}

func toolCallParamsValidForHistory(params json.RawMessage) bool {
	if strings.TrimSpace(string(params)) == "" {
		return false
	}
	return json.Valid(params)
}

func malformedToolParamsResult(tc llm.ToolCall) *types.ToolResult {
	errText := "malformed JSON"
	if len(tc.Params) == 0 {
		errText = "empty JSON"
	} else {
		var probe interface{}
		if err := json.Unmarshal(tc.Params, &probe); err != nil {
			errText = err.Error()
		}
	}
	summary := fmt.Sprintf(
		"invalid params: malformed JSON tool arguments for %s (%s). "+
			"Re-emit this tool call with a single native JSON object in arguments; do not wrap it as a string and do not emit partial JSON. "+
			"Original argument bytes were not executed.",
		tc.Name, errText,
	)
	logging.Warning("[agent] tool %q params rejected before execution: %s len=%d id=%s",
		tc.Name, errText, len(tc.Params), tc.ID)
	return &types.ToolResult{
		ToolName:  tc.Name,
		Success:   false,
		Summary:   summary,
		Timestamp: time.Now(),
	}
}

func truncForLog(s string, max int) string {
	return logging.Truncate(s, max)
}

// maxToolHistoryBytes caps the cumulative size of "tool" role messages
// kept verbatim in the ReAct conversation. Individual tool results are
// already bounded by tool.MaxInlineBytes (~32 KB), but a 15-iteration
// explorer loop with multiple read_file calls per iteration can still
// pile up 400+ KB of tool output and blow the model's context window.
//
// 150 KB is chosen to:
//   - comfortably fit ~5 full-size (32 KB) tool results in the "hot"
//     window, enough for the LLM to correlate the most recent step
//     with the 2-3 before it;
//   - leave substantial headroom for the system prompt, assistant
//     messages, and the model's own response on a typical 128k-token
//     context window;
//   - stay well below the 256 KB range where even permissive models
//     start to slow down.
//
// pruneToolHistory stubs out older "tool" role messages in-place when
// their cumulative content size exceeds the budget. Walks
// newest-to-oldest, keeping the hot window intact, and replaces every
// older tool message's content with a short placeholder.
//
// The ToolCallID is preserved on every stubbed message so OpenAI's
// tool_call ↔ tool response pairing stays valid — dropping the message
// entirely would produce a 400 "tool_call_id without matching response"
// from the API. Assistant messages are never touched: they carry the
// LLM's own reasoning and tool-call plans, are tiny by comparison, and
// removing them would erase the thread the model is working with.
//
// streamPreviewBuffer throttles content-chunk events from a streaming
// LLM adapter. onDelta is called per chunk by the adapter; it appends
// to the running buffer and emits an EventAgentContent at most every
// throttleInterval. flush fires one last emission at stream end so
// the user sees the final tail. Nil emit (tests, headless) makes the
// whole thing a no-op.
type streamPreviewBuffer struct {
	emit            render.EventEmitter
	agent           types.AgentName
	stage           types.PipelineStage
	iter            int
	parallelGroupID string
	parallelUnitID  string
	buf             strings.Builder
	lastEmitAt      time.Time
}

// streamPreviewThrottle is the minimum gap between consecutive
// EventAgentContent emissions. 250ms keeps the UI feel responsive
// (~4 updates/sec) without flooding pterm.Area redraws or the
// logging pipeline.
const streamPreviewThrottle = 250 * time.Millisecond

func newStreamPreviewBuffer(emit render.EventEmitter, agent types.AgentName, stage types.PipelineStage, iter int, parallelGroupID, parallelUnitID string) *streamPreviewBuffer {
	return &streamPreviewBuffer{
		emit:            emit,
		agent:           agent,
		stage:           stage,
		iter:            iter,
		parallelGroupID: parallelGroupID,
		parallelUnitID:  parallelUnitID,
	}
}

func (b *streamPreviewBuffer) onDelta(delta string) {
	if b == nil || b.emit == nil || delta == "" {
		return
	}
	b.buf.WriteString(delta)
	if time.Since(b.lastEmitAt) < streamPreviewThrottle {
		return
	}
	b.lastEmitAt = time.Now()
	b.emit(render.Event{
		Kind:            render.EventAgentContent,
		Timestamp:       b.lastEmitAt,
		Agent:           b.agent,
		Stage:           b.stage,
		Iteration:       b.iter,
		Reasoning:       b.buf.String(),
		ParallelGroupID: b.parallelGroupID,
		ParallelUnitID:  b.parallelUnitID,
	})
}

// flush emits one last event so the tail of the stream always reaches
// the renderer. Safe to call even if no deltas arrived.
func (b *streamPreviewBuffer) flush() {
	if b == nil || b.emit == nil || b.buf.Len() == 0 {
		return
	}
	b.emit(render.Event{
		Kind:            render.EventAgentContent,
		Timestamp:       time.Now(),
		Agent:           b.agent,
		Stage:           b.stage,
		Iteration:       b.iter,
		Reasoning:       b.buf.String(),
		ParallelGroupID: b.parallelGroupID,
		ParallelUnitID:  b.parallelUnitID,
	})
}

// softNoToolCallMessage renders the user-visible line fired when a
// must-emit stage gets an LLM response with zero tool calls. The
// orchestrator's soft-message contract is to stay zh/en aware and
// never leak internal jargon; this helper mirrors that convention.
// Lives in agent rather than orchestrator because the agent layer
// is where the empty-ToolCalls observation actually happens (and
// importing orchestrator from agent would reverse the dependency).
func softNoToolCallMessage(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese", "简体中文":
		return "⟳ 模型未返回工具调用，重新发起请求"
	}
	return "⟳ Model returned no tool call — re-prompting"
}

// softMidLoopCapMessage renders the dock notice the BaseAgent emits
// the first time a per-key mid-loop hint cap saturates AND the gate
// the hint targets is still unsatisfied. The hint key itself is
// internal vocab (e.g. `explorer.mid-loop.closure-repair`) so it is
// kept out of the user-facing message and surfaced only on the
// trace log and the LearningFailure reason.
//
// docs/design/post_phase2a_forensic_followups.md §2.1.G #4 + #7.
func softMidLoopCapMessage(lang string, capCount int) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese", "简体中文":
		return fmt.Sprintf("· 同类提示已达上限 (%d/%d)，模型仍未推进对应卡点；按现有证据继续", capCount, capCount)
	}
	return fmt.Sprintf("· Mid-loop hint cap reached (%d/%d) — model is not advancing the gated state; continuing with what we have", capCount, capCount)
}

// escalateMidLoopCapSaturation surfaces a per-key cap-saturation
// event onto the three channels the user + the rest of the pipeline
// need:
//
//   - dock: typed EventOrchestratorNotice with NoticeUpstreamCap
//     so the user observes that the loop hit a structural cap
//     instead of silent quota burn followed by a fail-loud header.
//   - trace + cross-Run telemetry: AppendLearningFailure with stage
//     `explorer` and the cap reason; the orchestrator's end-of-Run
//     summary surfaces the count so operators notice a pattern.
//   - success-criteria layer: NoteMidLoopHintExhausted records the
//     (HintKey, Reason) on Mut so downstream criterion expressions
//     and the orchestrator's fallback gate can pivot earlier on
//     "model structurally unable to satisfy" instead of waiting for
//     the dispatch's max-step quota.
//
// One-shot per (dispatch, key) — the loop-policy de-duplicates
// CapSaturatedHintKey to "" after the first fire, so this helper
// only runs once per saturated key. Safe to call with nil ctx /
// Mutable / Emit; missing pieces simply skip their channel.
//
// docs/design/post_phase2a_forensic_followups.md §2.1.G #4 + #7.
func (b *BaseAgent) escalateMidLoopCapSaturation(ctx *types.AgentContext, iteration int, hintKey, reason string) {
	if hintKey == "" {
		return
	}
	logging.Info("[diag %s] iter=%d phase=midloop_cap_saturated key=%q %s",
		b.name, iteration, hintKey, reason)
	capCount := DefaultLoopPolicy().MaxPerKeyInjects
	if b.deps != nil && b.deps.LoopPolicy.MaxPerKeyInjects > 0 {
		capCount = b.deps.LoopPolicy.MaxPerKeyInjects
	}
	if b.deps != nil && b.deps.Emit != nil {
		var (
			lang  string
			stage types.PipelineStage
		)
		if ctx != nil {
			lang = ctx.Language
			stage = ctx.Stage
		}
		b.deps.Emit(render.Event{
			Kind:       render.EventOrchestratorNotice,
			Timestamp:  time.Now(),
			Agent:      b.name,
			Stage:      stage,
			Iteration:  iteration,
			NoticeKind: render.NoticeUpstreamCap,
			Reasoning:  softMidLoopCapMessage(lang, capCount),
		})
	}
	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.AppendLearningFailure(string(b.name), fmt.Sprintf("midloop hint cap saturated: key=%s; %s", hintKey, reason))
		ctx.Mutable.NoteMidLoopHintExhausted(hintKey, reason)
	}
}

// toolChoiceForStage returns the OpenAI-style tool_choice value to
// attach to the LLM request for this dispatch stage. The stages whose
// terminal action is a specific emit_* call — analyze, extract,
// finalize, log_triage — set "required" so the LLM protocol itself
// rejects a no-tool-call response. Without this, a model that
// chooses prose over tool-calling (observed on some GLM / MiniMax
// variants, especially with Think Aloud enabled) burns the whole
// continuation retry budget (5+ minutes of wall time) before the
// soft-stop handler finally fails the stage.
//
// Explore stays "auto" because the explorer's ReAct loop legitimately
// alternates between tool-calling and reasoning — forcing a tool
// call on every iteration would push the LLM into redundant reads.
// The empty string falls through to the OpenAI default (also "auto").
func toolChoiceForStage(stage types.PipelineStage) string {
	switch stage {
	case types.StageAnalyze, types.StageExtract, types.StageFinalize,
		types.StageLogTriage, types.StagePerfTriage:
		return "required"
	}
	return ""
}

// emitToolForStage returns the canonical structured-emit tool name for
// stages whose terminal action is a single named tool call. Used by
// the terminal-forcing escalation: when an emit-required stage retry
// fires (EmitStageRetryAttempt > 0), the agent layer switches
// ChatOptions.ToolChoice from the bare "required" string to the
// named-function form `{"type":"function","function":{"name":"<tool>"}}`
// because some providers honor the named form more reliably than the
// "required" string. The empty return falls through to the existing
// "required" string so non-listed stages are unaffected.
//
// Stages with multiple legitimate emit shapes (extract emits both
// emit_answer_symbol and emit_hypothesis_verdict) are NOT listed: the
// "required" string already forces ONE of them, and constraining to a
// single tool would block the other half of the contract. Only
// single-emit stages benefit from named-function escalation.
func emitToolForStage(stage types.PipelineStage) string {
	switch stage {
	case types.StageAnalyze:
		return "emit_analysis"
	case types.StageFinalize:
		return "emit_answer_document"
	case types.StageLogTriage:
		return "emit_log_triage"
	case types.StagePerfTriage:
		return "emit_perf_trace"
	}
	return ""
}

// resolveToolChoice picks the tool_choice wire payload for this
// dispatch. On the happy path it returns the per-stage default from
// toolChoiceForStage. When ctx.EmitStageRetryAttempt > 0 AND the
// stage has a single canonical emit tool, it returns the named-
// function JSON-object form so the model is constrained to that
// specific tool — terminal forcing for the second-or-later attempt
// at an emit-required stage. Generalizes the bug-fix pattern across
// every emit-required single-tool stage rather than analyzer-only.
func resolveToolChoice(ctx *types.AgentContext) string {
	stage := ctx.Stage
	base := toolChoiceForStage(stage)
	if ctx.EmitStageRetryAttempt <= 0 {
		return base
	}
	emitTool := emitToolForStage(stage)
	if emitTool == "" {
		return base
	}
	return fmt.Sprintf(`{"type":"function","function":{"name":%q}}`, emitTool)
}

// Returns true when at least one message was stubbed, so the caller
// can log the event.
// contextPressureDirective renders the per-agent body the watchdog
// injects when the ReAct loop crosses the hard-pressure threshold.
//
// Built on top of the internal/analysis/hint Composer so every
// retry-style directive the orchestrator emits (contract-check
// retries, CGEC dry-run, DAG window hints, and now context
// pressure) shares one format — "**What failed** / **Why it
// failed** / **What I already did** / **How to fix now** /
// **Allowed** / **Do NOT**" — instead of one ad-hoc string per
// producer.
//
// Each agent has a DIFFERENT terminal tool set; the AllowedSet
// named below names ONLY tools the target agent has access to. A
// sibling-stage tool name (e.g. "emit_change_plan" in the
// verifier's directive) would drive the LLM into a tool-not-
// available dead-end and waste the last iteration of a pressure-
// bounded dispatch. Unknown / custom agent names fall through to a
// generic wrap-up Hint so experimental agents still get the force-
// stop nudge rather than a silent empty directive.
//
// Inputs promptBytes / byteBudget / hardRatio are folded into
// WhyItFailed so the LLM sees the specific numeric threshold it
// breached, not a vague "pressure is high."
func contextPressureDirective(name types.AgentName, promptBytes, byteBudget int, hardRatio float64) string {
	ratioPct := float64(promptBytes) / float64(byteBudget) * 100
	h := &hint.Hint{
		WhatFailed: "Context window approaching the model's limit",
		WhyItFailed: fmt.Sprintf(
			"Cumulative prompt bytes %d of %d estimated (%.1f%% ≥ hard threshold %.1f%%); next iteration risks a provider 400.",
			promptBytes, byteBudget, ratioPct, hardRatio*100,
		),
		WhatSystemDid:     "Marked the ReAct loop forceStop=true — the iteration about to fire will be the last, so its output must close the stage.",
		ForbiddenPatterns: contextPressureForbiddenPatterns(name),
	}
	h.ExactFix, h.AllowedSet = contextPressureFixAndAllowed(name)
	return hint.New(hint.DefaultConfig()).Render(h)
}

// contextPressureFixAndAllowed returns the per-agent ExactFix +
// AllowedSet pair for the pressure directive. Split out of the
// parent so tests can assert the per-agent mapping without parsing
// Markdown.
func contextPressureFixAndAllowed(name types.AgentName) (string, []hint.Allowed) {
	switch name {
	case types.AgentAnalyzer:
		return "Call `emit_analysis` now with whatever classification you have (best-effort intent / scenario / complexity / keywords / entities / question_kind / predicates). Downstream stages will adapt.",
			[]hint.Allowed{{Kind: AllowedTerminalTool, Value: "emit_analysis", Hint: "close the analyze stage"}}
	case types.AgentExplorer:
		return "Call `emit_investigation_complete` with `confidence=\"medium\"` and a `reason` explaining why the evidence is best-effort.",
			[]hint.Allowed{{Kind: AllowedTerminalTool, Value: "emit_investigation_complete", Hint: "close the exploration stage with existing evidence"}}
	case types.AgentExtractor:
		return "Emit `emit_answer_symbol` + `emit_hypothesis_verdict` (per hypothesis) with whatever you have. A `lower_bound` completeness claim is honest under pressure — do NOT claim `complete`.",
			[]hint.Allowed{
				{Kind: AllowedTerminalTool, Value: "emit_answer_symbol", Hint: "close the extraction stage (claim lower_bound, not complete)"},
				{Kind: AllowedTerminalTool, Value: "emit_hypothesis_verdict", Hint: "verdict per hypothesis entry"},
			}
	case types.AgentFinalizer:
		return "Call `emit_answer_document` with the cited evidence already collected. If citation grounding rejects an item, use `citation_ref=-1` and move on.",
			[]hint.Allowed{{Kind: AllowedTerminalTool, Value: "emit_answer_document", Hint: "produce the final structured answer now"}}
	case types.AgentLogTriager:
		return "Call `emit_log_triage` with whatever meta, errors and residue you have parsed so far. Incomplete bundles are tolerated by the validator (coverage low but non-zero).",
			[]hint.Allowed{{Kind: AllowedTerminalTool, Value: "emit_log_triage", Hint: "emit best-effort log bundle"}}
	case types.AgentPerfTriager:
		return "Call `emit_perf_trace` with whatever meta, frames, janks, stalls and startup you have parsed so far. An empty frames/janks/stalls set with a meta.summary explaining why is acceptable when the trace genuinely has no perf signal.",
			[]hint.Allowed{{Kind: AllowedTerminalTool, Value: "emit_perf_trace", Hint: "emit best-effort perf bundle"}}
	case types.AgentWriteAnalyzer:
		return "Call `emit_write_analysis` with the task classification you have. `task.kind=misc` and a one-line summary are acceptable defaults when scope or risk are uncertain — the schema prefers a coarse-but-emitted answer over a perfectly-tuned one that never lands.",
			[]hint.Allowed{{Kind: AllowedTerminalTool, Value: "emit_write_analysis", Hint: "emit best-effort task description"}}
	case types.AgentPlanner:
		return "Call `emit_change_plan` with the changes already drafted. If kind=patch units need regeneration, narrow the plan to kind=modify (full bodies) — the pre-flight gate still protects kind=patch units but kind=modify skips it.",
			[]hint.Allowed{{Kind: AllowedTerminalTool, Value: "emit_change_plan", Hint: "close the plan stage with best-effort ChangePlan"}}
	case types.AgentCoder:
		return "Apply the next remaining `plan.target_paths` entry via `apply_patch({path, kind})` — do NOT re-read files. The tool reads content directly from `Mutable.ChangePlan()`; you do not need extra context to make the call.",
			[]hint.Allowed{{Kind: AllowedTerminalTool, Value: "apply_patch", Hint: "advance AppliedSet; content flows from Mutable"}}
	case types.AgentVerifier:
		return "Call `run_tests` with empty suite (run the whole project). Skip any `emit_test_results` narrative — the parser will populate `ChangeReport.Passed` authoritatively.",
			[]hint.Allowed{{Kind: AllowedTerminalTool, Value: "run_tests", Hint: "sync-populate ChangeReport"}}
	}
	return "Produce this stage's terminal tool call now with whatever you have.",
		[]hint.Allowed{{Kind: AllowedTerminalTool, Value: "stage_terminal", Hint: "the tool that closes the current stage"}}
}

// contextPressureForbiddenPatterns returns the **Do NOT** list the
// pressure directive appends — shared core plus per-agent extensions.
// The shared core (investigate / read / search) covers every stage;
// per-agent extensions call out stage-specific temptations the LLM
// historically reaches for under pressure.
func contextPressureForbiddenPatterns(name types.AgentName) []string {
	patterns := []string{
		"call new investigative / read / search tools — they only push us past the wire limit",
		"attempt fresh grounding / citation recovery reads",
	}
	switch name {
	case types.AgentExplorer:
		patterns = append(patterns, "emit more `emit_evidence` items — the closure has enough; drain with investigation_complete")
	case types.AgentExtractor:
		patterns = append(patterns, "claim `complete` when evidence coverage is partial — use `lower_bound`")
	case types.AgentCoder:
		patterns = append(patterns, "re-read the file; `apply_patch` reads content from `Mutable.ChangePlan()` on its own")
	case types.AgentPlanner:
		patterns = append(patterns, "emit complex multi-kind plans; narrow to the single most-valuable change if any one unit is risky")
	}
	return patterns
}

// AllowedTerminalTool is the AllowedKind label the context-pressure
// directive uses for its tool-name entries. The hint.Allowed struct
// renders "`value` — hint (kind)" so the LLM sees "`emit_analysis` — close
// the analyze stage (terminal_tool)" and recognises the category
// separately from file-citation / literal / shape entries other
// callers use.
const AllowedTerminalTool hint.AllowedKind = "terminal_tool"

func pruneToolHistory(messages []llm.Message, budget int) bool {
	total := 0
	cutoff := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "tool" {
			continue
		}
		total += len(messages[i].Content)
		if total > budget {
			cutoff = i
			break
		}
	}
	if cutoff < 0 {
		return false
	}
	pruned := false
	for i := 0; i <= cutoff; i++ {
		if messages[i].Role != "tool" {
			continue
		}
		orig := len(messages[i].Content)
		if orig == 0 {
			continue
		}
		// Already stubbed? Skip so we don't keep shrinking the
		// placeholder on subsequent iterations.
		if strings.HasPrefix(messages[i].Content, "[earlier tool result elided") {
			continue
		}
		messages[i].Content = fmt.Sprintf(
			"[earlier tool result elided — %d bytes. Re-invoke the tool if you need this content again.]",
			orig,
		)
		pruned = true
	}
	return pruned
}

// Execute implements the ReAct (Reason → Act → Observe) loop.
//
// Debug-level trace logging dumps the initial prompt, every assistant
// turn (content + tool calls), every tool result, and the reason for
// loop termination. This was originally added to localize Layer-2
// soft-stop and Layer-3 read_file slice issues during an early
// explorer knowledge-flow investigation, then removed because it was
// noisy on stderr. It is back as
// debug-gated logging so the same trace can be reproduced on demand by
// running with `-log-level debug` without polluting normal runs.
func (b *BaseAgent) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {
	// Pre-flight watchdog around the first schema build — kept here
	// so the diag trace's tool_schemas phase remains observable from
	// outside the loop. The actual schemas used in each iter are
	// re-derived per iter (see iterToolSchemas below) so dynamic
	// gates inside buildToolSchemas (notably
	// answerDocumentPatchBaseAvailable) flip ON as ctx.Mutable
	// accumulates state.
	stopPreflight := b.startPreflightWatchdog(ctx, "tool_schemas")
	_ = b.buildToolSchemas(sk, ctx)
	stopPreflight()

	// Initialize message history
	stopPreflight = b.startPreflightWatchdog(ctx, "initial_messages")
	messages := b.buildInitialMessages(ctx, sk)
	stopPreflight()

	// DIAGNOSTIC — dump initial prompt (debug only).
	// Full content goes to the log file (unlimited); stdout mirror gets truncated.
	for _, m := range messages {
		logging.Debug("[diag %s] INIT msg role=%s len=%d\n%s\n---",
			b.name, m.Role, len(m.Content), m.Content)
	}

	var allToolResults []types.ToolResult
	var allMCPResponses []types.MCPResponse

	// Reset the per-dispatch running tool-result buffer so a fresh
	// dispatch never inherits the previous one's read_file history.
	// The buffer is what in-loop tools (notably emit_evidence's
	// grounder) read to reconstruct the line index — bus.ToolResults
	// alone is too late because applyStageOutput only flushes after
	// ParseOutput.
	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.ResetDispatchToolResults()
	}

	// Loop-control state: snapshot the evaluator's LoopController
	// once (nil when the evaluator does not implement it) and
	// construct a fresh loopPolicyState from the configured
	// LoopPolicy. The state is per-dispatch — BaseAgent.Execute
	// never shares it across calls — so the idle counter, hint
	// budget, and dedup window always start clean.
	loopCtrl, _ := b.eval.(LoopController)
	policyState := newLoopPolicyState(b.deps.LoopPolicy)

	// forceStop is set by a mid-loop LoopPolicy decision that asks
	// BaseAgent to terminate the ReAct loop (e.g. idle-streak
	// force-stop, evaluator StopRequested). A bool flag plus a
	// `break` at the top of the next iteration preserves the
	// pre-refactor control-flow shape without requiring a labeled
	// break inside the tool-batch loop.
	forceStop := false
	finalizerNoToolDraftsPreserved := 0
	historyContentForNoTool := func(resp llm.Response) string {
		content, preserved := contentForNoToolHistoryWithFinalizerDraftBudget(ctx, resp, finalizerNoToolDraftsPreserved)
		finalizerNoToolDraftsPreserved = preserved
		return content
	}

	// ReAct loop
	maxIter := b.deps.MaxIterations
	if ctx != nil && ctx.MaxIterOverride > 0 {
		maxIter = ctx.MaxIterOverride
	}
	for i := 0; i < maxIter; i++ {
		if forceStop {
			break
		}
		// Cancel checkpoint: top of every ReAct iteration. The user's
		// Ctrl+C / `/cancel` from the REPL fires CancelChecker; we
		// unwind immediately rather than starting another LLM call.
		// Phase 1 design: this is the cooperative checkpoint — an
		// LLM call already in flight finishes naturally; we abort
		// before sending the next one. Phase 2 will plumb context
		// into llm.Chat for HTTP-level interruption.
		if b.deps.CancelChecker != nil {
			if err := b.deps.CancelChecker(); err != nil {
				logging.Info("[diag %s] iter=%d phase=cancel cancel checkpoint fired: %v", b.name, i, err)
				return nil, err
			}
		}
		// Prune older "tool" role messages in-place so cumulative tool
		// output never blows the model's context window on long
		// investigations. Runs every iteration because a single late
		// read_file batch can push us over the budget; the stub is
		// idempotent so already-pruned messages are skipped.
		toolHistBudget := b.deps.AgentSettings.MaxToolHistoryBytes
		if toolHistBudget <= 0 {
			toolHistBudget = 150 * 1024
		}
		if pruneToolHistory(messages, toolHistBudget) {
			logging.Debug("[diag %s] iter=%d phase=prune TOOL HISTORY PRUNED (budget=%d bytes)",
				b.name, i, toolHistBudget)
		}

		// Context-pressure watchdog. Measure the post-prune prompt
		// byte total against the adapter's declared context window
		// (× BytesPerToken). The soft ratio logs a warning; the hard
		// ratio sets forceStop and appends a terminal directive to
		// the user message list so the CURRENT iteration (the one
		// about to fire below) is the last. The watchdog is quiet
		// when any input is missing / pathological: zero window,
		// zero budget, zero both ratios.
		//
		// The hard directive is **agent-specific** — each agent's
		// terminal tool set differs, and naming tools an agent does
		// not have access to (e.g. telling a verifier to call
		// emit_answer_symbol) would actively harm the retry. The
		// directive body is composed from contextPressureDirective
		// below, which keys off the agent's name.
		if ctxWindow := b.deps.LLM.MaxContextTokens(); ctxWindow > 0 {
			byteBudget := ctxWindow * types.BytesPerToken
			promptBytes := llm.EstimateMessagesBytes(messages)
			soft := b.deps.AgentSettings.ContextPressureSoftRatio
			hard := b.deps.AgentSettings.ContextPressureHardRatio
			if byteBudget > 0 && (soft > 0 || hard > 0) {
				ratio := float64(promptBytes) / float64(byteBudget)
				switch {
				case hard > 0 && ratio >= hard:
					logging.Warning("[agent %s] context-pressure HARD: iter=%d prompt=%d/%d bytes (%.1f%% ≥ %.1f%%) — force-stopping after this iteration",
						b.name, i, promptBytes, byteBudget, ratio*100, hard*100)
					messages = append(messages, llm.Message{
						Role:    "user",
						Content: contextPressureDirective(b.name, promptBytes, byteBudget, hard),
					})
					forceStop = true
				case soft > 0 && ratio >= soft:
					logging.Warning("[agent %s] context-pressure SOFT: iter=%d prompt=%d/%d bytes (%.1f%% ≥ %.1f%%) — consider terminating soon",
						b.name, i, promptBytes, byteBudget, ratio*100, soft*100)
				}
			}
		}

		// Per-iter tool schema refresh (2026-05-17 T2 hotfix).
		// buildToolSchemas is now invoked PER ITER instead of once at
		// Execute() entry. The auto-gates inside buildToolSchemas
		// (notably the answerDocumentPatchBaseAvailable check at the
		// emit_answer_document_patch line) flip ON only AFTER the
		// first emit lands a rejected draft on
		// ctx.Mutable.AnswerDocumentV2() or rs.PrevEmitJSON — so a
		// once-at-Execute-entry build cannot include the patch tool
		// in iter 0's tool list, and any downstream filter that
		// drops emit_answer_document loses ALL tools.
		//
		// Refreshing per iter keeps the tool surface in sync with the
		// evaluator's state: iter 0 sees [emit_answer_document], iter
		// 1+ sees [emit_answer_document, emit_answer_document_patch]
		// once a rejected draft exists, and a ToolSchemaFilter (T2)
		// can then narrow the surface confident the alternative tool
		// is actually present.
		iterToolSchemas := b.buildToolSchemas(sk, ctx)
		effectiveTools := iterToolSchemas
		if filter, ok := b.eval.(ToolSchemaFilter); ok {
			effectiveTools = filter.FilterToolSchemas(ctx, iterToolSchemas)
		}

		// Reason — call LLM
		telemetry := llm.BuildRequestTelemetry(b.deps.LLM, messages, effectiveTools)
		logging.Debug("[diag %s] iter=%d phase=llm_request model=%s context_tokens_est=%d context_window=%d messages=%d tools=%d",
			b.name, i, telemetry.ModelID, telemetry.ContextTokensEstimate, telemetry.ContextWindowTokens, telemetry.MessageCount, telemetry.ToolCount)
		b.deps.Emit(render.Event{
			Kind:                  render.EventAgentThinking,
			Timestamp:             time.Now(),
			Agent:                 b.name,
			Stage:                 ctx.Stage,
			Iteration:             i,
			ContextTokensEstimate: telemetry.ContextTokensEstimate,
			ContextWindowTokens:   telemetry.ContextWindowTokens,
			ModelID:               telemetry.ModelID,
			ParallelGroupID:       ctx.ParallelGroupID,
			ParallelUnitID:        ctx.ExploreDispatchKey,
		})
		// Streaming preview: when the LLM adapter supports streaming,
		// each content chunk fires onDelta. Buffer the chunks and emit
		// an EventAgentContent event at most every 250ms so the row
		// detail updates live without flooding the renderer with
		// per-token events. Non-streaming adapters never call onDelta.
		streamBuf := newStreamPreviewBuffer(b.deps.Emit, b.name, ctx.Stage, i, ctx.ParallelGroupID, ctx.ExploreDispatchKey)
		// Finalizer-only live summary preview. When this dispatch is
		// the finalize stage's emit_answer_document call, install a
		// SummaryExtractor on the tool-call argument stream so the
		// renderer can show the `summary` field as it arrives. This is
		// a PASSIVE read-side tap — onToolCallDelta does not influence
		// the resp.ToolCalls accumulator, so the orchestrator's
		// downstream parse of the same call yields byte-identical
		// AnswerDocument with vs without preview.
		var summaryPreview *finalizePreviewHook
		var onToolCallDelta func(int, string, string)
		if ctx.Stage == types.StageFinalize {
			summaryPreview = newFinalizePreviewHook(b.deps.Emit)
			onToolCallDelta = summaryPreview.onToolCallDelta
		}
		// Forward L1 retry / L2 fallback signals as renderer events so
		// the dock status row can flip to "重试中 / 切换 provider 中"
		// during the backoff window. Without this the dock keeps
		// showing "请求模型中" during a 60-second retry window — the
		// user has no idea the adapter is sleeping.
		emit := b.deps.Emit
		stage := ctx.Stage
		agentName := b.name
		onRetry := func(attempt int, delay time.Duration, reason string) {
			if emit == nil {
				return
			}
			emit(render.Event{
				Kind:            render.EventAdapterRetry,
				Timestamp:       time.Now(),
				Stage:           stage,
				Agent:           agentName,
				RetryAttempt:    attempt,
				RetryDelay:      delay,
				RetryReason:     reason,
				ParallelGroupID: ctx.ParallelGroupID,
				ParallelUnitID:  ctx.ExploreDispatchKey,
			})
		}
		onFallback := func(from, to, reason string) {
			if emit == nil {
				return
			}
			emit(render.Event{
				Kind:            render.EventAdapterFallback,
				Timestamp:       time.Now(),
				Stage:           stage,
				Agent:           agentName,
				FallbackFrom:    from,
				FallbackTo:      to,
				RetryReason:     reason,
				ParallelGroupID: ctx.ParallelGroupID,
				ParallelUnitID:  ctx.ExploreDispatchKey,
			})
		}
		resp, err := b.deps.LLM.Chat(ctx.Context(), messages, effectiveTools, llm.ChatOptions{
			ToolChoice:      resolveToolChoice(ctx),
			OnContentDelta:  streamBuf.onDelta,
			OnToolCallDelta: onToolCallDelta,
			OnRetry:         onRetry,
			OnFallback:      onFallback,
		})
		streamBuf.flush()
		// Flush any throttled-out summary preview chunks before the
		// orchestrator emits its EventLivePreviewClear. Nil-safe so
		// non-finalize stages don't pay any cost.
		summaryPreview.flush()
		b.deps.Emit(render.Event{
			Kind:            render.EventAgentResponse,
			Timestamp:       time.Now(),
			Agent:           b.name,
			Stage:           ctx.Stage,
			Iteration:       i,
			ParallelGroupID: ctx.ParallelGroupID,
			ParallelUnitID:  ctx.ExploreDispatchKey,
		})
		if err != nil {
			// Salvage accumulated side-effects before bubbling the LLM
			// error. Without this, an upstream 429 / 5xx / context-length
			// spike in the middle of a long explore window erases every
			// read_file / evidence / investigation-note accumulated so
			// far: explorer.ParseOutput is the ONLY producer of
			// TurnAArtifacts, and returning early skips it. The
			// downstream extractor then sees `ta == nil`, renders
			// "No transcript available", and trips R4 fail-loud for
			// a worthless final answer.
			//
			// Side-effect only: ParseOutput writes onto the shared
			// ctx.Mutable (SetTurnAArtifacts / EvidenceClosure / etc.).
			// The returned StageOutput is discarded; the original LLM
			// error still propagates so the orchestrator treats the
			// window as failed and the force-finalize path engages.
			//
			// Finalizer preview chunks remain UI-only on transient
			// EOF/stall. Do not synthesize an assistant message from a
			// partial emit_answer_document argument: the scheduler's
			// transient retry lane must ask the model to re-emit a
			// complete structured document (or a typed inability),
			// while salvage below preserves deterministic tool
			// results / TurnA artifacts for the retry. Using partial
			// prose here would turn a transport failure into a
			// system-filled answer path.
			b.salvagePartialDispatch(ctx, messages, allToolResults, allMCPResponses, i, err)
			return &StageOutput{
				Error:        fmt.Sprintf("LLM call failed: %v", err),
				MissingPiece: ctx.MissingPiece,
			}, err
		}

		resp.ToolCalls = b.normalizeToolCallParams(resp.ToolCalls, effectiveTools)

		// DIAGNOSTIC — dump assistant response (debug only).
		logging.Debug("[diag %s] iter=%d ASSISTANT content_len=%d tool_calls=%d",
			b.name, i, len(resp.Content), len(resp.ToolCalls))
		if resp.Content != "" {
			logging.Debug("[diag %s] iter=%d ASSISTANT content:\n%s\n---",
				b.name, i, truncForLog(resp.Content, 4000))
			// Surface the LLM's reasoning text so the user can follow
			// the investigation in real time. The renderer decides how
			// to present it (dimmed text above the spinner, etc.).
			b.deps.Emit(render.Event{
				Kind:      render.EventAgentReasoning,
				Timestamp: time.Now(),
				Agent:     b.name,
				Stage:     ctx.Stage,
				Iteration: i,
				Reasoning: resp.Content,
			})
		}
		if len(resp.ToolCalls) > 0 {
			firstTool := resp.ToolCalls[0]
			b.deps.Emit(render.Event{
				Kind:          render.EventAgentToolCallBatch,
				Timestamp:     time.Now(),
				Agent:         b.name,
				Stage:         ctx.Stage,
				Iteration:     i,
				ToolName:      firstTool.Name,
				ToolDetail:    toolDetailForCall(firstTool),
				ToolCallCount: len(resp.ToolCalls),
				ToolNames:     toolCallNames(resp.ToolCalls),
			})
		}
		for j, tc := range resp.ToolCalls {
			logging.Debug("[diag %s] iter=%d phase=toolcall call[%d] tool=%s params=%s",
				b.name, i, j, tc.Name, string(tc.Params))
		}
		historyToolCalls := sanitizeToolCallsForHistory(resp.ToolCalls)

		emitRequiredNoToolNotice := func() {
			// Must-emit stage visibility: when the stage's tool_choice
			// is "required" but the provider still returned zero
			// tool_calls, emit the notice only on branches that
			// actually continue. Protocol-aware stages may accept a
			// no-tool soft stop (for example, an extractor dispatch
			// whose controller already consumed the prose), and showing
			// "retrying" before that decision misleads the REPL.
			stage := types.PipelineStage("")
			if ctx != nil {
				stage = ctx.Stage
			}
			if toolChoiceForStage(stage) != "required" || len(resp.ToolCalls) != 0 {
				return
			}
			lang := ""
			if ctx != nil {
				lang = ctx.Language
			}
			b.deps.Emit(render.Event{
				Kind:       render.EventOrchestratorNotice,
				Timestamp:  time.Now(),
				Agent:      "orchestrator",
				Stage:      stage,
				Iteration:  i,
				NoticeKind: render.NoticeNoToolCall,
				Reasoning:  softNoToolCallMessage(lang),
			})
		}

		routeNoToolThroughController := shouldRouteNoToolThroughStageProtocolController(ctx, resp, loopCtrl)
		routeEmptyNoToolThroughController := shouldRouteEmptyNoToolThroughStageProtocolController(ctx, resp, loopCtrl)
		if routeNoToolThroughController {
			logging.Debug("[diag %s] iter=%d phase=protocol_softstop route no-tool content through %s controller before evaluator stop",
				b.name, i, ctx.Stage)
		}
		if routeEmptyNoToolThroughController {
			logging.Debug("[diag %s] iter=%d phase=protocol_softstop route empty required-tool response through %s controller before evaluator stop",
				b.name, i, ctx.Stage)
		}

		// Hard stop from the evaluator (e.g., finalizer always stops at iter=0).
		if !routeNoToolThroughController && !routeEmptyNoToolThroughController && b.eval.ShouldStop(resp, i) {
			messages = append(messages, llm.Message{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: historyToolCalls,
			})
			logging.Debug("[diag %s] STOP at iter=%d (eval)", b.name, i)
			break
		}

		// Soft stop: LLM produced content but called no tools. By
		// default this is treated as voluntary completion. Agents that
		// implement LoopController can observe the soft-stop and vote
		// to inject a continuation hint instead — this exists because
		// the LLM often "thinks aloud" mid-investigation ("I'll check
		// X next") without acting, and the default break would
		// silently accept that as the final answer.
		if len(resp.ToolCalls) == 0 {
			// Empty response (no content, no tools) — treat as
			// a protocol failure on required-tool stages before
			// falling back to voluntary stop. This lets analyzers,
			// extractors, and finalizers inject their normal
			// "call the structured emit tool" repair hint instead of
			// shipping an empty answer shell.
			if resp.Content == "" {
				if routeEmptyNoToolThroughController {
					idle, conts, midLoopInjects := policyState.snapshot()
					obs := LoopObservation{
						Phase:              PhaseSoftStop,
						Iteration:          i,
						Response:           resp,
						AllToolResults:     allToolResults,
						IdleStreak:         idle,
						ContinuationsUsed:  conts,
						MidLoopInjectsUsed: midLoopInjects,
					}
					sig := loopCtrl.Observe(ctx, obs)
					result := policyState.Apply(PhaseSoftStop, obs, sig)
					logging.Debug("[diag %s] iter=%d phase=empty_softstop_signal hint=%t progress=%t stop=%t key=%q → %s (%s)",
						b.name, i, sig.HintRequested, sig.Progress, sig.StopRequested,
						sig.HintKey, result.Outcome, result.Reason)
					if result.Outcome == OutcomeInjectHint {
						emitRequiredNoToolNotice()
						messages = append(messages, llm.Message{
							Role:      "assistant",
							Content:   "",
							ToolCalls: historyToolCalls,
						})
						messages = append(messages, llm.Message{
							Role:    "user",
							Content: result.Hint,
						})
						logging.Debug("[diag %s] iter=%d phase=empty_softstop_inject SOFT-STOP inject len=%d:\n%s\n---",
							b.name, i, len(result.Hint), logging.Truncate(result.Hint, logging.HintBodyMax))
						continue
					}
				}
				logging.Debug("[diag %s] STOP at iter=%d (empty response)", b.name, i)
				break
			}
			// Detect LLM writing tool-call JSON in content instead of
			// using the function-calling mechanism. This happens when
			// the Think Aloud directive leads the LLM to embed tool
			// calls as markdown JSON blocks. Inject a correction hint
			// so the LLM retries with real tool_use blocks.
			if looksLikeEmbeddedToolCall(resp.Content) {
				logging.Debug("[diag %s] iter=%d phase=embedded_correction detected embedded tool-call JSON in content — injecting correction", b.name, i)
				emitRequiredNoToolNotice()
				messages = append(messages, llm.Message{
					Role:    "assistant",
					Content: historyContentForNoTool(resp),
				})
				messages = append(messages, llm.Message{
					Role: "user",
					Content: "You wrote tool-call JSON in your text content, but that does NOT execute the tool. " +
						"You MUST use the function-calling mechanism (tool_use blocks) to actually call tools. " +
						"Call the tool(s) again using the proper function-calling API — do NOT write JSON in text.",
				})
				continue
			}
			if loopCtrl != nil {
				idle, conts, midLoopInjects := policyState.snapshot()
				obs := LoopObservation{
					Phase:              PhaseSoftStop,
					Iteration:          i,
					Response:           resp,
					AllToolResults:     allToolResults,
					IdleStreak:         idle,
					ContinuationsUsed:  conts,
					MidLoopInjectsUsed: midLoopInjects,
				}
				sig := loopCtrl.Observe(ctx, obs)
				result := policyState.Apply(PhaseSoftStop, obs, sig)
				// `key=%q` surfaces the evaluator's HintKey so
				// post-hoc trace analysis can pinpoint which
				// detection branch fired without source-reading
				// detective work. `result.Reason` is independently
				// useful (carries the policy-layer rejection
				// explanation for dropped hints), so both fields
				// stay on the same line.
				logging.Debug("[diag %s] iter=%d phase=softstop_signal hint=%t progress=%t stop=%t key=%q → %s (%s)",
					b.name, i, sig.HintRequested, sig.Progress, sig.StopRequested,
					sig.HintKey, result.Outcome, result.Reason)
				if result.Outcome == OutcomeInjectHint {
					emitRequiredNoToolNotice()
					messages = append(messages, llm.Message{
						Role:      "assistant",
						Content:   historyContentForNoTool(resp),
						ToolCalls: historyToolCalls,
					})
					messages = append(messages, llm.Message{
						Role:    "user",
						Content: result.Hint,
					})
					logging.Debug("[diag %s] iter=%d phase=softstop_inject SOFT-STOP inject len=%d:\n%s\n---",
						b.name, i, len(result.Hint), logging.Truncate(result.Hint, logging.HintBodyMax))
					continue
				}
				// OutcomeStop or OutcomeContinue at PhaseSoftStop
				// both terminate the ReAct loop — the policy's
				// soft-stop semantics treat "no hint" as accept.
			}
			messages = append(messages, llm.Message{
				Role:      "assistant",
				Content:   historyContentForNoTool(resp),
				ToolCalls: historyToolCalls,
			})
			logging.Debug("[diag %s] STOP at iter=%d (soft)", b.name, i)
			break
		}

		// Record assistant message with tool calls
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: historyToolCalls,
		})

		// Act — execute tool calls. When the batch contains only
		// read-safe tools (no emit_* / propose_*), execute them in
		// PARALLEL for latency reduction (S1). When any tool writes
		// to MutableState (emit_evidence, emit_analysis, etc.), fall
		// back to SEQUENTIAL execution because the grounder may need
		// DispatchToolResults from a prior read_file in the same batch.
		var lastToolResultPtr *types.ToolResult
		if canParallelizeToolBatch(resp.ToolCalls) && len(resp.ToolCalls) > 1 {
			// ── PARALLEL PATH ──
			type toolExecResult struct {
				result  *types.ToolResult
				mcpResp *types.MCPResponse
			}
			execResults := make([]toolExecResult, len(resp.ToolCalls))
			toolStarts := make([]time.Time, len(resp.ToolCalls))

			// Emit start events sequentially (UI ordering).
			for idx, tc := range resp.ToolCalls {
				toolStarts[idx] = time.Now()
				b.deps.Emit(render.Event{
					Kind:            render.EventToolCallStart,
					Timestamp:       toolStarts[idx],
					Agent:           b.name,
					Stage:           ctx.Stage,
					ToolName:        tc.Name,
					ToolCallID:      tc.ID,
					ToolDetail:      toolDetailForCall(tc),
					ParallelGroupID: ctx.ParallelGroupID,
					ParallelUnitID:  ctx.ExploreDispatchKey,
				})
			}

			// Execute all tools concurrently.
			var wg sync.WaitGroup
			for idx, tc := range resp.ToolCalls {
				wg.Add(1)
				go func(i int, call llm.ToolCall) {
					defer wg.Done()
					r, m := b.executeTool(ctx, call)
					execResults[i] = toolExecResult{r, m}
				}(idx, tc)
			}
			wg.Wait()

			// Process results sequentially (message ordering).
			for idx, tc := range resp.ToolCalls {
				er := execResults[idx]
				toolOK := false
				if er.result != nil {
					toolOK = er.result.Success
					allToolResults = append(allToolResults, *er.result)
					lastToolResultPtr = &allToolResults[len(allToolResults)-1]
					if ctx != nil && ctx.Mutable != nil {
						ctx.Mutable.AppendDispatchToolResult(*er.result)
					}
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    er.result.Summary,
						ToolCallID: tc.ID,
					})
					logging.Debug("[diag %s] iter=%d phase=toolresult TOOLRESULT %s ok=%v len=%d:\n%s\n---",
						b.name, i, er.result.ToolName, er.result.Success, len(er.result.Summary),
						truncForLog(er.result.Summary, 2000))
				}
				if er.mcpResp != nil {
					toolOK = er.mcpResp.Success
					allMCPResponses = append(allMCPResponses, *er.mcpResp)
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    er.mcpResp.Summary,
						ToolCallID: tc.ID,
					})
				}
				b.deps.Emit(render.Event{
					Kind:              render.EventToolCallEnd,
					Timestamp:         time.Now(),
					Agent:             b.name,
					Stage:             ctx.Stage,
					ToolName:          tc.Name,
					ToolCallID:        tc.ID,
					ToolDetail:        toolDetailForCall(tc),
					ToolParamsJSON:    string(tc.Params),
					ToolOK:            toolOK,
					ToolTime:          time.Since(toolStarts[idx]),
					ToolResultSummary: toolResultSummary(er.result),
					ParallelGroupID:   ctx.ParallelGroupID,
					ParallelUnitID:    ctx.ExploreDispatchKey,
				})
			}
		} else {
			// ── SEQUENTIAL PATH (original) ──
			for _, tc := range resp.ToolCalls {
				toolStart := time.Now()
				b.deps.Emit(render.Event{
					Kind:            render.EventToolCallStart,
					Timestamp:       toolStart,
					Agent:           b.name,
					Stage:           ctx.Stage,
					ToolName:        tc.Name,
					ToolCallID:      tc.ID,
					ToolDetail:      toolDetailForCall(tc),
					ParallelGroupID: ctx.ParallelGroupID,
					ParallelUnitID:  ctx.ExploreDispatchKey,
				})

				result, mcpResp := b.executeTool(ctx, tc)

				toolOK := false
				if result != nil {
					toolOK = result.Success
					allToolResults = append(allToolResults, *result)
					lastToolResultPtr = &allToolResults[len(allToolResults)-1]
					if ctx != nil && ctx.Mutable != nil {
						ctx.Mutable.AppendDispatchToolResult(*result)
					}
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    result.Summary,
						ToolCallID: tc.ID,
					})
					logging.Debug("[diag %s] iter=%d phase=toolresult TOOLRESULT %s ok=%v len=%d:\n%s\n---",
						b.name, i, result.ToolName, result.Success, len(result.Summary),
						truncForLog(result.Summary, 2000))
				}
				if mcpResp != nil {
					toolOK = mcpResp.Success
					allMCPResponses = append(allMCPResponses, *mcpResp)
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    mcpResp.Summary,
						ToolCallID: tc.ID,
					})
				}

				b.deps.Emit(render.Event{
					Kind:              render.EventToolCallEnd,
					Timestamp:         time.Now(),
					Agent:             b.name,
					Stage:             ctx.Stage,
					ToolName:          tc.Name,
					ToolCallID:        tc.ID,
					ToolDetail:        toolDetailForCall(tc),
					ToolParamsJSON:    string(tc.Params),
					ToolOK:            toolOK,
					ToolTime:          time.Since(toolStart),
					ToolResultSummary: toolResultSummary(result),
					ParallelGroupID:   ctx.ParallelGroupID,
					ParallelUnitID:    ctx.ExploreDispatchKey,
				})
			}
		}

		// Mid-loop check: give the LoopController a chance to detect
		// drift and inject a corrective hint between tool batches.
		// Unlike PhaseSoftStop (which only fires when the LLM
		// returned content with no tools), PhaseMidLoop fires on
		// every iteration where the LLM is still actively calling
		// tools — closing the blind spot where the LLM keeps
		// tool-calling in the wrong direction. Throttling, dedup,
		// budget, and idle-streak force-stop all live in LoopPolicy
		// so the evaluator's Observe method stays detection-only.
		if loopCtrl != nil {
			idle, conts, midLoopInjects := policyState.snapshot()
			obs := LoopObservation{
				Phase:              PhaseMidLoop,
				Iteration:          i,
				Response:           resp,
				LastToolResult:     lastToolResultPtr,
				AllToolResults:     allToolResults,
				IdleStreak:         idle,
				ContinuationsUsed:  conts,
				MidLoopInjectsUsed: midLoopInjects,
			}
			sig := loopCtrl.Observe(ctx, obs)
			result := policyState.Apply(PhaseMidLoop, obs, sig)
			// Symmetric signal line with the soft-stop path above,
			// so `rg 'SOFT-STOP signal|MIDLOOP signal' logs/` yields
			// a uniform stream of controller-vote events across
			// both loop phases. `key=%q` exposes the evaluator's
			// HintKey — without it, trace analysis has to grep the
			// evaluator source to figure out which detection
			// branch fired.
			logging.Debug("[diag %s] iter=%d phase=midloop_signal hint=%t progress=%t stop=%t key=%q → %s (%s)",
				b.name, i, sig.HintRequested, sig.Progress, sig.StopRequested,
				sig.HintKey, result.Outcome, result.Reason)
			switch result.Outcome {
			case OutcomeInjectHint:
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: result.Hint,
				})
				logging.Debug("[diag %s] iter=%d phase=midloop_inject MIDLOOP inject len=%d:\n%s\n---",
					b.name, i, len(result.Hint), logging.Truncate(result.Hint, logging.HintBodyMax))
			case OutcomeStop:
				// Policy decided to terminate — e.g. idle-streak
				// force-stop or evaluator StopRequested. Record the
				// reason for the trace and break out of the ReAct
				// loop on the next iteration's top check.
				logging.Debug("[diag %s] iter=%d phase=midloop_force_stop MIDLOOP force-stop: %s",
					b.name, i, result.Reason)
				forceStop = true
			}
			// Per-key cap-saturation escalation. One-shot per
			// (dispatch, key) — the loop-policy fills
			// CapSaturatedHintKey only on the first cap-reached
			// rejection for a given key. Surfacing the event keeps
			// the loop from silently burning its remaining iteration
			// quota on a gate the model is structurally unable to
			// satisfy: the user sees a typed notice; the per-Run
			// LearningFailure feeds cross-Run telemetry; the typed
			// marker on Mut is what the success-criteria layer will
			// read to pivot fallback earlier.
			// docs/design/post_phase2a_forensic_followups.md §2.1.G #4 + #7.
			if result.CapSaturatedHintKey != "" {
				b.escalateMidLoopCapSaturation(ctx, i, result.CapSaturatedHintKey, result.Reason)
			}
		}
	}

	// Parse final output. This phase is local CPU work, not an LLM
	// request, but it can be non-trivial on large evidence pools. Keep
	// the same watchdog shape as prompt preflight so stalls are
	// diagnosable and Ctrl+C/cancel checkpoints have a visible owner.
	stopPreflight = b.startPreflightWatchdog(ctx, "parse_output")
	output, err := b.eval.ParseOutput(ctx, messages, allToolResults, allMCPResponses)
	stopPreflight()
	if err != nil {
		return &StageOutput{
			Error:        fmt.Sprintf("output parsing failed: %v", err),
			ToolResults:  allToolResults,
			MCPResponses: allMCPResponses,
			MissingPiece: ctx.MissingPiece,
		}, err
	}

	output.ToolResults = allToolResults
	output.MCPResponses = allMCPResponses
	stopPreflight = b.startPreflightWatchdog(ctx, "determine_missing_piece")
	output.MissingPiece = b.eval.DetermineMissingPiece(ctx, output)
	stopPreflight()

	// Auto-capture the LLM's synthesized narrative for downstream
	// stages. We walk the message history once for the last non-empty
	// assistant message; the evaluator may have already populated
	// StageReport itself, in which case we leave it alone.
	if output.StageReport == "" {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "assistant" && messages[i].Content != "" {
				output.StageReport = messages[i].Content
				break
			}
		}
	}

	return output, nil
}

// readFilePathMissMarkers lists substrings that identify a read_file
// failure caused by the LLM naming a path that does not exist (or
// pointing at a directory instead of a file). Matched case-insensitively
// against the tool's Summary. Kept narrow on purpose: refund fires
// only for the bucket of errors that a retry on a corrected path would
// fix — permission denied, EIO, mmap failures, etc. stay charged
// because they signal something the LLM cannot trivially work around.
var readFilePathMissMarkers = []string{
	"no such file or directory",
	"no such file",
	"is a directory",
	"file does not exist",
}

// isReadFilePathMiss reports whether a tool result came back failed
// from read_file for a reason the LLM can self-correct by supplying
// a different path on the next iteration.
func isReadFilePathMiss(name string, r *types.ToolResult) bool {
	if r == nil || r.Success || r.ToolName == "" {
		return false
	}
	if types.CanonicalToolName(name) != "read_file" {
		return false
	}
	summary := strings.ToLower(r.Summary)
	for _, m := range readFilePathMissMarkers {
		if strings.Contains(summary, m) {
			return true
		}
	}
	return false
}

func observationOnlyRuntimeBlocksTool(ctx *types.AgentContext, name string) bool {
	if ctx == nil || ctx.Stage != types.StageExplore {
		return false
	}
	if !observationOnlyRuntimeArtifactForExplorer(ctx) {
		return false
	}
	switch types.CanonicalToolName(name) {
	case "read_file", "grep", "repo_map", "list_files", "exec_command",
		"git_diff", "git_log", "propose_sub_agents", "run_tests":
		return true
	default:
		return false
	}
}

func validateObservationOnlyRuntimeToolCall(ctx *types.AgentContext, tc llm.ToolCall) *types.ToolResult {
	if !observationOnlyRuntimeBlocksTool(ctx, tc.Name) {
		return nil
	}
	canonical := types.CanonicalToolName(tc.Name)
	msg := fmt.Sprintf(
		"%s rejected: this is an observation-only runtime artifact question. "+
			"Do not search or read the current repository; use the structured Log/Trace Triage facts already in the prompt and close with emit_investigation_complete.",
		tc.Name)
	return &types.ToolResult{
		ToolName:  tc.Name,
		Summary:   msg,
		Success:   false,
		Timestamp: time.Now(),
		Repair: &types.ToolRepair{
			Code: "observation_only_runtime_repo_tool",
			Hint: "The attached runtime artifact is the answer source for this request. Do not substitute current-repo files, fixtures, helper symbols, sub-agents, or shell output for the artifact. Re-read the Log/Trace Triage section and call emit_investigation_complete with an evidence_floor_waiver when the artifact is sufficient.",
			Metadata: map[string]string{
				"tool":   canonical,
				"policy": "runtime_observation_only",
			},
		},
	}
}

// salvagePartialDispatch runs the evaluator's ParseOutput for its
// side-effects only, so a mid-loop LLM failure does not erase work
// the dispatch already accumulated on shared ctx.Mutable. The
// returned StageOutput is discarded — the caller still surfaces the
// LLM error. A panic inside ParseOutput on incomplete data is
// recovered and logged so the original error path remains reliable.
func (b *BaseAgent) salvagePartialDispatch(
	ctx *types.AgentContext,
	messages []llm.Message,
	toolResults []types.ToolResult,
	mcpResponses []types.MCPResponse,
	iter int,
	llmErr error,
) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warning("[agent/%s] ParseOutput panic during mid-loop salvage at iter=%d (ignored): %v",
				b.name, iter, r)
		}
	}()
	logging.Warning("[agent/%s] LLM call failed at iter=%d (%v) — salvaging accumulated artifacts via ParseOutput",
		b.name, iter, llmErr)
	_, _ = b.eval.ParseOutput(ctx, messages, toolResults, mcpResponses)
}

// buildToolSchemas assembles the tool-schema slice handed to the LLM
// for this dispatch. The output is the UNION of:
//
//  1. Tools named in sk.ToolSuggestions (the per-skill allowlist)
//  2. MCP tools from all configured servers
//  3. propose_sub_agents (auto-injected when a sub-agent of the same
//     name as the current agent is registered)
//
// (1) is the P2.1 Phase 12 stage-local tool whitelist mechanism. The
// extractor's extract-skill lists ONLY emit_evidence /
// emit_answer_symbol / emit_hypothesis_verdict in its ToolSuggestions
// (cmd/root.go P2.1 bootstrap block), so the extractor's LLM call
// physically cannot see read_file / grep / repo_map — they are never
// added to the schema list and the LLM's tool-selection mechanism has
// no way to invoke them. (2) does not affect the extractor because
// Turn B should not have MCP servers configured. (3) is also
// inactive for the extractor because there is no sub-agent named
// "extractor" in RegisterDefaultSubAgents. Therefore Turn B's LLM
// sees exactly the three emit_* tools — no more code is needed for
// Phase 12 beyond this documentation and the tests that pin the
// invariant.
func (b *BaseAgent) buildToolSchemas(sk *skill.Config, ctx *types.AgentContext) []llm.ToolSchema {
	var schemas []llm.ToolSchema

	// Add suggested tools from skill. Tools with high confidence
	// (evidence-bearing: grep, read_file, exec_command, …) get a
	// "[high-confidence]" tag appended to their description so the
	// LLM can see at schema-selection time which tools produce
	// citable evidence vs. navigation hints or side-effects.
	if b.deps.Tools != nil {
		for _, toolName := range sk.ToolSuggestions {
			if observationOnlyRuntimeBlocksTool(ctx, toolName) {
				continue
			}
			if toolName == "emit_answer_document_patch" && !answerDocumentPatchBaseAvailable(ctx, nil) {
				continue
			}
			t, err := b.deps.Tools.Get(toolName)
			if err != nil {
				continue
			}
			desc := t.Description()
			if c := t.Confidence(); c >= 0.8 {
				desc += " [high-confidence evidence]"
			} else if c >= 0.3 {
				desc += " [navigation index — verify with evidence tools]"
			}
			// emit_answer_document supports per-dispatch schema
			// projection: drop fields the AnswerSemanticView says
			// this dispatch will not need (no diagram → no
			// edge_anchors, no exact-absence → no exact_resolution,
			// etc.) so the LLM only sees the surface its contract
			// actually uses.
			params := t.Parameters()
			if ead, ok := t.(*tool.EmitAnswerDocument); ok {
				params = ead.ParametersFor(ctx)
			}
			schemas = append(schemas, llm.ToolSchema{
				Name:        t.Name(),
				Description: desc,
				Parameters:  params,
			})
		}
	}

	// Add MCP tools. Observation-only runtime artifact answers should
	// close from the structured triage payload; exposing extra tool
	// surfaces invites look-alike repo / fixture substitution.
	if b.deps.MCPServers != nil && !observationOnlyRuntimeArtifactForExplorer(ctx) {
		for _, ts := range b.deps.MCPServers.ListAllTools() {
			schemas = append(schemas, llm.ToolSchema{
				Name:        ts.Name,
				Description: ts.Description,
				Parameters:  ts.Parameters,
			})
		}
	}

	// Auto-inject propose_sub_agents if a sub-agent with the same name as this
	// agent is registered. The schema's sub_agent enum is scoped to [self name],
	// so the agent can only propose sub-tasks for its own kind.
	if b.deps.SubAgents != nil && b.deps.Tools != nil {
		if _, err := b.deps.SubAgents.Get(string(b.name)); err == nil {
			if observationOnlyRuntimeBlocksTool(ctx, "propose_sub_agents") {
				return schemas
			}
			if t, err := b.deps.Tools.Get("propose_sub_agents"); err == nil {
				params := t.Parameters()
				if psa, ok := t.(*tool.ProposeSubAgents); ok {
					params = psa.SchemaFor(string(b.name))
				}
				schemas = append(schemas, llm.ToolSchema{
					Name:        t.Name(),
					Description: t.Description(),
					Parameters:  params,
				})
			}
		}
	}

	return schemas
}

func (b *BaseAgent) normalizeToolCallParams(calls []llm.ToolCall, schemas []llm.ToolSchema) []llm.ToolCall {
	if len(calls) == 0 || len(schemas) == 0 || b == nil || b.deps == nil {
		return calls
	}
	cfg := b.toolParamCompatConfig()
	mode := cfg.NormalizedMode()
	if mode != types.ToolParamCompatAudit && mode != types.ToolParamCompatRepair {
		return calls
	}
	byName := make(map[string]json.RawMessage, len(schemas))
	for _, schema := range schemas {
		if strings.TrimSpace(schema.Name) == "" || len(schema.Parameters) == 0 {
			continue
		}
		if _, exists := byName[schema.Name]; !exists {
			byName[schema.Name] = schema.Parameters
		}
	}
	if len(byName) == 0 {
		return calls
	}

	var out []llm.ToolCall
	for i, call := range calls {
		current := call.Params
		var changed bool
		var summaries []string
		schema, ok := byName[call.Name]
		if ok {
			normalized, report := toolparam.Normalize(call.Params, schema, cfg)
			if report.Changed() {
				if mode == types.ToolParamCompatAudit {
					logging.Info("[tool_param_compat] agent=%s tool=%s audit repairable: %s",
						b.name, call.Name, report.Summary(6))
					continue
				}
				current = normalized
				changed = true
				summaries = append(summaries, report.Summary(6))
			}
		}
		if mode == types.ToolParamCompatRepair {
			if patched, notes, ok := normalizeKnownLocalModelToolParams(call.Name, current); ok {
				current = patched
				changed = true
				summaries = append(summaries, strings.Join(notes, "; "))
			}
		}
		if !changed {
			continue
		}
		if out == nil {
			out = append([]llm.ToolCall(nil), calls...)
		}
		out[i].Params = current
		logging.Warning("[tool_param_compat] agent=%s tool=%s params normalized: %s",
			b.name, call.Name, strings.Join(summaries, "; "))
	}
	if out == nil {
		return calls
	}
	return out
}

func normalizeKnownLocalModelToolParams(toolName string, raw json.RawMessage) (json.RawMessage, []string, bool) {
	switch toolName {
	case "emit_analysis":
		return normalizeEmitAnalysisLocalModelParams(raw)
	default:
		return raw, nil, false
	}
}

func normalizeEmitAnalysisLocalModelParams(raw json.RawMessage) (json.RawMessage, []string, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || len(obj) == 0 {
		return raw, nil, false
	}
	defaults := map[string]json.RawMessage{
		"answer_role_profile":       json.RawMessage(`{"is_role_binding_requested":false,"confidence":0.5}`),
		"error_granularity_profile": json.RawMessage(`{"is_granularity_question":false,"confidence":0.5}`),
	}
	var repaired []string
	for field, value := range defaults {
		current, exists := obj[field]
		if exists && string(bytesTrimSpace(current)) != "null" {
			continue
		}
		obj[field] = value
		repaired = append(repaired, "$."+field+" missing->default_false_profile via emit_analysis_required_profile_default")
	}
	if len(repaired) == 0 {
		return raw, nil, false
	}
	patched, err := json.Marshal(obj)
	if err != nil {
		return raw, nil, false
	}
	return patched, repaired, true
}

func bytesTrimSpace(raw json.RawMessage) []byte {
	start := 0
	for start < len(raw) {
		switch raw[start] {
		case ' ', '\t', '\n', '\r':
			start++
		default:
			goto endStart
		}
	}
endStart:
	end := len(raw)
	for end > start {
		switch raw[end-1] {
		case ' ', '\t', '\n', '\r':
			end--
		default:
			goto done
		}
	}
done:
	return raw[start:end]
}

func (b *BaseAgent) toolParamCompatConfig() types.ToolParamCompatConfig {
	if b == nil || b.deps == nil || len(b.deps.ToolParamCompatByAgent) == 0 {
		return types.ToolParamCompatConfig{}
	}
	if cfg, ok := b.deps.ToolParamCompatByAgent[b.name]; ok {
		return cfg
	}
	return types.ToolParamCompatConfig{}
}

func (b *BaseAgent) normalizeAnalyzerPrescanGrepCompat(ctx *types.AgentContext, tc llm.ToolCall) (llm.ToolCall, bool) {
	if ctx == nil || ctx.Stage != types.StageAnalyze || tc.Name != "grep" {
		return tc, false
	}
	if b == nil || b.toolParamCompatConfig().NormalizedMode() != types.ToolParamCompatRepair {
		return tc, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(tc.Params, &obj); err != nil || len(obj) == 0 {
		return tc, false
	}
	if raw, ok := obj["files_only"]; ok && string(bytesTrimSpace(raw)) == "true" {
		return tc, false
	}
	obj["files_only"] = json.RawMessage(`true`)
	patched, err := json.Marshal(obj)
	if err != nil || !json.Valid(patched) {
		return tc, false
	}
	out := tc
	out.Params = patched
	logging.Warning("[tool_param_compat] agent=%s tool=grep analyze-stage params normalized: $.files_only missing/false->true via analyzer_prescan_files_only",
		b.name)
	return out, true
}

func analyzerTerminalEmitOnly(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.Stage != types.StageAnalyze {
		return false
	}
	if ctx.EmitStageRetryAttempt > 0 {
		return true
	}
	if ctx.Mutable == nil {
		return false
	}
	limit := ctx.Mutable.PrescanRoundLimit()
	return limit > 0 && ctx.Mutable.PrescanRoundCount() >= limit
}

// buildInitialMessages is the sole entry point for producing the
// ReAct loop's initial message stream. It owns the ORCHESTRATION
// (assemble → render → append) and nothing else; every business
// rule lives in one of the three primitives below:
//
//  1. PromptAssembler.AssembleContext — builds the structured
//     PromptContext from ctx + skill. Pinned by the prompt-shape
//     regression tests in internal/context/builder_test.go.
//  2. PromptAssembler.RenderMessages — flattens the PromptContext
//     into llm.Messages. Owns the context.Message → llm.Message
//     conversion so a future schema change to llm.Message does not
//     ripple into every call site.
//  3. AppendDynamicInstruction — appends the evaluator's per-
//     dispatch supplement when non-empty. Kept as a free function
//     so the Skill/Evaluator boundary contract documented in
//     docs/architecture.md §3.3 stays grep-visible at the site
//     where the supplement actually enters the stream.
//
// The function intentionally has no control flow beyond the three
// sequential calls: every branch, length check, and type conversion
// belongs to one of the primitives. A future extension that wants to
// (for example) inject a retry hint between render and append simply
// adds a fourth step here — tests that target the individual
// primitives keep passing because this method never implements
// rules on its own.
func (b *BaseAgent) buildInitialMessages(ctx *types.AgentContext, sk *skill.Config) []llm.Message {
	// 1. Assemble the structured PromptContext from ctx + skill.
	stopPreflight := b.startPreflightWatchdog(ctx, "prompt_assemble")
	pc := b.deps.PromptAssembler.AssembleContext(ctx, sk)
	stopPreflight()
	// 2. Render the PromptContext as the llm.Message slice.
	stopPreflight = b.startPreflightWatchdog(ctx, "prompt_render")
	messages := b.deps.PromptAssembler.RenderMessages(pc)
	stopPreflight()
	// 3. Append evaluator instruction (evaluator's dynamic supplement,
	//    when non-empty; never restates the skill static contract).
	stopPreflight = b.startPreflightWatchdog(ctx, "prompt_dynamic_instruction")
	messages = AppendDynamicInstruction(messages, b.eval, ctx, sk)
	stopPreflight()
	return messages
}

const (
	agentPreflightSlowAfter = 5 * time.Second
	agentPreflightSlowEvery = 10 * time.Second
)

func (b *BaseAgent) startPreflightWatchdog(ctx *types.AgentContext, phase string) func() {
	agentName := b.name
	stage := ""
	if ctx != nil {
		stage = string(ctx.Stage)
	}
	start := time.Now()
	logging.Debug("[diag %s] phase=%s stage=%s start", agentName, phase, stage)

	done := make(chan struct{})
	var once sync.Once
	go func() {
		timer := time.NewTimer(agentPreflightSlowAfter)
		defer timer.Stop()
		for {
			select {
			case <-done:
				return
			case <-timer.C:
				logging.Warning("[diag %s] phase=%s stage=%s still running elapsed=%s",
					agentName, phase, stage, time.Since(start).Round(time.Second))
				timer.Reset(agentPreflightSlowEvery)
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
			logging.Debug("[diag %s] phase=%s stage=%s done elapsed=%s",
				agentName, phase, stage, time.Since(start).Round(time.Millisecond))
		})
	}
}

func (b *BaseAgent) executeTool(ctx *types.AgentContext, tc llm.ToolCall) (*types.ToolResult, *types.MCPResponse) {
	// Fix G (2026-05-07 customer report): repair common LLM-induced
	// JSON corruption in tool call parameters before validation /
	// execution. LLMs occasionally emit a trailing extra `}` (the
	// model closing its outer thought at the same indent as the
	// params object) or a trailing `,` before `}`/`]` (streaming
	// tokeniser artefact). Both fail strict json.Unmarshal and
	// previously caused the tool to error out with "invalid
	// character ... after top-level value", forcing a retry
	// round-trip. The repair is bounded — only trailing garbage /
	// pre-terminator commas are stripped; the function never adds
	// JSON syntax — and the repaired payload is re-validated
	// before being substituted.
	if repaired, ok := repairToolParamsJSON(tc.Params); ok {
		logging.Warning("[agent] tool %q params auto-repaired (LLM-corrupted JSON: structural repair)", tc.Name)
		tc.Params = repaired
	}
	if !toolCallParamsValidForHistory(tc.Params) {
		return malformedToolParamsResult(tc), nil
	}
	if normalized, ok := b.normalizeAnalyzerPrescanGrepCompat(ctx, tc); ok {
		tc = normalized
	}

	// Stage-specific pre-execution parameter validation. The
	// analyzer's evidence-lite boundary forbids line-level grep
	// results — pre-scan must use `files_only=true` because
	// line-level results blow past the analyze stage's context
	// budget and the "does this exist / in which files" question
	// does not need line numbers. This check promotes the rule
	// from a prompt-only hint to a runtime hard constraint: a
	// violating grep call returns a failed ToolResult with a
	// clear Summary so the LLM sees the error and can retry
	// within the same dispatch, AND `analysis_quality_probe`'s
	// hit-ratio measurement stays honest (line-level grep
	// summaries would flood the seen-blob with irrelevant code
	// snippets and mask true hit ratios).
	if violation := validateAnalyzerToolBoundary(ctx, tc); violation != nil {
		return violation, nil
	}
	if violation := validateAnalyzerPrescanToolCall(ctx, tc); violation != nil {
		return violation, nil
	}
	if violation := validateObservationOnlyRuntimeToolCall(ctx, tc); violation != nil {
		return violation, nil
	}

	// Explorer sourcemix budget gate. When an ExploreBudget is
	// installed on MutableState (runTaskGraph does this before
	// every explore window), refuse tool calls that would exceed
	// the per-tool or overall cap. The LLM sees a failed ToolResult
	// describing the exhausted budget and is expected to switch to
	// a different tool or stop. Non-explore stages (analyze,
	// extract, finalize) are untouched.
	if ctx != nil && ctx.Stage == types.StageExplore && ctx.Mutable != nil {
		canonical := types.CanonicalToolName(tc.Name)
		if rem := ctx.Mutable.BudgetRemaining(canonical); rem <= 0 {
			msg := fmt.Sprintf(
				"explore budget exhausted for tool %q: per-tool or overall cap reached. "+
					"Use a different tool or stop the investigation.", tc.Name)
			logging.Warning("[sourcemix] %s", msg)
			return &types.ToolResult{
				ToolName:  tc.Name,
				Summary:   msg,
				Success:   false,
				Timestamp: time.Now(),
			}, nil
		}
		ctx.Mutable.RecordToolCall(canonical)
	}

	// Try local tool first
	if b.deps.Tools != nil {
		if _, err := b.deps.Tools.Get(tc.Name); err == nil {
			// The busCtx handed to a tool is intentionally narrow:
			// RepoRoot / Branch / Commit / WorkDir / MainRepoRoot
			// (read-only env info) plus Mutable (the shared,
			// tool-writable region) plus AnalysisIR (read-only IR).
			// Memory / EnvFacts / EnvRecommendSettings / Language /
			// Preferences are read-only by construction (interface
			// or value type) and tools depend on them — recall_memory
			// reads Memory; run_tests env_recommend integration reads
			// EnvFacts + EnvRecommendSettings + Language; reports
			// honour Preferences. All other BusContext fields stay
			// zero-valued, so tools physically cannot mutate
			// stage-output state.
			busCtx := b.buildToolBusContext(ctx)
			result, execErr := b.deps.Tools.Execute(busCtx, tc.Name, tc.Params)
			if execErr != nil {
				logging.Error("tool %s execution error: %v", tc.Name, execErr)
			}
			// Session 11 C0' post-process: when a line-level grep
			// fires in analyze stage with the classification trigger
			// flag on, account the call against the per-dispatch
			// budget and capture the match lines into the sidecar
			// observation channel for the reconciler. Analyzer is
			// the only stage that takes this path; other stages
			// short-circuit on the trigger flag being off.
			analyzerPostProcessToolResult(ctx, tc, &result)
			// Refund budget for read_file calls that failed because
			// the LLM picked a path that doesn't exist (or named a
			// directory). The LLM is still triangulating the repo
			// layout and self-corrects on the next iteration; charging
			// these exploratory misses against the cap drains the
			// budget before any real file reads land. Only refund for
			// explore-stage dispatches where the gate recorded the
			// call to begin with.
			if ctx != nil && ctx.Stage == types.StageExplore && ctx.Mutable != nil &&
				isReadFilePathMiss(tc.Name, &result) {
				ctx.Mutable.RefundToolCall(types.CanonicalToolName(tc.Name))
				logging.Debug("[sourcemix] refunded read_file budget for path miss: %s", result.Summary)
			}
			return &result, nil
		}
	}

	// Try MCP servers
	if b.deps.MCPServers != nil {
		for _, serverName := range b.deps.MCPServers.List() {
			server, _ := b.deps.MCPServers.Get(serverName)
			for _, t := range server.ListTools() {
				if t.Name == tc.Name {
					resp, err := server.CallTool(tc.Name, tc.Params)
					if err != nil {
						logging.Error("mcp %s.%s error: %v", serverName, tc.Name, err)
					}
					return nil, &resp
				}
			}
		}
	}

	logging.Warning("tool not found: %s", tc.Name)
	return nil, nil
}

// buildToolBusContext narrows the agent's AgentContext into a
// BusContext for tool dispatch. Delegates to types.ToolBusContext
// (the single canonical narrowing helper) so every typed signal —
// MultiGraph / TypedDenials / PendingSubRepos / Memory / Ctx etc. —
// propagates automatically. Adding a new typed signal to AgentContext
// requires updating types.ToolBusContext + the
// projectionTypedSignalFields list, after which every narrowing site
// inherits the change.
//
// activeName falls back to the agent's own name when the AgentContext
// has no AgentName set — preserves the pre-projection behavior where
// the dispatch site stamped the active agent identity.
func (b *BaseAgent) buildToolBusContext(ctx *types.AgentContext) *types.BusContext {
	active := types.AgentName("")
	if ctx != nil {
		active = ctx.AgentName
	}
	if active == "" {
		active = b.name
	}
	return types.ToolBusContext(ctx, active)
}

// canParallelizeToolBatch returns true when all tool calls in the
// batch are safe to execute concurrently. A batch is parallelizable
// when it contains ONLY read-safe tools (grep, read_file, repo_map,
// list_files, exec_command, git_diff, git_log). Batches containing
// any emit_* or propose_* tool fall back to sequential execution
// because those tools write to MutableState and may depend on
// DispatchToolResults from earlier calls in the same batch (e.g.
// emit_evidence's grounder needs read_file gutter data).
//
// This is the safety gate for S1 (parallel tool execution): only
// truly independent reads run concurrently; anything that mutates
// shared state stays sequential.
func canParallelizeToolBatch(calls []llm.ToolCall) bool {
	for _, tc := range calls {
		if strings.HasPrefix(tc.Name, "emit_") ||
			strings.HasPrefix(tc.Name, "propose_") ||
			tc.Name == "todo_write" {
			return false
		}
	}
	return true
}

// validateAnalyzerPrescanToolCall is the runtime hard-enforcement
// companion to the analyzer skill's evidence-lite prompt rules.
// When the current dispatch is the analyze stage, it inspects the
// tool call before dispatching to the tool registry and rejects
// violations with a synthesized failed ToolResult.
//
// Current rules:
//
//   - `grep` MUST be called with `files_only=true`. Line-level
//     results blow past the analyze stage's context budget and
//     are not useful for the "does X exist / in which files"
//     question the analyzer is supposed to answer. Violations
//     return a ToolResult with Success=false and a descriptive
//     Summary so the LLM sees the error in the next iteration's
//     message stream and can retry correctly.
//
// Returns nil when no violation is detected — the caller then
// proceeds to the normal tool execution path. Returns a
// pre-synthesized failed ToolResult when a violation is found.
//
// The pre-scan round budget (MaxPrescanRounds) is enforced
// separately by `analyzerEvaluator.Observe` at the
// LoopController layer — that gate trips AFTER a successful
// pre-scan round completes, while this function trips BEFORE any
// tool execution for parameter-shape violations. The two gates
// are orthogonal and complementary: this function catches
// "grep without files_only", Observe catches "too many rounds".
func validateAnalyzerPrescanToolCall(ctx *types.AgentContext, tc llm.ToolCall) *types.ToolResult {
	if ctx == nil || ctx.Stage != types.StageAnalyze {
		return nil
	}
	if !isPrescanTool(tc.Name) {
		return nil
	}
	if ctx.EmitStageRetryAttempt > 0 {
		return rejectAnalyzerPrescanTool(ctx, tc, analyzerPrescanTerminalEmitModeCode,
			"analyze retry is already in terminal emit mode; do not call repo_map / grep / list_files again. Call emit_analysis now with the best classification you have.")
	}
	if ctx.Mutable != nil {
		limit := ctx.Mutable.PrescanRoundLimit()
		if limit > 0 && ctx.Mutable.PrescanRoundCount() >= limit {
			return rejectAnalyzerPrescanTool(ctx, tc, analyzerPrescanBudgetReachedCode,
				fmt.Sprintf("pre-scan budget already reached (%d/%d rounds used). Do not call repo_map / grep / list_files again; call emit_analysis now with the fields you have.",
					ctx.Mutable.PrescanRoundCount(), limit))
		}
	}
	if tc.Name != "grep" {
		return nil
	}
	// Parse minimal params to check files_only and max_count.
	// Defensive: an unparseable params blob goes through unchanged so
	// the real grep tool produces its canonical error message.
	var p struct {
		FilesOnly bool `json:"files_only"`
		MaxCount  int  `json:"max_count"`
	}
	if err := json.Unmarshal(tc.Params, &p); err != nil {
		return nil
	}
	if p.FilesOnly {
		// Round 1 happy path — evidence-lite boundary honoured.
		return nil
	}
	return rejectAnalyzerPrescanTool(ctx, tc, analyzerGrepFilesOnlyRequiredCode,
		"grep in analyze stage must use files_only=true; line-level matches are exploration evidence, not classification input. Retry with files_only=true or call emit_analysis with the fields you have.")
}

const (
	analyzerToolNotAllowedCode          = "analyzer_tool_not_allowed"
	analyzerPrescanTerminalEmitModeCode = "analyzer_terminal_emit_mode"
	analyzerPrescanBudgetReachedCode    = "analyzer_prescan_budget_reached"
	analyzerGrepFilesOnlyRequiredCode   = "analyzer_grep_files_only_required"
)

func validateAnalyzerToolBoundary(ctx *types.AgentContext, tc llm.ToolCall) *types.ToolResult {
	if ctx == nil || ctx.Stage != types.StageAnalyze {
		return nil
	}
	if isAnalyzerStageAllowedTool(tc.Name) {
		return nil
	}
	return rejectAnalyzerTool(ctx, tc, "analyzer_tool_rejected", analyzerToolNotAllowedCode,
		fmt.Sprintf("tool %q is not available in analyze stage. Analyze is classification-only: use repo_map / grep(files_only=true) / list_files for light location checks, or call emit_analysis now. Deep content-reading tools such as read_file belong to explore.", tc.Name))
}

func isAnalyzerStageAllowedTool(name string) bool {
	return name == "emit_analysis" || isPrescanTool(name)
}

// rejectAnalyzerPrescanTool synthesises the failed ToolResult
// returned from validateAnalyzerPrescanToolCall when the analyzer
// issues a prescan tool call that the runtime gate will not admit.
// Logs the rejection at WARN so operators can spot unexpected
// trigger misfires in production. R8 (post_shape_residual_audit.md,
// 2026-05-04): also records an AnalyzerDecisionSignal on Mutable
// so the end-of-Run summary surfaces the rejection, not just the
// log line.
func rejectAnalyzerPrescanTool(ctx *types.AgentContext, tc llm.ToolCall, code, reason string) *types.ToolResult {
	return rejectAnalyzerTool(ctx, tc, "prescan_rejected", code, reason)
}

func rejectAnalyzerTool(ctx *types.AgentContext, tc llm.ToolCall, decisionKind, code, reason string) *types.ToolResult {
	logging.Warning("[analyzer] tool %q rejected: %s", tc.Name, reason)
	if ctx != nil && ctx.Mutable != nil {
		ctx.Mutable.AppendAnalyzerDecision(types.AnalyzerDecisionSignal{
			Kind:   decisionKind,
			Stage:  string(types.StageAnalyze),
			Reason: reason,
			Detail: tc.Name,
		})
	}
	return &types.ToolResult{
		ToolName:  tc.Name,
		Success:   false,
		Summary:   reason,
		Repair:    &types.ToolRepair{Code: code, Hint: reason},
		Timestamp: time.Now(),
	}
}

// clampGrepMaxCount rewrites the max_count field of a grep param
// blob to the given cap. Preserves unrelated fields verbatim.
func clampGrepMaxCount(params json.RawMessage, cap int) (json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return params, err
	}
	capJSON, err := json.Marshal(cap)
	if err != nil {
		return params, err
	}
	m["max_count"] = capJSON
	out, err := json.Marshal(m)
	if err != nil {
		return params, err
	}
	return out, nil
}

// analyzerPostProcessToolResult is the Session 11 C0' accounting +
// sidecar capture hook. Runs after every tool execution in
// executeTool; short-circuits on anything that is not a line-level
// grep in analyze stage with the classification trigger on.
//
// Two effects:
//
//  1. Bump the per-dispatch call counter and byte counter on
//     MutableState so validateAnalyzerPrescanToolCall's next
//     admission check sees the updated budget state.
//
//  2. Parse the match lines out of the grep Summary and append
//     each as a ClassificationObs on MutableState's sidecar
//     channel. Pre-PR2 the buildAnalysisIR pipeline ran a
//     reconcileFromObservations step against this sidecar to nudge
//     AnalyzerHints.Shape toward "value" on a quoted-literal hit;
//     that step retired with AnswerShape. The sidecar still
//     accumulates so a future axis-specific reconciler
//     (answer_subject.kind / question_kind / entity_axes) can pick
//     it up — wiring a new consumer is the right place to add the
//     refinement, not putting shape rules back. The sidecar stays
//     OFF the TurnAArtifacts path, so no downstream stage ever sees
//     these lines as evidence.
//
// The parser is a best-effort line extractor — unparseable outputs
// fall through silently (reconciler still gets partial data from
// other rounds). We intentionally do not expand or enrich the
// observation; the raw grep line + the file/line/pattern tuple is
// enough signal for the classifier.
func analyzerPostProcessToolResult(ctx *types.AgentContext, tc llm.ToolCall, result *types.ToolResult) {
	if ctx == nil || result == nil {
		return
	}
	if ctx.Stage != types.StageAnalyze || tc.Name != "grep" {
		return
	}
	if ctx.Mutable == nil || !ctx.Mutable.ClassificationGrepTriggered() {
		return
	}
	if !result.Success {
		return
	}
	// Must be a line-level call; files_only=true means Round 1
	// happy path and never produces observations.
	var p struct {
		Pattern   string `json:"pattern"`
		Path      string `json:"path"`
		FilesOnly bool   `json:"files_only"`
	}
	if err := json.Unmarshal(tc.Params, &p); err != nil {
		return
	}
	if p.FilesOnly {
		return
	}
	// Account this call.
	ctx.Mutable.BumpClassificationGrepCall(len(result.Summary))
	// Parse grep output lines — the standard format produced by
	// the repo's grep tool is `<path>:<line>:<text>`. Lines that
	// do not match this format are skipped (e.g. the `[grep: N
	// matching files]` banner added by session 8's kvBanner).
	scanGrepLinesIntoClassificationObs(ctx, p.Pattern, result.Summary)
}

// scanGrepLinesIntoClassificationObs walks the grep Summary and
// converts each `path:line:text` match into a ClassificationObs
// appended to MutableState.classificationObservations. Tolerates
// banner lines and malformed rows by skipping them silently.
//
// Session-22 follow-up root-cause fix: lines whose `path` is a
// test file are skipped. A test assertion string like
// `"flag=on must write TurnAArtifacts"` matches extractQuotedLiterals
// just as well as a production-source literal would, so leaking
// test observations into the classification stream is structurally
// noisy. The C0' downgrade rule that originally consumed these
// observations was retired together with AnswerShape; the
// observations themselves still flow into MutableState for any
// future axis-specific reconciler (e.g. AnswerSubject.Kind), and
// the test-file exclusion remains because production-quality
// observations should not carry test-fixture noise regardless of
// downstream consumer.
func scanGrepLinesIntoClassificationObs(ctx *types.AgentContext, pattern, summary string) {
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip session-8 banner (`[grep: ...`).
		if strings.HasPrefix(line, "[") {
			continue
		}
		// Expect `path:lineNumber:text`.
		colonA := strings.Index(line, ":")
		if colonA <= 0 {
			continue
		}
		rest := line[colonA+1:]
		colonB := strings.Index(rest, ":")
		if colonB <= 0 {
			continue
		}
		path := line[:colonA]
		if isTestFilePath(path) {
			continue
		}
		lineNumStr := rest[:colonB]
		text := rest[colonB+1:]
		lineNum, err := strconv.Atoi(strings.TrimSpace(lineNumStr))
		if err != nil || lineNum <= 0 {
			continue
		}
		ctx.Mutable.AppendClassificationObs(types.ClassificationObs{
			Pattern: pattern,
			Path:    path,
			Line:    lineNum,
			Text:    strings.TrimSpace(text),
			TS:      time.Now(),
		})
	}
}

// isTestFilePath reports whether a repo-relative path looks like a
// test / spec file under the language conventions codrax supports.
// Used by scanGrepLinesIntoClassificationObs to keep C0' observations
// confined to production code — test assertion strings, fixture data
// and example snippets carry quoted literals that the reconciler
// would misread as answer-shape signal.
//
// Path form is always repo-relative with forward-slash separators
// (codrax's grep tool and ground.CanonicalRepoRelative normalise both
// platforms to /), so suffix matching on the full path is enough —
// no need to extract the basename first.
//
// Covered conventions, by ecosystem:
//
//   - Go                      _test.go
//   - Python (pytest/unittest) _test.py / test_<name>.py
//   - Ruby (RSpec)             _spec.rb / _test.rb
//   - JavaScript / TypeScript  .test.js / .test.ts / .spec.js / .spec.ts
//     plus .test.jsx / .spec.jsx / .test.tsx / .spec.tsx
//   - Java / Kotlin            Test.java / Tests.java / Test.kt
//     plus common prefixes Test*.java /
//     IT*.java (integration tests)
//   - C / C++                  _test.c / _test.cc / _test.cpp / _test.cxx
//     plus google-test convention *_unittest.cc
//
// The matcher is conservative: files that look like test helpers but
// do not match a recognised suffix fall through and are treated as
// production. Preferring false negatives here is deliberate — C0' is
// already conservative about firing, and admitting a non-test helper
// is harmless while mis-classifying a real declarative file as a
// test would silently re-introduce the bug this helper solves.
func isTestFilePath(p string) bool {
	if p == "" {
		return false
	}
	if types.LooksLikeTestFilePath(p) {
		return true
	}
	// Suffix checks on the full path. Cheaper than filepath.Base +
	// repeated HasSuffix, and correct because every listed suffix
	// includes either an underscore / dot boundary or a filename
	// token so a production file with one of these endings would
	// have to be named exactly that (e.g. a file literally called
	// `_test.go` at the repo root — a vanishingly rare case we
	// accept as a false positive).
	suffixes := []string{
		// Go.
		"_test.go",
		// Python.
		"_test.py",
		// Ruby.
		"_spec.rb", "_test.rb",
		// JavaScript / TypeScript.
		".test.js", ".test.jsx", ".test.ts", ".test.tsx",
		".spec.js", ".spec.jsx", ".spec.ts", ".spec.tsx",
		// Java / Kotlin common conventions.
		"Test.java", "Tests.java", "Test.kt", "Tests.kt",
		// C / C++ / google-test.
		"_test.c", "_test.cc", "_test.cpp", "_test.cxx",
		"_unittest.cc", "_unittest.cpp",
	}
	for _, s := range suffixes {
		if strings.HasSuffix(p, s) {
			return true
		}
	}
	// Python / Java prefix conventions.
	// - `test_<x>.py` at the basename (pytest discovery default).
	// - `Test<X>.java` / `IT<X>.java` at the basename (Maven
	//   Surefire / Failsafe discovery defaults).
	slash := strings.LastIndex(p, "/")
	base := p
	if slash >= 0 {
		base = p[slash+1:]
	}
	switch {
	case strings.HasSuffix(base, ".py") && strings.HasPrefix(base, "test_"):
		return true
	case strings.HasSuffix(base, ".java") && (strings.HasPrefix(base, "Test") || strings.HasPrefix(base, "IT")):
		// Guard against a real Test<capital> class that does not
		// end in Test.java: base-prefix match covers this case too.
		return true
	}
	return false
}

// toolDetail extracts a short human-readable detail from tool parameters
// (e.g. file path or command) for display in the status line.
func toolDetail(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(params, &m) != nil {
		return ""
	}
	// Try common keys in priority order.
	for _, key := range []string{"path", "command", "query", "pattern"} {
		raw, ok := m[key]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) != nil || s == "" {
			continue
		}
		// Truncate long values (commands can be very long).
		if len(s) > 60 {
			s = s[:57] + "..."
		}
		return s
	}
	return ""
}

func toolDetailForCall(call llm.ToolCall) string {
	if detail := structuredToolDetail(call.Name, call.Params); detail != "" {
		return detail
	}
	return toolDetail(call.Params)
}

func structuredToolDetail(toolName string, params json.RawMessage) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" || len(params) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(params, &m) != nil {
		return ""
	}
	switch toolName {
	case "emit_evidence", "emit_answer_symbol", "emit_hypothesis_verdict":
		return toolItemsDetail(m)
	case "emit_answer_document", "emit_answer_document_patch":
		return toolAnswerDocumentDetail(m)
	case "emit_analysis":
		return toolAnalysisDetail(m)
	default:
		return ""
	}
}

func toolItemsDetail(m map[string]json.RawMessage) string {
	n := jsonArrayLen(m["items"])
	if n <= 0 {
		return ""
	}
	parts := []string{fmt.Sprintf("items=%d", n)}
	if first := firstObjectStringFromArray(m["items"], "name", "subject", "anchor_symbol", "hypothesis_id"); first != "" {
		parts = append(parts, truncateToolDetailValue(first, 40))
	}
	return strings.Join(parts, " ")
}

func toolAnswerDocumentDetail(m map[string]json.RawMessage) string {
	var parts []string
	for _, field := range []string{"blocks", "replace_blocks", "add_blocks", "unchanged_block_ids", "citations"} {
		if n := jsonArrayLen(m[field]); n > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", field, n))
		}
	}
	return strings.Join(parts, " ")
}

func toolAnalysisDetail(m map[string]json.RawMessage) string {
	var parts []string
	for _, field := range []string{"intent", "question_kind", "scenario", "complexity"} {
		if v := jsonStringField(m, field); v != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", field, truncateToolDetailValue(v, 24)))
		}
	}
	for _, field := range []string{"entities", "keywords", "sub_topics", "required_files"} {
		if n := jsonArrayLen(m[field]); n > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", field, n))
		}
	}
	if len(parts) > 4 {
		parts = parts[:4]
	}
	return strings.Join(parts, " ")
}

func jsonArrayLen(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) != nil {
		return 0
	}
	return len(arr)
}

func jsonStringField(m map[string]json.RawMessage, key string) string {
	raw, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func firstObjectStringFromArray(raw json.RawMessage, keys ...string) string {
	if len(raw) == 0 {
		return ""
	}
	var arr []map[string]json.RawMessage
	if json.Unmarshal(raw, &arr) != nil || len(arr) == 0 {
		return ""
	}
	for _, key := range keys {
		if s := jsonStringField(arr[0], key); s != "" {
			return s
		}
	}
	return ""
}

func truncateToolDetailValue(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func toolResultSummary(result *types.ToolResult) string {
	if result == nil {
		return ""
	}
	if result.Success {
		return result.Summary
	}
	switch strings.TrimSpace(result.ToolName) {
	case "emit_answer_document", "emit_answer_document_patch":
		return result.Summary
	}
	return ""
}

func toolCallNames(calls []llm.ToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			name = "tool"
		}
		names = append(names, name)
	}
	return names
}
