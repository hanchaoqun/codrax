package tool

// XGAP-FIX ① completion-side pins (§29.104.8, witness
// 20260715-202022.323-89609): the monotonic stable-fact carry-forward in
// effectiveCompletionAggregateFacts must not resurrect the earlier version
// of an ordinal-seated ranking fact next to the later-accepted one.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func xgapToolWitnessFactEarlier() types.AnswerAggregateFact {
	return types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
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

func xgapToolWitnessFactLater() types.AnswerAggregateFact {
	return types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "根因排序",
		Value: "3",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Unit:  "席位数",
		Members: []string{
			"#1 .ugc.aweme.lite-17267 · running · 51.735ms · primary",
			"#2 RenderThread-17597 · running · 4.958ms · secondary",
			"#3 .ugc.aweme.lite-17267 · runnable_wait · 3.437ms · tertiary",
		},
	}
}

func countXgapRankingFacts(facts []types.AnswerAggregateFact) (int, *types.AnswerAggregateFact) {
	n := 0
	var last *types.AnswerAggregateFact
	for i := range facts {
		if facts[i].Kind == types.AnswerAggregateMemberSet && strings.Contains(facts[i].Label, "根因排序") {
			n++
			last = &facts[i]
		}
	}
	return n, last
}

func TestEffectiveCompletionAggregateFacts_OrdinalSupersedeWitnessReplay(t *testing.T) {
	mu := types.NewMutableState("xgap completion replay")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{xgapToolWitnessFactEarlier()})
	mu.SetInvestigationComplete("earlier emit accepted")
	ctx := &types.BusContext{Mutable: mu}

	effective := effectiveCompletionAggregateFacts(ctx, []types.AnswerAggregateFact{xgapToolWitnessFactLater()})
	n, fact := countXgapRankingFacts(effective)
	if n != 1 {
		t.Fatalf("witness pathology: %d 根因排序 fact(s) in effective completion facts, want 1: %+v", n, effective)
	}
	if len(fact.Members) != 3 || !strings.Contains(fact.Members[0], "51.735") {
		t.Fatalf("later-accepted emit must own the ordinal surface, got %v", fact.Members)
	}
	for _, member := range fact.Members {
		if strings.Contains(member, "143.499") {
			t.Fatalf("superseded seat value resurrected: %q", member)
		}
	}
}

func TestEffectiveCompletionAggregateFacts_NonOrdinalMonotonicCarryStays(t *testing.T) {
	// Negative lane: the monotonic carry-forward for NON-ordinal member
	// sets (a later emit that grows/re-states a plain member list) keeps
	// its pre-existing union behavior.
	stable := types.AnswerAggregateFact{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "链路节点",
		Value:   "2",
		Members: []string{"RenderThread-17597 (depth=0)", ".ugc.aweme.lite-17267 (depth=1)"},
	}
	current := types.AnswerAggregateFact{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "链路节点",
		Value:   "3",
		Members: []string{"RenderThread-17597 (depth=0)", ".ugc.aweme.lite-17267 (depth=1)", "keva-1-17437 (depth=2)"},
	}
	mu := types.NewMutableState("non-ordinal carry")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{stable})
	mu.SetInvestigationComplete("earlier emit accepted")
	ctx := &types.BusContext{Mutable: mu}

	effective := effectiveCompletionAggregateFacts(ctx, []types.AnswerAggregateFact{current})
	var got *types.AnswerAggregateFact
	for i := range effective {
		if effective[i].Label == "链路节点" {
			if got != nil {
				// The union/superset arbiter may keep them folded — one
				// fact is the expected shape; two would mean the merge
				// regressed.
				t.Fatalf("non-ordinal same-label facts must fold, got extra %+v", effective[i])
			}
			got = &effective[i]
		}
	}
	if got == nil || len(got.Members) != 3 {
		t.Fatalf("non-ordinal monotonic union must keep the superset, got %+v", got)
	}
}

func TestPreEmitObligations_CrossTargetBoardsBothPresent(t *testing.T) {
	// 修补轮 件A pin ② (consumption side, 2026-07-16): a dual-target run's
	// two legal 「根因排序」 boards (fork1 target logd.writer-2955, fork2
	// target com.baidu.tieba-9163 — same engine chip label, both #1..#3)
	// must BOTH survive MergeExploreFork and both stay principal member-set
	// obligations on the preEmitStableAggregateFacts consumption face. The
	// two-arm supersede key (label + ordinal intersection) used to delete
	// the 2955 board wholesale here; the shared-seat SUBJECT arm now fails
	// open on the cross-target conflict.
	boardA := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "根因排序",
		Value: "3",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"#1: logd.writer-2955 running 12.001ms (primary)",
			"#2: logd.reader.per-3001 runnable 5.002ms (secondary)",
			"#3: kworker/u16:5-771 io_wait 2.003ms",
		},
	}
	boardB := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "根因排序",
		Value: "3",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Unit:  "席位数",
		Members: []string{
			"#1 com.baidu.tieba-9163 · running · 30.500ms · primary",
			"#2 RenderThread-9188 · runnable · 8.100ms · secondary",
			"#3 binder:9163_2-9201 · io_wait · 3.300ms · tertiary",
		},
	}

	parent := types.NewMutableState("dual-target obligations")
	fork1 := parent.ForkForExploreDispatch()
	fork1.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{boardA})
	fork1.SetInvestigationComplete("target 2955 board accepted")
	fork1.RetainInvestigationAggregateFacts()
	parent.MergeExploreFork(fork1)

	fork2 := parent.ForkForExploreDispatch()
	fork2.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{boardB})
	fork2.SetInvestigationComplete("target 9163 board accepted")
	fork2.RetainInvestigationAggregateFacts()
	parent.MergeExploreFork(fork2)

	ctx := &types.BusContext{Mutable: parent}
	facts := preEmitStableAggregateFacts(ctx)
	refs := preEmitPrincipalAggregateMemberSetFactRefs(ctx, facts)
	var boards []types.AnswerAggregateFact
	for _, ref := range refs {
		if strings.Contains(ref.Fact.Label, "根因排序") {
			boards = append(boards, ref.Fact)
		}
	}
	if len(boards) != 2 {
		t.Fatalf("both targets' boards must stay principal obligations, got %d ref(s): %+v", len(boards), boards)
	}
	surface := ""
	for _, board := range boards {
		surface += strings.Join(board.Members, "\n") + "\n"
	}
	for _, subject := range []string{"logd.writer-2955", "com.baidu.tieba-9163"} {
		if !strings.Contains(surface, subject) {
			t.Fatalf("target %s obligation vanished from the consumption face:\n%s", subject, surface)
		}
	}
}

func TestPreEmitMemberSetMissingFingerprint_ProgressSensitive(t *testing.T) {
	roster := []preEmitMemberSetRosterEntry{
		{label: "根因排序", member: "#1 seat-a", present: false},
		{label: "根因排序", member: "#3 seat-b", present: false},
		{label: "根因排序", member: "#2 seat-c", present: true},
	}
	fp1 := preEmitMemberSetMissingFingerprint(roster)
	if fp1 == "" {
		t.Fatal("missing entries must produce a fingerprint")
	}
	// Order-insensitive over the missing set.
	swapped := []preEmitMemberSetRosterEntry{roster[1], roster[0], roster[2]}
	if got := preEmitMemberSetMissingFingerprint(swapped); got != fp1 {
		t.Fatalf("fingerprint must be order-insensitive: %s vs %s", got, fp1)
	}
	// Present-flag flips on OTHER members do not change the cause identity.
	presentFlipped := []preEmitMemberSetRosterEntry{roster[0], roster[1], {label: "根因排序", member: "#2 seat-c", present: true}}
	if got := preEmitMemberSetMissingFingerprint(presentFlipped); got != fp1 {
		t.Fatalf("present rows are not part of the cause identity: %s vs %s", got, fp1)
	}
	// Progress (one fewer missing member) changes the fingerprint.
	progressed := []preEmitMemberSetRosterEntry{
		{label: "根因排序", member: "#1 seat-a", present: true},
		{label: "根因排序", member: "#3 seat-b", present: false},
	}
	if got := preEmitMemberSetMissingFingerprint(progressed); got == fp1 {
		t.Fatal("progress must change the fingerprint (the breaker must not punish progress)")
	}
	// All present → no fingerprint.
	allPresent := []preEmitMemberSetRosterEntry{{label: "l", member: "m", present: true}}
	if got := preEmitMemberSetMissingFingerprint(allPresent); got != "" {
		t.Fatalf("no missing members → no fingerprint, got %s", got)
	}
}

func TestEmitFixHintsRepair_FerriesMemberSetFingerprint(t *testing.T) {
	repair := emitFixHintsRepair([]emitFixHint{
		{Field: "blocks[]", ExpectedShape: "unrelated hint"},
		{Field: "blocks[].items[]", ExpectedShape: "member roster", SameCauseFingerprint: "deadbeef01020304"},
	})
	if repair == nil {
		t.Fatal("repair must be built")
	}
	if got := repair.Metadata[types.ToolRepairMetaMemberSetMissingFingerprint]; got != "deadbeef01020304" {
		t.Fatalf("fingerprint must ride the repair metadata, got %q", got)
	}
	// Negative: no fingerprint hint → no metadata key.
	repair = emitFixHintsRepair([]emitFixHint{{Field: "blocks[]", ExpectedShape: "unrelated"}})
	if _, ok := repair.Metadata[types.ToolRepairMetaMemberSetMissingFingerprint]; ok {
		t.Fatal("fingerprint key must be absent when no hint carries one")
	}
}
