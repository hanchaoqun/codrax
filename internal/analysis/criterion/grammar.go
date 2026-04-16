// Package criterion is the executable-contract layer of Analyzer v3.
//
// Every {Kind, Expr} pair that appears anywhere in AnalysisIR — node
// EntryConditions / SuccessCriteria, hypothesis RequiredEvidence /
// FalsificationCondition, EvidencePlan StopConditions, AnswerContract
// AcceptanceTests — is parsed and evaluated by this package. The
// compiler writes Criterion values, the gate statically verifies that
// every Kind is registered, and the scheduler / extractor / contract
// checker evaluate them at runtime against a shared Env.
//
// The single source of truth for legal Kind values is RegisteredKinds.
// A gate check (criterion_resolvable) scans the IR for any Criterion
// whose Kind is not in that list and rejects the analyze stage
// outright — so downstream runtime evaluators can assume they will
// never see an unknown Kind, and panic if they do.
package criterion

import (
	"errors"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Kind is the closed namespace of legal criterion kinds. Values not
// in RegisteredKinds are rejected at gate time.
type Kind string

const (
	KindSymbolPresent                 Kind = "symbol_present"
	KindNoCallSites                   Kind = "no_call_sites"
	KindAnswerSetBounded              Kind = "answer_set_bounded"
	KindAnswerSetUnbounded            Kind = "answer_set_unbounded"
	KindMultipleResolutionChains      Kind = "multiple_resolution_chains"
	KindUserClauseUnresolved          Kind = "user_clause_unresolved"
	KindUntrustedReachesSink          Kind = "untrusted_reaches_sink"
	KindInvariantBroken               Kind = "invariant_broken"
	KindNoRelevantEvidence            Kind = "no_relevant_evidence"
	KindSignalPresent                 Kind = "signal_present"
	KindHasEnoughFacts                Kind = "has_enough_facts"
	KindAllHypothesesDecided          Kind = "all_hypotheses_decided"
	KindContractSatisfied             Kind = "contract_satisfied"
	KindBudgetExhausted               Kind = "budget_exhausted"
	KindEvidenceCount                 Kind = "evidence_count"
	KindCitationCountGE               Kind = "citation_count_ge"
	KindContainsSymbol                Kind = "contains_symbol"
	KindRegexMatch                    Kind = "regex_match"
	KindCounterfactualBranchesDecided Kind = "counterfactual_branches_decided"
)

// registered is the source of truth for legal Kind values. Gate's
// criterion_resolvable check reads this map; runtime evaluators walk
// dispatchTable directly.
var registered = map[Kind]bool{
	KindSymbolPresent:                 true,
	KindNoCallSites:                   true,
	KindAnswerSetBounded:              true,
	KindAnswerSetUnbounded:            true,
	KindMultipleResolutionChains:      true,
	KindUserClauseUnresolved:          true,
	KindUntrustedReachesSink:          true,
	KindInvariantBroken:               true,
	KindNoRelevantEvidence:            true,
	KindSignalPresent:                 true,
	KindHasEnoughFacts:                true,
	KindAllHypothesesDecided:          true,
	KindContractSatisfied:             true,
	KindBudgetExhausted:               true,
	KindEvidenceCount:                 true,
	KindCitationCountGE:               true,
	KindContainsSymbol:                true,
	KindRegexMatch:                    true,
	KindCounterfactualBranchesDecided: true,
}

// IsRegistered reports whether k is in the closed namespace.
func IsRegistered(k Kind) bool {
	return registered[k]
}

// RegisteredKinds returns every legal Kind. Ordering is not stable —
// callers that need deterministic output must sort.
func RegisteredKinds() []Kind {
	out := make([]Kind, 0, len(registered))
	for k := range registered {
		out = append(out, k)
	}
	return out
}

// ErrUnknownKind is returned by EvalAll when the Kind was not in
// RegisteredKinds. Runtime callers turn this into a panic because
// gate is supposed to have caught it at analyze time; Gate itself
// turns it into a hard check failure.
var ErrUnknownKind = errors.New("criterion: unknown kind")

// Env carries the runtime data a criterion evaluator inspects. Every
// field is a read-only snapshot; evaluators must never mutate. Fields
// that are irrelevant to the current call site may be left zero —
// evaluators document which fields they require.
type Env struct {
	IR             *types.AnalysisIR
	Evidence       []types.EvidenceItem
	AnswerSymbols  []types.AnswerSymbol
	AnswerChains   []types.AnswerChain
	ToolResults    []types.ToolResult
	PrescanBlob    string
	Signals        types.ExecutionSignals
	DraftAnswer    string
	DraftCitations int
	// ReactItersUsed is the per-task explore-window iteration count
	// the scheduler has already spent. Consumed by budget_exhausted.
	ReactItersUsed int
}

// Result is the outcome of evaluating a single Criterion.
type Result struct {
	// Satisfied is true when the criterion was met. A false value
	// with UnknownKind=false means the criterion is well-formed but
	// the current environment does not satisfy it.
	Satisfied bool
	// UnknownKind is true iff Eval could not find a handler for
	// c.Kind. This should never be observed at runtime because gate
	// rejects unregistered kinds at analyze time.
	UnknownKind bool
	// Detail is a short human-readable diagnosis. Populated on both
	// satisfied=true and satisfied=false so retry hints can show
	// exactly which criterion fired or did not fire.
	Detail string
	// Kind is copied into the result so callers rendering retry
	// hints do not need to pass the original Criterion alongside.
	Kind Kind
	// Expr is copied for the same reason.
	Expr string
}
