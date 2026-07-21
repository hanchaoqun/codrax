package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
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
//  5. Dropped `}` between adjacent array elements (AUTOREPAIR-1 件1,
//     §29.175 T1-NEW-BRACE; witness customlogs
//     emit_investigation_complete_log.txt:272) — the model emits
//     `..."text":"…",{"claim_uses"…` inside a native blocks array,
//     losing exactly the element's closing `}` before the `,{`
//     element boundary. Trigger is byte-precise, no heuristics: the
//     parse error's offset points at `{`, the previous non-whitespace
//     byte is `,`, the byte before that terminates a JSON value, and
//     a string-aware delimiter walk shows the open container at that
//     point is an object whose PARENT container is an array. The only
//     alternative reading of `,{` inside an object (a nested object
//     value missing its `"key":`) would require fabricating a key
//     name, which is Tier3-forbidden — the element-boundary reading
//     is the unique system-expressible candidate. The repair inserts
//     only the structural `}` token; every model-authored content
//     byte is preserved verbatim, and the result must re-parse (and
//     then still pass the tool's own full strict decode + validator
//     chain downstream) or the original bytes are returned so the
//     legacy reject stays byte-identical.
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
	return repairNamedToolParamsJSON("", raw)
}

func repairNamedToolParamsJSON(toolName string, raw json.RawMessage) (json.RawMessage, bool) {
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
	// Pattern 5: dropped `}` between adjacent array elements
	// (AUTOREPAIR-1 件1, §29.175 T1-NEW-BRACE). Tier1 — disclosure is
	// log-only because no content byte changes.
	if repaired, offset, count, ok := tryInsertMissingObjectClose(raw); ok {
		logging.Warning("[agent] repaired dropped object-close between array elements tool=%s offset=%d count=%d",
			toolName, offset, count)
		return repaired, true
	}
	return raw, false
}

func repairToolCallParamsJSON(toolName string, raw json.RawMessage) (json.RawMessage, bool) {
	if repaired, ok := repairNamedToolParamsJSON(toolName, raw); ok {
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

// maxMissingObjectCloseRepairs bounds pattern-5 iteration: each pass fixes
// exactly one dropped element-boundary `}` and must re-trigger the identical
// error class at a strictly later position, or the repair abandons and the
// caller keeps the original bytes (byte-identical legacy reject).
const maxMissingObjectCloseRepairs = 8

// tryInsertMissingObjectClose implements repair pattern 5 (AUTOREPAIR-1 件1,
// §29.175 T1-NEW-BRACE): a dropped `}` between adjacent elements of a native
// JSON array. Returns (repaired, firstOffset, count, true) when the bounded
// iteration converges on bytes that fully re-parse; otherwise the original
// bytes with ok=false. Only structural `}` tokens are inserted — content
// bytes are never touched.
func tryInsertMissingObjectClose(raw json.RawMessage) (json.RawMessage, int, int, bool) {
	cur := append(json.RawMessage(nil), raw...)
	firstOffset := -1
	lastInsert := -1
	count := 0
	for count < maxMissingObjectCloseRepairs {
		pos, ok := missingObjectCloseInsertionPoint(cur)
		if !ok || pos <= lastInsert {
			return raw, 0, 0, false
		}
		if firstOffset < 0 {
			firstOffset = pos
		}
		next := make(json.RawMessage, 0, len(cur)+1)
		next = append(next, cur[:pos]...)
		next = append(next, '}')
		next = append(next, cur[pos:]...)
		cur = next
		lastInsert = pos + 1
		count++
		var probe interface{}
		if err := json.Unmarshal(cur, &probe); err == nil {
			return cur, firstOffset, count, true
		}
	}
	return raw, 0, 0, false
}

// missingObjectCloseInsertionPoint locates the `,` byte to rewrite as `},`
// for exactly the T1-NEW-BRACE shape, byte-precise:
//
//  1. json.Unmarshal fails with a *json.SyntaxError whose offset points one
//     past a `{` byte (the stdlib offset convention, pinned by test);
//  2. the previous non-whitespace byte is `,`;
//  3. the byte before that terminates a JSON value (`"`, digit, `}`, `]`,
//     or the tail of true/false/null);
//  4. a string-aware delimiter walk up to the `,` shows the innermost open
//     container is an object whose parent container is an array.
//
// Any condition miss returns ok=false — the caller then leaves the payload
// for the legacy malformed-params reject.
func missingObjectCloseInsertionPoint(raw json.RawMessage) (int, bool) {
	var probe interface{}
	err := json.Unmarshal(raw, &probe)
	if err == nil {
		return 0, false
	}
	var syn *json.SyntaxError
	if !errors.As(err, &syn) {
		return 0, false
	}
	bracePos := int(syn.Offset) - 1
	if bracePos <= 0 || bracePos >= len(raw) || raw[bracePos] != '{' {
		return 0, false
	}
	commaPos := prevNonJSONWhitespace(raw, bracePos-1)
	if commaPos < 0 || raw[commaPos] != ',' {
		return 0, false
	}
	valueEnd := prevNonJSONWhitespace(raw, commaPos-1)
	if valueEnd < 0 {
		return 0, false
	}
	switch b := raw[valueEnd]; {
	case b == '"' || b == '}' || b == ']':
	case b >= '0' && b <= '9':
	case b == 'e' || b == 'l': // tail of true/false ('e') or null ('l')
	default:
		return 0, false
	}
	stack := make([]byte, 0, 8)
	inString := false
	escape := false
	for i := 0; i < commaPos; i++ {
		b := raw[i]
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
			stack = append(stack, '{')
		case '[':
			stack = append(stack, '[')
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return 0, false
			}
			stack = stack[:len(stack)-1]
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return 0, false
			}
			stack = stack[:len(stack)-1]
		}
	}
	if inString || len(stack) < 2 {
		return 0, false
	}
	if stack[len(stack)-1] != '{' || stack[len(stack)-2] != '[' {
		return 0, false
	}
	return commaPos, true
}

func prevNonJSONWhitespace(raw json.RawMessage, from int) int {
	for i := from; i >= 0; i-- {
		switch raw[i] {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			continue
		default:
			return i
		}
	}
	return -1
}

// Bounds for the malformed-params DEBUG evidence (AUTOREPAIR-1 件5, §29.175;
// RUN2FIX-B 件5 delegation point): a bounded prefix plus a bounded window
// around the parse-error offset — enough to reconstruct the malformed shape
// into a fixture after the fact, never the whole payload.
const (
	malformedParamsEvidencePrefixLimit   = 2048
	malformedParamsEvidenceContextRadius = 128 // 256-byte window centered on the error offset
)

// malformedParamsSensitiveKeyMarkers marks token-shaped field names whose
// string values must never reach the log (existing credential-scrub
// convention, cf. internal/llm redactCredential): matched case-insensitively
// as substrings of the field name, values replaced with "[redacted]".
var malformedParamsSensitiveKeyMarkers = []string{
	"token", "secret", "password", "passphrase", "api_key", "apikey", "authorization", "credential",
}

// malformedToolParamsEvidence builds the bounded, redacted DEBUG evidence for
// a params payload no repair lane could fix. Redaction runs over the FULL
// payload before any slicing, so a sensitive value can never straddle a
// window boundary into the log. The error offset is taken from the raw
// bytes' *json.SyntaxError (-1 when the failure has no offset); the context
// window is cut from the redacted bytes at that offset, so it is approximate
// when redaction shortened earlier bytes — acceptable for forensics, never
// for the redaction guarantee. Pure logging payload — zero behavior change.
func malformedToolParamsEvidence(raw json.RawMessage) (prefix string, errOffset int, offsetContext string) {
	redacted := redactTokenShapedParamValues(raw)
	limit := len(redacted)
	if limit > malformedParamsEvidencePrefixLimit {
		limit = malformedParamsEvidencePrefixLimit
	}
	prefix = string(redacted[:limit])
	errOffset = -1
	var probe interface{}
	err := json.Unmarshal(raw, &probe)
	var syn *json.SyntaxError
	if err == nil || !errors.As(err, &syn) {
		return prefix, errOffset, ""
	}
	errOffset = int(syn.Offset)
	center := errOffset
	if center > len(redacted) {
		center = len(redacted)
	}
	lo := center - malformedParamsEvidenceContextRadius
	if lo < 0 {
		lo = 0
	}
	hi := center + malformedParamsEvidenceContextRadius
	if hi > len(redacted) {
		hi = len(redacted)
	}
	return prefix, errOffset, string(redacted[lo:hi])
}

// redactTokenShapedParamValues walks possibly-malformed JSON bytes with a
// string-aware scanner and replaces the string value of any token-shaped key
// with "[redacted]". Best-effort by construction (the input failed to
// parse), but the scan is linear and never emits a sensitive value byte once
// its key matched.
func redactTokenShapedParamValues(raw json.RawMessage) []byte {
	out := make([]byte, 0, len(raw))
	i := 0
	for i < len(raw) {
		if raw[i] != '"' {
			out = append(out, raw[i])
			i++
			continue
		}
		end, ok := jsonStringEnd(raw, i)
		if !ok {
			out = append(out, raw[i:]...)
			break
		}
		key := raw[i+1 : end]
		k := end + 1
		for k < len(raw) && isJSONSpaceByte(raw[k]) {
			k++
		}
		if k >= len(raw) || raw[k] != ':' || !malformedParamsKeyLooksSensitive(key) {
			out = append(out, raw[i:end+1]...)
			i = end + 1
			continue
		}
		out = append(out, raw[i:k+1]...)
		i = k + 1
		for i < len(raw) && isJSONSpaceByte(raw[i]) {
			out = append(out, raw[i])
			i++
		}
		if i >= len(raw) || raw[i] != '"' {
			continue
		}
		valueEnd, ok := jsonStringEnd(raw, i)
		out = append(out, '"')
		out = append(out, []byte("[redacted]")...)
		if !ok {
			break
		}
		out = append(out, '"')
		i = valueEnd + 1
	}
	return out
}

// jsonStringEnd returns the index of the closing quote of the string opening
// at raw[start] (which must be '"'), escape-aware; ok=false when the string
// never terminates.
func jsonStringEnd(raw json.RawMessage, start int) (int, bool) {
	for j := start + 1; j < len(raw); j++ {
		switch raw[j] {
		case '\\':
			j++
		case '"':
			return j, true
		}
	}
	return 0, false
}

func malformedParamsKeyLooksSensitive(key []byte) bool {
	lower := strings.ToLower(string(key))
	for _, marker := range malformedParamsSensitiveKeyMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isJSONSpaceByte(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}
