package orchestrator

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

// mechanical_claim_check.go — EVALFIX-2C 类3 (2026-07-30,
// docs/design/evalfix2_class_designs_20260730.md CLASS 3).
//
// Problem class: prose assertions whose truth is decidable by ARITHMETIC
// ALONE ship self-contradicted. First family (numeric_direction): one
// sentence states ⟨quantity A⟩ ⟨comparator⟩ ⟨quantity B⟩ — negation
// forms included — with explicit same-dimension units on BOTH sides,
// and the written direction is the opposite of what the two normalized
// numbers say. Witness F7: 「Choreographer#doFrame 本身的执行时长（80ms）
// 未超过 60Hz 帧预算（16.67ms）」 while 80 > 16.67 (≈4.8×).
//
// Distinct from PSG: PSG asks "is this number published on the evidence
// face?"; this lane asks "does this sentence contradict itself?" — a
// perfectly grounded pair of numbers can still ship with the direction
// written backwards. The lane therefore has NO evidence-pool concept
// and NO trace-run scope gate: it applies to every run.
//
// Red-line compliance (precise-signals-for-hard-gates): the arithmetic
// COMPARISON is exact, but the EXTRACTION (quantity tokens, comparator
// words, negation scope, side binding) is a noisy regex/lexicon signal —
// so the whole lane is SOFT forever. One merged violation per run at
// most (one-round latch, same anti-livelock shape as PSG), never a hard
// emit-time reject, never a system rewrite of the body. Precision over
// recall: a false positive burns a bounded soft retry, so every doubtful
// extraction SKIPS (宁松勿严 inverted — this lane's loose direction is
// staying silent).
//
// Registry form: mechanicalClaimCheckers is the extension point — the
// next arithmetic-decidable family (percent_sum / count_vs_list /
// sum_identity) is ONE table row plus ONE Scan function; the shared
// shell owns prose-unit collection, sentence spans, the one-round latch,
// the single merged soft violation, the repair hint, and the
// decisive-tier ship-time caveat. The extension shape is pinned by test
// (TestMechanicalClaim_RegistryExtensionShape).
//
// One-round latch (anti-livelock), shared by ALL families:
//
//	part 1 — when the CURRENT dispatch's retry surface already lists
//	         this kind, the hint has been delivered; mark the sticky
//	         run-level latch and stay silent forever after.
//	part 2 — the sticky latch survives later retry rounds that rebuild
//	         the retry surface without this kind.
//
// Ship-time disclosure: after the hint round was consumed, a re-scan of
// the SHIPPED document with the SAME Scan functions (same registry, same
// constants — 谓词同源, no second recognizer) discloses any remaining
// DECISIVE-tier findings (≥2× magnitude) through the system caveat
// channel. The body is never rewritten; the caveat rides the CAVSTR
// replay register so it appends at most once.

// Work bounds and magnitude tiers — all named constants OWNED by this
// lane (容差常量禁跨语义借用: nothing here is borrowed from PSG /
// wallclock, and nothing here may be borrowed BY them).
const (
	// mechanicalClaimFindingCap bounds findings collected per run by the
	// shared shell; mechanicalClaimScanTokenCap bounds quantity tokens
	// extracted per prose text unit inside a family scanner. Generous
	// for real answers; they exist only to bound worst-case work.
	mechanicalClaimFindingCap   = 8
	mechanicalClaimScanTokenCap = 200

	// mechanicalClaimSentenceRuneCap — R9-1 (round-9 sweep): the whole
	// numeric_direction pass SKIPS any sentence longer than this many
	// runes. Degenerate generations (KB-scale run-on "sentences") are
	// not legitimate claim carriers, and every per-sentence predicate in
	// this lane is at worst O(sentence) per comparator, so the cap is
	// also the lane's worst-case work bound (pre-cap, a 27KB degenerate
	// sentence drove the scan to ~32s at finalize and AGAIN at the
	// ship-time rescan). Real answer prose sits far below the cap;
	// skipping an over-cap sentence costs recall only. Applies to the
	// hint round AND the ship-time rescan — both consume the same Scan
	// functions.
	mechanicalClaimSentenceRuneCap = 400

	// mechanicalClaimBindGapRunes is the comparator→quantity leash on
	// EACH side (rune distance between the quantity token and the
	// comparator word). Beyond the leash the binding is a guess, and
	// guesses skip.
	mechanicalClaimBindGapRunes = 30

	// mechanicalClaimNegationGapRunesZH / -GapWordsEN bound how far a
	// negation word may sit before the comparator base word.
	mechanicalClaimNegationGapRunesZH = 4
	mechanicalClaimNegationGapWordsEN = 3

	// Direction-contradiction magnitude tiers. The reverse magnitude
	// (relative difference of the two normalized values) must EXCEED the
	// margin for a finding at all; the decisive tier additionally
	// requires a ≥2× ratio and is the only tier the ship-time caveat
	// discloses.
	mechanicalClaimDirectionRelMargin       = 0.10
	mechanicalClaimApproxDirectionRelMargin = 0.25
	mechanicalClaimDecisiveRatio            = 2.0

	// Absolute floors per dimension (防微小数值的相对幅度虚高): the
	// normalized absolute difference must exceed the floor.
	mechanicalClaimTimeFloorMS   = 0.5 // time dimension, milliseconds
	mechanicalClaimPercentFloorP = 0.5 // percent dimension, percentage points
)

// claimDimension is the closed dimension set. Only SAME-dimension pairs
// are ever compared; time vs percent is structurally incomparable.
type claimDimension int

const (
	dimNone claimDimension = iota
	dimTime
	dimPercent
)

// mechanicalClaimUnits is the closed unit table — ONE definition for the
// regex face and the normalization face. Time normalizes to ms; percent
// values are percentage points. min/h etc. are deliberately out of the
// set (word-face noise, no witness; admission needs a new ruling).
var mechanicalClaimUnits = map[string]struct {
	dim    claimDimension
	factor float64
}{
	"us": {dimTime, 0.001}, "µs": {dimTime, 0.001}, "μs": {dimTime, 0.001}, "微秒": {dimTime, 0.001},
	"ms": {dimTime, 1}, "毫秒": {dimTime, 1},
	"s": {dimTime, 1000}, "秒": {dimTime, 1000},
	"%": {dimPercent, 1}, "％": {dimPercent, 1},
}

// mechanicalClaimQuantityRE captures a decimal numeral immediately
// followed by a closed-set unit. Longer unit spellings come first so
// leftmost-first alternation never truncates them. Guard logic (left
// neighbour, unit right neighbour, range adjacency) lives in
// extractMechanicalClaimTokens — same discipline as the PSG extractor
// but a SEPARATE regex: PSG's two-unit scope (§25 ruling b) is not
// touched.
var mechanicalClaimQuantityRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*(us|µs|μs|微秒|ms|毫秒|秒|s|%|％)`)

// mechanicalClaimToken is one extracted quantity with its normalized
// value. Positions are byte offsets in the text unit.
type mechanicalClaimToken struct {
	raw      string
	unit     string
	dim      claimDimension
	numStart int     // byte offset of the numeral
	unitEnd  int     // byte offset just past the unit
	value    float64 // normalized: ms (dimTime) or percentage points (dimPercent)
	ulp      float64 // one unit in the last written digit, normalized
	approx   bool    // explicitly approximation-marked (约/≈/~/approximately …)
	rangeEnd bool    // endpoint of a numeral–numeral range; never a comparison side
}

// mechanicalClaimFinding is one arithmetically PROVEN contradiction
// (the arithmetic layer re-verified it; only extraction noise remains).
// entry/entryZH are the en/zh faces, same dual-face shape as the
// prose_* family.
type mechanicalClaimFinding struct {
	entry, entryZH string
	blockID        string
	decisive       bool // magnitude reached the decisive tier (§4.4); ship-time caveat collects ONLY this tier
}

// mechanicalClaimChecker is one arithmetic-decidable claim family. Scan
// does extraction + arithmetic verdict ONLY; assembly, latch, caveat and
// caps all live in the shared shell.
type mechanicalClaimChecker struct {
	Name string // "numeric_direction"; future: "percent_sum" / "count_vs_list" / "sum_identity"
	Scan func(unit proseTextUnit, spans [][2]int) []mechanicalClaimFinding
}

// mechanicalClaimCheckers is the family registry. The next family is one
// row here plus one Scan function — nothing else (pinned by
// TestMechanicalClaim_RegistryExtensionShape).
var mechanicalClaimCheckers = []mechanicalClaimChecker{
	{Name: "numeric_direction", Scan: scanNumericDirectionClaims},
}

// scanMechanicalClaimFindings runs every registered family over the
// model-authored prose surfaces. This is the ONE detector both the hint
// round and the ship-time re-scan consume (谓词同源 — a second
// recognizer would be a predicate fork).
func scanMechanicalClaimFindings(doc *types.AnswerDocumentV2) []mechanicalClaimFinding {
	if doc == nil {
		return nil
	}
	var findings []mechanicalClaimFinding
	for _, unit := range collectModelProseUnits(doc) {
		if len(findings) >= mechanicalClaimFindingCap {
			break
		}
		spans := proseSentenceSpans(unit.text)
		for _, checker := range mechanicalClaimCheckers {
			if checker.Scan == nil {
				continue
			}
			for _, f := range checker.Scan(unit, spans) {
				if len(findings) >= mechanicalClaimFindingCap {
					break
				}
				findings = append(findings, f)
			}
		}
	}
	return findings
}

// runMechanicalClaimCheck is the shared shell: latch, scan, and the ONE
// merged soft violation for the whole lane (all families share the
// single per-run round budget). lang is threaded from the bus for
// face selection parity with the wiring signature; the violation Detail
// keeps the prose-lane discipline (en LLM/log face — each entry quotes
// the offending sentence fragment verbatim in its own language), and the
// zh faces ship on the residual caveat.
func runMechanicalClaimCheck(doc *types.AnswerDocumentV2, mut *types.MutableState, lang string) []types.Violation {
	_ = lang
	if doc == nil || mut == nil {
		return nil
	}
	// One-round latch part 1: the current dispatch was born carrying the
	// mechanical-claim hint — its output is final for this lane.
	if rs := mut.RetryState(); rs != nil && retryStateListsMechanicalClaimHint(rs) {
		mut.MarkMechanicalClaimHintDelivered()
		return nil
	}
	// One-round latch part 2: the hint was consumed on an earlier round.
	if mut.MechanicalClaimHintDelivered() {
		return nil
	}
	findings := scanMechanicalClaimFindings(doc)
	if len(findings) == 0 {
		return nil
	}
	entries := make([]string, 0, len(findings))
	for _, f := range findings {
		entries = append(entries, f.entry)
	}
	return []types.Violation{{
		Kind: types.ViolMechanicalClaimContradiction,
		Detail: fmt.Sprintf(
			"answer prose contains %d comparison statement(s) whose written direction contradicts the numbers' own arithmetic: %s",
			len(findings), strings.Join(entries, "; ")),
		Repair: "go through the listed sentences ONE BY ONE — for EACH listed sentence, re-read its two numbers exactly as written and compare them after converting both to one unit; if the comparison word points the wrong way, fix the comparison word (e.g. change 未超过 to 超过, or 'does not exceed' to 'exceeds'); if one of the two numbers is itself mis-stated, correct that number to the value this report's evidence publishes (never invent a replacement); if both numbers and the direction are intended and the sentence means something different, rewrite the sentence so the comparison it states agrees with its own numbers. Apply the fix with the SMALLEST possible edit: change ONLY the blocks named in the finding (by their block id) and keep every other block byte-for-byte identical — prefer emit_answer_document_patch, listing the untouched block ids as unchanged and replacing only the named blocks, over rewriting the whole answer.",
		Stage:  string(types.StageFinalize),
		ClusterKey: types.IdentityClusterKey("mechanical_claim_contradiction",
			"answer_prose_claims"),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_document.blocks.prose",
			Reason:     "prose comparison sentence whose direction word contradicts its own two unit-normalized numbers",
			Confidence: 0.6,
		},
		RepairLocusOverride: types.LocusFinalizer,
	}}
}

// retryStateListsMechanicalClaimHint reports whether the typed retry
// surface carries the mechanical-claim kind — i.e. the dispatch
// consuming that surface received the hint.
func retryStateListsMechanicalClaimHint(rs *types.RetryState) bool {
	if rs == nil {
		return false
	}
	for _, v := range rs.ActiveViolations {
		if v.Kind == types.ViolMechanicalClaimContradiction {
			return true
		}
	}
	return false
}

// mechanicalClaimResidualCaveatMessage renders the ship-time disclosure
// for decisive-tier findings still present on the shipped document.
// Wording audited against the caveat red lines (no internal pipeline
// vocabulary; CN branch stays CN).
func mechanicalClaimResidualCaveatMessage(lang string, findings []mechanicalClaimFinding) string {
	if len(findings) == 0 {
		return ""
	}
	items := make([]string, 0, len(findings))
	if isChineseLang(lang) {
		for _, f := range findings {
			items = append(items, f.entryZH)
		}
		return "以下断言中两个数值的比较方向与其数值本身不一致，请谨慎采信：" + strings.Join(items, "；") + "。"
	}
	for _, f := range findings {
		items = append(items, f.entry)
	}
	return "In the following statements the written comparison direction disagrees with the two numbers themselves; treat them with caution: " + strings.Join(items, "; ") + "."
}

// appendMechanicalClaimResidualCaveatToAnswer is the ship-time
// chokepoint for the decisive-tier disclosure. Activation reads PRECISE
// typed signals only: the run-level latch says the single hint round was
// delivered, and the re-scan of the SHIPPED document — with the SAME
// Scan functions the hint round used — still finds a decisive-tier
// contradiction. Runs where the lane never fired return the answer
// unchanged, byte for byte. Structurally livelock-free: this helper only
// APPENDS a disclosure at final assembly, raises no violation, requests
// no retry; dedup rides the CAVSTR replay register.
func (o *Orchestrator) appendMechanicalClaimResidualCaveatToAnswer(answer string) string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return answer
	}
	mut := o.busCtx.Mutable
	if !mut.MechanicalClaimHintDelivered() {
		return answer
	}
	var decisive []mechanicalClaimFinding
	for _, f := range scanMechanicalClaimFindings(mut.ShippedAnswerDocumentV2()) {
		if f.decisive {
			decisive = append(decisive, f)
		}
	}
	if len(decisive) == 0 {
		return answer
	}
	return o.appendRegisteredAnswerCaveatBullet(answer,
		mechanicalClaimResidualCaveatMessage(o.busCtx.Language, decisive))
}

// ─── numeric_direction family ────────────────────────────────────────

// scanNumericDirectionClaims is the first family: (quantity, comparator,
// quantity) direction claims. Conservative by construction — every
// doubtful extraction skips (each skip predicate is pinned by its own
// table-test case in mechanical_claim_check_test.go):
//
//	① modal/conditional wording in the comparator's leash region — a
//	  rule statement ("帧预算要求不应超过 16.67ms") is not a fact claim;
//	② question sentences;
//	③ range endpoints never serve as a comparison side;
//	④ near values (within one ulp, or under the relative margin);
//	⑤ approximation-marked sides use the wider approx margin;
//	⑥ a quantity token contested by two comparators voids every
//	  binding it appears in (binding ambiguity);
//	⑦ zero values carry no measurement claim (PSG ruling reused);
//	⑧ exempt blocks never reach this scanner (collectModelProseUnits);
//	⑨ (R6-4) operand binding stops at clause boundaries (，。；、,;.)
//	  — a monotone gap barrier (side-fatal: rejecting the nearest
//	  candidate rejects the whole side), so an attributive comparator
//	  with no in-clause operand is skipped, never bound across the
//	  boundary;
//	⑩ (R6-3) an ambiguous negation reading (a 不/未/not token that is
//	  neither a closed non-negating compound nor immediately before
//	  the comparator) skips the claim — never flip on a guess;
//	⑪ (R9-0/R9-2) a 在 with ANY later 内/中/里 rune before the
//	  comparator makes the left-operand binding AMBIGUOUS and the
//	  claim skips — the rounds-6..9 adverbial-parsing machinery
//	  (paired-在 recognition, clause walking) is retired; see
//	  mechanicalClaimZaiAdverbialAmbiguous;
//	⑫ (R9-1) sentences beyond mechanicalClaimSentenceRuneCap runes
//	  are skipped wholesale — degenerate generations are not
//	  legitimate claim carriers, and the cap is the lane's work bound.
func scanNumericDirectionClaims(unit proseTextUnit, spans [][2]int) []mechanicalClaimFinding {
	text := unit.text
	if strings.TrimSpace(text) == "" {
		return nil
	}
	toks := extractMechanicalClaimTokens(text)
	if len(toks) == 0 {
		return nil
	}
	var out []mechanicalClaimFinding
	for _, span := range spans {
		sent := text[span[0]:span[1]]
		// Skip ⑫ (R9-1): over-cap sentences are skipped wholesale BEFORE
		// any per-sentence machinery runs — this one check is what keeps
		// the pass linear for degenerate inputs, on the hint round and
		// the ship-time rescan alike (same Scan function).
		if utf8.RuneCountInString(sent) > mechanicalClaimSentenceRuneCap {
			continue
		}
		// Skip ②: question sentences make no fact claim.
		if strings.ContainsAny(sent, "?？") {
			continue
		}
		// Sentence-local token view (byte offsets rebased to the span).
		var st []mechanicalClaimToken
		for _, tok := range toks {
			if tok.numStart >= span[0] && tok.unitEnd <= span[1] {
				local := tok
				local.numStart -= span[0]
				local.unitEnd -= span[0]
				st = append(st, local)
			}
		}
		if len(st) < 2 {
			continue
		}
		comps := mechanicalClaimComparatorsInSentence(sent)
		if len(comps) == 0 {
			continue
		}
		lower := mechanicalClaimASCIILower(sent)
		// Skip ①: any comparator whose leash region carries a modal /
		// conditional marker voids the WHOLE sentence — rule statements
		// and hypotheticals routinely restate both numbers.
		modal := false
		for _, c := range comps {
			if mechanicalClaimModalNearComparator(sent, lower, c) {
				modal = true
				break
			}
		}
		if modal {
			continue
		}
		// Bind each comparator independently: A = nearest token strictly
		// left, B = nearest token strictly right, each within the rune
		// leash. Any missing side, over-leash gap, range endpoint,
		// cross-dimension pair or zero side discards THAT comparator.
		// R6-4 (round-6 sweep): binding additionally stops at clause
		// boundaries (，。；、,;.) — a GAP predicate, so the rejection is
		// side-fatal: the nearest candidate carries a SUBSET of any
		// farther candidate's gap, so a barrier that rejects the nearest
		// candidate rejects every farther one too. The rounds-7/8
		// adverbial-closer gap barrier that used to sit beside it is
		// retired (R9): 在…内/中/里 adverbial shapes are handled by the
		// comparator-level ambiguity skip stamped in
		// mechanicalClaimComparatorsInSentence — the comparator still
		// binds here so its operands feed the contested-token census,
		// but it never yields a finding.
		type mccBinding struct {
			comp        mechanicalClaimComparatorMatch
			left, right int
		}
		var binds []mccBinding
		useCount := map[int]int{}
		for _, c := range comps {
			li, ri := -1, -1
			for i, tok := range st {
				if tok.unitEnd <= c.start &&
					!mechanicalClaimClauseBarrierBetween(sent, tok.unitEnd, c.start) &&
					(li == -1 || tok.unitEnd > st[li].unitEnd) {
					li = i
				}
				if tok.numStart >= c.end &&
					!mechanicalClaimClauseBarrierBetween(sent, c.end, tok.numStart) &&
					(ri == -1 || tok.numStart < st[ri].numStart) {
					ri = i
				}
			}
			if li == -1 || ri == -1 {
				continue
			}
			if utf8.RuneCountInString(sent[st[li].unitEnd:c.start]) > mechanicalClaimBindGapRunes ||
				utf8.RuneCountInString(sent[c.end:st[ri].numStart]) > mechanicalClaimBindGapRunes {
				continue
			}
			a, b := st[li], st[ri]
			if a.rangeEnd || b.rangeEnd { // skip ③
				continue
			}
			if a.dim != b.dim { // same-dimension only, structurally
				continue
			}
			if a.value == 0 || b.value == 0 { // skip ⑦
				continue
			}
			binds = append(binds, mccBinding{comp: c, left: li, right: ri})
			useCount[li]++
			useCount[ri]++
		}
		for _, bnd := range binds {
			// Skip ⑥: contested token → binding ambiguity → discard.
			if useCount[bnd.left] > 1 || useCount[bnd.right] > 1 {
				continue
			}
			// Ambiguity skips the claim entirely while the binding above
			// still fed the contested-token census (more conservative,
			// never less): R6-3 ambiguous negation readings and R9
			// possible 在…内/中/里 adverbial coverage of the left operand
			// both arrive here as comp.skip.
			if bnd.comp.skip {
				continue
			}
			claimedGT := bnd.comp.gt != bnd.comp.negated
			if f, ok := mechanicalClaimJudgeDirection(unit.blockID, sent, st[bnd.left], st[bnd.right], claimedGT); ok {
				out = append(out, f)
			}
		}
	}
	return out
}

// mechanicalClaimJudgeDirection applies the arithmetic verdict with the
// magnitude tiers (§4.3/§4.4): near values and sub-margin reversals
// skip (④/⑤), the absolute floor kills tiny-value relative inflation,
// and the decisive flag marks ≥2× reversals for the ship-time caveat.
func mechanicalClaimJudgeDirection(blockID, sent string, a, b mechanicalClaimToken, claimedGT bool) (mechanicalClaimFinding, bool) {
	vA, vB := a.value, b.value
	diff := math.Abs(vA - vB)
	maxUlp := a.ulp
	if b.ulp > maxUlp {
		maxUlp = b.ulp
	}
	if diff <= maxUlp { // skip ④: rounding-distance values carry no direction
		return mechanicalClaimFinding{}, false
	}
	larger, smaller := vA, vB
	if vB > vA {
		larger, smaller = vB, vA
	}
	margin := mechanicalClaimDirectionRelMargin
	if a.approx || b.approx { // skip ⑤: approx sides get the wider margin
		margin = mechanicalClaimApproxDirectionRelMargin
	}
	if diff/larger <= margin {
		return mechanicalClaimFinding{}, false
	}
	floor := mechanicalClaimTimeFloorMS
	if a.dim == dimPercent {
		floor = mechanicalClaimPercentFloorP
	}
	if diff <= floor {
		return mechanicalClaimFinding{}, false
	}
	if (claimedGT && vA >= vB) || (!claimedGT && vA <= vB) {
		return mechanicalClaimFinding{}, false // direction agrees — no story
	}
	unitLabel := "ms"
	if a.dim == dimPercent {
		unitLabel = "%"
	}
	dispA := strconv.FormatFloat(vA, 'f', -1, 64) + unitLabel
	dispB := strconv.FormatFloat(vB, 'f', -1, 64) + unitLabel
	fragment := strings.TrimSpace(sent[a.numStart:b.unitEnd])
	f := mechanicalClaimFinding{
		blockID:  blockID,
		decisive: smaller > 0 && larger/smaller >= mechanicalClaimDecisiveRatio,
	}
	if claimedGT {
		f.entry = fmt.Sprintf("block %q states 「%s」 — as written the claim is %s > %s, but the numbers themselves give %s < %s (both normalized to one unit)",
			blockID, fragment, dispA, dispB, dispA, dispB)
		f.entryZH = fmt.Sprintf("块 %q 写道「%s」——句面声称 %s 超过 %s，但两个数值本身是 %s < %s（已换算到同一单位）",
			blockID, fragment, dispA, dispB, dispA, dispB)
	} else {
		f.entry = fmt.Sprintf("block %q states 「%s」 — as written the claim is %s ≤ %s, but the numbers themselves give %s > %s (both normalized to one unit)",
			blockID, fragment, dispA, dispB, dispA, dispB)
		f.entryZH = fmt.Sprintf("块 %q 写道「%s」——句面声称 %s 不超过 %s，但两个数值本身是 %s > %s（已换算到同一单位）",
			blockID, fragment, dispA, dispB, dispA, dispB)
	}
	return f, true
}

// ─── comparator location, negation and modal detection ───────────────

// mechanicalClaimComparatorMatch is one located comparator occurrence.
// skip marks an AMBIGUOUS negation reading (R6-3 precision rule): the
// comparator stays in the list — its bindings still count toward the
// contested-token predicate — but it never yields a finding.
type mechanicalClaimComparatorMatch struct {
	start, end int
	gt         bool
	negated    bool
	skip       bool
	word       string
}

// mechanicalClaimComparatorsInSentence locates every closed-set
// comparator occurrence, resolves overlaps longest-first ("no more
// than" beats its embedded "more than"), applies the 之内/以内
// admission gate, and stamps negation.
func mechanicalClaimComparatorsInSentence(sent string) []mechanicalClaimComparatorMatch {
	lower := mechanicalClaimASCIILower(sent)
	var raw []mechanicalClaimComparatorMatch
	for _, cw := range mechanicalClaimComparatorWords {
		hay := sent
		if cw.ascii {
			hay = lower
		}
		from := 0
		for {
			idx := strings.Index(hay[from:], cw.word)
			if idx < 0 {
				break
			}
			start := from + idx
			end := start + len(cw.word)
			from = start + 1
			if cw.ascii && !mechanicalClaimASCIIBoundary(lower, start, end) {
				continue
			}
			// 之内/以内 admission: only the "在…之内 / 在…以内"
			// comparison construction counts — bare temporal adverbials
			// (「三天之内完成」) stay out.
			if (cw.word == "之内" || cw.word == "以内") && !mechanicalClaimWithinAdmitted(sent, start) {
				continue
			}
			raw = append(raw, mechanicalClaimComparatorMatch{start: start, end: end, gt: cw.gt, word: cw.word})
		}
	}
	// Longest-first greedy overlap resolution.
	sort.SliceStable(raw, func(i, j int) bool {
		li, lj := raw[i].end-raw[i].start, raw[j].end-raw[j].start
		if li != lj {
			return li > lj
		}
		return raw[i].start < raw[j].start
	})
	var kept []mechanicalClaimComparatorMatch
	for _, c := range raw {
		overlaps := false
		for _, k := range kept {
			if c.start < k.end && k.start < c.end {
				overlaps = true
				break
			}
		}
		if !overlaps {
			kept = append(kept, c)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].start < kept[j].start })
	for i := range kept {
		switch mechanicalClaimComparatorNegation(sent, lower, kept[i]) {
		case mccNegationFlip:
			kept[i].negated = true
		case mccNegationAmbiguous:
			kept[i].skip = true
		}
		// R9-0/R9-2 conservative ambiguity rule: a 在 with any later
		// 内/中/里 before the comparator may put the left operand inside
		// an adverbial — skip the claim (binding still feeds the
		// contested-token census).
		if !kept[i].skip && mechanicalClaimZaiAdverbialAmbiguous(sent, kept[i].start) {
			kept[i].skip = true
		}
	}
	return kept
}

// mechanicalClaimNegationVerdict is the three-way outcome of the
// negation reading (R6-3, round-6 sweep): none (base direction stands),
// flip (a genuine negation immediately before the comparator), or
// AMBIGUOUS — a negation-like token whose scope cannot be established
// precisely. Ambiguity never flips AND never judges: the claim is
// skipped entirely (precision over recall — a wrong flip manufactures a
// contradiction on a correct sentence).
type mechanicalClaimNegationVerdict int

const (
	mccNegationNone mechanicalClaimNegationVerdict = iota
	mccNegationFlip
	mccNegationAmbiguous
)

// mechanicalClaimComparatorNegation resolves the negation reading for
// one comparator: zh first (the ≤4-rune punctuation-barriered window
// immediately before the base word — 「与预期不同，超过…」 must NOT read
// the 不 of 不同 as negation), then en (the ≤3 words immediately before
// the base word, parentheticals stripped first).
func mechanicalClaimComparatorNegation(sent, lower string, c mechanicalClaimComparatorMatch) mechanicalClaimNegationVerdict {
	start := c.start
	for count := 0; count < mechanicalClaimNegationGapRunesZH && start > 0; count++ {
		r, size := utf8.DecodeLastRuneInString(sent[:start])
		if mechanicalClaimNegationBarrier(r) {
			break
		}
		start -= size
	}
	if v := mechanicalClaimNegationVerdictZH(sent[start:c.start]); v != mccNegationNone {
		return v
	}
	return mechanicalClaimNegationVerdictEN(lower[:c.start])
}

// mechanicalClaimNegationVerdictZH scans the pre-comparator window
// compound-aware (R6-3): 不/未 are single-character SUBSTRINGS of many
// non-negating words, so a bare Contains over the window flipped
// correct sentences (不仅超过 read as negated 超过). Rules, in order:
//   - a closed non-negating compound (不仅/不但/不过/不止/不光) is read
//     over and never flips;
//   - a negation word IMMEDIATELY before the comparator (whitespace
//     only in between) flips;
//   - any other negation-like token in the window is AMBIGUOUS — skip
//     the claim (不再超过 / 未能超过: never flip on a guess).
func mechanicalClaimNegationVerdictZH(window string) mechanicalClaimNegationVerdict {
	i := 0
	for i < len(window) {
		neg := ""
		for _, cand := range mechanicalClaimNegationsZH {
			if strings.HasPrefix(window[i:], cand) {
				neg = cand
				break
			}
		}
		if neg == "" {
			_, size := utf8.DecodeRuneInString(window[i:])
			i += size
			continue
		}
		compound := ""
		for _, cand := range mechanicalClaimNonNegatingCompoundsZH {
			if strings.HasPrefix(window[i:], cand) {
				compound = cand
				break
			}
		}
		if compound != "" {
			i += len(compound)
			continue
		}
		if strings.TrimSpace(window[i+len(neg):]) == "" {
			return mccNegationFlip
		}
		return mccNegationAmbiguous
	}
	return mccNegationNone
}

// mechanicalClaimNegationVerdictEN reads the last ≤3 words before the
// comparator AFTER stripping balanced parentheticals (R6-3: a
// parenthetical aside — "(not counting GC)" — is not comparator
// negation). Rules, in order:
//   - an unclosed "(" before the comparator puts the comparator inside
//     a parenthetical — AMBIGUOUS, skip;
//   - "not only/just/merely" is a correlative, never negation;
//   - a negation word IMMEDIATELY before the comparator flips ("does
//     not exceed", "never exceeds", "no longer exceeds");
//   - any other negation-like word in the window is AMBIGUOUS — skip
//     the claim ("not counting GC exceeds": never flip on a guess).
func mechanicalClaimNegationVerdictEN(prefixLower string) mechanicalClaimNegationVerdict {
	stripped, unclosed := mechanicalClaimStripParentheticals(prefixLower)
	if unclosed {
		return mccNegationAmbiguous
	}
	fields := strings.Fields(stripped)
	if len(fields) > mechanicalClaimNegationGapWordsEN {
		fields = fields[len(fields)-mechanicalClaimNegationGapWordsEN:]
	}
	words := make([]string, 0, len(fields))
	for _, w := range fields {
		words = append(words, strings.Trim(w, ",.;:!()\"'"))
	}
	for i, w := range words {
		if w == "no" && i+1 < len(words) && words[i+1] == "longer" {
			if i+1 == len(words)-1 {
				return mccNegationFlip
			}
			return mccNegationAmbiguous
		}
		isNeg := false
		for _, neg := range mechanicalClaimNegationsEN {
			if w == neg {
				isNeg = true
				break
			}
		}
		if !isNeg {
			continue
		}
		if w == "not" && i+1 < len(words) && mechanicalClaimNonNegatingFollowersEN[words[i+1]] {
			continue
		}
		if i == len(words)-1 {
			return mccNegationFlip
		}
		return mccNegationAmbiguous
	}
	return mccNegationNone
}

// mechanicalClaimStripParentheticals removes balanced ( … ) / （ … ）
// spans (nesting included) and reports whether an opening parenthesis
// was left unclosed — i.e. the position the prefix ends at (the
// comparator) sits inside a parenthetical.
func mechanicalClaimStripParentheticals(s string) (string, bool) {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '(', '（':
			depth++
			continue
		case ')', '）':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 {
			b.WriteRune(r)
		}
	}
	return b.String(), depth > 0
}

// mechanicalClaimClauseBarrierRune — R6-4: the closed clause-boundary
// set operand binding never crosses. Deliberately narrower than the
// negation-window barrier: parentheses stay OUT (the F7 witness binds
// through （80ms）未超过（16.67ms）).
func mechanicalClaimClauseBarrierRune(r rune) bool {
	switch r {
	case '，', '。', '；', '、', ',', ';', '.':
		return true
	}
	return false
}

// mechanicalClaimClauseBarrierBetween reports a clause boundary inside
// sent[from:to] (the gap between an operand candidate and the
// comparator).
func mechanicalClaimClauseBarrierBetween(sent string, from, to int) bool {
	if from >= to || from < 0 || to > len(sent) {
		return false
	}
	for _, r := range sent[from:to] {
		if mechanicalClaimClauseBarrierRune(r) {
			return true
		}
	}
	return false
}

// mechanicalClaimZaiAdverbialAmbiguous — R9-0/R9-2 (round-9 sweep): the
// SIMPLIFICATION that ends the rounds-6..9 whack-a-mole on Chinese
// 在…内/中/里 adverbial parsing (per-candidate 在-prefix rejection →
// paired-在 gap barrier, leash-bounded → clause-walk-bounded pairing —
// each round's added precision machinery manufactured the NEXT round's
// decisive false contradiction on a correct sentence, and the clause
// walk was additionally O(n²) per degenerate sentence). Per the lane's
// precision-over-recall charter the arm no longer tries to RESOLVE the
// adverbial at all: when the sentence prefix before the comparator
// contains a 在 with ANY later 内/中/里 rune, an adverbial scope MAY
// cover the left operand, so the binding is declared AMBIGUOUS and the
// claim skips (via comp.skip — the binding still feeds the
// contested-token census, more conservative, never less). One cheap
// forward scan: no pairing precision, no clause walking, nothing to
// sever on enumeration 、 / version dots / long descriptors / an elided
// second 在. The trivially decidable word-internal shape stays honest:
// with NO 在 in the prefix a bare 内/中/里 (内存 / 其中 / (内部含 GC))
// cannot open an adverbial, so the binding stands (pinned by
// TestMechanicalClaim_AdverbialCloserNeedsPairedZai). Recorded
// residual: a sentence whose adverbial opener 在 is elided ENTIRELY
// (「压测过程中，10s 的采样窗口内…」 with no 在 anywhere) is
// indistinguishable from a word-internal closer without exactly the
// pairing precision this rule retires — it still binds.
func mechanicalClaimZaiAdverbialAmbiguous(sent string, compStart int) bool {
	if compStart < 0 || compStart > len(sent) {
		return false
	}
	prefix := sent[:compStart]
	zi := strings.Index(prefix, "在")
	if zi < 0 {
		return false
	}
	return strings.ContainsAny(prefix[zi+len("在"):], "内中里")
}

// mechanicalClaimNegationBarrier — punctuation the zh negation window
// never crosses.
func mechanicalClaimNegationBarrier(r rune) bool {
	switch r {
	case '，', '。', '；', '：', '、', '（', '）', '「', '」', '【', '】', '！', '？',
		',', '.', ';', ':', '(', ')', '!', '?':
		return true
	}
	return false
}

// mechanicalClaimWithinAdmitted — the 之内/以内 comparator ADMISSION
// gate's own predicate: a 在 anywhere earlier in the sentence marks the
// 「在 X 之内 / 在 X 以内」 comparison construction; bare temporal
// adverbials (「三天之内完成」) stay out. R9 (round-9 sweep): this is a
// deliberately simple sentence-bounded backward scan. Rounds 7-8 made
// this predicate the shared paired-在 recognizer for the adverbial gap
// barrier as well (leash-bounded, then clause-walking with
// numeric-internal-punctuation transparency), and the walk kept
// severing on legitimate descriptor content (enumeration 、, non-digit
// '.', an elided second 在) while going O(n²) on degenerate sentences —
// that machinery is retired. The former barrier consumer is replaced by
// the mechanicalClaimZaiAdverbialAmbiguous skip rule and no longer
// shares this predicate; admitting a 在 from an earlier clause at worst
// admits a comparator that the ambiguity rule or the operand binding
// then discards.
func mechanicalClaimWithinAdmitted(sent string, compStart int) bool {
	if compStart < 0 || compStart > len(sent) {
		return false
	}
	return strings.Contains(sent[:compStart], "在")
}

// mechanicalClaimModalNearComparator reports whether the comparator's
// leash region (mechanicalClaimBindGapRunes runes each side) carries a
// modal / conditional marker — skip predicate ①.
func mechanicalClaimModalNearComparator(sent, lower string, c mechanicalClaimComparatorMatch) bool {
	start := c.start
	for count := 0; count < mechanicalClaimBindGapRunes && start > 0; count++ {
		_, size := utf8.DecodeLastRuneInString(sent[:start])
		start -= size
	}
	end := c.end
	for count := 0; count < mechanicalClaimBindGapRunes && end < len(sent); count++ {
		_, size := utf8.DecodeRuneInString(sent[end:])
		end += size
	}
	region := sent[start:end]
	for _, w := range mechanicalClaimModalWordsZH {
		if strings.Contains(region, w) {
			return true
		}
	}
	lowerRegion := lower[start:end]
	for _, w := range mechanicalClaimModalWordsEN {
		if mechanicalClaimASCIIWordInRegion(lowerRegion, w) {
			return true
		}
	}
	return false
}

// mechanicalClaimASCIIWordInRegion reports a word-bounded occurrence of
// an (already lowercased) ASCII keyword in a lowercased region.
func mechanicalClaimASCIIWordInRegion(lowerRegion, word string) bool {
	from := 0
	for {
		idx := strings.Index(lowerRegion[from:], word)
		if idx < 0 {
			return false
		}
		start := from + idx
		if mechanicalClaimASCIIBoundary(lowerRegion, start, start+len(word)) {
			return true
		}
		from = start + 1
	}
}

// mechanicalClaimASCIIBoundary — ASCII word boundaries on both sides.
func mechanicalClaimASCIIBoundary(s string, start, end int) bool {
	if start > 0 {
		p := s[start-1]
		if p == '_' || (p >= 'a' && p <= 'z') || (p >= 'A' && p <= 'Z') || (p >= '0' && p <= '9') {
			return false
		}
	}
	if end < len(s) {
		n := s[end]
		if n == '_' || (n >= 'a' && n <= 'z') || (n >= 'A' && n <= 'Z') || (n >= '0' && n <= '9') {
			return false
		}
	}
	return true
}

// mechanicalClaimASCIILower lowercases ASCII letters only — byte length
// (and therefore every offset) is preserved exactly.
func mechanicalClaimASCIILower(s string) string {
	lowered := []byte(s)
	changed := false
	for i := 0; i < len(lowered); i++ {
		if c := lowered[i]; c >= 'A' && c <= 'Z' {
			lowered[i] = c + ('a' - 'A')
			changed = true
		}
	}
	if !changed {
		return s
	}
	return string(lowered)
}

// ─── extraction helpers ──────────────────────────────────────────────

// extractMechanicalClaimTokens applies the unit-scoped regex with the
// PSG-style manual guards: a numeral glued to a preceding
// letter/digit/./-/_/, is an identifier fragment or a truncated
// thousands form; an ASCII s-family unit glued to a following letter is
// a word ("5spec"); the bare "s" unit additionally requires a
// non-alphanumeric right neighbour ("K3s"-class readings die on the
// left guard already, "10s3"-class on this one).
func extractMechanicalClaimTokens(text string) []mechanicalClaimToken {
	var out []mechanicalClaimToken
	for _, m := range mechanicalClaimQuantityRE.FindAllStringSubmatchIndex(text, -1) {
		if len(out) >= mechanicalClaimScanTokenCap {
			break
		}
		numStart, numEnd := m[2], m[3]
		unitStart, unitEnd := m[4], m[5]
		if numStart > 0 {
			prev := text[numStart-1]
			if prev == '.' || prev == '-' || prev == '_' || prev == ',' ||
				(prev >= '0' && prev <= '9') ||
				(prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') {
				continue
			}
		}
		unitWord := text[unitStart:unitEnd]
		spec, ok := mechanicalClaimUnits[unitWord]
		if !ok {
			continue
		}
		if strings.HasSuffix(unitWord, "s") && unitEnd < len(text) {
			next := text[unitEnd]
			if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || next == '_' {
				continue
			}
			if unitWord == "s" && next >= '0' && next <= '9' {
				continue
			}
		}
		raw := text[numStart:numEnd]
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		out = append(out, mechanicalClaimToken{
			raw:      raw,
			unit:     unitWord,
			dim:      spec.dim,
			numStart: numStart,
			unitEnd:  unitEnd,
			value:    value * spec.factor,
			ulp:      proseScalarUlp(raw) * spec.factor,
			approx:   proseScalarApproxMarked(text, numStart),
			rangeEnd: mechanicalClaimRangeAdjacent(text, numStart, unitEnd),
		})
	}
	return out
}

// mechanicalClaimRangeConnectors join two numerals into a range
// ("10–20ms", "3680.569..3682.819s"). A quantity adjacent to one is a
// range ENDPOINT and never serves as a comparison side.
var mechanicalClaimRangeConnectors = []string{"–", "—", "~", "～", "-", ".."}

// mechanicalClaimRangeAdjacent reports whether the numeral at
// [numStart, …] / unit ending at unitEnd sits directly on a range
// connector that joins it to another numeral (or unit-suffixed numeral).
func mechanicalClaimRangeAdjacent(text string, numStart, unitEnd int) bool {
	// Backward: … <digit|unit> <connector> [spaces] <numeral …
	back := strings.TrimRight(text[:numStart], " \t　")
	for _, conn := range mechanicalClaimRangeConnectors {
		if !strings.HasSuffix(back, conn) {
			continue
		}
		before := strings.TrimRight(strings.TrimSuffix(back, conn), " \t　")
		if before == "" {
			continue
		}
		if last := before[len(before)-1]; last >= '0' && last <= '9' {
			return true
		}
		for unitWord := range mechanicalClaimUnits {
			if strings.HasSuffix(before, unitWord) {
				return true
			}
		}
	}
	// Forward: … numeral+unit [spaces] <connector> [spaces] <digit> …
	fwd := strings.TrimLeft(text[unitEnd:], " \t　")
	for _, conn := range mechanicalClaimRangeConnectors {
		if !strings.HasPrefix(fwd, conn) {
			continue
		}
		after := strings.TrimLeft(strings.TrimPrefix(fwd, conn), " \t　")
		if after != "" && after[0] >= '0' && after[0] <= '9' {
			return true
		}
	}
	return false
}

// ─── comparator lexicon (closed set, one definition) ─────────────────

// mechanicalClaimComparatorWord is one closed-set comparator base word.
// gt=true claims A > B; gt=false claims A ≤ B (GE/LT collapse into
// these two lanes through negation — the magnitude margins make the
// equality boundary irrelevant). Deliberately NOT in the set: 快于/慢于
// (duration-semantics inversion ambiguity), over/under (English
// preposition noise), bare 多/少 (no comparison structure).
type mechanicalClaimComparatorWord struct {
	word  string
	gt    bool
	ascii bool // needs ASCII word-boundary + case-insensitive matching
}

var mechanicalClaimComparatorWords = []mechanicalClaimComparatorWord{
	// zh, A > B
	{word: "超过", gt: true}, {word: "超出", gt: true}, {word: "高于", gt: true},
	{word: "大于", gt: true}, {word: "多于", gt: true}, {word: "长于", gt: true},
	// zh, A ≤ B (之内/以内 additionally require a preceding 在 — see
	// mechanicalClaimWithinAdmitted)
	{word: "低于", gt: false}, {word: "小于", gt: false}, {word: "少于", gt: false},
	{word: "短于", gt: false}, {word: "不足", gt: false},
	{word: "之内", gt: false}, {word: "以内", gt: false},
	// en, A > B ("exceed" covers the negated "does not exceed" form)
	{word: "exceeds", gt: true, ascii: true}, {word: "exceeded", gt: true, ascii: true},
	{word: "exceeding", gt: true, ascii: true}, {word: "exceed", gt: true, ascii: true},
	{word: "greater than", gt: true, ascii: true}, {word: "more than", gt: true, ascii: true},
	{word: "higher than", gt: true, ascii: true}, {word: "longer than", gt: true, ascii: true},
	{word: "above", gt: true, ascii: true},
	// en, A ≤ B ("no more than" outranks "more than" via longest-first
	// overlap resolution)
	{word: "no more than", gt: false, ascii: true}, {word: "less than", gt: false, ascii: true},
	{word: "lower than", gt: false, ascii: true}, {word: "below", gt: false, ascii: true},
	{word: "shorter than", gt: false, ascii: true}, {word: "within", gt: false, ascii: true},
	{word: "at most", gt: false, ascii: true},
}

// Negation prefixes flip the base word's claimed direction to its
// complement (未超过 → A ≤ B; does not exceed → A ≤ B). Longest first:
// the compound-aware scanner matches by prefix.
var (
	mechanicalClaimNegationsZH = []string{"并未", "并不", "没有", "未", "不"}
	mechanicalClaimNegationsEN = []string{"not", "never", "doesn't"}
)

// mechanicalClaimNonNegatingCompoundsZH — R6-3 closed set: words whose
// leading 不 is NOT negation (correlatives / conjunctions). The scanner
// reads over them; every OTHER 不/未-led token that is not immediately
// before the comparator is ambiguous and skips the claim, so a missing
// compound here costs recall, never precision.
var mechanicalClaimNonNegatingCompoundsZH = []string{"不仅", "不但", "不过", "不止", "不光"}

// mechanicalClaimNonNegatingFollowersEN — "not only/just/merely" are
// correlatives, not comparator negation.
var mechanicalClaimNonNegatingFollowersEN = map[string]bool{"only": true, "just": true, "merely": true}

// Modal / conditional markers: a comparator whose leash region carries
// one is a RULE or HYPOTHESIS, not a fact claim — the whole sentence
// skips (skip predicate ①).
var (
	mechanicalClaimModalWordsZH = []string{"不应", "应该", "应", "必须", "须", "需", "建议", "如果", "若", "假设", "一旦"}
	mechanicalClaimModalWordsEN = []string{"should", "must", "would", "could", "may", "might", "if", "unless", "expected to"}
)
