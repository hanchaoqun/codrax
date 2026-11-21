package tool

// HMC377 fix round 6 (R6-2 / R6-3, R-377-7) — whole-original public-entry
// regression on the protected NetEase reboot5 window (opt-in: skip when the
// capture variable is unset, fail when its byte size or SHA-256 differs).
//
// The two ◇ remainders HeapTaskDaemon-11098 0.310 ms and wk:1/1/0/8-6934
// 0.075 ms hand their anchored shares to the inversion candidates
// root_cause_rank:2 / :3, which the tree folds into the ➋ / ➌ running rows
// [E4(+1)] / [E5(+3)] (their own [E#] in the row bracket). Round 5 wrote
// 「锚定席未上本榜」 on both while the seats were drawn on the same board
// (batch-owner ruling R-377-7: the sentence is false and is corrected; values,
// ranks, lanes and structure do not move). The reviewer's round-5b probe adds
// a represented satellite naming root_cause_rank:2: its holder state must be
// rendered under the folded twin's [E#] and it must stay out of the ◎
// population.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC377R6WholeOriginalNetEaseHoldersDrawnInsideChainRows(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC376_NETEASE_REBOOT_FRAME", hmc377NetEaseDigest, hmc377NetEaseSize)
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	var results []types.ToolResult
	var native tracequery.Result
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": view, "pid": 11077, "target_scope": "thread",
			"time_start": 153.763595, "time_end": 153.887340})
		if !r.Success {
			t.Fatalf("%s failed: %s", view, r.Summary)
		}
		results = append(results, r)
		if view == "root_cause_rank" {
			native = hmc367NativeResult(t, r, view)
		}
	}
	if native.RootCauseRank == nil || len(native.RootCauseRank.Items) == 0 {
		t.Fatal("no rank board")
	}
	if first := native.RootCauseRank.Items[0]; first.Rank != 1 || first.Thread.PID != 11077 || !hmc376Near(first.EffectiveImpactMs, 123.003) {
		t.Fatalf("protected collection wait moved: %s", hmc376RowLabel(first))
	}
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
	bus.ToolResults = results
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) == 0 {
		t.Fatal("no projection")
	}
	projection := set.Projections[0]
	type remainder struct {
		subject                 string
		remainder, full, anchor string
	}
	wants := []remainder{
		{subject: "HeapTaskDaemon-11098", remainder: "0.310", full: "1.130", anchor: "0.820"},
		{subject: "wk:1/1/0/8-6934", remainder: "0.075", full: "0.106", anchor: "0.031"},
	}
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		index := runtimeTraceProjDrawnRecordIndex(&model, runtimeTraceProjSMR1AllRows(&model))
		seen := 0
		for _, row := range runtimeTraceProjSMR1AllRows(&model) {
			n := row.Node
			if !n.ChainAnchorRemainderSeat {
				continue
			}
			for _, w := range wants {
				if n.Subject != w.subject || hmc377R6Ms3(n.ChainAnchorFullMS) != w.full {
					continue
				}
				seen++
				if hmc377R6Ms3(runtimeTraceProjNodeDisplayImpact(n)) != w.remainder || hmc377R6Ms3(n.ChainAnchoredMS) != w.anchor || n.ChainRelevance != "adjacent" {
					t.Fatalf("zh=%t: protected remainder %s moved: %+v", zh, w.subject, n)
				}
				var holder string
				for _, c := range n.ShareClaims {
					if c.Part == string(tracequery.RootCauseShareClaimAnchored) && c.Resolved && len(c.HolderEvidenceIDs) == 1 {
						holder = c.HolderEvidenceIDs[0]
					}
				}
				drawn, ok := index[holder]
				if holder == "" || !ok || drawn.own || drawn.row == row {
					t.Fatalf("zh=%t: %s: the anchored share's holder %q must be drawn inside another row (the folded inversion candidate): %+v", zh, w.subject, holder, drawn)
				}
				if row.ChainAnchorTwinInvisible || row.ChainAnchorTwinUnreferenced {
					t.Fatalf("zh=%t: %s: the holder is drawn as [%s] inside [%s], the remainder says invisible=%t unreferenced=%t",
						zh, w.subject, drawn.tag, drawn.row.EvidenceTag, row.ChainAnchorTwinInvisible, row.ChainAnchorTwinUnreferenced)
				}
				if row.AccountRelRef != "" {
					t.Fatalf("zh=%t: %s: no pair sentence borrows the host row's account (got %q)", zh, w.subject, row.AccountRelRef)
				}
			}
		}
		if seen != len(wants) {
			t.Fatalf("zh=%t: both protected remainders render, saw %d", zh, seen)
		}
		fence := runtimeTraceProjTreeFence(model, zh)
		flat := strings.ReplaceAll(fence, "\n", " ")
		for _, banned := range []string{"锚定席未上本榜", "anchored seat not on this board"} {
			if strings.Contains(flat, banned) {
				t.Fatalf("zh=%t: R-377-7 — the holders are drawn on this board, the face says %q:\n%s", zh, banned, fence)
			}
		}
		for _, w := range wants {
			want := "全窗" + w.full + "ms=锚定" + w.anchor + "ms(⛓链上席)+本行其余" + w.remainder + "ms(无链上凭证)"
			if !zh {
				want = "full-window " + w.full + "ms=" + w.anchor + "ms anchored (⛓ chain seat) + this remainder " + w.remainder + "ms (no chain credential)"
			}
			if !rspaFenceContains(fence, want) {
				t.Fatalf("zh=%t: %s: the corrected split sentence %q:\n%s", zh, w.subject, want, fence)
			}
		}
		// The reviewer's round-5b probe: a represented satellite of
		// HeapTaskDaemon-11098 naming root_cause_rank:2.
		p := projection
		holder := ""
		for _, n := range p.OnChainCauses {
			if strings.HasSuffix(n.EvidenceID, "#root_cause_rank:2") {
				holder = n.EvidenceID
			}
		}
		if holder == "" {
			t.Fatal("holder record root_cause_rank:2 not in the projection")
		}
		sat := types.TraceCausalProjectionNode{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "hmc377-r6-sat",
			Subject: "HeapTaskDaemon-11098", Object: "scheduler_latency", TypeToken: "scheduler_latency", StateKind: "runnable", ChainRelevance: "adjacent",
			ImpactMS: 0.5, CumulativeImpactMS: 0.5, EffectiveImpactMS: 0.5, ChainAnchorRepresentedByChainSeat: true,
			Rank: 9, Confidence: 0.78, LineStart: 572500, LineEnd: 572600,
			ShareClaims: []types.TraceCausalProjectionShareClaim{{Part: "anchored", Role: "chain_seat", ShareMS: 0.5, Resolved: true, Represented: true, HolderEvidenceIDs: []string{holder}}}}
		p.AdjacentCauses = append(append([]types.TraceCausalProjectionNode(nil), p.AdjacentCauses...), sat)
		model = buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		index = runtimeTraceProjDrawnRecordIndex(&model, runtimeTraceProjSMR1AllRows(&model))
		row := hmc377R6RowOf(&model, "hmc377-r6-sat")
		drawn, ok := index[holder]
		if row == nil || !ok || drawn.own {
			t.Fatalf("zh=%t: the satellite renders and its holder is drawn inside the ➋ running row: row=%v drawn=%+v", zh, row != nil, drawn)
		}
		if row.RepresentedState != runtimeTraceProjShareRefRendered || strings.Join(row.RepresentedRefs, ",") != drawn.tag {
			t.Fatalf("zh=%t: the satellite's holder is drawn as [%s]: state=%d refs=%v", zh, drawn.tag, row.RepresentedState, row.RepresentedRefs)
		}
		if runtimeTraceProjElimEligible(*row) {
			t.Fatalf("zh=%t: the satellite's share is held by a drawn seat — it never enters the ◎ population", zh)
		}
	}
}

func hmc377R6Ms3(v float64) string { return fmt.Sprintf("%.3f", v) }
