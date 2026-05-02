package agent

import "github.com/hanchaoqun/codrax/internal/types"

// concrete_value_anchor_kind.go — projects each
// concreteValueEntry.kind value into the typed AnchorKind axis.
//
// extractConcreteValues + the language-aware scanners in
// concrete_values_lang.go emit 11 distinct kind tags, but the
// EvidenceItem AnchorKind axis is the closed 6-value set declared
// in types/evidence.go (definition / call / condition / return /
// assignment / import). Without this projection, every
// concrete_values evidence item lands at AnchorKind="" and
// ClaimFormOf falls through to ClaimUnknown — Phase 0 trace data
// (docs/design/semantic_surface_contract_p0_trace.md §2)
// measured 34% global ClaimUnknown rate, with concrete_values the
// dominant source. With this projection the same 11 kinds resolve
// to a real AnchorKind, and Phase 4's facet-coverage / claim-form
// oracles can run inferred-coverage paths instead of degrading.
//
// Mapping is many-to-one (11 → 6) because the syntactic categories
// the extractor distinguishes (e.g. "binds" / "maps" / "config")
// share an underlying anchor shape (assignment-like) at the
// AnchorKind axis. The mapping is conservative: when a category
// has no clean fit in the 6-value set (e.g. "implements", which is
// closer to a structural relation than a single anchor), we map to
// the closest definition-style anchor — this errs on
// over-classification rather than dropping back to AnchorKind="",
// which is what the original "all ClaimUnknown" defect was.
//
// Returns AnchorKind="" only on truly unknown / blank input so the
// caller's ClaimFormOf still falls through to ClaimUnknown for
// genuine gaps; never for the 11 known kinds.
//
// Single source of truth — adding a new kind in
// concrete_values_lang.go MUST add an arm here too. The mapping
// table is unit-tested for exhaustive coverage in
// concrete_value_anchor_kind_test.go.
func concreteValueKindToAnchorKind(kind string) types.AnchorKind {
	switch kind {
	// Return-shape: literal returns + error returns surface the
	// callee's outcome at the return site.
	case "returns", "errors":
		return types.AnchorReturn

	// Call-shape: direct calls + binding calls (Register / Bind /
	// Wire) both anchor at a call expression.
	case "calls", "binds":
		return types.AnchorCall

	// Condition-shape: extracted guard expressions.
	case "conditional":
		return types.AnchorCondition

	// Assignment-shape: variable assignments + map literal entries
	// + config-leaf assignments all set a value at a key.
	case "assigns", "maps", "config":
		return types.AnchorAssignment

	// Definition-shape: structural relations the extractor
	// surfaces (decorator targets, struct embeds, interface
	// implementations) all anchor at a declaration site. AnchorKind
	// has no separate "decoration" / "embeds" / "implements" so
	// AnchorDefinition is the closest typed bucket.
	case "decorates", "embeds", "implements":
		return types.AnchorDefinition
	}

	// Unknown / blank — fall through to AnchorKind="" so
	// ClaimFormOf returns ClaimUnknown and the existing
	// fallback path applies.
	return ""
}
