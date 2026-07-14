package tracefence

// display_tables.go — UXG-1 M1 (ledger docs/design/real_trace_campaign_20260705.md
// §29.38 收敛机制 M1-M7; matrix spec info_contract_report.md §④): the remaining
// FOUR hand-copied display-token tables between the fence generator
// (internal/tool) and the HTML preview classifier (internal/preview) move into
// this leaf package, alongside the P0 fence-head table in tracefence.go.
// Both sides consume THESE constants. Enforcement (exact shape, 修复轮 F2):
// internal/preview non-test sources carry ZERO table literals (hard grep
// tripwire, uxg1_singlesource_test.go); internal/tool FUNCTIONAL emitters
// (chips, wrap atoms, action cells, column headers, H2 titles, aux markers,
// the LLM-face position word) derive from the constants, while residual
// PROSE/legend mentions are pinned per file × literal in a reasoned
// allowlist (TestUXG1ToolAuthorityKeepsNoUncountedTableLiteral); engine-
// minted render pins: internal/tool/answer_document_projection_uxg1_test.go.
//
// EVOLUTION RECORD (UXG-1, 2026-07-12) — the retired hand copies:
//   - state-mark directory: internal/preview/markdown.go
//     traceProjectionIconClass hand-switch ↔ internal/tool §24.3 form-table
//     glyph column + state-icon literals (UXG-0 D1 过程根因: five closed-set
//     tables hand-copied because preview must not import tool);
//   - action tokens: preview traceProjectionActionToken {优化点, optimization
//     point} ↔ tool action-word / mention-floor emitters;
//   - seat-channel phrases + rank badges: preview traceProjectionRankToken
//     (根因排序/邻近影响/root-cause rank/adjacent-impact + ❶..❺) ↔ tool
//     runtimeTraceProjSeatChannelWord / runtimeTraceProjBadgeGlyph — the
//     "last hand copy" UXG-0 D2 flagged;
//   - generated section headings: preview traceAuditHeadingClass ↔ tool
//     renderV2ListOrTableHeading title emitters;
//   - aux-reference markers: preview auxRefMarkers ↔ tool 树读法:/各列口径:
//     emitters (§29.9).
// Every string below is verbatim the pre-UXG-1 byte form: user-facing output
// is byte-identical; only the ownership moved.

// --- Table ① state-mark directory (§24.3 glyph column + PTV7 state icons +
// UXG-0 D3) -------------------------------------------------------------------
//
// One-cell state/form marks the generator emits at row heads inside the
// causal-projection fence. The preview HTML face gives each mark a 2ch
// envelope slot (v5 P0 重-1) keyed on Class; the tool §24.3 form table and the
// state-icon emitters take their glyph BYTES from the Glyph* constants.
//
// Deliberately NOT in this directory (UXG-0 D3 rulings, 2026-07-11):
//
//	⚠ — rides INSIDE composed word tokens (⚠实际Xms, ⚠跨窗); boxing it would
//	    split a word into cell fragments mid-token.
//	◇ — line-head stanza FIELD mark (◇ 邻近区段 header + legend), not a
//	    one-cell state icon slot.
//	▒ — line-head stanza field mark AND the background bar-fill character
//	    (▒▒▒░░… runs); boxing would restyle bar ink.
const (
	// RootGlyph "⊚" lives in tracefence.go (v5 P0).
	GlyphSleep          = "☾"
	GlyphRunnable       = "⧖"
	GlyphRunning        = "⚙"
	GlyphIOChain        = "⛓" // chain-channel IO/D-state family (UXR-1 §29.36②)
	GlyphOffChainDState = "⧗" // off-chain (◇/▒) D-state/IO family (UXR-1 §29.36②)
	GlyphLock           = "⊗"
	GlyphUndrillable    = "⊘" // 链止 keep-mark + flat banner head (UXG-0 D3)
	GlyphInversion      = "⇅"
	GlyphOptimization   = "✦"
	GlyphInterrupt      = "↯"
	GlyphBlindSpot      = "◌"
	GlyphNeutral        = "◦" // 中转 / 无主导态 (§24.3; the binder-wait borrow ended with ⋈)
	// GlyphBinderWait — P2a rider 件3 (ledger §29.58.2 binder 借用义专属字形
	// 裁定, 2026-07-13; the FOURTH split precedent after T4/∿/⋯-withdrawn):
	// the self/chain binder IPC-wait rows leave the ◦ neutral borrow for a
	// dedicated mark. Chosen by the ∿ four-criteria audit (census §3 first
	// choice): U+22C8 BOWTIE is EAW=N (width 1 in BOTH east-asian contexts),
	// single rune / no VS16, Mathematical Operators block (same coverage
	// argument as ∿), two-parties-meeting reads as the IPC rendezvous, and
	// zero optical collision with the existing ◌/◦/⊚/⊘/⊗ circle marks.
	GlyphBinderWait = "⋈"
	// GlyphSubordinate — P2a rider 件2b (ledger §29.58.1 b, 2026-07-13): the
	// subordinate-component connector `↳` (v5 design 双行 ↳ 先例, causal_tree
	// v5 §C.2 重-2) worn by a component row placed directly under its owning
	// seat (self binder ⊂ sleep carve) and by the deterministic optimization
	// table's member/fold cells (§29.58.2 F4). Width note: U+21B3 is
	// EAW-AMBIGUOUS (width 2 under an east-asian terminal condition) — the
	// sanctioned treatment is the ⇅/⛓/❶ precedent recorded in the tool width
	// pin (both generator and preview measure through the same go-runewidth
	// condition, so grids stay self-consistent; census F6 record).
	GlyphSubordinate = "↳"
	// GlyphPacing — CAL-1 件⑥b (2026-07-12; 修复轮 P3-3 勘正): the
	// cadence-idle row mark (pacing_idle / periodic_idle independent rows,
	// 件⑤ PACE-ROW). Chosen by mechanical audit: U+223F SINE WAVE is width 1
	// in BOTH east-asian contexts (non-ambiguous EAW). Honesty note: SOME
	// circle candidates are EAW-ambiguous (○ U+25CB, ⊙ U+2299) but others
	// are dual-context width-1 (◉ U+25C9 among them) — the circle family was
	// rejected on the OTHER three criteria, not on width alone: symbol-font
	// coverage, 13px legibility, and optical collision with the existing
	// ◌/◦/⊚ circle marks in this directory. ∿ additionally reads as a
	// periodic waveform (the row's semantics) and sits in the well-covered
	// Mathematical Operators block.
	GlyphPacing = "∿"
)

// StateMark is one row of the state-mark directory: the glyph byte plus the
// preview HTML face's CSS class token (trace-icon-<Class>).
type StateMark struct {
	Glyph string
	Class string
}

// StateMarks returns the closed state-mark directory. Order is stable
// (directory order = documentation order); ⛓ and ⧗ share the "io" class by
// design (same optical family, UXR-1 §29.36②).
func StateMarks() []StateMark {
	return []StateMark{
		{RootGlyph, "root"},
		{GlyphSleep, "sleep"},
		{GlyphRunnable, "runnable"},
		{GlyphRunning, "running"},
		{GlyphIOChain, "io"},
		{GlyphOffChainDState, "io"},
		{GlyphLock, "lock"},
		{GlyphUndrillable, "undrillable"},
		{GlyphInversion, "inversion"},
		{GlyphOptimization, "optimization"},
		{GlyphInterrupt, "interrupt"},
		{GlyphBlindSpot, "blind"},
		{GlyphNeutral, "neutral"},
		{GlyphPacing, "pacing"},
		{GlyphBinderWait, "binder"},
		{GlyphSubordinate, "sub"},
	}
}

// StateMarkClass resolves a rune to its directory class ("" = not a state
// mark; the caller keeps the rune on its ordinary grid path).
func StateMarkClass(r rune) string {
	for _, mark := range StateMarks() {
		if []rune(mark.Glyph)[0] == r {
			return mark.Class
		}
	}
	return ""
}

// --- Table ② action tokens ---------------------------------------------------
//
// The generator's actionable optimization word inside a causal tree: the ✦
// row action tail (zh) and the §29.36.3 mention-floor word head (zh/en). The
// preview HTML face emphasizes exactly these tokens at their original grid
// width.
const (
	ActionWordZH = "优化点"
	ActionWordEN = "optimization point"
	// ActionWordENShort is the EN action TAIL word on tree rows and metric
	// cells ("· optimize" / "optimize·<class>") — table-② family member on
	// the word-source side (UXG-1 修复轮 F5, 2026-07-12), deliberately NOT in
	// the preview emphasis closed set below: (a) restyling the bare tail word
	// would be an un-ruled display change, and (b) "optimize" is a byte
	// prefix of "optimization point" — admitting it to the prefix-matched
	// emphasis scan would shadow the longer token mid-word.
	ActionWordENShort = "optimize"
)

// ActionTokens returns the closed action-token set the preview face may
// emphasize (ActionWordENShort excluded — see its waiver note above).
func ActionTokens() []string {
	return []string{ActionWordZH, ActionWordEN}
}

// --- Table ③ seat-channel phrases + rank badges (§29.36.2 / §29.27.1) ---------
//
// The channel-worded ordinal chip phrases (chip = word+#N zh, word+" #N" en;
// 通道3 背景 has NO ordinal by ruling) and the ❶..❺ TOP-5 badge family.
// runtimeTraceProjSeatChannelWord (internal/tool) is the phrase CONSUMER on
// the generator side; traceProjectionRankToken (internal/preview) is the
// classifier consumer. UXG-0 D2 called the preview copy "the phrase set's
// third hand copy and the last one allowed" — this table is that
// single-sourcing.
const (
	SeatChannelChainZH    = "根因排序"
	SeatChannelChainEN    = "root-cause rank"
	SeatChannelAdjacentZH = "邻近影响"
	SeatChannelAdjacentEN = "adjacent-impact"
)

// BadgeGlyphs returns the ❶..❺ TOP-5 badge family in seat order (index 0 =
// seat 1). §29.27.1: badges are seats 1..5 only, one U+2776.. dingbat block.
func BadgeGlyphs() []string {
	return []string{"❶", "❷", "❸", "❹", "❺"}
}

// --- Table ④ generated section headings ---------------------------------------
//
// The three deterministic H2 chapters the runtime-trace report generates; the
// preview face recognizes them as an EXACT closed set (optionally followed by
// the generator's " — <artifact>" suffix) to hang presentation wrappers on.
const (
	SectionOptimizationZH = "确定性优化点"
	SectionOptimizationEN = "Deterministic Optimization Points"
	SectionDetailZH       = "因果投影明细(逐节点完整属性)"
	SectionDetailEN       = "Causal Projection Detail (full attributes per node)"
	SectionEvidenceZH     = "证据索引"
	SectionEvidenceEN     = "Evidence Index"
	// SectionProjectionZH/EN — UX-ANCHOR 件a (§29.61.7, 2026-07-14): the
	// projection LEAD section's H2 title joins the table-④ single source. The
	// preview lead-segment decorator (markdown_trace_lead.go) uses it as the
	// lead-scope boundary heading (E# anchor links / compact ❶..❺ badges
	// decorate ONLY between this heading and its following projection fence);
	// the tool-side title emitters (runtimeTraceCausalProjectionTitle + the
	// 对比总览/分区边界/覆盖边界 composed titles) derive from the same bytes.
	SectionProjectionZH = "Trace 因果投影"
	SectionProjectionEN = "Trace Causal Projection"
)

// SectionOptimizationTitles / SectionDetailTitles / SectionEvidenceTitles
// return the zh/en closed heading sets.
func SectionOptimizationTitles() []string {
	return []string{SectionOptimizationZH, SectionOptimizationEN}
}

func SectionDetailTitles() []string {
	return []string{SectionDetailZH, SectionDetailEN}
}

func SectionEvidenceTitles() []string {
	return []string{SectionEvidenceZH, SectionEvidenceEN}
}

// SectionProjectionTitles returns the projection lead-section zh/en closed
// heading set (UX-ANCHOR 件a lead-scope boundary).
func SectionProjectionTitles() []string {
	return []string{SectionProjectionZH, SectionProjectionEN}
}

// --- Table ⑥ WF-xn merge-count family words (§29.52.1, 2026-07-12) -------------
//
// The ×N semantic-overload dissolution vocabulary (裁定 §29.52.1: the same
// ×N marker carried two meanings — instance-merge count vs thread count —
// and 「×」 read as multiplication, 「–」 as minus, in arithmetic-dense
// reports). The zh data tokens now spell the count meaning out:
//
//	instance/sum form   N次(a~b)          (en n=N(a~b))
//	same-value form     N次同值           (en n=N same-value)
//	cross-thread max    N线程取最大(单项a~b) (en N-thread max(each a~b))
//	cross-window max    N次跨窗取最大(单项a~b) (en n=N cross-window max(each a~b))
//	union form          N次(a~b)union     (en n=N(a~b)union)
//
// Existing caliber words (合计(共N段,同线程) / 单次最大(a~b,共N次) …) stay
// untouched by ruling. The words below are the zh family HEADS that fuse
// with their LEADING count digits into ONE wrap atom on the tool side
// ("4次(…)" / "6线程取最大(…)" never break between the count and its word).
// Longest-first inside the shared 次 prefix (matcher returns first hit).
const (
	MergeCountCrossWindowMaxZH = "次跨窗取最大"
	MergeCountSameValueZH      = "次同值"
	MergeCountUnionZH          = "次union"
	MergeCountThreadMaxZH      = "线程取最大"
	MergeCountOccurrenceZH     = "次"
)

// MergeCountWrapAtoms returns the zh merge-count family word heads in
// matcher order (longest-first within the shared 次 prefix).
func MergeCountWrapAtoms() []string {
	return []string{
		MergeCountCrossWindowMaxZH,
		MergeCountSameValueZH,
		MergeCountUnionZH,
		MergeCountThreadMaxZH,
		MergeCountOccurrenceZH,
	}
}

// --- Table ⑤ aux-reference markers (§29.9) -------------------------------------
//
// The two auxiliary reference-block marker paragraphs the generator emits
// (ASCII colon — the engine-actual byte form, hex-verified against customer
// specimens). The preview face additionally accepts a fullwidth-colon variant
// because the §29.9 ruling text writes the markers with "：" — that variant is
// DERIVED (preview side), never a second table entry. Do NOT add other marker
// words without a new user ruling (闭集只这两个起步).
const (
	AuxTreeLegendMarker     = "树读法:"
	AuxColumnGlossaryMarker = "各列口径:"
)

// AuxRefMarkers returns the generator-emitted marker closed set.
func AuxRefMarkers() []string {
	return []string{AuxTreeLegendMarker, AuxColumnGlossaryMarker}
}
