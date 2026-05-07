package agent

import (
	"bytes"
	"encoding/json"
)

// repairToolParamsJSON attempts to repair common LLM-induced JSON
// corruption in tool call parameters, returning (repaired, true)
// when a repair was applied or (original, false) when the input
// is already valid or no recognised pattern matched.
//
// Two repair patterns covered (s5a-style customer report, REPL
// status-line case 2026-05-07):
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
// The repair is bounded: it never adds JSON syntax, only removes
// trailing or pre-terminator garbage. A successful repair is
// always either a prefix of the original or a strictly shorter
// version with selected commas dropped. The function re-parses
// the repaired bytes via json.Unmarshal before returning, so a
// repaired payload is guaranteed valid; if validation fails the
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
	return raw, false
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
