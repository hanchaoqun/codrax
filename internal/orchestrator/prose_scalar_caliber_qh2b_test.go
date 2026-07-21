package orchestrator

// prose_scalar_caliber_qh2b_test.go — QH2-B caliber-word audit pins (§29.79
// 观察续档 + §29.101 h9 残留, docs/design/real_trace_campaign_20260705.md,
// 2026-07-15).
//
// Material (h9 趟1 artifact 20260715-150233.691-47104):
//   - prose 「running 折算席位 143.499ms」 while the evidence face published
//     「· running 原始 143.499ms → 计入 51.735ms(折算,按全域最大核最高频)」
//     — the raw magnitude wore the neighbouring account's word;
//   - prose 「占 146.899 席（running 143.499ms + runnable 3.400ms）」 — a
//     self-summed magnitude wearing the seat word instead of a unit, so the
//     ms/%-scoped scan never saw it (回查落空);
//   - §29.79 witness: 全额 paraphrased into the never-published 满额 with
//     the value intact.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// qh2bH9EvidenceBlock is the h9-shape system evidence block: the raw→counted
// row, the mixed-seat composition row, the caliber-parenthesis chain row and
// the paren-annotated seat row (all forms live in artifact
// 20260715-150233.691-47104 / the 155119 replay).
func qh2bH9EvidenceBlock() types.AnswerBlock {
	return types.AnswerBlock{
		ID: "runtime_trace_metric_snapshot", Kind: types.BlockSection,
		SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace, Text: strings.Join([]string{
			"├─链上─ ➊ ⚙ .ugc.aweme.lite-17267 · running ██████░░░░ 143.499ms 62% [E8(+1)]",
			"│ · running 原始 143.499ms → 计入 51.735ms(折算,按全域最大核最高频)",
			"│ · 有效归因 3.399ms = runnable(全额) 1.023ms + running(折算) 2.286ms",
			"│ · running 原始 2.579ms → 计入 2.286ms(折算,按全域最大核最高频,运行频点非最高,下界)",
			"│ · 有效归因 3.429ms(全额)",
			"│ · running 原始 7.305ms → 计入 4.958ms(折算,按全域最大核最高频)",
		}, "\n"),
	}
}

func qh2bAdvisory(t *testing.T, doc *types.AnswerDocumentV2, records ...types.ObservationRecord) []proseScalarBindingFinding {
	t.Helper()
	if len(records) == 0 {
		records = []types.ObservationRecord{psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_primary", "143.499")}
	}
	mut := psgTraceMutable(records...)
	bus := psgBus(mut)
	// 禁硬拦 pin (QH2-B discipline): the caliber arms live on the advisory
	// slice only — they must never raise the ViolProseScalarUngrounded
	// violation on their own (grounded values stay violation-free).
	_, advisory := proseScalarResidualAppendixInputs(doc, bus, mut)
	return advisory
}

func qh2bCaliberFinding(advisory []proseScalarBindingFinding, token string) (proseScalarBindingFinding, bool) {
	for _, f := range advisory {
		if strings.Contains(f.entryZH, token) && strings.Contains(f.entryZH, "口径词") {
			return f, true
		}
	}
	return proseScalarBindingFinding{}, false
}

// TestQH2B_ArmA_NeverPublishedWordDisclosed — the §29.79 headline form: the
// prose swaps 全额 for the never-published 满额 with the value intact. The
// word-list membership is directly decidable; the disclosure names the
// published word the value actually carries.
func TestQH2B_ArmA_NeverPublishedWordDisclosed(t *testing.T) {
	doc := psgBindingDoc("主因是 ➊ 席位的反转等待(满额) 1.023ms,供给缺口为主。", qh2bH9EvidenceBlock())
	mut := psgTraceMutable(psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_primary", "143.499"))
	bus := psgBus(mut)
	if vs := psgViolations(runProseScalarGroundingCheck(doc, bus, mut)); len(vs) != 0 {
		t.Fatalf("caliber findings must never raise a violation (禁硬拦), got %+v", vs)
	}
	advisory := qh2bAdvisory(t, doc)
	f, ok := qh2bCaliberFinding(advisory, "1.023ms")
	if !ok {
		t.Fatalf("arm A must disclose the never-published word, advisory=%+v", advisory)
	}
	for _, want := range []string{"「满额」未在本报告证据面出现", "该值在证据面以口径词「全额」发布"} {
		if !strings.Contains(f.entryZH, want) {
			t.Fatalf("arm A zh face missing %q: %q", want, f.entryZH)
		}
	}
	for _, want := range []string{`the caliber word "满额" next to 1.023ms`, `is published under the caliber word(s) "全额"`} {
		if !strings.Contains(f.entry, want) {
			t.Fatalf("arm A en face missing %q: %q", want, f.entry)
		}
	}
}

// TestQH2B_ArmB_RawValueWordedFolded — the h9 material form: the prose calls
// the raw 143.499 a 折算席位 while every published pairing of the value
// carries a different word. The finding states the evidence-side fact only
// (juxtaposition doctrine).
func TestQH2B_ArmB_RawValueWordedFolded(t *testing.T) {
	doc := psgBindingDoc(".ugc.aweme.lite-17267 的 running 折算席位 143.499ms 主导本窗。", qh2bH9EvidenceBlock())
	advisory := qh2bAdvisory(t, doc)
	f, ok := qh2bCaliberFinding(advisory, "143.499ms")
	if !ok {
		t.Fatalf("arm B must disclose the published caliber word, advisory=%+v", advisory)
	}
	if !strings.Contains(f.entryZH, "在证据面以口径词「原始」") {
		t.Fatalf("arm B must name the published 原始 pairing: %q", f.entryZH)
	}
	if strings.Contains(f.entryZH, "正文") || strings.Contains(f.entry, "prose claims") {
		t.Fatalf("arm B must state the evidence-side fact only (never characterize the prose): %q", f.entryZH)
	}
}

// TestQH2B_SeatWordedMagnitudeJoinsScan — the 146.899 escape: a decimal
// magnitude wearing the 席 word instead of a unit joins the scan. With the
// h9 pool (143.499 and 3.399 published), 146.899 = 143.499 + 3.399(±ulp)
// grounds ONLY as a self-sum → the existing F-2④ disclosure fires on the
// 席-worded token; integer seat COUNTS stay out of scope.
func TestQH2B_SeatWordedMagnitudeJoinsScan(t *testing.T) {
	doc := psgBindingDoc(".ugc.aweme.lite-17267 CPU running 占 146.899 席（running 143.499ms + runnable 3.400ms）。", qh2bH9EvidenceBlock())
	advisory := qh2bAdvisory(t, doc)
	var hit proseScalarBindingFinding
	found := false
	for _, f := range advisory {
		if strings.Contains(f.entryZH, "146.899席") {
			hit, found = f, true
			break
		}
	}
	if !found {
		t.Fatalf("the 席-worded self-sum must draw the F-2④ disclosure, advisory=%+v", advisory)
	}
	if !strings.Contains(hit.entryZH, "未在证据面单独发布") {
		t.Fatalf("the disclosure must state the value is not itself published: %q", hit.entryZH)
	}
	// Integer seat counts never parse as magnitudes.
	for _, tok := range extractProseScalarTokens("b", "共3席,另有 5 席合计 0.094ms。") {
		if tok.Unit == "席" {
			t.Fatalf("integer 席 counts must stay out of scope, got %+v", tok)
		}
	}
	// The decimal 席 form parses with the seat word as its unit.
	toks := extractProseScalarTokens("b", "合计 146.899 席")
	if len(toks) != 1 || toks[0].Unit != "席" || toks[0].Raw != "146.899" {
		t.Fatalf("decimal 席 magnitude must join the scan, got %+v", toks)
	}
}

// TestQH2B_SeatWordedUnpublishedValueUnmatched — a 席-worded magnitude that
// nothing publishes and no pair reproduces lands on the membership arm like
// any other scalar (the disclosure lane the STYLE-1 「理想值 34.8」 witness
// rode).
func TestQH2B_SeatWordedUnpublishedValueUnmatched(t *testing.T) {
	record := psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_primary", "143.499")
	mut := psgTraceMutable(record)
	bus := psgBus(mut)
	doc := psgProseDoc("该线程合计 146.899 席。")
	vs := psgViolations(runProseScalarGroundingCheck(doc, bus, mut))
	if len(vs) != 1 || !strings.Contains(vs[0].Detail, "146.899席") {
		t.Fatalf("an unpublished 席-worded magnitude must reach the membership arm, got %+v", vs)
	}
}

// TestQH2B_NegativeControls — the silence set:
//   - the verbatim quote of a published word+value pair;
//   - a published word next to a value with NO published pairing;
//   - a negated word occurrence (未计入);
//   - a banned word with no magnitude in its sentence.
func TestQH2B_NegativeControls(t *testing.T) {
	cases := []struct{ name, prose string }{
		{"verbatim quote", "该席构成为 runnable(全额) 1.023ms + running(折算) 2.286ms,计入 51.735ms(折算)。"},
		{"no published pairing", "有效归因合计折算 3.399ms。"},
		{"negated word", "另有 17 条链上行未计入 143.499ms 的账目。"},
		{"banned word, no magnitude", "该席位报名已满额,不再扩充。窗口另有 143.499ms 的 running 账目。"},
		// 155119 趟 live-shape controls (复放实锤, silenced by refinement):
		// a word binds its NEAREST token only — the 原始值 label belongs to
		// 4.710, never to the 3.429 downstream of it; and a word the engine
		// publishes inside the value's own caliber parenthesis chain (下界)
		// is an agreement, not a contradiction.
		{"nearest-token ownership", "runnable 2.181ms(主导) + running 1.419ms = 4.710ms 原始值,有效归因 3.429ms。"},
		{"paren-chain agreement", "该缺口对应 2.286ms 等效下界。"},
		// 155119 复放趟2 live shape: 「折算后」 is a transition connective —
		// it words the FOLLOWING (post-fold) value, never the raw value
		// upstream of it.
		{"transition connective", "自身 running 7.305ms,折算后有效 attribution 为 4.958ms(supply-fold deficit)。"},
	}
	for _, tc := range cases {
		doc := psgBindingDoc(tc.prose, qh2bH9EvidenceBlock())
		advisory := qh2bAdvisory(t, doc)
		for _, f := range advisory {
			if strings.Contains(f.entryZH, "口径词") {
				t.Fatalf("%s must stay silent, got %q", tc.name, f.entryZH)
			}
		}
	}
}

// TestQH2B_EvidencePairingNearestPerSide — the evidence-side pairing keeps
// the flanking words only: the h9 row pairs 143.499 with 原始 (and the
// over-collected 计入), NEVER with the next account's 折算; the composite
// 行3 value 3.399 pairs with nothing (its nearest word sits beyond the
// pairing leash).
func TestQH2B_EvidencePairingNearestPerSide(t *testing.T) {
	set := proseScalarEvidenceSet{}
	collectProseScalarCaliberBindings("· running 原始 143.499ms → 计入 51.735ms(折算,按全域最大核最高频)", &set)
	collectProseScalarCaliberBindings("· 有效归因 3.399ms = runnable(全额) 1.023ms + running(折算) 2.286ms", &set)
	collectProseScalarCaliberBindings("· running 原始 2.579ms → 计入 2.286ms(折算,按全域最大核最高频,运行频点非最高,下界)", &set)
	words := func(v float64) map[string]bool {
		out := map[string]bool{}
		for _, w := range proseScalarCaliberWordsForValue(set.caliberBindings, v, 0.001) {
			out[w] = true
		}
		return out
	}
	if w := words(143.499); !w["原始"] || w["折算"] {
		t.Fatalf("143.499 must pair with 原始 and never 折算, got %v", w)
	}
	if w := words(51.735); !w["计入"] || !w["折算"] {
		t.Fatalf("51.735 must pair with 计入 and 折算, got %v", w)
	}
	if w := words(3.399); len(w) != 0 {
		t.Fatalf("the composite 3.399 must pair with nothing, got %v", w)
	}
	if w := words(1.023); !w["全额"] {
		t.Fatalf("1.023 must pair with 全额, got %v", w)
	}
	// The caliber-parenthesis chain pairs its TAIL words too (the 155119
	// live 2.286 finding: 下界 sat at the chain tail beyond the flat leash).
	if w := words(2.286); !w["计入"] || !w["折算"] || !w["下界"] {
		t.Fatalf("2.286 must pair with the whole caliber chain 计入/折算/下界, got %v", w)
	}
}
