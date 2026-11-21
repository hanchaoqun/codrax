package tool

// HMC377 fix round 7 (R7-1, batch-owner ruling R-377-8) — no reader face
// asserts a holder's absence. Rounds 4, 5 and 6 each found a new record
// identity under which a holder can be drawn that the renderer's lookup did
// not know (a rank twin folded into a chain row; peers and absorbed rows; the
// chain view's own record of the same wakeup causal impact after the rank cap
// cut the rank copy — review round 6: 105 蜂鸟 container #1 TID 7912 ◇ E43
// 「锚定1.903ms(锚定席未上本榜,见明细)」 beside ⇅ E27(+1) drawing that
// runnable 1.903, 105 元宝 container #2 TID 50874 ◇ E41 and 105 网易 container
// #1 TID 22853 ◇ E41 「链席未上本榜(见明细)」 beside E21(+1) / E11(+1)). An
// absence sentence is only true if the lookup is complete over every identity
// that can draw an account, which the renderer cannot prove — so a holder is
// named only from a positive proof (the held form or its [E#]), and otherwise
// the existing see-detail form, which claims nothing about the board, speaks.
// The retired words leave every reader face and the legend's promise text.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc377R7RetiredAbsenceWords are the retired absence words (R-377-8).
var hmc377R7RetiredAbsenceWords = []string{"未上本榜", "溢出未上榜", "not on this board", "overflowed off this board"}

// hmc377R7Squash drops whitespace and the tree's box-drawing gutter so a word
// wrapped across fence lines is still found.
func hmc377R7Squash(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) || (r >= 0x2500 && r <= 0x257f) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// hmc377R7RetiredAbsenceWord returns the first retired absence word the text
// carries ("" when none).
func hmc377R7RetiredAbsenceWord(text string) string {
	squashed := hmc377R7Squash(text)
	for _, w := range hmc377R7RetiredAbsenceWords {
		if strings.Contains(squashed, hmc377R7Squash(w)) {
			return w
		}
	}
	return ""
}

// The legend's promise text says what the face does: no entry asserts a
// seat's absence, and the three-way entry promises the see-detail pointer.
func TestHMC377R7LegendPromisesNoAbsence(t *testing.T) {
	for _, e := range runtimeTraceProjLegendCatalog() {
		for _, text := range []string{e.ZH, e.EN} {
			if w := hmc377R7RetiredAbsenceWord(text); w != "" {
				t.Errorf("legend entry %v promises an absence word %q: %s", e.Mark, w, text)
			}
		}
		if e.Mark == runtimeTraceProjMarkChainAnchorThreeWay {
			if !strings.Contains(e.ZH, "行内以「见明细」指向明细,不对该席是否在本榜作断言") ||
				!strings.Contains(e.EN, "the row points to the detail index instead (see the detail index) and asserts nothing about whether that seat is on the board") {
				t.Errorf("the three-way entry promises the see-detail pointer:\n%s\n%s", e.ZH, e.EN)
			}
		}
	}
}

// hmc377R7Claimant is one row whose sentences name holders.
type hmc377R7Claimant struct {
	name     string
	threeWay bool
	// build returns the claimant row with its claims naming holder (and the
	// unwalked part's holder unwalked on a three-way split).
	build func(holders, unwalked []string) runtimeTraceProjTreeRow
	// seeDetail is the claimant's see-detail word for the main holder.
	seeDetailZH, seeDetailEN string
}

func hmc377R7Claimants() []hmc377R7Claimant {
	base := func(kind string, onChain bool) runtimeTraceProjTreeRow {
		rel := "adjacent"
		if onChain {
			rel = "on_chain"
		}
		return runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{EvidenceID: "claimant",
			Subject: "dep-200", TypeToken: "runnable_wait", StateKind: "runnable", ChainRelevance: rel,
			ImpactMS: 1, CumulativeImpactMS: 1, EffectiveImpactMS: 1, Rank: 1},
			Kind: kind, HasData: true, EvidenceTag: "E1"}
	}
	split := func(row *runtimeTraceProjTreeRow, threeWay bool) {
		row.Node.ChainAnchoredMS, row.Node.ChainAnchorFullMS = 3, 4
		if threeWay {
			row.Node.ChainAnchorUnwalkedMS, row.Node.ChainAnchorOutsideMS = 0.5, 0.5
		}
	}
	withUnwalked := func(row *runtimeTraceProjTreeRow, unwalked []string) {
		if row.Node.ChainAnchorUnwalkedMS > 0 {
			row.Node.ShareClaims = append(row.Node.ShareClaims,
				hmc377Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderUnwalkedSeat, 0.5, true, unwalked...))
		}
	}
	var out []hmc377R7Claimant
	for _, three := range []bool{false, true} {
		three := three
		suffix := map[bool]string{false: "two-way", true: "three-way"}[three]
		out = append(out,
			hmc377R7Claimant{name: "◇ remainder " + suffix, threeWay: three, seeDetailZH: "(锚定席见明细)", seeDetailEN: "(anchored seat: see the detail index)",
				build: func(holders, unwalked []string) runtimeTraceProjTreeRow {
					row := base(runtimeTraceProjTreeRowAdjacent, false)
					split(&row, three)
					row.Node.ChainAnchorRemainderSeat = true
					row.Node.ShareClaims = []types.TraceCausalProjectionShareClaim{
						hmc377Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 3, true, holders...)}
					withUnwalked(&row, unwalked)
					return row
				}},
			hmc377R7Claimant{name: "⛓ seat " + suffix, threeWay: three, seeDetailZH: "(◇余段席见明细)", seeDetailEN: "(◇ remainder seat: see the detail index)",
				build: func(holders, unwalked []string) runtimeTraceProjTreeRow {
					row := base(runtimeTraceProjTreeRowChain, true)
					split(&row, three)
					rem := 1.0
					if three {
						rem = 0.5
					}
					row.Node.ShareClaims = []types.TraceCausalProjectionShareClaim{
						hmc377Claim(tracequery.RootCauseShareClaimRemainder, tracequery.RootCauseShareHolderRemainderSeat, rem, true, holders...)}
					withUnwalked(&row, unwalked)
					return row
				}},
			hmc377R7Claimant{name: "◇ divergent remainder " + suffix, threeWay: three, seeDetailZH: "链席自账见明细", seeDetailEN: "the chain seat's own account: see the detail index",
				build: func(holders, unwalked []string) runtimeTraceProjTreeRow {
					row := base(runtimeTraceProjTreeRowAdjacent, false)
					split(&row, three)
					row.Node.ChainAnchorRemainderSeat, row.Node.ChainAnchorOwnershipDivergent = true, true
					row.Node.ChainAnchorChainLaneMS, row.Node.ChainAnchorCensusMS = 2.5, 3
					row.Node.ShareClaims = []types.TraceCausalProjectionShareClaim{
						hmc377Claim(tracequery.RootCauseShareClaimChainAccount, tracequery.RootCauseShareHolderChainSeat, 2.5, false, holders...)}
					withUnwalked(&row, unwalked)
					return row
				}},
		)
	}
	out = append(out, hmc377R7Claimant{name: "represented satellite",
		seeDetailZH: "锚定份由本线程另一席持有(见明细)", seeDetailEN: "anchored share held by another seat of this thread (see the detail index)",
		build: func(holders, _ []string) runtimeTraceProjTreeRow {
			return runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{EvidenceID: "claimant",
				Subject: "dep-200", TypeToken: "scheduler_latency", Predicate: "root_cause_adjacent", StateKind: "runnable", ChainRelevance: "adjacent",
				ImpactMS: 3, CumulativeImpactMS: 3, EffectiveImpactMS: 3, Rank: 2, Confidence: 0.78,
				ChainAnchorRepresentedByChainSeat: true,
				ShareClaims: []types.TraceCausalProjectionShareClaim{
					hmc377RepresentedClaim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 3, holders...)}},
				Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E1"}
		}})
	return out
}

// hmc377R7UnwalkedCase places the unwalked part's holder.
type hmc377R7UnwalkedCase struct {
	name     string
	resolved bool
	drawn    bool
}

// TestHMC377R7NoFaceAssertsAHolderAbsent — the property universe: every
// claimant kind (◇ remainder, ⛓ seat, divergent remainder — two- and
// three-way — and a represented satellite) × every holder placement class
// (drawn under each drawn-id class, drawn nowhere, drawn but unresolved, drawn
// only inside the claimant's own row, no typed reference at all) × the
// unwalked part's holder (drawn, drawn nowhere, unresolved), in both
// languages: no face carries a retired absence word, and a holder the lookup
// does not prove drawn elsewhere takes the claimant's see-detail form.
func TestHMC377R7NoFaceAssertsAHolderAbsent(t *testing.T) {
	const holder, unwalkedHolder = "pub#root_cause_rank:7", "pub#root_cause_rank:8"
	type placement struct {
		hmc377R6DrawnCase
		unreferenced bool
	}
	var placements []placement
	for _, c := range hmc377R6DrawnCases() {
		placements = append(placements, placement{hmc377R6DrawnCase: c})
	}
	placements = append(placements, placement{hmc377R6DrawnCase: hmc377R6DrawnCase{name: "no typed reference", place: func(*runtimeTraceProjTreeModel, string) {}}, unreferenced: true})
	unwalkedCases := []hmc377R7UnwalkedCase{{"unwalked holder drawn", true, true}, {"unwalked holder drawn nowhere", true, false}, {"unwalked claim unresolved", false, true}}
	boards := 0
	for _, claimant := range hmc377R7Claimants() {
		for _, p := range placements {
			uws := unwalkedCases
			if !claimant.threeWay {
				uws = uws[:1]
			}
			for _, uw := range uws {
				for _, zh := range []bool{true, false} {
					boards++
					label := fmt.Sprintf("[%s | %s | %s] zh=%t", claimant.name, p.name, uw.name, zh)
					model := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
					holders := []string{holder}
					if !p.resolved {
						holders = nil
					}
					unwalked := []string{unwalkedHolder}
					if !uw.resolved {
						unwalked = nil
					}
					row := claimant.build(holders, unwalked)
					if p.unreferenced {
						row.Node.ShareClaims = nil
					}
					if p.self {
						row.Node.MergedEvidenceIDs = []string{holder}
						row.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{EvidenceTag: "E3", drawn: hmc377R6PeerDrawn(holder, 3)}}
					}
					if row.Kind == runtimeTraceProjTreeRowChain {
						model.TreeRows = append(model.TreeRows, row)
					} else {
						model.Adjacent = append(model.Adjacent, row)
					}
					p.place(&model, holder)
					if claimant.threeWay && uw.drawn {
						seat := hmc377R6Row(unwalkedHolder, "dep-200", "E60", runtimeTraceProjTreeRowChain)
						seat.Node.ImpactMS, seat.Node.CumulativeImpactMS, seat.Node.EffectiveImpactMS = 0.5, 0.5, 0.5
						model.TreeRows = append(model.TreeRows, seat)
					}
					runtimeTraceProjMarkAccountRelations(&model, zh)
					fence := runtimeTraceProjTreeFence(model, zh)
					if w := hmc377R7RetiredAbsenceWord(fence); w != "" {
						t.Fatalf("%s: the face asserts a holder's absence (%q):\n%s", label, w, fence)
					}
					provenElsewhere := p.wantTag != "" && !p.self && !p.unreferenced
					want := claimant.seeDetailZH
					if !zh {
						want = claimant.seeDetailEN
					}
					if got := rspaFenceContains(fence, want); got == provenElsewhere {
						t.Fatalf("%s: holder proven drawn elsewhere=%t, see-detail form %q present=%t:\n%s", label, provenElsewhere, want, got, fence)
					}
					if claimant.threeWay && !p.unreferenced {
						word := "(唤醒锚定席见明细)"
						if !zh {
							word = "(wakeup-anchored seat: see the detail index)"
						}
						if got := rspaFenceContains(fence, word); got == (uw.resolved && uw.drawn) {
							t.Fatalf("%s: unwalked holder proven drawn=%t, see-detail form present=%t:\n%s", label, uw.resolved && uw.drawn, got, fence)
						}
					}
				}
			}
		}
	}
	// 4 single-unwalked-case kinds (three two-way, the satellite) + 3
	// three-way kinds × 3 unwalked cases, in two languages.
	if want := 2 * (4 + 3*3) * len(placements); boards != want {
		t.Fatalf("universe size moved: %d boards, want %d", boards, want)
	}
}

// hmc377R7Window is one real window of the round-7 face audit (opt-in: each
// skips without its variable and fails when the file is not the pinned
// original).
type hmc377R7Window struct {
	name, variable, digest string
	size                   int64
	tid                    int
	start, end             float64
}

func hmc377R7Windows() []hmc377R7Window {
	return []hmc377R7Window{
		{"honeybird 64156 (protected)", "CODRAX_HMC365_HONEYBIRD", hmc377HoneybirdDigest, hmc377HoneybirdSize, 64156, 11343.248, 11343.310},
		{"netease reboot5 11077 (protected)", "CODRAX_HMC376_NETEASE_REBOOT_FRAME", hmc377NetEaseDigest, hmc377NetEaseSize, 11077, 153.763595, 153.887340},
		{"douyin reboot5 11105", "CODRAX_HMC377_DOUYIN_REBOOT", hmc377DouyinDigest, hmc377DouyinSize, 11105, 140.911304, 141.835458},
		{"honeybird reboot 10566", "CODRAX_HMC377_HONEYBIRD_REBOOT", "54405241db707b901057b41a5c94e69a3fc233425fdf7a1f745e43585cf248d4", 124269104, 10566, 162.372184, 163.616645},
		{"yuanbao reboot #2 10500", "CODRAX_HMC377_YUANBAO_REBOOT2", "6bdca58a1a7cefd4d33c0d9cc0f9e5b7a7591b15c69644714c31d5f78a466b2c", 78415317, 10500, 158.472783, 160.598142},
		{"yuanbao container #2 50874", "CODRAX_HMC377_YUANBAO_CONTAINER2", "22c9ba3708a9bbc0c1c31703138859c8b00f9f6bfe39df1e1cdd9e18802edd4a", 62845037, 50874, 3820.346447, 3821.062086},
		{"netease container #1 22853", "CODRAX_HMC377_NETEASE_CONTAINER1", "54a8c7156c3db0892fee70347a856cd5abf7257f77fca6523dfbd39d88a5bf02", 115428261, 22853, 1511.649083, 1512.730965},
		{"honeybird container #1 7912", "CODRAX_HMC377_HONEYBIRD_CONTAINER1", "ed9dcdc80abeb03c9c9f2613220ca63c71c0af4b2103293423bc1c4eb5843792", 115311764, 7912, 4323.860465, 4324.625879},
	}
}

// hmc377R7ProjectionText is every string the compiled projection publishes
// (its JSON form: the text fields a reader face or a model reads).
func hmc377R7ProjectionText(t *testing.T, results []types.ToolResult) string {
	t.Helper()
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
	bus.ToolResults = results
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	data, err := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	if err != nil {
		t.Fatal(err)
	}
	var strs []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			strs = append(strs, x)
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	walk(decoded)
	return strings.Join(strs, "\n")
}

// TestHMC377R7WholeOriginalNoFaceAssertsAHolderAbsent — the real windows:
// the delivered zh and en documents, every text field of the compiled
// projection and every tool and observation summary carry no retired word.
func TestHMC377R7WholeOriginalNoFaceAssertsAHolderAbsent(t *testing.T) {
	for _, w := range hmc377R7Windows() {
		w := w
		t.Run(w.name, func(t *testing.T) {
			path := hmc376Original(t, w.variable, w.digest, w.size)
			d := hmc377DeliverWindow(t, path, w.tid, w.start, w.end)
			faces := map[string]string{"zh document": d.zh, "en document": d.en, "projection text": hmc377R7ProjectionText(t, d.results)}
			for i, r := range d.results {
				var b strings.Builder
				b.WriteString(r.Summary)
				for _, o := range r.Observations {
					b.WriteString("\n" + o.Summary)
				}
				faces[fmt.Sprintf("tool result %d summaries", i)] = b.String()
			}
			for face, text := range faces {
				if word := hmc377R7RetiredAbsenceWord(text); word != "" {
					at := strings.Index(text, word)
					if at < 0 {
						at = 0
					}
					t.Errorf("%s: %s carries the retired absence word %q near: %s", w.name, face, word, text[at:min(len(text), at+200)])
				}
			}
		})
	}
}
