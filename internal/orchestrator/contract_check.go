package orchestrator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// contract_check.go is the orchestrator-side hook that runs the
// Analyzer-v3 AnswerContract checker after the finalizer produces its
// draft answer and before runTaskGraph marks the finalize node done.
//
// The checker itself lives in internal/analysis/contract; this file
// only handles the orchestrator-facing glue: extracting citations
// from the rendered answer text and translating contract.Result into
// the orchestrator's backtrack signal.
//
// P1.3 design notes:
//
//   - Citations are extracted by a structural file:line regex, NOT
//     a curated list of valid citations. A "valid citation" is any
//     `path/to/file.ext:NNN` token in the answer body. This is a
//     universal Go/Python/JS code-locator format with no repo-
//     specific hardcoding, satisfying the no-stopword guardrail.
//
//   - The checker's own `containsSymbol` helper is a substring match;
//     for must_include / must_exclude, that's intentionally permissive
//     — we want "TaskList" to match "TaskListSize" rather than reject
//     valid superstrings.
//
//   - When AnswerContract is empty (every field zero) the checker
//     short-circuits to Passed=true. So a nil-IR or unwired-shape
//     run never sees a spurious violation.

// runContractCheck runs the AnswerContract validator over a finalizer
// StageOutput and returns the violations slice. nil means no
// violations OR no contract was declared. The caller is expected to
// inspect both the returned slice length and the IR's contract
// presence to decide what to do.
//
// Returns the typed contract.Result so callers can render a
// per-violation diagnostic for the explorer's retry hint.
func runContractCheck(out *agent.StageOutput, c types.AnswerContract) contract.Result {
	if out == nil {
		return contract.Result{Passed: true}
	}
	draft := contract.Answer{
		Text:      out.FinalAnswer,
		Citations: extractCitationsFromAnswer(out.FinalAnswer),
	}
	return contract.Check(draft, c)
}

// citationRegex matches `path/to/file.ext:NNN` style references. The
// path must contain at least one `/` or end in a typical source
// extension; the line is a positive integer. Permissive on the path
// shape so it catches subdir hits like `internal/agent/explorer.go:42`
// without matching prose like "step 3:" or "10:30".
//
// The pattern is intentionally structural (path char class + dot +
// extension + colon + digits), not a curated list of file extensions
// known to this repo. A new repo with .lua / .php / .swift sources
// gets the same recall without code changes.
var citationRegex = regexp.MustCompile(
	`(?:^|[\s\(\[\<` + "`" + `])` + // word boundary leader
		`([A-Za-z0-9_\-./]*[A-Za-z0-9_\-]+\.[A-Za-z0-9]{1,8})` + // path with extension
		`:(\d{1,6})` + // : line
		`(?:-(\d{1,6}))?`, // optional -end for ranges
)

// extractCitationsFromAnswer pulls every file:line[-end] reference
// out of the answer body and returns them as a contract.Citation
// slice. Duplicates (same file, same line) are de-duplicated so a
// single reference repeated three times in the prose still counts
// as one citation (the contract checker measures distinct anchors).
func extractCitationsFromAnswer(text string) []contract.Citation {
	if text == "" {
		return nil
	}
	matches := citationRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]contract.Citation, 0, len(matches))
	for _, m := range matches {
		file := m[1]
		// Reject obvious prose hits that the regex's permissive
		// path-char class still lets through. The structural cues
		// are: must contain at least one `/` (otherwise it's a bare
		// "go.mod:5" or "README.md:1" — still a valid citation, so
		// we let those through) AND the matched line must parse as
		// a non-zero positive int (already enforced by \d+).
		if file == "" {
			continue
		}
		line, err := strconv.Atoi(m[2])
		if err != nil || line == 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d", file, line)
		if seen[key] {
			continue
		}
		seen[key] = true
		c := contract.Citation{File: file, Line: line}
		if len(m) > 3 && m[3] != "" {
			if end, err := strconv.Atoi(m[3]); err == nil && end >= line {
				c.Lines = []int{line, end}
			}
		}
		out = append(out, c)
	}
	return out
}

// renderViolations turns a contract.Result into a single short
// diagnostic string suitable for injecting into the explorer's
// retry hint. One sentence per violation, separated by `; `, so
// the Retry Directive section stays compact.
func renderViolations(res contract.Result) string {
	if res.Passed || len(res.Violations) == 0 {
		return ""
	}
	parts := make([]string, 0, len(res.Violations))
	for _, v := range res.Violations {
		s := v.Detail
		if v.Repair != "" {
			s += " (" + v.Repair + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// finalizerCitationPoolSize returns the authoritative citation count
// to feed into criterion.Env.DraftCitations. The count is sourced in
// this priority order:
//
//  1. Mutable.AnswerDocument().Citations — populated by
//     emit_answer_document.Execute after grounding + remap. This is
//     the exact pool the renderer consults.
//  2. extractCitationsFromAnswer(out.FinalAnswer) — legacy text-regex
//     fallback when the AnswerDocument was never set (test harnesses
//     that route directly through StageOutput.FinalAnswer).
//
// The regex fallback under-counts on list_of_symbols / step_list
// because those shapes inline citations into per-row renders and do
// not emit the pool as a bulleted list. Using the pool count fixes
// the "4 citations but orchestrator says 1" class of bugs.
func finalizerCitationPoolSize(mut *types.MutableState, out *agent.StageOutput) int {
	if mut != nil {
		if doc := mut.AnswerDocument(); doc != nil && len(doc.Citations) > 0 {
			return len(doc.Citations)
		}
	}
	if out != nil {
		return len(extractCitationsFromAnswer(out.FinalAnswer))
	}
	return 0
}

// appendViolationsToAnswer prepends a single visible warning line to
// the final answer text when the contract checker has exhausted its
// retry budget. The original answer is preserved beneath the warning
// so no information is lost — same fail-loud pattern P0.2 uses for
// shape-validator exhaustion. See feedback_honesty_over_cleverness.md.
func appendViolationsToAnswer(originalAnswer string, res contract.Result) string {
	if res.Passed || len(res.Violations) == 0 {
		return originalAnswer
	}
	var b strings.Builder
	b.WriteString("⚠️ answer-contract validation exhausted: ")
	b.WriteString(renderViolations(res))
	b.WriteString("\n\n")
	b.WriteString(originalAnswer)
	return b.String()
}
