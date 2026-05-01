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
	ShapeText string
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

// Violation, ViolationKind, and SuspectedRoot definitions moved to
// internal/types/violation.go in Session 11 F1 so EvidenceClosure
// can embed a []Violation ledger without creating a circular import
// (contract → types → contract). The aliases + re-exported const
// values below keep every existing contract.Violation / contract.ViolShape
// caller compiling unchanged.

// ViolationKind is an alias for types.ViolationKind.
type ViolationKind = types.ViolationKind

// SuspectedRoot is an alias for types.SuspectedRoot.
type SuspectedRoot = types.SuspectedRoot

// Violation is an alias for types.Violation.
type Violation = types.Violation

// ViolationKind constants re-exported from the types package. The
// compiler resolves both names to the same typed string so comparison,
// switches, and map keys work identically to the pre-move code.
const (
	ViolShape       = types.ViolShape
	ViolCitation    = types.ViolCitation
	ViolMustInclude = types.ViolMustInclude
	ViolMustExclude = types.ViolMustExclude
	ViolAcceptance  = types.ViolAcceptance

	// ViolSuccessCriterion marks a finalize TaskNode.SuccessCriteria
	// failure that was merged into the Result by the orchestrator
	// after the contract.Check returned. Merging lets the existing
	// retry path (requeue + pendingViolation injection + retry
	// budget) treat SuccessCriteria failures uniformly with
	// contract.Check violations, replacing the pre-existing behaviour
	// where a failing success criterion only produced a log line and
	// the pipeline silently accepted the answer.
	ViolSuccessCriterion = types.ViolSuccessCriterion

	// Session 11 F1 kinds (see types/violation.go for definitions).
	ViolGhostAnchor          = types.ViolGhostAnchor
	ViolChainDemoted         = types.ViolChainDemoted
	ViolSelfRefLiteral       = types.ViolSelfRefLiteral
	ViolPreCompleteDowngrade = types.ViolPreCompleteDowngrade
	ViolLiteralFormFailed    = types.ViolLiteralFormFailed
	ViolShapeSwap            = types.ViolShapeSwap

	// Commit 53 P2/P4 — read-mode answer-coherence violations.
	ViolShapeIntentMismatch   = types.ViolShapeIntentMismatch
	ViolSubTopicCountMismatch = types.ViolSubTopicCountMismatch
	ViolDiagramIdentifier     = types.ViolDiagramIdentifier
)

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
	// Absence answers legitimately do not match shape heuristics —
	// an honest "0 files match" has no bullets for list_of_symbols,
	// no numbered steps for step_list, no yes/no for boolean (it
	// might be explained in prose). The whole point is that the
	// cited thing does not exist, so the shape-specific formatting
	// rule does not apply. The IsAbsence flag is only set upstream
	// when the LLM ran real investigation tools, so this waiver
	// cannot rescue a lazy "didn't look and declared zero" run.
	if draft.IsAbsence {
		return nil
	}
	text := draft.Text
	if shapeText := strings.TrimSpace(draft.ShapeText); shapeText != "" {
		text = shapeText
	}
	// shapeRoot is the common SuspectedRoot template for every
	// checkShape violation: the finalizer emitted a shape the
	// answer contract does not accept. F2 aggregates these events
	// per-answer_shape so the F3 patcher can decide whether to
	// reconcile (e.g. config_value → value based on cue match).
	shapeRoot := SuspectedRoot{
		IRField:    "answer_shape",
		Reason:     fmt.Sprintf("finalizer output violates contract shape=%s", c.RequiredAnswerShape),
		Confidence: 0.80,
	}
	switch c.RequiredAnswerShape {
	case types.ShapeBoolean:
		lower := strings.ToLower(strings.TrimSpace(text))
		// Accept yes/no as either the full answer or a leading token.
		if !(strings.HasPrefix(lower, "yes") || strings.HasPrefix(lower, "no") ||
			strings.HasPrefix(lower, "是") || strings.HasPrefix(lower, "否")) {
			return []Violation{{Kind: ViolShape, Detail: "boolean answer must start with yes/no", SuspectedRoot: shapeRoot}}
		}
	case types.ShapeValue:
		// A value answer must be short and non-empty. "Short" is a
		// rough heuristic: ≤ 200 chars. Longer indicates the model
		// wrote an explanation instead of returning the value.
		if len(strings.TrimSpace(text)) == 0 {
			return []Violation{{Kind: ViolShape, Detail: "value answer must not be empty", SuspectedRoot: shapeRoot}}
		}
		if len(text) > shapeValueMaxLen {
			return []Violation{{Kind: ViolShape,
				Detail:        fmt.Sprintf("value answer too long (%d chars) — expected a literal", len(text)),
				SuspectedRoot: shapeRoot}}
		}
	case types.ShapeListOfSymbols:
		// Require at least one line that looks like a bullet or
		// symbol reference. We accept either explicit "-"/"*" bullets,
		// numbered items, or backtick-fenced identifiers.
		if !hasSymbolListShape(text) {
			return []Violation{{Kind: ViolShape, Detail: "list_of_symbols answer must contain bulleted or fenced symbol entries", SuspectedRoot: shapeRoot}}
		}
	case types.ShapeStepList:
		if !hasNumberedSteps(text) {
			return []Violation{{Kind: ViolShape, Detail: "step_list answer must contain numbered steps", SuspectedRoot: shapeRoot}}
		}
	case types.ShapeConfigValue:
		if !strings.Contains(text, "=") && !strings.Contains(text, ":") && !strings.Contains(text, " is ") {
			return []Violation{{Kind: ViolShape, Detail: "config_value answer must express a key=value or key: value pair", SuspectedRoot: shapeRoot}}
		}
	case types.ShapeExplanation:
		if len(strings.TrimSpace(text)) < shapeExplanationMinLen {
			return []Violation{{Kind: ViolShape, Detail: "explanation answer too short to be meaningful", SuspectedRoot: shapeRoot}}
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
				SuspectedRoot: SuspectedRoot{
					IRField:    "CitationReq",
					Reason:     "finalizer produced zero citations though contract requires ≥N",
					Confidence: 0.75,
				},
			}}
		}
	}
	// citGranRoot covers granularity failures (missing file, missing
	// line). SuspectedRoot points at CitationReq because granularity
	// is a contract setting; F2 aggregates these alongside count-based
	// citation failures.
	citGranRoot := SuspectedRoot{
		IRField:    "CitationReq",
		Reason:     "citation granularity does not match contract requirement",
		Confidence: 0.70,
	}
	for _, cit := range draft.Citations {
		switch req.Granularity {
		case "file":
			if strings.TrimSpace(cit.File) == "" {
				return []Violation{{Kind: ViolCitation, Detail: "citation missing file", SuspectedRoot: citGranRoot}}
			}
		case "file_line":
			if strings.TrimSpace(cit.File) == "" || cit.Line <= 0 {
				return []Violation{{Kind: ViolCitation, Detail: fmt.Sprintf("citation %q missing line number", cit.File), SuspectedRoot: citGranRoot}}
			}
		case "file_line_range":
			if strings.TrimSpace(cit.File) == "" || (cit.Line <= 0 && len(cit.Lines) == 0) {
				return []Violation{{Kind: ViolCitation, Detail: fmt.Sprintf("citation %q missing line range", cit.File), SuspectedRoot: citGranRoot}}
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
	// acceptanceRoot covers contains_symbol / regex_match / invalid
	// regex / unknown kind paths. SuspectedRoot points at
	// AcceptanceTests because those tests are themselves an IR
	// declaration — mis-authored criteria are the primary
	// reconciliation target.
	acceptanceRoot := SuspectedRoot{
		IRField:    "AcceptanceTests",
		Reason:     "finalizer answer does not satisfy acceptance criterion",
		Confidence: 0.65,
	}
	for _, a := range c.AcceptanceTests {
		switch a.Kind {
		case types.CritContainsSymbol:
			if !containsSymbol(draft.Text, a.Expr) {
				out = append(out, Violation{Kind: ViolAcceptance,
					Detail:        fmt.Sprintf("acceptance contains_symbol %q failed", a.Expr),
					SuspectedRoot: acceptanceRoot})
			}
		case types.CritRegexMatch:
			re, err := regexp.Compile(a.Expr)
			if err != nil {
				out = append(out, Violation{Kind: ViolAcceptance,
					Detail:        fmt.Sprintf("invalid regex %q: %v", a.Expr, err),
					SuspectedRoot: acceptanceRoot})
				continue
			}
			if !re.MatchString(draft.Text) {
				out = append(out, Violation{Kind: ViolAcceptance,
					Detail:        fmt.Sprintf("acceptance regex %q did not match", a.Expr),
					SuspectedRoot: acceptanceRoot})
			}
		case types.CritCitationCountGE:
			// Absence answers ("0 Python files", "no handlers do X")
			// have nothing to cite — the whole point is that the
			// cited thing does not exist. Mirror the carve-out that
			// checkCitations already applies to CitationReq and the
			// orchestrator's SC merge already applies to the finalize
			// TaskNode's SuccessCriteria, so all three
			// citation-threshold paths agree.
			if draft.IsAbsence {
				continue
			}
			n, err := strconv.Atoi(strings.TrimSpace(a.Expr))
			if err != nil {
				out = append(out, Violation{Kind: ViolAcceptance,
					Detail:        fmt.Sprintf("citation_count_ge expects integer, got %q", a.Expr),
					SuspectedRoot: acceptanceRoot})
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
					Detail: fmt.Sprintf("only %d citations, need ≥%d", len(draft.Citations), n),
					SuspectedRoot: SuspectedRoot{
						IRField:    "CitationReq",
						Reason:     "zero citations vs acceptance floor — answer is ungrounded",
						Confidence: 0.80,
					}})
			}
		default:
			out = append(out, Violation{Kind: ViolAcceptance,
				Detail:        fmt.Sprintf("unknown acceptance test kind %q (expr=%q)", a.Kind, a.Expr),
				SuspectedRoot: acceptanceRoot})
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
