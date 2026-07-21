package orchestrator

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// prose_headline_elim_check.go — HEADLINE-ELIM 件2+件3 (§29.104.14 立案 +
// §29.104.14.1 witness 定谳 + §29.104.13 用户原则,
// docs/design/real_trace_campaign_20260705.md, 2026-07-16).
//
// Witness (customer cust_span_runnable.txt, donghu 970481 帧): every
// deterministic face was correct — 根因排序#1 = 类校验 9.586ms tier=primary,
// the ◎ 可消除量总览 TOP1 and the projection head line 「主根因:类校验」 all
// agreed — and the model PROSE overturned the board: the headline sentence
// named the #2 seat (shadowhook-task 优先级反转, 8.608ms) as the 核心丢帧原因,
// demoted #1 to 「可归为次要因素」 on a category argument (「并非由外部阻塞
// 引起」), and separately declared 「已运行在超大核最高频点…排除算力供给不足」
// while the report published a typed supply-fold deficit seat (E5 计入
// 4.843ms 折算,运行频点非最高). Nothing on the page juxtaposed the two.
//
// Two disclosure arms, BOTH feeding ONLY the system cross-check appendix
// (collectSystemCrossCheckFindings). §29.104.13 discipline: 纯披露 caveat —
// never a violation, never a retry round, never a body edit; the prose
// extraction is a NOISY signal, so every ambiguity silences the whole arm
// (宁漏勿假指 — a missed finding is a sanctioned cost, a false pointer is
// not):
//
//	arm A (件2) — headline cause vs board #1 juxtaposition. Noisy side: a
//	  CLOSED claim-word family locates the prose headline sentence(s); the
//	  entity is extracted ONLY by verbatim hits against PUBLISHED identities
//	  (seated thread tokens; seated cause-class zh/EN word families). No
//	  extractable entity, conflicting entities, or a thread+class pair no
//	  published seat carries → silent. Precise side: the analysis-target
//	  board's typed rank==1 row (chain channel only — the adjacent channel
//	  numbers its own #N; multi-board reports resolve the target board via
//	  the typed rank_board_target ↔ target_window_states account identity,
//	  unresolvable → silent; XLANE-3 gave every seat its board identity).
//	  Entity identity equal (canonical subject tid, or cause-class family)
//	  → silent; mismatch → ONE finding quoting both published values.
//
//	arm B (件3, rider) — supply/frequency adequacy claim vs published
//	  supply-fold deficit seat. Noisy side: a CLOSED zh/EN claim-word family
//	  (排除…算力/供给/频率/频点…不足 · 算力/供给充足 · 已(运行)在…最高频 ·
//	  no supply deficit · at max frequency). Precise side: a typed
//	  supply_fold_deficit_ms > 0 note on a chain-channel seat row (typed
//	  field decides, never the word face). Both present → ONE finding
//	  quoting the seat; either side absent → silent.
//
//	arm C (FREQDIR-1 件4, §29.149 修向④, 2026-07-19; witness 95946: the
//	  prose 修复方向 enumeration silently dropped the board's #1 direction
//	  频率与热治理 58.320ms and no lane audited the OMISSION — every prior
//	  arm is presence/claim-triggered) — direction-omission disclosure.
//	  Precise side: the resolved chain #1 seat wears an engine-stamped
//	  fix_direction token that resolves in the closed registry word table
//	  (both faces). Noisy side: a model-authored prose unit carries a
//	  direction-enumeration head from the CLOSED head family (修复方向 /
//	  提升空间 / 优化方向) AND an enumeration shape in the SAME unit. The
//	  direction's display word (either face) or its raw token present in
//	  ANY model unit → silent. Both sides present → ONE information-only
//	  juxtaposition line; any ambiguity → silent (宁漏勿假指).
//
// Wording: the §29.104.14.1 并置句 is the user-ratified form for this arm —
// it states the body's claimed entity as a fact equation (正文核心/首要原因=X)
// and juxtaposes the board fact beside it. The CR-4 retired accusatory forms
// (正文将/正文称/表述为) stay banned and never render here.
//
// 复核修补轮 (2026-07-16, 件1–件6): every hit site — arm-B claim positions,
// arm-A entity-word and thread-ref positions — now passes the SAME negation
// look-behind discipline the anchor already had (a matched substring cut out
// of a negated / subjunctive / attributed / membership sentence is an ACTIVE
// distortion of the body, the worst false pointer); the arm-B finding quotes
// the typed supply_fold_deficit_ms value (the value that gates the arm),
// never the seat's eff (which may carry full-amount components that must not
// wear the folded label); and the 核心…原因 anchor infix refuses structural
// particles so cross-phrase shapes (「多个核心被抢占的原因」) never anchor.
const (
	// proseHeadlineNegationWindowRunes bounds the negation look-behind before
	// a claim anchor ("并非核心原因" / "is not the root cause" never anchors).
	proseHeadlineNegationWindowRunes = 12
	// proseHeadlineSupplyClaimQuoteCap bounds the verbatim claim quote length
	// (runes) inside the arm-B finding.
	proseHeadlineSupplyClaimQuoteCap = 40
)

// proseHeadlineAnchorZHRE / proseHeadlineAnchorENRE — the CLOSED headline
// claim-word family (§29.104.14.1 编队词锚). The 核心…原因 alternative allows
// a short bounded infix so the witness form 「核心丢帧原因」 anchors; every
// other member is verbatim. 件6① (P2-2): the infix refuses the structural
// particles 被/的/个/种/类 so cross-phrase shapes where 核心 means CPU cores
// (「多个核心被抢占的原因」) never anchor — a legitimate 「核心的原因」 is a
// sanctioned miss (宁漏勿假指).
//
// ANSWERFACE-1 件3 (§29.140 G7, 2026-07-19): the 根因 members are
// COPULA-BOUND (根因是/根因为/根因在于) — never bare 根因, which is saturated
// by the report's own seat-channel label 「根因排序」 (排 is not a copula, so
// the label can never anchor) and by ordinary 根因分析 prose. The witness
// headline 「…丢帧的根因是优先级反转:…」 anchors on 根因是; interrogative
// forms (根因是否/根因为何) are carved at the anchor site (a question is not
// a claim).
var proseHeadlineAnchorZHRE = regexp.MustCompile(`核心(?:[^\P{Han}被的个种类]|[A-Za-z0-9#]){0,8}原因|核心驱动|首要原因|主要原因|根本原因|主因|根因[是为]|根因在于`)
var proseHeadlineAnchorENRE = regexp.MustCompile(`(?i)\b(?:core|primary|root)\s+cause\b`)

// proseHeadlineNegationMarkers — an anchor immediately preceded by one of
// these (within the look-behind window) is a NEGATED mention, not a claim.
var proseHeadlineNegationMarkers = []string{
	"并非", "而非", "不是", "并不是", "绝非", "不构成", "不属于", "排除",
	"not ", "never ", "isn't ", "is not ",
}

// proseHeadlineEntityNegationMarkers — 件2 (P1-2): an entity word (class
// word or seated thread ref) immediately preceded by one of these is a
// NEGATED entity mention (「…而非优先级反转阻塞」 / "not priority inversion")
// — the body is DENYING that entity, and counting the hit would invert a
// correct body into a false juxtaposition. In the 「A 而非 B」 shape only the
// negated B hit dies: a resolvable A still extracts and compares normally
// (A=榜首 → silent, A≠榜首 → juxtaposed with A as the claimed cause), while
// an unresolvable A leaves no entity and silences the whole arm.
var proseHeadlineEntityNegationMarkers = []string{
	"而非", "并非", "不是", "并不是", "绝非", "不构成", "不属于", "排除了", "排除",
	"not ", "never ", "isn't ", "is not ", "rather than ", "instead of ",
}

// proseHeadlineClaimInabilityMarkers — 件1 (P1-1): an arm-B claim hit
// immediately preceded by an inability prefix is the OPPOSITE claim
// (「不能排除算力供给不足」 asserts the deficit MAY exist) — quoting the
// affirmative substring out of it would actively distort the body. Same
// look-behind window mechanism as proseHeadlineNegationMarkers.
var proseHeadlineClaimInabilityMarkers = []string{
	"不能", "难以", "无法", "未能", "不足以", "尚不能", "尚无法", "并不能",
	"并未", "尚未", "不可",
	"cannot ", "can't ", "could not ", "couldn't ", "unable to ",
	"hard to ", "difficult to ",
}

// proseHeadlineSubjunctiveClauseLeads — 件1 rider (P3-2): an arm-B claim
// inside a clause that OPENS with a subjunctive lead is a hypothesis
// (「若排除算力供给不足,则…」), never an asserted adequacy claim. Clause-start
// check, not a window substring: single-rune 若 as a window needle would hit
// 宛若/恍若-style compounds.
var proseHeadlineSubjunctiveClauseLeads = []string{
	"若", "如果", "假如", "倘若", "假若", "if ", "assuming ", "suppose ",
}

// proseHeadlineAttributionMarkers — 件3 (P2-3): a headline anchor preceded
// anywhere earlier in its sentence by an attribution lead reports someone
// ELSE's causal claim (「用户认为核心丢帧原因是…」), never the body's own
// self-stated headline — the anchor dies.
var proseHeadlineAttributionMarkers = []string{
	"用户认为", "用户称", "客户认为", "客户称", "前置分析认为",
}

// proseHeadlineMembershipZHSuffix / proseHeadlineMembershipENPrefixes — 件3
// (P2-1): 「主要原因之一」 / "one of the primary causes" claim MEMBERSHIP in
// a cause set, never the sole headline cause — the anchor dies under the
// same rule on both language faces (zh suffix, EN prefix; the EN plural form
// already fails the singular \bcause\b anchor, the prefix rule keeps the two
// faces symmetric by semantics rather than by regex accident).
const proseHeadlineMembershipZHSuffix = "之一"

var proseHeadlineMembershipENPrefixes = []string{"one of"}

// proseHeadlineSupplyClaimREs — the CLOSED arm-B claim family (zh + EN).
var proseHeadlineSupplyClaimREs = []*regexp.Regexp{
	regexp.MustCompile(`排除[^。;；，,\n]{0,12}(?:算力|供给|频率|频点)[^。;；，,\n]{0,6}不足`),
	regexp.MustCompile(`(?:算力|供给)充足`),
	regexp.MustCompile(`(?:已运行在|已在|运行在)[^。;；，,\n]{0,10}最高频`),
	regexp.MustCompile(`(?i)no\s+(?:compute[- ])?supply\s+deficit`),
	regexp.MustCompile(`(?i)at\s+(?:the\s+)?max(?:imum)?\s+frequency`),
}

// proseHeadlineSeatRow is one published typed board-seat row.
type proseHeadlineSeatRow struct {
	subject             string
	tid                 string
	classToken          string // canonical published cause-class token ("" when none)
	family              string // class identity after family collapse
	rank                int
	eff                 float64
	hasEff              bool
	adjacent            bool // adjacent channel (邻近影响#N numbers its own space)
	boardTarget         string
	supplyFoldDeficitMS float64
	// fixDirection (FREQDIR-1 件4, §29.149 修向④, 2026-07-19): the seat's
	// engine-stamped registry direction token, verbatim from the typed
	// fix_direction note ("" when unstamped — absence stays absent).
	fixDirection string
}

// proseHeadlineClassFamily collapses the published cause-class token to its
// comparison identity: the two priority-inversion tokens share one prose word
// family (优先级反转…), every other token is its own family.
func proseHeadlineClassFamily(token string) string {
	token = strings.ToLower(strings.TrimSpace(token))
	if strings.HasPrefix(token, "priority_inversion") {
		return "priority_inversion"
	}
	return token
}

// proseHeadlineClassWordEligible excludes bare scheduler-state / diagnostic
// tokens from the entity word table: state words saturate ordinary prose
// ("在 runnable 状态排队") and would anchor entities everywhere (宁漏勿假指).
func proseHeadlineClassWordEligible(token string) bool {
	switch token {
	case "", "runnable_wait", "fragmented_runnable_wait", "sleep_wait",
		"fragmented_sleep_wait", "running", "fragmented_running", "io_wait",
		"d_state_or_io_wait", "fragmented_d_state_or_io_wait", "state_churn",
		"trace_span", "trace_gap", "missing_wakeup":
		return false
	}
	return true
}

// proseHeadlineCollectSeatRows parses the run's published typed board rows
// (rank>0; background and caliber-side rows never seat).
func proseHeadlineCollectSeatRows(ledger types.ObservationLedger) []proseHeadlineSeatRow {
	var out []proseHeadlineSeatRow
	for _, record := range ledger.Records {
		if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		if !strings.Contains(record.ID, "#root_cause_rank:") {
			continue
		}
		notes := record.RichNotes
		rank, ok := proseFactNoteInt(notes, types.TraceNoteKeyRank)
		if !ok || rank <= 0 {
			continue
		}
		relevance := strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyChainRelevance))
		if relevance == "background" || relevance == "self_caliber_side" {
			continue
		}
		subject := strings.TrimSpace(record.Subject)
		token := strings.ToLower(strings.TrimSpace(record.Object))
		if token == "" {
			token = strings.ToLower(strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyType)))
		}
		row := proseHeadlineSeatRow{
			subject:      subject,
			tid:          proseWallClockSubjectTID(subject),
			classToken:   token,
			family:       proseHeadlineClassFamily(token),
			rank:         rank,
			adjacent:     relevance == "adjacent",
			boardTarget:  strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyRankBoardTarget)),
			fixDirection: strings.TrimSpace(proseWallClockNoteValue(notes, types.TraceNoteKeyFixDirection)),
		}
		// RANKDIS-M18 (§29.104.17 裁定② 2026-07-16, 复核件2 勘正):
		// composite-score rows publish the *_score twin instead of the ms
		// key. Under the current closed matrix composite rows never hold a
		// seat (io_pressure → context_only; block_io_by_inode → ⌗ caliber
		// side) — this arm is a defensive reservation for a future seating
		// ruling (board-template ms suit must be synced then). Zero behavior
		// today.
		if v, ok := proseFactNoteFloat(notes, types.TraceNoteKeyEffectiveImpactMS); ok && v > 0 {
			row.eff, row.hasEff = v, true
		} else if v, ok := proseFactNoteFloat(notes, types.TraceNoteKeyEffectiveImpactScore); ok && v > 0 {
			row.eff, row.hasEff = v, true
		} else if v, err := strconv.ParseFloat(strings.TrimSpace(record.Value), 64); err == nil && v > 0 {
			row.eff, row.hasEff = v, true
		}
		if v, ok := proseFactNoteFloat(notes, types.TraceNoteKeySupplyFoldDeficitMS); ok && v > 0 {
			row.supplyFoldDeficitMS = v
		}
		out = append(out, row)
	}
	return out
}

// proseHeadlineBoardNumberOne resolves the analysis-target board's typed
// rank==1 row. Chain channel only. When several boards publish conflicting
// #1 identities, the target board is the one whose typed rank_board_target
// carries the SAME tid as the run's sole target_window_states account;
// anything still ambiguous stays silent.
func proseHeadlineBoardNumberOne(rows []proseHeadlineSeatRow, ledger types.ObservationLedger) (proseHeadlineSeatRow, bool) {
	var ones []proseHeadlineSeatRow
	for _, r := range rows {
		if !r.adjacent && r.rank == 1 {
			ones = append(ones, r)
		}
	}
	if len(ones) == 0 {
		return proseHeadlineSeatRow{}, false
	}
	resolve := func(cands []proseHeadlineSeatRow) (proseHeadlineSeatRow, bool) {
		distinct := map[string]proseHeadlineSeatRow{}
		for _, r := range cands {
			key := r.subject + "\x00" + r.family
			if have, seen := distinct[key]; !seen || (r.hasEff && (!have.hasEff || r.eff > have.eff)) {
				distinct[key] = r
			}
		}
		if len(distinct) != 1 {
			return proseHeadlineSeatRow{}, false
		}
		for _, r := range distinct {
			return r, true
		}
		return proseHeadlineSeatRow{}, false
	}
	if one, ok := resolve(ones); ok {
		return one, true
	}
	accounts := proseWallClockAccountsFromLedger(ledger)
	if len(accounts) != 1 {
		return proseHeadlineSeatRow{}, false
	}
	targetTID := accounts[0].tid
	var onTarget []proseHeadlineSeatRow
	for _, r := range ones {
		if r.boardTarget != "" && proseWallClockSubjectTID(r.boardTarget) == targetTID {
			onTarget = append(onTarget, r)
		}
	}
	return resolve(onTarget)
}

// proseHeadlineClassWord is one published cause-class word the sentence
// extractor matches verbatim.
type proseHeadlineClassWord struct {
	word   string
	ascii  bool
	family string
}

// proseHeadlineClassWords builds the published cause-class word table: for
// every eligible seated class token, its zh display word (the SAME lexicon
// the report surfaces render — tool.TraceRootCauseTypeZHLabel), the raw
// token, and the token's spaced twin; the inversion family matches on its
// base words so 「优先级反转阻塞」 and "priority inversion" both hit. Words
// that would map to two different families are dropped (ambiguous words
// never extract).
func proseHeadlineClassWords(rows []proseHeadlineSeatRow) []proseHeadlineClassWord {
	byWord := map[string]proseHeadlineClassWord{}
	conflicted := map[string]bool{}
	add := func(word, family string, ascii bool) {
		word = strings.TrimSpace(word)
		if word == "" || conflicted[word] {
			return
		}
		if have, seen := byWord[word]; seen {
			if have.family != family {
				conflicted[word] = true
				delete(byWord, word)
			}
			return
		}
		byWord[word] = proseHeadlineClassWord{word: word, ascii: ascii, family: family}
	}
	for _, r := range rows {
		token := r.classToken
		if !proseHeadlineClassWordEligible(token) {
			continue
		}
		if r.family == "priority_inversion" {
			add("优先级反转", r.family, false)
			add("priority inversion", r.family, true)
			add(token, r.family, true)
			continue
		}
		if zh := tool.TraceRootCauseTypeZHLabel(token); zh != "" && zh != token {
			add(zh, r.family, false)
		}
		add(token, r.family, true)
		if strings.Contains(token, "_") {
			add(strings.ReplaceAll(token, "_", " "), r.family, true)
		}
	}
	out := make([]proseHeadlineClassWord, 0, len(byWord))
	for _, w := range byWord {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].word < out[j].word })
	return out
}

// proseHeadlineWordHits returns the byte start offsets of verbatim word
// hits; ASCII words match case-insensitively with ASCII word boundaries, zh
// words match byte-exact. Position-aware so every hit site can pass the
// entity-negation look-behind (件2).
func proseHeadlineWordHits(text, lowerText string, w proseHeadlineClassWord) []int {
	var hits []int
	if !w.ascii {
		idx := 0
		for {
			rel := strings.Index(text[idx:], w.word)
			if rel < 0 {
				return hits
			}
			hits = append(hits, idx+rel)
			idx += rel + len(w.word)
		}
	}
	needle := strings.ToLower(w.word)
	idx := 0
	for {
		rel := strings.Index(lowerText[idx:], needle)
		if rel < 0 {
			return hits
		}
		start := idx + rel
		end := start + len(needle)
		idx = end
		if start > 0 {
			prev := lowerText[start-1]
			if prev == '_' || (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
				continue
			}
		}
		if end < len(lowerText) {
			next := lowerText[end]
			if next == '_' || (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') {
				continue
			}
		}
		hits = append(hits, start)
	}
}

// proseHeadlineWindowMarkerBefore reports whether one of markers occurs
// inside the fixed look-behind window ending at byte offset start — the
// shared negation/qualifier discipline for EVERY hit site (anchors, arm-B
// claims, entity words, thread refs).
func proseHeadlineWindowMarkerBefore(text string, start int, markers []string) bool {
	prefix := text[:start]
	runes := []rune(prefix)
	if len(runes) > proseHeadlineNegationWindowRunes {
		runes = runes[len(runes)-proseHeadlineNegationWindowRunes:]
	}
	window := strings.ToLower(string(runes))
	for _, marker := range markers {
		if strings.Contains(window, marker) {
			return true
		}
	}
	return false
}

// proseHeadlineClauseSubjunctive reports whether the clause containing byte
// offset start opens with a subjunctive lead (件1 rider / P3-2).
func proseHeadlineClauseSubjunctive(sentence string, start int) bool {
	clause := sentence[:start]
	for _, sep := range []string{"，", ",", "；", ";", "。", "：", ":", "、", "\n"} {
		if i := strings.LastIndex(clause, sep); i >= 0 {
			clause = clause[i+len(sep):]
		}
	}
	clause = strings.ToLower(strings.TrimSpace(clause))
	for _, lead := range proseHeadlineSubjunctiveClauseLeads {
		if strings.HasPrefix(clause, lead) {
			return true
		}
	}
	return false
}

// proseHeadlineSentenceAnchored reports whether the sentence carries a
// headline claim word that is neither negated (existing lane) nor a
// membership form (件3/P2-1: 「…之一」 / "one of …") nor an attributed
// third-party claim (件3/P2-3: 「用户认为…」).
func proseHeadlineSentenceAnchored(sentence string) bool {
	lowerSentence := strings.ToLower(sentence)
	for _, re := range []*regexp.Regexp{proseHeadlineAnchorZHRE, proseHeadlineAnchorENRE} {
		for _, loc := range re.FindAllStringIndex(sentence, -1) {
			if proseHeadlineWindowMarkerBefore(sentence, loc[0], proseHeadlineNegationMarkers) {
				continue
			}
			if strings.HasPrefix(sentence[loc[1]:], proseHeadlineMembershipZHSuffix) {
				continue
			}
			// ANSWERFACE-1 件3: interrogative copula tails (根因是否…/
			// 根因为何…) ask a question instead of claiming a cause — the
			// anchor dies (claim sentences only, 宁漏勿假指).
			if next := sentence[loc[1]:]; strings.HasPrefix(next, "否") || strings.HasPrefix(next, "何") {
				continue
			}
			if proseHeadlineWindowMarkerBefore(sentence, loc[0], proseHeadlineMembershipENPrefixes) {
				continue
			}
			attributed := false
			for _, marker := range proseHeadlineAttributionMarkers {
				if idx := strings.Index(lowerSentence, marker); idx >= 0 && idx < loc[0] {
					attributed = true
					break
				}
			}
			if attributed {
				continue
			}
			return true
		}
	}
	return false
}

// proseHeadlineEntity is the resolved headline entity: a seated thread tid
// and/or a published cause-class family (either side may be empty, never
// both).
type proseHeadlineEntity struct {
	tid    string
	tidRaw string // as the prose spelled it
	family string
}

// proseHeadlineExtractEntity scans the model prose for headline sentences
// and resolves ONE unambiguous entity; ok=false silences the arm.
func proseHeadlineExtractEntity(prose []proseTextUnit, rows []proseHeadlineSeatRow, words []proseHeadlineClassWord) (proseHeadlineEntity, bool) {
	subjectSpellings := map[string]map[string]bool{} // tid → accepted spellings
	for _, r := range rows {
		if r.tid == "" {
			continue
		}
		if subjectSpellings[r.tid] == nil {
			subjectSpellings[r.tid] = map[string]bool{}
		}
		subjectSpellings[r.tid][r.subject] = true
		if trimmed := strings.TrimLeft(r.subject, "."); trimmed != r.subject && trimmed != "" {
			subjectSpellings[r.tid][trimmed] = true
		}
	}
	tids := map[string]string{} // tid → prose spelling
	families := map[string]bool{}
	for _, unit := range prose {
		for _, span := range proseSentenceSpans(unit.text) {
			sentence := unit.text[span[0]:span[1]]
			if !proseHeadlineSentenceAnchored(sentence) {
				continue
			}
			for _, tref := range extractProseScalarThreadRefs(sentence) {
				spellings := subjectSpellings[tref.TID]
				if spellings == nil || !spellings[tref.Raw] {
					continue
				}
				// 件2 (P1-2): a negated thread mention is a denial, not the
				// body's claimed headline cause.
				if proseHeadlineWindowMarkerBefore(sentence, tref.Pos, proseHeadlineEntityNegationMarkers) {
					continue
				}
				tids[tref.TID] = tref.Raw
			}
			lower := strings.ToLower(sentence)
			for _, w := range words {
				for _, pos := range proseHeadlineWordHits(sentence, lower, w) {
					// 件2 (P1-2): negated class-word hits die; any surviving
					// hit counts the family once.
					if proseHeadlineWindowMarkerBefore(sentence, pos, proseHeadlineEntityNegationMarkers) {
						continue
					}
					families[w.family] = true
					break
				}
			}
		}
	}
	if len(tids) > 1 || len(families) > 1 {
		return proseHeadlineEntity{}, false
	}
	entity := proseHeadlineEntity{}
	for tid, raw := range tids {
		entity.tid, entity.tidRaw = tid, raw
	}
	for family := range families {
		entity.family = family
	}
	if entity.tid == "" && entity.family == "" {
		return proseHeadlineEntity{}, false
	}
	// A thread+class pair must co-occur on some published seat — otherwise
	// the two words point at different rows and the entity is not one thing.
	if entity.tid != "" && entity.family != "" {
		coherent := false
		for _, r := range rows {
			if r.tid == entity.tid && r.family == entity.family {
				coherent = true
				break
			}
		}
		if !coherent {
			return proseHeadlineEntity{}, false
		}
	}
	return entity, true
}

// proseHeadlineBestSeat picks the entity's best published seat: chain channel
// first, lowest rank, then largest effective value.
func proseHeadlineBestSeat(rows []proseHeadlineSeatRow, entity proseHeadlineEntity) (proseHeadlineSeatRow, bool) {
	pick := func(adjacent bool) (proseHeadlineSeatRow, bool) {
		var best proseHeadlineSeatRow
		found := false
		for _, r := range rows {
			if r.adjacent != adjacent {
				continue
			}
			if entity.tid != "" && r.tid != entity.tid {
				continue
			}
			if entity.family != "" && r.family != entity.family {
				continue
			}
			if !found || r.rank < best.rank || (r.rank == best.rank && r.eff > best.eff) {
				best, found = r, true
			}
		}
		return best, found
	}
	if seat, ok := pick(false); ok {
		return seat, true
	}
	return pick(true)
}

// proseHeadlineSeatLabel renders one seat as 「主体 · 类词(通道#N·有效归因
// X ms)」 on the zh face and the raw-token twin on the EN face. includeRank
// lets the #1 side drop its rank chip (the finding prefix already names it).
func proseHeadlineSeatLabel(seat proseHeadlineSeatRow, zh, includeRank bool) string {
	classWord := seat.classToken
	if zh {
		if label := tool.TraceRootCauseTypeZHLabel(seat.classToken); label != "" {
			classWord = label
		}
	}
	name := seat.subject
	if name == "" {
		name = classWord
		classWord = ""
	}
	if classWord != "" {
		name += " · " + classWord
	}
	var details []string
	channelZH, channelEN := tracefence.SeatChannelChainZH, tracefence.SeatChannelChainEN
	if seat.adjacent {
		channelZH, channelEN = tracefence.SeatChannelAdjacentZH, tracefence.SeatChannelAdjacentEN
	}
	if includeRank && seat.rank > 0 {
		if zh {
			details = append(details, fmt.Sprintf("%s#%d", channelZH, seat.rank))
		} else {
			details = append(details, fmt.Sprintf("%s #%d", channelEN, seat.rank))
		}
	}
	if seat.hasEff {
		if zh {
			details = append(details, fmt.Sprintf("有效归因 %.3fms", seat.eff))
		} else {
			details = append(details, fmt.Sprintf("effective %.3fms", seat.eff))
		}
	}
	if len(details) == 0 {
		return name
	}
	if zh {
		return name + "(" + strings.Join(details, "·") + ")"
	}
	return name + " (" + strings.Join(details, ", ") + ")"
}

// proseHeadlineEntityLabel renders the prose-claimed entity: its best
// published seat when one exists, else the bare matched identity.
func proseHeadlineEntityLabel(rows []proseHeadlineSeatRow, entity proseHeadlineEntity, zh bool) string {
	if seat, ok := proseHeadlineBestSeat(rows, entity); ok {
		return proseHeadlineSeatLabel(seat, zh, true)
	}
	if entity.tidRaw != "" {
		return entity.tidRaw
	}
	if zh {
		if label := tool.TraceRootCauseTypeZHLabel(entity.family); label != "" {
			return label
		}
	}
	return entity.family
}

// proseHeadlineElimFindings is the appendix provider for both arms.
func proseHeadlineElimFindings(doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState) []proseScalarBindingFinding {
	if doc == nil || bus == nil || mut == nil {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return nil
	}
	rows := proseHeadlineCollectSeatRows(ledger)
	if len(rows) == 0 {
		return nil
	}
	prose := collectModelProseUnits(doc)
	var out []proseScalarBindingFinding
	if f, ok := proseHeadlineCauseFinding(prose, rows, ledger); ok {
		out = append(out, f)
	}
	if f, ok := proseHeadlineSupplyFinding(prose, rows); ok {
		out = append(out, f)
	}
	if f, ok := proseHeadlineDirectionOmissionFinding(prose, rows, ledger); ok {
		out = append(out, f)
	}
	return out
}

// proseHeadlineJuxtaposition renders the single-point arm-A juxtaposition
// sentence pair (词面单点 — the entity-mismatch lane and the ANSWERFACE-1 件3
// unpublished-inversion lane speak the SAME template; only the X label
// differs). 件6② (P3): the ledger-spec original tail — 「(如适用)」 keeps the
// sentence honest for bodies that already declared the divergence in a
// neighbouring sentence (the sentence-scoped extractor cannot see across
// sentences, so the duty is stated conditionally, never as an accusation).
func proseHeadlineJuxtaposition(xZH, xEN string, one proseHeadlineSeatRow) proseScalarBindingFinding {
	yZH := proseHeadlineSeatLabel(one, true, false)
	yEN := proseHeadlineSeatLabel(one, false, false)
	return proseScalarBindingFinding{
		entryZH: fmt.Sprintf("正文核心/首要原因=%s;本报告%s#1=%s。两者不一致;正文如为有意分歧,应显式声明分歧并给出数值依据(如适用)",
			xZH, tracefence.SeatChannelChainZH, yZH),
		entry: fmt.Sprintf("the body's core/primary cause = %s; this report's %s #1 = %s. The two differ; if the divergence is intentional, the body should state it explicitly with the numeric basis (if applicable)",
			xEN, tracefence.SeatChannelChainEN, yEN),
	}
}

// proseHeadlineCauseFinding — arm A (件2) plus the ANSWERFACE-1 件3
// unpublished-inversion fallback lane (§29.140 G7).
func proseHeadlineCauseFinding(prose []proseTextUnit, rows []proseHeadlineSeatRow, ledger types.ObservationLedger) (proseScalarBindingFinding, bool) {
	one, ok := proseHeadlineBoardNumberOne(rows, ledger)
	if !ok {
		return proseScalarBindingFinding{}, false
	}
	entity, ok := proseHeadlineExtractEntity(prose, rows, proseHeadlineClassWords(rows))
	if !ok {
		// No resolvable published entity — the closed-lexicon lane is
		// structurally blind here; the 件3 unpublished-inversion lane may
		// still see an affirmative inversion claim (witness: 「根因是优先级
		// 反转」 with zero inversion seats — the wrong-cause word names a
		// cause the board never published, so no verbatim seat identity can
		// resolve).
		return proseHeadlineUnpublishedInversionFinding(prose, rows, one)
	}
	// Entity identical to #1 (either identity lane) → nothing to juxtapose on
	// the entity lane; the 件3 lane still applies (semantic run-2 shape: the
	// sentence names BOTH the #1 entity as the low-priority worker AND the
	// unpublished inversion word as the claimed cause — naming the #1 entity
	// in a supporting role does not make the inversion headline honest).
	if (entity.tid != "" && one.tid != "" && entity.tid == one.tid) ||
		(entity.family != "" && entity.family == one.family) {
		return proseHeadlineUnpublishedInversionFinding(prose, rows, one)
	}
	return proseHeadlineJuxtaposition(
		proseHeadlineEntityLabel(rows, entity, true),
		proseHeadlineEntityLabel(rows, entity, false),
		one), true
}

// proseHeadlineUnpublishedInversionWords — the CLOSED global inversion word
// family for the ANSWERFACE-1 件3 lane (§29.140 G7). Deliberately global (not
// seat-derived): the lane's whole point is a cause word the board did NOT
// publish, so the seat-derived lexicon can never carry it. Closed two-form zh
// + one EN form; byte-exact zh / ASCII-word-boundary EN via the shared
// proseHeadlineWordHits matcher.
var proseHeadlineUnpublishedInversionWords = []proseHeadlineClassWord{
	{word: "优先级反转", family: "priority_inversion"},
	{word: "优先级倒置", family: "priority_inversion"},
	{word: "priority inversion", ascii: true, family: "priority_inversion"},
}

// proseHeadlineUnpublishedInversionClaim scans anchored headline sentences
// for an AFFIRMATIVE inversion-family word hit. Every hit walks the full
// guard set (§29.110 修补轮 discipline): entity-negation look-behind (「而非
// 优先级反转」 denies), inability look-behind (「不能排除优先级反转」 is the
// opposite claim), and the subjunctive clause lead (「若根因是优先级反转…」 is
// a hypothesis). Returns the matched verbatim word.
func proseHeadlineUnpublishedInversionClaim(prose []proseTextUnit) (string, bool) {
	for _, unit := range prose {
		for _, span := range proseSentenceSpans(unit.text) {
			sentence := unit.text[span[0]:span[1]]
			if !proseHeadlineSentenceAnchored(sentence) {
				continue
			}
			lower := strings.ToLower(sentence)
			for _, w := range proseHeadlineUnpublishedInversionWords {
				for _, pos := range proseHeadlineWordHits(sentence, lower, w) {
					if proseHeadlineWindowMarkerBefore(sentence, pos, proseHeadlineEntityNegationMarkers) {
						continue
					}
					if proseHeadlineWindowMarkerBefore(sentence, pos, proseHeadlineClaimInabilityMarkers) {
						continue
					}
					if proseHeadlineClauseSubjunctive(sentence, pos) {
						continue
					}
					return w.word, true
				}
			}
		}
	}
	return "", false
}

// proseHeadlineUnpublishedInversionFinding — the 件3 lane (§29.140 G7,
// semantic run-2 witness: headline 「丢帧的根因是优先级反转」 while the
// deterministic #1 is a class-verification seat and the tree seats NO
// priority-inversion row). Precise side: the typed absence of any
// priority_inversion-family seat (closed family collapse over published
// tokens) plus the resolved board #1. Noisy side: the closed global word
// family above under the full guard set. ANY published inversion-family seat
// silences the lane wholesale — the seat-derived entity lanes own that shape
// (identity/coherence rules, short_runnable 形 stays silent by family
// identity match).
func proseHeadlineUnpublishedInversionFinding(prose []proseTextUnit, rows []proseHeadlineSeatRow, one proseHeadlineSeatRow) (proseScalarBindingFinding, bool) {
	for _, r := range rows {
		if r.family == "priority_inversion" {
			return proseScalarBindingFinding{}, false
		}
	}
	word, ok := proseHeadlineUnpublishedInversionClaim(prose)
	if !ok {
		return proseScalarBindingFinding{}, false
	}
	xZH := word + "(本报告无已发布的该类席位)"
	xEN := "priority inversion (this report publishes no seat of that class)"
	return proseHeadlineJuxtaposition(xZH, xEN, one), true
}

// proseHeadlineSupplyClaimInSentence returns the first arm-B claim hit in
// the sentence that survives the inability look-behind (件1/P1-1: 「不能排除
// 算力供给不足」 is the OPPOSITE claim — quoting the affirmative substring out
// of it would actively distort the body), the shared negation look-behind,
// and the subjunctive clause check (P3-2: 「若排除算力供给不足,则…」 is a
// hypothesis, not an adequacy claim).
func proseHeadlineSupplyClaimInSentence(sentence string) string {
	for _, re := range proseHeadlineSupplyClaimREs {
		for _, loc := range re.FindAllStringIndex(sentence, -1) {
			if proseHeadlineWindowMarkerBefore(sentence, loc[0], proseHeadlineClaimInabilityMarkers) {
				continue
			}
			if proseHeadlineWindowMarkerBefore(sentence, loc[0], proseHeadlineNegationMarkers) {
				continue
			}
			if proseHeadlineClauseSubjunctive(sentence, loc[0]) {
				continue
			}
			return sentence[loc[0]:loc[1]]
		}
	}
	return ""
}

// proseHeadlineSupplySubjectBound — 件4 (P2-4) subject binding: true when
// the claim sentence names no thread identity at all (default: the claim is
// about the analysis subject, current behavior) or when every named identity
// is the deficit seat's own tid. ANY foreign tid unbinds the hit — a true
// statement about ANOTHER thread's frequency (「RenderThread-64334 运行在
// 接近最高频」) must never be juxtaposed with the target's deficit seat
// (宁漏勿假指).
func proseHeadlineSupplySubjectBound(sentence, seatTID string) bool {
	refs := extractProseScalarThreadRefs(sentence)
	for _, ref := range refs {
		if ref.TID != seatTID {
			return false
		}
	}
	return true
}

// proseHeadlineSupplyFinding — arm B (件3 rider).
func proseHeadlineSupplyFinding(prose []proseTextUnit, rows []proseHeadlineSeatRow) (proseScalarBindingFinding, bool) {
	// Precise side first: the largest typed supply-fold deficit on a chain
	// seat — 件5 (P2-5): the finding quotes THIS gating value, never the
	// seat's eff, which on composite seats (E11 shape: eff = runnable full
	// amount + running folded deficit) carries full-amount components that
	// must not wear the folded label.
	var seat proseHeadlineSeatRow
	found := false
	for _, r := range rows {
		if r.adjacent || r.supplyFoldDeficitMS <= 0 {
			continue
		}
		if !found || r.supplyFoldDeficitMS > seat.supplyFoldDeficitMS {
			seat, found = r, true
		}
	}
	if !found {
		return proseScalarBindingFinding{}, false
	}
	claim := ""
	for _, unit := range prose {
		for _, span := range proseSentenceSpans(unit.text) {
			sentence := unit.text[span[0]:span[1]]
			hit := proseHeadlineSupplyClaimInSentence(sentence)
			if hit == "" || !proseHeadlineSupplySubjectBound(sentence, seat.tid) {
				continue
			}
			claim = hit
			break
		}
		if claim != "" {
			break
		}
	}
	if claim == "" {
		return proseScalarBindingFinding{}, false
	}
	if runes := []rune(claim); len(runes) > proseHeadlineSupplyClaimQuoteCap {
		claim = string(runes[:proseHeadlineSupplyClaimQuoteCap]) + "…"
	}
	seatZH, seatEN := "", ""
	if seat.rank > 0 {
		seatZH = fmt.Sprintf("·%s#%d", tracefence.SeatChannelChainZH, seat.rank)
		seatEN = fmt.Sprintf(" · %s #%d", tracefence.SeatChannelChainEN, seat.rank)
	}
	return proseScalarBindingFinding{
		entryZH: fmt.Sprintf("正文含「%s」;本报告已发布供给折算缺口席:%s 供给折算缺口 %.3fms(运行频点非最高)%s",
			claim, seat.subject, seat.supplyFoldDeficitMS, seatZH),
		entry: fmt.Sprintf("the body contains %q; this report publishes a typed supply-fold deficit seat: %s, supply-fold deficit %.3fms (running frequency below the maximum)%s",
			claim, seat.subject, seat.supplyFoldDeficitMS, seatEN),
	}, true
}

// proseHeadlineDirectionHeadWords — the CLOSED direction-enumeration head
// family for arm C (FREQDIR-1 件4; byte-exact zh substrings). Deliberately
// tiny: a head word alone never fires the arm — the SAME unit must also
// carry an enumeration shape (a plain sentence mentioning 提升空间 without a
// list is not a direction enumeration). Extending the family is a wordface
// ruling — never widen it in passing.
var proseHeadlineDirectionHeadWords = []string{"修复方向", "提升空间", "优化方向"}

// proseHeadlineEnumerationLineRE — the enumeration shape: a line opening
// with a bullet / numbered / circled-digit / 方向N enumerator (leading
// blockquote/emphasis/heading markup tolerated). Two or more such lines in
// one unit make an enumeration; one bullet is not a list (宁漏勿假指).
// 返工 P2-2(b) (双复核): bullet symbols require a following whitespace
// ([-•‣][ \t]) so glued prose dashes never count, and horizontal-rule lines
// (----) are carved out per line before matching.
var proseHeadlineEnumerationLineRE = regexp.MustCompile(`^[\t >#*]*(?:[-•‣][ \t]|\d+[.、)．]|[①②③④⑤⑥⑦⑧⑨⑩➊➋➌➍➎]|(?:修复)?方向[一二三四五六七八九十])`)

// proseHeadlineHorizontalRuleRE — a markdown horizontal rule (`---` family):
// never an enumeration line (返工 P2-2(b) — an hr's leading `-` matched the
// old bullet class and two hr separators minted a fake enumeration).
var proseHeadlineHorizontalRuleRE = regexp.MustCompile(`^[ \t]*-{2,}[ \t]*$`)

// proseHeadlineEnumerationLineCount counts the unit's enumeration lines with
// the hr carve applied per line.
func proseHeadlineEnumerationLineCount(text string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if proseHeadlineHorizontalRuleRE.MatchString(line) {
			continue
		}
		if proseHeadlineEnumerationLineRE.MatchString(line) {
			count++
		}
	}
	return count
}

// proseHeadlineDirectionOmissionFinding — arm C (FREQDIR-1 件4, §29.149
// 修向④): the typed board #1 direction absent from a prose direction
// enumeration → ONE information-only juxtaposition line.
//
// FREQDIR-1 件5 boundary (§29.149 修向⑤, user-adjudicated 2026-07-19,
// recorded here so a future session does not re-litigate it): direction
// completeness / prose-vs-board direction consistency NEVER gets a hard
// gate. Whether prose "enumerates directions" is a noisy extraction, and
// §29.42.4 (排序一致性车道全线无硬拦) + §29.104.13 (非致命不硬拦) place the
// answer face under model ownership — a hard reject here would fire on
// structurally fine answers (e.g. a user question deliberately scoped to one
// direction). This arm and any future sibling stay appendix-only
// disclosures: never a violation, never a retry round, never a body edit.
func proseHeadlineDirectionOmissionFinding(prose []proseTextUnit, rows []proseHeadlineSeatRow, ledger types.ObservationLedger) (proseScalarBindingFinding, bool) {
	// Precise side: the resolved chain #1 seat wears a registry-resolved
	// direction token (unstamped or unregistered → silent; the shared
	// board-#1 resolution already silences every ambiguous board shape).
	one, ok := proseHeadlineBoardNumberOne(rows, ledger)
	if !ok {
		return proseScalarBindingFinding{}, false
	}
	token := strings.TrimSpace(one.fixDirection)
	wordZH, okZH := tracefence.FixDirectionWord(token, true)
	wordEN, okEN := tracefence.FixDirectionWord(token, false)
	if !okZH || !okEN {
		return proseScalarBindingFinding{}, false
	}
	// The direction's population: same-board chain seats wearing this
	// direction token. Its maximum recoverable amount = the LARGEST
	// published effective value (the ◎ section-head formula: 「最大可消」恒
	// 为该节最大席值 — a pure MAX over published typed values, never a
	// sum). No published value → silent (a juxtaposition without its
	// magnitude teaches nothing and a guessed one would lie). The published
	// eff faces double as the P2-2(c) co-reference silence vocabulary below.
	var dirEffFaces []string
	maxEff, hasEff := 0.0, false
	for _, r := range rows {
		if r.adjacent || r.fixDirection != token || !r.hasEff || r.boardTarget != one.boardTarget {
			continue
		}
		dirEffFaces = append(dirEffFaces, fmt.Sprintf("%.3f", r.eff))
		if !hasEff || r.eff > maxEff {
			maxEff, hasEff = r.eff, true
		}
	}
	if !hasEff {
		return proseScalarBindingFinding{}, false
	}
	// Noisy side: model units carrying a closed-family enumeration head AND
	// the enumeration shape (hr-carved, ≥2 lines) in the SAME unit.
	var enumUnits []proseTextUnit
	for _, unit := range prose {
		headed := false
		for _, head := range proseHeadlineDirectionHeadWords {
			if strings.Contains(unit.text, head) {
				headed = true
				break
			}
		}
		if !headed {
			continue
		}
		if proseHeadlineEnumerationLineCount(unit.text) >= 2 {
			enumUnits = append(enumUnits, unit)
		}
	}
	if len(enumUnits) == 0 {
		return proseScalarBindingFinding{}, false
	}
	// Silence side 1: the direction's display word (either face) or its raw
	// registry token anywhere in the model prose → the direction was
	// mentioned somewhere and the omission claim would be false (词在场任意
	// 块→静默; negated or peripheral mentions silence too — more silence is
	// the sanctioned direction).
	tokenLower := strings.ToLower(token)
	tokenSpaced := strings.ReplaceAll(tokenLower, "_", " ")
	wordENLower := strings.ToLower(wordEN)
	for _, unit := range prose {
		if strings.Contains(unit.text, wordZH) {
			return proseScalarBindingFinding{}, false
		}
		lower := strings.ToLower(unit.text)
		if strings.Contains(lower, wordENLower) || strings.Contains(lower, tokenLower) ||
			strings.Contains(lower, tokenSpaced) {
			return proseScalarBindingFinding{}, false
		}
	}
	// Silence side 2 (返工 P2-2(c), semantic co-reference): an enumeration
	// unit that quotes a same-direction seat's published eff face (e.g.
	// 58.320) covers the direction IN SUBSTANCE under other words — the
	// word-absence juxtaposition would read as an accusation against an
	// entry that exists (宁漏勿假指: substantive coverage silences).
	for _, unit := range enumUnits {
		for _, face := range dirEffFaces {
			if strings.Contains(unit.text, face) {
				return proseScalarBindingFinding{}, false
			}
		}
	}
	// 返工 P2-2(a): the sentence states the verbatim ABSENCE fact only (the
	// deterministic check performed) — it never presupposes that the prose
	// enumeration is a 清单 of directions.
	return proseScalarBindingFinding{
		entryZH: fmt.Sprintf("typed 事实: 修向 %s · 最大可消 %.3fms(该方向最大席值,%s#%d 席所在方向)——正文未出现该方向词",
			wordZH, maxEff, tracefence.SeatChannelChainZH, one.rank),
		entry: fmt.Sprintf("typed fact: fix-direction %s · max recoverable %.3fms (the direction's largest seat value; the direction of the %s #%d seat) — the direction's word does not appear in the body",
			wordEN, maxEff, tracefence.SeatChannelChainEN, one.rank),
	}, true
}
