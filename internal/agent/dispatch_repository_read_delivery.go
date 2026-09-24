package agent

import (
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This map belongs to one Execute call, never to durable or model-authored
// state. A tool result is only a candidate until its exact message is sent.
type dispatchRepositoryReadDelivery struct {
	mutable    *types.MutableState
	generation uint64
	messages   map[string]types.DispatchRepositoryReadMessageReceipt
	seen       map[string]bool
}

func newDispatchRepositoryReadDelivery(ctx *types.AgentContext) *dispatchRepositoryReadDelivery {
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() {
		return nil
	}
	return &dispatchRepositoryReadDelivery{ctx.Mutable, ctx.Mutable.BeginDispatchRepositoryFileRead(), map[string]types.DispatchRepositoryReadMessageReceipt{}, map[string]bool{}}
}

func (d *dispatchRepositoryReadDelivery) observe(callID string, result types.ToolResult) {
	if d == nil || callID == "" {
		return
	}
	if d.seen[callID] {
		delete(d.messages, callID) // reused transport IDs cannot identify a page
		return
	}
	d.seen[callID] = true
	d.messages[callID] = d.mutable.BindDispatchRepositoryReadMessage(d.generation, callID, result)
}

func (d *dispatchRepositoryReadDelivery) snapshot(messages []llm.Message) []types.DispatchRepositoryReadMessageReceipt {
	if d == nil {
		return nil
	}
	counts := map[string]int{}
	for _, message := range messages {
		if message.Role == "tool" {
			counts[message.ToolCallID]++
		}
	}
	var out []types.DispatchRepositoryReadMessageReceipt
	for _, message := range messages {
		if message.Role != "tool" || counts[message.ToolCallID] != 1 {
			continue
		}
		receipt := d.messages[message.ToolCallID]
		if receipt.MatchesMessage(message.ToolCallID, message.Content) {
			out = append(out, receipt)
		}
	}
	return out
}

func (d *dispatchRepositoryReadDelivery) commit(ctx *types.AgentContext, receipts []types.DispatchRepositoryReadMessageReceipt) {
	if d != nil && ctx != nil && ctx.Mutable == d.mutable && ctx.Context().Err() == nil {
		d.mutable.RecordDispatchRepositoryReadDelivery(d.generation, receipts)
	}
}
