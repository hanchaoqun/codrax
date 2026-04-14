package agent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// P0.2 — runtime validators for step_list / value / boolean /
// config_value answer shapes. Each validator returns an empty string
// on pass and a human-readable violation message on fail. The
// message is fed into the finalizer correction prompt on retry and
// into the fail-loud honesty note after retries are exhausted.
//
// These validators form part of filter F11 in docs/filtering-pipeline.md
// (the "answer-shape validator family" — alongside the legacy
// outOfListSymbols for list_of_symbols). F11 runs strictly last in the
// pipeline; see §3 invariant 8.
//
// Design rules (from docs/architecture-root-cause-remediation.md
// §P0.2 and the session's pre-design audit):
//
//  1. Predicates are STRUCTURAL — they key off EvidenceItem kinds and
//     regex shapes, never hardcoded word lists.
//  2. Each validator has a SKIP GUARD for the "no upstream data"
//     case: when the evidence family a validator keys off is empty,
//     it passes rather than fails. This avoids over-triggering when
//     the deterministic layers never produced the feedstock, and
//     lets the legacy prompt-level constraint stay in charge.
//  3. Retry cap exhaustion does NOT silent-accept (unlike L0-2 S3
//     which does) — the caller (ParseOutput) appends an honest
//     "answer-shape validation exhausted" note. See the fail-loud
//     red line in §P0.2.

// stepLeaderRe matches the leading token of a numbered or bulleted
// step. Language-independent: digits+dot, or bullet marker. This is
// the no-bait structural count — LLM can't game it by producing a
// prose paragraph (no leader, no count).
var stepLeaderRe = regexp.MustCompile(`(?m)^\s*(?:\d+\.|[-*•])\s`)

// fileLineCiteRe matches a file:line citation like `foo.go:42`. The
// extension is restricted to 2-6 ASCII letters so we don't fire on
// URL-looking tokens.
var fileLineCiteRe = regexp.MustCompile(`[\w/.\-]+\.[A-Za-z]{2,6}:\d+`)

// validateStepList returns a violation when the answer's enumerated
// step count is below the number of distinct [CONDITIONAL] evidence
// branches. Prose collapse — the 2026-04-14 Pattern 1 fake-green —
// lands here.
func validateStepList(answer string, items []types.EvidenceItem) string {
	branches := 0
	for _, ev := range items {
		if ev.Kind == types.EvidenceConditional {
			branches++
		}
	}
	if branches == 0 {
		return "" // skip guard: no conditional feedstock → vacuously true
	}
	steps := len(stepLeaderRe.FindAllStringIndex(answer, -1))
	if steps >= branches {
		return ""
	}
	return fmt.Sprintf(
		"The evidence contains %d distinct [CONDITIONAL] branches but your answer has only %d enumerated step(s). Each branch must be its own numbered or bulleted step — do not collapse them into a narrative.",
		branches, steps,
	)
}

// validateValue returns a violation when the answer does not contain
// any concrete literal declared in the EvidenceConcrete items. The
// comparison is substring-based on the Object field, so a literal
// like `"explorer"` in evidence must appear verbatim in the answer.
func validateValue(answer string, items []types.EvidenceItem) string {
	var literals []string
	for _, ev := range items {
		if ev.Kind == types.EvidenceConcrete && strings.TrimSpace(ev.Object) != "" {
			literals = append(literals, ev.Object)
		}
	}
	if len(literals) == 0 {
		return "" // skip guard
	}
	for _, lit := range literals {
		if strings.Contains(answer, lit) {
			return ""
		}
	}
	return fmt.Sprintf(
		"The answer must state one concrete value from the evidence verbatim. Candidates from Evidence Items: %s. None of these appeared in your answer.",
		strings.Join(literals, ", "),
	)
}

// booleanPrefixRe matches the bilingual (Q1 decision) set of accepted
// leading boolean tokens. The answer must open with one of them,
// optionally preceded by whitespace or a markdown emphasis marker.
// Trailing context after the token is free.
var booleanPrefixRe = regexp.MustCompile(`^[\s*_` + "`" + `]*(?:YES|NO|是|否)\b|^[\s*_` + "`" + `]*(?:是|否)`)

// validateBoolean requires the answer to open with YES / NO / 是 / 否.
// Hedging prefixes ("It depends…", "Well, …") fail. The token must
// be leading — a buried YES deeper in the prose does not count.
func validateBoolean(answer string) string {
	trimmed := strings.TrimLeft(answer, " \t\r\n*_`")
	if trimmed == "" {
		return "A boolean answer must start with YES, NO, 是, or 否. The answer was empty."
	}
	switch {
	case strings.HasPrefix(trimmed, "YES"):
		return ""
	case strings.HasPrefix(trimmed, "NO"):
		return ""
	case strings.HasPrefix(trimmed, "是"):
		return ""
	case strings.HasPrefix(trimmed, "否"):
		return ""
	}
	return "A boolean answer must open with YES, NO, 是, or 否 followed by a short evidence citation. Do not hedge."
}

// validateConfigValue requires three structural parts in the answer:
//
//	(a) a config key mention  — taken from any EvidenceConcrete.Subject
//	(b) a resolved literal    — taken from any EvidenceConcrete.Object
//	(c) a file:line citation  — matched by fileLineCiteRe
//
// Skip guard fires when no EvidenceConcrete items exist. When a
// Source is set on an evidence item, that file path is accepted as
// the cite too, even without an explicit line number (deterministic
// extraction may not always produce a line).
func validateConfigValue(answer string, items []types.EvidenceItem) string {
	var keys, values, sources []string
	for _, ev := range items {
		if ev.Kind != types.EvidenceConcrete {
			continue
		}
		if k := strings.TrimSpace(ev.Subject); k != "" {
			keys = append(keys, k)
		}
		if v := strings.TrimSpace(ev.Object); v != "" {
			values = append(values, v)
		}
		if s := strings.TrimSpace(ev.Source); s != "" {
			sources = append(sources, s)
		}
	}
	if len(keys) == 0 && len(values) == 0 {
		return "" // skip guard
	}

	var missing []string

	if len(keys) > 0 {
		if !containsAny(answer, keys) {
			missing = append(missing, fmt.Sprintf("config key (one of: %s)", strings.Join(keys, ", ")))
		}
	}
	if len(values) > 0 {
		if !containsAny(answer, values) {
			missing = append(missing, fmt.Sprintf("resolved value literal (one of: %s)", strings.Join(values, ", ")))
		}
	}
	if !fileLineCiteRe.MatchString(answer) && !containsAny(answer, sources) {
		missing = append(missing, "file:line citation")
	}

	if len(missing) == 0 {
		return ""
	}
	return "The config_value answer must state (a) the config key, (b) the resolved value, (c) the file:line. Missing: " + strings.Join(missing, "; ") + "."
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if n == "" {
			continue
		}
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// shapeValidatorFor dispatches on the analyzer-declared answer shape
// and returns the validator result plus a flag indicating whether
// this shape is covered by P0.2 at all. When covered==false, the
// finalizer stays on the legacy one-shot path.
func shapeValidatorFor(shape string, answer string, items []types.EvidenceItem) (violation string, covered bool) {
	switch shape {
	case "step_list":
		return validateStepList(answer, items), true
	case "value":
		return validateValue(answer, items), true
	case "boolean":
		return validateBoolean(answer), true
	case "config_value":
		return validateConfigValue(answer, items), true
	}
	return "", false
}
