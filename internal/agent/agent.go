package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
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

	// ExploreHeuristics carries the tunable thresholds for the
	// explorer evaluator's mid-loop and soft-stop detection branches.
	// Optional: zero fields are filled from DefaultExploreHeuristics()
	// in cmd/root.go before agent construction.
	ExploreHeuristics types.ExploreHeuristics

	// AgentSettings carries all per-agent tunable limits (iteration
	// caps, tool-history budget, correction retries). Resolved from
	// YAML in cmd/root.go before agent construction.
	AgentSettings types.AgentSettings
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
	return &BaseAgent{
		name: name,
		deps: &Dependencies{
			LLM:             deps.LLM,
			Tools:           deps.Tools,
			MCPServers:      deps.MCPServers,
			SubAgents:       deps.SubAgents,
			MaxIterations:   maxIter,
			Emit:            emit,
			PromptAssembler: assembler,
			LoopPolicy:      loopPolicy,
		},
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
//  1. Content contains a JSON code block (```json ... ```) or a
//     top-level '{' followed by a closing '}'.
//  2. Inside that JSON block, a concrete emit_* tool name appears
//     as a quoted string value (e.g. "emit_answer_symbol",
//     "emit_answer_document", "emit_evidence").
//
// This avoids false positives from LLM prose that merely discusses
// tool names or JSON structures — the combination of a JSON block
// with an actual tool name as a string value is the signature of a
// serialized-but-not-executed tool call.
func looksLikeEmbeddedToolCall(content string) bool {
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

func truncForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("...[truncated %d bytes]", len(s)-max)
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
// Returns true when at least one message was stubbed, so the caller
// can log the event.
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
	// Build tool schemas for LLM
	toolSchemas := b.buildToolSchemas(sk)

	// Initialize message history
	messages := b.buildInitialMessages(ctx, sk)

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

	// ReAct loop
	maxIter := b.deps.MaxIterations
	if ctx != nil && ctx.MaxIterOverride > 0 {
		maxIter = ctx.MaxIterOverride
	}
	for i := 0; i < maxIter; i++ {
		if forceStop {
			break
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
			logging.Debug("[diag %s] iter=%d TOOL HISTORY PRUNED (budget=%d bytes)",
				b.name, i, toolHistBudget)
		}

		// Reason — call LLM
		b.deps.Emit(render.Event{
			Kind:      render.EventAgentThinking,
			Timestamp: time.Now(),
			Agent:     b.name,
			Stage:     ctx.Stage,
			Iteration: i,
		})
		resp, err := b.deps.LLM.Chat(messages, toolSchemas)
		if err != nil {
			return &StageOutput{
				Error:        fmt.Sprintf("LLM call failed: %v", err),
				MissingPiece: ctx.MissingPiece,
			}, err
		}

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
		for j, tc := range resp.ToolCalls {
			logging.Debug("[diag %s] iter=%d call[%d] tool=%s params=%s",
				b.name, i, j, tc.Name, string(tc.Params))
		}

		// Hard stop from the evaluator (e.g., finalizer always stops at iter=0).
		if b.eval.ShouldStop(resp, i) {
			messages = append(messages, llm.Message{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
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
			// voluntary stop to avoid an infinite thinking loop.
			if resp.Content == "" {
				logging.Debug("[diag %s] STOP at iter=%d (empty response)", b.name, i)
				break
			}
			// Detect LLM writing tool-call JSON in content instead of
			// using the function-calling mechanism. This happens when
			// the Think Aloud directive leads the LLM to embed tool
			// calls as markdown JSON blocks. Inject a correction hint
			// so the LLM retries with real tool_use blocks.
			if looksLikeEmbeddedToolCall(resp.Content) {
				logging.Debug("[diag %s] iter=%d detected embedded tool-call JSON in content — injecting correction", b.name, i)
				messages = append(messages, llm.Message{
					Role:    "assistant",
					Content: resp.Content,
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
				logging.Debug("[diag %s] iter=%d SOFT-STOP signal hint=%t progress=%t stop=%t key=%q → %s (%s)",
					b.name, i, sig.HintRequested, sig.Progress, sig.StopRequested,
					sig.HintKey, result.Outcome, result.Reason)
				if result.Outcome == OutcomeInjectHint {
					messages = append(messages, llm.Message{
						Role:      "assistant",
						Content:   resp.Content,
						ToolCalls: resp.ToolCalls,
					})
					messages = append(messages, llm.Message{
						Role:    "user",
						Content: result.Hint,
					})
					continue
				}
				// OutcomeStop or OutcomeContinue at PhaseSoftStop
				// both terminate the ReAct loop — the policy's
				// soft-stop semantics treat "no hint" as accept.
			}
			messages = append(messages, llm.Message{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})
			logging.Debug("[diag %s] STOP at iter=%d (soft)", b.name, i)
			break
		}

		// Record assistant message with tool calls
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Act — execute tool calls
		var lastToolResultPtr *types.ToolResult
		for _, tc := range resp.ToolCalls {
			toolStart := time.Now()
			b.deps.Emit(render.Event{
				Kind:       render.EventToolCallStart,
				Timestamp:  toolStart,
				Agent:      b.name,
				Stage:      ctx.Stage,
				ToolName:   tc.Name,
				ToolCallID: tc.ID,
				ToolDetail: toolDetail(tc.Params),
			})

			result, mcpResp := b.executeTool(ctx, tc)

			toolOK := false
			if result != nil {
				toolOK = result.Success
				allToolResults = append(allToolResults, *result)
				lastToolResultPtr = &allToolResults[len(allToolResults)-1]
				// Mirror into the Mutable-side running buffer so tools
				// that run later in THIS dispatch (emit_evidence's
				// grounder) can see the read_file history produced
				// earlier in the same ReAct loop.
				if ctx != nil && ctx.Mutable != nil {
					ctx.Mutable.AppendDispatchToolResult(*result)
				}
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    result.Summary,
					ToolCallID: tc.ID,
				})
				// DIAGNOSTIC — dump tool result (debug only).
				logging.Debug("[diag %s] iter=%d TOOLRESULT %s ok=%v len=%d:\n%s\n---",
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
				Kind:       render.EventToolCallEnd,
				Timestamp:  time.Now(),
				Agent:      b.name,
				Stage:      ctx.Stage,
				ToolName:   tc.Name,
				ToolCallID: tc.ID,
				ToolDetail: toolDetail(tc.Params),
				ToolOK:     toolOK,
				ToolTime:   time.Since(toolStart),
			})
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
			logging.Debug("[diag %s] iter=%d MIDLOOP signal hint=%t progress=%t stop=%t key=%q → %s (%s)",
				b.name, i, sig.HintRequested, sig.Progress, sig.StopRequested,
				sig.HintKey, result.Outcome, result.Reason)
			switch result.Outcome {
			case OutcomeInjectHint:
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: result.Hint,
				})
				logging.Debug("[diag %s] iter=%d MIDLOOP inject len=%d:\n%s\n---",
					b.name, i, len(result.Hint), truncForLog(result.Hint, 1000))
			case OutcomeStop:
				// Policy decided to terminate — e.g. idle-streak
				// force-stop or evaluator StopRequested. Record the
				// reason for the trace and break out of the ReAct
				// loop on the next iteration's top check.
				logging.Debug("[diag %s] iter=%d MIDLOOP force-stop: %s",
					b.name, i, result.Reason)
				forceStop = true
			}
		}
	}

	// Parse final output
	output, err := b.eval.ParseOutput(ctx, messages, allToolResults, allMCPResponses)
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
	output.MissingPiece = b.eval.DetermineMissingPiece(ctx, output)

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
func (b *BaseAgent) buildToolSchemas(sk *skill.Config) []llm.ToolSchema {
	var schemas []llm.ToolSchema

	// Add suggested tools from skill. Tools with high confidence
	// (evidence-bearing: grep, read_file, exec_command, …) get a
	// "[high-confidence]" tag appended to their description so the
	// LLM can see at schema-selection time which tools produce
	// citable evidence vs. navigation hints or side-effects.
	if b.deps.Tools != nil {
		for _, toolName := range sk.ToolSuggestions {
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
			schemas = append(schemas, llm.ToolSchema{
				Name:        t.Name(),
				Description: desc,
				Parameters:  t.Parameters(),
			})
		}
	}

	// Add MCP tools
	if b.deps.MCPServers != nil {
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
	pc := b.deps.PromptAssembler.AssembleContext(ctx, sk)
	// 2. Render the PromptContext as the llm.Message slice.
	messages := b.deps.PromptAssembler.RenderMessages(pc)
	// 3. Append evaluator instruction (evaluator's dynamic supplement,
	//    when non-empty; never restates the skill static contract).
	messages = AppendDynamicInstruction(messages, b.eval, ctx, sk)
	return messages
}

func (b *BaseAgent) executeTool(ctx *types.AgentContext, tc llm.ToolCall) (*types.ToolResult, *types.MCPResponse) {
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
	if violation := validateAnalyzerPrescanToolCall(ctx, tc); violation != nil {
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
			// only RepoRoot/Branch/Commit (read-only env info) plus
			// Mutable (the shared, tool-writable region) are populated.
			// All other BusContext fields are zero-valued, so tools
			// physically cannot mutate stage-output state.
			busCtx := &types.BusContext{
				RepoRoot:   ctx.RepoRoot,
				Branch:     ctx.Branch,
				Commit:     ctx.Commit,
				WorkDir:    ctx.WorkDir,
				Mutable:    ctx.Mutable,
				AnalysisIR: ctx.AnalysisIR,
			}
			result, execErr := b.deps.Tools.Execute(busCtx, tc.Name, tc.Params)
			if execErr != nil {
				logging.Error("tool %s execution error: %v", tc.Name, execErr)
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
	if tc.Name != "grep" {
		return nil
	}
	// Parse minimal params to check files_only. Defensive: an
	// unparseable params blob goes through unchanged so the real
	// grep tool produces its canonical error message.
	var p struct {
		FilesOnly bool `json:"files_only"`
	}
	if err := json.Unmarshal(tc.Params, &p); err != nil {
		return nil
	}
	if p.FilesOnly {
		return nil
	}
	reason := "grep in analyze stage must be called with files_only=true " +
		"(evidence-lite boundary: line-level results overflow the analyze " +
		"stage's context budget). Retry with files_only=true."
	logging.Warning("[analyzer] grep without files_only rejected: %s", reason)
	return &types.ToolResult{
		ToolName:  tc.Name,
		Success:   false,
		Summary:   reason,
		Timestamp: time.Now(),
	}
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
