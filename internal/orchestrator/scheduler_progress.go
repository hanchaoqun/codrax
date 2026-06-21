package orchestrator

import (
	"context"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/types"
)

// envShape is the scheduler's low-cost cursor over criterion.Env.
// It intentionally stores precise counts only; noisy ranking,
// timing, or model prose must not enter this hard-loop guard.
//
// Fields:
//   - EvidenceCount:      len(criterion.Env.Evidence)
//   - AnswerSymbolCount:  len(BusContext.AnswerSymbols)
//   - AnswerChainCount:   len(BusContext.AnswerChains)
//   - AggregateFactCount: len(criterion.Env.AggregateFacts)
//   - ToolResultCount:    len(BusContext.ToolResults)
//   - ReadSetSize:        |EvidenceClosure.ReadSet|
//   - PendingReadsSize:   |EvidenceClosure.PendingReads|
//   - DecidedHypotheses:  count of HypothesisSet entries with a
//     non-unknown non-empty Status
//   - PrescanBytes:       len(PrescanSummaryBlob)
//
// A change in any field means "something Env-visible advanced since
// last snapshot" — the predicate may now return a different verdict
// and re-evaluation is legitimate. All fields zero is the valid empty
// shape (used as the sentinel "never evaluated" value via pointer nil
// at call sites rather than zero-value confusion).
type envShape struct {
	EvidenceCount      int
	AnswerSymbolCount  int
	AnswerChainCount   int
	AggregateFactCount int
	ToolResultCount    int
	ReadSetSize        int
	PendingReadsSize   int
	DecidedHypotheses  int
	PrescanBytes       int
}

// computeEnvShape captures the current cursor positions of every
// state source that feeds criterion.Env. Pure function; safe to call
// from anywhere that has a BusContext + Env. Nil bus is tolerated for
// unit-test ergonomics and returns the zero shape.
func computeEnvShape(bus *types.BusContext, env criterion.Env) envShape {
	s := envShape{
		EvidenceCount:      len(env.Evidence),
		AnswerSymbolCount:  len(env.AnswerSymbols),
		AnswerChainCount:   len(env.AnswerChains),
		AggregateFactCount: len(env.AggregateFacts),
		ToolResultCount:    len(env.ToolResults),
		PrescanBytes:       len(env.PrescanBlob),
	}
	if env.IR != nil {
		for _, h := range env.IR.HypothesisSet {
			if h.Status != "" && h.Status != types.HypUnknown {
				s.DecidedHypotheses++
			}
		}
	}
	if bus != nil && bus.Mutable != nil {
		if closure := bus.Mutable.EvidenceClosure(); closure != nil {
			s.ReadSetSize = len(closure.ReadSet())
			s.PendingReadsSize = len(closure.PendingReads())
		}
	}
	return s
}

// equals reports whether two shapes represent the same Env cursor
// position. Struct comparison is sufficient since every field is a
// scalar int and the fields cover all Env inputs the scheduler cares
// about.
func (a envShape) equals(b envShape) bool {
	return a == b
}

// hypProgress is the per-validate-node hypothesis-scope progress
// fingerprint. Complements envShape — session 22 extension.
//
// The bug envShape alone cannot catch: the explorer keeps emitting
// evidence that does not satisfy any unknown hypothesis's
// RequiredEvidence (e.g. traceback pasted with paths outside the
// repo → explorer fishes in codrax's own infrastructure). envShape's
// EvidenceCount / ToolResultCount / ReadSetSize advance each round,
// so envShape.equals() never triggers and the validate loop runs
// until step budget drains.
//
// hypProgress collapses the progress picture to two integers that
// only move when an unknown hypothesis actually inches toward
// decidable:
//
//   - UnknownCount:     count of hypotheses still in HypUnknown (or "")
//   - SatisfiedReqSum:  sum over those unknowns of criterion.EvalAll
//     (h.RequiredEvidence, env) hits — the same
//     primitive runAutoVerdicts uses to promote
//     HypUnknown → HypInconclusive
//
// Two SC failures with equal hypProgress ⇒ no unknown hypothesis
// advanced its RequiredEvidence satisfaction between them, even if
// global env shape did. Scheduler treats this as stuck (identical
// philosophy to envShape's full-env stall), with the OR semantics
// applied at the call site.
//
// Reused primitive: criterion.EvalAll is the same function the
// orchestrator already calls from runAutoVerdicts; adding a second
// fingerprint is a composition over existing capability, not a new
// evaluation pathway.
type hypProgress struct {
	UnknownCount    int
	SatisfiedReqSum int
}

// computeHypProgress captures per-validate-node hypothesis progress
// from the current Env. Pure function over the same Env the
// scheduler already hands to criterion.EvalAll; safe to call from
// any call site holding a buildEnv result.
func computeHypProgress(env criterion.Env) hypProgress {
	p, _ := computeHypProgressContext(context.Background(), env)
	return p
}

func computeHypProgressContext(ctx context.Context, env criterion.Env) (hypProgress, error) {
	if env.IR == nil {
		return hypProgress{}, nil
	}
	p := hypProgress{}
	for _, h := range env.IR.HypothesisSet {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		if h.Status != types.HypUnknown && h.Status != "" {
			continue
		}
		p.UnknownCount++
		if len(h.RequiredEvidence) == 0 {
			continue
		}
		for _, c := range h.RequiredEvidence {
			if err := ctx.Err(); err != nil {
				return p, err
			}
			if criterion.Eval(c, env).Satisfied {
				p.SatisfiedReqSum++
			}
		}
	}
	return p, ctx.Err()
}

// equals reports whether two hypProgress fingerprints describe the
// same hypothesis-scope progress cursor. Cheap struct ==.
func (a hypProgress) equals(b hypProgress) bool {
	return a == b
}
