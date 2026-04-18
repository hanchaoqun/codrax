// Package axis implements the PredicateAxis × AnchorKind affinity
// matrix used by the evidence ranker to bias evidence items whose
// AnchorKind matches the user's question axis.
//
// The matrix is pure enum × enum — no project-specific identifiers,
// no language-specific tokens. Questions carry a PredicateAxis
// (extracted by analyzer_predicate.go from verb cues); evidence
// items carry an AnchorKind (emitted by the LLM via emit_evidence).
// When the two align ("how does X CALL Y" + AnchorKind=call), the
// item is boosted; when they misalign ("how does X CALL Y" +
// AnchorKind=definition), the item is demoted.
//
// Design principle: sparse + conservative. Only non-neutral
// weights are listed. Any (axis, kind) pair absent from the table
// scores 1.0 (neutral). AxisUnknown disables the boost entirely.
// Weights are multiplicative and compose with the subject-aware
// boost (alpha=2.0) and the base entity-relevance score.
package axis

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// Affinity returns the multiplicative weight for combining a
// PredicateAxis with an AnchorKind. w > 1 boosts, w < 1 demotes,
// w == 1 is neutral. When axis == AxisUnknown, returns 1.0 for
// every kind (no-op). When kind is unset or unrecognised, returns
// 1.0 (we never demote an item just because its anchor was not
// emitted).
func Affinity(axis types.PredicateAxis, kind types.AnchorKind) float64 {
	if axis == types.AxisUnknown || kind == "" {
		return 1.0
	}
	row, ok := affinityMatrix[axis]
	if !ok {
		return 1.0
	}
	w, ok := row[kind]
	if !ok {
		return 1.0
	}
	return w
}

// affinityMatrix is the hand-curated weight table. Rows are the
// user's question axis; columns are the evidence item's anchor
// kind. Only non-neutral weights are listed.
//
// Curation rules:
//   - Weight > 1 for items whose anchor kind directly realises the
//     axis (e.g. AxisCall × AnchorCall = 1.5).
//   - Weight < 1 for items whose anchor kind is a neighbouring but
//     less-targeted surface (e.g. AxisCall × AnchorDefinition = 0.7
//     because "X defined" is auxiliary to "X calls Y"). Demotion
//     is bounded at 0.7 to keep the top-N from starving in cases
//     where the LLM only emitted definition anchors.
//   - Weight == 1 (absent entries) is the default. Never demote
//     below 0.7; never boost above 1.6.
//
// Add a cell only when a failing eval surfaces its absence. Do
// not pre-populate speculative entries — each non-neutral weight
// is a claim that future ranker outputs will be reordered.
var affinityMatrix = map[types.PredicateAxis]map[types.AnchorKind]float64{
	types.AxisCall: {
		types.AnchorCall:       1.5,
		types.AnchorDefinition: 0.7,
		types.AnchorAssignment: 0.9,
		types.AnchorReturn:     1.1,
	},
	types.AxisRegister: {
		// A registration pattern is typically a call site
		// (Register(...)) whose effect is an assignment into a
		// registry map. Both anchors are boosted; definitions
		// of the registered type are auxiliary.
		types.AnchorCall:       1.2,
		types.AnchorAssignment: 1.4,
		types.AnchorDefinition: 0.8,
	},
	types.AxisDefine: {
		types.AnchorDefinition: 1.5,
		types.AnchorCall:       0.8,
	},
	types.AxisReturn: {
		types.AnchorReturn:     1.6,
		types.AnchorDefinition: 0.9,
	},
	types.AxisConfigure: {
		types.AnchorAssignment: 1.4,
		types.AnchorImport:     1.1,
		types.AnchorDefinition: 0.9,
	},
	types.AxisCondition: {
		types.AnchorCondition: 1.5,
		types.AnchorCall:      0.9,
	},
	types.AxisImplement: {
		types.AnchorDefinition: 1.3,
		types.AnchorCall:       1.1,
	},
}
