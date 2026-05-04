package tool

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
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
// path; mismatch passes the original error verbatim.
//
// Red lines:
//   - Hints are typed exact-match (Field name + ContainerNames),
//     never fuzzy / similarity (R3 precise signal).
//   - The remap NEVER changes the error VALUE (it stays an error,
//     not a silent recovery) — only the message text.
//   - Hint table per emit tool, owned where it's consumed; no
//     global registry.

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

// RemapStrictDecodeError inspects err for a `json: unknown field
// "X"` pattern; if X matches a hint AND the surrounding context
// suggests one of ContainerNames, returns a wrapped error whose
// message names the correct paths. Otherwise returns err unchanged.
//
// Note: Go's json package does not expose the path of the
// offending field — only the field name. So the heuristic for
// "context suggests wrong container" is:
//   - hint matches by field name alone (LLM-frequent misplacements
//     are name-driven; the wrong-container assertion in
//     ContainerNames is documentation for operators, not a runtime
//     filter)
//
// Callers may pass nil/empty hints — in that case the function is
// a no-op pass-through.
func RemapStrictDecodeError(err error, hints []MisplacedFieldHint) error {
	if err == nil || len(hints) == 0 {
		return err
	}
	field := extractUnknownFieldName(err)
	if field == "" {
		return err
	}
	for _, h := range hints {
		if h.Field != field {
			continue
		}
		paths := strings.Join(h.CorrectPaths, " / ")
		containers := strings.Join(h.ContainerNames, " / ")
		return fmt.Errorf(
			"%w — field %q exists in the schema at %s, NOT inside %s; relocate the value (do not rename or remove it)",
			err, field, paths, containers)
	}
	return err
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
