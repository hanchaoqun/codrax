package reasoninggraph

import (
	"context"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// GraphReplayRequest describes a read-only replay/local recompute operation.
// It accepts already-materialized events or an event JSON file. Replay never
// calls tools, LLMs, MCP, or worktree APIs; it only normalizes, optionally
// filters, and reduces typed graph events.
type GraphReplayRequest struct {
	Events     []ReasoningEvent
	EventFile  string
	Source     string
	AuditLimit int
	NodeID     string
	MaxEvents  int
}

type GraphReplayResult struct {
	Events     []ReasoningEvent                  `json:"events,omitempty"`
	View       ReasoningGraphView                `json:"view"`
	Audit      *types.ReasoningGraphAuditSummary `json:"audit,omitempty"`
	ReadOnly   bool                              `json:"read_only"`
	ReplayMode string                            `json:"replay_mode,omitempty"`
	NodeID     string                            `json:"node_id,omitempty"`
}

type GraphReplayExecutor struct{}

func (GraphReplayExecutor) Replay(ctx context.Context, req GraphReplayRequest) (GraphReplayResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return GraphReplayResult{}, err
	}
	events := NormalizeEvents(req.Events)
	if len(events) == 0 && strings.TrimSpace(req.EventFile) != "" {
		loaded, err := LoadEventsFromFile(req.EventFile)
		if err != nil {
			return GraphReplayResult{}, err
		}
		events = loaded
	}
	if err := ctx.Err(); err != nil {
		return GraphReplayResult{}, err
	}
	if req.MaxEvents > 0 && len(events) > req.MaxEvents {
		return GraphReplayResult{}, fmt.Errorf("GraphReplayExecutor: event budget exceeded: %d > %d", len(events), req.MaxEvents)
	}
	nodeID := strings.TrimSpace(req.NodeID)
	mode := "full"
	if nodeID != "" {
		events = graphReplayFilterNode(events, nodeID)
		mode = "node"
	}
	view := ReduceEvents(events)
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "graph_replay"
	}
	result := GraphReplayResult{
		Events:     NormalizeEvents(events),
		View:       view,
		Audit:      AuditSummaryFromView(view, source, req.AuditLimit),
		ReadOnly:   true,
		ReplayMode: mode,
		NodeID:     nodeID,
	}
	return result, nil
}

func graphReplayFilterNode(events []ReasoningEvent, nodeID string) []ReasoningEvent {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return NormalizeEvents(events)
	}
	out := make([]ReasoningEvent, 0, len(events))
	for _, event := range NormalizeEvents(events) {
		if strings.TrimSpace(event.NodeID) == nodeID {
			out = append(out, event)
		}
	}
	return NormalizeEvents(out)
}
