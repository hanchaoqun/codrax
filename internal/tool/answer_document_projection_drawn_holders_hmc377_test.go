package tool

// HMC377 fix round 6 (R6-2 / R6-3, R-377-7) — a share's holder is "on this
// board" iff its record is drawn somewhere on the projection, under the [E#]
// it is drawn with: a row's own record, a record merged into a row, a rank
// twin folded into its chain row, every other folded peer, an engine-absorbed
// chain row inside its family row. Round 5 indexed the rows' own and merged
// ids only: the NetEase inversion candidate (root_cause_rank:2), folded into
// the ➋ running row, read as not on this board — its ◇ remainder said
// 「锚定席未上本榜」 while the seat was drawn, and a represented satellite of
// it went back into the ◎ population beside it (two owners of one share).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	hmc377R6Chain  = "pub-chain#wakeup_causal_impact:2"
	hmc377R6Holder = "pub-rank#root_cause_rank:2"
	hmc377R6Twin   = "pub-rank#root_cause_rank:18"
	hmc377R6Sat    = "pub-rank#root_cause_rank:9"
)

// hmc377R6FoldedHolderProjection is the NetEase reboot5 shape: the engine
// publishes one inversion-candidate segment of dep-200 through the chain lane
// (wakeup_causal_impact) and the rank lane (root_cause_rank:2, the ⛓ holder
// of the anchored share); the tree folds the rank twin into the chain row.
// root_cause_rank:18 is its ◇ remainder, and — the reviewer's round-5b
// probe — a represented satellite names the same holder.
func hmc377R6FoldedHolderProjection(withSatellite bool) types.TraceCausalProjection {
	window := func(n *types.TraceCausalProjectionNode) {
		n.QueryWindowStartTs, n.QueryWindowEndTs = 1.0, 1.1
		n.StartTs, n.EndTs = 1.001, 1.099
	}
	p := types.TraceCausalProjection{WakeupPath: []string{"dep-200", "app-100"}, WindowStartTs: 1.0, WindowEndTs: 1.1}
	hop := types.TraceCausalProjectionNode{Role: types.TraceCausalRoleCausalHop, EvidenceID: hmc377R6Chain,
		Subject: "dep-200", Predicate: "wakeup_causal_impact", Object: "running", StateKind: "running",
		ChainRelevance: "on_chain", ChainDepth: 1, PriorityInversionCandidate: true,
		ImpactMS: 50, CumulativeImpactMS: 50, EffectiveImpactMS: 5.793, Confidence: 0.78, LineStart: 100, LineEnd: 200}
	window(&hop)
	holder := types.TraceCausalProjectionNode{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: hmc377R6Holder,
		Subject: "dep-200", Predicate: "root_cause_secondary", Object: "priority_inversion_candidate", TypeToken: "priority_inversion_candidate",
		StateKind: "running", ChainRelevance: "on_chain", ChainDepth: 1, Rank: 2, Tier: "secondary",
		ImpactMS: 5.793, CumulativeImpactMS: 50, EffectiveImpactMS: 5.793, Confidence: 0.91, LineStart: 100, LineEnd: 200,
		ChainAnchoredMS: 0.82, ChainAnchorFullMS: 1.13,
		ShareClaims: []types.TraceCausalProjectionShareClaim{
			hmc377Claim(tracequery.RootCauseShareClaimRemainder, tracequery.RootCauseShareHolderRemainderSeat, 0.31, true, hmc377R6Twin)}}
	window(&holder)
	twin := types.TraceCausalProjectionNode{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: hmc377R6Twin,
		Subject: "dep-200", Predicate: "root_cause_adjacent", Object: "runnable_wait", TypeToken: "runnable_wait",
		StateKind: "runnable", ChainRelevance: "adjacent", Rank: 1, Tier: "tertiary",
		ImpactMS: 0.31, CumulativeImpactMS: 0.31, EffectiveImpactMS: 0.31, Confidence: 0.76, LineStart: 120, LineEnd: 210,
		ChainAnchoredMS: 0.82, ChainAnchorFullMS: 1.13, ChainAnchorRemainderSeat: true,
		ShareClaims: []types.TraceCausalProjectionShareClaim{
			hmc377Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 0.82, true, hmc377R6Holder)}}
	window(&twin)
	p.OnChainCauses = append(p.OnChainCauses, hop, holder)
	p.SupportingHops = append(p.SupportingHops, hop)
	p.AdjacentCauses = append(p.AdjacentCauses, twin)
	if withSatellite {
		sat := types.TraceCausalProjectionNode{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: hmc377R6Sat,
			Subject: "dep-200", Predicate: "root_cause_adjacent", Object: "scheduler_latency", TypeToken: "scheduler_latency",
			StateKind: "runnable", ChainRelevance: "adjacent", Rank: 2, Tier: "tertiary",
			ImpactMS: 0.5, CumulativeImpactMS: 0.5, EffectiveImpactMS: 0.5, Confidence: 0.78, LineStart: 150, LineEnd: 160,
			ChainAnchorRepresentedByChainSeat: true,
			ShareClaims: []types.TraceCausalProjectionShareClaim{
				hmc377RepresentedClaim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 0.5, hmc377R6Holder)}}
		window(&sat)
		p.AdjacentCauses = append(p.AdjacentCauses, sat)
	}
	return p
}

// hmc377R6AllRows returns every row of the four lanes.
func hmc377R6AllRows(model *runtimeTraceProjTreeModel) []*runtimeTraceProjTreeRow {
	return runtimeTraceProjSMR1AllRows(model)
}

func hmc377R6RowOf(model *runtimeTraceProjTreeModel, id string) *runtimeTraceProjTreeRow {
	for _, row := range hmc377R6AllRows(model) {
		if row.Node.EvidenceID == id {
			return row
		}
	}
	return nil
}

// TestHMC377R6FoldedRankTwinHolderIsOnTheBoard — R6-2 / R6-3: the holder the
// typed claims name is drawn inside the chain row as its folded rank twin;
// the ◇ remainder's anchored word says the chain seat holds it (never "not on
// this board"), the represented satellite names the twin's [E#] and leaves the
// ◎ population, and no pair sentence borrows the host row's account.
func TestHMC377R6FoldedRankTwinHolderIsOnTheBoard(t *testing.T) {
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(hmc377R6FoldedHolderProjection(true), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		if hmc377R6RowOf(&model, hmc377R6Holder) != nil {
			t.Fatalf("zh=%t: fixture must fold the rank twin into its chain row (it renders as its own row)", zh)
		}
		host := hmc377R6RowOf(&model, hmc377R6Chain)
		if host == nil || len(host.RankFoldPeers) != 1 || strings.TrimSpace(host.RankFoldPeers[0].EvidenceTag) == "" {
			t.Fatalf("zh=%t: the chain row must carry the folded rank twin: %+v", zh, host)
		}
		holderTag := strings.TrimSpace(host.RankFoldPeers[0].EvidenceTag)
		twin, sat := hmc377R6RowOf(&model, hmc377R6Twin), hmc377R6RowOf(&model, hmc377R6Sat)
		if twin == nil || sat == nil {
			t.Fatalf("zh=%t: the remainder and the satellite render", zh)
		}
		if twin.ChainAnchorTwinInvisible || twin.ChainAnchorTwinUnreferenced {
			t.Fatalf("zh=%t: the remainder's holder is drawn [%s]: invisible=%t unreferenced=%t", zh, holderTag, twin.ChainAnchorTwinInvisible, twin.ChainAnchorTwinUnreferenced)
		}
		if twin.AccountRelRef != "" || host.AccountRelRef != "" {
			t.Fatalf("zh=%t: the host row publishes the chain lane's account, not the anchored share — no pair sentence: twin→%q host→%q", zh, twin.AccountRelRef, host.AccountRelRef)
		}
		if sat.RepresentedState != runtimeTraceProjShareRefRendered || strings.Join(sat.RepresentedRefs, ",") != holderTag {
			t.Fatalf("zh=%t: the satellite's holder is drawn as [%s]: state=%d refs=%v", zh, holderTag, sat.RepresentedState, sat.RepresentedRefs)
		}
		if runtimeTraceProjElimEligible(*sat) || !runtimeTraceProjElimRepresentedExcluded(*sat) {
			t.Fatalf("zh=%t: a share whose holder is drawn never enters the ◎ population a second time", zh)
		}
		fence := strings.ReplaceAll(runtimeTraceProjTreeFence(model, zh), "\n", " ")
		for _, banned := range []string{"未上本榜", "not on this board"} {
			if strings.Contains(fence, banned) {
				t.Fatalf("zh=%t: the holder is drawn on this board, the face says %q:\n%s", zh, banned, fence)
			}
		}
		anchoredWord, represented := "(⛓链上席)", "锚定份由链席["+holderTag+"]代表"
		if !zh {
			anchoredWord, represented = "(⛓ chain seat)", "represented by the chain seat ["+holderTag+"]"
		}
		if !rspaFenceContains(runtimeTraceProjTreeFence(model, zh), anchoredWord) || !rspaFenceContains(runtimeTraceProjTreeFence(model, zh), represented) {
			t.Fatalf("zh=%t: the remainder names the chain seat %q and the satellite names [%s]:\n%s", zh, anchoredWord, holderTag, fence)
		}
	}
}

// --- property: holder state over every drawn-id class -----------------------

// hmc377R6DrawnCase places one holder record somewhere (or nowhere) and
// expects the holder state and [E#] of the two claimants naming it.
type hmc377R6DrawnCase struct {
	name     string
	place    func(model *runtimeTraceProjTreeModel, holder string)
	resolved bool
	// wantTag: the [E#] the holder is drawn under ("" = not drawn).
	wantTag string
	// self: the holder is drawn only inside the claimant's own row.
	self bool
}

func hmc377R6Node(id, subject, state string) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{EvidenceID: id, Subject: subject, StateKind: state, TypeToken: "runnable_wait",
		ImpactMS: 3, CumulativeImpactMS: 3, EffectiveImpactMS: 3}
}

func hmc377R6Row(id, subject, tag, kind string) runtimeTraceProjTreeRow {
	return runtimeTraceProjTreeRow{Node: hmc377R6Node(id, subject, "runnable"), Kind: kind, HasData: true, EvidenceTag: tag}
}

func hmc377R6PeerDrawn(id string, ms float64) runtimeTraceProjPeerDrawn {
	return runtimeTraceProjPeerDrawn{recordIDs: []string{id}, displayMS: ms}
}

func hmc377R6DrawnCases() []hmc377R6DrawnCase {
	return []hmc377R6DrawnCase{
		{name: "self row (own record)", resolved: true, wantTag: "E31", place: func(m *runtimeTraceProjTreeModel, h string) {
			m.SelfRows = append(m.SelfRows, hmc377R6Row(h, "app-100", "E31", runtimeTraceProjTreeRowSelf))
		}},
		{name: "tree trunk chain node", resolved: true, wantTag: "E32", place: func(m *runtimeTraceProjTreeModel, h string) {
			m.TreeRows = append(m.TreeRows, hmc377R6Row(h, "dep-200", "E32", runtimeTraceProjTreeRowChain))
		}},
		{name: "tree cause child", resolved: true, wantTag: "E33", place: func(m *runtimeTraceProjTreeModel, h string) {
			m.TreeRows = append(m.TreeRows, hmc377R6Row(h, "dep-200", "E33", runtimeTraceProjTreeRowCause))
		}},
		{name: "depthless on-chain row", resolved: true, wantTag: "E34", place: func(m *runtimeTraceProjTreeModel, h string) {
			m.TreeRows = append(m.TreeRows, hmc377R6Row(h, "dep-200", "E34", runtimeTraceProjTreeRowDepthless))
		}},
		{name: "record merged into another row", resolved: true, wantTag: "E35(+1)", place: func(m *runtimeTraceProjTreeModel, h string) {
			row := hmc377R6Row("merged-host", "dep-200", "E35(+1)", runtimeTraceProjTreeRowChain)
			row.Node.MergedEvidenceIDs = []string{h}
			m.TreeRows = append(m.TreeRows, row)
		}},
		{name: "rank twin folded into a chain row", resolved: true, wantTag: "E37", place: func(m *runtimeTraceProjTreeModel, h string) {
			row := hmc377R6Row("chain-host", "dep-200", "E36(+1)", runtimeTraceProjTreeRowChain)
			row.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{Rank: 2, EvidenceTag: "E37", DisplayImpactMS: 3, drawn: hmc377R6PeerDrawn(h, 3)}}
			m.TreeRows = append(m.TreeRows, row)
		}},
		{name: "self-symptom twin folded into a self row", resolved: true, wantTag: "E39", place: func(m *runtimeTraceProjTreeModel, h string) {
			row := hmc377R6Row("self-host", "app-100", "E38", runtimeTraceProjTreeRowSelf)
			row.SelfSymptomFoldPeers = []runtimeTraceProjSelfSymptomFoldPeer{{EvidenceTag: "E39", drawn: hmc377R6PeerDrawn(h, 3)}}
			m.SelfRows = append(m.SelfRows, row)
		}},
		{name: "same-segment mirror", resolved: true, wantTag: "E41", place: func(m *runtimeTraceProjTreeModel, h string) {
			row := hmc377R6Row("mirror-host", "dep-200", "E40", runtimeTraceProjTreeRowChain)
			row.SameSegMirrorPeers = []runtimeTraceProjSameSegMirrorPeer{{EvidenceTag: "E41", drawn: hmc377R6PeerDrawn(h, 3)}}
			m.TreeRows = append(m.TreeRows, row)
		}},
		{name: "branch-twin aggregate copy", resolved: true, wantTag: "E43", place: func(m *runtimeTraceProjTreeModel, h string) {
			row := hmc377R6Row("branch-host", "dep-200", "E42", runtimeTraceProjTreeRowDepthless)
			row.BranchTwinFoldPeers = []runtimeTraceProjBranchTwinFoldPeer{{EvidenceTag: "E43", drawn: hmc377R6PeerDrawn(h, 3)}}
			m.TreeRows = append(m.TreeRows, row)
		}},
		{name: "same-thread IO peer", resolved: true, wantTag: "E45", place: func(m *runtimeTraceProjTreeModel, h string) {
			row := hmc377R6Row("io-host", "dep-200", "E44", runtimeTraceProjTreeRowAdjacent)
			row.IOFoldPeers = []runtimeTraceProjIOFoldPeer{{Token: "io_wait", ImpactMS: 3, EvidenceTag: "E45", drawn: hmc377R6PeerDrawn(h, 3)}}
			m.Adjacent = append(m.Adjacent, row)
		}},
		{name: "adjacent stanza row", resolved: true, wantTag: "E46", place: func(m *runtimeTraceProjTreeModel, h string) {
			m.Adjacent = append(m.Adjacent, hmc377R6Row(h, "dep-200", "E46", runtimeTraceProjTreeRowAdjacent))
		}},
		{name: "background stanza row", resolved: true, wantTag: "E47", place: func(m *runtimeTraceProjTreeModel, h string) {
			m.Background = append(m.Background, hmc377R6Row(h, "dep-200", "E47", runtimeTraceProjTreeRowBackground))
		}},
		{name: "self side lane (relocated non-chain self row)", resolved: true, wantTag: "E48", place: func(m *runtimeTraceProjTreeModel, h string) {
			row := hmc377R6Row(h, "app-100", "E48", runtimeTraceProjTreeRowSelf)
			row.SelfSymptomRelocated = true
			m.SelfRows = append(m.SelfRows, row)
		}},
		{name: "engine-absorbed chain row inside its family row", resolved: true, wantTag: "E49", place: func(m *runtimeTraceProjTreeModel, h string) {
			row := hmc377R6Row("family-host", "dep-200", "E49", runtimeTraceProjTreeRowChain)
			row.Node.RankFamilyKey = "fam-k"
			m.TreeRows = append(m.TreeRows, row)
			absorbed := hmc377R6Node(h, "dep-200", "runnable")
			absorbed.AbsorbedByRankFamily, absorbed.AbsorbedInto = true, "fam-k"
			m.absorbedChainRows = append(m.absorbedChainRows, absorbed)
		}},
		{name: "drawn nowhere (compacted off the projection)", resolved: true, place: func(m *runtimeTraceProjTreeModel, h string) {}},
		{name: "absorbed row whose family row is not drawn", resolved: true, place: func(m *runtimeTraceProjTreeModel, h string) {
			absorbed := hmc377R6Node(h, "dep-200", "runnable")
			absorbed.AbsorbedByRankFamily, absorbed.AbsorbedInto = true, "fam-gone"
			m.absorbedChainRows = append(m.absorbedChainRows, absorbed)
		}},
		{name: "drawn but the engine did not resolve the claim", resolved: false, place: func(m *runtimeTraceProjTreeModel, h string) {
			m.TreeRows = append(m.TreeRows, hmc377R6Row(h, "dep-200", "E50", runtimeTraceProjTreeRowChain))
		}},
		{name: "drawn only inside the claimant's own row", resolved: true, self: true, place: func(m *runtimeTraceProjTreeModel, h string) {}},
	}
}

// TestHMC377R6HolderStateFollowsEveryDrawnIDClass — the property: for every
// claim, the holder state is rendered [E#] iff the holder record is drawn
// somewhere on the projection (in another row than the claimant's), under the
// [E#] it is drawn with; and the ◎ population never contains a share whose
// holder is drawn. Each claimant (a ◇ remainder naming its anchored share's
// holder, a represented satellite) runs on its own board.
func TestHMC377R6HolderStateFollowsEveryDrawnIDClass(t *testing.T) {
	const holder = "pub#root_cause_rank:7"
	for _, c := range hmc377R6DrawnCases() {
		for _, claimant := range []string{"remainder", "represented"} {
			for _, zh := range []bool{true, false} {
				label := fmt.Sprintf("[%s] %s zh=%t", c.name, claimant, zh)
				model := runtimeTraceProjTreeModel{WindowMS: 100, Target: "app-100"}
				holders := []string{holder}
				if !c.resolved {
					holders = nil
				}
				row := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{EvidenceID: "claimant",
					Subject: "dep-200", TypeToken: "runnable_wait", StateKind: "runnable", ChainRelevance: "adjacent",
					ImpactMS: 1, CumulativeImpactMS: 1, EffectiveImpactMS: 1, Rank: 1,
					ChainAnchoredMS: 3, ChainAnchorFullMS: 4, ChainAnchorRemainderSeat: true,
					ShareClaims: []types.TraceCausalProjectionShareClaim{
						hmc377Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 3, true, holders...)}},
					Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E1"}
				if claimant == "represented" {
					row.Node = types.TraceCausalProjectionNode{EvidenceID: "claimant",
						Subject: "dep-200", TypeToken: "scheduler_latency", Predicate: "root_cause_adjacent", StateKind: "runnable", ChainRelevance: "adjacent",
						ImpactMS: 3, CumulativeImpactMS: 3, EffectiveImpactMS: 3, Rank: 2, Confidence: 0.78,
						ChainAnchorRepresentedByChainSeat: true,
						ShareClaims: []types.TraceCausalProjectionShareClaim{
							hmc377RepresentedClaim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 3, holders...)}}
				}
				if c.self {
					// The holder rides the claimant's own row (a merged record
					// and a folded peer of it).
					row.Node.MergedEvidenceIDs = []string{holder}
					row.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{EvidenceTag: "E3", drawn: hmc377R6PeerDrawn(holder, 3)}}
				}
				model.Adjacent = append(model.Adjacent, row)
				c.place(&model, holder)
				runtimeTraceProjMarkAccountRelations(&model, zh)
				got := hmc377R6RowOf(&model, "claimant")
				drawnElsewhere := c.wantTag != ""
				if claimant == "remainder" {
					switch {
					case c.self:
						if got.ChainAnchorTwinInvisible || !got.ChainAnchorTwinUnreferenced {
							t.Fatalf("%s: a holder drawn only inside the claimant's own row names no other seat and claims no absence: invisible=%t unreferenced=%t",
								label, got.ChainAnchorTwinInvisible, got.ChainAnchorTwinUnreferenced)
						}
					case drawnElsewhere:
						if got.ChainAnchorTwinInvisible || got.ChainAnchorTwinUnreferenced {
							t.Fatalf("%s: the holder is drawn as [%s], the remainder says invisible=%t unreferenced=%t", label, c.wantTag, got.ChainAnchorTwinInvisible, got.ChainAnchorTwinUnreferenced)
						}
					default:
						if !got.ChainAnchorTwinInvisible {
							t.Fatalf("%s: a holder drawn nowhere (or an unresolved claim) reads as compacted", label)
						}
					}
				} else {
					switch {
					case c.self:
						if got.RepresentedState != runtimeTraceProjShareRefNone {
							t.Fatalf("%s: a holder drawn only inside the claimant's own row names no other seat: state=%d refs=%v", label, got.RepresentedState, got.RepresentedRefs)
						}
					case drawnElsewhere:
						if got.RepresentedState != runtimeTraceProjShareRefRendered || strings.Join(got.RepresentedRefs, ",") != c.wantTag {
							t.Fatalf("%s: the satellite names the holder as drawn [%s]: state=%d refs=%v", label, c.wantTag, got.RepresentedState, got.RepresentedRefs)
						}
					default:
						if got.RepresentedState != runtimeTraceProjShareRefCompacted || len(got.RepresentedRefs) != 0 {
							t.Fatalf("%s: a holder drawn nowhere (or an unresolved claim) reads as compacted: state=%d refs=%v", label, got.RepresentedState, got.RepresentedRefs)
						}
					}
					if eligible := runtimeTraceProjElimEligible(*got); eligible == drawnElsewhere {
						t.Fatalf("%s: ◎ population membership %t, holder drawn elsewhere %t — one owner per share", label, eligible, drawnElsewhere)
					}
				}
				fence := strings.ReplaceAll(runtimeTraceProjTreeFence(model, zh), "\n", " ")
				absent := strings.Contains(fence, "未上本榜") || strings.Contains(fence, "not on this board")
				if drawnElsewhere && absent {
					t.Fatalf("%s: the holder is drawn [%s], the face says it is not on this board:\n%s", label, c.wantTag, fence)
				}
				if !drawnElsewhere && !c.self && !absent {
					t.Fatalf("%s: the holder is drawn nowhere, the face must say so:\n%s", label, fence)
				}
				if claimant == "represented" && drawnElsewhere && !strings.Contains(fence, "["+c.wantTag+"]") {
					t.Fatalf("%s: the represented sentence names the holder's [%s]:\n%s", label, c.wantTag, fence)
				}
			}
		}
	}
}
