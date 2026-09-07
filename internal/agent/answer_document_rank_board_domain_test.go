package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func rankBoardDomainProjection() types.TraceCausalProjection {
	p := rankPopulationCoverageFixture(0)
	p.ArtifactPath = "/captures/customer.trace"
	for _, params := range []string{"depth-default", "depth-48"} {
		for rank := 1; rank <= 2; rank++ {
			p.RankedSeats = append(p.RankedSeats, types.TraceCausalProjectionNode{
				EvidenceID: fmt.Sprintf("%s-%d", params, rank), Subject: fmt.Sprintf("%s-worker-%d", params, rank),
				Object: "runnable", TypeToken: "runnable_wait", StateKind: "runnable", Rank: rank,
				EffectiveImpactMS: 9 - float64(rank), ChainRelevance: "on_chain",
				RankBoardTarget: "app-100", RankBoardParamsFingerprint: params,
				RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1,
			})
		}
	}
	return p
}

func TestB1590bFinalRankPopulationKeepsQueryDomains(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en"} {
		p := rankBoardDomainProjection()
		before, _ := json.Marshal(p)
		got := renderTraceFinalPrincipalRankPopulation(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}}, lang)
		if strings.Count(got, "- selected_window_reader_rank_roster ") != 2 {
			t.Fatalf("distinct query settings were presented as one ordinal space:\n%s", got)
		}
		for _, params := range []string{"depth-default", "depth-48"} {
			if !strings.Contains(got, "board_params=`"+params+"`") {
				t.Errorf("missing query-domain settings %q:\n%s", params, got)
			}
		}
		if strings.Count(got, "ranked_row_count=`2`; emitted_row_count=`2`; ranked_rows_complete=`true`; displayed_ordinals=`#1,#2`") != 2 ||
			!strings.Contains(got, "board_target=`app-100`") || !strings.Contains(got, "artifact_path=`/captures/customer.trace`") {
			t.Fatalf("per-board rank census or exact artifact/target was lost:\n%s", got)
		}
		if !strings.Contains(got, "Ranks are local to this query board") {
			t.Fatalf("the prompt still permits cross-board ordinal comparison:\n%s", got)
		}
		after, _ := json.Marshal(p)
		if string(before) != string(after) {
			t.Fatal("prompt grouping changed the evidence or query results")
		}
		for i, j := 0, len(p.RankedSeats)-1; i < j; i, j = i+1, j-1 {
			p.RankedSeats[i], p.RankedSeats[j] = p.RankedSeats[j], p.RankedSeats[i]
		}
		if reversed := renderTraceFinalPrincipalRankPopulation(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}}, lang); reversed != got {
			t.Fatalf("input publication order selected a different board:\n%s\n%s", got, reversed)
		}
	}
}

func TestB1590bFinalRankUnknownDomainDoesNotMintSharedBoard(t *testing.T) {
	p := rankBoardDomainProjection()
	p.RankedSeats = p.RankedSeats[:2]
	for i := range p.RankedSeats {
		p.RankedSeats[i].RankBoardParamsFingerprint = ""
	}
	got := renderTraceFinalPrincipalRankPopulation(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}}, "en")
	if strings.Count(got, "- selected_window_reader_rank_roster ") != 2 ||
		strings.Count(got, "board_identity_complete=`false`") != 2 ||
		strings.Count(got, "board_params=`not_provided`") != 2 ||
		!strings.Contains(got, "ranked_rows_complete describes display coverage of the admitted rows only") {
		t.Fatalf("missing query settings were guessed into a shared ordinal board:\n%s", got)
	}
	if strings.Count(got, "  - reader_rank=") != 2 {
		t.Fatal("unknown domain erased previously admitted rows")
	}
}

func TestB1590bFinalRankBudgetIsGlobalAcrossBoards(t *testing.T) {
	p := rankBoardDomainProjection()
	first := p.RankedSeats[0]
	p.RankedSeats = p.RankedSeats[2:]
	for rank := 1; rank <= 12; rank++ {
		n := first
		n.Rank, n.Subject, n.EvidenceID = rank, fmt.Sprintf("large-%d", rank), fmt.Sprintf("large-%d", rank)
		p.RankedSeats = append(p.RankedSeats, n)
	}
	got := renderTraceFinalPrincipalRankPopulation(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}}, "en")
	if strings.Count(got, "  - reader_rank=") != 8 || strings.Count(got, "- selected_window_reader_rank_roster ") != 2 ||
		!strings.Contains(got, "selected_window_population_rows=`14`; displayed_rows=`8`; omitted_rows=`6`") {
		t.Fatalf("per-board display multiplied the old budget or hid its full census:\n%s", got)
	}
	if !strings.Contains(got, "subject=`depth-48-worker-1`") || !strings.Contains(got, "subject=`large-1`") {
		t.Fatalf("a large board displaced another board's first ranked observation:\n%s", got)
	}
}

func TestB1590bFinalRankSameBasenameCapturesRemainDistinct(t *testing.T) {
	a := rankBoardDomainProjection()
	a.RankedSeats = a.RankedSeats[:1]
	b := a
	b.ArtifactPath = "/other/customer.trace"
	got := renderTraceFinalPrincipalRankPopulation(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{a, b}}, "en")
	for _, path := range []string{a.ArtifactPath, b.ArtifactPath} {
		if !strings.Contains(got, "artifact_path=`"+path+"`") {
			t.Fatalf("same basename hid distinct capture %q:\n%s", path, got)
		}
	}
}

func TestB1590bFinalizerRealEntryShowsQuerySettings(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(false)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 200, Thread: "worker-200", Source: "user_explicit"}}
	var rows []types.ObservationRecord
	for _, params := range []string{"depth-default", "depth-48"} {
		row := traceStateOccupancyTestRecord("runnable", "runnable=7.405", "fix_direction=scheduling_supply")
		row.ID, row.ClaimKey, row.Subject = params, "root_cause_primary:"+params, params+"-worker"
		row.RichNotes = append(row.RichNotes, types.TraceNoteKeyRankBoardTarget+"=worker-200", types.TraceNoteKeyRankBoardParams+"="+params)
		rows = append(rows, row)
	}
	state := rows[0]
	state.ID, state.ClaimKey, state.Subject = "target-state", "target_window_states:worker-200", "worker-200"
	state.Predicate, state.Object, state.Value = "target_window_states", "state_partition", "100.000"
	state.RichNotes = []string{types.TraceNoteKeySelectedWindow + "=10.000000..10.100000", types.TraceNoteKeyRunning + "=100.000", types.TraceNoteKeyTotal + "=100.000"}
	rows = append(rows, state)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "root_cause_rank"}, Observations: rows,
	}}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, params := range []string{"depth-default", "depth-48"} {
		if !strings.Contains(prompt, "board_params=`"+params+"`") {
			t.Fatalf("actual finalizer dropped query settings %q", params)
		}
	}
}

func TestB1590bReaderRankAuthorityDisclosesItsAlreadySeparateBoards(t *testing.T) {
	var rows []types.ObservationRecord
	for _, params := range []string{"depth-default", "depth-48"} {
		row := traceStateOccupancyTestRecord("runnable", "runnable=7.405", "fix_direction=scheduling_supply")
		row.ID, row.ClaimKey, row.Subject = params, "root_cause_primary:"+params, params+"-worker"
		row.RichNotes = append(row.RichNotes, types.TraceNoteKeyRankBoardTarget+"=worker-200", types.TraceNoteKeyRankBoardParams+"="+params)
		rows = append(rows, row)
	}
	ledger := types.ObservationLedger{Records: rows}
	before, _ := json.Marshal(ledger)
	for _, lang := range []string{"zh-CN", "en"} {
		got := renderAnswerDocTraceRankAuthority(ledger, lang)
		for _, params := range []string{"depth-default", "depth-48"} {
			if !strings.Contains(got, params) {
				t.Fatalf("reader-ready already-grouped roster hid its distinct settings %q:\n%s", params, got)
			}
		}
		for _, retired := range []string{"Only a total explicitly published within one merged evidence row", "只有证据在一条合并行内明确发布的合计"} {
			if strings.Contains(got, retired) {
				t.Fatalf("reader-ready roster contradicts separately authorized typed subtotals: %q", retired)
			}
		}
		if lang == "en" && !strings.Contains(got, "A separately published exact subtotal covers only its named members and caliber") {
			t.Fatal("the roster must neither waive exact composition evidence nor blanket-forbid its use")
		}
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("reader-ready grouping changed its ledger")
	}
}

func TestB1590bReaderUnknownBoardDoesNotClaimCompleteSharedRoster(t *testing.T) {
	p := rankBoardDomainProjection()
	p.RankedSeats = p.RankedSeats[:2]
	for i := range p.RankedSeats {
		p.RankedSeats[i].RankBoardParamsFingerprint = ""
	}
	got := traceReaderRankDisplayAuthorities(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}})
	if len(got) != 2 {
		t.Fatalf("unknown board should preserve independent admitted rows, got %d", len(got))
	}
	for _, group := range got {
		if group.Complete || group.identity.Complete || len(group.Seats) != 1 {
			t.Fatalf("unknown board acquired a complete shared roster: %+v", group)
		}
	}
}
