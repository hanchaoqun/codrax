package types

// AnswerBlockKind classifies an AnswerDocumentV2 block (B3 落地). The
// closed enum mirrors the user's block-only carrier proposal §4.2 +
// §7.4 which lists the structural shapes a block-native answer can
// carry. A V2 answer composes from one or more blocks each tagged
// with one of these kinds; there is no top-level "shape" decision —
// the block kinds + their FacetIDs + ClaimUses tell the renderer +
// validator what the block represents.
//
// Mapping intent (B2 编译规则的目标 — 这里 only 定义闭枚举):
//
//	BlockSummary     — opening / lead-in prose paragraph(s); every
//	                   family carries at least one
//	BlockSection     — sub-headed body section for explanation /
//	                   architecture answers; indexed by Title
//	BlockOrderedList — sequence of items with positional meaning
//	                   (root cause hops, precedence layers, call-
//	                   chain links, principal step list)
//	BlockBulletList  — unordered enumeration (categories, options,
//	                   enumerated members where order is irrelevant)
//	BlockScalar      — single-literal answer (config value, count,
//	                   identifier, return value)
//	BlockDecision    — boolean / yes-no answer with rationale
//	BlockTable       — multi-column structured grid (precedence
//	                   layer-by-key, comparison, enumeration with
//	                   multiple attributes)
//	BlockDiagram     — mermaid / text diagram explicitly carried as
//	                   its own block so node-edge claim_uses can be
//	                   validated independently of surrounding prose
//	BlockCaveat      — trailing scope-warning or out-of-band note
//	                   (drift markers, exception cases, log-source
//	                   provenance disclosure)
//
// Adding a new value requires updating AllAnswerBlockKinds() and any
// V2 validator that switches on Kind. The structural test
// TestAllAnswerBlockKindsCovered enforces both surfaces.
type AnswerBlockKind string

const (
	BlockSummary     AnswerBlockKind = "summary"
	BlockSection     AnswerBlockKind = "section"
	BlockOrderedList AnswerBlockKind = "ordered_list"
	BlockBulletList  AnswerBlockKind = "bullet_list"
	BlockScalar      AnswerBlockKind = "scalar"
	BlockDecision    AnswerBlockKind = "decision"
	BlockTable       AnswerBlockKind = "table"
	BlockDiagram     AnswerBlockKind = "diagram"
	BlockCaveat      AnswerBlockKind = "caveat"
)

// AllAnswerBlockKinds returns every declared AnswerBlockKind in a
// stable enumeration order. Mirrors the AllAnchorKinds /
// AllViolationKinds pattern (see internal/types/anchor_kind.go and
// internal/types/violation.go) so structural tests and downstream
// switches can iterate the canonical set without re-listing it.
//
// The order is the human-readable order used in docs/migration/
// block_only_carrier.md §5.1 B1-T1 task description; downstream
// callers MUST treat the order as informational, not load-bearing.
func AllAnswerBlockKinds() []AnswerBlockKind {
	return []AnswerBlockKind{
		BlockSummary,
		BlockSection,
		BlockOrderedList,
		BlockBulletList,
		BlockScalar,
		BlockDecision,
		BlockTable,
		BlockDiagram,
		BlockCaveat,
	}
}

// IsValidAnswerBlockKind reports whether k is one of the declared
// AnswerBlockKind values. The empty string is considered invalid so
// schema validators can fail-loud on unset / corrupted Kind values.
func IsValidAnswerBlockKind(k AnswerBlockKind) bool {
	for _, declared := range AllAnswerBlockKinds() {
		if k == declared {
			return true
		}
	}
	return false
}

// SurfaceRole is re-exported here as a doc-comment cross-reference
// for B1 readers — the actual type lives in
// internal/types/rendered_claim_use.go (SurfacePrincipal). V2 blocks
// carry SurfaceRole to tell validators whether the block is
// principal answer content; empty means "not principal".
