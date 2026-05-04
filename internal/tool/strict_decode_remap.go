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
//     artefact). Rewrite to "<field>[] must be a native JSON
//     array, not a quoted string. Re-emit as [...], not as
//     "[...]"".
//   - Otherwise the message is sanitized (R4 — Go internal type
//     names stripped) and returned as-is.
//
// Callers may pass nil/empty hints — in that case the function
// still sanitizes the error message (R4 always applies).
func RemapStrictDecodeError(err error, hints []MisplacedFieldHint) error {
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
	if rewritten, matched := remapCannotUnmarshalStringIntoArray(err); matched {
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
// name (e.g. `blocks`) so the rewritten message can use it.
//
// The pattern matches both top-level fields and nested-struct
// references. We deliberately do NOT capture the Go type names <T>
// or <U> — they are R4-banned implementation jargon.
var cannotUnmarshalStringRe = regexp.MustCompile(
	`json: cannot unmarshal string into Go struct field [A-Za-z_][A-Za-z0-9_]*\.([A-Za-z_][A-Za-z0-9_]*) of type \[?\]?`,
)

// remapCannotUnmarshalStringIntoArray detects "blocks-as-string"
// streaming artefacts (LLM JSON.stringify'd an array before
// putting it in the tool call) and rewrites the error to give
// LLM-actionable guidance. Returns (wrapped err, true) on match;
// (nil, false) otherwise.
func remapCannotUnmarshalStringIntoArray(err error) (error, bool) {
	if err == nil {
		return nil, false
	}
	for cur := err; cur != nil; cur = errors.Unwrap(cur) {
		m := cannotUnmarshalStringRe.FindStringSubmatch(cur.Error())
		if len(m) != 2 {
			continue
		}
		field := m[1] // e.g. "blocks"
		return fmt.Errorf(
			"the %q field must be a native JSON array of objects (e.g. %q), not a JSON-encoded string. The streaming layer wrapped your %q value in quotes — re-emit %q as a native array (no surrounding quotes, no escaped inner quotes)",
			field, field+`: [{...}, {...}]`, field, field), true
	}
	return nil, false
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
