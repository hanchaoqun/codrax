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
// values below keep every existing contract.Violation / contract.ViolFamilyMismatch
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
	ViolFamilyMismatch       = types.ViolFamilyMismatch
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
	ViolViewSwap            = types.ViolViewSwap

	// Commit 53 P2/P4 — read-mode answer-coherence violations.
	ViolViewIntentMismatch   = types.ViolViewIntentMismatch
	ViolSubTopicCountMismatch = types.ViolSubTopicCountMismatch
	ViolDiagramIdentifier     = types.ViolDiagramIdentifier
	// Commit 55 Batch A.3 — declared-count drift.
	ViolDeclaredCountDrift = types.ViolDeclaredCountDrift
	// Commit 62 — answer prose self-contradiction.
	ViolSelfContradiction = types.ViolSelfContradiction
)

// Check validates draft against c. It is safe to call with an empty
// contract — every field is treated as "not required" so a
// zero-value contract never rejects an answer.
//
// Back-compat: this signature preserves the pre-commit-55 shape.
// New code that has a SymbolOracle handy should use CheckWithOracle
// for the stricter must_include / must_exclude / acceptance
// substring path.
func Check(draft Answer, c types.AnswerContract) Result {
	return CheckWithOracle(draft, c, nil)
}

// CheckWithOracle is the commit-55 Batch C oracle-aware variant.
// When `oracle` is non-nil, must_include / must_exclude /
// acceptance(contains_symbol) substring matches are
// SUPPLEMENTARY-validated: a substring hit is accepted only when
// the term ALSO resolves to a real Tier 1-2 symbol. This closes
// the audit MEDIUM #2 gap — pre-fix, an LLM-emitted must_include
// like "FooHandler" could pass via prose containing "FooHandlerStub"
// even when no real FooHandler symbol exists in the repo.
//
// nil oracle = legacy substring-only behaviour. The default Check
// signature wires nil so pre-commit-55 callers see no change.
//
// Permissive substring intent ("TaskList → TaskListSize") is
// preserved when the substring's TERM (TaskList) IS itself a real
// symbol; the oracle-strict path only rejects when the term is
// NOT a known symbol. So legitimate prefix-style includes still
// pass; only hallucinated includes fail.
func CheckWithOracle(draft Answer, c types.AnswerContract, oracle types.SymbolOracle) Result {
	var vs []Violation

	vs = append(vs, checkShape(draft, c)...)
	vs = append(vs, checkCitations(draft, c)...)
	vs = append(vs, checkMustIncludeOracle(draft, c, oracle)...)
	vs = append(vs, checkMustExcludeOracle(draft, c, oracle)...)
	vs = append(vs, checkAcceptanceOracle(draft, c, oracle)...)

	return Result{Passed: len(vs) == 0, Violations: vs}
}

// ── individual checks ─────────────────────────────────────────

// checkShape is retired per docs/migration/answer_shape_retirement.md.
// V1 shape-text-heuristics (boolean prefix detection, list-bullet
// regex, numbered-step detection) have been superseded by the V2
// block oracles in internal/orchestrator/contract_check_block.go,
// which read the typed AnswerDocumentV2 carrier rather than text
// heuristics. This stub is preserved so contract.CheckWithOracle
// callers compile; it never produces violations.
func checkShape(_ Answer, _ types.AnswerContract) []Violation {
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
				Kind:       ViolCitation,
				ClusterKey: types.RootClusterKey("CitationReq"),
				Detail:     fmt.Sprintf("%d citations provided, %d required", len(draft.Citations), req.MinCitations),
				Repair:     "collect more evidence with file:line anchors",
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
				return []Violation{{
					Kind:          ViolCitation,
					ClusterKey:    types.RootClusterKey("CitationReq"),
					Detail:        "citation missing file",
					SuspectedRoot: citGranRoot,
				}}
			}
		case "file_line":
			if strings.TrimSpace(cit.File) == "" || cit.Line <= 0 {
				key := types.RootClusterKey("CitationReq")
				if strings.TrimSpace(cit.File) != "" {
					key = types.IdentityClusterKey("file:"+cit.File, "CitationReq")
				}
				return []Violation{{
					Kind:          ViolCitation,
					ClusterKey:    key,
					Detail:        fmt.Sprintf("citation %q missing line number", cit.File),
					SuspectedRoot: citGranRoot,
				}}
			}
		case "file_line_range":
			if strings.TrimSpace(cit.File) == "" || (cit.Line <= 0 && len(cit.Lines) == 0) {
				key := types.RootClusterKey("CitationReq")
				if strings.TrimSpace(cit.File) != "" {
					key = types.IdentityClusterKey("file:"+cit.File, "CitationReq")
				}
				return []Violation{{
					Kind:          ViolCitation,
					ClusterKey:    key,
					Detail:        fmt.Sprintf("citation %q missing line range", cit.File),
					SuspectedRoot: citGranRoot,
				}}
			}
		}
	}
	return nil
}

func checkMustInclude(draft Answer, c types.AnswerContract) []Violation {
	return checkMustIncludeOracle(draft, c, nil)
}

// checkMustIncludeOracle is the commit-55 Batch C oracle-aware
// variant of checkMustInclude. When oracle is nil, behaviour is
// byte-identical to the pre-commit-55 substring-only path. With an
// oracle, a substring hit on a term that is NOT a real Tier 1-2
// symbol gets treated as a miss (the LLM hallucinated the include).
// Non-symbol-shaped terms (multi-word phrases, terms shorter than
// 4 chars) bypass the oracle gate so they keep substring-only
// semantics — only identifier-shaped tokens get the strict path.
func checkMustIncludeOracle(draft Answer, c types.AnswerContract, oracle types.SymbolOracle) []Violation {
	var out []Violation
	for _, sym := range c.MustInclude {
		if sym == "" {
			continue
		}
		hit := containsSymbol(draft.Text, sym)
		if hit && oracle != nil && shouldOracleGateInclude(sym) {
			if found, tier := oracle.SymbolExists(sym); !found || tier >= 3 {
				// Substring matches but the term isn't a real symbol —
				// treat as miss (LLM hallucinated the include).
				hit = false
			}
		}
		if !hit {
			out = append(out, Violation{
				Kind:       ViolMustInclude,
				ClusterKey: types.IdentityClusterKey("term:"+sym, "must_include"),
				Detail:     fmt.Sprintf("required term %q missing from answer", sym),
				Repair:     "include " + sym + " in the final answer",
			})
		}
	}
	return out
}

// shouldOracleGateInclude reports whether a must_include term is
// shaped like a code identifier (CamelCase / camelCase /
// snake_case ≥ 4 chars). Multi-word phrases, common English
// words, and short tokens bypass oracle gating to preserve the
// substring permissiveness for prose content. Mirrors the
// commit-54 P4 diagram-validator regex shape but is independently
// scoped because the contract layer doesn't depend on
// emit_answer_document.
func shouldOracleGateInclude(sym string) bool {
	if len(sym) < 4 {
		return false
	}
	for i := 0; i < len(sym); i++ {
		c := sym[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_':
			continue
		default:
			// Whitespace / punctuation = phrase, not identifier.
			return false
		}
	}
	return true
}

func checkMustExclude(draft Answer, c types.AnswerContract) []Violation {
	return checkMustExcludeOracle(draft, c, nil)
}

// checkMustExcludeOracle: with oracle, substring hits on terms
// that are NOT real Tier 1-2 symbols are NOT counted as forbidden
// presences (the term is gibberish, not the prohibited concept).
// Without oracle, behaviour is byte-identical to pre-commit-55.
// Tightens the gate's precision: hallucinated forbidden terms
// don't raise spurious violations.
func checkMustExcludeOracle(draft Answer, c types.AnswerContract, oracle types.SymbolOracle) []Violation {
	var out []Violation
	for _, sym := range c.MustExclude {
		if sym == "" {
			continue
		}
		hit := containsSymbol(draft.Text, sym)
		if hit && oracle != nil && shouldOracleGateInclude(sym) {
			if found, tier := oracle.SymbolExists(sym); !found || tier >= 3 {
				hit = false
			}
		}
		if hit {
			out = append(out, Violation{
				Kind:       ViolMustExclude,
				ClusterKey: types.IdentityClusterKey("term:"+sym, "must_exclude"),
				Detail:     fmt.Sprintf("forbidden term %q present in answer", sym),
			})
		}
	}
	return out
}

func checkAcceptance(draft Answer, c types.AnswerContract) []Violation {
	return checkAcceptanceOracle(draft, c, nil)
}

// checkAcceptanceOracle: oracle gates the contains_symbol
// acceptance check identically to must_include — a substring hit
// on a hallucinated term doesn't satisfy the acceptance.
func checkAcceptanceOracle(draft Answer, c types.AnswerContract, oracle types.SymbolOracle) []Violation {
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
			hit := containsSymbol(draft.Text, a.Expr)
			if hit && oracle != nil && shouldOracleGateInclude(a.Expr) {
				if found, tier := oracle.SymbolExists(a.Expr); !found || tier >= 3 {
					hit = false
				}
			}
			if !hit {
				out = append(out, Violation{Kind: ViolAcceptance,
					ClusterKey:    types.IdentityClusterKey("acceptance:contains_symbol", "AcceptanceTests"),
					Detail:        fmt.Sprintf("acceptance contains_symbol %q failed", a.Expr),
					SuspectedRoot: acceptanceRoot})
			}
		case types.CritRegexMatch:
			re, err := regexp.Compile(a.Expr)
			if err != nil {
				out = append(out, Violation{Kind: ViolAcceptance,
					ClusterKey:    types.IdentityClusterKey("acceptance:regex", "AcceptanceTests"),
					Detail:        fmt.Sprintf("invalid regex %q: %v", a.Expr, err),
					SuspectedRoot: acceptanceRoot})
				continue
			}
			if !re.MatchString(draft.Text) {
				out = append(out, Violation{Kind: ViolAcceptance,
					ClusterKey:    types.IdentityClusterKey("acceptance:regex", "AcceptanceTests"),
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
					ClusterKey:    types.RootClusterKey("AcceptanceTests"),
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
					ClusterKey: types.RootClusterKey("CitationReq"),
					Detail:     fmt.Sprintf("only %d citations, need ≥%d", len(draft.Citations), n),
					SuspectedRoot: SuspectedRoot{
						IRField:    "CitationReq",
						Reason:     "zero citations vs acceptance floor — answer is ungrounded",
						Confidence: 0.80,
					}})
			}
		default:
			out = append(out, Violation{Kind: ViolAcceptance,
				ClusterKey:    types.RootClusterKey("AcceptanceTests"),
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
//
// Token-form-aware fallback (root cause of s5a r2 retry storm):
// when the plain substring miss happens AND the needle is
// identifier-shaped (ASCII letters/digits/_/- only), the matcher
// scans the text for contiguous identifier-character runs and
// checks whether any run's case-folded, separator-stripped
// "flat form" contains the needle's flat form. This makes
// snake_case ↔ camelCase ↔ PascalCase ↔ kebab-case ↔ SCREAMING_SNAKE
// ↔ flatcase equivalences match — e.g. analyzer-extracted
// wire-name "sub_explorer" hits finalizer-emitted Go-type
// "subExplorerEvaluator" without forcing a contract retry.
//
// Language coverage: the equivalence set is purposely conservative
// (only `_` and `-` separators stripped, ASCII case folded), so it
// is correct for every language repomap supports — Go (camel /
// Pascal), Python (snake / Pascal), Rust (snake / Pascal /
// SCREAMING_SNAKE), Swift (camel / Pascal), Java (camel / Pascal /
// SCREAMING_SNAKE), Ruby (snake / Pascal), JS/TS (camel / Pascal),
// ArkTS (camel / Pascal), Cangjie (camel / Pascal / snake), Kotlin
// (camel / Pascal). Language-specific qualifier separators —
// Rust/C++ `::`, Ruby `#`, Java/Kotlin/JS/Python/Swift `.` package
// dots — are handled by run-boundary semantics: `::` and `#` break
// runs (so `mod::Item` produces two runs and the answer "Item"
// still satisfies the needle), while `.` is kept inside a run so
// filenames like "sub_explorer.go" / "SubExplorer.kt" /
// "sub_explorer.py" satisfy the needle.
//
// Run boundaries (whitespace / punctuation other than _-./) are
// respected, so the prose phrase "the sub explorer" still does NOT
// satisfy a "sub_explorer" must_include — only contiguous
// identifier-shaped occurrences count. Symmetric with must_exclude:
// forbidden-symbol matches across token forms also count as
// violations.
func containsSymbol(text, needle string) bool {
	if strings.Contains(text, needle) {
		return true
	}
	if !isIdentifierShaped(needle) {
		return false
	}
	flatNeedle := flattenIdentifier(needle)
	if len(flatNeedle) < 4 {
		return false
	}
	return flatCaseRunContains(text, flatNeedle)
}

// isIdentifierShaped reports whether s is composed solely of ASCII
// identifier characters (letters, digits, _, -). Empty strings,
// strings with whitespace, dots, slashes, or any other punctuation
// return false. Used as the gate for token-form-aware matching:
// only identifier-shaped needles get the flat-form fallback;
// multi-word phrases keep strict substring semantics.
func isIdentifierShaped(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '-':
			continue
		default:
			return false
		}
	}
	return true
}

// flattenIdentifier strips _ and - separators and lowercases ASCII
// letters, leaving an unambiguous "flat" representation that's
// invariant across snake_case, camelCase, PascalCase, kebab-case,
// and flatcase. Examples: "sub_explorer" → "subexplorer",
// "subExplorer" → "subexplorer", "Sub-Explorer" → "subexplorer",
// "SUBEXPLORER" → "subexplorer".
func flattenIdentifier(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || c == '-' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}

// flatCaseRunContains scans text for contiguous identifier-character
// runs (a-zA-Z0-9_-.) and reports whether any run's flat form
// contains flatNeedle as a substring. The . is included in the run
// charset because filenames like "sub_explorer.go" should satisfy a
// "sub_explorer" needle — the file basename is still a recognisable
// reference to the symbol. Whitespace / other punctuation breaks
// runs, so "sub explorer" (English prose) does NOT match a
// "sub_explorer" needle — only contiguous identifier-shaped
// occurrences satisfy.
func flatCaseRunContains(text, flatNeedle string) bool {
	var run strings.Builder
	flush := func() bool {
		if run.Len() == 0 {
			return false
		}
		flat := flattenIdentifier(run.String())
		run.Reset()
		return strings.Contains(flat, flatNeedle)
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '-', c == '.':
			run.WriteByte(c)
		default:
			if flush() {
				return true
			}
		}
	}
	return flush()
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
