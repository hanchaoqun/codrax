package types_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTurnAClosureLifecyclePublicForkDoesNotReplaySupersededProse(t *testing.T) {
	// Ordinary prose may resemble an old system wrapper; it must not be
	// classified by a prefix, keyword, or substring scan.
	ordinary := "Previous accepted closure reason (preserved advisory, not a citation): user supplied text"
	parent := types.NewMutableState("q")
	parent.SetTurnAArtifacts(types.TurnAArtifacts{
		InvestigationNotes:    []string{ordinary},
		AcceptedClosureReason: "old unbound interpretation",
		AcceptedResultKind:    "resolved",
	})
	fork := parent.ForkForExploreDispatch()
	current := fork.TurnAArtifacts()
	current.AcceptedClosureReason = "new interpretation"
	fork.SetTurnAArtifacts(*current)
	parent.MergeExploreFork(fork)
	got := parent.TurnAArtifacts()
	if !reflect.DeepEqual(got.InvestigationNotes, []string{ordinary}) {
		t.Fatalf("superseded closure must remain audit history, not ordinary narrative: %+v", got.InvestigationNotes)
	}
	if got.AcceptedClosureReason != current.AcceptedClosureReason {
		t.Fatalf("current closure changed: %+v", got)
	}
	if !reflect.DeepEqual(got.SupersededClosures, []types.InvestigationClosureHistoryEntry{{
		Reason: "old unbound interpretation", ResultKind: "resolved",
	}}) {
		t.Fatalf("old closure lost from audit: %+v", got.SupersededClosures)
	}
}

func TestTurnAClosureLifecyclePublicHistoryPreservesRawTextAndLegacyJSON(t *testing.T) {
	const legacy = `{"InvestigationNotes":["Previous accepted closure reason (preserved advisory, not a citation): legacy text"],"AcceptedClosureReason":"  old\nreason  ","AcceptedResultKind":"resolved"}`
	var prior types.TurnAArtifacts
	if err := json.Unmarshal([]byte(legacy), &prior); err != nil {
		t.Fatal(err)
	}
	if len(prior.SupersededClosures) != 0 {
		t.Fatal("legacy narrative must not be classified or migrated")
	}
	current := prior
	current.AcceptedClosureReason = "new reason"
	current.SupersededClosures = types.MergeInvestigationClosureHistory(&prior, &current)
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip types.TurnAArtifacts
	if err := json.Unmarshal(encoded, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roundtrip.InvestigationNotes, prior.InvestigationNotes) ||
		len(roundtrip.SupersededClosures) != 1 || roundtrip.SupersededClosures[0].Reason != prior.AcceptedClosureReason {
		t.Fatalf("raw audit text or legacy notes changed: %+v", roundtrip)
	}
	empty, err := json.Marshal(types.TurnAArtifacts{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(empty, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["SupersededClosures"]; exists {
		t.Fatal("empty history must not alter the old snapshot JSON shape")
	}
}

func TestTurnAClosureLifecyclePublicHistoryIsolatedAcrossSnapshots(t *testing.T) {
	entry := types.InvestigationClosureHistoryEntry{Reason: "original history", ResultKind: "absence"}
	input := types.TurnAArtifacts{SupersededClosures: []types.InvestigationClosureHistoryEntry{entry}}
	m := types.NewMutableState("q")
	m.SetTurnAArtifacts(input)
	input.SupersededClosures[0].Reason = "caller mutation"
	got := m.TurnAArtifacts()
	got.SupersededClosures[0].Reason = "getter mutation"
	snapshot, _, _, _, _, _, _, _ := m.GroundingContextSnapshot()
	snapshot.SupersededClosures[0].Reason = "grounding snapshot mutation"
	fork := m.ForkForExploreDispatch()
	child := fork.TurnAArtifacts()
	child.SupersededClosures[0].Reason = "fork mutation"
	fork.SetTurnAArtifacts(*child)
	if !reflect.DeepEqual(m.TurnAArtifacts().SupersededClosures, []types.InvestigationClosureHistoryEntry{entry}) {
		t.Fatal("mutable/fork/grounding snapshots shared history backing storage")
	}
	m.ResetTurnAArtifacts()
	if m.TurnAArtifacts() != nil {
		t.Fatal("reset must clear audit history with the run snapshot")
	}
}

func TestTurnAClosureLifecyclePublicHistoryMergesWithoutNarrativeInference(t *testing.T) {
	old := types.InvestigationClosureHistoryEntry{Reason: "older", ResultKind: "absence"}
	prior := types.TurnAArtifacts{
		SupersededClosures:    []types.InvestigationClosureHistoryEntry{old},
		AcceptedClosureReason: "same text",
		AcceptedResultKind:    "resolved",
	}
	for _, tc := range []struct {
		name, reason, kind string
		wantReplacement    bool
	}{
		{"no new closure", "", "", false},
		{"unchanged", "same text", "resolved", false},
		{"kind inherited", "same text", "", false},
		{"replaced kind", "same text", "absence", true},
		{"replaced raw reason", "SAME TEXT", "resolved", true},
		{"replaced reason", "new text", "resolved", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := types.TurnAArtifacts{
				SupersededClosures:    []types.InvestigationClosureHistoryEntry{old},
				AcceptedClosureReason: tc.reason,
				AcceptedResultKind:    tc.kind,
				InvestigationNotes:    []string{"ordinary note quotes same text"},
			}
			want := []types.InvestigationClosureHistoryEntry{old}
			if tc.wantReplacement {
				want = append(want, types.InvestigationClosureHistoryEntry{Reason: prior.AcceptedClosureReason, ResultKind: prior.AcceptedResultKind})
			}
			got := types.MergeInvestigationClosureHistory(&prior, &current)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("history=%+v want %+v", got, want)
			}
			current.SupersededClosures = got
			if again := types.MergeInvestigationClosureHistory(&prior, &current); !reflect.DeepEqual(again, want) {
				t.Fatalf("repeated merge duplicated audit: %+v", again)
			}
			got[0].Reason = "mutated merged slice"
			if prior.SupersededClosures[0] != old {
				t.Fatal("merge aliases prior history")
			}
		})
	}
}

func TestTurnAClosureLifecyclePublicSiblingForksRetainEveryReplacement(t *testing.T) {
	parent := types.NewMutableState("q")
	parent.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedClosureReason: "base closure", AcceptedResultKind: "resolved",
	})
	first, second := parent.ForkForExploreDispatch(), parent.ForkForExploreDispatch()
	for _, child := range []struct {
		state *types.MutableState
		label string
	}{{first, "first"}, {second, "second"}} {
		snapshot := child.state.TurnAArtifacts()
		snapshot.AcceptedClosureReason = child.label + " closure"
		snapshot.InvestigationNotes = []string{child.label + " independent note"}
		child.state.SetTurnAArtifacts(*snapshot)
		parent.MergeExploreFork(child.state)
	}
	got := parent.TurnAArtifacts()
	wantHistory := []types.InvestigationClosureHistoryEntry{
		{Reason: "base closure", ResultKind: "resolved"},
		{Reason: "first closure", ResultKind: "resolved"},
	}
	if !reflect.DeepEqual(got.SupersededClosures, wantHistory) ||
		!reflect.DeepEqual(got.InvestigationNotes, []string{"first independent note", "second independent note"}) ||
		got.AcceptedClosureReason != "second closure" {
		t.Fatalf("sibling merge lost history or ordinary narrative: %+v", got)
	}
}
