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

	// Task list changes
	EventTaskListUpdated
	EventTaskStatusChanged

	// Stage transition
	EventTransition

	// Skill binding
	EventSkillBound
)

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

	// Tool call
	ToolName   string
	ToolCallID string
	ToolOK     bool
	ToolTime   time.Duration

	// Sub-agent
	SubAgentName string
	SubAgentID   string
	SubTaskTitle string
	SubTaskCount int

	// Task
	TaskID     string
	TaskTitle  string
	TaskStatus types.TaskStatus

	// Transition
	FromStage types.PipelineStage
	ToStage   types.PipelineStage
	Reason    string

	// Counts / stats (for end events)
	ToolCallCount int
	MCPCallCount  int
	FactCount     int
	Error         string

	// Full task list snapshot (for EventTaskListUpdated)
	TaskList *types.TaskList
}

// EventEmitter is the callback signature for pipeline event delivery.
// Implementations must be safe for concurrent use.
type EventEmitter func(Event)

// NopEmitter discards all events. Used as the default when no renderer
// is attached so callers never need nil checks.
func NopEmitter(Event) {}
