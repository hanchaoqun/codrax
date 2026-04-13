package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentctx "github.com/hanchaoqun/codrax/internal/context"
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

	// Answer chains: deterministic resolution chains identified as
	// directly answering the user's question. These get priority
	// presentation in the finalizer prompt.
	AnswerChains []string `json:"answer_chains,omitempty"`

	// Answer symbols: L0-2 structured terminals extracted from
	// AnswerChains. For registration / call_chain / return_value
	// kinds, these are the canonical names the finalizer must list
	// verbatim (no add, no drop). For other kinds, empty; the
	// finalizer falls back to the legacy prose path.
	AnswerSymbols []types.AnswerSymbol `json:"answer_symbols,omitempty"`

	// Tool results collected during execution
	ToolResults []types.ToolResult `json:"tool_results,omitempty"`

	// MCP responses collected during execution
	MCPResponses []types.MCPResponse `json:"mcp_responses,omitempty"`

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

	// AnalysisIR is the analyzer's canonical structured output.
	// The orchestrator stores it on BusContext and downstream stages
	// (especially explore) should read analyzer semantics from here
	// instead of from TaskItem text fields.
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
	// BuildInitialPrompt creates the first user message for the ReAct loop.
	BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string

	// ShouldStop decides if the loop should terminate based on the LLM response.
	ShouldStop(resp llm.Response, iteration int) bool

	// ParseOutput extracts the structured StageOutput from accumulated results.
	ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, mcpResponses []types.MCPResponse) (*StageOutput, error)

	// DetermineMissingPiece decides what's still missing after this stage.
	DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece
}

// ContinuingEvaluator is an optional interface for agents that want to
// reject the BaseAgent's default "no tool calls + content = stop" rule.
// When the LLM produces a content-only response, BaseAgent.Execute
// checks whether the evaluator implements this interface and, if so,
// asks it for a continuation prompt. Returning shouldContinue=true
// causes the loop to inject the prompt as a user message and run
// another LLM turn, instead of breaking.
//
// The continuationCount parameter is the number of continuation prompts
// already injected for this Execute call, so the evaluator can bound
// its own retry budget. MaxIterations remains the hard upper bound.
type ContinuingEvaluator interface {
	ContinuationPrompt(resp llm.Response, iteration int, continuationCount int, history []types.ToolResult) (prompt string, shouldContinue bool)
}

// SynthesizingEvaluator is an optional interface for agents that need
// a dedicated synthesis step after the ReAct investigation loop. When
// the loop ends, BaseAgent.Execute checks whether the evaluator
// implements this interface and, if so, makes one final LLM call with
// a synthesis prompt to produce a clean, comprehensive StageReport.
//
// This cleanly separates investigation (the ReAct loop, where the LLM
// alternates between tool calls and thinking-aloud text) from synthesis
// (one focused call that reads all tool results and produces the final
// answer). Without this separation, the StageReport is whichever
// assistant message happened to be last — which after continuation
// pushes is often a brief fragment, not the comprehensive answer.
//
// The synthesis prompt receives all tool results so it can build a
// structured digest. The returned prompt is injected as a user message
// in a fresh two-message sequence (system + user) — NOT appended to
// the investigation conversation — so the LLM gets a clean context
// without the noise of intermediate summaries and continuation pushes.
type SynthesizingEvaluator interface {
	// SynthesisPrompt returns the prompt for the final synthesis call.
	// toolResults contains every successful tool result from the ReAct
	// loop. The bool return indicates whether synthesis should run;
	// returning false skips it (e.g., when the agent already produced
	// a satisfactory answer during the loop).
	SynthesisPrompt(ctx *types.AgentContext, toolResults []types.ToolResult) (prompt string, shouldSynthesize bool)
}

// MidLoopEvaluator is an optional interface for agents that need to
// inject corrective hints WHILE the LLM is still actively calling
// tools. ContinuationPrompt only fires on soft-stop (LLM produced
// content with no tool calls); when the LLM keeps calling tools but
// in the wrong direction (reading wrong files, missing the question's
// focus), every ContinuationPrompt-based check is blind. MidLoopCheck
// fills that gap: BaseAgent.Execute calls it after each tool-execution
// batch and, if inject=true, appends the returned hint as a fresh
// user message before the next LLM Chat call.
//
// Implementations should:
//   - Throttle internally (e.g., fire at most every 3 iterations) to
//     avoid over-steering the LLM.
//   - Keep hints short and surgical — this runs every iteration, not
//     just at termination.
//   - Return inject=false when there is nothing actionable to say.
type MidLoopEvaluator interface {
	MidLoopCheck(iteration int, lastResult *types.ToolResult, allResults []types.ToolResult) (hint string, inject bool)
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
	return &BaseAgent{
		name: name,
		deps: &Dependencies{
			LLM:           deps.LLM,
			Tools:         deps.Tools,
			MCPServers:    deps.MCPServers,
			SubAgents:     deps.SubAgents,
			MaxIterations: maxIter,
			Emit:          emit,
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
// Tuneable by future config if needed; hard-coded for now because the
// failure mode is acute and any value in [100 KB, 200 KB] would fix it.
const maxToolHistoryBytes = 150 * 1024

// pruneToolHistory stubs out older "tool" role messages in-place when
// their cumulative content size exceeds maxToolHistoryBytes. Walks
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
func pruneToolHistory(messages []llm.Message) bool {
	total := 0
	cutoff := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "tool" {
			continue
		}
		total += len(messages[i].Content)
		if total > maxToolHistoryBytes {
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
// loop termination. This was originally added during the explorer
// knowledge-flow investigation (docs/investigation-explorer-knowledge-
// flow.md) to localize Layer-2 soft-stop and Layer-3 read_file slice
// issues, then removed because it was noisy on stderr. It is back as
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
	continuationsUsed := 0

	// ReAct loop
	for i := 0; i < b.deps.MaxIterations; i++ {
		// Prune older "tool" role messages in-place so cumulative tool
		// output never blows the model's context window on long
		// investigations. Runs every iteration because a single late
		// read_file batch can push us over the budget; the stub is
		// idempotent so already-pruned messages are skipped.
		if pruneToolHistory(messages) {
			logging.Debug("[diag %s] iter=%d TOOL HISTORY PRUNED (budget=%d bytes)",
				b.name, i, maxToolHistoryBytes)
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
				b.name, i, truncForLog(resp.Content, 2000))
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
		// implement ContinuingEvaluator can override and inject a
		// continuation prompt instead — this exists because the LLM
		// often "thinks aloud" mid-investigation ("I'll check X next")
		// without acting, and the default break would silently accept
		// that as the final answer.
		if len(resp.ToolCalls) == 0 {
			// Empty response (no content, no tools) — treat as
			// voluntary stop to avoid an infinite thinking loop.
			if resp.Content == "" {
				logging.Debug("[diag %s] STOP at iter=%d (empty response)", b.name, i)
				break
			}
			if c, ok := b.eval.(ContinuingEvaluator); ok {
				if prompt, cont := c.ContinuationPrompt(resp, i, continuationsUsed, allToolResults); cont {
					continuationsUsed++
					messages = append(messages, llm.Message{
						Role:      "assistant",
						Content:   resp.Content,
						ToolCalls: resp.ToolCalls,
					})
					messages = append(messages, llm.Message{
						Role:    "user",
						Content: prompt,
					})
					logging.Debug("[diag %s] CONTINUE at iter=%d (continuationsUsed=%d)",
						b.name, i, continuationsUsed)
					continue
				}
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

		// Mid-loop check (#34): give the evaluator a chance to inject a
		// corrective hint between tool batches. Unlike ContinuationPrompt
		// (which only fires on soft-stop), this fires on every iteration
		// where the LLM is still actively calling tools — closing the
		// blind spot where the LLM keeps tool-calling in the wrong
		// direction. The evaluator is responsible for throttling.
		if m, ok := b.eval.(MidLoopEvaluator); ok {
			if hint, inject := m.MidLoopCheck(i, lastToolResultPtr, allToolResults); inject {
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: hint,
				})
				logging.Debug("[diag %s] iter=%d MIDLOOP inject len=%d:\n%s\n---",
					b.name, i, len(hint), truncForLog(hint, 1000))
			}
		}
	}

	// Synthesis step: if the evaluator separates investigation from
	// synthesis, make one clean LLM call to produce the final answer.
	// This runs BEFORE ParseOutput so the synthesis response is part
	// of the message history that ParseOutput processes.
	if s, ok := b.eval.(SynthesizingEvaluator); ok {
		if prompt, shouldSynthesize := s.SynthesisPrompt(ctx, allToolResults); shouldSynthesize {
			logging.Debug("[diag %s] SYNTHESIS prompt len=%d", b.name, len(prompt))

			// Build a fresh two-message sequence: reuse the system
			// prompt from the investigation but replace the conversation
			// with a focused synthesis prompt. This gives the LLM clean
			// context without the noise of intermediate summaries.
			synthMessages := []llm.Message{
				messages[0], // system prompt
				{Role: "user", Content: prompt},
			}

			synthResp, err := b.deps.LLM.Chat(synthMessages, nil)
			if err != nil {
				logging.Debug("[diag %s] SYNTHESIS failed: %v", b.name, err)
			} else if synthResp.Content != "" {
				logging.Debug("[diag %s] SYNTHESIS result len=%d", b.name, len(synthResp.Content))
				messages = append(messages, llm.Message{
					Role:    "assistant",
					Content: synthResp.Content,
				})
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

func (b *BaseAgent) buildInitialMessages(ctx *types.AgentContext, sk *skill.Config) []llm.Message {
	// Build full prompt context from agent context + skill config
	pc := agentctx.BuildPromptContext(ctx, sk)
	ctxMsgs := agentctx.ToMessages(pc)

	// Convert context.Message to llm.Message
	var messages []llm.Message
	for _, m := range ctxMsgs {
		messages = append(messages, llm.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// Append evaluator-specific instruction if provided
	if instruction := b.eval.BuildInitialPrompt(ctx, sk); instruction != "" {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: instruction,
		})
	}

	return messages
}

func (b *BaseAgent) executeTool(ctx *types.AgentContext, tc llm.ToolCall) (*types.ToolResult, *types.MCPResponse) {
	// Try local tool first
	if b.deps.Tools != nil {
		if _, err := b.deps.Tools.Get(tc.Name); err == nil {
			// The busCtx handed to a tool is intentionally narrow:
			// only RepoRoot/Branch/Commit (read-only env info) plus
			// Mutable (the shared, tool-writable region) are populated.
			// All other BusContext fields are zero-valued, so tools
			// physically cannot mutate stage-output state.
			busCtx := &types.BusContext{
				RepoRoot: ctx.RepoRoot,
				Branch:   ctx.Branch,
				Commit:   ctx.Commit,
				WorkDir:  ctx.WorkDir,
				Mutable:  ctx.Mutable,
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
