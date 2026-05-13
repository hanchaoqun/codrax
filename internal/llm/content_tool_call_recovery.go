package llm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// recoverTextToolCalls is an opt-in provider-compatibility shim for small
// OpenAI-compatible models that receive a tool catalog but serialize
// the intended tool call into assistant content instead of the
// protocol-level tool_calls field.
//
// The boundary is intentionally narrow:
//   - only fires when the provider returned zero real tool calls;
//   - only fires when tools were actually supplied and tool_choice
//     did not forbid tools;
//   - always accepts only complete JSON / tool_call envelopes for known
//     tools;
//   - only in required-tool stages, also accepts wrapper prose around
//     fenced, tagged, or balanced JSON-object envelopes.
//
// It never reads prose-looking content or repairs malformed JSON /
// schema fields. Tool parameter validation remains owned by the
// normal tool execution path.
func recoverTextToolCalls(resp Response, tools []ToolSchema, opts ChatOptions) Response {
	if len(resp.ToolCalls) > 0 || len(tools) == 0 || opts.ToolChoice == "none" {
		return resp
	}
	calls, ok := parseTextToolCallEnvelope(resp.Content, tools, opts.ToolChoice == "required")
	if !ok || len(calls) == 0 {
		return resp
	}
	resp.Content = ""
	resp.ToolCalls = calls
	resp.StopReason = "tool_use"
	return resp
}

func parseTextToolCallEnvelope(content string, tools []ToolSchema, allowEmbedded bool) ([]ToolCall, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, false
	}
	allowed := make(map[string]bool, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name != "" {
			allowed[name] = true
		}
	}
	if len(allowed) == 0 {
		return nil, false
	}
	if calls, ok := parseTextToolCallJSON(content, allowed); ok {
		return calls, true
	}
	if !allowEmbedded {
		return nil, false
	}
	blocks := embeddedTextToolCallBlocks(content)
	if len(blocks) == 0 {
		blocks = embeddedJSONObjectBlocks(content)
	}
	if len(blocks) == 0 {
		return nil, false
	}
	var out []ToolCall
	for _, block := range blocks {
		calls, ok := parseTextToolCallJSON(block, allowed)
		if !ok {
			return nil, false
		}
		out = append(out, calls...)
	}
	return renumberGeneratedContentToolCallIDs(out), len(out) > 0
}

func parseTextToolCallJSON(content string, allowed map[string]bool) ([]ToolCall, bool) {
	content = trimWholeTextToolCallEnvelope(content)
	if content == "" {
		return nil, false
	}
	var raw any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, false
	}
	calls, ok := parseTextToolCallValue(raw, allowed)
	if !ok || len(calls) == 0 {
		return nil, false
	}
	return renumberGeneratedContentToolCallIDs(calls), true
}

func trimWholeTextToolCallEnvelope(content string) string {
	s := strings.TrimSpace(content)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 3 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") &&
			strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			s = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}
	for _, tag := range []string{"tool_call", "minimax:tool_call"} {
		open := "<" + tag + ">"
		close := "</" + tag + ">"
		if strings.HasPrefix(s, open) && strings.HasSuffix(s, close) {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, open), close))
		}
	}
	return s
}

type embeddedToolCallBlock struct {
	start int
	body  string
}

func embeddedTextToolCallBlocks(s string) []string {
	if blocks := embeddedFencedBlocks(s); len(blocks) > 0 {
		return embeddedBlockBodies(blocks)
	}
	var blocks []embeddedToolCallBlock
	for _, tag := range []string{"tool_call", "minimax:tool_call"} {
		blocks = append(blocks, embeddedTaggedBlocks(s, "<"+tag+">", "</"+tag+">")...)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].start < blocks[j].start })
	return embeddedBlockBodies(blocks)
}

func embeddedBlockBodies(blocks []embeddedToolCallBlock) []string {
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if body := strings.TrimSpace(block.body); body != "" {
			out = append(out, body)
		}
	}
	return out
}

func embeddedFencedBlocks(s string) []embeddedToolCallBlock {
	var blocks []embeddedToolCallBlock
	for pos := 0; pos < len(s); {
		relStart := strings.Index(s[pos:], "```")
		if relStart < 0 {
			break
		}
		start := pos + relStart
		afterStart := s[start+3:]
		lineEnd := strings.Index(afterStart, "\n")
		if lineEnd < 0 {
			break
		}
		bodyStart := start + 3 + lineEnd + 1
		endRel := strings.Index(s[bodyStart:], "```")
		if endRel < 0 {
			break
		}
		bodyEnd := bodyStart + endRel
		blocks = append(blocks, embeddedToolCallBlock{start: start, body: s[bodyStart:bodyEnd]})
		pos = bodyEnd + 3
	}
	return blocks
}

func embeddedTaggedBlocks(s, open, close string) []embeddedToolCallBlock {
	var blocks []embeddedToolCallBlock
	for pos := 0; pos < len(s); {
		relStart := strings.Index(s[pos:], open)
		if relStart < 0 {
			break
		}
		start := pos + relStart
		bodyStart := start + len(open)
		endRel := strings.Index(s[bodyStart:], close)
		if endRel < 0 {
			break
		}
		bodyEnd := bodyStart + endRel
		blocks = append(blocks, embeddedToolCallBlock{start: start, body: s[bodyStart:bodyEnd]})
		pos = bodyEnd + len(close)
	}
	return blocks
}

func embeddedJSONObjectBlocks(s string) []string {
	var blocks []string
	for pos := 0; pos < len(s); {
		startRel := strings.IndexByte(s[pos:], '{')
		if startRel < 0 {
			break
		}
		start := pos + startRel
		end, ok := balancedJSONObjectEnd(s, start)
		if !ok {
			return nil
		}
		blocks = append(blocks, strings.TrimSpace(s[start:end]))
		pos = end
	}
	return blocks
}

func balancedJSONObjectEnd(s string, start int) (int, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

func parseTextToolCallValue(v any, allowed map[string]bool) ([]ToolCall, bool) {
	switch x := v.(type) {
	case []any:
		var out []ToolCall
		for _, item := range x {
			calls, ok := parseTextToolCallValue(item, allowed)
			if !ok {
				return nil, false
			}
			out = append(out, calls...)
		}
		return out, len(out) > 0
	case map[string]any:
		if rawCalls, ok := x["tool_calls"]; ok {
			return parseTextToolCallsArray(rawCalls, allowed)
		}
		if rawCall, ok := x["function_call"]; ok {
			return parseTextToolCallValue(rawCall, allowed)
		}
		call, ok := parseSingleTextToolCall(x, allowed, 0)
		if !ok {
			return nil, false
		}
		return []ToolCall{call}, true
	default:
		return nil, false
	}
}

func parseTextToolCallsArray(raw any, allowed map[string]bool) ([]ToolCall, bool) {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, false
	}
	out := make([]ToolCall, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		call, ok := parseSingleTextToolCall(m, allowed, i)
		if !ok {
			return nil, false
		}
		out = append(out, call)
	}
	return out, true
}

func parseSingleTextToolCall(m map[string]any, allowed map[string]bool, idx int) (ToolCall, bool) {
	name, args, ok := textToolCallNameAndArgs(m)
	if !ok || !allowed[name] {
		return ToolCall{}, false
	}
	params, ok := normalizeTextToolCallArgs(args)
	if !ok {
		return ToolCall{}, false
	}
	id := textToolCallStringField(m, "id")
	if id == "" {
		id = fmt.Sprintf("content_tool_call_%d", idx)
	}
	return ToolCall{ID: id, Name: name, Params: params}, true
}

func renumberGeneratedContentToolCallIDs(calls []ToolCall) []ToolCall {
	for i := range calls {
		if calls[i].ID == "" || strings.HasPrefix(calls[i].ID, "content_tool_call_") {
			calls[i].ID = fmt.Sprintf("content_tool_call_%d", i)
		}
	}
	return calls
}

func textToolCallNameAndArgs(m map[string]any) (string, any, bool) {
	if fnRaw, ok := m["function"]; ok {
		if fn, ok := fnRaw.(map[string]any); ok {
			name := textToolCallStringField(fn, "name")
			args, hasArgs := firstPresent(fn, "arguments", "parameters", "input")
			if name != "" {
				if !hasArgs {
					args = map[string]any{}
				}
				return name, args, true
			}
		}
	}
	name := firstNonEmptyString(
		textToolCallStringField(m, "name"),
		textToolCallStringField(m, "tool"),
		textToolCallStringField(m, "tool_name"),
	)
	if name == "" {
		return "", nil, false
	}
	args, ok := firstPresent(m, "arguments", "parameters", "input", "params")
	if !ok {
		args = map[string]any{}
	}
	return name, args, true
}

func normalizeTextToolCallArgs(raw any) (json.RawMessage, bool) {
	if raw == nil {
		return json.RawMessage(`{}`), true
	}
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return json.RawMessage(`{}`), true
		}
		var decoded any
		if err := json.Unmarshal([]byte(s), &decoded); err != nil {
			return nil, false
		}
		raw = decoded
	}
	if _, ok := raw.(map[string]any); !ok {
		return nil, false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	return json.RawMessage(b), true
}

func textToolCallStringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func firstPresent(m map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func firstNonEmptyString(vals ...string) string {
	for _, val := range vals {
		if strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}
