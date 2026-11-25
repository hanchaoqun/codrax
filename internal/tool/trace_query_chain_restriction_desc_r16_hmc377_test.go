package tool

// HMC377 round 16 (batch-owner ruling R-377-16 ③/④, 2026-11-24): a drawn row
// that prices ready instants of a CPU-restriction seat through a relation the
// engine types prints, beneath itself, how much of its time is restricted and
// the seat's CPU-constraint description — when the seat is not drawn as a row
// of its own on that face.
//
// Real windows at c797b464c (merge 14 outputs,
// ~/hmc-work/hmc377/fix14/merge/sweep/out; the typed pair set:
// ~/hmc-work/hmc377/fix16/desc/res/pairs_m14.txt — 31 thread-windows of 315):
//   - dio w01 udk-irq-7-76 (part): main drew 「⧖ udk-irq-7-76 · CPU亲和/cpuset
//     限制 18.308ms」 with 「CPU约束描述:允许核 7 · 排除全域观测核 0-6 · 判定依据
//     sched_switch_next_info」; merge 14 drew 「├─链上─ ➋ ⧖ udk-irq-7-76 ·
//     runnable 11.364ms」, the restriction seat (6.944 of 18.308) sat in the
//     ◇ context fold row and the description was on no face.
//   - z40 zr23 Dns_100_10098-57468 (whole): the restriction seat (0.038) is an
//     absorbed member of 「⧖ Dns_100_10098-57468 · runnable 7.940ms」.
//   - new n13m #NetImmediateD#-65059: the share's holder is the thread's
//     inversion seat [E32], drawn inside its hop row 「⇅ … · running 1.400ms
//     [E10(+1)+E32]」 whose headline is the hop's own account.
//   - pop w12m udk-irq-7-77: the holder is a member of the micro anchored-seat
//     fold row.
//
// Every fixture is built through the production path: engine rank items with
// the typed fields carveCPUConstraintReadyInstants publishes -> observation
// records -> projection compile -> tree model -> fence / detail blocks, and
// the rank preview writer.

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

var hmc377R16Thread = tracequery.ThreadRef{Comm: "udk-irq-7", PID: 76}

const (
	hmc377R16Subject = "udk-irq-7-76"
	hmc377R16Kind    = "sched_switch_next_info"
	hmc377R16DescZH  = "CPU约束描述:允许核 7 · 排除全域观测核 0-6 · 判定依据 sched_switch_next_info"
	hmc377R16DescEN  = "CPU-constraint description: allowed CPUs 7 · excludes observed CPUs 0-6 · basis sched_switch_next_info"
)

// hmc377R16ChainSeat is an on-chain runnable seat of the restricted thread as
// the board publishes it (interval-proven anchored account), carrying the
// holder key the resolver's claims name.
func hmc377R16ChainSeat(thread tracequery.ThreadRef, rank int, ms float64, key string, line int) tracequery.RootCauseRankItem {
	item := tracequery.RootCauseRankItem{Rank: rank, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: thread,
		ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
		ChainAnchoredMs: ms, ChainAnchorFullMs: ms + 5, ChainCredentialCensus: "interval_proven",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.76,
		StartTs: 10.2, EndTs: 10.9, LineStart: line, LineEnd: line + 90}
	item.ShareHolderKey = key
	return item
}

// hmc377R16Restriction is the thread's CPU-restriction seat as the carve's
// part arm leaves it: the whole restricted ready account, the part rows of the
// published board count (the claims) and the part it counts itself, on the
// adjacent lane, with the engine's constraint judgment.
func hmc377R16Restriction(thread tracequery.ThreadRef, rank int, account, elsewhere float64, claims ...tracequery.RootCauseShareClaim) tracequery.RootCauseRankItem {
	here := account - elsewhere
	item := tracequery.RootCauseRankItem{Rank: rank, Tier: "tertiary", Type: "cpu_affinity_or_cpuset", Source: "window_stats.cpu_constraints", Thread: thread,
		ImpactMs: here, CumulativeImpactMs: here, EffectiveImpactMs: here, RunnableMs: here, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.72, LineStart: 902, LineEnd: 902,
		CPUConstraintKind: hmc377R16Kind, CPUConstraintAllowedCPUs: []int{7}, CPUConstraintExcludedCPUs: []int{0, 1, 2, 3, 4, 5, 6},
		CPUConstraintPolicy: "next_info affinity=80 load=72 group=0 ices_boost=false"}
	item.RestrictedReadyAccountMs, item.ShareCountedElsewhereMs, item.ShareCountedHereMs = account, elsewhere, here
	item.ShareClaims = claims
	return item
}

// hmc377R16ContextSeats are n adjacent seats of other threads, each larger
// than the restriction seat's own part: they take the context bucket's eight
// seats, so the restriction seat is a member of the bucket's fold row (the
// form of the 23 part pairs of the 315 windows).
func hmc377R16ContextSeats(n int) []tracequery.RootCauseRankItem {
	var out []tracequery.RootCauseRankItem
	for i := 0; i < n; i++ {
		ms := 60.0 - float64(i)
		out = append(out, tracequery.RootCauseRankItem{Rank: 30 + i, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats",
			Thread:   tracequery.ThreadRef{Comm: fmt.Sprintf("ctx%d", i), PID: 7000 + i},
			ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.6, LineStart: 2000 + 10*i, LineEnd: 2005 + 10*i})
	}
	return out
}

func hmc377R16Claim(part tracequery.RootCauseShareClaimPart, role tracequery.RootCauseShareHolderRole, ms float64, holders ...string) tracequery.RootCauseShareClaim {
	return tracequery.RootCauseShareClaim{Part: part, Role: role, ShareMs: ms, Resolved: true, Exact: len(holders) == 1, HolderKeys: holders}
}

type hmc377R16Faces struct {
	zh, en             string
	detailZH, detailEN string
	modelZH            runtimeTraceProjTreeModel
	preview            string
}

func (f hmc377R16Faces) fence(zh bool) string {
	if zh {
		return f.zh
	}
	return f.en
}

func (f hmc377R16Faces) detail(zh bool) string {
	if zh {
		return f.detailZH
	}
	return f.detailEN
}

func hmc377R16Run(t *testing.T, result tracequery.Result) hmc377R16Faces {
	t.Helper()
	records := traceQueryTypedObservations(result, "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("one projection expected, got %d", len(set.Projections))
	}
	var out hmc377R16Faces
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence, detail := runtimeTraceProjTreeFence(model, zh), runtimeTraceProjDetailFullText(model, zh)
		if zh {
			out.zh, out.detailZH, out.modelZH = fence, detail, model
		} else {
			out.en, out.detailEN = fence, detail
		}
	}
	var b strings.Builder
	writeTraceRootCauseRankPreview(&b, result.RootCauseRank, "payload")
	out.preview = b.String()
	return out
}

// hmc377R16RowBlock returns the tree lines of the first row whose head line
// contains head: the head and the note lines beneath it.
func hmc377R16RowBlock(fence, head string) string {
	var out []string
	for _, l := range strings.Split(fence, "\n") {
		if len(out) > 0 && (hmc377R13RowHead(l) || strings.TrimSpace(l) == "" || strings.HasPrefix(l, "◇ ") || strings.HasPrefix(l, "▒ ") || strings.HasPrefix(l, "```")) {
			break
		}
		if len(out) > 0 || (hmc377R13RowHead(l) && strings.Contains(l, head)) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func hmc377R16Has(text, want string) bool {
	return strings.Contains(hmc377R7Squash(text), hmc377R7Squash(want))
}

func hmc377R16Count(text, want string) int {
	return strings.Count(hmc377R7Squash(text), hmc377R7Squash(want))
}

// hmc377R16OfWhich is the holding row's sentence for an amount, in the words
// of the 「其中」 form (R-377-12 ④ / R-377-14 ④) turned to the holding row.
func hmc377R16OfWhich(ms string, zh bool) string {
	if zh {
		return "其中CPU亲和/cpuset限制下" + ms + "ms是本线程runnable时间里受限的那一部分,已含在本行账内,不另计"
	}
	return "of which " + ms + "ms under the CPU affinity/cpuset restriction is the restricted part of this thread's runnable time, already inside this row's account, never added again"
}

func hmc377R16Desc(zh bool) string {
	if zh {
		return hmc377R16DescZH
	}
	return hmc377R16DescEN
}

// hmc377R16RestrictedWord is the word every form of the holding row's line
// carries and no other line of these fixtures does.
func hmc377R16RestrictedWord(zh bool) string {
	if zh {
		return "受限的那一部分"
	}
	return "is the restricted part of"
}

var hmc377R16TagRE = regexp.MustCompile(`\[(E\d+)[^\]]*\]\s*$`)

// hmc377R16HeadTag returns the first [E#] of the row's bracket.
func hmc377R16HeadTag(block string) string {
	head := strings.SplitN(block, "\n", 2)[0]
	if m := hmc377R16TagRE.FindStringSubmatch(head); m != nil {
		return m[1]
	}
	return ""
}

// hmc377R16PartResult is the part shape: the thread's chain seat (20.000 ms)
// holds 11.364 ms of the seat's 18.308 ms restricted ready account; the seat
// counts 6.944 ms itself. folded: the context bucket is over its cap and the
// seat is a member of its fold row.
func hmc377R16PartResult(folded bool) tracequery.Result {
	items := []tracequery.RootCauseRankItem{
		hmc377R16ChainSeat(hmc377R16Thread, 2, 20.000, "seat-udk", 900),
		hmc377R16Restriction(hmc377R16Thread, 10, 18.308, 11.364,
			hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 11.364, "seat-udk")),
	}
	if folded {
		items = append(items, hmc377R16ContextSeats(9)...)
	}
	return hmc377R11Result(items...)
}

// The part shape whose restriction seat is a member of the context fold row
// (dio w01 udk-irq-7-76): the chain row states the restricted time it prices —
// the engine's share, not the row's 20.000 ms — and the seat's CPU-constraint
// description, once on the face, in both languages, in the tree and in the
// row's detail block.
func TestHMC377R16ChainRowStatesTheRestrictedPartOfAFoldedSeat(t *testing.T) {
	faces := hmc377R16Run(t, hmc377R16PartResult(true))
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		// Universe precondition: the seat is no row of its own.
		for _, l := range strings.Split(fence, "\n") {
			if strings.Contains(l, hmc377R16Subject) && (strings.Contains(l, "cpuset限制") || strings.Contains(l, "cpuset restriction")) && !strings.Contains(l, "· ") {
				t.Fatalf("zh=%t: the restriction seat must be folded in this universe: %q\n%s", zh, l, fence)
			}
		}
		block := hmc377R16RowBlock(fence, hmc377R16Subject+" · runnable")
		if block == "" {
			t.Fatalf("zh=%t: the chain row is drawn:\n%s", zh, fence)
		}
		if !hmc377R16Has(block, hmc377R16OfWhich("11.364", zh)) {
			t.Errorf("zh=%t: the chain row states the 11.364 ms it prices of the restricted time:\n%s", zh, block)
		}
		if hmc377R16Has(block, hmc377R16OfWhich("20.000", zh)) {
			t.Errorf("zh=%t: the amount is the engine's share, never the row's value:\n%s", zh, block)
		}
		if !hmc377R16Has(block, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the chain row prints the seat's CPU-constraint description:\n%s", zh, block)
		}
		if got := hmc377R16Count(fence, hmc377R16Desc(zh)); got != 1 {
			t.Errorf("zh=%t: the description is printed %d time(s) on the tree, want once:\n%s", zh, got, fence)
		}
		if got := hmc377R16Count(fence, hmc377R16RestrictedWord(zh)); got != 1 {
			t.Errorf("zh=%t: one row states the share, got %d line(s):\n%s", zh, got, fence)
		}
		// The row's detail block says the same as its tree lines.
		detail := r13DetailBlock(faces.detail(zh), hmc377R16HeadTag(block))
		label, head := "- CPU亲和/cpuset限制: ", "- CPU约束描述: 允许核 7 · 排除全域观测核 0-6 · 判定依据 sched_switch_next_info"
		if !zh {
			label, head = "- CPU affinity/cpuset restriction: ", "- CPU-constraint description: allowed CPUs 7 · excludes observed CPUs 0-6 · basis sched_switch_next_info"
		}
		if !strings.Contains(detail, label+hmc377R16OfWhich("11.364", zh)) || !strings.Contains(detail, head) {
			t.Errorf("zh=%t: the detail block of the chain row carries the sentence and the description:\n%s", zh, detail)
		}
		if got := strings.Count(faces.detail(zh), hmc377R16RestrictedWord(zh)); got != 1 {
			t.Errorf("zh=%t: one detail block states the share, got %d", zh, got)
		}
	}
}

// hmc377R16WholeResult is the whole shape (z40 zr23 Dns_100_10098-57468): every
// restricted instant lies inside one chain seat, so the engine absorbed the
// restriction seat into that seat's family (0.038 ms of the row's 7.940 ms).
func hmc377R16WholeResult() tracequery.Result {
	holder := hmc377R16ChainSeat(hmc377R16Thread, 2, 7.940, "seat-udk", 900)
	holder.RankFamilyKey = "rank_pair:runnable_wait>cpu_affinity_or_cpuset|pid:76|on_chain|lines:900-990"
	holder.AbsorbedRankRows = 1
	absorbed := tracequery.RootCauseRankItem{Tier: "absorbed", Type: "cpu_affinity_or_cpuset", Source: "window_stats.cpu_constraints", Thread: hmc377R16Thread,
		ImpactMs: 0.038, CumulativeImpactMs: 0.038, RunnableMs: 0.038, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.72, LineStart: 902, LineEnd: 902,
		CPUConstraintKind: hmc377R16Kind, CPUConstraintAllowedCPUs: []int{7}, CPUConstraintExcludedCPUs: []int{0, 1, 2, 3, 4, 5, 6},
		CPUConstraintPolicy:  "next_info affinity=80 load=72 group=0 ices_boost=false",
		AbsorbedByRankFamily: true, AbsorbedIntoFamily: holder.RankFamilyKey}
	result := hmc377R11Result(holder)
	result.RootCauseRank.AbsorbedItems = []tracequery.RootCauseRankItem{absorbed}
	return result
}

// The whole shape: the row states its absorbed restriction member's time
// (0.038 of 7.940), points at the member's own [E#] — the number the row's
// 「链上并入」 note cites — and prints the description once.
func TestHMC377R16ChainRowStatesItsAbsorbedRestrictionMember(t *testing.T) {
	faces := hmc377R16Run(t, hmc377R16WholeResult())
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		block := hmc377R16RowBlock(fence, hmc377R16Subject+" · runnable")
		if !hmc377R16Has(block, hmc377R16OfWhich("0.038", zh)) || hmc377R16Has(block, hmc377R16OfWhich("7.940", zh)) {
			t.Fatalf("zh=%t: the row states the absorbed member's 0.038 ms, never its own 7.940 ms:\n%s", zh, fence)
		}
		detail := r13DetailBlock(faces.detail(zh), hmc377R16HeadTag(block))
		member := regexp.MustCompile(`\((E\d+)\)`).FindStringSubmatch(detail)
		if member == nil {
			t.Fatalf("zh=%t: the row's detail block cites its absorbed member:\n%s", zh, detail)
		}
		if !hmc377R16Has(block, hmc377R16OfWhich("0.038", zh)+" ["+member[1]+"]") {
			t.Errorf("zh=%t: the sentence points at the absorbed member [%s]:\n%s", zh, member[1], block)
		}
		if !hmc377R16Has(block, hmc377R16Desc(zh)) || hmc377R16Count(fence, hmc377R16Desc(zh)) != 1 {
			t.Errorf("zh=%t: the description is printed once, under the row:\n%s", zh, fence)
		}
		if !strings.Contains(detail, hmc377R16OfWhich("0.038", zh)+" ["+member[1]+"]") {
			t.Errorf("zh=%t: the detail block carries the same sentence:\n%s", zh, detail)
		}
	}
}

// Where the restriction row is drawn it keeps its description and its own
// 「…另行计入[E#]」 note; the chain row adds nothing (one description per
// thread and face).
func TestHMC377R16DrawnRestrictionRowKeepsItsDescriptionAlone(t *testing.T) {
	faces := hmc377R16Run(t, hmc377R16PartResult(false))
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		counted := "11.364ms另行计入["
		if !zh {
			counted = "11.364ms is counted separately on ["
		}
		// Universe precondition: the restriction row is drawn with its account.
		if !hmc377R16Has(fence, counted) {
			t.Fatalf("zh=%t: the restriction row is drawn with its account sentence:\n%s", zh, fence)
		}
		if got := hmc377R16Count(fence, hmc377R16Desc(zh)); got != 1 {
			t.Errorf("zh=%t: the description is printed %d time(s), want once (on the restriction row):\n%s", zh, got, fence)
		}
		if got := hmc377R16Count(fence, hmc377R16RestrictedWord(zh)); got != 0 {
			t.Errorf("zh=%t: the chain row adds nothing beside the drawn restriction row:\n%s", zh, fence)
		}
		if block := hmc377R16RowBlock(fence, hmc377R16Subject+" · runnable"); hmc377R16Has(block, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the description stays on the restriction row:\n%s", zh, block)
		}
		if got := strings.Count(faces.detail(zh), hmc377R16RestrictedWord(zh)); got != 0 {
			t.Errorf("zh=%t: no detail block states a share, got %d", zh, got)
		}
	}
}

// hmc377R16TwoSeats: two chain rows of the restricted thread — its walked
// chain seat (20.000 ms, drawn first) and its wakeup-anchored seat (3.364 ms).
func hmc377R16TwoSeats() (tracequery.RootCauseRankItem, tracequery.RootCauseRankItem) {
	walked := hmc377R16ChainSeat(hmc377R16Thread, 2, 20.000, "seat-walked", 900)
	unwalked := hmc377R16ChainSeat(hmc377R16Thread, 3, 3.364, "seat-unwalked", 1200)
	unwalked.ChainAnchorUnwalkedSeat = true
	return walked, unwalked
}

func hmc377R16TwoSeatResult(claims ...tracequery.RootCauseShareClaim) tracequery.Result {
	walked, unwalked := hmc377R16TwoSeats()
	elsewhere := 0.0
	for _, c := range claims {
		if c.Part != tracequery.RootCauseShareClaimChainAccount {
			elsewhere += c.ShareMs
		}
	}
	items := []tracequery.RootCauseRankItem{walked, unwalked, hmc377R16Restriction(hmc377R16Thread, 10, elsewhere+6.944, elsewhere, claims...)}
	return hmc377R11Result(append(items, hmc377R16ContextSeats(9)...)...)
}

// The holder is the row the resolver's claim names — never a row found by the
// thread's name: of the thread's two chain rows only the one the claim names
// (here the second one drawn) states the share.
func TestHMC377R16OnlyTheRowTheClaimNamesStatesTheShare(t *testing.T) {
	faces := hmc377R16Run(t, hmc377R16TwoSeatResult(
		hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.364, "seat-unwalked")))
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		first, second := hmc377R16RowBlock(fence, "20.000ms"), hmc377R16RowBlock(fence, "3.364ms")
		if !strings.Contains(first, hmc377R16Subject) || !strings.Contains(second, hmc377R16Subject) {
			t.Fatalf("zh=%t: the thread has two chain rows:\n%s", zh, fence)
		}
		if hmc377R16Has(first, hmc377R16RestrictedWord(zh)) || hmc377R16Has(first, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the row the claim does not name states nothing:\n%s", zh, first)
		}
		if !hmc377R16Has(second, hmc377R16OfWhich("3.364", zh)) || !hmc377R16Has(second, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the row the claim names states the share and the description:\n%s", zh, second)
		}
		if got := hmc377R16Count(fence, hmc377R16RestrictedWord(zh)); got != 1 {
			t.Errorf("zh=%t: one row states the share, got %d:\n%s", zh, got, fence)
		}
	}
}

// Two parts of the seat held by two rows: each row states its own share; the
// description is printed once, on the first of them in drawing order.
func TestHMC377R16EachHoldingRowStatesItsOwnShareAndOneDescription(t *testing.T) {
	faces := hmc377R16Run(t, hmc377R16TwoSeatResult(
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 8.000, "seat-walked"),
		hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.364, "seat-unwalked")))
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		first, second := hmc377R16RowBlock(fence, "20.000ms"), hmc377R16RowBlock(fence, "3.364ms")
		if !hmc377R16Has(first, hmc377R16OfWhich("8.000", zh)) || !hmc377R16Has(first, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the first holding row states 8.000 ms and the description:\n%s", zh, first)
		}
		if !hmc377R16Has(second, hmc377R16OfWhich("3.364", zh)) || hmc377R16Has(second, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the second holding row states 3.364 ms and no second description:\n%s", zh, second)
		}
		if got := hmc377R16Count(fence, hmc377R16Desc(zh)); got != 1 {
			t.Errorf("zh=%t: the description is printed %d time(s), want once:\n%s", zh, got, fence)
		}
	}
}

// One share the engine resolved to two holding rows (it types no split between
// them) is stated once, on the first row, naming the other row by its [E#].
func TestHMC377R16OneShareHeldByTwoRowsIsStatedOnce(t *testing.T) {
	faces := hmc377R16Run(t, hmc377R16TwoSeatResult(
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 11.364, "seat-walked", "seat-unwalked")))
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		first, second := hmc377R16RowBlock(fence, "20.000ms"), hmc377R16RowBlock(fence, "3.364ms")
		other := hmc377R16HeadTag(second)
		want := "其中CPU亲和/cpuset限制下11.364ms是本线程runnable时间里受限的那一部分,已含在本行与[" + other + "]账内,不另计"
		if !zh {
			want = "of which 11.364ms under the CPU affinity/cpuset restriction is the restricted part of this thread's runnable time, already inside the accounts of this row and [" + other + "], never added again"
		}
		if other == "" || !hmc377R16Has(first, want) {
			t.Errorf("zh=%t: the first row states the share inside both rows' accounts (other row [%s]):\n%s", zh, other, first)
		}
		if hmc377R16Has(second, hmc377R16RestrictedWord(zh)) || hmc377R16Count(fence, hmc377R16RestrictedWord(zh)) != 1 {
			t.Errorf("zh=%t: the share is stated once:\n%s", zh, fence)
		}
	}
}

// Case A' (the chain seat keeps its own, diverging account — the engine's
// chain-account claim): the share lies inside the row's time and is no part of
// its value, so the row takes the restriction row's own A' words turned to
// itself; 「其中…已含在本行账内」 would be false (pop w02w2 form: 30.537 ms
// against a 20.256 ms row).
func TestHMC377R16ChainSeatWithItsOwnAccountSaysInsideItsTime(t *testing.T) {
	holder := hmc377R16ChainSeat(hmc377R16Thread, 2, 20.256, "seat-udk", 900)
	items := []tracequery.RootCauseRankItem{holder, hmc377R16Restriction(hmc377R16Thread, 10, 31.834, 30.537,
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 30.537, "seat-udk"),
		hmc377R16Claim(tracequery.RootCauseShareClaimChainAccount, tracequery.RootCauseShareHolderChainSeat, 20.256, "seat-udk"))}
	faces := hmc377R16Run(t, hmc377R11Result(append(items, hmc377R16ContextSeats(9)...)...))
	for _, zh := range []bool{true, false} {
		block := hmc377R16RowBlock(faces.fence(zh), hmc377R16Subject+" · runnable")
		want := "CPU亲和/cpuset限制下30.537ms落在本行时段内(本行另列自账)"
		if !zh {
			want = "30.537ms under the CPU affinity/cpuset restriction lies inside the time of this row (this row keeps its own separate account)"
		}
		if !hmc377R16Has(block, want) || !hmc377R16Has(block, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the row says the share lies inside its time and prints the description:\n%s", zh, block)
		}
		if hmc377R16Has(block, hmc377R16RestrictedWord(zh)) {
			t.Errorf("zh=%t: a row with its own account never says the share is inside its account:\n%s", zh, block)
		}
	}
}

// The holder is a member of the micro anchored-seat fold row (pop w12m
// udk-irq-7-77): the fold row publishes its members' account sum and names the
// member whose runnable time is restricted.
func TestHMC377R16MicroFoldRowNamesTheHoldingMember(t *testing.T) {
	member := tracequery.ThreadRef{Comm: "udk-irq-7", PID: 77}
	items := []tracequery.RootCauseRankItem{
		hmc377R16ChainSeat(member, 7, 0.029, "seat-micro", 900),
		hmc377R16ChainSeat(tracequery.ThreadRef{Comm: "binder:431_4", PID: 8650}, 8, 0.082, "seat-other", 1300),
		hmc377R16Restriction(member, 10, 0.042, 0.029,
			hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 0.029, "seat-micro")),
	}
	faces := hmc377R16Run(t, hmc377R11Result(append(items, hmc377R16ContextSeats(9)...)...))
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		head, want := "其余 2 项微额锚定席", "其中CPU亲和/cpuset限制下0.029ms是成员 udk-irq-7-77 runnable时间里受限的那一部分,已含在本行账内,不另计"
		if !zh {
			head, want = "2 more micro anchored seats", "of which 0.029ms under the CPU affinity/cpuset restriction is the restricted part of member udk-irq-7-77's runnable time, already inside this row's account, never added again"
		}
		block := hmc377R16RowBlock(fence, head)
		if block == "" {
			t.Fatalf("zh=%t: the two micro seats are one fold row:\n%s", zh, fence)
		}
		if !hmc377R16Has(block, want) || !hmc377R16Has(block, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the fold row names the member and prints the description:\n%s", zh, block)
		}
		if got := hmc377R16Count(fence, hmc377R16Desc(zh)); got != 1 {
			t.Errorf("zh=%t: the description is printed %d time(s), want once:\n%s", zh, got, fence)
		}
	}
}

// A share whose holder this face cannot place is stated on no row: a claim the
// engine did not resolve, and a claim whose holder is not a row of the board.
func TestHMC377R16UnplacedHolderStatesNothing(t *testing.T) {
	unresolved := hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 11.364, "seat-udk")
	unresolved.Resolved, unresolved.Exact = false, false
	// A claim the engine resolved only in part (some of it returned to the
	// claimant) still names its holder on the record; it is no resolved claim.
	returned := unresolved
	returned.ResolvedMs, returned.ReturnedMs = 9.364, 2.000
	offBoard := hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 11.364, "seat-not-published")
	for name, claim := range map[string]tracequery.RootCauseShareClaim{"unresolved claim": unresolved, "claim returned in part": returned, "holder off the board": offBoard} {
		items := []tracequery.RootCauseRankItem{hmc377R16ChainSeat(hmc377R16Thread, 2, 20.000, "seat-udk", 900),
			hmc377R16Restriction(hmc377R16Thread, 10, 18.308, 11.364, claim)}
		faces := hmc377R16Run(t, hmc377R11Result(append(items, hmc377R16ContextSeats(9)...)...))
		for _, zh := range []bool{true, false} {
			if fence := faces.fence(zh); hmc377R16Count(fence, hmc377R16RestrictedWord(zh)) != 0 || hmc377R16Count(fence, hmc377R16Desc(zh)) != 0 {
				t.Errorf("%s zh=%t: no row states a share the face cannot place:\n%s", name, zh, fence)
			}
		}
	}
}

// The relation is the engine's typed one, on a CPU-restriction seat only: a
// seat of another type whose claim names the chain row states no restriction
// under it, and a restriction seat without the typed account split (no
// restricted ready account, no share counted elsewhere — z40 zc12
// Animax_GPU-12212's form: the seat keeps its whole 0.028 ms) states none
// either, whatever claim rides beside it.
func TestHMC377R16OnlyARestrictionSeatWithItsTypedAccountSpeaks(t *testing.T) {
	claim := hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 11.364, "seat-udk")
	otherType := hmc377R16Restriction(hmc377R16Thread, 10, 18.308, 11.364, claim)
	otherType.Type, otherType.Source = "runnable_wait", "window_stats"
	otherType.CPUConstraintKind, otherType.CPUConstraintAllowedCPUs, otherType.CPUConstraintExcludedCPUs, otherType.CPUConstraintPolicy = "", nil, nil, ""
	noAccount := hmc377R16Restriction(hmc377R16Thread, 10, 18.308, 11.364, claim)
	noAccount.RestrictedReadyAccountMs, noAccount.ShareCountedElsewhereMs, noAccount.ShareCountedHereMs = 0, 0, 0
	for name, seat := range map[string]tracequery.RootCauseRankItem{"a seat of another type": otherType, "a restriction seat without the typed account split": noAccount} {
		items := []tracequery.RootCauseRankItem{hmc377R16ChainSeat(hmc377R16Thread, 2, 20.000, "seat-udk", 900), seat}
		faces := hmc377R16Run(t, hmc377R11Result(append(items, hmc377R16ContextSeats(9)...)...))
		for _, zh := range []bool{true, false} {
			fence := faces.fence(zh)
			if hmc377R16RowBlock(fence, hmc377R16Subject+" · runnable") == "" {
				t.Fatalf("%s zh=%t: the chain row is drawn:\n%s", name, zh, fence)
			}
			if hmc377R16Count(fence, hmc377R16RestrictedWord(zh)) != 0 || hmc377R16Count(fence, hmc377R16Desc(zh)) != 0 ||
				strings.Count(faces.detail(zh), hmc377R16RestrictedWord(zh)) != 0 {
				t.Errorf("%s zh=%t: no row states a restricted part:\n%s", name, zh, fence)
			}
		}
	}
}

// hmc377R16Seat is the restriction seat's roster node with one resolved claim
// naming holder.
func hmc377R16SeatNode(holder string) types.TraceCausalProjectionNode {
	seat := hmc377R14Constrain(types.TraceCausalProjectionNode{EvidenceID: "pub#root_cause_rank:25", Subject: "dep-200",
		Predicate: "root_cause_adjacent", Object: "cpu_affinity_or_cpuset", TypeToken: "cpu_affinity_or_cpuset", StateKind: "runnable",
		ChainRelevance: "adjacent", Rank: 9, ImpactMS: 18.753, CumulativeImpactMS: 18.753, EffectiveImpactMS: 18.753},
		hmc377R16Kind, []int{4, 5, 6, 7}, []int{0, 1, 2, 3})
	hmc377R12SetRestricted(&seat, 19.025, 0.272, 18.753)
	seat.ShareClaims = []types.TraceCausalProjectionShareClaim{{Part: string(tracequery.RootCauseShareClaimAnchored),
		Role: string(tracequery.RootCauseShareHolderChainSeat), ShareMS: 0.272, Resolved: true, Exact: true, HolderEvidenceIDs: []string{holder}}}
	return seat
}

// The property over the drawn-id classes: the row that states the share is the
// row the holder record is drawn in; the account named is the row's own when
// the row publishes the holder's account, and the holder's [E#] when the row
// draws the holder as a seat under its own [E#] while publishing its own
// account (new n13m: the inversion seat [E32] inside the hop row 「running
// 1.400ms [E10(+1)+E32]」 — 「本行账」 would name the hop's running time).
// A holder drawn in no row, in a context fold row or in an overflow fold row
// states nothing; a drawn restriction row silences every holder.
func TestHMC377R16HoldingRowFollowsEveryDrawnIDClass(t *testing.T) {
	const holder = "pub#root_cause_rank:7"
	type drawnCase struct {
		name  string
		place func(m *runtimeTraceProjTreeModel)
		// wantTag: the head tag of the row that states the share ("" = none).
		wantTag string
		// account: the account words expected (zh).
		account string
	}
	cases := []drawnCase{
		{name: "the row's own record", wantTag: "E32", account: "已含在本行账内", place: func(m *runtimeTraceProjTreeModel) {
			m.TreeRows = append(m.TreeRows, hmc377R6Row(holder, "dep-200", "E32", runtimeTraceProjTreeRowChain))
		}},
		{name: "a cause child row", wantTag: "E33", account: "已含在本行账内", place: func(m *runtimeTraceProjTreeModel) {
			m.TreeRows = append(m.TreeRows, hmc377R6Row(holder, "dep-200", "E33", runtimeTraceProjTreeRowCause))
		}},
		{name: "a rank seat inside its hop row, host publishes its own account", wantTag: "E36(+1)", account: "已含在[E37]账内", place: func(m *runtimeTraceProjTreeModel) {
			row := hmc377R6Row("chain-host", "dep-200", "E36(+1)", runtimeTraceProjTreeRowChain)
			row.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{Rank: 7, EvidenceTag: "E37", DisplayImpactMS: 0.511, drawn: hmc377R6PeerDrawn(holder, 0.511)}}
			m.TreeRows = append(m.TreeRows, row)
		}},
		{name: "a rank seat inside its hop row, host adopted the seat's account", wantTag: "E36(+1)", account: "已含在本行账内", place: func(m *runtimeTraceProjTreeModel) {
			row := hmc377R6Row("chain-host", "dep-200", "E36(+1)", runtimeTraceProjTreeRowChain)
			drawn := hmc377R6PeerDrawn(holder, 0.511)
			drawn.own = true
			row.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{Rank: 7, EvidenceTag: "E37", DisplayImpactMS: 0.511, drawn: drawn}}
			m.TreeRows = append(m.TreeRows, row)
		}},
		{name: "an engine-absorbed row inside its family row", wantTag: "E49", account: "已含在本行账内", place: func(m *runtimeTraceProjTreeModel) {
			row := hmc377R6Row("family-host", "dep-200", "E49", runtimeTraceProjTreeRowChain)
			row.Node.RankFamilyKey = "fam-k"
			m.TreeRows = append(m.TreeRows, row)
			absorbed := hmc377R6Node(holder, "dep-200", "runnable")
			absorbed.AbsorbedByRankFamily, absorbed.AbsorbedInto = true, "fam-k"
			m.absorbedChainRows = append(m.absorbedChainRows, absorbed)
		}},
		{name: "drawn nowhere", place: func(m *runtimeTraceProjTreeModel) {}},
		{name: "a member of a context fold row", place: func(m *runtimeTraceProjTreeModel) {
			row := hmc377R6Row("fold-first", "", "E60(+1)", runtimeTraceProjTreeRowAdjacent)
			row.Node.MergedEvidenceIDs, row.Node.MergedCount, row.Node.TypeToken = []string{holder}, 2, ""
			m.Adjacent = append(m.Adjacent, row)
		}},
		{name: "a member of the on-chain overflow fold row", place: func(m *runtimeTraceProjTreeModel) {
			row := hmc377R6Row("overflow-first", "", "E61(+1)", runtimeTraceProjTreeRowChain)
			row.Node.MergedEvidenceIDs, row.Node.MergedCount, row.Node.OnChainOverflowFold = []string{holder}, 2, true
			m.TreeRows = append(m.TreeRows, row)
		}},
		{name: "the restriction row is drawn", place: func(m *runtimeTraceProjTreeModel) {
			m.TreeRows = append(m.TreeRows, hmc377R6Row(holder, "dep-200", "E32", runtimeTraceProjTreeRowChain))
			m.Adjacent = append(m.Adjacent, runtimeTraceProjTreeRow{Node: hmc377R16SeatNode(holder), Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E70"})
		}},
	}
	for _, c := range cases {
		model := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
		c.place(&model)
		runtimeTraceProjStampRestrictedHeld(&model, []types.TraceCausalProjectionNode{hmc377R16SeatNode(holder)}, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		var got []string
		for _, row := range runtimeTraceProjSMR1AllRows(&model) {
			for _, held := range row.RestrictedHeld {
				text := runtimeTraceProjRestrictedHeldText(held, true)
				got = append(got, row.EvidenceTag+"|"+text)
				if !held.Describe || held.MS != 0.272 || !strings.Contains(text, "0.272ms") {
					t.Errorf("[%s] the one holding row states the engine's 0.272 ms and the description: %+v", c.name, held)
				}
			}
		}
		switch {
		case c.wantTag == "" && len(got) != 0:
			t.Errorf("[%s] no row states the share, got %v", c.name, got)
		case c.wantTag != "" && (len(got) != 1 || !strings.HasPrefix(got[0], c.wantTag+"|") || !strings.Contains(got[0], c.account)):
			t.Errorf("[%s] row [%s] states the share with %q, got %v", c.name, c.wantTag, c.account, got)
		}
	}
}

// R-377-16 ④, the rank preview: a board row whose absorbed member is a
// CPU-restriction seat carries the restricted time inside it and the engine's
// constraint judgment under the keys the seat's own record carries (the seat
// is no row of the board). A seat that keeps a board row of its own speaks
// there: the preview of the part shape is unchanged, row for row.
func TestHMC377R16RankPreviewCarriesTheAbsorbedRestriction(t *testing.T) {
	whole := hmc377R16WholeResult()
	preview := hmc377R16Run(t, whole).preview
	const fields = " cpu_restricted_ready_inside_this_row=0.038ms cpu_constraint_kind=sched_switch_next_info cpu_constraint_allowed_cpus=7 cpu_constraint_excluded_cpus=0,1,2,3,4,5,6"
	var holding []string
	for _, l := range strings.Split(preview, "\n") {
		if strings.Contains(l, "cpu_restricted_ready_inside_this_row") || strings.Contains(l, "cpu_constraint_") {
			holding = append(holding, l)
		}
	}
	if len(holding) != 1 || !strings.Contains(holding[0], "board_order=2 ") || !strings.Contains(holding[0], "subject="+hmc377R16Subject+" ") ||
		!strings.HasSuffix(holding[0], "source=window_stats"+fields) {
		t.Fatalf("the holding row, and only it, carries the restricted time and the constraint judgment:\n%s", preview)
	}
	// Every other byte of the preview is the preview of the same board without
	// the absorbed member.
	whole.RootCauseRank.AbsorbedItems = nil
	if bare := hmc377R16Run(t, whole).preview; strings.Replace(preview, fields, "", 1) != bare {
		t.Errorf("only the holding row's suffix is added:\n%s\n--- without the absorbed member:\n%s", preview, bare)
	}
	// The seat's own record keeps its typed judgment, key order included.
	for _, r := range traceQueryTypedObservations(hmc377R16PartResult(true), "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC()) {
		if r.Subject != hmc377R16Subject || r.Object != "cpu_affinity_or_cpuset" {
			continue
		}
		var keys []string
		for _, n := range r.RichNotes {
			if strings.HasPrefix(n, "cpu_constraint_") {
				keys = append(keys, n)
			}
		}
		want := []string{"cpu_constraint_kind=sched_switch_next_info", "cpu_constraint_policy=next_info affinity=80 load=72 group=0 ices_boost=false",
			"cpu_constraint_allowed_cpus=7", "cpu_constraint_excluded_cpus=0,1,2,3,4,5,6"}
		if strings.Join(keys, "|") != strings.Join(want, "|") {
			t.Errorf("the seat's record carries its judgment unchanged: %v", keys)
		}
	}
	if part := hmc377R16Run(t, hmc377R16PartResult(true)).preview; strings.Contains(part, "cpu_restricted_ready_inside_this_row") || strings.Contains(part, "cpu_constraint_") {
		t.Errorf("a seat with a board row of its own speaks there; the holding row's preview is unchanged:\n%s", part)
	}
}

// 100 抖音极速版 容器重启 #1 (CODRAX_HMC376_GALLERY_FULLGC), on the original
// capture through the public trace_query entry, one window of each arm:
//   - part, review window w01 [2493.173005, 2494.208465) TID 56888:
//     udk-irq-7-76's chain row 「runnable 11.364ms」 holds 11.364 of the seat's
//     18.308 ms restricted ready account; the seat (6.944) is a member of the
//     ◇ context fold row;
//   - whole, review window zr23 [2498.683045, 2498.883045) TID 56984:
//     Dns_100_10098-57468's restriction seat (0.038) is an absorbed member of
//     its chain row 「runnable 7.940ms」.
func TestHMC377R16WholeOriginalChainRowStatesItsRestrictedTime(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC376_GALLERY_FULLGC", "d27ff641787dfa0768928e0ce3d11a7d2a6caa0f2910c941fae3e97f2256e22e", 245420208)
	for _, c := range []struct {
		name       string
		tid        int
		start, end float64
		head, ms   string
		descZH     string
		descEN     string
		pointer    bool
	}{
		{name: "part w01", tid: 56888, start: 2493.173005, end: 2494.208465, head: "udk-irq-7-76 · runnable", ms: "11.364",
			descZH: hmc377R16DescZH, descEN: hmc377R16DescEN},
		{name: "whole zr23", tid: 56984, start: 2498.683045, end: 2498.883045, head: "Dns_100_10098-57468 · runnable", ms: "0.038",
			descZH: "CPU约束描述:判定依据 sched_switch_next_info", descEN: "CPU-constraint description: basis sched_switch_next_info", pointer: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			faces := hmc377R11QueryPID(t, path, c.tid, c.start, c.end)
			for _, zh := range []bool{true, false} {
				fence, desc := faces.en, c.descEN
				if zh {
					fence, desc = faces.zh, c.descZH
				}
				block := hmc377R16RowBlock(fence, c.head)
				if block == "" {
					t.Fatalf("zh=%t: the chain row is drawn", zh)
				}
				want := hmc377R16OfWhich(c.ms, zh)
				if c.pointer {
					want += " [E"
				}
				if !hmc377R16Has(block, want) {
					t.Errorf("zh=%t: the chain row states its %s ms of restricted time:\n%s", zh, c.ms, block)
				}
				if !hmc377R16Has(block, desc) {
					t.Errorf("zh=%t: the chain row prints the seat's CPU-constraint description:\n%s", zh, block)
				}
			}
		})
	}
}
