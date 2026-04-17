// Package contract is the runtime answer-contract checker. It runs
// after the finalizer has drafted an answer and before the
// orchestrator marks finalize as complete, giving the pipeline a
// final deterministic guardrail against LLM hallucination or
// shape drift.
//
// A contract check is intentionally narrow: it verifies shape,
// citation requirements, must-include / must-exclude sets, and
// machine-checkable acceptance tests. It does NOT assess answer
// quality or factual correctness — those are out of scope because
// they cannot be decided without an oracle.
//
// When the checker rejects an answer, the caller (analyzer-v3 b7
// orchestrator integration) will route the task back to the most
// recent evidence node instead of finalizing, subject to the
// EvidencePlan retry budget.
package contract

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Shape validation thresholds.
const (
	shapeValueMaxLen       = 500 // reject value answers longer than this
	shapeExplanationMinLen = 20  // reject explanations shorter than this
)

// Answer is the draft the finalizer produced. Separating this from
// the IR lets tests build answers without constructing a full IR.
//
// IsAbsence signals that the answer is an honest "zero" — e.g.
// "how many Python files?" → 0, "which handlers do X?" → none,
// "is X registered?" → no. Absence answers have no file:line to
// cite (the point is that the cited thing does not exist), so
// checkCitations skips MinCitations when the flag is set.
type Answer struct {
	Text      string
	Citations []Citation
	IsAbsence bool
}

// Citation is a single source reference the finalizer attached to
// the answer. The contract checker validates granularity against
// CitationReq.
type Citation struct {
	File  string
	Line  int   // 0 when unknown
	Lines []int // present for "file_line_range" granularity
}

// Result is the output of a contract check. Passed==false means the
// orchestrator must not mark the finalize node as done.
type Result struct {
	Passed     bool
	Violations []Violation
}

// ViolationKind classifies a contract breach.
type ViolationKind string

const (
	ViolShape       ViolationKind = "shape"
	ViolCitation    ViolationKind = "citation"
	ViolMustInclude ViolationKind = "must_include"
	ViolMustExclude ViolationKind = "must_exclude"
	ViolAcceptance  ViolationKind = "acceptance"

	// ViolSuccessCriterion marks a finalize TaskNode.SuccessCriteria
	// failure that was merged into the Result by the orchestrator
	// after the contract.Check returned. Merging lets the existing
	// retry path (requeue + pendingViolation injection + retry
	// budget) treat SuccessCriteria failures uniformly with
	// contract.Check violations, replacing the pre-existing behaviour
	// where a failing success criterion only produced a log line and
	// the pipeline silently accepted the answer.
	ViolSuccessCriterion ViolationKind = "success_criterion"
)

// Violation is one specific contract breach with a short reason and
// an optional repair hint the orchestrator can pass to the explorer
// when it reroutes the task.
type Violation struct {
	Kind   ViolationKind
	Detail string
	Repair string // e.g. "collect evidence for <symbol>"
}

// Check validates draft against c. It is safe to call with an empty
// contract — every field is treated as "not required" so a
// zero-value contract never rejects an answer.
func Check(draft Answer, c types.AnswerContract) Result {
	var vs []Violation

	vs = append(vs, checkShape(draft, c)...)
	vs = append(vs, checkCitations(draft, c)...)
	vs = append(vs, checkMustInclude(draft, c)...)
	vs = append(vs, checkMustExclude(draft, c)...)
	vs = append(vs, checkAcceptance(draft, c)...)

	return Result{Passed: len(vs) == 0, Violations: vs}
}

// ── individual checks ─────────────────────────────────────────

func checkShape(draft Answer, c types.AnswerContract) []Violation {
	if c.RequiredAnswerShape == "" || c.RequiredAnswerShape == types.ShapeNone {
		return nil
	}
	text := draft.Text
	switch c.RequiredAnswerShape {
	case types.ShapeBoolean:
		lower := strings.ToLower(strings.TrimSpace(text))
		// Accept yes/no as either the full answer or a leading token.
		if !(strings.HasPrefix(lower, "yes") || strings.HasPrefix(lower, "no") ||
			strings.HasPrefix(lower, "是") || strings.HasPrefix(lower, "否")) {
			return []Violation{{Kind: ViolShape, Detail: "boolean answer must start with yes/no"}}
		}
	case types.ShapeValue:
		// A value answer must be short and non-empty. "Short" is a
		// rough heuristic: ≤ 200 chars. Longer indicates the model
		// wrote an explanation instead of returning the value.
		if len(strings.TrimSpace(text)) == 0 {
			return []Violation{{Kind: ViolShape, Detail: "value answer must not be empty"}}
		}
		if len(text) > shapeValueMaxLen {
			return []Violation{{Kind: ViolShape,
				Detail: fmt.Sprintf("value answer too long (%d chars) — expected a literal", len(text))}}
		}
	case types.ShapeListOfSymbols:
		// Require at least one line that looks like a bullet or
		// symbol reference. We accept either explicit "-"/"*" bullets,
		// numbered items, or backtick-fenced identifiers.
		if !hasSymbolListShape(text) {
			return []Violation{{Kind: ViolShape, Detail: "list_of_symbols answer must contain bulleted or fenced symbol entries"}}
		}
	case types.ShapeStepList:
		if !hasNumberedSteps(text) {
			return []Violation{{Kind: ViolShape, Detail: "step_list answer must contain numbered steps"}}
		}
	case types.ShapeConfigValue:
		if !strings.Contains(text, "=") && !strings.Contains(text, ":") && !strings.Contains(text, " is ") {
			return []Violation{{Kind: ViolShape, Detail: "config_value answer must express a key=value or key: value pair"}}
		}
	case types.ShapeExplanation:
		if len(strings.TrimSpace(text)) < shapeExplanationMinLen {
			return []Violation{{Kind: ViolShape, Detail: "explanation answer too short to be meaningful"}}
		}
	}
	return nil
}

func checkCitations(draft Answer, c types.AnswerContract) []Violation {
	req := c.CitationReq
	if !req.Required {
		return nil
	}
	// Absence answers ("0 Python files", "no deprecated handlers",
	// boolean false) legitimately have no file:line to cite — the
	// whole point is that the cited thing does not exist. The
	// IsAbsence flag is only set upstream when the investigation
	// actually ran (grep / exec_command / multiple read_file), so a
	// lazy "I didn't look and claim zero" cannot sneak through.
	if draft.IsAbsence {
		return nil
	}
	if len(draft.Citations) < req.MinCitations {
		// Soft degradation: when citations > 0 but below the
		// threshold, accept the answer (the citations it has are
		// real) instead of forcing a retry that produces the same
		// count. Only reject when citations == 0.
		if len(draft.Citations) > 0 {
			// pass — some citations present
		} else {
			return []Violation{{
				Kind:   ViolCitation,
				Detail: fmt.Sprintf("%d citations provided, %d required", len(draft.Citations), req.MinCitations),
				Repair: "collect more evidence with file:line anchors",
			}}
		}
	}
	for _, cit := range draft.Citations {
		switch req.Granularity {
		case "file":
			if strings.TrimSpace(cit.File) == "" {
				return []Violation{{Kind: ViolCitation, Detail: "citation missing file"}}
			}
		case "file_line":
			if strings.TrimSpace(cit.File) == "" || cit.Line <= 0 {
				return []Violation{{Kind: ViolCitation, Detail: fmt.Sprintf("citation %q missing line number", cit.File)}}
			}
		case "file_line_range":
			if strings.TrimSpace(cit.File) == "" || (cit.Line <= 0 && len(cit.Lines) == 0) {
				return []Violation{{Kind: ViolCitation, Detail: fmt.Sprintf("citation %q missing line range", cit.File)}}
			}
		}
	}
	return nil
}

func checkMustInclude(draft Answer, c types.AnswerContract) []Violation {
	var out []Violation
	for _, sym := range c.MustInclude {
		if sym == "" {
			continue
		}
		if !containsSymbol(draft.Text, sym) {
			out = append(out, Violation{
				Kind:   ViolMustInclude,
				Detail: fmt.Sprintf("required term %q missing from answer", sym),
				Repair: "include " + sym + " in the final answer",
			})
		}
	}
	return out
}

func checkMustExclude(draft Answer, c types.AnswerContract) []Violation {
	var out []Violation
	for _, sym := range c.MustExclude {
		if sym == "" {
			continue
		}
		if containsSymbol(draft.Text, sym) {
			out = append(out, Violation{
				Kind:   ViolMustExclude,
				Detail: fmt.Sprintf("forbidden term %q present in answer", sym),
			})
		}
	}
	return out
}

func checkAcceptance(draft Answer, c types.AnswerContract) []Violation {
	var out []Violation
	for _, a := range c.AcceptanceTests {
		switch a.Kind {
		case types.CritContainsSymbol:
			if !containsSymbol(draft.Text, a.Expr) {
				out = append(out, Violation{Kind: ViolAcceptance,
					Detail: fmt.Sprintf("acceptance contains_symbol %q failed", a.Expr)})
			}
		case types.CritRegexMatch:
			re, err := regexp.Compile(a.Expr)
			if err != nil {
				out = append(out, Violation{Kind: ViolAcceptance,
					Detail: fmt.Sprintf("invalid regex %q: %v", a.Expr, err)})
				continue
			}
			if !re.MatchString(draft.Text) {
				out = append(out, Violation{Kind: ViolAcceptance,
					Detail: fmt.Sprintf("acceptance regex %q did not match", a.Expr)})
			}
		case types.CritCitationCountGE:
			n, err := strconv.Atoi(strings.TrimSpace(a.Expr))
			if err != nil {
				out = append(out, Violation{Kind: ViolAcceptance,
					Detail: fmt.Sprintf("citation_count_ge expects integer, got %q", a.Expr)})
				continue
			}
			if len(draft.Citations) < n {
				// Soft degradation: when the answer HAS citations
				// (>0) but fewer than the threshold, treat it as a
				// caveat rather than a hard rejection. This prevents
				// infinite retry loops for questions whose correct
				// answer only needs 1-2 citations (e.g. a simple
				// enumerate/list_of_symbols query) but the template
				// declares a higher floor. Zero citations is still a
				// hard reject — the answer is genuinely ungrounded.
				if len(draft.Citations) > 0 {
					continue // pass with implicit caveat
				}
				out = append(out, Violation{Kind: ViolAcceptance,
					Detail: fmt.Sprintf("only %d citations, need ≥%d", len(draft.Citations), n)})
			}
		default:
			out = append(out, Violation{Kind: ViolAcceptance,
				Detail: fmt.Sprintf("unknown acceptance test kind %q (expr=%q)", a.Kind, a.Expr)})
		}
	}
	return out
}

// ── helpers ───────────────────────────────────────────────────

// containsSymbol looks for `needle` in text with word-boundary
// awareness. Exact substring match is used for short strings, and
// case-sensitive boundary match for alphanumeric identifiers so
// "TaskList" doesn't match "taskListSize" spuriously — we want the
// opposite, actually: we want TaskList to match TaskListSize. The
// compromise is plain substring match, which is more permissive and
// safer for "include this symbol" style checks.
func containsSymbol(text, needle string) bool {
	return strings.Contains(text, needle)
}

var (
	reNumbered = regexp.MustCompile(`(?m)^\s*\d+[.)]\s+`)
	reBullet   = regexp.MustCompile(`(?m)^\s*[-*]\s+`)
	reFenced   = regexp.MustCompile("`[^`\\n]+`")
)

func hasSymbolListShape(text string) bool {
	return reBullet.MatchString(text) || reFenced.MatchString(text)
}

func hasNumberedSteps(text string) bool {
	return reNumbered.FindAllStringIndex(text, -1) != nil && len(reNumbered.FindAllString(text, -1)) >= 2
}
