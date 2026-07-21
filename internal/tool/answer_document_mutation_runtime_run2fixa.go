package tool

// answer_document_mutation_runtime_run2fixa.go — RUN2FIX-A 批 (§29.174
// RUN2AUDIT-1 处置②, customer runnable_2.txt P1 显示六件, 2026-07-20).
// Self-contained helpers for the batch's display fixes; the host files
// (answer_document_mutation_runtime_tree.go / _elim.go) carry minimal
// call-site hunks only (battlefield isolation, RULER2/PARTSPLIT precedent).
//
//   - 件4 UX-6: name-truncation priority reversal helpers (legible head-prefix
//     floor + keep-word shrink lane) — the grammar-word reservation used to
//     squeeze thread names to 1-2 runes ("c…-59566", runnable_2:286).
//   - 件5 UX-7: the wait-denominator two-ruler note beside a rendered
//     four-state account (§29.158 RULER2 isomorph) — runnable_2:130/:132
//     printed 144.503 (window ruler) and 149.263 (state-view-sum ruler) on
//     adjacent lines with zero reconciliation path.
//   - 件6 UX-8①: the self-lane inversion badge arm (每板 rank#N 席佩章对称) —
//     runnable_2:179 E1 (根因排序#1) sat bare while E2 (#1, another board)
//     wore ❶ — plus the headline/◎ caliber-divergence parenthetical
//     (锚点板#1 vs 具名节最大, HEADLINE 家族软披露).

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// --- 件2 复核 P2-1: roster-pointer-aware max-member attribution -------------

// runtimeTraceProjFoldRosterBareSubject strips the display-only B6 榜位
// pointer suffix (runtimeTraceProjAnnotateFoldRosterRankPointers mints exactly
// 「(见榜位#N)」 zh / " (see root-cause rank #N)" en, digits-only ordinal) off
// a roster string, returning the bare subject. 复核 P2-1 (对抗 F3+冷读 CR-1):
// the max-member attribution used to bare-HasPrefix the canonical strings,
// which both misattributed prefix-colliding names (max=app-951 naming
// app-9511) and existed only to tolerate this suffix — strip the KNOWN suffix
// at its exact boundary instead and compare canonically WHOLE. Non-pointer
// parentheses (thread names legitimately carry them) never match the
// glyph-exact boundary and pass through untouched.
func runtimeTraceProjFoldRosterBareSubject(entry string) string {
	if idx := strings.LastIndex(entry, "(见榜位#"); idx >= 0 &&
		runtimeTraceProjFoldRosterPointerOrdinal(entry[idx+len("(见榜位#"):], ")") {
		return entry[:idx]
	}
	enMark := " (see " + tracefence.SeatChannelChainEN + " #"
	if idx := strings.LastIndex(entry, enMark); idx >= 0 &&
		runtimeTraceProjFoldRosterPointerOrdinal(entry[idx+len(enMark):], ")") {
		return entry[:idx]
	}
	return entry
}

// runtimeTraceProjFoldRosterPointerOrdinal reports whether rest is one-or-more
// digits followed by exactly the closing mark at end-of-string — the only tail
// the pointer mint can produce.
func runtimeTraceProjFoldRosterPointerOrdinal(rest, close string) bool {
	body, ok := strings.CutSuffix(rest, close)
	if !ok || body == "" {
		return false
	}
	for _, r := range body {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// --- 件4: truncation priority reversal ---------------------------------------

// runtimeTraceProjNameHeadPrefixFloorCells is the minimum legible head-prefix
// width (display cells before the "…"): a name form below it ("c…-59566",
// "T…-60555") carries no recognizable identity. 禁 1-2 字符名形 (spec 件4).
const runtimeTraceProjNameHeadPrefixFloorCells = 6

// runtimeTraceProjNameHeadFloorWidth returns the minimum width the row's
// head name needs to stay legible: the whole head when it already fits inside
// the floor form, else prefix floor + "…" + the identity-bearing pid tail
// (kept whole — tid 恒全, the T2 discipline unchanged). Names without a pid
// tail floor at prefix + "…".
func runtimeTraceProjNameHeadFloorWidth(head string) int {
	full := runewidth.StringWidth(head)
	floor := runtimeTraceProjNameHeadPrefixFloorCells + 1
	if _, _, ok := runtimeTraceProjSplitNamePid(head); ok {
		if idx := strings.LastIndex(head, "-"); idx > 0 {
			floor = runtimeTraceProjNameHeadPrefixFloorCells + 1 + runewidth.StringWidth(head[idx:])
		}
	}
	if full < floor {
		return full
	}
	return floor
}

// runtimeTraceProjKeepStateWord (件4 截断策略反转 scope gate) resolves the
// 行1 keep suffix's SHRINKABLE state-phrase word: the " · "+word+chips form
// whose word is a STATE composition/state-family/inversion-family phrase
// (优先级反转·可运行等待 / D-state/iowait(原因未证) / iowait … — typed lanes:
// the inversion word-face family predicate + the state-token class, exactly
// the keep mint's own state-word arms). A contender whose keep word is a
// NON-state TYPE 词位 fails both arms and stays whole BY PRIOR RULING (RCM-2
// D2: 「块设备IO(inode) ×2」 must survive a name squeeze whole, the subject
// head mid-truncates instead) — the D2 shape keeps its legacy geometry
// byte-identically. ok=false → the caller leaves the legacy squeeze in place.
func runtimeTraceProjKeepStateWord(row runtimeTraceProjTreeRow, keep string, zh bool) (word, chips string, ok bool) {
	word, token := runtimeTraceProjRowCauseWordToken(row, zh)
	if word == "" || !strings.HasPrefix(keep, " · "+word) {
		return "", "", false
	}
	stateWord := runtimeTraceProjInversionFamilyNode(row.Node) ||
		runtimeTraceProjStateTokenClass(token) != ""
	if !stateWord {
		return "", "", false
	}
	return word, strings.TrimPrefix(keep, " · "+word), true
}

// runtimeTraceProjShrinkKeepStateWord (件4) rebuilds the keep suffix with the
// state-phrase word boundary-truncated to wordBudget: 超宽先截状态词短语
// (既有 … 机制)再截名 — the word survives in cut form ("优先级反转…"), never
// dropped whole (the §24.9 G2 stray-line lesson), and the count/dedup chips
// after it stay whole (grammar, never cut). ok=false when no boundary-aligned
// prefix fits or the cut would not shrink — the caller still holds the head
// floor and lets the row run past the shared column (PTS floor precedent).
func runtimeTraceProjShrinkKeepStateWord(word, chips string, wordBudget int) (string, bool) {
	if wordBudget >= runewidth.StringWidth(word) {
		return "", false // nothing to shrink — the word already fits
	}
	cut := runtimeTraceProjBoundaryTruncate(word, wordBudget)
	if cut == "" || runewidth.StringWidth(cut) >= runewidth.StringWidth(word) {
		return "", false
	}
	return " · " + cut + chips, true
}

// --- 件5: wait-denominator two-ruler note ------------------------------------

// runtimeTraceProjWaitDenomJitterMS reuses the shared state-boundary jitter
// tolerance for the ruler-divergence judgment (wording-only soft disclosure —
// 精确数值比较驱动词面,零硬门): below it the two rulers agree to boundary
// jitter and the note stays silent. 复核 F5 — this alias is NOT the
// 容差常量禁跨语义借用 shape (B-1, 2026-07-10 audit lesson): both sides of
// this comparison are the same semantic family — state-segment-derived
// wall-clock ms of the SAME thread at the same display precision, differing
// only in partition (four-state account vs self state-view row sum) — so the
// same boundary-jitter tolerance is the honest one; a future semantic
// divergence of either side must split the constant, not stretch it.
const runtimeTraceProjWaitDenomJitterMS = runtimeTraceProjSymptomOvershootJitterMS

// runtimeTraceProjWaitDenomRulerNote (件5, §29.158 RULER2 同构) renders the
// same-line ruler note for the 等待句 when a provable four-state account
// rendered ABOVE it and the wait denominator (the self state-view row sum,
// runtimeTraceProjTargetSymptomMS) measurably diverges from the account's own
// wait partition (runnable+sleep+D-state+io_wait): the two lines then carry
// two different rulers and must not be reconciled by subtraction
// (runnable_2:130 四态合计 144.503 vs :132 等待 149.263 — 149.263 exceeded
// the window with zero explanation; LEAD-1 filed the value as un-replayable
// on the carve, so this line is the 口径溯源+尺注 defense; the arithmetic
// final check rides the full-trace revisit). "" whenever the account is
// absent (legacy shapes byte-identical) or the rulers agree within jitter.
func runtimeTraceProjWaitDenomRulerNote(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, symptom float64, zh bool) string {
	account := runtimeTraceProjFourStateAccountProvable(projection, model)
	if account == nil || symptom <= 0 {
		return ""
	}
	accountWait := account.RunnableMS + account.SleepMS + account.DStateMS + account.IOWaitMS
	diff := symptom - accountWait
	if diff < 0 {
		diff = -diff
	}
	if diff <= runtimeTraceProjWaitDenomJitterMS {
		return ""
	}
	if zh {
		return "(按自身状态视图行合计尺,与上行四态尺不同尺,不可直接对账)"
	}
	return " (self state-view row-sum ruler — a different ruler from the four-state line above; not directly reconcilable)"
}

// --- 件6: self-lane inversion badge arm + headline caliber note --------------

// runtimeTraceProjSelfInversionSeatBadge (件6 对称性补洞, §29.27.1 徽章=已发布
// 席位序数的象形) admits a SelfRows-lane INVERSION seat to the badge lane:
// runnable_2 E1 (自身·优先级反转候选, engine 根因排序#1, tier=primary) sat
// bare because the §29.30.1 four-family closed set — a defense against
// stale/legacy sleep/binder/lock self rows WITHOUT their engine symptom tier
// — also caught the engine-published inversion seat. Precise arms: the shared
// seat arm, the chain ordinal channel, the typed inversion form, the ENGINE's
// own rank (node.Rank — resolver-only fold adoptions stay out), rank ≤ TOP-5.
//
// Badge lane ONLY (deliberate, spec 件6 + 旁件): the lead-election population
// (runtimeTraceProjRowValidSeat) is untouched — widening it would flip the
// crown from the anchor board's #1 onto this seat, and the batch's ruling is
// the symmetric badge plus the headline caliber note, never a crown change
// (crown-follow question delegated to A2).
//
// 复核 F6 — known residual (fold-twin shape): the arm requires the ENGINE's
// own node.Rank to equal the displayed ordinal, so an inversion seat whose
// TOP-5 ordinal arrives only via the folded rank-twin peer resolver
// (runtimeTraceProjCauseRankConfidence min-peer adoption) still sits bare —
// deliberately: adopting a resolver-minted ordinal here would badge a seat
// the engine never published at that rank. Settle rides the same A2
// crown-follow delegation (§29.174 处置②); until ruled, the residual shape
// keeps the honest bare form.
func runtimeTraceProjSelfInversionSeatBadge(row runtimeTraceProjTreeRow) (int, bool) {
	if row.Kind != runtimeTraceProjTreeRowSelf || !runtimeTraceProjRowSharedSeatArm(row) {
		return 0, false
	}
	if runtimeTraceProjRowOrdinalChannel(row) != runtimeTraceProjOrdinalChannelChain {
		return 0, false
	}
	if runtimeTraceProjImpactFormForNode(row.Node, row.Kind) != runtimeTraceProjImpactFormInversion {
		return 0, false
	}
	if row.Node.Rank < 1 || row.Node.Rank > runtimeTraceProjBadgeTopN {
		return 0, false
	}
	rank, _ := runtimeTraceProjCauseRankConfidence(row)
	if rank != row.Node.Rank {
		return 0, false // displayed ordinal must be the engine's own seat
	}
	return rank, true
}

// runtimeTraceProjHeadlineElimCaliberNote (件6 旁件, HEADLINE 家族软披露)
// renders the headline-tail parenthetical when the 主根因 seat and the ◎
// overview's FIRST section head take their numbers from two different seats
// (runnable_2:127 调度压力 22.993 = 锚点板#1 vs :142 锁与优先级 24.813 =
// ◎ 首节(具名节)最大 — two "top" numbers, zero reconciliation words). Identity
// comparison, never value coincidence: the ◎ first-section max entry row vs
// the crowned node (EvidenceID when both carry one, else canonical subject +
// printed eff). "" when the ◎ fence cannot render, has no chain section, or
// the crown IS the first-section top (the healthy shape, byte-identical).
func runtimeTraceProjHeadlineElimCaliberNote(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, primary *types.TraceCausalProjectionNode, zh bool) string {
	if primary == nil || !projection.RootCauseFamilyObserved {
		return ""
	}
	top, ok := runtimeTraceProjElimChainFirstSectionTop(model)
	if !ok {
		return ""
	}
	if runtimeTraceProjElimSameSeatIdentity(top.row.Node, *primary) {
		return ""
	}
	anchorBoard := primary.Rank == 1 &&
		runtimeTraceCausalProjectionCanonicalNode(primary.RankBoardTarget) != "" &&
		runtimeTraceCausalProjectionCanonicalNode(primary.RankBoardTarget) ==
			runtimeTraceCausalProjectionCanonicalNode(model.Target)
	// 复核 P2-2 (对抗 F2): the ◎ clause names what the trigger actually
	// compares — the FIRST rendered section's head (the named-section max,
	// since the unresolved/composite tail parks last), never a board-wide
	// claim the helper does not verify. Wording only; the trigger is untouched.
	if zh {
		if anchorBoard {
			return "(锚点板#1;◎ 按具名节最大)"
		}
		return "(主榜席;◎ 按具名节最大)"
	}
	if anchorBoard {
		return " (anchor-board #1; ◎ takes the named-section max)"
	}
	return " (lead-board seat; ◎ takes the named-section max)"
}

// runtimeTraceProjElimSameSeatIdentity is the note's typed seat-identity
// comparison: EvidenceID equality when both rows carry one, else canonical
// subject + %.3f printed effective value.
func runtimeTraceProjElimSameSeatIdentity(a, b types.TraceCausalProjectionNode) bool {
	aID, bID := strings.TrimSpace(a.EvidenceID), strings.TrimSpace(b.EvidenceID)
	if aID != "" && bID != "" {
		return runtimeTraceCausalProjectionCanonicalNode(aID) == runtimeTraceCausalProjectionCanonicalNode(bID)
	}
	return runtimeTraceCausalProjectionCanonicalNode(a.Subject) == runtimeTraceCausalProjectionCanonicalNode(b.Subject) &&
		fmt.Sprintf("%.3f", a.EffectiveImpactMS) == fmt.Sprintf("%.3f", b.EffectiveImpactMS)
}

// runtimeTraceProjElimChainFirstSectionTop resolves the ◎ chain block's FIRST
// section and returns its max-value entry. It reuses the fence's own member
// authorities (runtimeTraceProjElimBoard TOP-K slice +
// runtimeTraceProjElimSectionsFor) over the top-K chain members; the fence's
// two fallback appends (semantic / ◇) provably never change any section max —
// a fallback's eff is ≤ every TOP-K chain eff by the board's blocked order —
// so this reduced view answers "first section head value/owner" identically
// (单值源 via the two shared authorities; divergence would need a third
// implementation, which this helper deliberately is not).
//
// 复核 F7 — two recorded corners: (a) all-unresolved boards — when every
// chain member's direction is unresolved, the tail section IS sections[0],
// so "first section head" resolves to the unresolved tail's top; the typed
// seat-identity comparison stays exact (it compares seats, not words), but
// the note's 具名节 word would then name a board with zero named sections —
// a known wording corner, accepted: the ◎ fence's own first ▸ head is still
// precisely the seat compared against, so the pointer stays truthful even
// where the label noun is loose. (b) fallback-only boards — zero
// chain-channel entries leave sections empty and ok=false, so the note stays
// silent (the ◎ fence then renders only fallback rows with no ▸ heads to
// disagree with).
func runtimeTraceProjElimChainFirstSectionTop(model runtimeTraceProjTreeModel) (runtimeTraceProjElimEntry, bool) {
	board := runtimeTraceProjElimBoard(model)
	top := board
	if len(top) > runtimeTraceProjElimTopN {
		top = top[:runtimeTraceProjElimTopN]
	}
	var chain []runtimeTraceProjElimEntry
	for _, entry := range top {
		if entry.channelRank == 0 {
			chain = append(chain, entry)
		}
	}
	sections := runtimeTraceProjElimSectionsFor(chain)
	if len(sections) == 0 || len(sections[0].entries) == 0 {
		return runtimeTraceProjElimEntry{}, false
	}
	best := sections[0].entries[0]
	for _, entry := range sections[0].entries[1:] {
		if entry.row.Node.EffectiveImpactMS > best.row.Node.EffectiveImpactMS {
			best = entry
		}
	}
	return best, true
}
