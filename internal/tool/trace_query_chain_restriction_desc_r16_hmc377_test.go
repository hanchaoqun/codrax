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

// hmc377R16OfWhich is the holding row's sentence for a seat whose whole
// restricted time it prices (the absorbed member), in the words of the 「其中」
// form (R-377-12 ④ / R-377-14 ④) turned to the holding row.
func hmc377R16OfWhich(ms string, zh bool) string {
	if zh {
		return "其中CPU亲和/cpuset限制下" + ms + "ms是本线程runnable时间里受限的那一部分,已含在本行账内,不另计"
	}
	return "of which " + ms + "ms under the CPU affinity/cpuset restriction is the restricted part of this thread's runnable time, already inside this row's account, never added again"
}

// hmc377R16OfAccount is the holding row's sentence for one part of a seat
// whose restricted ready account the engine split between rows: the whole
// account, then this row's part — the words of the restriction row's own
// account sentence (「…下合计X ms:其中…」) turned to the holding row.
func hmc377R16OfAccount(account, ms string, zh bool) string {
	if zh {
		return "CPU亲和/cpuset限制下合计" + account + "ms:其中" + ms + "ms已含在本行账内,不另计"
	}
	return account + "ms under the CPU affinity/cpuset restriction in all: of which " + ms + "ms is already inside this row's account, never added again"
}

func hmc377R16Desc(zh bool) string {
	if zh {
		return hmc377R16DescZH
	}
	return hmc377R16DescEN
}

// hmc377R16RestrictedWord is the word of the 「其中」 sentence: the amount is
// the thread's whole restricted time. No other line of these fixtures has it.
func hmc377R16RestrictedWord(zh bool) string {
	if zh {
		return "受限的那一部分"
	}
	return "is the restricted part of"
}

// hmc377R16HeldCount counts the holding row's lines of a face, in each of
// their forms: the 「其中」 sentence, the part of a split account inside an
// account, and the part inside a row's time (case A'). No other line of these
// fixtures carries one of these words.
func hmc377R16HeldCount(text string, zh bool) int {
	words := []string{"受限的那一部分", "已含在", "另列自账)"}
	if !zh {
		words = []string{"is the restricted part of", "already inside", "keeps its own separate account)"}
	}
	n := 0
	for _, w := range words {
		n += hmc377R16Count(text, w)
	}
	return n
}

var hmc377R16TagRE = regexp.MustCompile(`\[(E\d+[^\]]*)\]\s*$`)

// hmc377R16HeadTag returns the row's bracket (the key of its detail block).
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
		if !hmc377R16Has(block, hmc377R16OfAccount("18.308", "11.364", zh)) {
			t.Errorf("zh=%t: the chain row states the 11.364 ms it prices of the 18.308 ms restricted account:\n%s", zh, block)
		}
		if hmc377R16Has(block, hmc377R16OfAccount("18.308", "20.000", zh)) || hmc377R16Has(block, "20.000ms已含在") || hmc377R16Has(block, "20.000ms is already inside") {
			t.Errorf("zh=%t: the amount is the engine's share, never the row's value:\n%s", zh, block)
		}
		if !hmc377R16Has(block, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the chain row prints the seat's CPU-constraint description:\n%s", zh, block)
		}
		if got := hmc377R16Count(fence, hmc377R16Desc(zh)); got != 1 {
			t.Errorf("zh=%t: the description is printed %d time(s) on the tree, want once:\n%s", zh, got, fence)
		}
		if got := hmc377R16HeldCount(fence, zh); got != 1 {
			t.Errorf("zh=%t: one row states the share, got %d line(s):\n%s", zh, got, fence)
		}
		// The row's detail block says the same as its tree lines.
		detail := r13DetailBlock(faces.detail(zh), hmc377R16HeadTag(block))
		label, head := "- CPU亲和/cpuset限制: ", "- CPU约束描述: 允许核 7 · 排除全域观测核 0-6 · 判定依据 sched_switch_next_info"
		if !zh {
			label, head = "- CPU affinity/cpuset restriction: ", "- CPU-constraint description: allowed CPUs 7 · excludes observed CPUs 0-6 · basis sched_switch_next_info"
		}
		if !strings.Contains(detail, label+hmc377R16OfAccount("18.308", "11.364", zh)) || !strings.Contains(detail, head) {
			t.Errorf("zh=%t: the detail block of the chain row carries the sentence and the description:\n%s", zh, detail)
		}
		if got := hmc377R16HeldCount(faces.detail(zh), zh); got != 1 {
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
	absorbed.ShareClaims = []tracequery.RootCauseShareClaim{{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderClippedSeat,
		ShareMs: 0.038, Resolved: true, HolderKeys: []string{"seat-udk"}}}
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

// hmc377R16HopWholeResult: the family's seat is no row of its own — the
// thread's inversion seat and its chain hop are one measured account, drawn as
// the one row 「⇅ udk-irq-7-76 · runnable 1.903ms [E#(+1)]」 (the round-14
// engine-review universes' hop form, Z1own2_H). The absorbed restriction seat
// (1.050 ms) names the seat's record by its resolved claim.
func hmc377R16HopWholeResult() tracequery.Result {
	const account = "state_account:v2:r16-76-900"
	holder := tracequery.RootCauseRankItem{Rank: 2, Tier: "secondary", Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts", Thread: hmc377R16Thread,
		ImpactMs: 1.903, CumulativeImpactMs: 1.903, EffectiveImpactMs: 1.903, RunnableMs: 1.903, DominantState: "runnable", GatedRunnableMs: 1.903,
		PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 3, PriorityRelationArtifactSources: []string{"artifact:0"},
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: 10.2, EndTs: 10.2046, LineStart: 900, LineEnd: 990,
		ShareHolderKey: "seat-udk", MeasuredStateAccountKey: account}
	holder.RankFamilyKey = "rank_pair:priority_inversion_candidate>cpu_affinity_or_cpuset|pid:76|on_chain|lines:900-990"
	holder.AbsorbedRankRows = 1
	absorbed := tracequery.RootCauseRankItem{Tier: "absorbed", Type: "cpu_affinity_or_cpuset", Source: "window_stats.cpu_constraints", Thread: hmc377R16Thread,
		ImpactMs: 1.050, CumulativeImpactMs: 1.050, RunnableMs: 1.050, DominantState: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.72, LineStart: 902, LineEnd: 902,
		CPUConstraintKind: hmc377R16Kind, CPUConstraintAllowedCPUs: []int{7}, CPUConstraintExcludedCPUs: []int{0, 1, 2, 3, 4, 5, 6},
		AbsorbedByRankFamily: true, AbsorbedIntoFamily: holder.RankFamilyKey}
	absorbed.ShareClaims = []tracequery.RootCauseShareClaim{{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderChainSeat,
		ShareMs: 1.050, Resolved: true, HolderKeys: []string{"seat-udk"}}}
	result := hmc377R11Result(holder)
	result.RootCauseRank.AbsorbedItems = []tracequery.RootCauseRankItem{absorbed}
	chain := *result.WakeupChain
	chain.CausalImpacts = append(append([]tracequery.WakeupCausalImpact(nil), chain.CausalImpacts...), tracequery.WakeupCausalImpact{
		Thread: hmc377R16Thread, ChainDepth: 2, DominantState: "runnable", DominantImpactMs: 1.903, TotalMs: 1.903, RunnableMs: 1.903,
		OnChain: true, LineStart: 900, LineEnd: 990, Window: tracequery.TimeWindow{StartTs: 10.2, EndTs: 10.2046},
		PriorityInversionCandidate: true, PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
		PriorityRelationProvenLowerMs: 3, PriorityRelationArtifactSources: []string{"artifact:0"},
		PriorityInversionGatedMs: 1.903, GatedRunnableMs: 1.903, StateAccountKey: account})
	result.WakeupChain = &chain
	return result
}

// The whole shape whose family seat is drawn inside its chain hop row: the
// reader row that draws the seat's record states the absorbed member's time
// and the description — the same fact the rank preview row of the seat
// carries. The absorbed seat has no [E#] on this face, so the line points at
// none.
func TestHMC377R16AbsorbedMemberOfASeatDrawnInsideItsHopRow(t *testing.T) {
	faces := hmc377R16Run(t, hmc377R16HopWholeResult())
	// Universe precondition: one row of the thread, the hop and the seat as
	// one object; no row wears the family key (no 「链上并入」 note).
	var rows []runtimeTraceProjTreeRow
	model := faces.modelZH
	for _, row := range runtimeTraceProjSMR1AllRows(&model) {
		if strings.TrimSpace(row.Node.Subject) == hmc377R16Subject {
			rows = append(rows, *row)
		}
	}
	if len(rows) != 1 || rows[0].Node.Predicate != "wakeup_causal_impact" || len(rows[0].Node.MergedEvidenceIDs) != 1 ||
		strings.TrimSpace(rows[0].Node.RankFamilyKey) != "" || len(rows[0].AbsorbedChainPeers) != 0 {
		t.Fatalf("the hop row draws the seat's record as one object and wears no family key: %d row(s)", len(rows))
	}
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		block := hmc377R16RowBlock(fence, hmc377R16Subject+" · runnable")
		if !hmc377R16Has(block, hmc377R16OfWhich("1.050", zh)) || hmc377R16Has(block, hmc377R16OfWhich("1.903", zh)) {
			t.Errorf("zh=%t: the row states the absorbed member's 1.050 ms, never its own 1.903 ms:\n%s", zh, fence)
		}
		if hmc377R16Has(block, hmc377R16OfWhich("1.050", zh)+" [E") {
			t.Errorf("zh=%t: the absorbed seat has no [E#] on this face; the line points at none:\n%s", zh, block)
		}
		if !hmc377R16Has(block, hmc377R16Desc(zh)) || hmc377R16Count(fence, hmc377R16Desc(zh)) != 1 {
			t.Errorf("zh=%t: the description is printed once, under the row:\n%s", zh, fence)
		}
		if detail := r13DetailBlock(faces.detail(zh), hmc377R16HeadTag(block)); !strings.Contains(detail, hmc377R16OfWhich("1.050", zh)) {
			t.Errorf("zh=%t: the detail block carries the same sentence:\n%s", zh, detail)
		}
	}
	// The rank preview says the same on the seat's board row.
	var holding []string
	for _, l := range strings.Split(faces.preview, "\n") {
		if strings.Contains(l, "cpu_restricted_ready_inside_this_row=") {
			holding = append(holding, l)
		}
	}
	if len(holding) != 1 || !strings.Contains(holding[0], "type=priority_inversion_candidate subject="+hmc377R16Subject+" ") ||
		!strings.Contains(holding[0], " cpu_restricted_ready_inside_this_row=1.050ms cpu_constraint_kind=sched_switch_next_info cpu_constraint_allowed_cpus=7 ") {
		t.Errorf("the seat's preview row carries the same restricted time and judgment:\n%s", faces.preview)
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
		if got := hmc377R16HeldCount(fence, zh); got != 0 {
			t.Errorf("zh=%t: the chain row adds nothing beside the drawn restriction row:\n%s", zh, fence)
		}
		if block := hmc377R16RowBlock(fence, hmc377R16Subject+" · runnable"); hmc377R16Has(block, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the description stays on the restriction row:\n%s", zh, block)
		}
		if got := hmc377R16HeldCount(faces.detail(zh), zh); got != 0 {
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
		if hmc377R16HeldCount(first, zh) != 0 || hmc377R16Has(first, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the row the claim does not name states nothing:\n%s", zh, first)
		}
		if !hmc377R16Has(second, hmc377R16OfAccount("10.308", "3.364", zh)) || !hmc377R16Has(second, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the row the claim names states the share and the description:\n%s", zh, second)
		}
		if got := hmc377R16HeldCount(fence, zh); got != 1 {
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
		if !hmc377R16Has(first, hmc377R16OfAccount("18.308", "8.000", zh)) || !hmc377R16Has(first, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the first holding row states 8.000 ms of the 18.308 ms account and the description:\n%s", zh, first)
		}
		if !hmc377R16Has(second, hmc377R16OfAccount("18.308", "3.364", zh)) || hmc377R16Has(second, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the second holding row states 3.364 ms of the 18.308 ms account and no second description:\n%s", zh, second)
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
		want := "CPU亲和/cpuset限制下合计18.308ms:其中11.364ms已含在本行与[" + other + "]账内,不另计"
		if !zh {
			want = "18.308ms under the CPU affinity/cpuset restriction in all: of which 11.364ms is already inside the accounts of this row and [" + other + "], never added again"
		}
		if other == "" || !hmc377R16Has(first, want) {
			t.Errorf("zh=%t: the first row states the share inside both rows' accounts (other row [%s]):\n%s", zh, other, first)
		}
		if hmc377R16HeldCount(second, zh) != 0 || hmc377R16HeldCount(fence, zh) != 1 {
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
		want, account := "CPU亲和/cpuset限制下合计31.834ms:其中30.537ms落在本行时段内(本行另列自账)", "已含在"
		if !zh {
			want, account = "31.834ms under the CPU affinity/cpuset restriction in all: of which 30.537ms lies inside the time of this row (this row keeps its own separate account)", "already inside"
		}
		if !hmc377R16Has(block, want) || !hmc377R16Has(block, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the row says the share of the 31.834 ms account lies inside its time and prints the description:\n%s", zh, block)
		}
		if hmc377R16Has(block, hmc377R16RestrictedWord(zh)) || hmc377R16Has(block, account) {
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
		head, want := "其余 2 项微额锚定席", "成员 udk-irq-7-77 CPU亲和/cpuset限制下合计0.042ms:其中0.029ms已含在本行账内,不另计"
		if !zh {
			head, want = "2 more micro anchored seats", "member udk-irq-7-77's 0.042ms under the CPU affinity/cpuset restriction in all: of which 0.029ms is already inside this row's account, never added again"
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
			if fence := faces.fence(zh); hmc377R16HeldCount(fence, zh) != 0 || hmc377R16Count(fence, hmc377R16Desc(zh)) != 0 {
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
			if hmc377R16HeldCount(fence, zh) != 0 || hmc377R16Count(fence, hmc377R16Desc(zh)) != 0 ||
				hmc377R16HeldCount(faces.detail(zh), zh) != 0 {
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

// Review finding of round 16 (fix-up, 2026-11-24): a part of a split account is
// never given as the thread's restricted time. dio w01 udk-irq-7-76: the seat's
// typed restricted ready account is 18.308 ms; its chain row prices 11.364 ms
// of it and the seat keeps 6.944 ms. The 「其中」 sentence's words 「11.364ms是
// 本线程runnable时间里受限的那一部分」 stood under 「窗内 runnable 合计 18.308ms」
// and read as "6.944 ms of this thread's ready time was not restricted" (23 of
// the 23 part thread-windows of the 315; main printed 18.308 on the restriction
// row). The row states the whole account and its part, opening with the words
// the restriction row's own account sentence opens with — one typed number and
// one wording on both sides of the split. The whole shape keeps the 「其中」
// sentence: its amount is the seat's whole restricted time.
func TestHMC377R16PartOfASplitAccountIsStatedBesideTheWholeAccount(t *testing.T) {
	folded, drawn, whole := hmc377R16Run(t, hmc377R16PartResult(true)), hmc377R16Run(t, hmc377R16PartResult(false)), hmc377R16Run(t, hmc377R16WholeResult())
	for _, zh := range []bool{true, false} {
		head, counted := "CPU亲和/cpuset限制下合计18.308ms:其中", "11.364ms另行计入["
		if !zh {
			head, counted = "18.308ms under the CPU affinity/cpuset restriction in all: of which ", "11.364ms is counted separately on ["
		}
		block := hmc377R16RowBlock(folded.fence(zh), hmc377R16Subject+" · runnable")
		if !hmc377R16Has(block, head+"11.364ms") {
			t.Errorf("zh=%t: the holding row states the seat's 18.308 ms account before its 11.364 ms part:\n%s", zh, block)
		}
		if hmc377R16Has(block, hmc377R16RestrictedWord(zh)) || hmc377R16Has(block, hmc377R16OfWhich("11.364", zh)) {
			t.Errorf("zh=%t: the part is never given as the thread's restricted time:\n%s", zh, block)
		}
		for name, text := range map[string]string{"tree": folded.fence(zh), "detail": folded.detail(zh)} {
			if got := hmc377R16Count(text, head); got != 1 {
				t.Errorf("zh=%t %s: the account is stated %d time(s) on the holding row's face, want once", zh, name, got)
			}
		}
		// The same account, in the same words, on the restriction row where it
		// is drawn (and no holding row's line beside it).
		if !hmc377R16Has(drawn.fence(zh), head+counted) || hmc377R16Count(drawn.fence(zh), head) != 1 {
			t.Errorf("zh=%t: the drawn restriction row opens its account sentence with the same words:\n%s", zh, drawn.fence(zh))
		}
		// The whole shape: the amount is the seat's whole restricted time.
		wholeBlock := hmc377R16RowBlock(whole.fence(zh), hmc377R16Subject+" · runnable")
		total := "下合计"
		if !zh {
			total = " in all: of which "
		}
		if !hmc377R16Has(wholeBlock, hmc377R16OfWhich("0.038", zh)) || hmc377R16Has(wholeBlock, total) {
			t.Errorf("zh=%t: a seat whose account is not split keeps the of-which sentence:\n%s", zh, wholeBlock)
		}
	}
}

// The one wording rule over every form of the holding row's line: where the
// engine typed the seat's restricted ready account as split
// (RestrictedReadyAccountMS), the line states that account and the row's part —
// also when the part equals the account — and never the 「其中」 sentence's
// thread-scope words; a seat without the typed split keeps its forms.
func TestHMC377R16HeldSentenceStatesTheSplitAccountInEveryForm(t *testing.T) {
	seat := func(account float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{Subject: hmc377R16Subject, Object: "cpu_affinity_or_cpuset", TypeToken: "cpu_affinity_or_cpuset",
			RestrictedReadyAccountMS: account}
	}
	for _, c := range []struct {
		name   string
		held   runtimeTraceProjRestrictedHeld
		zh, en string
	}{
		{name: "split, the row's own account", held: runtimeTraceProjRestrictedHeld{MS: 11.364, Seat: seat(18.308)},
			zh: "CPU亲和/cpuset限制下合计18.308ms:其中11.364ms已含在本行账内,不另计",
			en: "18.308ms under the CPU affinity/cpuset restriction in all: of which 11.364ms is already inside this row's account, never added again"},
		{name: "split, the part equals the account", held: runtimeTraceProjRestrictedHeld{MS: 18.308, Seat: seat(18.308)},
			zh: "CPU亲和/cpuset限制下合计18.308ms:其中18.308ms已含在本行账内,不另计",
			en: "18.308ms under the CPU affinity/cpuset restriction in all: of which 18.308ms is already inside this row's account, never added again"},
		{name: "split, a seat drawn inside the row under its own [E#]", held: runtimeTraceProjRestrictedHeld{MS: 0.272, Seat: seat(19.025), Elsewhere: true, Refs: []string{"E32"}},
			zh: "CPU亲和/cpuset限制下合计19.025ms:其中0.272ms已含在[E32]账内,不另计",
			en: "19.025ms under the CPU affinity/cpuset restriction in all: of which 0.272ms is already inside the account of [E32], never added again"},
		{name: "split, this row and another row", held: runtimeTraceProjRestrictedHeld{MS: 11.364, Seat: seat(18.308), Refs: []string{"E9"}},
			zh: "CPU亲和/cpuset限制下合计18.308ms:其中11.364ms已含在本行与[E9]账内,不另计",
			en: "18.308ms under the CPU affinity/cpuset restriction in all: of which 11.364ms is already inside the accounts of this row and [E9], never added again"},
		{name: "split, a member of a row of several threads", held: runtimeTraceProjRestrictedHeld{MS: 0.029, Seat: seat(0.042), Member: "udk-irq-7-77"},
			zh: "成员 udk-irq-7-77 CPU亲和/cpuset限制下合计0.042ms:其中0.029ms已含在本行账内,不另计",
			en: "member udk-irq-7-77's 0.042ms under the CPU affinity/cpuset restriction in all: of which 0.029ms is already inside this row's account, never added again"},
		{name: "split, the row keeps its own account", held: runtimeTraceProjRestrictedHeld{MS: 30.537, Seat: seat(31.834), OwnAccount: true},
			zh: "CPU亲和/cpuset限制下合计31.834ms:其中30.537ms落在本行时段内(本行另列自账)",
			en: "31.834ms under the CPU affinity/cpuset restriction in all: of which 30.537ms lies inside the time of this row (this row keeps its own separate account)"},
		{name: "split, a chain seat inside the row keeps its own account", held: runtimeTraceProjRestrictedHeld{MS: 30.537, Seat: seat(31.834), OwnAccount: true, Elsewhere: true, Refs: []string{"E9"}},
			zh: "CPU亲和/cpuset限制下合计31.834ms:其中30.537ms落在链上席[E9]时段内(链席另列自账)",
			en: "31.834ms under the CPU affinity/cpuset restriction in all: of which 30.537ms lies inside the time of the chain seat [E9] (the chain seat keeps its own separate account)"},
		{name: "split, a member keeps its own account", held: runtimeTraceProjRestrictedHeld{MS: 0.029, Seat: seat(0.042), OwnAccount: true, Member: "udk-irq-7-77"},
			zh: "成员 udk-irq-7-77 CPU亲和/cpuset限制下合计0.042ms:其中0.029ms落在成员 udk-irq-7-77 时段内(该成员另列自账)",
			en: "member udk-irq-7-77's 0.042ms under the CPU affinity/cpuset restriction in all: of which 0.029ms lies inside the time of member udk-irq-7-77 (the member keeps its own separate account)"},
		{name: "not split, the absorbed member", held: runtimeTraceProjRestrictedHeld{MS: 0.038, Seat: seat(0), Tag: "E54"},
			zh: "其中CPU亲和/cpuset限制下0.038ms是本线程runnable时间里受限的那一部分,已含在本行账内,不另计 [E54]",
			en: "of which 0.038ms under the CPU affinity/cpuset restriction is the restricted part of this thread's runnable time, already inside this row's account, never added again [E54]"},
		{name: "not split, a seat drawn inside the row under its own [E#]", held: runtimeTraceProjRestrictedHeld{MS: 1.050, Seat: seat(0), Elsewhere: true, Refs: []string{"E7"}},
			zh: "其中CPU亲和/cpuset限制下1.050ms是本线程runnable时间里受限的那一部分,已含在[E7]账内,不另计",
			en: "of which 1.050ms under the CPU affinity/cpuset restriction is the restricted part of this thread's runnable time, already inside the account of [E7], never added again"},
		{name: "not split, the row keeps its own account", held: runtimeTraceProjRestrictedHeld{MS: 0.500, Seat: seat(0), OwnAccount: true},
			zh: "CPU亲和/cpuset限制下0.500ms落在本行时段内(本行另列自账)",
			en: "0.500ms under the CPU affinity/cpuset restriction lies inside the time of this row (this row keeps its own separate account)"},
	} {
		for _, zh := range []bool{true, false} {
			want := c.en
			if zh {
				want = c.zh
			}
			got := runtimeTraceProjRestrictedHeldText(c.held, zh)
			if got != want {
				t.Errorf("[%s] zh=%t:\n got %q\nwant %q", c.name, zh, got, want)
			}
			if c.held.Seat.RestrictedReadyAccountMS > 0 && strings.Contains(got, hmc377R16RestrictedWord(zh)) {
				t.Errorf("[%s] zh=%t: a part of a split account is never given as the thread's restricted time: %q", c.name, zh, got)
			}
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
//     ◇ context fold row; the row states 「…下合计18.308ms:其中11.364ms已含在
//     本行账内,不另计」;
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
		account    string
		descZH     string
		descEN     string
		pointer    bool
	}{
		{name: "part w01", tid: 56888, start: 2493.173005, end: 2494.208465, head: "udk-irq-7-76 · runnable", ms: "11.364", account: "18.308",
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
				if c.account != "" {
					// The seat's account is split between rows: the row states
					// the whole account beside its part, never the part as the
					// thread's restricted time.
					want = hmc377R16OfAccount(c.account, c.ms, zh)
					if hmc377R16Has(block, hmc377R16RestrictedWord(zh)) {
						t.Errorf("zh=%t: a part of a split account is never given as the thread's restricted time:\n%s", zh, block)
					}
				}
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
