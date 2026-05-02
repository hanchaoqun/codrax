// Package types — axis_anchor_map.go
//
// Block 2 of the architecture overhaul (2026-05-02): translate the
// LLM-emitted PredicateAxis (the action-direction axis of the user's
// question) into the AnchorKind values an evidence item must carry
// to count as "the axis was actually traced". Used by the
// PredicateAxisOracle in internal/orchestrator/contract_check.go to
// detect mis-classifications where the analyzer declared an axis but
// the explorer/extractor never produced anchor evidence of that
// shape.
//
// One axis can map to multiple AnchorKinds because a single user
// intent surfaces in different code shapes:
//
//   - "X registers Y" can be a function call to a registry method
//     (AxisCall-shape evidence) OR a map literal assignment
//     (AxisAssignment-shape evidence). Both are acceptable proof.
//   - "configure X" usually surfaces as an assignment to a config
//     struct (Assignment) but can also be a defaults function
//     (Definition).
//
// Returning a SET avoids the false-positive of insisting on one
// canonical shape — the LLM's evidence emission is shape-agnostic
// when it cites a real anchor, and the system should accept any
// member of the axis's allowed set.
//
// Empty result (axis = AxisUnknown OR an axis without registered
// anchor kinds — none today) means "no oracle should fire" — the
// caller MUST treat empty as "axis check is not applicable" and
// skip the violation.
//
// Red line: this map is purely structural — no question-text
// matching, no keyword tables. Inputs are typed enums LLM emitted
// via emit_analysis.predicate_axis. Adding a new PredicateAxis
// constant in analysis_ir.go MUST add a row here too; the test
// suite in internal/orchestrator/contract_check_intent_oracle_test.go
// pins the structural completeness.

package types

// PredicateAxisToAnchorKinds returns the set of AnchorKind values
// an evidence item may carry to count as proof of the named
// PredicateAxis. Returns nil for AxisUnknown or any unrecognised
// axis — callers MUST treat nil as "no oracle applicable".
//
// Sets are deliberately TIGHT: the second-choice anchor for an
// axis must be a real, common code shape (e.g. registration via
// map assignment), not every plausible anchor kind. Loose sets
// destroy the oracle's signal/noise ratio.
func PredicateAxisToAnchorKinds(axis PredicateAxis) []AnchorKind {
	switch axis {
	case AxisCall:
		return []AnchorKind{AnchorCall}
	case AxisRegister:
		// Registration surfaces as either:
		//   - a method call to a registry: registry.Register("x", h)
		//     → AnchorCall on the registry method
		//   - a map literal entry: handlers["x"] = h
		//     → AnchorAssignment on the map index
		// Both are legitimate registration evidence.
		return []AnchorKind{AnchorCall, AnchorAssignment}
	case AxisDefine:
		return []AnchorKind{AnchorDefinition}
	case AxisReturn:
		return []AnchorKind{AnchorReturn}
	case AxisConfigure:
		// Configuration surfaces as either:
		//   - an assignment: cfg.MaxConn = 10 (Assignment)
		//   - a defaults function: func defaultCfg() Cfg{...} (Definition)
		// Both are legitimate configure-axis evidence.
		return []AnchorKind{AnchorAssignment, AnchorDefinition}
	case AxisCondition:
		return []AnchorKind{AnchorCondition}
	case AxisImplement:
		// Implementation is a definition — `func (T) Method()...`
		// or `class T { method() {...} }`. Both flatten to
		// AnchorDefinition.
		return []AnchorKind{AnchorDefinition}
	}
	return nil
}

// PredicateAxisHasMatchingAnchor reports whether the supplied
// EvidenceItem's AnchorKind belongs to the PredicateAxis's
// allowed-kind set. False when axis is AxisUnknown OR the item's
// AnchorKind is unset OR the kinds don't intersect. Helper for the
// PredicateAxisOracle's per-evidence loop.
func PredicateAxisHasMatchingAnchor(axis PredicateAxis, item EvidenceItem) bool {
	if axis == AxisUnknown || item.AnchorKind == "" {
		return false
	}
	allowed := PredicateAxisToAnchorKinds(axis)
	for _, k := range allowed {
		if k == item.AnchorKind {
			return true
		}
	}
	return false
}
