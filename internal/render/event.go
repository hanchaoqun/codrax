// Package render provides a rich CLI rendering system for the codrax pipeline.
//
// The rendering architecture is event-driven: the orchestrator and agents emit
// typed Event values through an EventEmitter callback, and the Renderer
// consumes them to produce styled terminal output in real time. This
// decoupling means the pipeline code never imports terminal libraries and the
// renderer can be swapped (e.g. JSON for CI, plain text for pipes).
package render

import (
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EventKind identifies what happened in the pipeline.
type EventKind int

const (
	// Pipeline lifecycle
	EventPipelineStart EventKind = iota
	EventPipelineEnd

	// Stage lifecycle
	EventStageStart
	EventStageEnd

	// Agent activity
	EventAgentThinking // LLM call started
	EventAgentResponse // LLM call returned

	// Tool calls
	EventToolCallStart
	EventToolCallEnd

	// Sub-agent lifecycle
	EventSubAgentStart
	EventSubAgentEnd

	// Task / objective lifecycle
	EventObjectiveStarted
	EventObjectiveDone

	// Agent reasoning text (LLM's thinking before tool calls)
	EventAgentReasoning

	// Stage transition
	EventTransition

	// Skill binding
	EventSkillBound

	// AnalysisIR task graph ready — emitted once by the orchestrator
	// after the analyze phase succeeds. Carries the derived task node
	// list so the renderer can replace its stage-dispatch task rows
	// with the analyzer's actual task / sub-task breakdown.
	EventAnalysisReady

	// Per-TaskNode lifecycle — emitted from the DAG scheduler as
	// nodes enter / leave the running state. Renderers use these to
	// drive node-row transitions (pending → running → done/failed).
	EventTaskNodeStart
	EventTaskNodeEnd

	// Live preview of streaming assistant content. Emitted by
	// BaseAgent when the LLM adapter surfaces content chunks mid-
	// response (streaming opt-in). Renderer updates the current
	// task row's detail line in place — does NOT print a new line
	// above the area — so the user sees the reply being typed out
	// without the reasoning feed ballooning.
	EventAgentContent

	// Live preview of the finalizer's `summary` field as the
	// emit_answer_document tool-call arguments stream in. PreviewText
	// carries the cumulative decoded summary so far; PreviewRound
	// labels the draft round (1 for first attempt, 2+ for retries
	// after a contract reject so the user knows the visible text is
	// tentative). Renderer manages a pterm.AreaPrinter dedicated to
	// this preview — first chunk stops the spinner and opens the
	// area, subsequent chunks update it in place.
	//
	// EventLivePreviewClear ends the preview: PreviewRejected
	// indicates whether the round was rejected (true → flash a
	// "已重写" marker briefly, then erase) or accepted/finalised
	// (false → erase). Either way the area is gone before the
	// orchestrator's final RenderAnswerDocument-derived bordered
	// answer prints.
	EventLivePreviewChunk
	EventLivePreviewClear

	// Adapter-level retry / fallback signals. Emitted from inside
	// the LLM adapter retry loop (openai.go) and the FallbackAdapter
	// (fallback.go) just before sleeping / swapping. Without these
	// the renderer dock would keep showing "请求模型中" during the
	// retry window even though the request has already failed and
	// the adapter is sleeping in backoff. RetryAttempt is 1-based
	// (1 = "first try failed, about to retry"); RetryDelaySec is
	// the next sleep duration in seconds; ToolName is overloaded
	// here to carry the new provider's model id when this is a
	// fallback event.
	EventAdapterRetry
	EventAdapterFallback
)

// TaskNodeInfo is the renderable summary of a TaskGraph node carried
// on EventAnalysisReady. It is a projection of types.TaskNode onto the
// fields the renderer needs — id for matching, type for row icon /
// ordering, objective for the label. Hidden nodes (counterfactual,
// probe) are filtered out by the orchestrator before this list is
// emitted so the renderer does not need to know the filtering rules.
type TaskNodeInfo struct {
	ID        string
	Type      string
	Objective string
}

// Event is a single lifecycle occurrence emitted by the pipeline.
type Event struct {
	Kind      EventKind
	Timestamp time.Time

	// Pipeline
	TraceID string

	// Stage / agent
	Stage types.PipelineStage
	Agent types.AgentName
	Skill string

	// ReAct iteration (0-based, carried on EventAgentThinking)
	Iteration int

	// Tool call
	ToolName   string
	ToolCallID string
	ToolDetail string // short arg summary, e.g. file path or command
	ToolOK     bool
	ToolTime   time.Duration

	// Sub-agent
	SubAgentName string
	SubAgentID   string
	SubTaskTitle string
	SubTaskCount int

	// Objective (formerly Task)
	Objective string

	// Agent reasoning (think-aloud text from LLM)
	Reasoning string

	// Transition
	FromStage types.PipelineStage
	ToStage   types.PipelineStage
	Reason    string

	// Counts / stats (for end events)
	ToolCallCount int
	MCPCallCount  int
	FactCount     int
	Error         string

	// Analysis-ready payload: the analyzer's task graph projected onto
	// renderer-consumable fields. Populated only on EventAnalysisReady.
	TaskNodes []TaskNodeInfo

	// Per-node lifecycle payload — populated on EventTaskNodeStart /
	// EventTaskNodeEnd. NodeID is the TaskGraph node identifier; the
	// renderer matches these against the TaskNodes list emitted with
	// EventAnalysisReady to locate the row to update.
	NodeID        string
	NodeKind      string
	NodeObjective string

	// EventLivePreviewChunk / EventLivePreviewClear payload.
	//   PreviewText     — cumulative decoded summary text so far
	//   PreviewRound    — 1-based round counter (1 = first finalize
	//                     attempt; ≥2 means earlier rounds were
	//                     rejected by AnswerContract / Tier1 floor)
	//   PreviewRejected — only meaningful on EventLivePreviewClear.
	//                     true → contract rejected this round, area
	//                     gets a "已重写" marker before erase. false
	//                     → final clean erase before bordered answer.
	PreviewText     string
	PreviewRound    int
	PreviewRejected bool

	// EventAdapterRetry / EventAdapterFallback payload.
	//   RetryAttempt — 1-based index of the failed attempt
	//   RetryDelay   — backoff duration before next attempt
	//   RetryReason  — short human phrase ("rate limit" / "stream stalled")
	//   FallbackFrom / FallbackTo — provider model ids on swap
	RetryAttempt int
	RetryDelay   time.Duration
	RetryReason  string
	FallbackFrom string
	FallbackTo   string
}

// EventEmitter is the callback signature for pipeline event delivery.
// Implementations must be safe for concurrent use.
type EventEmitter func(Event)

// NopEmitter discards all events. Used as the default when no renderer
// is attached so callers never need nil checks.
func NopEmitter(Event) {}
