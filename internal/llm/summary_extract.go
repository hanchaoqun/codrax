package llm

import (
	"strings"
	"unicode/utf8"
)

// SummaryExtractor is a best-effort streaming extractor that yields
// the decoded text content of a JSON object's `summary` field as
// chunks arrive. It is designed to consume the raw `arguments` JSON
// string of a tool-call SSE delta and emit decoded text the renderer
// can drop into a live-preview area.
//
// Key design choices:
//
//   - Best-effort, not strict. Garbled escape sequences (truncated
//     `\u00`, malformed `\\X`) degrade to "show as-is in the preview"
//     rather than panic. The user explicitly accepted "杂乱中间态" —
//     the only hard requirement is that the FINAL parsed answer is
//     unaffected, which it is because the extractor never writes back
//     to the LLM adapter's accumulator buffer.
//
//   - No JSON parser dependency. A six-state machine handles
//     scanning, key matching, value entry, and string body, with a
//     small carry-over for splits across `\` escape sequences.
//
//   - Idempotent done state. Once the closing `"` of the summary
//     value has been seen, all subsequent Feed calls are no-ops.
//     Defends against the LLM emitting a second `"summary"` key in a
//     malformed JSON.
//
//   - Single-field scope. Only `summary` is extracted; other fields
//     (`citations`, `symbols`, `snippets`, `steps`, `value`, …) are
//     skipped. The extractor has no opinion about object structure
//     beyond "find the summary key, read its string value".
//
// Use case: BaseAgent wires an instance into the LLM adapter's
// OnToolCallDelta callback when the active stage is finalize and the
// active tool is emit_answer_document. As `arguments` chunks stream
// in, Feed returns decoded summary text the renderer puts into a
// pterm.Area. The orchestrator's downstream parse of the same tool
// call's complete arguments is unaffected.
type SummaryExtractor struct {
	state   summaryState
	done    bool
	carry   string // bytes carried across chunks when we couldn't decide yet
	keyBuf  strings.Builder
	keyDone bool // current key fully scanned (waiting for ':')

	// Skip-mode tracking for stateSkipValue. Used when the current
	// key is not "summary" so we have to consume a non-summary
	// value before returning to outer scanning. skipMode picks the
	// sub-state machine (string body / structured nesting / bare
	// literal); skipDepth tracks {/[ depth for skipStructured.
	skipMode  skipMode
	skipDepth int
}

type summaryState int

const (
	// stateScanning is the outer object scan: looking for a key
	// string opening quote.
	stateScanning summaryState = iota

	// stateInKey is reading the characters of a key string until
	// its closing quote. The accumulated key is then compared to
	// "summary"; on match the state advances to stateAfterKey, on
	// mismatch back to stateScanning (and the value associated with
	// the wrong key is skipped).
	stateInKey

	// stateAfterKey waits for the colon and the opening quote of
	// the value string. We assume `summary` is always a string-
	// valued field; if the LLM emits `"summary": null` or a non-
	// string, we degrade by never entering stateInValue and the
	// preview stays empty (acceptable best-effort).
	stateAfterKey

	// stateInValue reads the characters of the summary string
	// value, emitting decoded text via Feed's return value, until
	// the closing unescaped `"`. Then state advances to stateDone.
	stateInValue

	// stateSkipValue consumes a non-summary value, tracking nesting
	// of strings / objects / arrays so we know when the value ends
	// and we can return to stateScanning.
	stateSkipValue
)

// Feed processes the next chunk of partial JSON. Returns:
//
//	out  — decoded summary text observed in this chunk (may be empty
//	       if no summary content arrived yet).
//	done — true once the closing quote of the summary string has been
//	       seen. Subsequent Feed calls return ("", true) immediately.
//
// Feed is safe to call with empty input (returns "", current done
// flag). It is NOT safe to share an extractor across goroutines —
// callers should keep one per stream.
func (e *SummaryExtractor) Feed(chunk string) (string, bool) {
	if e == nil || e.done {
		return "", true
	}
	if chunk == "" {
		return "", false
	}
	src := e.carry + chunk
	e.carry = ""
	var out strings.Builder
	i := 0
loop:
	for i < len(src) {
		switch e.state {
		case stateScanning:
			r, sz := utf8.DecodeRuneInString(src[i:])
			if r == '"' {
				e.state = stateInKey
				e.keyBuf.Reset()
				e.keyDone = false
				i += sz
				continue
			}
			i += sz

		case stateInKey:
			r, sz := utf8.DecodeRuneInString(src[i:])
			if r == '\\' {
				// Escape inside key: take next byte literally to
				// keep the scanner simple. Keys rarely contain
				// escapes; if they do we just bias toward "no
				// match" which falls back to stateScanning.
				if i+sz >= len(src) {
					e.carry = src[i:]
					break loop
				}
				next, nsz := utf8.DecodeRuneInString(src[i+sz:])
				e.keyBuf.WriteRune(next)
				i += sz + nsz
				continue
			}
			if r == '"' {
				e.keyDone = true
				if e.keyBuf.String() == "summary" {
					e.state = stateAfterKey
				} else {
					e.state = stateAfterKey // shared: wait for ':' then either dive or skip
				}
				i += sz
				continue
			}
			e.keyBuf.WriteRune(r)
			i += sz

		case stateAfterKey:
			r, sz := utf8.DecodeRuneInString(src[i:])
			switch {
			case r == '"':
				// Opening of a string value.
				if e.keyBuf.String() == "summary" {
					e.state = stateInValue
				} else {
					// Non-summary string value: skip its body.
					e.state = stateSkipValue
					e.skipDepth = 1 // simulating "inside one string"
					e.skipMode = skipString
				}
				i += sz
			case r == '{' || r == '[':
				// Object or array value (only relevant for non-
				// summary keys; summary is a string in our schema).
				e.state = stateSkipValue
				e.skipDepth = 1
				e.skipMode = skipStructured
				i += sz
			case r == ':' || r == ' ' || r == '\t' || r == '\n' || r == '\r':
				i += sz
				continue
			default:
				// Bare value (number / true / false / null). Skip
				// until next `,` or `}` at depth 0.
				e.state = stateSkipValue
				e.skipMode = skipBare
				e.skipDepth = 0
			}

		case stateInValue:
			// Read decoded chars until closing unescaped quote.
			r, sz := utf8.DecodeRuneInString(src[i:])
			if r == '\\' {
				// Need at least one more byte to decide.
				if i+sz >= len(src) {
					e.carry = src[i:]
					break loop
				}
				esc, escSz := utf8.DecodeRuneInString(src[i+sz:])
				switch esc {
				case '"':
					out.WriteByte('"')
				case '\\':
					out.WriteByte('\\')
				case '/':
					out.WriteByte('/')
				case 'b':
					out.WriteByte('\b')
				case 'f':
					out.WriteByte('\f')
				case 'n':
					out.WriteByte('\n')
				case 'r':
					out.WriteByte('\r')
				case 't':
					out.WriteByte('\t')
				case 'u':
					// Need 4 more hex chars. If they're not all
					// here yet, carry over and stop.
					hexStart := i + sz + escSz
					if hexStart+4 > len(src) {
						e.carry = src[i:]
						break loop
					}
					hex := src[hexStart : hexStart+4]
					if cp, ok := parseHex4(hex); ok {
						out.WriteRune(cp)
					} else {
						// Malformed — write literal so user sees
						// the raw fragment instead of silent loss.
						out.WriteString("\\u" + hex)
					}
					i += sz + escSz + 4
					continue
				default:
					// Unknown escape: write the raw escape so the
					// user sees something rather than swallowing.
					out.WriteByte('\\')
					out.WriteRune(esc)
				}
				i += sz + escSz
				continue
			}
			if r == '"' {
				// End of summary string.
				e.state = stateDone
				e.done = true
				i += sz
				break loop
			}
			out.WriteRune(r)
			i += sz

		case stateSkipValue:
			r, sz := utf8.DecodeRuneInString(src[i:])
			switch e.skipMode {
			case skipString:
				if r == '\\' {
					if i+sz >= len(src) {
						e.carry = src[i:]
						break loop
					}
					_, escSz := utf8.DecodeRuneInString(src[i+sz:])
					i += sz + escSz
					continue
				}
				if r == '"' {
					e.state = stateScanning
					e.keyBuf.Reset()
				}
				i += sz
			case skipStructured:
				switch r {
				case '"':
					// Enter a string within the structure; consume
					// it without affecting depth.
					e.skipMode = skipNestedString
				case '{', '[':
					e.skipDepth++
				case '}', ']':
					e.skipDepth--
					if e.skipDepth <= 0 {
						e.state = stateScanning
						e.keyBuf.Reset()
					}
				}
				i += sz
			case skipNestedString:
				if r == '\\' {
					if i+sz >= len(src) {
						e.carry = src[i:]
						break loop
					}
					_, escSz := utf8.DecodeRuneInString(src[i+sz:])
					i += sz + escSz
					continue
				}
				if r == '"' {
					e.skipMode = skipStructured
				}
				i += sz
			case skipBare:
				if r == ',' || r == '}' || r == ']' {
					e.state = stateScanning
					e.keyBuf.Reset()
					// Don't consume the delimiter — let the
					// scanning state see it.
					continue
				}
				i += sz
			}

		case stateDone:
			break loop
		}
	}
	return out.String(), e.done
}

type skipMode int

const (
	skipString skipMode = iota
	skipStructured
	skipNestedString
	skipBare
)

const stateDone summaryState = -1

// extra fields on SummaryExtractor (placed here so the file reads
// top-down without a forward-decl tax).
//
// These are state for the skip-non-summary-value paths only; they
// are unused while extracting the summary itself.
//
//nolint:revive // intentional split for readability
type skipState struct {
	mode  skipMode
	depth int
}

// We attach skipMode + skipDepth as direct fields on SummaryExtractor
// (rather than a nested struct) to keep Feed's hot path branch-free
// in the common case. Declared here as additional struct members
// via a method is not how Go works — instead we put them on the
// struct directly above. The skipState type is reserved for a
// future refactor that wants to make the skip subsystem testable
// in isolation.
//
// The actual fields live on SummaryExtractor below — see the
// `skipMode` / `skipDepth` accessors immediately following.

func parseHex4(s string) (rune, bool) {
	if len(s) != 4 {
		return 0, false
	}
	var v rune
	for i := 0; i < 4; i++ {
		c := s[i]
		var d rune
		switch {
		case c >= '0' && c <= '9':
			d = rune(c - '0')
		case c >= 'a' && c <= 'f':
			d = rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = rune(c-'A') + 10
		default:
			return 0, false
		}
		v = v<<4 | d
	}
	return v, true
}
