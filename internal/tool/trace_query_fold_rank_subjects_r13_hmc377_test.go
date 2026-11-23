package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 13, integration of the three round-13 lanes (R-377-13 ① × ②).
//
// The one-object fold (①) draws a chain hop row and the rank seat that shows
// exactly the hop's measured account as one row: the hop row hosts, the seat's
// rank identity travels with it. A seat's typed rank subjects (its board's
// analysed thread and the ranked member) exist only on the rank record, and
// two faces read them off a rank row: the lead's subject-scope sentence and
// the analysed thread's 「其中」 host (②, TraceRankSubjects.MemberIsTarget).
// At the naive merge of the lanes (dfe54bcc1) the host carried the seat's
// rank, account disclosure, family key, cause token and process attribution
// but not its rank subjects, so
//   - a lead whose seat was folded lost its subject-scope sentence (present at
//     532dc208d, where the seat was a row of its own; absent at the one-object
//     lane head 65c8375c4 and at the naive merge), and
//   - on a hand board whose folded seat is the analysed thread's only ready
//     account row, the thread's CPU-restriction row went back to an additive
//     row of its own (each lane head alone draws the 「其中」 line there).
//
// Evidence: ~/hmc-work/hmc377/fix13/merge/probe_hand/{base,mirror,face,n13,fx}.{lead,boards}.out,
// red/green logs ~/hmc-work/hmc377/fix13/merge/fix/.

// hmc377R13FoldedLeadResult: the board's rank-1 seat is the dependency
// thread's priority-inversion seat (gated composite 2.409 = runnable 1.903 +
// running deficit 0.506), and the chain publishes that thread's hop row on the
// same instants. sameAccount=true: both carry the producer's one
// scheduler-state account identity (the seat's measured account, the hop's
// state account). sameAccount=false is the control: two different accounts,
// two rows.
func hmc377R13FoldedLeadResult(sameAccount bool) tracequery.Result {
	const hopAccount = "state_account:v2:r13-merge-6326"
	seatAccount := hopAccount
	if !sameAccount {
		seatAccount = "state_account:v2:r13-merge-other"
	}
	window := tracequery.TimeWindow{StartTs: 10.2, EndTs: 10.2046}
	seat := tracequery.RootCauseRankItem{Rank: 1, Tier: "primary", Type: "priority_inversion_candidate", Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Dep,
		ImpactMs: 4.622, CumulativeImpactMs: 4.622, EffectiveImpactMs: 2.409, RunnableMs: 1.903, RunningMs: 2.719, DominantState: "running",
		GatedRunnableMs: 1.903, GatedRunningDeficitMs: 0.506,
		PriorityRelationCaliber: "closed_range_stable", PriorityRelationProvenLowerMs: 3,
		PriorityRelationArtifactSources: []string{"artifact:1"},
		ChainRelevance:                  "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: window.StartTs, EndTs: window.EndTs, LineStart: 10, LineEnd: 20,
		MeasuredStateAccountKey: seatAccount}
	hop := tracequery.WakeupCausalImpact{Thread: hmc377R9Dep, ChainDepth: 1, DominantState: "runnable", DominantImpactMs: 1.903, TotalMs: 4.622,
		RunnableMs: 1.903, RunningMs: 2.719, OnChain: true, LineStart: 10, LineEnd: 20, Window: window,
		PriorityInversionCandidate: true, PriorityRelation: "lower_priority_dependency", PriorityRelationCaliber: "closed_range_stable",
		PriorityRelationProvenLowerMs: 3, PriorityRelationArtifactSources: []string{"artifact:1"},
		PriorityInversionGatedMs: 2.409, GatedRunnableMs: 1.903, GatedRunningDeficitMs: 0.506, StateAccountKey: hopAccount}
	return tracequery.Result{
		TraceArtifacts: []tracequery.TraceArtifactSource{{SourcePath: "capture.ftrace", Kind: "ftrace", CausalCompatible: true},
			{SourcePath: "other.htrace", Kind: "hitrace", CausalCompatible: true}},
		WakeupChain: &tracequery.ChainResult{Target: hmc377R9Target, Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			CausalImpacts: []tracequery.WakeupCausalImpact{hop}},
		RootCauseRank: &tracequery.RootCauseRankResult{Target: hmc377R9Target, Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
			Items: []tracequery.RootCauseRankItem{seat}},
	}
}

// The lead of a board whose rank-1 seat is drawn as one row with its hop keeps
// the subject-scope sentence of the seat's board: engine records → typed
// observations → the ledger of a request that declares its target → the
// projection → the reader lead. Hand oracle from the universe definition: the
// board analyses app-100 (no TGID recorded) and the request names no thread,
// so the sentence is the unbound form for TID 100, once, in both languages,
// whether or not the seat and its hop are one row.
func TestHMC377R13FoldedLeadSeatKeepsItsSubjectScope(t *testing.T) {
	request := types.RequestModel{RuntimeTargetProfile: &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNoNamedTarget}}
	want := map[bool]string{
		true:  "这条因果链围绕查询选中的线程 TID 100；这条链的证据不能外推为应用层面的结论。",
		false: "This chain is for the query-selected thread TID 100; the evidence for this chain does not establish an application-wide conclusion.",
	}
	for _, sameAccount := range []bool{true, false} {
		name := "two accounts (control)"
		wantRows := 2
		if sameAccount {
			name, wantRows = "one account", 1
		}
		t.Run(name, func(t *testing.T) {
			ledger := types.CompileObservationLedger(types.ObservationLedgerInput{RequestModel: &request})
			ledger.Records = traceQueryTypedObservations(hmc377R13FoldedLeadResult(sameAccount), "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC())
			set := types.CompileTraceCausalProjectionSet(ledger)
			if len(set.Projections) != 1 {
				t.Fatalf("projections %d", len(set.Projections))
			}
			projection := set.Projections[0]
			for _, zh := range []bool{true, false} {
				lang := "en"
				if zh {
					lang = "zh"
				}
				model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				fence := runtimeTraceProjTreeFence(model, zh)
				// Universe precondition: one account is one tree row, two
				// accounts are two.
				if rows := r13HeadLines(fence, hmc377R9DepSubject); len(rows) != wantRows {
					t.Fatalf("zh=%t: %d tree rows of %s, want %d:\n%s", zh, len(rows), hmc377R9DepSubject, wantRows, fence)
				}
				lead := runtimeTraceProjReaderLeadText(projection, model, lang, zh)
				if got := strings.Count(lead, want[zh]); got != 1 {
					t.Errorf("zh=%t: the lead states the subject scope %d time(s), want once:\n%s", zh, got, lead)
				}
			}
		})
	}
}

// The same carrier under R-377-13 ②, on a hand board (the rank wire mints the
// inversion candidacy on chain records only, so no engine board makes a census
// ready row a fold arm today; the board composes the two lanes' rules
// directly): the analysed thread's hop row and its inversion ready seat show
// one measured account and are one row, and that row is still the thread's
// ready account — its CPU-restriction row hangs under it as the 「其中」 line,
// wearing the restriction seat's ➍ and [E#], and is never a row of its own.
// The control (two accounts, two rows) draws the same line under the seat.
func TestHMC377R13FoldedSelfSeatStillHostsTheRestrictionLine(t *testing.T) {
	const target, tid = ".ugc.aweme.lite-59591", 59591
	subjects := &types.TraceRankSubjects{Target: types.TraceThreadCoordinate{TID: tid, TGID: 10600}, Member: types.TraceThreadCoordinate{TID: tid, TGID: 10600}}
	board := func(sameAccount, flat bool) types.TraceCausalProjection {
		const hopAccount = "state_account:v2:r13-merge-self"
		hop := r13Hop("self-hop", hopAccount)
		hop.Subject = target
		seat := r13Seat("self-seat", hopAccount)
		if !sameAccount {
			seat.MeasuredStateAccountKey = "state_account:v2:r13-merge-self-other"
		}
		seat.Subject, seat.Object, seat.TypeToken = target, "priority_inversion_runnable_wait", "priority_inversion_runnable_wait"
		seat.PriorityInversionCandidate = true
		seat.ShareClaims, seat.ChainAnchoredMS, seat.ChainAnchorFullMS = nil, 0, 0
		seat.RankSubjects = subjects
		restriction := hmc377R13Rank("self-cpu", target, tid, tid, "cpu_affinity_or_cpuset", 4, 3.0, 3.0, 100)
		projection := r13Projection(hop, seat, restriction)
		if flat {
			projection.WakeupPath = nil
		}
		return projection
	}
	for _, flat := range []bool{false, true} {
		for _, sameAccount := range []bool{true, false} {
			for _, zh := range []bool{true, false} {
				projection := board(sameAccount, flat)
				model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				fence := runtimeTraceProjTreeFence(model, zh)
				name := map[bool]string{false: "self stanza", true: "flat board"}[flat] + "/" + map[bool]string{true: "one account", false: "two accounts"}[sameAccount] +
					"/" + map[bool]string{true: "zh", false: "en"}[zh]
				ofWhich, restrictionWord := "其中CPU亲和/cpuset限制下3.000ms", "CPU亲和"
				if !zh {
					ofWhich, restrictionWord = "of which 3.000ms under the CPU affinity/cpuset restriction", "CPU affinity"
				}
				// The restriction is said once, as the 「其中」 line, with
				// its badge on that line and nowhere else.
				if got := strings.Count(fence, ofWhich); got != 1 {
					t.Errorf("%s: %d 「其中」 line(s) for the restriction, want 1:\n%s", name, got, fence)
				}
				if got := strings.Count(fence, restrictionWord); got != 1 {
					t.Errorf("%s: the restriction is named %d time(s) on the tree, want once (the 「其中」 line):\n%s", name, got, fence)
				}
				if got := strings.Count(fence, "➍"); got != 1 {
					t.Errorf("%s: ➍ drawn %d time(s), want once:\n%s", name, got, fence)
				}
			}
		}
	}
}
