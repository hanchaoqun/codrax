package types

// XGAP-FIX ① pins (§29.104.6/.8, witness run 20260715-202022.323-89609):
// three accepted emit_investigation_complete calls in one run minted TWO
// 「根因排序」 member_set facts (one role-less/unit-less with 8 seats, one
// principal_answer/席位数 with 3 seats). Seat #1 published two values
// (143.499ms full vs 51.735ms converted) and seat #3 two word forms
// (runnable vs runnable_wait), so the verbatim member-presence obligation
// set became self-contradictory and the finalizer burned five alternating
// rejections. The fix: completion bookkeeping supersedes the earlier
// same-label ordinal-seated member_set fact with the later-accepted one.

import (
	"strings"
	"testing"
)

// xgapWitnessFactEarlier reproduces the emit#2 fact verbatim (no role, no
// unit, 8 ordinal seats with "#N:" separators).
func xgapWitnessFactEarlier() AnswerAggregateFact {
	return AnswerAggregateFact{
		Kind:  AnswerAggregateMemberSet,
		Label: "根因排序",
		Value: "8",
		Members: []string{
			"#1: .ugc.aweme.lite-17267 running 143.499ms (primary)",
			"#2: RenderThread-17597 running 4.958ms (secondary)",
			"#3: .ugc.aweme.lite-17267 runnable 3.437ms",
			"#4: keva-1-17437 priority_inversion 3.437ms",
			"#5: keva-3-17439 priority_inversion 3.309ms",
			"#6: keva-3-17439 io_wait 1.354ms",
			"#7: keva-3-17439 runnable_wait 1.319ms",
			"#8: binder:496_9-10961 running supply_deficit 0.933ms",
		},
	}
}

// xgapWitnessFactLater reproduces the later-accepted emit fact verbatim
// (role=principal_answer, unit=席位数, 3 ordinal seats with "#N " + "·"
// separators and the engine-true converted values).
func xgapWitnessFactLater() AnswerAggregateFact {
	return AnswerAggregateFact{
		Kind:  AnswerAggregateMemberSet,
		Label: "根因排序",
		Value: "3",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Unit:  "席位数",
		Members: []string{
			"#1 .ugc.aweme.lite-17267 · running · 51.735ms · primary",
			"#2 RenderThread-17597 · running · 4.958ms · secondary",
			"#3 .ugc.aweme.lite-17267 · runnable_wait · 3.437ms · tertiary",
		},
	}
}

func TestSupersedeOrdinalMemberSetFactsByLabel_WitnessShape(t *testing.T) {
	earlier := []AnswerAggregateFact{
		xgapWitnessFactEarlier(),
		{Kind: AnswerAggregateScalar, Label: "窗口时长", Value: "233.19ms"},
	}
	later := []AnswerAggregateFact{xgapWitnessFactLater()}
	out := SupersedeOrdinalMemberSetFactsByLabel(earlier, later)
	if len(out) != 1 {
		t.Fatalf("expected the earlier 根因排序 fact to be superseded, got %d fact(s): %+v", len(out), out)
	}
	if out[0].Kind != AnswerAggregateScalar || out[0].Label != "窗口时长" {
		t.Fatalf("supersede must keep the unrelated scalar fact, got %+v", out[0])
	}
}

func TestSupersedeOrdinalMemberSetFactsByLabel_MergedObligationIsUnique(t *testing.T) {
	// End-to-end mint pin: the completion bookkeeping merge shape
	// (earlier-superseded base + later group) must yield exactly ONE
	// 根因排序 fact carrying the later-accepted seats.
	earlier := []AnswerAggregateFact{xgapWitnessFactEarlier()}
	later := []AnswerAggregateFact{xgapWitnessFactLater()}
	merged := MergeAnswerAggregateFacts(
		SupersedeOrdinalMemberSetFactsByLabel(earlier, later), later)
	var ranked []AnswerAggregateFact
	for _, fact := range merged {
		if fact.Kind == AnswerAggregateMemberSet && strings.Contains(fact.Label, "根因排序") {
			ranked = append(ranked, fact)
		}
	}
	if len(ranked) != 1 {
		t.Fatalf("expected exactly one 根因排序 fact after supersede+merge, got %d: %+v", len(ranked), ranked)
	}
	if len(ranked[0].Members) != 3 {
		t.Fatalf("later-accepted fact must win whole, got members %v", ranked[0].Members)
	}
	for _, member := range ranked[0].Members {
		if strings.Contains(member, "143.499") {
			t.Fatalf("superseded seat value leaked into the surviving fact: %q", member)
		}
	}
	if !strings.Contains(ranked[0].Members[0], "51.735") {
		t.Fatalf("seat #1 must carry the later-accepted value, got %q", ranked[0].Members[0])
	}
}

func TestSupersedeOrdinalMemberSetFactsByLabel_NegativeLanes(t *testing.T) {
	later := []AnswerAggregateFact{xgapWitnessFactLater()}

	t.Run("different label kept", func(t *testing.T) {
		other := xgapWitnessFactEarlier()
		other.Label = "链路节点排序"
		out := SupersedeOrdinalMemberSetFactsByLabel([]AnswerAggregateFact{other}, later)
		if len(out) != 1 {
			t.Fatalf("different-label ordinal fact must be kept, got %d", len(out))
		}
	})

	t.Run("non-ordinal member set kept", func(t *testing.T) {
		other := AnswerAggregateFact{
			Kind:    AnswerAggregateMemberSet,
			Label:   "根因排序",
			Value:   "2",
			Members: []string{"keva-1-17437 (depth=2)", "keva-3-17439 (depth=2)"},
		}
		out := SupersedeOrdinalMemberSetFactsByLabel([]AnswerAggregateFact{other}, later)
		if len(out) != 1 {
			t.Fatalf("non-ordinal member set must never enter the supersede arbitration, got %d", len(out))
		}
	})

	t.Run("mixed ordinal shape disqualifies whole fact", func(t *testing.T) {
		other := xgapWitnessFactEarlier()
		other.Members = append(other.Members, "binder aggregate remainder (no seat)")
		out := SupersedeOrdinalMemberSetFactsByLabel([]AnswerAggregateFact{other}, later)
		if len(out) != 1 {
			t.Fatalf("a single non-ordinal member must disqualify the fact from supersede, got %d", len(out))
		}
	})

	t.Run("disjoint ordinals kept", func(t *testing.T) {
		other := xgapWitnessFactEarlier()
		other.Members = []string{"#9: extra-seat 0.1ms", "#10: extra-seat 0.05ms"}
		out := SupersedeOrdinalMemberSetFactsByLabel([]AnswerAggregateFact{other}, later)
		if len(out) != 1 {
			t.Fatalf("ordinal facts sharing no seat must be kept (fail-open), got %d", len(out))
		}
	})

	t.Run("later without ordinal facts is a no-op", func(t *testing.T) {
		out := SupersedeOrdinalMemberSetFactsByLabel(
			[]AnswerAggregateFact{xgapWitnessFactEarlier()},
			[]AnswerAggregateFact{{Kind: AnswerAggregateScalar, Label: "折算基准", Value: "R5"}})
		if len(out) != 1 {
			t.Fatalf("no later ordinal fact → earlier kept, got %d", len(out))
		}
	})
}

// xgapCrossTargetBoardA / B reproduce the adversarial P1 shape (修补轮 件A,
// 2026-07-16): a multi-step dual-target run mints one legal 「根因排序」 board
// per target — same engine chip label, both wearing #1..#3, seat text not in
// conflict. fork1's board (target logd.writer-2955) uses the earlier emit
// style; fork2's board (target com.baidu.tieba-9163) the later style. Label +
// ordinal intersection alone must NOT supersede across boards.
func xgapCrossTargetBoardA() AnswerAggregateFact {
	return AnswerAggregateFact{
		Kind:  AnswerAggregateMemberSet,
		Label: "根因排序",
		Value: "3",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"#1: logd.writer-2955 running 12.001ms (primary)",
			"#2: logd.reader.per-3001 runnable 5.002ms (secondary)",
			"#3: kworker/u16:5-771 io_wait 2.003ms",
		},
	}
}

func xgapCrossTargetBoardB() AnswerAggregateFact {
	return AnswerAggregateFact{
		Kind:  AnswerAggregateMemberSet,
		Label: "根因排序",
		Value: "3",
		Role:  AnswerAggregateRolePrincipalAnswer,
		Unit:  "席位数",
		Members: []string{
			"#1 com.baidu.tieba-9163 · running · 30.500ms · primary",
			"#2 RenderThread-9188 · runnable · 8.100ms · secondary",
			"#3 binder:9163_2-9201 · io_wait · 3.300ms · tertiary",
		},
	}
}

func TestSupersedeOrdinalMemberSetFactsByLabel_CrossTargetBoardsKept(t *testing.T) {
	// P1 probe shape (direct arm): every shared seat resolves on both sides
	// and every subject pair CONFLICTS → fail-open keep-both.
	out := SupersedeOrdinalMemberSetFactsByLabel(
		[]AnswerAggregateFact{xgapCrossTargetBoardA()},
		[]AnswerAggregateFact{xgapCrossTargetBoardB()})
	if len(out) != 1 {
		t.Fatalf("cross-target board must survive the later board's acceptance, got %d fact(s)", len(out))
	}
	if !strings.Contains(out[0].Members[0], "logd.writer-2955") {
		t.Fatalf("survivor must be the 2955 board, got %v", out[0].Members)
	}
}

func TestSupersedeOrdinalMemberSetFactsByLabel_PartialSharedSeatConflictFailOpen(t *testing.T) {
	// ③ shape: shared #1/#3 subjects agree, shared #2 subject differs — ONE
	// conflicting seat vetoes the whole supersede.
	earlier := xgapWitnessFactEarlier()
	later := xgapWitnessFactLater()
	later.Members[1] = "#2 mali_gpu_worker-17601 · running · 4.958ms · secondary"
	out := SupersedeOrdinalMemberSetFactsByLabel(
		[]AnswerAggregateFact{earlier}, []AnswerAggregateFact{later})
	if len(out) != 1 {
		t.Fatalf("partial shared-seat conflict must fail open and keep the earlier fact, got %d", len(out))
	}

	// Sanity flip: restoring seat #2's subject re-enables the supersede,
	// proving the subject arm (not some other property) decided above.
	out = SupersedeOrdinalMemberSetFactsByLabel(
		[]AnswerAggregateFact{earlier}, []AnswerAggregateFact{xgapWitnessFactLater()})
	if len(out) != 0 {
		t.Fatalf("agreeing shared-seat subjects must supersede, got %d fact(s)", len(out))
	}
}

func TestSupersedeOrdinalMemberSetFactsByLabel_ZeroComparableSeatsFailOpen(t *testing.T) {
	// ④ shape: all seats shared but NO subject resolves as a canonical
	// thread token on either side → zero successful comparisons → keep.
	prose := func(value string, members ...string) AnswerAggregateFact {
		return AnswerAggregateFact{
			Kind:    AnswerAggregateMemberSet,
			Label:   "根因排序",
			Value:   value,
			Members: members,
		}
	}
	earlier := prose("2", "#1: 主体未标识 running 3.000ms", "#2: 聚合剩余 sleep 1.000ms")
	later := prose("2", "#1 未具名主体 · running · 2.000ms", "#2 聚合剩余段 · sleep · 0.500ms")
	out := SupersedeOrdinalMemberSetFactsByLabel(
		[]AnswerAggregateFact{earlier}, []AnswerAggregateFact{later})
	if len(out) != 1 {
		t.Fatalf("zero comparable shared seats must fail open and keep the earlier fact, got %d", len(out))
	}
}

func TestAnswerAggregateMemberOrdinalSeatSubjectShapes(t *testing.T) {
	cases := []struct {
		member  string
		subject string
		ok      bool
	}{
		{"#1: .ugc.aweme.lite-17267 running 143.499ms (primary)", ".ugc.aweme.lite-17267", true},
		{"#3 .ugc.aweme.lite-17267 · runnable_wait · 3.437ms · tertiary", ".ugc.aweme.lite-17267", true},
		{"#8: binder:496_9-10961 running supply_deficit 0.933ms", "binder:496_9-10961", true},
		{"#4：logd.writer-2955 running", "logd.writer-2955", true},
		{"#12", "", false},                   // no remainder
		{"#7·seat", "", false},               // leading token has no tid tail
		{"#1: 主体未标识 running 3ms", "", false}, // prose subject
		// The §11-N7 tail parse is deliberately permissive: any pure-digit
		// tail reads as a thread token ("keva-1" → tid 1). Safe here — the
		// arbitration still compares VERBATIM tokens, so a permissive parse
		// can only turn keep-both into keep-both-or-exact-match.
		{"#2: keva-1 priority_inversion", "keva-1", true},
		{"seat #1 not leading", "", false},
	}
	for _, tc := range cases {
		subject, ok := answerAggregateMemberOrdinalSeatSubject(tc.member)
		if ok != tc.ok || (ok && subject != tc.subject) {
			t.Fatalf("answerAggregateMemberOrdinalSeatSubject(%q) = (%q,%v), want (%q,%v)",
				tc.member, subject, ok, tc.subject, tc.ok)
		}
	}
}

func TestAnswerAggregateMemberOrdinalSeatShapes(t *testing.T) {
	cases := []struct {
		member string
		seat   int
		ok     bool
	}{
		{"#1: .ugc.aweme.lite-17267 running 143.499ms (primary)", 1, true},
		{"#3 .ugc.aweme.lite-17267 · runnable_wait · 3.437ms · tertiary", 3, true},
		{"#12", 12, true},
		{"#7·seat", 7, true},
		{"#4：中文冒号席", 4, true},
		{"#2.dotted seat", 2, true},
		{"#x not a seat", 0, false},
		{"seat #1 not leading", 0, false},
		{"#1st suffix letters", 0, false},
		{"", 0, false},
		{"#", 0, false},
	}
	for _, tc := range cases {
		seat, ok := answerAggregateMemberOrdinalSeat(tc.member)
		if ok != tc.ok || (ok && seat != tc.seat) {
			t.Fatalf("answerAggregateMemberOrdinalSeat(%q) = (%d,%v), want (%d,%v)", tc.member, seat, ok, tc.seat, tc.ok)
		}
	}
}

func TestMergeAnswerAggregateFacts_OrdinalSeatGuardBlocksSameBucketSeatConflict(t *testing.T) {
	// Same explicit bucket (kind/label/role/unit/dims all equal) — the
	// member-slot union path. A conflicting version of an occupied seat must
	// NOT be appended; a genuinely new seat still unions.
	base := xgapWitnessFactLater()
	conflicting := xgapWitnessFactLater()
	conflicting.Members = []string{
		"#1 .ugc.aweme.lite-17267 · running · 143.499ms · primary",  // occupied seat, different value
		"#4 keva-1-17437 · priority_inversion · 3.429ms · tertiary", // new seat
	}
	merged := MergeAnswerAggregateFacts(
		[]AnswerAggregateFact{base},
		[]AnswerAggregateFact{conflicting})
	var ranked *AnswerAggregateFact
	for i := range merged {
		if merged[i].Kind == AnswerAggregateMemberSet && strings.Contains(merged[i].Label, "根因排序") {
			if ranked != nil {
				t.Fatalf("same-bucket facts must fold into one slot, got a second: %+v", merged[i])
			}
			ranked = &merged[i]
		}
	}
	if ranked == nil {
		t.Fatal("merged output lost the 根因排序 fact")
	}
	seat1 := 0
	seat4 := 0
	for _, member := range ranked.Members {
		if strings.HasPrefix(member, "#1 ") {
			seat1++
			if !strings.Contains(member, "51.735") {
				t.Fatalf("seat #1 must keep the first slot's value, got %q", member)
			}
		}
		if strings.HasPrefix(member, "#4 ") {
			seat4++
		}
	}
	if seat1 != 1 {
		t.Fatalf("seat #1 published %d time(s), want exactly 1: %v", seat1, ranked.Members)
	}
	if seat4 != 1 {
		t.Fatalf("non-conflicting seat #4 must still union, got %d occurrence(s): %v", seat4, ranked.Members)
	}
}

func TestMergeExploreFork_LaterAcceptedOrdinalFactSupersedesRetained(t *testing.T) {
	parent := NewMutableState("xgap witness replay")

	// Dispatch 1: fork accepts the earlier emit (根因排序 v1, 8 seats).
	fork1 := parent.ForkForExploreDispatch()
	fork1.SetInvestigationAggregateFacts([]AnswerAggregateFact{xgapWitnessFactEarlier()})
	fork1.SetInvestigationComplete("emit#2 accepted")
	fork1.RetainInvestigationAggregateFacts()
	parent.MergeExploreFork(fork1)

	// Dispatch 2: a later fork accepts the corrected emit (根因排序 v2, 3
	// seats, engine-true values). Its effective facts — like production —
	// still carry the monotonic union shape, so hand it only its own facts
	// and let MergeExploreFork arbitrate against the parent's retained v1.
	fork2 := parent.ForkForExploreDispatch()
	fork2.SetInvestigationAggregateFacts([]AnswerAggregateFact{xgapWitnessFactLater()})
	fork2.SetInvestigationComplete("emit#5 accepted")
	fork2.RetainInvestigationAggregateFacts()
	parent.MergeExploreFork(fork2)

	stable := parent.StableInvestigationAggregateFacts()
	var ranked []AnswerAggregateFact
	for _, fact := range stable {
		if fact.Kind == AnswerAggregateMemberSet && strings.Contains(fact.Label, "根因排序") {
			ranked = append(ranked, fact)
		}
	}
	if len(ranked) != 1 {
		t.Fatalf("witness pathology reproduced: %d 根因排序 fact(s) survived fork merge, want 1: %+v", len(ranked), ranked)
	}
	if len(ranked[0].Members) != 3 || !strings.Contains(ranked[0].Members[0], "51.735") {
		t.Fatalf("later-accepted fact must own the surface, got %v", ranked[0].Members)
	}
	for _, member := range ranked[0].Members {
		if strings.Contains(member, "143.499") {
			t.Fatalf("contradictory seat value survived: %q", member)
		}
	}
}

func TestMergeExploreFork_CrossTargetBoardsBothSurvive(t *testing.T) {
	// P1 end-to-end (types side): two forks complete with one legal board
	// per target. MergeExploreFork must keep BOTH — the earlier board is a
	// different target's principal obligation, not a stale version.
	parent := NewMutableState("dual-target run")

	fork1 := parent.ForkForExploreDispatch()
	fork1.SetInvestigationAggregateFacts([]AnswerAggregateFact{xgapCrossTargetBoardA()})
	fork1.SetInvestigationComplete("target logd.writer-2955 board accepted")
	fork1.RetainInvestigationAggregateFacts()
	parent.MergeExploreFork(fork1)

	fork2 := parent.ForkForExploreDispatch()
	fork2.SetInvestigationAggregateFacts([]AnswerAggregateFact{xgapCrossTargetBoardB()})
	fork2.SetInvestigationComplete("target com.baidu.tieba-9163 board accepted")
	fork2.RetainInvestigationAggregateFacts()
	parent.MergeExploreFork(fork2)

	stable := parent.StableInvestigationAggregateFacts()
	var boards []AnswerAggregateFact
	for _, fact := range stable {
		if fact.Kind == AnswerAggregateMemberSet && strings.Contains(fact.Label, "根因排序") {
			boards = append(boards, fact)
		}
	}
	if len(boards) != 2 {
		t.Fatalf("dual-target merge must keep both boards, got %d: %+v", len(boards), boards)
	}
	surface := ""
	for _, board := range boards {
		surface += strings.Join(board.Members, "\n") + "\n"
	}
	for _, subject := range []string{"logd.writer-2955", "com.baidu.tieba-9163"} {
		if !strings.Contains(surface, subject) {
			t.Fatalf("target %s board vanished from the merged obligations:\n%s", subject, surface)
		}
	}
}

func TestMergeExploreFork_RetainedLaneSupersedesOnRequeueDivergence(t *testing.T) {
	// 件B① pin — the context.go retained-arm supersede wiring. Divergence
	// window: a ResetInvestigationComplete-class requeue leaves
	// investigationComplete=false on both sides while fork.retained
	// (later-accepted board) ≠ parent.retained (earlier board), so ONLY the
	// retained-lane merge arm runs. Same-subject boards must still fold to
	// the later-accepted version there.
	parent := NewMutableState("retained divergence")

	fork1 := parent.ForkForExploreDispatch()
	fork1.SetInvestigationAggregateFacts([]AnswerAggregateFact{xgapWitnessFactEarlier()})
	fork1.SetInvestigationComplete("emit#2 accepted")
	fork1.RetainInvestigationAggregateFacts()
	parent.MergeExploreFork(fork1)
	// Requeue lane (pendingCompletionReset): completion cleared, retained
	// facts survive.
	parent.ResetInvestigationComplete()

	fork2 := parent.ForkForExploreDispatch()
	fork2.SetInvestigationAggregateFacts([]AnswerAggregateFact{xgapWitnessFactLater()})
	fork2.SetInvestigationComplete("emit#5 accepted")
	fork2.RetainInvestigationAggregateFacts()
	fork2.ResetInvestigationComplete()
	if fork2.investigationComplete {
		t.Fatal("precondition: fork must merge with investigationComplete=false")
	}
	parent.MergeExploreFork(fork2)

	stable := parent.StableInvestigationAggregateFacts()
	var ranked []AnswerAggregateFact
	for _, fact := range stable {
		if fact.Kind == AnswerAggregateMemberSet && strings.Contains(fact.Label, "根因排序") {
			ranked = append(ranked, fact)
		}
	}
	if len(ranked) != 1 {
		t.Fatalf("retained-lane merge must supersede the earlier same-subject board, got %d: %+v", len(ranked), ranked)
	}
	if len(ranked[0].Members) != 3 || !strings.Contains(ranked[0].Members[0], "51.735") {
		t.Fatalf("later-accepted board must own the retained surface, got %v", ranked[0].Members)
	}
	// Cross-target boards keep the fail-open on this lane too.
	parentB := NewMutableState("retained divergence cross-target")
	forkA := parentB.ForkForExploreDispatch()
	forkA.SetInvestigationAggregateFacts([]AnswerAggregateFact{xgapCrossTargetBoardA()})
	forkA.SetInvestigationComplete("board A accepted")
	forkA.RetainInvestigationAggregateFacts()
	parentB.MergeExploreFork(forkA)
	parentB.ResetInvestigationComplete()
	forkB := parentB.ForkForExploreDispatch()
	forkB.SetInvestigationAggregateFacts([]AnswerAggregateFact{xgapCrossTargetBoardB()})
	forkB.SetInvestigationComplete("board B accepted")
	forkB.RetainInvestigationAggregateFacts()
	forkB.ResetInvestigationComplete()
	parentB.MergeExploreFork(forkB)
	count := 0
	for _, fact := range parentB.StableInvestigationAggregateFacts() {
		if fact.Kind == AnswerAggregateMemberSet && strings.Contains(fact.Label, "根因排序") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("retained-lane cross-target boards must both survive, got %d", count)
	}
}

func TestObservationLedgerInputs_CrossTurnOrdinalSupersede(t *testing.T) {
	// 件B② pin — the observation_ledger_context.go twin supersede wirings
	// (AgentContext and BusContext builders). This input feeds the
	// projection compilation, so a prior turn's board must not co-publish
	// next to the current turn's same-subject version.
	build := func() *MutableState {
		mu := NewMutableState("xgap ledger mirror")
		mu.SetInvestigationAggregateFacts([]AnswerAggregateFact{xgapWitnessFactLater()})
		mu.SetInvestigationComplete("current turn accepted")
		mu.SetTurnAArtifacts(TurnAArtifacts{
			AcceptedAggregateFacts: []AnswerAggregateFact{xgapWitnessFactEarlier()},
		})
		return mu
	}
	inputs := map[string]ObservationLedgerInput{
		"agent_context": ObservationLedgerInputFromAgentContext(&AgentContext{Mutable: build()}, 0),
		"bus_context":   ObservationLedgerInputFromBusContext(&BusContext{Mutable: build()}, 0),
	}
	for name, input := range inputs {
		var ranked []AnswerAggregateFact
		for _, fact := range input.AggregateFacts {
			if fact.Kind == AnswerAggregateMemberSet && strings.Contains(fact.Label, "根因排序") {
				ranked = append(ranked, fact)
			}
		}
		if len(ranked) != 1 {
			t.Fatalf("%s: cross-turn ledger input minted %d 根因排序 fact(s), want 1: %+v", name, len(ranked), ranked)
		}
		if len(ranked[0].Members) != 3 || !strings.Contains(ranked[0].Members[0], "51.735") {
			t.Fatalf("%s: current turn must own the ledger surface, got %v", name, ranked[0].Members)
		}
	}

	// Negative twin: with no current-turn version the TurnA ferry is intact.
	muEmpty := NewMutableState("xgap ledger mirror empty")
	muEmpty.SetTurnAArtifacts(TurnAArtifacts{
		AcceptedAggregateFacts: []AnswerAggregateFact{xgapWitnessFactEarlier()},
	})
	input := ObservationLedgerInputFromBusContext(&BusContext{Mutable: muEmpty}, 0)
	found := false
	for _, fact := range input.AggregateFacts {
		if strings.Contains(fact.Label, "根因排序") && len(fact.Members) == 8 {
			found = true
		}
	}
	if !found {
		t.Fatalf("prior-turn board must ferry when the current turn has no version, got %+v", input.AggregateFacts)
	}
}

func TestBuildAnswerSurfacePlan_CurrentTurnOrdinalFactSupersedesTurnA(t *testing.T) {
	// Cross-turn mirror of the ① supersede: the plan's StableAggregateFacts
	// list is exactly what the finalize member-set obligation check
	// consumes, so a prior turn's 根因排序 (ferried through TurnAArtifacts)
	// must not co-publish next to the current turn's version.
	mut := NewMutableState("xgap cross-turn")
	mut.SetInvestigationAggregateFacts([]AnswerAggregateFact{xgapWitnessFactLater()})
	mut.SetInvestigationComplete("current turn accepted")
	mut.SetTurnAArtifacts(TurnAArtifacts{
		AcceptedAggregateFacts: []AnswerAggregateFact{xgapWitnessFactEarlier()},
	})
	ir := &AnalysisIR{RequestModel: RequestModel{}}
	plan := BuildAnswerSurfacePlan(ir, mut, nil, nil, nil, nil)
	if plan == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil")
	}
	var ranked []AnswerAggregateFact
	for _, fact := range plan.StableAggregateFacts {
		if fact.Kind == AnswerAggregateMemberSet && strings.Contains(fact.Label, "根因排序") {
			ranked = append(ranked, fact)
		}
	}
	if len(ranked) != 1 {
		t.Fatalf("cross-turn merge minted %d 根因排序 fact(s), want 1: %+v", len(ranked), ranked)
	}
	if len(ranked[0].Members) != 3 || !strings.Contains(ranked[0].Members[0], "51.735") {
		t.Fatalf("current turn must own the ordinal surface, got %v", ranked[0].Members)
	}

	// Negative: with NO current-turn version, the prior turn's fact still
	// flows (the ferry is not weakened).
	mutEmpty := NewMutableState("xgap cross-turn empty")
	mutEmpty.SetTurnAArtifacts(TurnAArtifacts{
		AcceptedAggregateFacts: []AnswerAggregateFact{xgapWitnessFactEarlier()},
	})
	planEmpty := BuildAnswerSurfacePlan(ir, mutEmpty, nil, nil, nil, nil)
	if planEmpty == nil {
		t.Fatal("BuildAnswerSurfacePlan returned nil for empty current turn")
	}
	found := false
	for _, fact := range planEmpty.StableAggregateFacts {
		if strings.Contains(fact.Label, "根因排序") && len(fact.Members) == 8 {
			found = true
		}
	}
	if !found {
		t.Fatalf("prior-turn fact must survive when the current turn has no version, got %+v", planEmpty.StableAggregateFacts)
	}
}
