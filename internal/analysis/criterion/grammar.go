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
	KindSymbolPresent                 Kind = Kind(types.CritSymbolPresent)
	KindNoCallSites                   Kind = Kind(types.CritNoCallSites)
	KindAnswerSetBounded              Kind = Kind(types.CritAnswerSetBounded)
	KindAnswerSetUnbounded            Kind = Kind(types.CritAnswerSetUnbounded)
	KindMultipleResolutionChains      Kind = Kind(types.CritMultipleResolutionChains)
	KindUserClauseUnresolved          Kind = Kind(types.CritUserClauseUnresolved)
	KindUntrustedReachesSink          Kind = Kind(types.CritUntrustedReachesSink)
	KindInvariantBroken               Kind = Kind(types.CritInvariantBroken)
	KindNoRelevantEvidence            Kind = Kind(types.CritNoRelevantEvidence)
	KindSignalPresent                 Kind = Kind(types.CritSignalPresent)
	KindHasEnoughFacts                Kind = Kind(types.CritHasEnoughFacts)
	KindAllHypothesesDecided          Kind = Kind(types.CritAllHypothesesDecided)
	KindContractSatisfied             Kind = Kind(types.CritContractSatisfied)
	KindBudgetExhausted               Kind = Kind(types.CritBudgetExhausted)
	KindEvidenceCount                 Kind = Kind(types.CritEvidenceCount)
	KindCitationCountGE               Kind = Kind(types.CritCitationCountGE)
	KindContainsSymbol                Kind = Kind(types.CritContainsSymbol)
	KindRegexMatch                    Kind = Kind(types.CritRegexMatch)
	KindCounterfactualBranchesDecided Kind = Kind(types.CritCounterfactualBranchesDecided)
	KindRelationAbsent                Kind = Kind(types.CritRelationAbsent)
	// Write-mode kinds (B0). Evaluator bodies land in Day 5; B0
	// just registers the constants so gate's criterion_resolvable
	// check doesn't reject templateWriteProposal IRs, and so
	// downstream code using them against a read-mode Env won't
	// panic with ErrUnknownKind.
	KindPlanReady    Kind = Kind(types.CritPlanReady)
	KindPatchApplies Kind = Kind(types.CritPatchApplies)
	KindTestsPass    Kind = Kind(types.CritTestsPass)
	KindNoRegression Kind = Kind(types.CritNoRegression)

	// KindExternalArtifactDecoded is retained for compatibility with
	// historical success criteria. G63 retired its DraftAnswer
	// token-density semantics; runtime-artifact coverage is now
	// enforced by typed AnswerDocumentV2 carriers in the orchestrator
	// contract layer (`observed_artifact_fact` +
	// `external_observation`).
	KindExternalArtifactDecoded Kind = Kind(types.CritExternalArtifactDecoded)
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
	KindRelationAbsent:                true,
	KindPlanReady:                     true,
	KindPatchApplies:                  true,
	KindTestsPass:                     true,
	KindNoRegression:                  true,
	KindExternalArtifactDecoded:       true,
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
	AggregateFacts []types.AnswerAggregateFact
	ToolResults    []types.ToolResult
	PrescanBlob    string
	Signals        types.ExecutionSignals
	DraftAnswer    string
	DraftCitations int
	// ReactItersUsed is the per-task explore-window iteration count
	// the scheduler has already spent. Consumed by budget_exhausted.
	ReactItersUsed int
	// InvestigationComplete is true when the LLM explicitly called
	// emit_investigation_complete. Under the "soft" policy, this
	// lowers evidence_count thresholds to >=1.
	InvestigationComplete bool
	// WriteClosure is the write-mode ground truth for Kind{PlanReady,
	// PatchApplies, TestsPass, NoRegression}. Nil in read-only
	// pipelines (the corresponding evaluators short-circuit to
	// Satisfied=true under the L3 read-mode byte-identity rule).
	// Populated by buildEnv at evaluator-call time; non-nil in write
	// phases (MutableState always allocates via lazy getter).
	WriteClosure *types.WriteClosure

	// ChangePlan is the plan currently being applied/verified. Nil
	// in read-only pipelines and during the plan stage before
	// emit_change_plan fires. evalPatchApplies dereferences this
	// to intersect TargetPaths with WriteClosure.AppliedSet.
	ChangePlan *types.ChangePlan

	// ChangeReport is the verify-stage test outcome. Nil until
	// run_tests populates it. evalTestsPass reads .Passed +
	// per-TestResult status from here.
	ChangeReport *types.ChangeReport

	// BaselineReport is the pre-apply test snapshot (captured by
	// the apply stage hook before the coder agent dispatches) used by
	// evalNoRegression to detect tests that passed pre-apply but
	// fail post-apply. Nil when baseline capture was skipped or
	// the feature is disabled.
	BaselineReport *types.ChangeReport

	// LogTriage is the structured LogBundle the log_triager pre-stage
	// produced when an attached runtime log was present at Run entry.
	// Nil when no log was attached or triage failed (advisory). The
	// external-artifact criterion keeps this slot for compatibility
	// but no longer scans DraftAnswer text; typed coverage lives in
	// AnswerDocumentV2 contract checks.
	LogTriage *types.LogBundle

	// PerfTrace is the structured PerfBundle the perf_triager
	// pre-stage produced when an attached HiTrace / atrace / systrace
	// was present. Nil when no trace was attached or triage failed.
	// Same role as LogTriage but for performance-trace tokens
	// (trigger_span, stall.symbol, jank.reason, startup.mode).
	PerfTrace *types.PerfBundle
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
