package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func rankPopulationCoverageFixture(count int) types.TraceCausalProjection {
	p := types.TraceCausalProjection{
		ArtifactLabel: "customer.trace", WindowStartTs: 10, WindowEndTs: 10.1,
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{
			Subject: "app-100", WindowStartTs: 10, WindowEndTs: 10.1, TotalMS: 100,
		},
	}
	for i := 1; i <= count; i++ {
		p.RankedSeats = append(p.RankedSeats, types.TraceCausalProjectionNode{
			EvidenceID: fmt.Sprintf("rank-%d", i), Subject: fmt.Sprintf("worker-%d", i),
			Object: "runnable", StateKind: "runnable", Rank: i,
			EffectiveImpactMS: 20 - float64(i)/2, ChainRelevance: "on_chain",
			QueryWindowStartTs: 10, QueryWindowEndTs: 10.1,
			// This census fixture represents one known query board. Missing
			// identity is covered separately and must not mint a shared board.
			RankBoardTarget: "app-100", RankBoardParamsFingerprint: "same-query",
		})
	}
	return p
}

func TestPrincipalRankPopulationReportsFullCensusBeforeDisplayLimit(t *testing.T) {
	for _, count := range []int{1, 8, 14} {
		for _, lang := range []string{"zh-CN", "en"} {
			t.Run(fmt.Sprintf("%d/%s", count, lang), func(t *testing.T) {
				p := rankPopulationCoverageFixture(count)
				// The existing selector owns identity, eligibility and ordering.
				// Duplicate publication must not inflate the census.
				p.RankedSeats = append(p.RankedSeats, p.RankedSeats[0])
				before, _ := json.Marshal(p)
				got := renderTraceFinalPrincipalRankPopulation(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}}, lang)
				emitted := min(count, 8)
				want := fmt.Sprintf("ranked_row_count=`%d`; emitted_row_count=`%d`; ranked_rows_complete=`%t`", count, emitted, count == emitted)
				if !strings.Contains(got, want) || strings.Count(got, "  - reader_rank=") != emitted {
					t.Fatalf("bounded display was mistaken for the full population: want %q\n%s", want, got)
				}
				for _, forbidden := range []string{"every_other_row=", "only the rows below may receive", "allowed_visible_ordinals="} {
					if strings.Contains(got, forbidden) {
						t.Errorf("display limit still controls eligibility: %q", forbidden)
					}
				}
				if !strings.Contains(got, "not displayed here does not change a row's eligibility or published rank") {
					t.Fatal("display-only limit was not disclosed")
				}
				after, _ := json.Marshal(p)
				if string(before) != string(after) {
					t.Fatal("rendering changed the model-independent rank population")
				}
			})
		}
	}
}

func TestPrincipalRankPopulationCensusPreservesWindowAndLaneExclusions(t *testing.T) {
	p := rankPopulationCoverageFixture(9)
	for _, lane := range []string{"adjacent", "background"} {
		n := p.RankedSeats[0]
		n.EvidenceID, n.Subject, n.ChainRelevance = lane, lane, lane
		p.RankedSeats = append(p.RankedSeats, n)
	}
	other := p.RankedSeats[0]
	other.EvidenceID, other.Subject, other.QueryWindowEndTs = "other-window", "outside", 10.2
	p.RankedSeats = append(p.RankedSeats, other)
	got := renderTraceFinalPrincipalRankPopulation(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}}, "en")
	if !strings.Contains(got, "ranked_row_count=`9`; emitted_row_count=`8`; ranked_rows_complete=`false`") ||
		!strings.Contains(got, "unranked_context_row subject=`outside`") ||
		!strings.Contains(got, "selected_window_ordinal_permission=`forbidden`") {
		t.Fatalf("complete census widened exact eligibility or lost the explicit different-window caveat:\n%s", got)
	}
	if strings.Contains(got, "subject=`adjacent`") || strings.Contains(got, "subject=`background`") {
		t.Fatalf("context rows entered the selected-window ranked population:\n%s", got)
	}
	if got := renderTraceFinalPrincipalRankPopulation(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{rankPopulationCoverageFixture(0)}}, "en"); got != "" {
		t.Fatalf("empty population must not invent a ranked roster: %s", got)
	}
}

func TestFinalizerRankPopulationCensusDoesNotDemoteUnlistedTypedCandidates(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(false)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 200, Thread: "worker-200", Source: "user_explicit"}}
	var rows []types.ObservationRecord
	for i := 1; i <= 14; i++ {
		row := traceStateOccupancyTestRecord("runnable", "runnable=7.405", "fix_direction=scheduling_supply")
		row.ID, row.ClaimKey, row.Subject = fmt.Sprintf("rank-%d", i), fmt.Sprintf("root_cause_primary:worker-%d", i), fmt.Sprintf("worker-%d", i)
		row.RichNotes[0] = fmt.Sprintf("rank=%d", i)
		row.RichNotes = append(row.RichNotes, types.TraceNoteKeyRankBoardTarget+"=worker-200", types.TraceNoteKeyRankBoardParams+"=same-query")
		rows = append(rows, row)
	}
	state := rows[0]
	state.ID, state.ClaimKey, state.Subject = "target-state", "target_window_states:worker-200", "worker-200"
	state.Predicate, state.Object, state.Value = "target_window_states", "state_partition", "100.000"
	state.RichNotes = []string{
		types.TraceNoteKeySelectedWindow + "=10.000000..10.100000",
		types.TraceNoteKeyRunning + "=100.000", types.TraceNoteKeyRunnable + "=0.000",
		types.TraceNoteKeySleep + "=0.000", types.TraceNoteKeyDState + "=0.000",
		types.TraceNoteKeyIOWait + "=0.000", types.TraceNoteKeyTotal + "=100.000",
	}
	rows = append(rows, state)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "root_cause_rank"}, Observations: rows,
	}}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "ranked_row_count=`14`; emitted_row_count=`8`; ranked_rows_complete=`false`") {
		t.Fatalf("actual finalizer prompt lost the untruncated population:\n%s", prompt)
	}
	if strings.Contains(prompt, "every_other_row=`unranked_context_or_symptom`") || strings.Contains(prompt, "only the rows below may receive these ordinals") {
		t.Fatal("production prompt demoted a valid row because its summary was not displayed")
	}
}
