package orchestrator

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
)

// prose_scalar_caliber_check.go — QH2-B caliber-word audit (§29.79 观察续档
// 立案, docs/design/real_trace_campaign_20260705.md, 2026-07-15).
//
// Witnesses:
//   - 「全额→满额」 (§29.79): the prose paraphrased the seat fact's caliber
//     word 全额 into 满额 with the VALUE intact — every value-membership arm
//     stayed silent (值零损) while the account the number belongs to
//     silently changed meaning, and no audit lane had a caliber-word face.
//   - h9 趟1 (§29.101 残留, artifact 20260715-150233.691-47104): the prose
//     called the RAW running magnitude 143.499ms a 「折算席位」 while the
//     evidence face published it as 「running 原始 143.499ms → 计入
//     51.735ms(折算,…)」 — the value grounded fine, the caliber word was
//     transplanted from the neighbouring account.
//
// Two arms, both INFORMATION-only (the same advisory slice as the F-2④
// self-sum disclosures — appendix face at ship time, never a violation,
// never a rewrite round; 检测→披露, 禁硬拦):
//
//	arm A (never-published word, directly decidable): a word from the
//	  tracefence never-published near-synonym list (满额-class) next to a
//	  grounded magnitude — the engine never prints these words, so the
//	  word-list membership is a precise signal; the adjacency extraction
//	  is noisy, which is why the verdict still only feeds the soft lane.
//	arm B (published-word contradiction, all-pairings form): the prose
//	  puts a PUBLISHED caliber word next to a value, the evidence
//	  surfaces publish that value with caliber words, and NONE of those
//	  pairings carries the prose's word — the finding states the
//	  evidence-side fact only (「…在证据面以口径词「原始」发布」, the
//	  juxtaposition doctrine: the system never characterizes the prose).
//	  A value with no published caliber pairing stays silent (宁松勿严),
//	  and any agreement silences the token.
//
// Word bytes: tracefence Table ③c single source (CaliberWordFacesZH /
// CaliberWordNeverPublishedZH) — shared with the display emission faces and
// the seat-composition feed, so the three faces can never hand-mirror
// apart.

const (
	// proseScalarCaliberWordLeash is the PROSE-side byte distance within
	// which a caliber word and a magnitude read as one claim (CJK runs 3
	// bytes per character; the witnessed forms — 「折算席位 143.499ms」 /
	// 「反转等待(满额) 0.109ms」 — sit well inside it). Loose on purpose:
	// more prose words can only ADD an agreement and silence the audit.
	proseScalarCaliberWordLeash = 30
	// proseScalarCaliberPairLeash is the EVIDENCE-side pairing distance —
	// tight, because pairings assert what the engine published: the engine
	// row shapes put the word flush against its value (「原始 143.499ms」=1,
	// 「(全额) 1.023ms」=2, 「7.296ms 下界」=3, 「51.735ms(折算」=3), while a
	// NEIGHBOURING account's word sits ≥14 bytes away (行3 「有效归因
	// 3.399ms = runnable(全额)…」 must NOT pair 3.399 with 全额 — the
	// composite value carries no single caliber word).
	proseScalarCaliberPairLeash = 8
	// proseScalarCaliberBindingCap bounds the evidence-side pairing pool.
	proseScalarCaliberBindingCap = 4096
)

// proseScalarCaliberBinding is one published (value, caliber word) pairing.
type proseScalarCaliberBinding struct {
	value float64
	word  string
}

// proseScalarCaliberWordRef is one caliber-word occurrence in a text.
type proseScalarCaliberWordRef struct {
	Word   string
	Banned bool // member of the never-published near-synonym list
	Start  int
	End    int
	// FollowOnly (155119 复放趟2 live 7.305 witness): the occurrence is a
	// transition connective — 「折算后 X」 claims the FOLLOWING value is
	// post-fold; it never words the value BEFORE it (the witnessed honest
	// sentence 「running 7.305ms,折算后…4.958ms」 had the raw value
	// upstream and the folded value downstream).
	FollowOnly bool
}

// proseScalarCaliberNegationPrefixes: an occurrence immediately preceded by
// a negation particle (未计入 / 不计入 / 没计入 / 并非全额) is not a caliber
// claim about the neighbouring value — skip it on both faces.
var proseScalarCaliberNegationPrefixes = []string{"未", "不", "没", "非"}

// extractProseScalarCaliberWordRefs finds every caliber-word occurrence
// (published closed set + never-published list) with the negation guard.
// Shared by the evidence-side pairing collection and the prose scan, so the
// two faces cannot diverge in shape.
func extractProseScalarCaliberWordRefs(text string) []proseScalarCaliberWordRef {
	if text == "" {
		return nil
	}
	var out []proseScalarCaliberWordRef
	scan := func(word string, banned bool) {
		for from := 0; ; {
			idx := strings.Index(text[from:], word)
			if idx < 0 {
				return
			}
			start := from + idx
			from = start + len(word)
			negated := false
			for _, neg := range proseScalarCaliberNegationPrefixes {
				if strings.HasSuffix(text[:start], neg) {
					negated = true
					break
				}
			}
			if negated {
				continue
			}
			out = append(out, proseScalarCaliberWordRef{
				Word: word, Banned: banned, Start: start, End: start + len(word),
				FollowOnly: strings.HasPrefix(text[start+len(word):], "后"),
			})
		}
	}
	for _, word := range tracefence.CaliberWordFacesZH() {
		scan(word, false)
	}
	for _, word := range tracefence.CaliberWordNeverPublishedZH() {
		scan(word, true)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// proseScalarCaliberDistance is the byte gap between a word span and a
// token span (0 when they touch or overlap).
func proseScalarCaliberDistance(ref proseScalarCaliberWordRef, tokStart, tokEnd int) int {
	switch {
	case ref.End <= tokStart:
		return tokStart - ref.End
	case ref.Start >= tokEnd:
		return ref.Start - tokEnd
	}
	return 0
}

// collectProseScalarCaliberBindings extracts one evidence TEXT's published
// (value, caliber word) pairings into the evidence set: per token, the
// NEAREST word on each side within the leash — the engine's own row shape
// 「running 原始 143.499ms → 计入 51.735ms(折算,…)」 pairs each magnitude
// with its flanking words only; an all-within-leash sweep would pair the
// raw value with the NEXT account's 折算 and silence the audit on the very
// h9 witness. Banned words never mint a pairing (the engine does not
// publish them; a stray quote must not legitimize the word).
func collectProseScalarCaliberBindings(text string, set *proseScalarEvidenceSet) {
	if text == "" || len(set.caliberBindings) >= proseScalarCaliberBindingCap {
		return
	}
	refs := extractProseScalarCaliberWordRefs(text)
	if len(refs) == 0 {
		return
	}
	for _, tok := range extractProseScalarTokens("", text) {
		if tok.Value == 0 || tok.percent() {
			continue
		}
		tokEnd := tok.Pos + len(tok.Raw)
		var before, after *proseScalarCaliberWordRef
		for i := range refs {
			ref := &refs[i]
			if ref.Banned || proseScalarCaliberDistance(*ref, tok.Pos, tokEnd) > proseScalarCaliberPairLeash {
				continue
			}
			switch {
			case ref.End <= tok.Pos:
				if before == nil || tok.Pos-ref.End < tok.Pos-before.End {
					before = ref
				}
			case ref.Start >= tokEnd:
				// A FollowOnly word after the value (「X …折算后」) points at
				// the NEXT value, never backward at X.
				if !ref.FollowOnly && (after == nil || ref.Start-tokEnd < after.Start-tokEnd) {
					after = ref
				}
			}
		}
		pair := func(word string) bool {
			if len(set.caliberBindings) >= proseScalarCaliberBindingCap {
				return false
			}
			set.caliberBindings = append(set.caliberBindings, proseScalarCaliberBinding{value: tok.Value, word: word})
			return true
		}
		for _, ref := range []*proseScalarCaliberWordRef{before, after} {
			if ref == nil {
				continue
			}
			if !pair(ref.Word) {
				return
			}
		}
		// Caliber-parenthesis chain (h9 复放实锤, 155119 趟 live finding
		// 2.286ms): the engine's CaliberFull grammar annotates a value with
		// a WHOLE parenthesized word chain — 「计入 2.286ms(折算,按全域最大核
		// 最高频,运行频点非最高,下界,…)」 — whose tail words sit far beyond
		// the flat pairing leash. Every caliber word inside the value's own
		// parenthesis pairs with it; a paren opening further away (「3.399ms
		// = runnable(全额)」) is the NEXT term's annotation and stays
		// unpaired.
		if start, end, ok := proseScalarCaliberParenSpan(text, tokEnd); ok {
			for i := range refs {
				ref := &refs[i]
				if ref.Banned || ref.Start < start || ref.End > end {
					continue
				}
				if !pair(ref.Word) {
					return
				}
			}
		}
	}
}

// proseScalarCaliberParenChainWindow / proseScalarCaliberParenChainCap: the
// opening paren must sit within the unit's width of the numeral ("ms(" = 2
// bytes; never as far as the next term), and the chain is bounded.
const (
	proseScalarCaliberParenChainWindow = 6
	proseScalarCaliberParenChainCap    = 200
)

// proseScalarCaliberParenSpan locates the caliber-annotation parenthesis
// that belongs to the token ending at tokEnd: an opening paren within the
// chain window, closed within the chain cap. Returns the byte span BETWEEN
// the parens.
func proseScalarCaliberParenSpan(text string, tokEnd int) (int, int, bool) {
	limit := tokEnd + proseScalarCaliberParenChainWindow
	if limit > len(text) {
		limit = len(text)
	}
	open, openLen := -1, 0
	for i := tokEnd; i < limit; i++ {
		if text[i] == '(' {
			open, openLen = i, 1
			break
		}
		if strings.HasPrefix(text[i:], "（") {
			open, openLen = i, len("（")
			break
		}
	}
	if open < 0 {
		return 0, 0, false
	}
	stop := open + openLen + proseScalarCaliberParenChainCap
	if stop > len(text) {
		stop = len(text)
	}
	for i := open + openLen; i < stop; i++ {
		if text[i] == ')' || strings.HasPrefix(text[i:], "）") {
			return open + openLen, i, true
		}
	}
	return 0, 0, false
}

// proseScalarCaliberWordsForValue collects the caliber words the evidence
// surfaces publish within tol of value.
func proseScalarCaliberWordsForValue(bindings []proseScalarCaliberBinding, value, tol float64) []string {
	var out []string
	seen := map[string]bool{}
	for _, b := range bindings {
		if math.Abs(b.value-value) > tol+1e-9 || seen[b.word] {
			continue
		}
		seen[b.word] = true
		out = append(out, b.word)
	}
	return out
}

// proseScalarCaliberWordListLabel renders up to two words for a finding.
func proseScalarCaliberWordListLabel(words []string, zh bool) string {
	labels := make([]string, 0, 2)
	for _, w := range words {
		if len(labels) >= 2 {
			break
		}
		if zh {
			labels = append(labels, "「"+w+"」")
		} else {
			labels = append(labels, "\""+w+"\"")
		}
	}
	return strings.Join(labels, " / ")
}

// assignProseScalarCaliberWords binds each caliber-word occurrence to its
// NEAREST scanned token (same sentence, within the prose leash) — a word
// belongs to one value, not to every value in reach (h9 复放实锤, 155119 趟
// live finding 3.429ms: the 「4.710 ms 原始值」 label 27 bytes upstream got
// read as 3.429's caliber word). Returns token-index → assigned refs.
func assignProseScalarCaliberWords(toks []proseScalarToken, refs []proseScalarCaliberWordRef, sentenceOf func(int) [2]int) map[int][]proseScalarCaliberWordRef {
	if len(toks) == 0 || len(refs) == 0 {
		return nil
	}
	out := map[int][]proseScalarCaliberWordRef{}
	for _, ref := range refs {
		span := sentenceOf(ref.Start)
		best, bestDist := -1, proseScalarCaliberWordLeash+1
		for i, tok := range toks {
			if tok.Pos < span[0] || tok.Pos >= span[1] {
				continue // same-sentence scope, like every binding arm
			}
			if ref.FollowOnly && tok.Pos < ref.End {
				continue // 「折算后 X」 words the FOLLOWING value only
			}
			if d := proseScalarCaliberDistance(ref, tok.Pos, tok.Pos+len(tok.Raw)); d < bestDist {
				best, bestDist = i, d
			}
		}
		if best >= 0 {
			out[best] = append(out[best], ref)
		}
	}
	return out
}

// proseScalarCaliberAudit runs the two caliber arms for one grounded,
// non-zero, non-percent token against the words ASSIGNED to it. Returns
// (finding, true) on a disclosure.
func proseScalarCaliberAudit(tok proseScalarToken, near []proseScalarCaliberWordRef, evidence proseScalarEvidenceSet) (proseScalarBindingFinding, bool) {
	if len(near) == 0 {
		return proseScalarBindingFinding{}, false
	}
	published := proseScalarCaliberWordsForValue(evidence.caliberBindings, tok.Value, proseScalarTokenTol(tok))
	// Arm A — never-published near-synonym next to the value: directly
	// decidable by word-list membership; name the published pairing when
	// one exists so the reader sees the real word.
	for _, ref := range near {
		if !ref.Banned {
			continue
		}
		entry := fmt.Sprintf("the caliber word \"%s\" next to %s (block %q) is not a caliber word this report's evidence surfaces publish",
			ref.Word, tok.label(), tok.BlockID)
		entryZH := fmt.Sprintf("%s（块 %s）旁的口径词「%s」未在本报告证据面出现", tok.label(), tok.BlockID, ref.Word)
		if len(published) > 0 {
			entry += fmt.Sprintf("; the value is published under the caliber word(s) %s on the evidence face", proseScalarCaliberWordListLabel(published, false))
			entryZH += fmt.Sprintf("；该值在证据面以口径词%s发布", proseScalarCaliberWordListLabel(published, true))
		}
		return proseScalarBindingFinding{entry: entry, entryZH: entryZH}, true
	}
	// Arm B — published word contradicting every published pairing of the
	// value: a value with no published pairing stays silent, any agreement
	// stays silent (宁松勿严). The finding states the evidence-side fact
	// only — the reader juxtaposes it with the body's own wording.
	if len(published) == 0 {
		return proseScalarBindingFinding{}, false
	}
	for _, ref := range near {
		for _, w := range published {
			if ref.Word == w {
				return proseScalarBindingFinding{}, false
			}
		}
	}
	return proseScalarBindingFinding{
		entry: fmt.Sprintf("%s (block %q) is published under the caliber word(s) %s on the evidence face",
			tok.label(), tok.BlockID, proseScalarCaliberWordListLabel(published, false)),
		entryZH: fmt.Sprintf("%s（块 %s）在证据面以口径词%s发布", tok.label(), tok.BlockID, proseScalarCaliberWordListLabel(published, true)),
	}, true
}
