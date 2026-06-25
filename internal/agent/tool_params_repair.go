package agent

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// repairToolParamsJSON attempts to repair common LLM-induced JSON
// corruption in tool call parameters, returning (repaired, true)
// when a repair was applied or (original, false) when the input
// is already valid or no recognised pattern matched.
//
// Repair patterns covered (s5a-style customer report, REPL
// status-line case 2026-05-07, plus local-model streaming truncation
// reports):
//
//  1. Trailing JSON garbage — a complete top-level value followed
//     by extra whitespace + `}` / `]` / `,`. Common LLM artefact:
//     the model closes its outer thought at the same indent as the
//     params object, producing a stray closing brace. Strict
//     json.Unmarshal rejects with "invalid character '}' after
//     top-level value"; the repair trims the trailing garbage and
//     re-validates.
//
//  2. Trailing comma before `}` / `]` — a streaming-tokeniser
//     artefact where the model emits the array/object terminator
//     after the final element's separator was already written.
//     The repair walks the bytes (string-aware) and removes any
//     `,` whose next non-whitespace neighbour is `}` or `]`.
//
//  3. Missing closing object/array delimiters — a provider may stop
//     the function.arguments stream after a complete field value but
//     before the final `}` / `]`. The repair appends only structural
//     closers already implied by the open delimiters. It refuses to
//     close unterminated strings or incomplete key/value positions
//     such as `{"path":`.
//
//  4. Leading delimiter garbage — a streaming/tool-call aggregator may
//     carry over one or more closing delimiters from the previous tool
//     call and produce `}{"path":"..."}`. The repair strips only
//     whitespace plus `}` / `]` / `,` before the first JSON opener and
//     then requires the remaining bytes to parse as exactly one JSON
//     value. If the remaining value is also missing only trailing
//     structural closers, it applies the same bounded closer repair
//     used for pattern 3. It refuses arbitrary text prefixes and
//     double-object payloads.
//
// The repair is bounded: it only removes trailing/pre-terminator
// garbage or appends deterministic JSON delimiters. The function
// re-parses the repaired bytes via json.Unmarshal before returning,
// so a repaired payload is guaranteed valid; if validation fails the
// original is returned unchanged.
//
// Generic across all tools — no per-tool schema knowledge. Wired
// into BaseAgent.executeTool so every tool benefits without
// per-tool changes.
func repairToolParamsJSON(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 {
		return raw, false
	}
	// Fast path: already valid.
	var probe interface{}
	if err := json.Unmarshal(raw, &probe); err == nil {
		return raw, false
	}
	// Pattern 0: strip safe leading delimiter garbage.
	if repaired, ok := tryTrimLeadingGarbage(raw); ok {
		return repaired, true
	}
	// Pattern 1: decode the first complete value via streaming
	// decoder; if it succeeds and only safe trailing chars remain,
	// trim them.
	if repaired, ok := tryTrimTrailingGarbage(raw); ok {
		return repaired, true
	}
	// Pattern 2: trailing comma removal.
	if repaired, ok := tryRemoveTrailingComma(raw); ok {
		return repaired, true
	}
	// Pattern 3: missing structural closers.
	if repaired, ok := tryCompleteTruncatedJSON(raw); ok {
		return repaired, true
	}
	return raw, false
}

func repairToolCallParamsJSON(toolName string, raw json.RawMessage) (json.RawMessage, bool) {
	if repaired, ok := repairToolParamsJSON(raw); ok {
		return repaired, true
	}
	if json.Valid(bytes.TrimSpace(raw)) {
		return raw, false
	}
	switch types.CanonicalToolName(toolName) {
	case "emit_answer_document":
		if repaired, ok := tool.RepairEmitAnswerDocumentMalformedParams(raw); ok && json.Valid(repaired) {
			return repaired, true
		}
	}
	return raw, false
}

func tryTrimLeadingGarbage(raw json.RawMessage) (json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw, false
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return raw, false
	}
	firstOpener := -1
	for i, b := range trimmed {
		switch b {
		case ' ', '\t', '\n', '\r', '\v', '\f', '}', ']', ',':
			continue
		case '{', '[':
			firstOpener = i
			goto found
		default:
			return raw, false
		}
	}
found:
	if firstOpener <= 0 {
		return raw, false
	}
	repaired := trimmed[firstOpener:]
	var verify interface{}
	if err := json.Unmarshal(repaired, &verify); err == nil {
		return append(json.RawMessage(nil), repaired...), true
	}
	if completed, ok := tryCompleteTruncatedJSON(repaired); ok {
		return completed, true
	}
	return raw, false
}

func toolParamsMalformedJSONKind(raw json.RawMessage, errText string) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "empty_json"
	}
	if toolParamsLookTruncated(raw, errText) {
		return "truncated_json"
	}
	return "malformed_json"
}

func toolParamsLookTruncated(raw json.RawMessage, errText string) bool {
	if strings.Contains(strings.ToLower(errText), "unexpected end of json input") {
		return true
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') || json.Valid(trimmed) {
		return false
	}
	stack := make([]byte, 0, 4)
	inString := false
	escape := false
	for _, b := range trimmed {
		if escape {
			escape = false
			continue
		}
		if inString {
			switch b {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 || stack[len(stack)-1] != b {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
	if escape || inString || len(stack) > 0 {
		return true
	}
	switch trimmed[len(trimmed)-1] {
	case ':', ',':
		return true
	default:
		return false
	}
}

// tryTrimTrailingGarbage handles the pattern "valid JSON value
// followed by extra closing braces / commas / whitespace". Uses
// json.Decoder to parse the first complete value, then validates
// that the bytes after the decoder offset contain only safe
// trailing characters (whitespace + `}` / `]` / `,`). Anything
// else (e.g. a second top-level object) means we'd be silently
// discarding LLM intent — bail.
func tryTrimTrailingGarbage(raw json.RawMessage) (json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var firstValue interface{}
	if err := dec.Decode(&firstValue); err != nil {
		return raw, false
	}
	consumed := dec.InputOffset()
	if int(consumed) >= len(raw) {
		// Decoder consumed everything but Unmarshal still failed?
		// Defensive: bail rather than guess.
		return raw, false
	}
	trail := raw[consumed:]
	for _, b := range trail {
		switch b {
		case ' ', '\t', '\n', '\r', '\v', '\f', '}', ']', ',':
			continue
		default:
			return raw, false
		}
	}
	repaired := raw[:consumed]
	var verify interface{}
	if err := json.Unmarshal(repaired, &verify); err != nil {
		return raw, false
	}
	return repaired, true
}

// tryRemoveTrailingComma scans the input byte-by-byte, string-aware,
// and removes any `,` whose next non-whitespace neighbour is `}` or
// `]`. Returns repaired + true when at least one comma was removed
// AND the result re-parses; otherwise original + false.
func tryRemoveTrailingComma(raw json.RawMessage) (json.RawMessage, bool) {
	out := make([]byte, 0, len(raw))
	inString := false
	escape := false
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		if escape {
			escape = false
			out = append(out, b)
			continue
		}
		if inString {
			switch b {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			out = append(out, b)
			continue
		}
		if b == '"' {
			inString = true
			out = append(out, b)
			continue
		}
		if b == ',' {
			// Look ahead for next non-whitespace byte.
			j := i + 1
			for j < len(raw) {
				switch raw[j] {
				case ' ', '\t', '\n', '\r', '\v', '\f':
					j++
					continue
				}
				break
			}
			if j < len(raw) && (raw[j] == '}' || raw[j] == ']') {
				continue // skip this comma
			}
		}
		out = append(out, b)
	}
	if len(out) == len(raw) {
		return raw, false
	}
	var verify interface{}
	if err := json.Unmarshal(out, &verify); err != nil {
		return raw, false
	}
	return out, true
}

// tryCompleteTruncatedJSON repairs inputs that are syntactically
// complete up to the final object/array delimiter, for example
// `{"pattern":"foo","files_only":true`. It does NOT invent values:
// unterminated strings, dangling colons, and mismatched leading
// closers are left for the model-facing invalid-params path.
func tryCompleteTruncatedJSON(raw json.RawMessage) (json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return raw, false
	}

	out := append([]byte(nil), trimmed...)
	stack := make([]byte, 0, 4)
	inString := false
	escape := false
	for _, b := range out {
		if escape {
			escape = false
			continue
		}
		if inString {
			switch b {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 || stack[len(stack)-1] != b {
				return raw, false
			}
			stack = stack[:len(stack)-1]
		}
	}
	if escape || inString || len(stack) == 0 {
		return raw, false
	}

	out = trimJSONWhitespaceRight(out)
	if len(out) == 0 {
		return raw, false
	}
	switch out[len(out)-1] {
	case ':':
		return raw, false
	case ',':
		out = trimJSONWhitespaceRight(out[:len(out)-1])
		if len(out) == 0 {
			return raw, false
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		out = append(out, stack[i])
	}
	var verify interface{}
	if err := json.Unmarshal(out, &verify); err != nil {
		return raw, false
	}
	return out, true
}

func trimJSONWhitespaceRight(in []byte) []byte {
	for len(in) > 0 {
		switch in[len(in)-1] {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			in = in[:len(in)-1]
		default:
			return in
		}
	}
	return in
}
