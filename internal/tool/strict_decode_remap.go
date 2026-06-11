package tool

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// strict_decode_remap.go — Phase 1-B (V2 runtime eval followup,
// 2026-05-04). When `json.Decoder.DisallowUnknownFields()` rejects a
// field with `json: unknown field "X"`, the LLM only sees the field
// name — not WHERE in the schema X actually lives. The s1a / u3a
// real-eval forensic showed LLMs burn 5-7 retry iters on this class
// of error: schema description grew from 4 → 6 inner fields when
// Phase 3-C3 added from_node/to_node, and the LLM increasingly
// guessed sibling fields (`citation_ref`) into the wrong nested
// container (`claim_use`).
//
// MisplacedFieldHint is the typed remediation entry: a known
// "wrong-container" pattern + the correct paths the LLM should
// have used. RemapStrictDecodeError walks the hint list and
// rewrites the strict-decode error message to surface the correct
// path; mismatch passes the original error verbatim — but always
// runs through SanitizeDecodeErrorMessage so Go-internal type
// names never leak to the LLM (R4 red line, m1a-20260504-141035
// forensic — Go names like `tool.emitAnswerBlockV2` and
// `emitAnswerDocumentV2Params` were appearing in LLM-facing error
// messages and confusing retries).
//
// Red lines:
//   - Hints are typed exact-match (Field name + ContainerNames),
//     never fuzzy / similarity (R3 precise signal).
//   - The remap NEVER changes the error VALUE (it stays an error,
//     not a silent recovery) — only the message text.
//   - Hint table per emit tool, owned where it's consumed; no
//     global registry.
//   - R4: Go-internal type names are stripped before the message
//     reaches the LLM (caller must use the wrapped error's text,
//     not the inner err).

// MisplacedFieldHint identifies one known LLM error pattern.
type MisplacedFieldHint struct {
	// Field is the field name the LLM placed wrongly. Example:
	// "citation_ref".
	Field string

	// ContainerNames is the set of nested-object names where the
	// field is INVALID (i.e. the LLM-frequent wrong destinations).
	// Example: ["claim_use", "claim_uses"].
	ContainerNames []string

	// CorrectPaths is the set of valid schema paths for the
	// field. The remap renders these as a comma-separated list so
	// the LLM sees concrete locations to copy. Example:
	// ["items[i].citation_ref", "value.citation_ref",
	//  "boolean.citation_ref"].
	CorrectPaths []string
}

// RemapStrictDecodeError inspects err for a json strict-decode
// pattern and rewrites the message to LLM-friendly guidance:
//
//   - `json: unknown field "X"` — when X matches a hint, return a
//     wrapped error naming the correct paths.
//   - `json: cannot unmarshal string into Go struct field
//     <T>.<field> of type <U>` — caller emitted a JSON-encoded
//     string where an array / object was expected (streaming
//     artefact). Rewrite with the target shape the schema actually
//     expects, so object carriers are not mis-taught as arrays.
//   - Otherwise the message is sanitized (R4 — Go internal type
//     names stripped) and returned as-is.
//
// Callers may pass nil/empty hints — in that case the function
// still sanitizes the error message (R4 always applies).
func RemapStrictDecodeError(err error, hints []MisplacedFieldHint) error {
	return RemapStrictDecodeErrorWithRaw(err, hints, nil)
}

// RemapStrictDecodeErrorWithRaw is RemapStrictDecodeError with the raw
// tool params available. The raw bytes disambiguate the
// cannot-unmarshal-string class: when the failing field's value is
// ALREADY a native JSON array, no stringify ever happened — the array
// ELEMENTS have the wrong shape (plain strings where the schema wants
// objects), and the quote-wrapping diagnosis would be doubly wrong
// (blaming an artefact that never occurred and steering toward a
// native object when the correct shape is an array of objects).
func RemapStrictDecodeErrorWithRaw(err error, hints []MisplacedFieldHint, raw []byte) error {
	if err == nil {
		return err
	}
	// Step 1: try the unknown-field hint match.
	if field := extractUnknownFieldName(err); field != "" {
		for _, h := range hints {
			if h.Field != field {
				continue
			}
			paths := strings.Join(h.CorrectPaths, " / ")
			containers := strings.Join(h.ContainerNames, " / ")
			// Operator-side telemetry — record every remap event so
			// eval scripts can count misplacement frequency and the
			// G7 path-sensitive prescan ROI can be evaluated against
			// real data without speculation.
			logging.Info("[strict_decode_remap] misplaced field %q → suggested paths: %s",
				field, paths)
			return fmt.Errorf(
				"%w — field %q exists in the schema at %s, NOT inside %s; relocate the value (do not rename or remove it)",
				err, field, paths, containers)
		}
	}
	// Step 2: try the cannot-unmarshal-string artefact pattern.
	if rewritten, matched := remapCannotUnmarshalStringIntoNativeJSON(err, raw); matched {
		return rewritten
	}
	// Step 3: sanitize the error message — strip Go-internal
	// type names that violate R4 (no internal jargon to LLM).
	return sanitizeDecodeError(err)
}

// unknownFieldRe matches Go's standard strict-decode error message
// shape: `json: unknown field "<name>"`. The `<name>` capture
// pulls the field literal verbatim.
var unknownFieldRe = regexp.MustCompile(`json: unknown field "([^"]+)"`)

// extractUnknownFieldName returns the field name from err's
// message when err matches Go's strict-decode shape; "" otherwise.
// Uses errors.As / errors.Is would be nicer if Go exposed a typed
// error here, but encoding/json wraps strict-decode failures in
// fmt.Errorf strings only (since Go 1.5+), so we string-match.
func extractUnknownFieldName(err error) string {
	if err == nil {
		return ""
	}
	// errors.Unwrap chain — handle wrapped strict-decode errors.
	for cur := err; cur != nil; cur = errors.Unwrap(cur) {
		if m := unknownFieldRe.FindStringSubmatch(cur.Error()); len(m) == 2 {
			return m[1]
		}
	}
	return ""
}

// cannotUnmarshalStringRe matches the Go json error shape
// `json: cannot unmarshal string into Go struct field <T>.<field>
// of type <U>`. The capture groups extract the user-visible field
// name (e.g. `blocks`) and the target type shape so the rewritten
// message can distinguish native arrays from native objects.
//
// The pattern matches both top-level fields and nested-struct
// references. The raw target type is never surfaced to the LLM; it is
// used only to choose array-vs-object wording before R4 sanitization.
var cannotUnmarshalStringRe = regexp.MustCompile(
	`json: cannot unmarshal string into Go struct field [A-Za-z_][A-Za-z0-9_]*\.([A-Za-z_][A-Za-z0-9_]*) of type ([^\s]+)`,
)

type jsonStringCarrierKind string

const (
	jsonStringCarrierArray  jsonStringCarrierKind = "array"
	jsonStringCarrierObject jsonStringCarrierKind = "object"
)

// remapCannotUnmarshalStringIntoNativeJSON detects streaming artefacts where
// the LLM JSON.stringify'd a structured value before putting it in the tool
// call. It uses the decoder's target type only to choose array/object guidance;
// the Go type name never appears in the LLM-facing message.
func remapCannotUnmarshalStringIntoNativeJSON(err error, raw []byte) (error, bool) {
	field, kind := extractCannotUnmarshalStringTarget(err)
	if field == "" || kind == "" {
		return nil, false
	}
	// Disambiguate against the raw params: a field whose raw value is
	// already a native array was never stringified — the decode error
	// came from a string ELEMENT where the schema wants an object. Go's
	// error cites the element struct type (package-qualified), which the
	// kind classifier reads as "object", so without this check the model
	// is told to quote-strip a wrapper that does not exist and to emit a
	// native OBJECT when the correct shape is an array of objects.
	if rawFieldValueKind(raw, field) == '[' {
		logging.Info("[strict_decode_remap] array-element shape field %q", field)
		return fmt.Errorf(
			"each entry in the %q array must be a native JSON object carrying this tool's schema fields for that entry (e.g. %q), not a plain string — keep the array, re-emit every entry as an object",
			field, field+`: [{...}, {...}]`), true
	}
	logging.Info("[strict_decode_remap] string-carrier field %q kind=%s", field, kind)
	if kind == jsonStringCarrierObject {
		return fmt.Errorf(
			"the %q field must be a native JSON object (e.g. %q), not a JSON-encoded string. The streaming layer wrapped your %q value in quotes — re-emit %q as a native object (no surrounding quotes, no escaped inner quotes)",
			field, field+`: {...}`, field, field), true
	}
	return fmt.Errorf(
		"the %q field must be a native JSON array of objects (e.g. %q), not a JSON-encoded string. The streaming layer wrapped your %q value in quotes — re-emit %q as a native array (no surrounding quotes, no escaped inner quotes)",
		field, field+`: [{...}, {...}]`, field, field), true
}

// rawFieldValueKind reports the first byte of the JSON value following
// the first `"<field>"` key occurrence in raw: '"' for a string, '['
// for an array, '{' for an object, 0 when absent or raw is nil. A
// byte-level scan, not a full parse — raw may be the very bytes that
// failed strict decode, but the key/value syntax up to the failing
// field is well-formed (encoding/json validated it before erroring on
// the type mismatch).
func rawFieldValueKind(raw []byte, field string) byte {
	if len(raw) == 0 || field == "" {
		return 0
	}
	key := []byte(`"` + field + `"`)
	idx := 0
	for {
		j := indexBytesFrom(raw, key, idx)
		if j < 0 {
			return 0
		}
		k := j + len(key)
		for k < len(raw) && (raw[k] == ' ' || raw[k] == '\t' || raw[k] == '\n' || raw[k] == '\r') {
			k++
		}
		if k < len(raw) && raw[k] == ':' {
			k++
			for k < len(raw) && (raw[k] == ' ' || raw[k] == '\t' || raw[k] == '\n' || raw[k] == '\r') {
				k++
			}
			if k < len(raw) {
				return raw[k]
			}
			return 0
		}
		idx = j + 1
	}
}

func indexBytesFrom(haystack, needle []byte, from int) int {
	if from >= len(haystack) {
		return -1
	}
	i := strings.Index(string(haystack[from:]), string(needle))
	if i < 0 {
		return -1
	}
	return from + i
}

func extractCannotUnmarshalStringField(err error) string {
	field, _ := extractCannotUnmarshalStringTarget(err)
	return field
}

func extractCannotUnmarshalStringTarget(err error) (string, jsonStringCarrierKind) {
	if err == nil {
		return "", ""
	}
	for cur := err; cur != nil; cur = errors.Unwrap(cur) {
		m := cannotUnmarshalStringRe.FindStringSubmatch(cur.Error())
		if len(m) != 3 {
			continue
		}
		field := m[1]
		targetType := strings.TrimSpace(m[2])
		switch {
		case strings.HasPrefix(targetType, "[]"):
			return field, jsonStringCarrierArray
		case strings.HasPrefix(targetType, "*") ||
			strings.HasPrefix(targetType, "map[") ||
			strings.Contains(targetType, "."):
			return field, jsonStringCarrierObject
		default:
			return "", ""
		}
	}
	return "", ""
}

// goInternalTypeRe matches Go-internal type tokens that R4 forbids
// in LLM-facing strings. The pattern covers:
//
//   - package-qualified types (`tool.emitAnswerBlockV2`,
//     `types.AnswerBlockItem`)
//   - bare camelCase struct names that are clearly Go-shape
//     (start with lowercase, contain at least one uppercase, end
//     with capitalised "Params" / "Block" / "Item" etc.)
//
// Go-internal-shape detection is deliberately conservative — false
// positives (a legitimate user-facing token also matching) are
// preferable to LLM-confusion; the replacement is a generic
// "internal type" marker that still preserves the field /
// position context.
var goInternalTypeRe = regexp.MustCompile(
	`\b(?:[a-z][a-zA-Z0-9_]*\.)?(?:emit[A-Z][a-zA-Z0-9_]+|[A-Z][a-zA-Z0-9_]*Params|[A-Z][a-zA-Z0-9_]*BlockV[0-9]+|[A-Z][a-zA-Z0-9_]*BlockItem)\b`,
)

// sanitizeDecodeError strips Go-internal type names from err's
// message (R4 enforcement). Returns a NEW error whose Error()
// surface is fully sanitized; the original err is intentionally
// NOT preserved in the surface message (would re-leak Go names),
// only in errors.Unwrap for non-LLM diagnostics.
func sanitizeDecodeError(err error) error {
	if err == nil {
		return nil
	}
	original := err.Error()
	cleaned := goInternalTypeRe.ReplaceAllString(original, "the schema's internal payload type")
	// Also strip "Go struct field" prose that names a struct
	// shape the LLM has no schema for.
	cleaned = strings.ReplaceAll(cleaned, "into Go struct field", "into the schema field")
	cleaned = strings.ReplaceAll(cleaned, "Go struct field", "the schema field")
	if cleaned == original {
		return err
	}
	return &sanitizedError{surface: cleaned, inner: err}
}

// sanitizedError surfaces only the cleaned message via Error();
// the original err is reachable via errors.Unwrap for non-LLM
// callers (logging, debugging) but NEVER leaks to the LLM.
type sanitizedError struct {
	surface string
	inner   error
}

func (e *sanitizedError) Error() string { return e.surface }
func (e *sanitizedError) Unwrap() error { return e.inner }
