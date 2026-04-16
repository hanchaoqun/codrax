package criterion

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Eval evaluates a single Criterion against env. Unknown Kind is
// reported via Result.UnknownKind — callers at runtime should treat
// that as a programmer error (gate is supposed to catch it first).
func Eval(c types.Criterion, env Env) Result {
	k := Kind(c.Kind)
	if !IsRegistered(k) {
		return Result{UnknownKind: true, Kind: k, Expr: c.Expr,
			Detail: fmt.Sprintf("criterion kind %q is not registered", c.Kind)}
	}
	r := dispatch(k, c.Expr, env)
	r.Kind = k
	r.Expr = c.Expr
	return r
}

// EvalAll evaluates every criterion and returns:
//
//   - allOK=true when every criterion is Satisfied AND no unknown
//     kinds were seen.
//   - failed is the list of unsatisfied and unknown-kind results; a
//     caller can walk it to build a retry hint or panic on unknown.
//
// When any criterion has UnknownKind=true, allOK is false regardless
// of the other criteria — the IR is not trusted.
func EvalAll(cs []types.Criterion, env Env) (allOK bool, failed []Result) {
	allOK = true
	for _, c := range cs {
		r := Eval(c, env)
		if r.UnknownKind || !r.Satisfied {
			allOK = false
			failed = append(failed, r)
		}
	}
	return allOK, failed
}

// dispatch is the central switch. Each case is an independent
// evaluator — intentionally flat rather than a map+closure so every
// handler is grep-discoverable.
func dispatch(k Kind, expr string, env Env) Result {
	switch k {
	case KindSymbolPresent:
		return evalSymbolPresent(expr, env)
	case KindNoCallSites:
		return evalNoCallSites(expr, env)
	case KindAnswerSetBounded:
		return evalAnswerSetBounded(expr, env)
	case KindAnswerSetUnbounded:
		return evalAnswerSetUnbounded(env)
	case KindMultipleResolutionChains:
		return evalMultipleResolutionChains(env)
	case KindUserClauseUnresolved:
		return evalUserClauseUnresolved(expr, env)
	case KindUntrustedReachesSink:
		return evalUntrustedReachesSink(env)
	case KindInvariantBroken:
		return evalInvariantBroken(expr, env)
	case KindNoRelevantEvidence:
		return evalNoRelevantEvidence(env)
	case KindSignalPresent:
		return evalSignalPresent(expr, env)
	case KindHasEnoughFacts:
		return evalHasEnoughFacts(env)
	case KindAllHypothesesDecided:
		return evalAllHypothesesDecided(env)
	case KindContractSatisfied:
		return evalContractSatisfied(env)
	case KindBudgetExhausted:
		return evalBudgetExhausted(env)
	case KindEvidenceCount:
		return evalEvidenceCount(expr, env)
	case KindCitationCountGE:
		return evalCitationCountGE(expr, env)
	case KindContainsSymbol:
		return evalContainsSymbol(expr, env)
	case KindRegexMatch:
		return evalRegexMatch(expr, env)
	case KindCounterfactualBranchesDecided:
		return evalCounterfactualBranchesDecided(env)
	}
	return Result{UnknownKind: true,
		Detail: fmt.Sprintf("no handler for kind %q (internal bug: registered but unreachable)", k)}
}

// ── individual evaluators ─────────────────────────────────────────

func evalSymbolPresent(expr string, env Env) Result {
	sym := strings.TrimSpace(expr)
	if sym == "" {
		return Result{Satisfied: true, Detail: "empty symbol — treated as trivially satisfied"}
	}
	needle := strings.ToLower(sym)
	for _, e := range env.Evidence {
		if containsLower(e.Subject, needle) || containsLower(e.Object, needle) ||
			containsLower(e.Summary, needle) {
			return Result{Satisfied: true, Detail: "symbol found in evidence"}
		}
	}
	for _, s := range env.AnswerSymbols {
		if containsLower(s.Name, needle) {
			return Result{Satisfied: true, Detail: "symbol found in answer slate"}
		}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("symbol %q not seen in any evidence or answer slate", sym)}
}

func evalNoCallSites(expr string, env Env) Result {
	sym := strings.TrimSpace(expr)
	if sym == "" {
		return Result{Satisfied: true, Detail: "empty symbol — trivially no call sites"}
	}
	needle := strings.ToLower(sym)
	hits := 0
	for _, r := range env.ToolResults {
		if !r.Success {
			continue
		}
		if containsLower(r.Summary, needle) {
			hits++
		}
	}
	if hits == 0 {
		return Result{Satisfied: true, Detail: "no call sites observed in tool results"}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("%d tool result(s) mention %q", hits, sym)}
}

func evalAnswerSetBounded(expr string, env Env) Result {
	n := len(env.AnswerSymbols)
	op, threshold, ok := parseComparison(expr)
	if !ok {
		return Result{Satisfied: false,
			Detail: fmt.Sprintf("malformed comparison %q (expect e.g. <=5, >=2, ==3)", expr)}
	}
	satisfied := compareInt(n, op, threshold)
	return Result{Satisfied: satisfied,
		Detail: fmt.Sprintf("|answer_symbols|=%d %s %d", n, op, threshold)}
}

func evalAnswerSetUnbounded(env Env) Result {
	if len(env.AnswerChains) > 50 {
		return Result{Satisfied: true, Detail: fmt.Sprintf("answer chains exploded to %d (>50)", len(env.AnswerChains))}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("answer chains still bounded (%d ≤ 50)", len(env.AnswerChains))}
}

func evalMultipleResolutionChains(env Env) Result {
	n := 0
	for _, e := range env.Evidence {
		if e.Predicate == "config_resolution" {
			n++
		}
	}
	if n > 1 {
		return Result{Satisfied: true, Detail: fmt.Sprintf("%d config_resolution evidence items", n)}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("only %d config_resolution evidence items", n)}
}

func evalUserClauseUnresolved(expr string, env Env) Result {
	if env.IR == nil {
		return Result{Satisfied: false, Detail: "no IR available"}
	}
	needle := strings.TrimSpace(expr)
	for _, a := range env.IR.RequestModel.Ambiguities {
		if needle != "" && !strings.EqualFold(strings.TrimSpace(a.Clause), needle) {
			continue
		}
		if strings.TrimSpace(a.Resolution) == "" {
			return Result{Satisfied: true,
				Detail: fmt.Sprintf("ambiguity %q has no resolution", a.Clause)}
		}
	}
	return Result{Satisfied: false, Detail: "all matching ambiguities resolved"}
}

func evalUntrustedReachesSink(env Env) Result {
	// Use the IR's RiskMatrix Security dimension as the proxy for
	// "the analyzer flagged an untrusted-to-sink concern". The
	// criterion fires when a Security level ≥ 4 landed AND at least
	// one evidence item mentions a boundary or sink.
	if env.IR == nil {
		return Result{Satisfied: false, Detail: "no IR"}
	}
	if env.IR.RequestModel.RiskMatrix.Security.Level < 4 {
		return Result{Satisfied: false,
			Detail: fmt.Sprintf("security risk level %d < 4", env.IR.RequestModel.RiskMatrix.Security.Level)}
	}
	for _, e := range env.Evidence {
		s := strings.ToLower(e.Predicate + " " + e.Summary)
		if strings.Contains(s, "untrusted") || strings.Contains(s, "sink") || strings.Contains(s, "boundary") {
			return Result{Satisfied: true, Detail: "evidence mentions untrusted/sink/boundary"}
		}
	}
	return Result{Satisfied: false, Detail: "no evidence mentions an untrusted path"}
}

func evalInvariantBroken(expr string, env Env) Result {
	needle := strings.ToLower(strings.TrimSpace(expr))
	for _, e := range env.Evidence {
		pred := strings.ToLower(e.Predicate)
		if pred == "invariant_violation" || pred == "invariant_broken" {
			if needle == "" || containsLower(e.Summary, needle) || containsLower(e.Subject, needle) {
				return Result{Satisfied: true, Detail: "invariant violation in evidence"}
			}
		}
	}
	return Result{Satisfied: false, Detail: "no invariant_violation evidence"}
}

func evalNoRelevantEvidence(env Env) Result {
	if len(env.Evidence) == 0 {
		return Result{Satisfied: true, Detail: "no evidence items collected"}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("%d evidence items present", len(env.Evidence))}
}

func evalSignalPresent(expr string, env Env) Result {
	switch strings.TrimSpace(expr) {
	case "has_enough_facts", "":
		return evalHasEnoughFacts(env)
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("unknown signal %q", expr)}
}

func evalHasEnoughFacts(env Env) Result {
	if env.Signals.HasEnoughFacts {
		return Result{Satisfied: true, Detail: "HasEnoughFacts=true"}
	}
	return Result{Satisfied: false, Detail: "HasEnoughFacts=false"}
}

func evalAllHypothesesDecided(env Env) Result {
	if env.IR == nil {
		return Result{Satisfied: false, Detail: "no IR"}
	}
	undecided := 0
	for _, h := range env.IR.HypothesisSet {
		if h.Status == types.HypUnknown || h.Status == "" {
			undecided++
		}
	}
	if undecided == 0 {
		return Result{Satisfied: true, Detail: "all hypotheses decided"}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("%d hypothesis(es) still unknown", undecided)}
}

func evalContractSatisfied(env Env) Result {
	if strings.TrimSpace(env.DraftAnswer) == "" {
		return Result{Satisfied: false, Detail: "no draft answer yet"}
	}
	if env.IR == nil {
		return Result{Satisfied: true, Detail: "draft exists; no IR to cross-check"}
	}
	if env.IR.AnswerContract.CitationReq.Required && env.DraftCitations < env.IR.AnswerContract.CitationReq.MinCitations {
		return Result{Satisfied: false,
			Detail: fmt.Sprintf("citations %d < required %d", env.DraftCitations, env.IR.AnswerContract.CitationReq.MinCitations)}
	}
	return Result{Satisfied: true, Detail: "draft answer meets citation floor"}
}

func evalBudgetExhausted(env Env) Result {
	if env.IR == nil {
		return Result{Satisfied: false, Detail: "no IR to read budget from"}
	}
	cap := env.IR.EvidencePlan.Budget.MaxReactIters
	if cap <= 0 {
		return Result{Satisfied: false, Detail: "no MaxReactIters set"}
	}
	if env.ReactItersUsed >= cap {
		return Result{Satisfied: true, Detail: fmt.Sprintf("react iters %d ≥ cap %d", env.ReactItersUsed, cap)}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("react iters %d < cap %d", env.ReactItersUsed, cap)}
}

func evalEvidenceCount(expr string, env Env) Result {
	op, threshold, ok := parseComparison(expr)
	if !ok {
		return Result{Satisfied: false, Detail: fmt.Sprintf("malformed comparison %q", expr)}
	}
	n := len(env.Evidence)
	return Result{
		Satisfied: compareInt(n, op, threshold),
		Detail:    fmt.Sprintf("|evidence|=%d %s %d", n, op, threshold),
	}
}

func evalCitationCountGE(expr string, env Env) Result {
	threshold, err := strconv.Atoi(strings.TrimSpace(expr))
	if err != nil {
		return Result{Satisfied: false, Detail: fmt.Sprintf("malformed integer %q", expr)}
	}
	if env.DraftCitations >= threshold {
		return Result{Satisfied: true, Detail: fmt.Sprintf("%d citations ≥ %d", env.DraftCitations, threshold)}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("%d citations < %d", env.DraftCitations, threshold)}
}

func evalContainsSymbol(expr string, env Env) Result {
	needle := strings.TrimSpace(expr)
	if needle == "" {
		return Result{Satisfied: true, Detail: "empty needle"}
	}
	if strings.Contains(env.DraftAnswer, needle) {
		return Result{Satisfied: true, Detail: "needle found in draft"}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("needle %q not in draft", needle)}
}

func evalRegexMatch(expr string, env Env) Result {
	if strings.TrimSpace(expr) == "" {
		return Result{Satisfied: true, Detail: "empty pattern"}
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return Result{Satisfied: false, Detail: fmt.Sprintf("invalid regex: %v", err)}
	}
	if re.MatchString(env.DraftAnswer) {
		return Result{Satisfied: true, Detail: "pattern matches draft"}
	}
	return Result{Satisfied: false, Detail: "pattern does not match draft"}
}

func evalCounterfactualBranchesDecided(env Env) Result {
	if env.IR == nil {
		return Result{Satisfied: true, Detail: "no IR"}
	}
	for _, h := range env.IR.HypothesisSet {
		if h.Status == types.HypUnknown || h.Status == "" {
			// Only require decisions on hypotheses bound to
			// counterfactual nodes. If no such binding exists the
			// criterion trivially holds.
			bound := false
			for _, n := range env.IR.TaskGraph.Nodes {
				if !n.IsCounterfactual {
					continue
				}
				for _, hid := range n.Hypotheses {
					if hid == h.ID {
						bound = true
						break
					}
				}
				if bound {
					break
				}
			}
			if bound {
				return Result{Satisfied: false,
					Detail: fmt.Sprintf("counterfactual-bound hypothesis %q still unknown", h.ID)}
			}
		}
	}
	return Result{Satisfied: true, Detail: "all counterfactual-bound hypotheses decided"}
}

// ── small helpers ──────────────────────────────────────────────

func containsLower(hay, needleLower string) bool {
	if needleLower == "" {
		return false
	}
	return strings.Contains(strings.ToLower(hay), needleLower)
}

// parseComparison accepts "<=N", ">=N", "==N", "<N", ">N", "=N",
// or a bare integer (treated as "==N"). Returns the operator and
// integer threshold.
func parseComparison(expr string) (op string, threshold int, ok bool) {
	s := strings.TrimSpace(expr)
	if s == "" {
		return "", 0, false
	}
	// Two-char operators first.
	for _, cand := range []string{"<=", ">=", "==", "!="} {
		if strings.HasPrefix(s, cand) {
			v, err := strconv.Atoi(strings.TrimSpace(s[len(cand):]))
			if err != nil {
				return "", 0, false
			}
			return cand, v, true
		}
	}
	// Single-char operators.
	for _, cand := range []string{"<", ">", "="} {
		if strings.HasPrefix(s, cand) {
			v, err := strconv.Atoi(strings.TrimSpace(s[len(cand):]))
			if err != nil {
				return "", 0, false
			}
			norm := cand
			if cand == "=" {
				norm = "=="
			}
			return norm, v, true
		}
	}
	// Bare integer → "==".
	if v, err := strconv.Atoi(s); err == nil {
		return "==", v, true
	}
	return "", 0, false
}

func compareInt(n int, op string, threshold int) bool {
	switch op {
	case "<=":
		return n <= threshold
	case ">=":
		return n >= threshold
	case "==":
		return n == threshold
	case "!=":
		return n != threshold
	case "<":
		return n < threshold
	case ">":
		return n > threshold
	}
	return false
}
