package agent

import (
	"sync"

	"github.com/hanchaoqun/codrax/internal/llm"
)

// llmStreamActivityTracker is request-local passive telemetry. It never feeds
// routing, retries, evidence, validation, or answer construction; the only
// consumer is the slow-request heartbeat shown to an operator.
type llmStreamActivityTracker struct {
	mu            sync.Mutex
	transportSeen bool
	protocolSeen  bool
	semanticSeen  bool
	lastKind      llm.StreamActivityKind
	transportByte int64
}

type llmStreamActivitySnapshot struct {
	TransportSeen bool
	ProtocolSeen  bool
	SemanticSeen  bool
	LastKind      string
	TransportByte int64
}

func (t *llmStreamActivityTracker) observe(activity llm.StreamActivity) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastKind = activity.Kind
	switch activity.Kind {
	case llm.StreamActivityTransportBytes:
		t.transportSeen = true
		if activity.Bytes > 0 {
			t.transportByte += int64(activity.Bytes)
		}
	case llm.StreamActivitySSEFraming, llm.StreamActivityEmptyData, llm.StreamActivityMalformedData:
		t.transportSeen = true
	case llm.StreamActivityProtocol:
		t.transportSeen = true
		t.protocolSeen = true
	case llm.StreamActivityReasoning, llm.StreamActivityContent, llm.StreamActivityToolCall:
		t.transportSeen = true
		t.protocolSeen = true
		t.semanticSeen = true
	}
}

func (t *llmStreamActivityTracker) snapshot() llmStreamActivitySnapshot {
	if t == nil {
		return llmStreamActivitySnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return llmStreamActivitySnapshot{
		TransportSeen: t.transportSeen,
		ProtocolSeen:  t.protocolSeen,
		SemanticSeen:  t.semanticSeen,
		LastKind:      string(t.lastKind),
		TransportByte: t.transportByte,
	}
}
