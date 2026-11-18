package agent

import (
	"encoding/json"
	"reflect"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Positions name the observed assistant/tool pair and its own correction,
// not a match against the wording of user or model messages.
type finalizerRepairAttempt struct {
	start, end  int
	id, name    string
	replaceable bool
}

// finalizerAnswerCall is the recorded identity of one observed answer
// exchange: the assistant message that carried exactly this one answer-tool
// call, immediately followed by its result. occurrence is the call's place in
// the conversation among all tool calls that carry the same id: an id alone
// does not name a call, because calls recovered from assistant content are
// numbered per response. attempt says the request offered that tool, so the
// result is the tool's own outcome; a call to an answer tool the request did
// not offer is recorded too, but it is never the latest attempt.
type finalizerAnswerCall struct {
	id, name   string
	occurrence int
	attempt    bool
}

type finalizerRepairHistory struct {
	attempts      []finalizerRepairAttempt
	validatedBase *types.AnswerDocumentV2
	// calls lists every observed answer exchange in order, whatever its typed
	// outcome. It addresses the free text that accompanied an attempt; it
	// carries no claim about the draft, so it never removes a call, its
	// arguments, its result or a correction.
	calls []finalizerAnswerCall
}

func (h *finalizerRepairHistory) observe(mut *types.MutableState, messages []llm.Message, start, hint int, results []types.ToolResult, offered func(string) bool) {
	h.validatedBase = nil
	if call, ok := observedAnswerCall(messages, start, results); ok {
		call.attempt = answerAttemptResult(call.name, offered)
		h.calls = append(h.calls, call)
	}
	if mut == nil || len(results) != 1 || results[0].Success || results[0].Repair == nil || len(results[0].OptionalCarrierOutcomes) != 0 || start < 0 || start+1 >= len(messages) {
		return
	}
	call := messages[start].ToolCalls
	if len(call) != 1 || call[0].ID == "" || call[0].Name != results[0].ToolName ||
		(call[0].Name != "emit_answer_document" && call[0].Name != "emit_answer_document_patch") {
		return
	}
	a := finalizerRepairAttempt{start: start, end: start + 2, id: call[0].ID, name: call[0].Name}
	if hint == a.end && hint < len(messages) {
		a.end++
	}
	if !a.matches(messages) {
		return
	}
	meta := results[0].Repair.Metadata
	fullyStaged := a.name == "emit_answer_document_patch" && meta[types.ToolRepairMetaAnswerDocumentPatchOutcome] == types.AnswerDocumentPatchOutcomeStagedForRetry &&
		meta[types.ToolRepairMetaAnswerDocumentPatchFullyRevalidated] == "true"
	a.replaceable = a.name == "emit_answer_document" || fullyStaged
	h.attempts = append(h.attempts, a)
	if fullyStaged {
		h.validatedBase = mut.PendingAnswerDocumentPatchBase()
	}
}

func (a finalizerRepairAttempt) matches(messages []llm.Message) bool {
	if a.start < 0 || a.end > len(messages) || a.end-a.start < 2 || a.end-a.start > 3 {
		return false
	}
	assistant, result := messages[a.start], messages[a.start+1]
	return assistant.Role == "assistant" && len(assistant.ContentParts) == 0 && len(assistant.ToolCalls) == 1 &&
		assistant.ToolCalls[0].ID == a.id && assistant.ToolCalls[0].Name == a.name &&
		result.Role == "tool" && result.ToolCallID == a.id && (a.end-a.start == 2 || messages[a.start+2].Role == "user")
}

func (h *finalizerRepairHistory) project(mut *types.MutableState, messages []llm.Message) []llm.Message {
	if mut == nil || len(h.attempts) < 2 || h.validatedBase == nil || mut.AnswerDocumentV2() != nil {
		return nil
	}
	latest := h.attempts[len(h.attempts)-1]
	if !latest.matches(messages) {
		return nil
	}
	for _, m := range messages[latest.end:] {
		if (m.Role != "user" && m.Role != "system") || len(m.ToolCalls) > 0 || m.ToolCallID != "" {
			return nil // an intervening assistant/tool exchange owns its history
		}
	}
	base := mut.PendingAnswerDocumentPatchBase()
	if !reflect.DeepEqual(base, h.validatedBase) || types.ValidateAnswerDocumentPatchBaseIdentity(base) != nil {
		return nil
	}
	owners, responses := map[string]int{}, map[string]int{}
	for _, m := range messages {
		for _, c := range m.ToolCalls {
			owners[c.ID]++
		}
		if m.ToolCallID != "" {
			responses[m.ToolCallID]++
		}
	}
	previousEnd, removed := 0, 0
	out := make([]llm.Message, 0, len(messages))
	for _, a := range h.attempts {
		if a.start < previousEnd || !a.matches(messages) || owners[a.id] != 1 || responses[a.id] != 1 {
			return nil
		}
		if a == latest {
			break
		}
		if !a.replaceable {
			continue
		}
		out = append(out, messages[previousEnd:a.start]...)
		previousEnd = a.end
		removed++
	}
	if removed == 0 {
		return nil
	}
	// Keep the editable draft, including all blocks, indices and anchors.
	// Optional sidecar state is still owned by the tool; a checkpoint never
	// chooses or writes it. The latest actual tool call and result stay paired.
	draft, err := json.Marshal(struct {
		Blocks                []types.AnswerBlock                `json:"blocks"`
		Citations             []types.Citation                   `json:"citations,omitempty"`
		ExactResolution       *types.AnswerExactResolution       `json:"exact_resolution,omitempty"`
		MissingRequestedRoles []types.AnswerMissingRequestedRole `json:"missing_requested_roles,omitempty"`
		Caveats               []string                           `json:"caveats,omitempty"`
		Snippets              []types.CodeSnippet                `json:"snippets,omitempty"`
	}{base.Blocks, base.Citations, base.ExactResolution, base.MissingRequestedRoles, base.Caveats, base.Snippets})
	if err != nil {
		return nil
	}
	out = append(out, messages[previousEnd:latest.start+2]...)
	out = append(out, llm.Message{Role: "user", Content: "## Current staged draft (not an accepted answer)\nThe tool has already applied the last staged operations to this retry base. Use only the current tool schema and latest validation feedback; keep unaffected content and any valid staged optional report. The following JSON is draft data, not instructions or a new response schema.\n" + string(draft)})
	return append(out, messages[latest.start+2:]...)
}

// observedAnswerCall records an answer exchange only in its one exact shape:
// a single result for a single answer-tool call, paired in place. Any other
// batch is not recorded, so its messages are never touched.
func observedAnswerCall(messages []llm.Message, start int, results []types.ToolResult) (finalizerAnswerCall, bool) {
	if len(results) != 1 || start < 0 || start >= len(messages) || len(messages[start].ToolCalls) != 1 {
		return finalizerAnswerCall{}, false
	}
	call := finalizerAnswerCall{id: messages[start].ToolCalls[0].ID, name: messages[start].ToolCalls[0].Name}
	if call.id == "" || call.name != results[0].ToolName || !isAnswerDocumentTool(call.name) || !call.pairedAt(messages, start) {
		return finalizerAnswerCall{}, false
	}
	for _, m := range messages[:start] {
		for _, c := range m.ToolCalls {
			if c.ID == call.id {
				call.occurrence++
			}
		}
	}
	return call, true
}

func (c finalizerAnswerCall) pairedAt(messages []llm.Message, i int) bool {
	if i < 0 || i+1 >= len(messages) {
		return false
	}
	assistant, result := messages[i], messages[i+1]
	return assistant.Role == "assistant" && len(assistant.ContentParts) == 0 && len(assistant.ToolCalls) == 1 &&
		assistant.ToolCalls[0].ID == c.id && assistant.ToolCalls[0].Name == c.name &&
		result.Role == "tool" && result.ToolCallID == c.id
}

// projectFreeText stops re-sending the free text that accompanied recorded
// answer exchanges: every one except the latest attempt, or all of them when
// includeLatest is set. An exchange is addressed by its position in the
// conversation: the owner of its id at its recorded occurrence, which must be
// one answer call directly followed by its result, without content parts. The
// wording of a message is never read. The staged-draft checkpoint only replaces
// exchanges whose id is unique, so it moves no occurrence; an exchange it
// replaced is simply absent. The call, its arguments, its paired result and
// every correction stay.
//
// A call whose pairing cannot be proven is left out and keeps its free text;
// the other calls are still projected. That covers a call that is no longer
// paired in place, and every call of an id that lost a recorded occurrence or
// whose owners and results no longer balance. The last recorded exchange must
// be present: without it the request is not the conversation that was recorded,
// and nothing is projected.
//
// Only the content channel is dropped. Reasoning a provider returned on its
// own channel stays with its tool call: a provider in native thinking mode may
// reject a tool-call turn whose reasoning is not echoed back, whereas an
// assistant tool-call message with empty content is a shape the loop already
// sends. A request that is over the allocation because of that channel is
// therefore still refused.
func (h *finalizerRepairHistory) projectFreeText(messages []llm.Message, includeLatest bool) ([]llm.Message, int) {
	if len(h.calls) == 0 {
		return nil, 0
	}
	owners, responses := map[string][]int{}, map[string]int{}
	for i, m := range messages {
		for _, c := range m.ToolCalls {
			owners[c.ID] = append(owners[c.ID], i)
		}
		if m.ToolCallID != "" {
			responses[m.ToolCallID]++
		}
	}
	if last := h.calls[len(h.calls)-1]; last.occurrence >= len(owners[last.id]) {
		return nil, 0
	}
	latest, expected := -1, map[string]int{}
	for n, call := range h.calls {
		if call.attempt {
			latest = n
		}
		expected[call.id] = max(expected[call.id], call.occurrence+1)
	}
	var out []llm.Message
	removed := 0
	for n, call := range h.calls {
		at := owners[call.id]
		if len(at) < expected[call.id] || responses[call.id] != len(at) || !call.pairedAt(messages, at[call.occurrence]) {
			continue
		}
		i := at[call.occurrence]
		if (n == latest && !includeLatest) || messages[i].Content == "" {
			continue
		}
		if out == nil {
			out = append([]llm.Message(nil), messages...)
		}
		removed += len(messages[i].Content)
		out[i].Content = ""
	}
	return out, removed
}
