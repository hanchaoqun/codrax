// Package amplifier augments the LLM-emitted RequestModel with
// deterministic structural inferences derived from existing typed
// signals on the model itself. It is the post-emit_analysis sibling
// of the reconciler family in internal/agent/analyzer_*.go: where
// reconcilers patch a single typed slot per LLM dimension, the
// amplifier fills OPTIONAL typed slots that the LLM legitimately may
// have left empty (Predicates.IsCategoryEnumeration / SubTopics /
// Buckets).
//
// Design contract — six red lines, see docs/design/analyzer_amplifier_layer.md §9:
//  1. Never read rm.RawRequest text. Function signatures intentionally
//     forbid passing anything that could leak the raw question.
//  2. Never override an LLM-emitted non-zero field. Each rule prefixes
//     with `if existing != zero { skip }`.
//  3. Rules must not key on case-specific entity names. Tests cover
//     structural inputs (TermKind / Confidence shape), never literals.
//  4. Read only typed slots — TermGraph, AnalyzerHints, Predicates,
//     Intent, AnswerSubject. No ctx, no I/O, no reader injection.
//  5. Observation output is shape-compatible with reconcileEvent so
//     the agent caller can thread firings through the existing
//     recordReconcileObservation telemetry chain.
//  6. Pure functions. Same input → same output. No state.
//
// Phase 1.1 establishes the package skeleton with an empty rule
// registry. Phase 2-4 wire individual rules (R1 multi-subject /
// R2 typed-name parity / R3 typed-identifier MustInclude). The
// public surface stays stable across phases so the agent integration
// site does not churn.
package amplifier

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// Observation records one rule firing. Shape-aligned with
// types.ReconcileObservation so the caller can wrap each firing
// through reconcileEvent without losing information.
type Observation struct {
	Rule   string // stable identifier, e.g. "R1_multi_subject_predicate"
	Field  string // dotted typed-slot path, e.g. "predicates.is_category_enumeration"
	Before string // serialized prior value (empty if the slot was zero)
	After  string // serialized new value
	Reason string // structural rationale, no case-specific names
}

// preCompileRule is the function shape of every Phase 2/3 rule. It
// reads from rm and writes back to *out (a working copy). Returns
// nil to indicate "rule did not fire". A non-nil Observation is the
// canonical signal that out was mutated.
type preCompileRule func(in types.RequestModel, out *types.RequestModel) *Observation

// postCompileRule is the function shape of every Phase 4 rule. It
// reads rm and may mutate *contract in place; returns nil when the
// rule did not fire.
type postCompileRule func(rm types.RequestModel, contract *types.AnswerContract) *Observation

// preCompileRules is the registered set of rules that run BEFORE
// compiler.Compile. Phase 1.1 leaves this empty; Phase 2/3 append.
var preCompileRules = []preCompileRule{}

// postCompileRules is the registered set of rules that run AFTER
// compiler.Compile (i.e. once AnswerContract is populated). Phase
// 1.1 leaves this empty; Phase 4 appends.
var postCompileRules = []postCompileRule{}

// Amplify augments rm by running every pre-compile rule in
// registration order. Returns the augmented copy and the list of
// firings (one per rule that mutated a slot). When no rule fires,
// the returned RequestModel is byte-identical to the input and the
// observations slice is nil.
//
// The function is pure: rm is passed by value, the working copy is
// returned by value, and rules see the working copy in registration
// order so a later rule can read an earlier rule's writes.
func Amplify(rm types.RequestModel) (types.RequestModel, []Observation) {
	out := rm
	var obs []Observation
	for _, rule := range preCompileRules {
		if firing := rule(rm, &out); firing != nil {
			obs = append(obs, *firing)
		}
	}
	return out, obs
}

// AmplifyPostCompile runs every post-compile rule against the
// supplied AnswerContract. Mutates *contract in place. Returns the
// list of firings; nil when no rule fired.
//
// The agent integration site calls this AFTER compiler.Compile and
// compiler.RecomputeBudget so contract.MustInclude / Diagram / etc.
// are already populated by the template; Phase 4 R3 then pins
// typed identifiers that the template did not surface.
func AmplifyPostCompile(rm types.RequestModel, contract *types.AnswerContract) []Observation {
	if contract == nil {
		return nil
	}
	var obs []Observation
	for _, rule := range postCompileRules {
		if firing := rule(rm, contract); firing != nil {
			obs = append(obs, *firing)
		}
	}
	return obs
}
