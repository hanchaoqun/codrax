package tool

// HMC377 round 16 review (batch-owner ruling R-377-17, 2026-11-24): the part
// arm on the rank summary (②), one decision for every face (③), pins for the
// reviewers' survivors and one description per thread (⑤), the transport cap
// (⑥), and the conditions on the uncharged summary fields (①).
//
// Fixtures reuse the round-16 shapes (trace_query_chain_restriction_desc_r16_hmc377_test.go),
// built through the production path: engine rank items -> observation records
// -> projection compile -> tree model -> fence / detail blocks, and the rank
// summary writer.

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const hmc377R17Judgment = " cpu_constraint_kind=sched_switch_next_info cpu_constraint_allowed_cpus=7 cpu_constraint_excluded_cpus=0,1,2,3,4,5,6"

// hmc377R17PreviewFields maps each preview row that ends with the
// restricted-ready fields to them (keyed by its board_order).
func hmc377R17PreviewFields(preview string) map[string]string {
	out := map[string]string{}
	for _, l := range strings.Split(preview, "\n") {
		if !strings.HasPrefix(l, traceQueryRankPreviewRowPrefix) {
			continue
		}
		at := strings.Index(l, traceQueryRankRestrictedReadyKey)
		if at < 0 {
			if strings.Contains(l, "restricted_ready_account_ms") || strings.Contains(l, "cpu_constraint_") {
				out["stray:"+l] = l
			}
			continue
		}
		order := strings.Fields(strings.TrimPrefix(l, traceQueryRankPreviewRowPrefix))[0]
		out[order] = l[at:]
	}
	return out
}

// R-377-17 ②: the part arm on the summary. Two parts of one seat held by two
// rows: each row states its own part and the seat's split account; the
// thread's constraint judgment follows once, on the first holding row in
// board order. The seat's own preview row carries nothing.
func TestHMC377R17SummaryStatesEachPartOnceWithTheJudgmentOncePerThread(t *testing.T) {
	result := hmc377R16TwoSeatResult(
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 8.000, "seat-walked"),
		hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.364, "seat-unwalked"))
	got := hmc377R17PreviewFields(hmc377R16Run(t, result).preview)
	want := map[string]string{
		"board_order=2": " cpu_restricted_ready_inside_this_row=8.000ms restricted_ready_account_ms=18.308" + hmc377R17Judgment,
		"board_order=3": " cpu_restricted_ready_inside_this_row=3.364ms restricted_ready_account_ms=18.308",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("each holding row states its part and the split account; the judgment once, on the first:\n got %v\nwant %v", got, want)
	}
	// The reader says the same: each row its part of the 18.308 ms account,
	// one description, on the first holding row; the second holding row's
	// detail block carries the sentence and no description.
	faces := hmc377R16Run(t, result)
	for _, zh := range []bool{true, false} {
		second := hmc377R16RowBlock(faces.fence(zh), "3.364ms")
		detail := r13DetailBlock(faces.detail(zh), hmc377R16HeadTag(second))
		if !strings.Contains(detail, hmc377R16OfAccount("18.308", "3.364", zh)) || strings.Contains(detail, hmc377R16Desc(zh)) ||
			strings.Contains(detail, strings.TrimSpace(runtimeTraceProjCPUConstraintDescriptionHead(zh))) {
			t.Errorf("zh=%t: the second holding row's detail block states its part and no description:\n%s", zh, detail)
		}
		if got := hmc377R16Count(faces.detail(zh), hmc377R16Desc(zh)[len(runtimeTraceProjCPUConstraintDescriptionHead(zh)):]); got != 1 {
			t.Errorf("zh=%t: one detail block describes the constraint, got %d", zh, got)
		}
	}
}

// R-377-17 ②: a claim that names two rows (the engine types no split between
// them) and a row that would hold the time of two seats state nothing on the
// summary — the key speaks of one row and one seat; the seats' records carry
// the relation. The reader states the shared claim once, naming the other row.
func TestHMC377R17SummaryLeavesASharedClaimOrRowToTheRecords(t *testing.T) {
	shared := hmc377R16TwoSeatResult(
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 11.364, "seat-walked", "seat-unwalked"))
	if got := hmc377R17PreviewFields(hmc377R16Run(t, shared).preview); len(got) != 0 {
		t.Errorf("a claim held by two rows states nothing on the summary: %v", got)
	}
	twoSeats := hmc377R17TwoSeatsOneThreadResult()
	if got := hmc377R17PreviewFields(hmc377R16Run(t, twoSeats).preview); len(got) != 0 {
		t.Errorf("a row holding the time of two seats states nothing on the summary: %v", got)
	}
}

// Second review (2026-11-25): a hold in case A' (the seat's chain-account
// claim beside the walked chain-seat part; the holder keeps its own account)
// states nothing on the summary — the key says the time is inside the row's
// account, which is false there (pop w02w2 row 2: 30.537 ms against 20.256 ms
// of ready time); the seat's record carries the relation. A part of the same
// seat that is inside a row's account is still stated, on the same row or on
// another, and the thread's judgment moves to its first stating row. The
// reader keeps its A' words.
func TestHMC377R17SummaryLeavesTheOwnAccountFormToTheRecord(t *testing.T) {
	ownWords := map[bool]string{true: "落在本行时段内(本行另列自账)", false: "lies inside the time of this row (this row keeps its own separate account)"}
	chainAccount := func(ms float64, holder string) tracequery.RootCauseShareClaim {
		return hmc377R16Claim(tracequery.RootCauseShareClaimChainAccount, tracequery.RootCauseShareHolderChainSeat, ms, holder)
	}
	for _, c := range []struct {
		name   string
		result tracequery.Result
		want   map[string]string
		ownRow string
	}{
		{"case A'", hmc377R17OwnAccountResult(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat),
			map[string]string{}, "20.256ms"},
		{"unwalked part beside the chain account (control)", hmc377R17OwnAccountResult(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderChainSeat),
			map[string]string{"board_order=2": " cpu_restricted_ready_inside_this_row=30.537ms restricted_ready_account_ms=31.834" + hmc377R17Judgment}, ""},
		{"clipped part beside the chain account (control)", hmc377R17OwnAccountResult(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat),
			map[string]string{"board_order=2": " cpu_restricted_ready_inside_this_row=30.537ms restricted_ready_account_ms=31.834" + hmc377R17Judgment}, ""},
		{"case A' and a part inside the same row", hmc377R17TwoPartsOneRowResult(
			hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 8.000, "seat-udk"),
			hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.364, "seat-udk"),
			chainAccount(20.000, "seat-udk")),
			map[string]string{"board_order=2": " cpu_restricted_ready_inside_this_row=3.364ms restricted_ready_account_ms=18.308" + hmc377R17Judgment}, "20.000ms"},
		{"case A' on one row, a part inside another (w02w2 form)", hmc377R16TwoSeatResult(
			hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 8.000, "seat-walked"),
			hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.364, "seat-unwalked"),
			chainAccount(20.000, "seat-walked")),
			map[string]string{"board_order=3": " cpu_restricted_ready_inside_this_row=3.364ms restricted_ready_account_ms=18.308" + hmc377R17Judgment}, "20.000ms"},
		{"case A' of an absorbed member on its family's row", hmc377R17AbsorbedOwnAccount(), map[string]string{}, "7.940ms"},
	} {
		faces := hmc377R16Run(t, c.result)
		if got := hmc377R17PreviewFields(faces.preview); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("[%s] summary fields:\n got %v\nwant %v", c.name, got, c.want)
		}
		if c.ownRow == "" {
			continue
		}
		for _, zh := range []bool{true, false} {
			if block := hmc377R16RowBlock(faces.fence(zh), c.ownRow); !strings.Contains(block, hmc377R16Subject+" · runnable") || !hmc377R16Has(block, ownWords[zh]) {
				t.Errorf("[%s] zh=%t: the reader keeps the A' words on the holding row:\n%s", c.name, zh, block)
			}
		}
	}
}

// hmc377R17AbsorbedOwnAccount: the whole shape whose absorbed restriction
// member hands its walked part to its family's chain seat beside a
// chain-account claim (case A' on the absorbed arm).
func hmc377R17AbsorbedOwnAccount() tracequery.Result {
	result := hmc377R16WholeResult()
	rank := *result.RootCauseRank
	rank.AbsorbedItems = append([]tracequery.RootCauseRankItem(nil), rank.AbsorbedItems...)
	rank.AbsorbedItems[0].ShareClaims = []tracequery.RootCauseShareClaim{
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 0.038, "seat-udk"),
		hmc377R16Claim(tracequery.RootCauseShareClaimChainAccount, tracequery.RootCauseShareHolderChainSeat, 7.940, "seat-udk")}
	result.RootCauseRank = &rank
	return result
}

// hmc377R17TwoSeatsOneThreadResult is the reviewer's two-seats-one-thread
// board: one restriction seat of the thread drawn as a row of its own
// (88.636 ms kept), a second one folded; both hand a part to the thread's
// chain row.
func hmc377R17TwoSeatsOneThreadResult() tracequery.Result {
	drawn := hmc377R16Restriction(hmc377R16Thread, 9, 100.0, 11.364,
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 11.364, "seat-udk"))
	folded := hmc377R16Restriction(hmc377R16Thread, 10, 5.0, 3.0,
		hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.0, "seat-udk"))
	folded.LineStart, folded.LineEnd = 1902, 1902
	items := []tracequery.RootCauseRankItem{hmc377R16ChainSeat(hmc377R16Thread, 2, 20.000, "seat-udk", 900), drawn, folded}
	return hmc377R11Result(append(items, hmc377R16ContextSeats(9)...)...)
}

// R-377-17 ⑤: one CPU-constraint description per (thread, face), the thread
// read by its typed identity. Two restriction seats of one thread, one drawn:
// the drawn row describes; the chain row states the folded seat's part and no
// second description.
func TestHMC377R17OneDescriptionPerThreadAndFace(t *testing.T) {
	faces := hmc377R16Run(t, hmc377R17TwoSeatsOneThreadResult())
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		if got := hmc377R16Count(fence, hmc377R16Desc(zh)); got != 1 {
			t.Errorf("zh=%t: the thread's description is printed %d time(s), want once:\n%s", zh, got, fence)
		}
		block := hmc377R16RowBlock(fence, hmc377R16Subject+" · runnable")
		if !hmc377R16Has(block, hmc377R16OfAccount("5.000", "3.000", zh)) || hmc377R16Has(block, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the chain row states the folded seat's part and leaves the description to the drawn row:\n%s", zh, block)
		}
		if got := hmc377R16Count(faces.detail(zh), strings.TrimSpace(runtimeTraceProjCPUConstraintDescriptionHead(zh))); got != 0 {
			t.Errorf("zh=%t: no detail block describes a thread whose restriction row is drawn, got %d", zh, got)
		}
	}
	// A seat without a typed TID is its own identity.
	a := types.TraceCausalProjectionNode{EvidenceID: "a"}
	b := types.TraceCausalProjectionNode{EvidenceID: "b"}
	c := types.TraceCausalProjectionNode{EvidenceID: "c", RankSubjects: &types.TraceRankSubjects{Member: types.TraceThreadCoordinate{TID: 76}}}
	d := types.TraceCausalProjectionNode{EvidenceID: "d", RankSubjects: &types.TraceRankSubjects{Member: types.TraceThreadCoordinate{TID: 76}}}
	e := types.TraceCausalProjectionNode{EvidenceID: "e", RankSubjects: &types.TraceRankSubjects{Member: types.TraceThreadCoordinate{TID: 77}}}
	f := d
	f.RankBoardTarget = "other-board"
	if TraceRestrictedReadyThread(a) == TraceRestrictedReadyThread(b) || TraceRestrictedReadyThread(c) != TraceRestrictedReadyThread(d) ||
		TraceRestrictedReadyThread(c) == TraceRestrictedReadyThread(e) || TraceRestrictedReadyThread(d) == TraceRestrictedReadyThread(f) {
		t.Errorf("thread identity: tid and board, else the seat itself")
	}
}

// R-377-17 ②⑤ (reviewer survivors Q57–Q62 on the preview): only a
// CPU-restriction seat that the engine absorbed into a family (the flag and a
// non-empty key) speaks as restricted time, on the first row that wears the
// family key, with the claim's share; an absorbed member of another type, a
// restriction seat that only names a family key, and one absorbed without a
// key state nothing.
func TestHMC377R17PreviewSpeaksForAbsorbedRestrictionSeatsOnly(t *testing.T) {
	result := hmc377R16WholeResult()
	key := ""
	for _, item := range result.RootCauseRank.Items {
		if item.ShareHolderKey == "seat-udk" {
			key = item.RankFamilyKey
		}
	}
	if key == "" {
		t.Fatalf("fixture precondition: the holding row wears its family key")
	}
	claim := func(ms float64, holder string) []tracequery.RootCauseShareClaim {
		return []tracequery.RootCauseShareClaim{{Part: tracequery.RootCauseShareClaimAnchored, Role: tracequery.RootCauseShareHolderClippedSeat,
			ShareMs: ms, Resolved: true, HolderKeys: []string{holder}}}
	}
	absorbedOf := func(typ string, thread tracequery.ThreadRef, family string, ms float64, line int) tracequery.RootCauseRankItem {
		item := tracequery.RootCauseRankItem{Tier: "absorbed", Type: typ, Source: "window_stats", Thread: thread,
			ImpactMs: ms, CumulativeImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.6, LineStart: line, LineEnd: line,
			AbsorbedByRankFamily: true, AbsorbedIntoFamily: family}
		item.ShareClaims = claim(ms, "seat-udk")
		return item
	}
	// Beside the restriction member, an absorbed member of another type in the
	// same family (Q58), with a claim naming the same row.
	other := absorbedOf("priority_inversion_runnable_wait", hmc377R16Thread, key, 1.234, 905)
	// A second family row of another thread with only a member of another type.
	second := hmc377R16ChainSeat(tracequery.ThreadRef{Comm: "binder:431_4", PID: 8650}, 3, 5.5, "seat-second", 1300)
	second.RankFamilyKey = "rank_pair:runnable_wait>unknown_state|pid:8650|on_chain|lines:1300-1390"
	second.AbsorbedRankRows = 1
	other2 := absorbedOf("unknown_state", second.Thread, second.RankFamilyKey, 2.345, 1305)
	other2.ShareClaims = claim(2.345, "seat-second")
	// A later row wearing the first family key (Q62): the hold sits on the
	// first row that wears it.
	twin := hmc377R16ChainSeat(hmc377R16Thread, 4, 2.000, "seat-twin", 1400)
	twin.RankFamilyKey = key
	// A restriction seat of a third thread that names a family key without the
	// engine's absorbed flag (Q57) and one absorbed without a key (Q59): no
	// typed split account either, so neither is a hold; their claims name rows
	// of the board that wear no family key.
	third := tracequery.ThreadRef{Comm: "worker", PID: 9100}
	plain := hmc377R16ChainSeat(third, 5, 4.000, "seat-third", 1500)
	flagless := absorbedOf("cpu_affinity_or_cpuset", third, "rank_pair:runnable_wait>cpu_affinity_or_cpuset|pid:9100|on_chain|lines:1500-1590", 0.500, 1502)
	flagless.AbsorbedByRankFamily = false
	flagless.ShareClaims = claim(0.500, "seat-third")
	keyless := absorbedOf("cpu_affinity_or_cpuset", third, "", 0.700, 1503)
	keyless.ShareClaims = claim(0.700, "seat-third")
	result.RootCauseRank.Items = append(result.RootCauseRank.Items, second, twin, plain)
	result.RootCauseRank.AbsorbedItems = append(result.RootCauseRank.AbsorbedItems, other, other2, flagless, keyless)
	got := hmc377R17PreviewFields(hmc377R16Run(t, result).preview)
	want := map[string]string{"board_order=2": " cpu_restricted_ready_inside_this_row=0.038ms" + hmc377R17Judgment}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("only the first row wearing the restriction member's family key states it, with the claim's share:\n got %v\nwant %v", got, want)
	}
}

// R-377-17 ③: one decision. The summary reads the rank rows through the
// records' own encoders; the reader and the finalizer read the projection
// compiled from the records. On every fixture of round 16 and 17 both
// carriers give the same holds (seat, family, holders, amount, account,
// case A'), holders named by their board positions.
func TestHMC377R17OneDecisionOnEveryFace(t *testing.T) {
	fixtures := map[string]tracequery.Result{
		"part folded":         hmc377R16PartResult(true),
		"part drawn":          hmc377R16PartResult(false),
		"whole":               hmc377R16WholeResult(),
		"whole, hop":          hmc377R16HopWholeResult(),
		"two parts two rows":  hmc377R16TwoSeatResult(hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 8.000, "seat-walked"), hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.364, "seat-unwalked")),
		"one claim two rows":  hmc377R16TwoSeatResult(hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 11.364, "seat-walked", "seat-unwalked")),
		"two seats one row":   hmc377R17TwoSeatsOneThreadResult(),
		"case A'":             hmc377R17OwnAccountResult(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat),
		"transport cap kept":  hmc377R17CapResult(27),
		"transport cap cut":   hmc377R17CapResult(28),
		"two parts one row":   hmc377R17TwoPartsOneRowResult(),
		"micro fold member":   hmc377R17MicroResult(),
		"whole, host stanza":  hmc377R17WholeHostAs("adjacent", "adjacent_to_wakeup_chain"),
		"whole, host context": hmc377R17WholeHostAs("background", "background_pressure"),
		// An absorbed member with an engine account its record does not carry
		// (the record states the account only beside a share counted
		// elsewhere): both carriers read no account.
		"whole, account off the record": hmc377R17AbsorbedWithAccount(),
		"whole, case A'":                hmc377R17AbsorbedOwnAccount(),
	}
	names := make([]string, 0, len(fixtures))
	for name := range fixtures {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result := fixtures[name]
		rank := hmc377R17HoldKeys(traceQueryRankRestrictedReadyHolds(result.RootCauseRank))
		records := traceQueryTypedObservations(result, "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC())
		set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
		if len(set.Projections) != 1 {
			t.Fatalf("[%s] one projection expected, got %d", name, len(set.Projections))
		}
		projection := hmc377R17HoldKeys(TraceRestrictedReadyHolds(set.Projections[0]))
		if strings.Join(rank, "\n") != strings.Join(projection, "\n") {
			t.Errorf("[%s] the summary's and the projection's decisions differ:\n summary   %v\n projection %v", name, rank, projection)
		}
		if name != "transport cap cut" && len(rank) == 0 {
			t.Errorf("[%s] fixture precondition: the board holds restricted time", name)
		}
	}
}

// hmc377R17HoldKeys renders holds carrier-free: record ids by their board
// position (the test's own reading of an id).
func hmc377R17HoldKeys(holds []TraceRestrictedReadyHold) []string {
	pos := func(id string) string {
		if at := strings.LastIndex(id, "#root_cause_rank:"); at >= 0 {
			return id[at+len("#root_cause_rank:"):]
		}
		return id
	}
	var out []string
	for _, h := range holds {
		holders := make([]string, 0, len(h.HolderIDs))
		for _, id := range h.HolderIDs {
			holders = append(holders, pos(id))
		}
		out = append(out, fmt.Sprintf("seat=%s family=%t:%s holders=%v ms=%.3f account=%.3f own=%t",
			pos(h.Seat.EvidenceID), h.FamilyKey != "", h.FamilyKey, holders, h.MS, h.AccountMS, h.OwnAccount))
	}
	sort.Strings(out)
	return out
}

// hmc377R17CapResult is the whole shape with extra on-chain board rows before
// the absorbed restriction member (Items then AbsorbedItems: the member sits at
// record position extra+2).
func hmc377R17CapResult(extra int) tracequery.Result {
	result := hmc377R16WholeResult()
	for i := 0; i < extra; i++ {
		ms := 59 - float64(i)*0.5
		result.RootCauseRank.Items = append(result.RootCauseRank.Items, tracequery.RootCauseRankItem{Rank: 40 + i, Tier: "tertiary", Type: "runnable_wait",
			Source: "window_stats", Thread: tracequery.ThreadRef{Comm: fmt.Sprintf("chn%d", i), PID: 8000 + i},
			ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
			ChainAnchoredMs: ms, ChainAnchorFullMs: ms + 1, ChainCredentialCensus: "interval_proven",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.7, StartTs: 10.3, EndTs: 10.4,
			LineStart: 3000 + 100*i, LineEnd: 3050 + 100*i})
	}
	return result
}

// R-377-17 ⑥: the transport cap. An absorbed restriction member whose record
// the per-board position cap keeps is stated on the reader face and on the
// summary; one whose record the cap drops is stated on neither — every face
// reads the records the transport publishes (traceQueryRankRecordEmitted).
func TestHMC377R17TransportCapIsReadByEveryFace(t *testing.T) {
	cap := traceQueryWidthTypedFamilyRowCap()
	for _, c := range []struct {
		extra int
		want  bool
	}{{cap - 5, true}, {cap - 4, false}} {
		result := hmc377R17CapResult(c.extra)
		if pos := len(result.RootCauseRank.Items) + 1; (pos <= cap) != c.want {
			t.Fatalf("fixture precondition: the member's record position %d against the cap %d", pos, cap)
		}
		faces := hmc377R16Run(t, result)
		summary := strings.Contains(faces.preview, "cpu_restricted_ready_inside_this_row=0.038ms")
		for _, zh := range []bool{true, false} {
			block := hmc377R16RowBlock(faces.fence(zh), hmc377R16Subject+" · runnable")
			if block == "" {
				t.Fatalf("extra=%d zh=%t: the holding row is drawn", c.extra, zh)
			}
			reader := hmc377R16Has(block, hmc377R16OfWhich("0.038", zh)) && hmc377R16Has(block, hmc377R16Desc(zh))
			if reader != c.want || summary != c.want {
				t.Errorf("extra=%d zh=%t: reader states it %t, summary %t, want both %t", c.extra, zh, reader, summary, c.want)
			}
		}
	}
}

// hmc377R17OwnAccountResult: a chain seat beside a chain-account claim (case
// A' when the walked part is an anchored chain-seat part).
func hmc377R17OwnAccountResult(part tracequery.RootCauseShareClaimPart, role tracequery.RootCauseShareHolderRole) tracequery.Result {
	holder := hmc377R16ChainSeat(hmc377R16Thread, 2, 20.256, "seat-udk", 900)
	items := []tracequery.RootCauseRankItem{holder, hmc377R16Restriction(hmc377R16Thread, 10, 31.834, 30.537,
		hmc377R16Claim(part, role, 30.537, "seat-udk"),
		hmc377R16Claim(tracequery.RootCauseShareClaimChainAccount, tracequery.RootCauseShareHolderChainSeat, 20.256, "seat-udk"))}
	return hmc377R11Result(append(items, hmc377R16ContextSeats(9)...)...)
}

// Case A' is the walked anchored part held by the chain seat beside the
// chain-account claim (reviewer survivors R09/R10): an unwalked part, or an
// anchored part of another holder role, keeps the account words.
func TestHMC377R17OwnAccountFormNeedsTheWalkedChainSeatPart(t *testing.T) {
	for _, c := range []struct {
		name string
		part tracequery.RootCauseShareClaimPart
		role tracequery.RootCauseShareHolderRole
		own  bool
	}{
		{"anchored chain-seat part", tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, true},
		{"unwalked part of the chain seat", tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderChainSeat, false},
		{"anchored part of a clipped seat", tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, false},
	} {
		faces := hmc377R16Run(t, hmc377R17OwnAccountResult(c.part, c.role))
		for _, zh := range []bool{true, false} {
			block := hmc377R16RowBlock(faces.fence(zh), hmc377R16Subject+" · runnable")
			own, account := "落在本行时段内(本行另列自账)", hmc377R16OfAccount("31.834", "30.537", zh)
			if !zh {
				own = "lies inside the time of this row (this row keeps its own separate account)"
			}
			if hmc377R16Has(block, own) != c.own || hmc377R16Has(block, account) == c.own {
				t.Errorf("[%s] zh=%t: case A' form %t expected:\n%s", c.name, zh, c.own, block)
			}
		}
	}
}

// hmc377R17TwoPartsOneRowResult: two parts of one seat named on one row.
func hmc377R17TwoPartsOneRowResult(claims ...tracequery.RootCauseShareClaim) tracequery.Result {
	if len(claims) == 0 {
		claims = []tracequery.RootCauseShareClaim{
			hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 8.000, "seat-udk"),
			hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.364, "seat-udk"),
		}
	}
	items := []tracequery.RootCauseRankItem{hmc377R16ChainSeat(hmc377R16Thread, 2, 20.000, "seat-udk", 900),
		hmc377R16Restriction(hmc377R16Thread, 10, 18.308, 11.364, claims...)}
	return hmc377R11Result(append(items, hmc377R16ContextSeats(9)...)...)
}

// Two parts of one seat priced on one row's own account are one line and one
// summary amount (disjoint instants of one account, reviewer survivor R26); a
// part in case A' beside a plain part stays its own line (R27).
func TestHMC377R17TwoPartsOnOneRow(t *testing.T) {
	faces := hmc377R16Run(t, hmc377R17TwoPartsOneRowResult())
	for _, zh := range []bool{true, false} {
		block := hmc377R16RowBlock(faces.fence(zh), hmc377R16Subject+" · runnable")
		if !hmc377R16Has(block, hmc377R16OfAccount("18.308", "11.364", zh)) || hmc377R16HeldCount(block, zh) != 1 {
			t.Errorf("zh=%t: two parts on one row are one line of 11.364 ms:\n%s", zh, block)
		}
	}
	if got := hmc377R17PreviewFields(faces.preview); got["board_order=2"] != " cpu_restricted_ready_inside_this_row=11.364ms restricted_ready_account_ms=18.308"+hmc377R17Judgment || len(got) != 1 {
		t.Errorf("the summary states the two parts as one amount: %v", got)
	}
	mixed := hmc377R16Run(t, hmc377R17TwoPartsOneRowResult(
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 8.000, "seat-udk"),
		hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.364, "seat-udk"),
		hmc377R16Claim(tracequery.RootCauseShareClaimChainAccount, tracequery.RootCauseShareHolderChainSeat, 20.000, "seat-udk")))
	for _, zh := range []bool{true, false} {
		block := hmc377R16RowBlock(mixed.fence(zh), hmc377R16Subject+" · runnable")
		if hmc377R16HeldCount(block, zh) != 2 || !hmc377R16Has(block, hmc377R16OfAccount("18.308", "3.364", zh)) {
			t.Errorf("zh=%t: a case-A' part and a plain part are two lines:\n%s", zh, block)
		}
	}
}

// hmc377R17MicroResult: the holder is a member of the micro anchored-seat fold
// row (pop w12m udk-irq-7-77).
func hmc377R17MicroResult() tracequery.Result {
	member := tracequery.ThreadRef{Comm: "udk-irq-7", PID: 77}
	items := []tracequery.RootCauseRankItem{
		hmc377R16ChainSeat(member, 7, 0.029, "seat-micro", 900),
		hmc377R16ChainSeat(tracequery.ThreadRef{Comm: "binder:431_4", PID: 8650}, 8, 0.082, "seat-other", 1300),
		hmc377R16Restriction(member, 10, 0.042, 0.029,
			hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 0.029, "seat-micro")),
	}
	return hmc377R11Result(append(items, hmc377R16ContextSeats(9)...)...)
}

// hmc377R17WholeHostAs: the whole shape with its family row moved to another
// lane (the reviewer's probes K and L).
func hmc377R17WholeHostAs(relevance, causality string) tracequery.Result {
	result := hmc377R16WholeResult()
	items := append([]tracequery.RootCauseRankItem(nil), result.RootCauseRank.Items...)
	for i := range items {
		if items[i].ShareHolderKey == "seat-udk" {
			items[i].ChainRelevance, items[i].Causality = relevance, causality
			items[i].ChainAnchoredMs, items[i].ChainAnchorFullMs, items[i].ChainCredentialCensus = 0, 0, ""
		}
	}
	rank := *result.RootCauseRank
	rank.Items = items
	result.RootCauseRank = &rank
	return result
}

// Every fence site that draws a holding row carries its line: the absorbing
// family row drawn in the adjacent stanza and in the background stanza
// (reviewer survivors T53, T54), and a self row of the analysed thread
// (T51: the analysed thread's restriction seat folded by the roster).
func TestHMC377R17HoldingRowInEveryStanza(t *testing.T) {
	for name, result := range map[string]tracequery.Result{
		"adjacent stanza":   hmc377R17WholeHostAs("adjacent", "adjacent_to_wakeup_chain"),
		"background stanza": hmc377R17WholeHostAs("background", "background_pressure"),
	} {
		faces := hmc377R16Run(t, result)
		for _, zh := range []bool{true, false} {
			fence := faces.fence(zh)
			stanza := "◇ "
			if name == "background stanza" {
				stanza = "▒ "
			}
			at := strings.Index(fence, "\n"+stanza)
			if at < 0 || !hmc377R16Has(fence[at:], hmc377R16OfWhich("0.038", zh)) || hmc377R16Count(fence, hmc377R16Desc(zh)) != 1 ||
				!hmc377R16Has(fence[at:], hmc377R16Desc(zh)) {
				t.Errorf("[%s] zh=%t: the stanza row states the absorbed member's time and the description:\n%s", name, zh, fence)
			}
		}
	}
	// The analysed thread's self row as the holder: its restriction seat is
	// no row of the face (only the roster keeps it).
	const kind = "sched_switch_next_info"
	const target, tid = "tquic_network_t-46761", 46761
	edge := hmc377R13Rank("r17-edge", target, tid, tid, "runnable_wait", 2, 112.009, 112.009, 100)
	edge.Causality, edge.OnChainBasis = "self_wall_clock", "self_wall_clock_interval"
	dep := hmc377R13Rank("r17-dep", "Thread-28-46781", 46781, tid, "runnable_wait", 4, 4.406, 4.406, 700)
	cpu := hmc377R14Constrain(hmc377R13Rank("r17-cpu", target, tid, tid, "cpu_affinity_or_cpuset", 13, 0.129, 0.129, 600), kind, []int{4, 5, 6, 7}, []int{0, 1, 2, 3})
	hmc377R12SetRestricted(&cpu, 5.129, 5.0, 0.129)
	cpu.ShareClaims = []types.TraceCausalProjectionShareClaim{{Part: string(tracequery.RootCauseShareClaimAnchored),
		Role: string(tracequery.RootCauseShareHolderChainSeat), ShareMS: 5.0, Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r17-edge"}}}
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true, WakeupPath: []string{"Thread-28-46781", target}, WindowStartTs: 3734.575541, WindowEndTs: 3734.775541,
		OnChainCauses: []types.TraceCausalProjectionNode{dep, edge},
		RankedSeats:   []types.TraceCausalProjectionNode{dep, edge, cpu},
	}
	for _, zh := range []bool{true, false} {
		model, fence := hmc377R12Fence(t, projection, zh)
		if len(model.SelfRows) == 0 {
			t.Fatalf("zh=%t: fixture precondition: the analysed thread's row is a self row:\n%s", zh, fence)
		}
		desc := "CPU约束描述:允许核 4-7 · 排除全域观测核 0-3 · 判定依据 sched_switch_next_info"
		if !zh {
			desc = "CPU-constraint description: allowed CPUs 4-7 · excludes observed CPUs 0-3 · basis sched_switch_next_info"
		}
		head := "CPU亲和/cpuset限制下合计5.129ms:其中5.000ms已含在本行账内,不另计"
		if !zh {
			head = "5.129ms under the CPU affinity/cpuset restriction in all: of which 5.000ms is already inside this row's account, never added again"
		}
		if !hmc377R16Has(fence, head) || !hmc377R16Has(fence, desc) {
			t.Errorf("zh=%t: the self row states the restricted part it holds and the description:\n%s", zh, fence)
		}
	}
}

// The claim selection and the typed account fields (reviewer survivors R02,
// R04, R34, R35, R36): a represented claim, a zero share, a seat with only one
// of the two typed account fields and a seat met twice in the roster.
func TestHMC377R17SelectionAndTypedAccountFields(t *testing.T) {
	const holder = "pub#root_cause_rank:7"
	place := func(m *runtimeTraceProjTreeModel) {
		m.TreeRows = append(m.TreeRows, hmc377R6Row(holder, "dep-200", "E32", runtimeTraceProjTreeRowChain))
	}
	run := func(seats ...types.TraceCausalProjectionNode) []string {
		model := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
		place(&model)
		runtimeTraceProjStampRestrictedHeld(&model, TraceRestrictedReadyHolds(types.TraceCausalProjection{RankedSeats: seats}),
			newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		var got []string
		for _, row := range runtimeTraceProjSMR1AllRows(&model) {
			for _, held := range row.RestrictedHeld {
				got = append(got, runtimeTraceProjRestrictedHeldText(held, true))
			}
		}
		return got
	}
	if got := run(hmc377R16SeatNode(holder)); len(got) != 1 {
		t.Fatalf("fixture precondition: the seat's share is stated once, got %v", got)
	}
	represented := hmc377R16SeatNode(holder)
	represented.ShareClaims[0].Represented = true
	zero := hmc377R16SeatNode(holder)
	zero.ShareClaims[0].ShareMS = 0
	accountOnly := hmc377R16SeatNode(holder)
	accountOnly.ShareCountedElsewhereMS = 0
	elsewhereOnly := hmc377R16SeatNode(holder)
	elsewhereOnly.RestrictedReadyAccountMS = 0
	for name, seat := range map[string]types.TraceCausalProjectionNode{"represented claim": represented, "zero share": zero,
		"account without a share counted elsewhere": accountOnly, "share counted elsewhere without an account": elsewhereOnly} {
		if got := run(seat); len(got) != 0 {
			t.Errorf("%s: nothing is stated, got %v", name, got)
		}
	}
	if got := run(hmc377R16SeatNode(holder), hmc377R16SeatNode(holder)); len(got) != 1 {
		t.Errorf("a seat met twice in the roster decides once, got %v", got)
	}
}

// Where the claim places a share (reviewer survivors R11, R14, R16, R21,
// R22, R25, R28, R45en): a restriction record drawn inside another row is no
// row of its own; an overflow fold row and a row without data publish no
// account; a holder named twice is one holder; the line sits on the first
// holder in drawing order whatever order the claim names them in; another
// row that publishes the holder's account is named by its own tag; the
// absorbed member's family row must publish an account; and the en account
// word of a seat drawn inside its hop row names that seat.
func TestHMC377R17PlacementOfAClaim(t *testing.T) {
	seat := func(holders ...string) types.TraceCausalProjectionNode {
		node := hmc377R16SeatNode(holders[0])
		node.ShareClaims[0].HolderEvidenceIDs = holders
		return node
	}
	stamp := func(m *runtimeTraceProjTreeModel, seats ...types.TraceCausalProjectionNode) map[string][]string {
		runtimeTraceProjStampRestrictedHeld(m, TraceRestrictedReadyHolds(types.TraceCausalProjection{RankedSeats: seats, AbsorbedChainRows: m.absorbedChainRows}),
			newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		out := map[string][]string{}
		for _, row := range runtimeTraceProjSMR1AllRows(m) {
			for _, held := range row.RestrictedHeld {
				out[row.EvidenceTag] = append(out[row.EvidenceTag], runtimeTraceProjRestrictedHeldText(held, true), runtimeTraceProjRestrictedHeldText(held, false))
			}
		}
		return out
	}
	// R11: the restriction seat's record drawn inside the chain row's bracket
	// (not its own row): the holding row still states the share.
	{
		m := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
		host := hmc377R6Row("pub#root_cause_rank:7", "dep-200", "E32(+1)", runtimeTraceProjTreeRowChain)
		host.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{Rank: 9, EvidenceTag: "E33", DisplayImpactMS: 18.753, drawn: hmc377R6PeerDrawn("pub#root_cause_rank:25", 18.753)}}
		m.TreeRows = append(m.TreeRows, host)
		if got := stamp(&m, seat("pub#root_cause_rank:7")); len(got["E32(+1)"]) != 2 {
			t.Errorf("R11: a restriction record drawn inside another row is no row of its own: %v", got)
		}
	}
	// R14, R16: a one-member overflow fold row and a row without data hold nothing.
	for name, edit := range map[string]func(*runtimeTraceProjTreeRow){
		"overflow fold row": func(r *runtimeTraceProjTreeRow) { r.Node.OnChainOverflowFold = true },
		"row without data":  func(r *runtimeTraceProjTreeRow) { r.HasData = false },
	} {
		m := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
		row := hmc377R6Row("pub#root_cause_rank:7", "dep-200", "E32", runtimeTraceProjTreeRowChain)
		edit(&row)
		m.TreeRows = append(m.TreeRows, row)
		if got := stamp(&m, seat("pub#root_cause_rank:7")); len(got) != 0 {
			t.Errorf("%s: no line, got %v", name, got)
		}
		// The same for an absorbed member's family row (R28).
		fm := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
		fam := hmc377R6Row("pub#root_cause_rank:7", "dep-200", "E32", runtimeTraceProjTreeRowChain)
		fam.Node.RankFamilyKey = "fam-k"
		edit(&fam)
		fm.TreeRows = append(fm.TreeRows, fam)
		member := hmc377R16SeatNode("pub#root_cause_rank:7")
		member.EvidenceID, member.AbsorbedByRankFamily, member.AbsorbedInto = "pub#root_cause_rank:26", true, "fam-k"
		member.RestrictedReadyAccountMS, member.ShareCountedElsewhereMS = 0, 0
		fm.absorbedChainRows = []types.TraceCausalProjectionNode{member}
		if got := stamp(&fm); len(got) != 0 {
			t.Errorf("%s as the family row: no line, got %v", name, got)
		}
	}
	// R21, R22, R25: a holder named twice; holders named against drawing
	// order; another row that adopted the holder's account named by its own tag.
	{
		m := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
		m.TreeRows = append(m.TreeRows, hmc377R6Row("pub#root_cause_rank:6", "dep-200", "E31", runtimeTraceProjTreeRowChain))
		hop := hmc377R6Row("chain-host", "dep-200", "E36(+1)", runtimeTraceProjTreeRowChain)
		drawn := hmc377R6PeerDrawn("pub#root_cause_rank:7", 0.511)
		drawn.own = true
		hop.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{Rank: 7, EvidenceTag: "E37", DisplayImpactMS: 0.511, drawn: drawn}}
		m.TreeRows = append(m.TreeRows, hop)
		got := stamp(&m, seat("pub#root_cause_rank:7", "pub#root_cause_rank:6", "pub#root_cause_rank:7"))
		zh, en := "CPU亲和/cpuset限制下合计19.025ms:其中0.272ms已含在本行与[E36(+1)]账内,不另计",
			"19.025ms under the CPU affinity/cpuset restriction in all: of which 0.272ms is already inside the accounts of this row and [E36(+1)], never added again"
		if len(got) != 1 || len(got["E31"]) != 2 || got["E31"][0] != zh || got["E31"][1] != en {
			t.Errorf("R21/R22/R25: one line on the first drawn holder, naming the other row by its own tag once:\n%v", got)
		}
	}
	// R45en: a rank seat drawn inside its hop row, the host publishing its own
	// account: both languages name the seat's account.
	{
		m := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
		hop := hmc377R6Row("chain-host", "dep-200", "E36(+1)", runtimeTraceProjTreeRowChain)
		hop.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{Rank: 7, EvidenceTag: "E37", DisplayImpactMS: 0.511, drawn: hmc377R6PeerDrawn("pub#root_cause_rank:7", 0.511)}}
		m.TreeRows = append(m.TreeRows, hop)
		got := stamp(&m, seat("pub#root_cause_rank:7"))
		zh, en := "CPU亲和/cpuset限制下合计19.025ms:其中0.272ms已含在[E37]账内,不另计",
			"19.025ms under the CPU affinity/cpuset restriction in all: of which 0.272ms is already inside the account of [E37], never added again"
		if len(got["E36(+1)"]) != 2 || got["E36(+1)"][0] != zh || got["E36(+1)"][1] != en {
			t.Errorf("R45en: the hop row names the seat's account in both languages:\n%v", got)
		}
	}
}

// R-377-17 ①: the uncharged spans are found only on the writer's own preview
// rows (the rows directly below a preview header line, through the shared
// constants), at most the preview cap of them per header; a line that is no
// such row is never uncharged — the same row text without a header above it,
// a row past the cap, a row after an ordinary line, the header text inside
// another line.
func TestHMC377R17UnchargedSpansOnlyOnTheWritersPreviewRows(t *testing.T) {
	const fields = hmc377R16ShortFields
	row := hmc377R16PreviewRow
	var b strings.Builder
	b.WriteString(row(0, fields)) // no header above it
	b.WriteString("note " + hmc377R16PreviewHeader(1))
	b.WriteString(row(0, fields)) // below a header text inside another line
	b.WriteString(hmc377R16PreviewHeader(traceQueryRootCauseRankPreviewCap + 1))
	for i := 1; i <= traceQueryRootCauseRankPreviewCap+1; i++ {
		b.WriteString(row(i, fields)) // the last one is past the cap
	}
	b.WriteString(hmc377R16PreviewHeader(2))
	b.WriteString(row(1, fields))
	b.WriteString("- body_line index=1\n")
	b.WriteString(row(2, fields)) // after an ordinary line
	text := b.String()
	spans := traceQuerySummaryUnchargedSpans(text)
	if len(spans) != traceQueryRootCauseRankPreviewCap+1 {
		t.Fatalf("spans on the %d rows of the two preview blocks, got %d: %+v", traceQueryRootCauseRankPreviewCap+1, len(spans), spans)
	}
	for i, s := range spans {
		lineStart := strings.LastIndexByte(text[:s.start], '\n') + 1
		prev := strings.LastIndexByte(text[:lineStart-1], '\n') + 1
		if text[s.start:s.end] != fields || !strings.HasPrefix(text[lineStart:], traceQueryRankPreviewRowPrefix) {
			t.Errorf("span %d is the fields of a preview row: %q", i, text[s.start:s.end])
		}
		if i == 0 && !strings.HasPrefix(text[prev:], traceQueryRankPreviewHeaderPrefix) {
			t.Errorf("the first span sits on the first row below a header")
		}
	}
	if strings.Count(text[spans[traceQueryRootCauseRankPreviewCap-1].end:spans[traceQueryRootCauseRankPreviewCap].start], "\n") != 3 {
		t.Errorf("the row past the cap is skipped and the next block's first row is a span")
	}
}

// R-377-17 ①: StoreBlob is byte-identical to the plain preview for every other
// tool (whatever its output carries) and for a trace_query output without the
// fields, with and without a work directory; the stored file is the output.
func TestHMC377R17StoreBlobIsUnchangedWithoutTheFields(t *testing.T) {
	long := func(prefix string) string {
		var b strings.Builder
		b.WriteString(prefix)
		for i := 0; b.Len() < MaxInlineBytes+3*previewHeadBytes; i++ {
			fmt.Fprintf(&b, "- body_line index=%06d value=%06d\n", i, i*7)
		}
		return b.String()
	}
	withFields := long(hmc377R16PreviewHeader(1) + hmc377R16PreviewRow(1, hmc377R16ShortFields))
	without := long(hmc377R16PreviewHeader(1) + hmc377R16PreviewRow(1, ""))
	for _, c := range []struct {
		tool, output string
	}{{"exec_command", withFields}, {"read_file", withFields}, {"trace_query", without}, {"trace_query", long("plain\n")}} {
		if got, ref := StoreBlob(nil, c.tool, c.output); got != buildPreview([]byte(c.output), "") || ref != "" {
			t.Errorf("%s without a work directory: the plain preview", c.tool)
		}
		dir := t.TempDir()
		got, ref := StoreBlob(&types.BusContext{WorkDir: dir}, c.tool, c.output)
		if ref == "" || got != buildPreview([]byte(c.output), ref) {
			t.Errorf("%s with a work directory: the plain preview of the stored output (ref %q)", c.tool, ref)
		}
		if raw, err := os.ReadFile(ref); err != nil || string(raw) != c.output {
			t.Errorf("%s: the stored file is the output (err=%v)", c.tool, err)
		}
	}
	// A trace_query output with the fields is not the plain preview: its head
	// keeps the lines it has without them.
	if got, _ := StoreBlob(nil, "trace_query", withFields); got == buildPreview([]byte(withFields), "") {
		t.Errorf("fixture precondition: the fields are uncharged on a trace_query summary")
	}
}

// R-377-17 ①: at most the preview's rows carry the fields — the writer appends
// them only to the rows it emits, and the scanner reads no more rows than it.
func TestHMC377R17OnlyEmittedPreviewRowsCarryTheFields(t *testing.T) {
	result := hmc377R16PartResult(true)
	items := append([]tracequery.RootCauseRankItem(nil), result.RootCauseRank.Items...)
	// Push the holding row past the preview: twelve larger on-chain rows first.
	var front []tracequery.RootCauseRankItem
	for i := 0; i < traceQueryRootCauseRankPreviewCap; i++ {
		ms := 90 - float64(i)
		front = append(front, tracequery.RootCauseRankItem{Rank: 1 + i, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats",
			Thread:   tracequery.ThreadRef{Comm: fmt.Sprintf("lead%d", i), PID: 9500 + i},
			ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.7, LineStart: 4000 + 10*i, LineEnd: 4005 + 10*i})
	}
	rank := *result.RootCauseRank
	rank.Items = append(front, items...)
	result.RootCauseRank = &rank
	preview := hmc377R16Run(t, result).preview
	if got := hmc377R17PreviewFields(preview); len(got) != 0 {
		t.Errorf("a holding row past the preview carries nothing on it: %v", got)
	}
	if fields := traceQueryRankRestrictedReadyFields(result.RootCauseRank, traceQueryRootCauseRankPreviewCap); len(fields) != 0 {
		t.Errorf("the writer gives no row below the visible ones fields: %v", fields)
	}
	if fields := traceQueryRankRestrictedReadyFields(result.RootCauseRank, len(rank.Items)); len(fields) != 1 {
		t.Errorf("fixture precondition: with every row visible the holding row has its fields: %v", fields)
	}
}

// hmc377R17TwoSeatsTwoRowsResult: two restriction seats of one thread, each
// folded, each handing a part to another chain row of the thread.
func hmc377R17TwoSeatsTwoRowsResult() tracequery.Result {
	walked, unwalked := hmc377R16TwoSeats()
	a := hmc377R16Restriction(hmc377R16Thread, 10, 18.308, 8.000,
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 8.000, "seat-walked"))
	b := hmc377R16Restriction(hmc377R16Thread, 11, 5.000, 3.364,
		hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.364, "seat-unwalked"))
	b.LineStart, b.LineEnd = 1902, 1902
	return hmc377R11Result(append([]tracequery.RootCauseRankItem{walked, unwalked, a, b}, hmc377R16ContextSeats(9)...)...)
}

// One description per thread, not per seat (R-377-17 ⑤): two restriction
// seats of one thread, neither drawn, holding on two rows — each row states
// its seat's part; the thread's description (reader) and judgment (summary)
// follow once, on the first holding row.
func TestHMC377R17TwoSeatsOfOneThreadDescribeOnce(t *testing.T) {
	result := hmc377R17TwoSeatsTwoRowsResult()
	faces := hmc377R16Run(t, result)
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		first, second := hmc377R16RowBlock(fence, "20.000ms"), hmc377R16RowBlock(fence, "3.364ms")
		if !hmc377R16Has(first, hmc377R16OfAccount("18.308", "8.000", zh)) || !hmc377R16Has(first, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the first holding row states seat A's part and the thread's description:\n%s", zh, first)
		}
		if !hmc377R16Has(second, hmc377R16OfAccount("5.000", "3.364", zh)) || hmc377R16Has(second, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the second holding row states seat B's part and no second description:\n%s", zh, second)
		}
		if got := hmc377R16Count(fence, hmc377R16Desc(zh)); got != 1 {
			t.Errorf("zh=%t: the thread is described %d time(s), want once:\n%s", zh, got, fence)
		}
	}
	got := hmc377R17PreviewFields(faces.preview)
	want := map[string]string{
		"board_order=2": " cpu_restricted_ready_inside_this_row=8.000ms restricted_ready_account_ms=18.308" + hmc377R17Judgment,
		"board_order=3": " cpu_restricted_ready_inside_this_row=3.364ms restricted_ready_account_ms=5.000",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the summary gives each row its seat's part; the judgment once per thread:\n got %v\nwant %v", got, want)
	}
}

// The split rule of every face (TraceRestrictedReadySplit): the account is
// split when it exceeds the stated time at the printed precision — never by
// float noise below it, never when it equals it.
func TestHMC377R17SplitRule(t *testing.T) {
	for _, c := range []struct {
		account, stated float64
		split           bool
	}{{18.308, 11.364, true}, {18.308, 18.308, false}, {18.3080004, 18.308, false}, {18.3086, 18.308, true}, {0, 0.038, false}} {
		if got := TraceRestrictedReadySplit(c.account, c.stated); got != c.split {
			t.Errorf("account %v stated %v: split %t, want %t", c.account, c.stated, got, c.split)
		}
	}
	seat := types.TraceCausalProjectionNode{Subject: hmc377R16Subject, Object: "cpu_affinity_or_cpuset", TypeToken: "cpu_affinity_or_cpuset", RestrictedReadyAccountMS: 18.3080004}
	if got := runtimeTraceProjRestrictedHeldText(runtimeTraceProjRestrictedHeld{MS: 18.308, Seat: seat}, true); got != hmc377R16OfWhich("18.308", true) {
		t.Errorf("an account equal to the stated time at the printed precision is not split: %q", got)
	}
}

// The exported decision names only holds a face can place: a split seat's
// claim without a holder on the board is no hold; an absorbed seat's claim is
// one even without holders (its family row places it).
func TestHMC377R17DecisionHoldsArePlaceable(t *testing.T) {
	seat := hmc377R16SeatNode("pub#root_cause_rank:7")
	seat.ShareClaims[0].HolderEvidenceIDs = nil
	if got := TraceRestrictedReadyHolds(types.TraceCausalProjection{RankedSeats: []types.TraceCausalProjectionNode{seat}}); len(got) != 0 {
		t.Errorf("a split seat's claim without holders is no hold: %+v", got)
	}
	absorbed := seat
	absorbed.AbsorbedByRankFamily, absorbed.AbsorbedInto = true, "fam-k"
	absorbed.RestrictedReadyAccountMS, absorbed.ShareCountedElsewhereMS = 0, 0
	got := TraceRestrictedReadyHolds(types.TraceCausalProjection{AbsorbedChainRows: []types.TraceCausalProjectionNode{absorbed}})
	if len(got) != 1 || got[0].FamilyKey != "fam-k" || got[0].MS != 0.272 || len(got[0].HolderIDs) != 0 {
		t.Errorf("an absorbed seat's claim is one hold on its family row: %+v", got)
	}
	// Where no row wears the family key and the claim names no holder, the
	// reader places it nowhere (reviewer survivor R17's shape).
	m := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
	m.TreeRows = append(m.TreeRows, hmc377R6Row("pub#root_cause_rank:7", "dep-200", "E32", runtimeTraceProjTreeRowChain))
	runtimeTraceProjStampRestrictedHeld(&m, got, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range runtimeTraceProjSMR1AllRows(&m) {
		if len(row.RestrictedHeld) != 0 {
			t.Errorf("a hold neither a family row nor a holder places is stated nowhere: %+v", row.RestrictedHeld)
		}
	}
}

// A restriction record drawn inside another restriction row's bracket (not a
// row of its own, reviewer survivor R11) still hands its part to the row that
// holds it; the thread's description stays on the drawn restriction row. Two
// parts of one seat on one row, one inside the row's own account and one
// inside a seat drawn there under its own [E#], are two lines (R27b).
func TestHMC377R17SeatDrawnInsideARestrictionRowAndMixedParts(t *testing.T) {
	stamp := func(m *runtimeTraceProjTreeModel, seats ...types.TraceCausalProjectionNode) map[string][]string {
		runtimeTraceProjStampRestrictedHeld(m, TraceRestrictedReadyHolds(types.TraceCausalProjection{RankedSeats: seats}),
			newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		out := map[string][]string{}
		for _, row := range runtimeTraceProjSMR1AllRows(m) {
			for _, held := range row.RestrictedHeld {
				out[row.EvidenceTag] = append(out[row.EvidenceTag], fmt.Sprintf("%s|describe=%t", runtimeTraceProjRestrictedHeldText(held, true), held.Describe))
			}
		}
		return out
	}
	{
		m := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
		m.TreeRows = append(m.TreeRows, hmc377R6Row("pub#root_cause_rank:7", "dep-200", "E32", runtimeTraceProjTreeRowChain))
		drawn := hmc377R16SeatNode("pub#root_cause_rank:7")
		drawn.EvidenceID, drawn.ShareClaims = "pub#root_cause_rank:24", nil
		drawn.RankSubjects = &types.TraceRankSubjects{Member: types.TraceThreadCoordinate{TID: 200}}
		host := runtimeTraceProjTreeRow{Node: drawn, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E70(+1)"}
		host.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{Rank: 9, EvidenceTag: "E71", DisplayImpactMS: 18.753, drawn: hmc377R6PeerDrawn("pub#root_cause_rank:25", 18.753)}}
		m.Adjacent = append(m.Adjacent, host)
		inside := hmc377R16SeatNode("pub#root_cause_rank:7")
		inside.RankSubjects = &types.TraceRankSubjects{Member: types.TraceThreadCoordinate{TID: 200}}
		got := stamp(&m, inside)
		want := "CPU亲和/cpuset限制下合计19.025ms:其中0.272ms已含在本行账内,不另计|describe=false"
		if len(got) != 1 || len(got["E32"]) != 1 || got["E32"][0] != want {
			t.Errorf("R11: the seat drawn inside a restriction row hands its part to the holding row, the thread described on the drawn row:\n%v", got)
		}
	}
	{
		m := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
		hop := hmc377R6Row("pub#root_cause_rank:6", "dep-200", "E36(+1)", runtimeTraceProjTreeRowChain)
		hop.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{Rank: 7, EvidenceTag: "E37", DisplayImpactMS: 0.511, drawn: hmc377R6PeerDrawn("pub#root_cause_rank:7", 0.511)}}
		m.TreeRows = append(m.TreeRows, hop)
		seat := hmc377R16SeatNode("pub#root_cause_rank:6")
		second := seat.ShareClaims[0]
		second.Part, second.ShareMS, second.HolderEvidenceIDs = string(tracequery.RootCauseShareClaimUnwalked), 0.100, []string{"pub#root_cause_rank:7"}
		seat.ShareClaims = append(seat.ShareClaims, second)
		got := stamp(&m, seat)
		if len(got["E36(+1)"]) != 2 || !strings.HasPrefix(got["E36(+1)"][0], "CPU亲和/cpuset限制下合计19.025ms:其中0.272ms已含在本行账内") ||
			!strings.HasPrefix(got["E36(+1)"][1], "CPU亲和/cpuset限制下合计19.025ms:其中0.100ms已含在[E37]账内") {
			t.Errorf("R27b: a part inside the row's account and a part inside a seat drawn there are two lines:\n%v", got)
		}
	}
}

// The whole arm's family row past the preview carries nothing on it.
func TestHMC377R17WholeFamilyRowPastThePreview(t *testing.T) {
	result := hmc377R16WholeResult()
	var front []tracequery.RootCauseRankItem
	for i := 0; i < traceQueryRootCauseRankPreviewCap; i++ {
		ms := 90 - float64(i)
		front = append(front, tracequery.RootCauseRankItem{Rank: 1 + i, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats",
			Thread:   tracequery.ThreadRef{Comm: fmt.Sprintf("lead%d", i), PID: 9500 + i},
			ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.7, LineStart: 4000 + 10*i, LineEnd: 4005 + 10*i})
	}
	rank := *result.RootCauseRank
	rank.Items = append(front, rank.Items...)
	result.RootCauseRank = &rank
	if got := hmc377R17PreviewFields(hmc377R16Run(t, result).preview); len(got) != 0 {
		t.Errorf("a family row past the preview carries nothing on it: %v", got)
	}
	if fields := traceQueryRankRestrictedReadyFields(result.RootCauseRank, traceQueryRootCauseRankPreviewCap); len(fields) != 0 {
		t.Errorf("the writer gives no row below the visible ones fields: %v", fields)
	}
	if fields := traceQueryRankRestrictedReadyFields(result.RootCauseRank, len(rank.Items)); len(fields) != 1 {
		t.Errorf("fixture precondition: with every row visible the family row has its fields: %v", fields)
	}
}

// hmc377R17AbsorbedWithAccount: the whole shape whose absorbed member carries a
// restricted ready account and no share counted elsewhere.
func hmc377R17AbsorbedWithAccount() tracequery.Result {
	result := hmc377R16WholeResult()
	rank := *result.RootCauseRank
	rank.AbsorbedItems = append([]tracequery.RootCauseRankItem(nil), rank.AbsorbedItems...)
	rank.AbsorbedItems[0].RestrictedReadyAccountMs = 0.500
	result.RootCauseRank = &rank
	return result
}

// HMC377 merge 16 (batch-owner ruling R-377-19 ⑥, 2026-11-25): default-suite
// pins of the shapes whose right behaviour is decided and which the second
// code review's mutants (fix16/review2-code/tools/mutants.py) left unguarded
// by both full default suites — E04, F10, F06, R18, W22, H01, H04. Test only.
// G19 and R10 are not pinned here: they wait on R16X-7 (one host rule) and
// R16X-6 (the reader's co-holder form).

// E04: the restriction subset's typed fields ride only beside a share counted
// elsewhere (R-377-12 ③). A restriction seat with a restricted ready account
// and no share counted elsewhere (real: n05w, n21w, x01w, y41r) keeps none of
// them on its record, and every face reads it as without the account: the
// whole-arm member with such an account is stated unsplit on the reader and
// without the account on the summary.
func TestHMC377R19SubsetFieldsRideOnlyBesideAShareCountedElsewhere(t *testing.T) {
	seat := hmc377R16Restriction(hmc377R16Thread, 10, 7.000, 0)
	result := hmc377R11Result(append([]tracequery.RootCauseRankItem{hmc377R16ChainSeat(hmc377R16Thread, 2, 20.000, "seat-udk", 900), seat}, hmc377R16ContextSeats(3)...)...)
	found := 0
	for _, r := range traceQueryTypedObservations(result, "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC()) {
		if r.Subject != hmc377R16Subject || r.Object != "cpu_affinity_or_cpuset" {
			continue
		}
		found++
		for _, n := range r.RichNotes {
			for _, key := range []string{types.TraceNoteKeyRestrictedReadyAccount, types.TraceNoteKeyShareCountedElsewhere, types.TraceNoteKeyShareCountedHere} {
				if strings.HasPrefix(n, key+"=") {
					t.Errorf("a restriction record without a share counted elsewhere carries %q", n)
				}
			}
		}
	}
	if found != 1 {
		t.Fatalf("fixture precondition: one record of the restriction seat, got %d", found)
	}
	bare := result
	rank := *result.RootCauseRank
	rank.Items = append([]tracequery.RootCauseRankItem(nil), rank.Items...)
	for i := range rank.Items {
		if rank.Items[i].Type == "cpu_affinity_or_cpuset" {
			rank.Items[i].RestrictedReadyAccountMs = 0
		}
	}
	bare.RootCauseRank = &rank
	with, without := hmc377R16Run(t, result), hmc377R16Run(t, bare)
	if with.zh != without.zh || with.en != without.en || with.preview != without.preview {
		t.Errorf("the faces read a restriction seat without a share counted elsewhere as without its account:\n%s\n--- without the account:\n%s", with.zh, without.zh)
	}
	// The whole arm: the absorbed member's own account (0.500 ms) is off its
	// record, so the row states the member's 0.038 ms unsplit.
	faces := hmc377R16Run(t, hmc377R17AbsorbedWithAccount())
	for _, zh := range []bool{true, false} {
		block := hmc377R16RowBlock(faces.fence(zh), hmc377R16Subject+" · runnable")
		if !hmc377R16Has(block, hmc377R16OfWhich("0.038", zh)) || hmc377R16Has(block, "0.500") {
			t.Errorf("zh=%t: the absorbed member is stated unsplit, without an account its record does not carry:\n%s", zh, block)
		}
	}
	if got := hmc377R17PreviewFields(faces.preview); len(got) != 1 || strings.Contains(fmt.Sprint(got), types.TraceNoteKeyRestrictedReadyAccount) {
		t.Errorf("the summary states the member's time without an account: %v", got)
	}
}

// F10: the summary gives the seat's account only where it exceeds the time the
// row states. A seat the engine split whose whole restricted account lies in
// one holder (share = account, nothing counted on the seat itself) is stated
// on that row as its time alone — on the reader the unsplit 「其中」 form.
func TestHMC377R19SummaryAccountOnlyWhenSplit(t *testing.T) {
	walked, unwalked := hmc377R16TwoSeats()
	result := hmc377R11Result(append([]tracequery.RootCauseRankItem{walked, unwalked, hmc377R16Restriction(hmc377R16Thread, 10, 8.000, 8.000,
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 8.000, "seat-walked"))}, hmc377R16ContextSeats(9)...)...)
	faces := hmc377R16Run(t, result)
	got := hmc377R17PreviewFields(faces.preview)
	want := map[string]string{"board_order=2": " cpu_restricted_ready_inside_this_row=8.000ms" + hmc377R17Judgment}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("an account equal to the row's time is not given beside it:\n got %v\nwant %v", got, want)
	}
	for _, zh := range []bool{true, false} {
		if block := hmc377R16RowBlock(faces.fence(zh), "20.000ms"); !hmc377R16Has(block, hmc377R16OfWhich("8.000", zh)) || hmc377R16Has(block, hmc377R16OfAccount("8.000", "8.000", zh)) {
			t.Errorf("zh=%t: the reader states the whole account in the unsplit form:\n%s", zh, block)
		}
	}
}

// F06: a claim that names a visible preview row and a row past the 12 preview
// rows is a claim of several rows: the summary states it on neither (not on
// the visible holder as if that row held the whole share). Named alone, the
// visible holder states it.
func TestHMC377R19SummaryClaimWithAHolderPastThePreview(t *testing.T) {
	build := func(holders ...string) tracequery.Result {
		walked, unwalked := hmc377R16TwoSeats()
		items := []tracequery.RootCauseRankItem{walked}
		for i := 0; i < traceQueryRootCauseRankPreviewCap; i++ {
			ms := 60 - float64(i)
			items = append(items, tracequery.RootCauseRankItem{Rank: 3 + i, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats",
				Thread:   tracequery.ThreadRef{Comm: fmt.Sprintf("lead%d", i), PID: 9500 + i},
				ImpactMs: ms, CumulativeImpactMs: ms, EffectiveImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
				ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.6, LineStart: 4000 + 10*i, LineEnd: 4005 + 10*i})
		}
		unwalked.Rank = 3 + traceQueryRootCauseRankPreviewCap
		items = append(items, unwalked, hmc377R16Restriction(hmc377R16Thread, 40, 18.308, 11.364,
			hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 11.364, holders...)))
		return hmc377R11Result(items...)
	}
	two := build("seat-walked", "seat-unwalked")
	if row := hmc377R19BoardRow(two, "seat-unwalked"); row < traceQueryRootCauseRankPreviewCap {
		t.Fatalf("fixture precondition: the second holder is past the preview (board row %d)", row)
	}
	if got := hmc377R17PreviewFields(hmc377R16Run(t, two).preview); len(got) != 0 {
		t.Errorf("a claim of a visible row and a row past the preview states nothing on the summary: %v", got)
	}
	want := map[string]string{"board_order=2": " cpu_restricted_ready_inside_this_row=11.364ms restricted_ready_account_ms=18.308" + hmc377R17Judgment}
	if got := hmc377R17PreviewFields(hmc377R16Run(t, build("seat-walked")).preview); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("fixture control: the visible holder named alone states the share:\n got %v\nwant %v", got, want)
	}
}

// hmc377R19BoardRow is the board row index of the item carrying holder key.
func hmc377R19BoardRow(result tracequery.Result, key string) int {
	for i, item := range result.RootCauseRank.Items {
		if item.ShareHolderKey == key {
			return i
		}
	}
	return -1
}

// R18: the thread's description goes to its first stated share in drawing
// order, whatever order the seat's claims are listed in. The seat lists its
// unwalked part (the row drawn second) before its walked part (drawn first).
func TestHMC377R19DescriptionOnTheFirstHoldingRowInDrawingOrder(t *testing.T) {
	result := hmc377R16TwoSeatResult(
		hmc377R16Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 3.364, "seat-unwalked"),
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 8.000, "seat-walked"))
	faces := hmc377R16Run(t, result)
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		first, second := hmc377R16RowBlock(fence, "20.000ms"), hmc377R16RowBlock(fence, "3.364ms")
		if !hmc377R16Has(first, hmc377R16OfAccount("18.308", "8.000", zh)) || !hmc377R16Has(first, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the row drawn first states its part and the description:\n%s", zh, first)
		}
		if !hmc377R16Has(second, hmc377R16OfAccount("18.308", "3.364", zh)) || hmc377R16Has(second, hmc377R16Desc(zh)) {
			t.Errorf("zh=%t: the row drawn second states its part and no description:\n%s", zh, second)
		}
		if got := hmc377R16Count(fence, hmc377R16Desc(zh)); got != 1 {
			t.Errorf("zh=%t: the description is printed %d time(s), want once:\n%s", zh, got, fence)
		}
	}
}

// W22: a micro anchored-seat fold row draws several threads, so the 「其中」
// form of a member's unsplit share names the member as its subject in both
// languages (「成员 X」 / "member X's"), never 「本线程」 / "this thread's".
func TestHMC377R19FoldRowNamesTheMemberInTheUnsplitForm(t *testing.T) {
	member := tracequery.ThreadRef{Comm: "udk-irq-7", PID: 77}
	result := hmc377R11Result(append([]tracequery.RootCauseRankItem{
		hmc377R16ChainSeat(member, 7, 0.029, "seat-micro", 900),
		hmc377R16ChainSeat(tracequery.ThreadRef{Comm: "binder:431_4", PID: 8650}, 8, 0.082, "seat-other", 1300),
		hmc377R16Restriction(member, 10, 0.029, 0.029,
			hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 0.029, "seat-micro")),
	}, hmc377R16ContextSeats(9)...)...)
	faces := hmc377R16Run(t, result)
	want := map[bool]string{
		true:  "其中CPU亲和/cpuset限制下0.029ms是成员 udk-irq-7-77 runnable时间里受限的那一部分,已含在本行账内,不另计",
		false: "of which 0.029ms under the CPU affinity/cpuset restriction is the restricted part of member udk-irq-7-77's runnable time, already inside this row's account, never added again",
	}
	for _, zh := range []bool{true, false} {
		fence := faces.fence(zh)
		if hmc377R16Count(fence, want[zh]) != 1 {
			t.Errorf("zh=%t: the fold row names its member in the unsplit form:\n%s", zh, fence)
		}
		if hmc377R16Has(fence, "是本线程runnable") || hmc377R16Has(fence, "part of this thread's runnable") {
			t.Errorf("zh=%t: a fold row never says 「本线程」 / \"this thread's\" for a member's share:\n%s", zh, fence)
		}
	}
}

// H01, H04: a thread is read by its typed identity on its board — the member
// TID (only when typed, TID > 0) with the board's target and params
// fingerprint; a seat without a typed TID is its own identity. Two boards of
// one target in one projection (two queries with different params) each
// describe their seat once.
func TestHMC377R19ThreadIdentityIsTypedTIDOnItsBoard(t *testing.T) {
	untyped := func(id string) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{EvidenceID: id, RankSubjects: &types.TraceRankSubjects{Member: types.TraceThreadCoordinate{TID: 0, Comm: "swapper"}}}
	}
	if TraceRestrictedReadyThread(untyped("a")) == TraceRestrictedReadyThread(untyped("b")) {
		t.Errorf("two seats without a typed TID are two identities")
	}
	board := func(params string) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{EvidenceID: "s-" + params, RankBoardTarget: "app-100", RankBoardParamsFingerprint: params,
			RankSubjects: &types.TraceRankSubjects{Member: types.TraceThreadCoordinate{TID: 76}}}
	}
	if TraceRestrictedReadyThread(board("params-a")) == TraceRestrictedReadyThread(board("params-b")) {
		t.Errorf("one TID on two boards of one target (different params) is two identities")
	}
	if TraceRestrictedReadyThread(board("params-a")) != TraceRestrictedReadyThread(types.TraceCausalProjectionNode{EvidenceID: "other", RankBoardTarget: "app-100",
		RankBoardParamsFingerprint: "params-a", RankSubjects: &types.TraceRankSubjects{Member: types.TraceThreadCoordinate{TID: 76}}}) {
		t.Errorf("one TID on one board is one identity")
	}
	// Through the production path: two queries of one target, one projection.
	a := hmc377R16PartResult(true)
	seatB := hmc377R16ChainSeat(hmc377R16Thread, 2, 25.000, "seat-udk-b", 1500)
	restB := hmc377R16Restriction(hmc377R16Thread, 10, 9.000, 5.000,
		hmc377R16Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 5.000, "seat-udk-b"))
	restB.LineStart, restB.LineEnd = 1902, 1902
	b := hmc377R11Result(append([]tracequery.RootCauseRankItem{seatB, restB}, hmc377R16ContextSeats(9)...)...)
	ra, rb := *a.RootCauseRank, *b.RootCauseRank
	ra.BoardParamsFingerprint, rb.BoardParamsFingerprint = "params-a", "params-b"
	a.RootCauseRank, b.RootCauseRank = &ra, &rb
	records := traceQueryTypedObservations(a, "capture.ftrace", "payload-a", "raw-a", "qa", time.Unix(0, 0).UTC())
	records = append(records, traceQueryTypedObservations(b, "capture.ftrace", "payload-b", "raw-b", "qb", time.Unix(0, 0).UTC())...)
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 || len(TraceRestrictedReadyHolds(set.Projections[0])) != 2 {
		t.Fatalf("fixture precondition: one projection with one hold per board")
	}
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		if !hmc377R16Has(hmc377R16RowBlock(fence, "25.000ms"), hmc377R16OfAccount("9.000", "5.000", zh)) ||
			!hmc377R16Has(hmc377R16RowBlock(fence, "20.000ms"), hmc377R16OfAccount("18.308", "11.364", zh)) {
			t.Fatalf("zh=%t: fixture precondition: each board's holding row states its part:\n%s", zh, fence)
		}
		if got := hmc377R16Count(fence, hmc377R16Desc(zh)); got != 2 {
			t.Errorf("zh=%t: each board describes its seat once, got %d description(s):\n%s", zh, got, fence)
		}
	}
}
