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

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
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
	ViolFamilyMismatch = types.ViolFamilyMismatch
	ViolCitation       = types.ViolCitation
	ViolMustInclude    = types.ViolMustInclude
	ViolMustExclude    = types.ViolMustExclude
	ViolAcceptance     = types.ViolAcceptance

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
	ViolViewSwap             = types.ViolViewSwap

	// Commit 53 P2/P4 — read-mode answer-coherence violations.
	ViolViewIntentMismatch    = types.ViolViewIntentMismatch
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
	for _, term := range normalizedMustIncludeTerms(c) {
		if strings.TrimSpace(term.Text) == "" {
			continue
		}
		var hit bool
		if term.Kind == types.ContractTermSymbol && types.QualifiedTermTrailingSegment(term.Text) != "" {
			// QNO F1 (2026-07-05): qualifier-carrying symbol terms
			// ("gate.Run") are judged EXCLUSIVELY by whole-token
			// equality (full spelling or bare tail) — see
			// qualifiedSymbolTermHit. contractTermHit's raw substring
			// path is deliberately bypassed: "gate.RunWith" contains
			// "gate.Run" as a substring and would false-satisfy the
			// exact s1a sibling-identifier failure this pin exists to
			// catch. No oracle re-gate here: such terms are produced
			// only by R3b from provenance rows the oracle ALREADY
			// resolved. Include-side only — must_exclude keeps
			// contractTermHit semantics (widening a violation-CREATING
			// surface needs its own ruling; precise-signals red line).
			hit = qualifiedSymbolTermHit(draft.Text, term.Text)
		} else {
			hit = contractTermHit(draft, term, oracle)
		}
		if !hit {
			repair := "include " + term.Text + " in the final answer"
			if term.Kind == types.ContractTermFileStem {
				repair = "mention " + term.Text + " in the final answer or cite the matching source file"
			}
			out = append(out, Violation{
				Kind:       ViolMustInclude,
				ClusterKey: types.IdentityClusterKey("term:"+term.Text, "must_include"),
				Detail:     fmt.Sprintf("required %s %q missing from answer", contractTermKindLabel(term.Kind), term.Text),
				Repair:     repair,
			})
		}
	}
	return out
}

func normalizedMustIncludeTerms(c types.AnswerContract) []types.ContractTerm {
	return types.NormalizedMustIncludeTerms(c)
}

func contractTermHit(draft Answer, term types.ContractTerm, oracle types.SymbolOracle) bool {
	switch term.Kind {
	case types.ContractTermUserPhrase:
		return strings.Contains(strings.ToLower(draft.Text), strings.ToLower(term.Text))
	case types.ContractTermToolName:
		return containsSymbol(draft.Text, term.Text)
	case types.ContractTermFileStem:
		return containsSymbol(draft.Text, term.Text) ||
			contractFileStemCoveredByCitation(draft.Citations, term.Text)
	default:
		hit := containsSymbol(draft.Text, term.Text)
		if hit && oracle != nil && shouldOracleGateInclude(term.Text) {
			if !oracleHasReliableSymbol(oracle, term.Text) {
				// Substring matches but the term isn't a real symbol —
				// treat as miss (LLM hallucinated the include).
				hit = false
			}
		}
		return hit
	}
}

func contractFileStemCoveredByCitation(citations []Citation, term string) bool {
	termKeys := contractFileTermKeys(term)
	if len(termKeys) == 0 {
		return false
	}
	want := make(map[string]bool, len(termKeys))
	for _, key := range termKeys {
		want[key] = true
	}
	for _, cit := range citations {
		for _, key := range contractFileTermKeys(cit.File) {
			if want[key] {
				return true
			}
		}
	}
	return false
}

func contractFileTermKeys(path string) []string {
	clean := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`)))
	if clean == "" {
		return nil
	}
	out := []string{clean}
	base := clean
	if i := strings.LastIndex(base, "/"); i >= 0 && i+1 < len(base) {
		base = base[i+1:]
	}
	if base != "" && base != clean {
		out = append(out, base)
	}
	if dot := strings.LastIndex(base, "."); dot > 0 {
		out = append(out, base[:dot])
	}
	return out
}

func contractTermKindLabel(kind types.ContractTermKind) string {
	switch kind {
	case types.ContractTermToolName:
		return "tool name"
	case types.ContractTermFileStem:
		return "file stem"
	case types.ContractTermUserPhrase:
		return "phrase"
	default:
		return "symbol"
	}
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

// qualifiedSymbolTermHit (QNO F1, 2026-07-05) — include-side
// acceptance for symbol-kind must_include terms that carry a scope
// qualifier ("gate.Run", "mod::Type::method"). Such terms exist only
// via TYPED MustIncludeTerms (R3b forces Kind=symbol on oracle-
// resolved provenance surfaces); InferContractTermKind still routes
// dotted text to file_stem, so inferred terms never reach this path.
//
// contractTermHit's verbatim-substring path already accepts the exact
// dotted spelling; this fallback additionally accepts, and ONLY
// accepts:
//   - a whole-token flat-equality occurrence of the FULL qualified
//     spelling (answer says "Gate.Run", term is "gate.Run"), or
//   - a whole-token flat-equality occurrence of the BARE trailing
//     segment (answer says "Run") — mirroring the
//     UnconsumedAnchorObligations ruling that a qualified user
//     spelling counts as consumed when the bare tail is present
//     verbatim.
//
// Whole-token EQUALITY, never substring, keeps the original s1a
// failure caught: an answer that only discusses "RunWith" does NOT
// satisfy a "gate.Run" obligation ("runwith" ≠ "run"). Unqualified
// terms return false immediately — their semantics stay owned by
// contractTermHit.
func qualifiedSymbolTermHit(text, term string) bool {
	tail := types.QualifiedTermTrailingSegment(term)
	if tail == "" {
		return false
	}
	if flatFull := flattenIdentifier(term); flatFull != "" && flatCaseRunEquals(text, flatFull) {
		return true
	}
	flatTail := flattenIdentifier(tail)
	return flatTail != "" && flatCaseRunEquals(text, flatTail)
}

// flatCaseRunEquals mirrors flatCaseRunContains' run scan but demands
// the whole run's flat form EQUAL flatNeedle instead of containing it.
// Equality is precise at any needle length (no ≥4 noise floor needed):
// "Run" matches the token "Run" but never "RunWith" / "runtime".
// Edge dots are trimmed before comparing because `.` is a run
// character: sentence-final "…drives Run." produces the run "Run."
// whose identifier content is still exactly "Run". Interior dots are
// load-bearing and kept ("gate.run" ≠ "run").
func flatCaseRunEquals(text, flatNeedle string) bool {
	var run strings.Builder
	flush := func() bool {
		if run.Len() == 0 {
			return false
		}
		flat := strings.Trim(flattenIdentifier(run.String()), ".")
		run.Reset()
		return flat == flatNeedle
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

func oracleHasReliableSymbol(oracle types.SymbolOracle, name string) bool {
	if oracle == nil {
		return false
	}
	if found, tier := oracle.SymbolExists(name); found && tier < 3 {
		return true
	}
	if found, tier := oracle.SymbolExistsFlat(name); found && tier < 3 {
		return true
	}
	return false
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
	for _, term := range normalizedMustExcludeTerms(c) {
		if strings.TrimSpace(term.Text) == "" {
			continue
		}
		hit := contractTermHit(draft, term, oracle)
		if hit {
			out = append(out, Violation{
				Kind:       ViolMustExclude,
				ClusterKey: types.IdentityClusterKey("term:"+term.Text, "must_exclude"),
				Detail:     fmt.Sprintf("forbidden %s %q present in answer", contractTermKindLabel(term.Kind), term.Text),
			})
		}
	}
	return out
}

func normalizedMustExcludeTerms(c types.AnswerContract) []types.ContractTerm {
	return types.NormalizedMustExcludeTerms(c)
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
				if !oracleHasReliableSymbol(oracle, a.Expr) {
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

// flattenIdentifier delegates to normalizer.NormalizeCodeKey, which
// is the SINGLE canonical implementation of identifier flat-form
// canonicalisation across the codebase (Fix K, 2026-05-07).
//
// The two functions used to be duplicate implementations — Fix A
// added flattenIdentifier here for the contract layer's
// identifier-existence checks, while NormalizeCodeKey predates it
// in the analysis/normalizer package for the symbol-resolver
// path. Both stripped `_` / `-` and lowercased ASCII identically.
// Fix K consolidates onto NormalizeCodeKey so future changes to
// the canonicalisation rule have one place to land.
//
// flattenIdentifier is preserved as a thin wrapper for back-
// compat with existing internal call sites; the public
// contract.FlattenIdentifier export and normalizer.NormalizeCodeKey
// both flow into the same implementation.
//
// Equivalence: snake_case, camelCase, PascalCase, kebab-case,
// SCREAMING_SNAKE_CASE, flatcase all collapse to the same lower-
// case separator-stripped form. Examples: "sub_explorer" →
// "subexplorer", "subExplorer" → "subexplorer", "Sub-Explorer"
// → "subexplorer", "SUBEXPLORER" → "subexplorer".
func flattenIdentifier(s string) string {
	return normalizer.NormalizeCodeKey(s)
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
