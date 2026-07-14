// Package tracefence is the SINGLE SOURCE for the trace causal-projection
// fence recognition contract shared by the fence generator (internal/tool,
// runtimeTraceProjTreeFence) and the HTML preview classifier
// (internal/preview, isTraceCausalProjectionFence).
//
// Causal tree v5 P0 (design docs/design/causal_tree_v5_design_20260711.md
// §C.3 / F 重-3 / F 备-2, user ruling 2026-07-11; ledger
// docs/design/real_trace_campaign_20260705.md §29.38):
//
//   - The fence opener carries a TYPED second info token
//     (```text trace-causal-projection). The first token stays "text" so
//     glamour / terminal / third-party markdown viewers keep byte-identical
//     behavior (goldmark's FencedCodeBlock.Language and glamour's chroma lane
//     both read only the first info word); ONLY the HTML preview classifier
//     consumes the second token, as a precise-signal hard gate.
//   - The former content sniffing (⊚ head arm / ⊘ banner arm / bar-scale
//     signature) is DEMOTED to a legacy fallback that classifies archived
//     reports rendered before this token existed. Its whitelist derives from
//     the constants below — the same constants the generator emits — so the
//     generator-emittable head-form set and the fallback-recognized set can
//     never drift apart (completeness sentinel:
//     TestUXG0FenceHeadsClassifiedByPreview census arm in internal/tool).
//
// EVOLUTION RECORD (v5 P0, 2026-07-11): these strings previously lived as
// inline literals in internal/tool (runtimeTraceProjFlatFallbackHeader /
// runtimeTraceProjTreeHeaderLabel / runtimeTraceProjScaleNote /
// runtimeTraceProjRootGlyph) with a hand-mirrored copy in internal/preview
// (UXG-0 D1 noted "UXG-1 will single-source this phrase set"). The v5 P0
// batch is that single-sourcing. Every string is verbatim the pre-P0 form:
// fence CONTENT lines are byte-identical; only the opener line changed.
package tracefence

// InfoToken is the typed second fence-info token minted by the generator and
// matched EXACTLY (case-sensitive equality) by the preview classifier. It is
// the precise-signal hard gate that replaced content sniffing (v5 重-3).
const InfoToken = "trace-causal-projection"

// Info is the full fence info string ("text" first so every non-HTML face is
// untouched; InfoToken second for the preview classifier).
const Info = "text " + InfoToken

// Opener is the complete fence opener line body (without the trailing
// newline) written by runtimeTraceProjTreeFence.
const Opener = "```" + Info

// RootGlyph is the target-anchored tree's root mark (PTV8-RCR-A §24.3:
// 🎯 → ⊚, single-cell text presentation, EAW-Neutral).
const RootGlyph = "⊚"

// ELIM-1 「◎ 窗内可消除量总览」 fence contract (rank_order_v2_design_20260712.md
// §2.1, GREENLIT 2026-07-12; RANK-U Stage 2, 2026-07-13). The overview is its
// own fence rendered directly AFTER the projection tree fence and BEFORE the
// detail table; it carries a DISTINCT typed second info token so
//
//   - the preview classifier grid-renders it exactly like the tree
//     (precise-signal hard gate, zero content sniffing — a brand-new form has
//     no archives, so no legacy fallback arm exists for it), and
//   - the E# anchor transformer can EXCLUDE it from the fence↔detail/evidence
//     pairing census (it borrows its sibling tree fence's pairing instead) —
//     a second trace-causal-projection token would break the count identity
//     and silently kill every anchor link in the document.
//
// ElimGlyph closed-set audit (design O-2 「tracefence 闭集查重后定稿」,
// recorded): U+25CE BULLSEYE is EAW-Ambiguous — the same width class as the
// ◇/▒ stanza FIELD marks it joins (line-head field mark, deliberately NOT a
// StateMarks envelope slot, UXG-0 D3 family). Optical neighbors checked: the
// root ⊚ (U+229A, small inner ring, EAW-N) renders visibly smaller/lighter
// than the double-ringed ◎ and the two never appear inside one fence (◎ is
// the overview's own head; ⊚ heads the tree fence); no collision with
// ◌/◦/⊘/⊗.
const (
	ElimInfoToken = "trace-elim-overview"
	ElimInfo      = "text " + ElimInfoToken
	ElimOpener    = "```" + ElimInfo
	ElimGlyph     = "◎"
)

// Target-head provenance chips (§7.30 C4a R2): the four closed-set suffixes a
// ⊚ header line carries after the target name.
const (
	TargetChipFocusedZH = "‹用户关注线程›"
	TargetChipAnchorZH  = "‹分析锚点线程›"
	TargetChipFocusedEN = "<user-focused thread>"
	TargetChipAnchorEN  = "<analysis anchor thread>"
)

// TargetProvenanceChips returns the closed set of ⊚ header provenance chips.
func TargetProvenanceChips() []string {
	return []string{TargetChipFocusedZH, TargetChipAnchorZH, TargetChipFocusedEN, TargetChipAnchorEN}
}

// Flat-fallback banner heads (UXR-1 §29.36①, unified typed form
// 「⊘ <短结论>(<短因>)」; the opaque fallback carries no parenthetical —
// absence never invents a cause). Two typed causes plus the opaque fallback,
// zh/en each. These are COMPLETE constant strings: the generator emits them
// verbatim as the fence's first line (followed by the scale note).
const (
	FlatHeadMissingWakeupZH = "⊘ 唤醒链无法上溯(窗内无唤醒记录)"
	FlatHeadMissingWakeupEN = "⊘ wakeup chain not traceable (no sched_wakeup record in the window)"
	FlatHeadNotDrilledZH    = "⊘ 唤醒链未下钻(本报告未运行 wakeup_chain,可追问补齐)"
	FlatHeadNotDrilledEN    = "⊘ wakeup chain not drilled (wakeup_chain was not run for this report; ask a follow-up to fill it in)"
	FlatHeadUnresolvedZH    = "⊘ 唤醒链路径未解析"
	FlatHeadUnresolvedEN    = "⊘ wakeup path unresolved"
)

// FlatFallbackHeads returns the closed set of generator-emittable ⊘ banner
// heads (the legacy-fallback whitelist source for current-era archives).
func FlatFallbackHeads() []string {
	return []string{
		FlatHeadMissingWakeupZH, FlatHeadMissingWakeupEN,
		FlatHeadNotDrilledZH, FlatHeadNotDrilledEN,
		FlatHeadUnresolvedZH, FlatHeadUnresolvedEN,
	}
}

// Bar-scale declaration markers: the second half of the legacy recognition
// signature. Every generator scale note (window mode AND fallback scale)
// contains exactly one of these (runtimeTraceProjScaleNote).
const (
	ScaleMarkZH = "满格="
	ScaleMarkEN = "bar full ="
)

// ScaleNoteMarkers returns the closed set of scale-declaration markers.
func ScaleNoteMarkers() []string {
	return []string{ScaleMarkZH, ScaleMarkEN}
}
