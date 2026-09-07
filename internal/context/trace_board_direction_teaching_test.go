package context

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// B1590a constrains only the system's producer -> prompt teaching. It never
// requires particular model prose, selection, diagnosis, or JSON fields.
func b1590aDirectionPrompt(t *testing.T, finite bool) (*types.PromptContext, string) {
	t.Helper()
	ledger := traceBoardTestLedger()
	ledger.Records[0].RichNotes = append(ledger.Records[0].RichNotes, "fix_direction=io_dependency")
	ledger.Records[1].RichNotes = append(ledger.Records[1].RichNotes, "fix_direction=lock_priority")
	bus := &types.BusContext{
		RepoRoot: "/tmp/repo", Mutable: types.NewMutableState("inspect measured contributors"),
		ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: ledger.Records}},
	}
	if finite {
		bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
				Scope:        types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState},
			},
		}}
	}
	before, err := json.Marshal(bus.ToolResults)
	if err != nil {
		t.Fatal(err)
	}
	pc := BuildPromptContext(BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize), &skill.Config{Name: "finalize-answer"})
	after, err := json.Marshal(bus.ToolResults)
	if err != nil || string(before) != string(after) {
		t.Fatal("direction teaching changed the measured observations")
	}
	var text strings.Builder
	for _, message := range ToMessages(pc) {
		text.WriteString(message.Content)
	}
	return pc, text.String()
}

func TestB1590aTraceBoardDirectionTeachingThroughRealPrompt(t *testing.T) {
	pc, prompt := b1590aDirectionPrompt(t, false)
	section := findSectionTitle(pc, SectionTraceRootCauseBoard)
	if section == nil {
		t.Fatal("actual trace prompt lost its measured root-cause board")
	}
	if got := strings.Count(section.Content, types.TraceRepairDirectionValueTeaching); got != 1 {
		t.Fatalf("concentrated board must consume the shared teaching exactly once, got %d", got)
	}
	for _, want := range []string{
		"largest SINGLE ranked contribution, not that direction's total or recoverable ceiling",
		"exact same-direction subtotal is published",
		"covers its listed members only",
		"a direction total has not been established",
		"do not replace the missing total with a maximum or infer additivity from a shared label",
		"a supply-fold/head-room estimate is not an observed saving",
		"a published lower bound or conservative fallback must not become an upper bound or a guaranteed improvement",
		"cross_seat_aggregation_authority=not_provided_by_this_row",
		"never sum rows together without exact typed composition authority",
		"36.757ms (effective attribution)",
		"修向=IO/内核/依赖 (IO / kernel / dependency)",
		"修向=锁与优先级 (lock & priority)",
	} {
		if !strings.Contains(section.Content, want) || !strings.Contains(prompt, want) {
			t.Errorf("producer-to-prompt direction teaching lost %q", want)
		}
	}
	for _, retired := range []string{
		"lane's maximum recoverable amount",
		"LARGEST on-chain seat value — never the seats' sum",
		"repair_lane_fold=max_on_chain_seat_not_sum",
		"cross_seat_aggregation_authority=forbidden",
	} {
		if strings.Contains(prompt, retired) {
			t.Errorf("actual prompt retained contradictory teaching %q", retired)
		}
	}
	finitePC, finitePrompt := b1590aDirectionPrompt(t, true)
	if findSectionTitle(finitePC, SectionTraceRootCauseBoard) != nil || strings.Contains(finitePrompt, "largest SINGLE ranked contribution") {
		t.Fatal("finite fact query must not gain a root-cause board or repair-direction obligation")
	}
}

func TestB1590aTraceBoardTeachingPreservesExactArithmeticBoundary(t *testing.T) {
	pc, _ := b1590aDirectionPrompt(t, false)
	section := findSectionTitle(pc, SectionTraceRootCauseBoard)
	for _, tc := range []struct {
		name     string
		second   types.TraceCausalProjectionNode
		want     types.TraceAnswerDirectionArithmetic
		value    float64
		teaching string
	}{
		{"typed_disjoint", types.TraceCausalProjectionNode{RankBoardTarget: "ui-10", StartTs: 1.020, EndTs: 1.023, EffectiveImpactMS: 3}, types.TraceAnswerDirectionArithmeticSubtotal, 12, "covers its listed members only"},
		{"overlap", types.TraceCausalProjectionNode{RankBoardTarget: "ui-10", StartTs: 1.005, EndTs: 1.023, EffectiveImpactMS: 3}, types.TraceAnswerDirectionArithmeticOverlap, 0, "do not replace the missing total with a maximum"},
		{"missing_interval", types.TraceCausalProjectionNode{RankBoardTarget: "ui-10", EffectiveImpactMS: 3}, types.TraceAnswerDirectionArithmeticNone, 0, "a direction total has not been established"},
		{"different_board", types.TraceCausalProjectionNode{RankBoardTarget: "other-20", StartTs: 1.020, EndTs: 1.023, EffectiveImpactMS: 3}, types.TraceAnswerDirectionArithmeticNone, 0, "infer additivity from a shared label"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			members := []types.TraceCausalProjectionNode{{RankBoardTarget: "ui-10", StartTs: 1, EndTs: 1.009, EffectiveImpactMS: 9}, tc.second}
			// These cases vary the occurrence envelope or target within an
			// otherwise fully identified rank query; absent board identity is
			// separately covered by the arithmetic authority's negative tests.
			for i := range members {
				members[i].RankBoardParamsFingerprint = "teaching-board"
				members[i].RankQueryWindowStartTs, members[i].RankQueryWindowEndTs = 1, 1.1
			}
			got, value := types.TraceAnswerDirectionSectionArithmetic("lock_priority", members, false, true)
			if got != tc.want || value != tc.value {
				t.Fatalf("existing typed arithmetic changed: (%s, %v), want (%s, %v)", got, value, tc.want, tc.value)
			}
			if section == nil || !strings.Contains(section.Content, tc.teaching) {
				t.Fatalf("board teaching contradicts the exact arithmetic boundary %q", tc.teaching)
			}
		})
	}
}
