package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public context-to-prompt path with published typed records.
// These fixtures test display ownership, not native scheduler measurement.
func b1660BoardPrompt(t *testing.T, records []types.ObservationRecord, summary string) (string, string) {
	t.Helper()
	bus := &types.BusContext{
		RepoRoot: t.TempDir(), Mutable: types.NewMutableState("inspect measured contributors"),
		ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Summary: summary, Observations: records}},
	}
	before, err := json.Marshal(bus.ToolResults)
	if err != nil {
		t.Fatal(err)
	}
	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	if summary != "" && !strings.Contains(strings.Join(ac.RelevantToolSummaries, "\n"), summary) {
		t.Fatal("independent source summary was changed in the agent context")
	}
	pc := BuildPromptContext(ac, &skill.Config{Name: "finalize-answer"})
	section := findSectionTitle(pc, SectionTraceRootCauseBoard)
	if section == nil || section.Content == "" {
		t.Fatal("public prompt must contain the nonempty seated board")
	}
	var prompt strings.Builder
	for _, message := range ToMessages(pc) {
		prompt.WriteString(message.Content)
	}
	after, err := json.Marshal(bus.ToolResults)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("display changed source records, values, notes, or receipts")
	}
	return section.Content, prompt.String()
}

func TestB1660CumulativeImpactHasItsOwnCaliberThroughPublicPrompt(t *testing.T) {
	for _, state := range []string{"running", "runnable_wait", "sleep_wait", "d_state_or_io_wait"} {
		for _, channel := range []string{"on_chain", "adjacent"} {
			t.Run(state+"/"+channel, func(t *testing.T) {
				first := traceBoardDomainRecord("first", "/capture/one.trace", "worker-42", "query-a", "1..2", "worker-42", "3.125", channel, 1)
				first.Object = state
				first.RichNotes = append(first.RichNotes, "cumulative_impact_ms=19.750", "member_fold_caliber=sum_disjoint", "tgid=40")
				second := traceBoardDomainRecord("second", "/capture/one.trace", "worker-42", "query-b", "1..2", "worker-42", "0.000", channel, 1)
				second.Object = state
				second.RichNotes = append(second.RichNotes, "cumulative_impact_ms=7.001")
				board, prompt := b1660BoardPrompt(t, []types.ObservationRecord{second, first}, "")
				for _, want := range []string{
					"3.125ms (effective attribution) · cumulative impact 19.750ms (not a substitute for state occupancy)",
					"0.000ms (effective attribution) · cumulative impact 7.001ms (not a substitute for state occupancy)",
					"params=`query-a`", "params=`query-b`", "fold=sum_disjoint", "tgid=40", "· " + state + " ·",
					"no single cross-board ranking", "never sum rows together without exact typed composition authority",
				} {
					if !strings.Contains(board, want) || !strings.Contains(prompt, want) {
						t.Errorf("public board lost or mislabeled %q", want)
					}
				}
				if strings.Contains(board, "raw occupancy 19.750ms") || strings.Contains(board, "raw occupancy 7.001ms") {
					t.Error("cumulative impact was mislabeled as state occupancy")
				}
				seat := "root-cause seat"
				if channel == "adjacent" {
					seat = "adjacent seat"
				}
				if strings.Count(board, "### Rank board ") != 2 || strings.Count(board, "#1 "+seat+" — worker-42") != 2 ||
					strings.Index(board, "params=`query-a`") > strings.Index(board, "params=`query-b`") {
					t.Fatal("display change merged query domains or changed their seats/order")
				}
			})
		}
	}
}

func TestB1660SilentTwinsDoNotBorrowIndependentRunningOccupancy(t *testing.T) {
	for _, cumulative := range []string{"", "3.125", "19.750"} {
		t.Run("cumulative="+cumulative, func(t *testing.T) {
			seat := traceBoardDomainRecord("seat", "/capture/one.trace", "worker-42", "query-a", "1..2", "worker-42", "3.125", "on_chain", 1)
			if cumulative != "" {
				seat.RichNotes = append(seat.RichNotes, "cumulative_impact_ms="+cumulative)
			}
			state := types.ObservationRecord{
				ID: "trace_query:state#target_window_states", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Subject: "worker-42", Predicate: "target_window_states", Object: "state_partition", Value: "1000.000", Unit: "ms",
				SourceRef: seat.SourceRef,
				RichNotes: []string{"selected_window=1..2", "running=11.250", "runnable=2.000", "sleep=986.750", "total=1000.000"},
			}
			account, ok := types.TraceCausalProjectionTargetStateAccountFromRecord(state)
			if !ok || account.RunningMS != 11.250 {
				t.Fatal("fixture must supply an independent typed running account")
			}
			const actualStateSummary = "target_window_states worker-42 running=11.250ms (raw occupancy); selected_window=1..2"
			board, _ := b1660BoardPrompt(t, []types.ObservationRecord{seat, state}, actualStateSummary)
			wantTwin := cumulative != "" && cumulative != "3.125"
			if strings.Contains(board, " · cumulative impact ") != wantTwin {
				t.Fatal("missing/same-value cumulative twins must keep their old silent behavior")
			}
			if !strings.Contains(board, "3.125ms (effective attribution)") || strings.Contains(board, "11.250") {
				t.Fatal("board filled a cumulative/state slot from the independent running account")
			}
		})
	}
}

func TestB1660PublicBoardKeepsExistingEightPlusFourBudget(t *testing.T) {
	var records []types.ObservationRecord
	for _, channel := range []string{"on_chain", "adjacent"} {
		for rank := 10; rank > 0; rank-- {
			record := traceBoardDomainRecord(fmt.Sprintf("%s-%d", channel, rank), "/capture/one.trace", "worker-42", "query-a", "1..2", fmt.Sprintf("%s-%d", channel, rank), "1.250", channel, rank)
			record.RichNotes = append(record.RichNotes, "cumulative_impact_ms=8.875")
			records = append(records, record)
		}
	}
	board, _ := b1660BoardPrompt(t, records, "")
	if strings.Count(board, " root-cause seat — ") != 8 || strings.Count(board, " adjacent seat — ") != 4 ||
		strings.Count(board, "cumulative impact 8.875ms (not a substitute for state occupancy)") != 12 {
		t.Fatal("quantity wording must not change the existing 8+4 display budget")
	}
	for _, want := range []string{"+2 more seated rows", "+6 more adjacent rows", "#8 root-cause seat — on_chain-8", "#4 adjacent seat — adjacent-4"} {
		if !strings.Contains(board, want) {
			t.Errorf("board lost existing order/cap disclosure %q", want)
		}
	}
}
