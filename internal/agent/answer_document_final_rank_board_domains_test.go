package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func finalRankDomainProjection() types.TraceCausalProjection {
	p := rankBoardDomainProjection()
	p.TargetStateAccount = &types.TraceCausalProjectionTargetStateAccount{Subject: "app-100", TotalMS: 100}
	for i := range p.RankedSeats {
		p.RankedSeats[i].FixDirection = "scheduling_supply"
		p.RankedSeats[i].ChainDepth = 2
	}
	return p
}

// These older tests exercise same-board selection. Supply the typed query
// identity they previously omitted, without changing any row or assertion.
func finalRankDomainCompleteFixture(p *types.TraceCausalProjection, target string) {
	if !types.TraceCausalProjectionWindowPresent(p.WindowStartTs, p.WindowEndTs) {
		p.WindowStartTs, p.WindowEndTs = 10, 10.1
	}
	p.WakeupPathQueryWindowStartTs, p.WakeupPathQueryWindowEndTs = p.WindowStartTs, p.WindowEndTs
	for i := range p.RankedSeats {
		n := &p.RankedSeats[i]
		n.RankBoardTarget, n.RankBoardParamsFingerprint = target, "same-board-fixture"
		if !types.TraceCausalProjectionWindowPresent(n.RankQueryWindowStartTs, n.RankQueryWindowEndTs) {
			n.RankQueryWindowStartTs, n.RankQueryWindowEndTs = p.WindowStartTs, p.WindowEndTs
		}
	}
}

func TestB1590cReaderCardsAndDirectionLeadersKeepIndependentQueryBoards(t *testing.T) {
	p := finalRankDomainProjection()
	before, _ := json.Marshal(p)
	leaders := traceFinalFixDirectionLeaders(p, 0)
	if len(leaders) != 2 {
		t.Errorf("same direction on two legitimate query boards needs two local leaders, got %d", len(leaders))
	}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}}
	for _, lang := range []string{"zh-CN", "en"} {
		faces := map[string]string{
			"reader":    renderTraceFinalReaderDecisionCards(set, nil, lang, nil, types.TraceWakeupTargetCPUIntegrity{}, false),
			"compact":   renderTraceFinalCompactAuthorityLedger(set),
			"mechanism": renderTraceFinalLeaderMechanismCeiling(set),
		}
		for name, text := range faces {
			for _, params := range []string{"depth-default", "depth-48"} {
				want := "board_params=`" + params + "`"
				if name == "reader" {
					want = "query settings identity: " + params
					if lang == "zh-CN" {
						want = "查询设置标识：" + params
					}
				}
				if !strings.Contains(text, want) {
					t.Errorf("%s hides the query identity %q:\n%s", name, params, text)
				}
			}
			if !strings.Contains(text, "app-100") || !strings.Contains(text, "/captures/customer.trace") {
				t.Errorf("%s lost target/capture identity:\n%s", name, text)
			}
		}
		if strings.Count(faces["compact"], "leader_effective_attribution=") != 2 || strings.Count(faces["mechanism"], "final_answer_mechanism_scope ") != 2 {
			t.Errorf("local direction leader was hidden or duplicated:\n%s\n%s", faces["compact"], faces["mechanism"])
		}
	}
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("display grouping mutated the projection")
	}
}

func TestB1590cUnknownDirectionBoardPreservesEachAdmittedRow(t *testing.T) {
	p := finalRankDomainProjection()
	p.RankedSeats = p.RankedSeats[:2]
	for i := range p.RankedSeats {
		p.RankedSeats[i].RankBoardParamsFingerprint = ""
	}
	if leaders := traceFinalFixDirectionLeaders(p, 0); len(leaders) != 2 {
		t.Fatalf("unknown identity may not elect a shared direction leader: %+v", leaders)
	}
	got := renderTraceFinalCompactAuthorityLedger(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}})
	for _, n := range p.RankedSeats {
		if !strings.Contains(got, "seat_subject=`"+n.Subject+"`") {
			t.Errorf("unknown identity lost %q:\n%s", n.Subject, got)
		}
	}
	var unknownBoards int
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "compact_direction_query_board;") && strings.Contains(line, "board_identity_complete=`false`") {
			unknownBoards++
		}
	}
	if unknownBoards != 2 || strings.Contains(got, "; direction_subtotal=") {
		t.Fatalf("unknown rows must stay separate without borrowing a subtotal:\n%s", got)
	}
	if strings.Contains(got, "leader_state_kind=") || strings.Contains(got, "leader_measured_state_occupancy=") || !strings.Contains(got, "seat_state_kind=") {
		t.Fatalf("an independent row must not be renamed a shared leader by its state fields:\n%s", got)
	}
}

func TestB1590cFinalBoardPreviewsKeepGlobalBudgets(t *testing.T) {
	p := finalRankDomainProjection()
	seed := p.RankedSeats[0]
	p.RankedSeats = nil
	for i := 0; i < 10; i++ {
		n := seed
		n.EvidenceID, n.Subject = fmt.Sprintf("row-%02d", i), fmt.Sprintf("worker-%02d", i)
		n.RankBoardParamsFingerprint, n.EffectiveImpactMS = fmt.Sprintf("params-%02d", i), float64(20-i)
		p.RankedSeats = append(p.RankedSeats, n)
	}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}}
	reader := renderTraceFinalReaderDecisionCards(set, nil, "en", nil, types.TraceWakeupTargetCPUIntegrity{}, false)
	compact := renderTraceFinalCompactAuthorityLedger(set)
	mechanism := renderTraceFinalLeaderMechanismCeiling(set)
	if strings.Count(reader, "  - Rank ") != 6 || strings.Count(compact, "leader_effective_attribution=") != 6 || strings.Count(mechanism, "final_answer_mechanism_scope ") != 3 {
		t.Fatalf("query grouping multiplied or shrank an existing preview budget:\n%s\n%s\n%s", reader, compact, mechanism)
	}
	if !strings.Contains(reader, "omitting 4 rows") || !strings.Contains(reader, "4 groups are not expanded here") ||
		!strings.Contains(compact, "omitted_rows=4") || !strings.Contains(compact, "omitted_board_groups=4") {
		t.Errorf("global preview omission must be disclosed on both faces:\n%s\n%s", reader, compact)
	}
	if !strings.Contains(mechanism, "omitted_by_leader_preview=4") || !strings.Contains(mechanism, "omitted_by_mechanism_preview=3") {
		t.Errorf("mechanism preview must disclose both unchanged cap stages:\n%s", mechanism)
	}
}

func TestB1590cCompactKeepsOriginalExactSubtotalWithinItsBoard(t *testing.T) {
	p := rankDonorRelationProjection()
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}}
	before, _ := json.Marshal(p)
	got := renderTraceFinalCompactAuthorityLedger(set)
	refs := types.TraceAnswerRelationMemberRef(p.RankedSeats[2]) + "," + types.TraceAnswerRelationMemberRef(p.RankedSeats[3])
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "leader_rank=#3;") {
			for _, want := range []string{"leader_effective_attribution=7.405ms", "direction_subtotal=12.115ms", "subtotal_member_count=2", "subtotal_member_refs=`" + refs + "`"} {
				if !strings.Contains(line, want) {
					t.Fatalf("display must consume the original exact section, not recompute all four direction members; missing %q:\n%s", want, line)
				}
			}
			after, _ := json.Marshal(p)
			if string(before) != string(after) {
				t.Fatal("section display mutated the original projection")
			}
			return
		}
	}
	t.Fatalf("same-board original subtotal leader was hidden:\n%s", got)
}

func TestB1590cFinalizerActualEntryCarriesBothReaderAndCompactBoardScopes(t *testing.T) {
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
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "root_cause_rank"}, Observations: append(rows, state),
	}}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, params := range []string{"depth-default", "depth-48"} {
		readerZH := "查询设置标识：" + params
		readerEN := "query settings identity: " + params
		if !strings.Contains(prompt, readerZH) && !strings.Contains(prompt, readerEN) {
			t.Fatalf("real finalizer entry lost the reader card's query scope %q", params)
		}
		if !strings.Contains(prompt, "board_params=`"+params+"`") {
			t.Fatalf("real finalizer entry lost compact board scope %q", params)
		}
	}
	if strings.Count(prompt, "leader_effective_attribution=7.405ms") < 2 {
		t.Fatal("actual finalizer must retain each distinct board's local direction leader")
	}
}
