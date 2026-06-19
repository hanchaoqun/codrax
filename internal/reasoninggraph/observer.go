package reasoninggraph

import (
	"strings"
	"sync"
	"time"
)

// Observer is the side-effect-only event sink used by read/write/tool/LLM
// boundaries. Implementations must never feed observed events back into the
// current tool result or model prompt synchronously; graph consumers read the
// reduced view later.
type Observer interface {
	ObserveReasoningEvent(event ReasoningEvent)
}

type EventCollector struct {
	mu      sync.Mutex
	graphID string
	nextSeq int64
	events  []ReasoningEvent
}

func NewEventCollector(graphID string) *EventCollector {
	return &EventCollector{graphID: strings.TrimSpace(graphID)}
}

func (c *EventCollector) ObserveReasoningEvent(event ReasoningEvent) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if event.GraphID == "" {
		event.GraphID = c.graphID
	}
	if event.Sequence <= 0 {
		c.nextSeq++
		event.Sequence = c.nextSeq
	} else if event.Sequence > c.nextSeq {
		c.nextSeq = event.Sequence
	}
	if event.At.IsZero() {
		event.At = time.Now()
	}
	c.events = append(c.events, NormalizeEvent(event))
}

func (c *EventCollector) Events() []ReasoningEvent {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]ReasoningEvent(nil), c.events...)
	return NormalizeEvents(out)
}

func (c *EventCollector) View() ReasoningGraphView {
	return ReduceEvents(c.Events())
}
