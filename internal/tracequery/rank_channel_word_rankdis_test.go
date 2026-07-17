package tracequery

// rank_channel_word_rankdis_test.go — RANKDIS-EXT A1 engine faces
// (§29.104.16/.16.1, 2026-07-16):
//   - RootCauseRankOrdinalChannelWord delegates to the ordinal allocator's
//     own channel single source (rootCauseOrdinalChannel) — the word the
//     rank text row wears is BY CONSTRUCTION the channel the allocator
//     seated the row on, and the background channel (no ordinal) yields no
//     word;
//   - renderStateDrilldownStep wears the scoped drill_rank word so the
//     engine Summary face matches the tool text and JSON faces.
//
// MUTATION self-check: re-implementing the channel decision inside the word
// helper (instead of delegating) desyncs it from assignRootCauseRanksAndTiers
// and reds the allocator-consistency arm below.

import (
	"strings"
	"testing"
)

func TestRankdisA1ChannelWordMatchesOrdinalAllocator(t *testing.T) {
	onChain := RootCauseRankItem{Type: "runnable_wait", ChainRelevance: "on_chain", Causality: "on_wakeup_chain"}
	adjacent := RootCauseRankItem{Type: "priority_inversion_candidate", ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain"}
	background := RootCauseRankItem{Type: "cpu_pressure", ChainRelevance: "background", Causality: "background"}
	chainless := RootCauseRankItem{Type: "long_sleep"} // empty relevance: flat fallback board stays chain

	if word := RootCauseRankOrdinalChannelWord(onChain); word != rootCauseOrdinalChannelChain {
		t.Fatalf("on-chain row channel word = %q, want %q", word, rootCauseOrdinalChannelChain)
	}
	if word := RootCauseRankOrdinalChannelWord(adjacent); word != rootCauseOrdinalChannelAdjacent {
		t.Fatalf("adjacent row channel word = %q, want %q", word, rootCauseOrdinalChannelAdjacent)
	}
	if word := RootCauseRankOrdinalChannelWord(background); word != "" {
		t.Fatalf("the background channel owns no ordinal and must yield no channel word, got %q", word)
	}
	if word := RootCauseRankOrdinalChannelWord(chainless); word != rootCauseOrdinalChannelChain {
		t.Fatalf("a chainless flat-fallback board stays one chain-universe ordinal space, got %q", word)
	}

	// Allocator consistency: for every item the word is exactly the
	// allocator's channel (or empty on the ordinal-less channel).
	for _, item := range []RootCauseRankItem{onChain, adjacent, background, chainless} {
		channel := rootCauseOrdinalChannel(item)
		word := RootCauseRankOrdinalChannelWord(item)
		if channel == rootCauseOrdinalChannelBackground {
			if word != "" {
				t.Fatalf("background-channel item %q must have no word, got %q", item.Type, word)
			}
			continue
		}
		if word != channel {
			t.Fatalf("channel word desynced from the allocator for %q: word=%q channel=%q", item.Type, word, channel)
		}
	}
}

func TestRankdisA1DrilldownEngineSummaryWearsScopedWord(t *testing.T) {
	step := StateDrilldownStep{
		Rank:   3,
		Thread: ThreadRef{Comm: "app", PID: 20},
		State:  "runnable", ImpactMs: 5.0, TotalMs: 8.0,
		Source:           "state_churn",
		RecommendedViews: []string{"scheduler_latency_stats"},
		WindowProportion: 0.25, Significant: true,
		LineStart: 11, LineEnd: 19,
	}
	summary := renderStateDrilldownStep(step)
	if !strings.HasPrefix(summary, "state_drilldown drill_rank=3 ") {
		t.Fatalf("engine drilldown summary must wear the scoped drill_rank word: %q", summary)
	}
	if strings.Contains(summary, "state_drilldown rank=") {
		t.Fatalf("engine drilldown summary must not wear the bare rank= word: %q", summary)
	}
}

// RANKDIS-EXT B6 (§29.104.16.1 M11, 2026-07-16): total= carries TWO
// populations across sources — the in-row total_scope word forks on the
// typed Source field so the two accounts stop wearing one bare column word.
//
// MUTATION self-check: swapping the fork arms reds both positive pins;
// making the token unconditional reds the unknown-source omission arm.
func TestRankdisB6TotalScopeWordForksOnSource(t *testing.T) {
	base := StateDrilldownStep{
		Rank:   1,
		Thread: ThreadRef{Comm: "app", PID: 20},
		State:  "s_sleep", ImpactMs: 21.0, TotalMs: 30.0,
		LineStart: 11, LineEnd: 19,
	}
	single := base
	single.Source = "top_sleep"
	if got := renderStateDrilldownStep(single); !strings.Contains(got, "total=30.000ms total_scope=single_state source=top_sleep") {
		t.Fatalf("top_* rows must wear total_scope=single_state: %q", got)
	}
	churn := base
	churn.Source = "state_churn"
	if got := renderStateDrilldownStep(churn); !strings.Contains(got, "total=30.000ms total_scope=all_states source=state_churn") {
		t.Fatalf("state_churn rows must wear total_scope=all_states: %q", got)
	}
	unknown := base
	unknown.Source = ""
	if got := renderStateDrilldownStep(unknown); strings.Contains(got, "total_scope=") {
		t.Fatalf("an unknown source must omit the scope token, never guess: %q", got)
	}
	for _, source := range []string{"top_sleep", "top_runnable", "top_running", "top_io_wait", "top_d_state"} {
		if got := StateDrilldownTotalScopeWord(source); got != "single_state" {
			t.Fatalf("%s: want single_state, got %q", source, got)
		}
	}
}
