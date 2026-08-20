package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

func stageRoleAuthorityTestContext(t *testing.T) (*types.BusContext, []stageauthority.StageRow) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx := flowOperationCompletionContext(nil)
	ctx.RepoRoot = repoRoot
	ctx.Mode = types.ModeRead
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	rows := completionSelectedReadModeStageRows(ctx, nil)
	if len(rows) != 4 {
		t.Fatalf("expected four checkout-verified main read stages, got %+v", rows)
	}
	return ctx, rows
}

func stageRoleAuthorityTestFact(rows []stageauthority.StageRow, refs []string) types.AnswerAggregateFact {
	fact := types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "current read workflow",
		Value:       "4",
		Role:        types.AnswerAggregateRoleSupportingCoverage,
		Provenance:  "emit_investigation_complete.aggregate_facts",
		SupportRefs: append([]string(nil), refs...),
	}
	for _, row := range rows {
		fact.Members = append(fact.Members, row.StageIdent)
		fact.MemberNotes = append(fact.MemberNotes, row.Responsibility)
	}
	return fact
}

func TestCompletionReadModeStageRoleAuthority_RejectsHomonymousHelperSupport(t *testing.T) {
	ctx, rows := stageRoleAuthorityTestContext(t)
	wrongRefs := []string{
		"internal/analysis/dataflow/dataflow.go:20",
		"internal/agent/explorer.go:40",
		"internal/agent/extractor.go:60",
		"internal/agent/finalizer.go:80",
	}
	fact := stageRoleAuthorityTestFact(rows, wrongRefs)
	gaps := completionReadModeStageRoleAuthorityGaps(ctx, []types.AnswerAggregateFact{fact}, nil)
	if len(gaps) != 1 || len(gaps[0].Missing) != len(rows) {
		t.Fatalf("every stage row must bind to its exact checkout-verified provider row: %+v", gaps)
	}
	if got := completionStageRoleAuthorityGapSummary(gaps); !strings.Contains(got, "stage=StageAnalyze") ||
		!strings.Contains(got, "actual_support=internal/analysis/dataflow/dataflow.go:20") {
		t.Fatalf("typed gap summary lost exact expected/actual locations: %s", got)
	}
	if completionStageMemberMatchesRow("dataflow.Analyze", rows[0]) {
		t.Fatal("qualified homonymous helper must not collapse to the canonical analyze stage")
	}
}

func TestCompletionReadModeStageRoleAuthority_AcceptsExactProviderRows(t *testing.T) {
	ctx, rows := stageRoleAuthorityTestContext(t)
	refs := make([]string, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, aggregateSupportLocationKey(row.File, row.Line))
	}
	fact := stageRoleAuthorityTestFact(rows, refs)
	if gaps := completionReadModeStageRoleAuthorityGaps(ctx, []types.AnswerAggregateFact{fact}, nil); len(gaps) != 0 {
		t.Fatalf("exact checkout-verified support rows must pass: %+v", gaps)
	}
	for i := range fact.Members {
		fact.Members[i] += " @ " + refs[i]
	}
	if gaps := completionReadModeStageRoleAuthorityGaps(ctx, []types.AnswerAggregateFact{fact}, nil); len(gaps) != 0 {
		t.Fatalf("explicit source-suffixed canonical stage labels must preserve the same exact authority: %+v", gaps)
	}
}

func TestCompletionReadModeStageRoleAuthority_IgnoresPartialOrMixedRoster(t *testing.T) {
	ctx, rows := stageRoleAuthorityTestContext(t)
	partial := stageRoleAuthorityTestFact(rows[:2], []string{"x.go:1", "x.go:2"})
	partial.Value = "2"
	mixed := stageRoleAuthorityTestFact(rows, []string{"x.go:1", "x.go:2", "x.go:3", "x.go:4"})
	mixed.Members[2] = "BusContext"
	for _, fact := range []types.AnswerAggregateFact{partial, mixed} {
		if gaps := completionReadModeStageRoleAuthorityGaps(ctx, []types.AnswerAggregateFact{fact}, nil); len(gaps) != 0 {
			t.Fatalf("non-exclusive roster stays in ordinary evidence contracts: fact=%+v gaps=%+v", fact, gaps)
		}
	}
}

func TestEmitInvestigationComplete_StageRoleAuthorityRoutesAndDropsPersistentConflict(t *testing.T) {
	ctx, rows := stageRoleAuthorityTestContext(t)
	fact := stageRoleAuthorityTestFact(rows, []string{
		"internal/analysis/dataflow/dataflow.go:20",
		"internal/agent/explorer.go:40",
		"internal/agent/extractor.go:60",
		"internal/agent/finalizer.go:80",
	})
	params, err := json.Marshal(map[string]any{
		"reason":          "the current read workflow responsibilities were investigated",
		"confidence":      "high",
		"result_kind":     "resolved",
		"aggregate_facts": []types.AnswerAggregateFact{fact},
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := &EmitInvestigationComplete{}
	for attempt := 1; attempt <= 2; attempt++ {
		res, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if ctx.Mutable.IsInvestigationComplete() || res.Repair == nil || res.Repair.Code != "current_read_stage_role_authority" {
			t.Fatalf("attempt %d must stay on the exact stage-role repair lane: %+v", attempt, res)
		}
		if strings.Contains(res.Summary, "find the concrete producer/callsite/consumer") {
			t.Fatalf("stage-role repair must not redirect the model to a homonymous mechanism search: %s", res.Summary)
		}
		if res.Repair.Metadata["lane"] != string(types.DowngradeLaneStageRoleAuthority) ||
			len(res.Repair.Targets) != 1 || len(res.Repair.Targets[0].Lines) != 4 {
			t.Fatalf("stage-role repair lost its bounded typed target: %+v", res.Repair)
		}
	}
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !ctx.Mutable.IsInvestigationComplete() || !strings.Contains(res.Summary, "was excluded from authoritative aggregate handoff") {
		t.Fatalf("third unchanged conflict should close with an explicit exclusion caveat: %+v", res)
	}
	if got := ctx.Mutable.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("known-invalid stage roster must not survive bounded convergence: %+v", got)
	}
}

func TestCompletionReadModeStageRoleAuthority_TraceBypassesReadWorkflowAuthority(t *testing.T) {
	ctx, rows := stageRoleAuthorityTestContext(t)
	ctx.Mode = types.ModeRead
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	fact := stageRoleAuthorityTestFact(rows, []string{"x.go:1", "x.go:2", "x.go:3", "x.go:4"})
	if gaps := completionReadModeStageRoleAuthorityGaps(ctx, []types.AnswerAggregateFact{fact}, nil); len(gaps) != 0 {
		t.Fatalf("trace analysis must not enter current-read source workflow authority: %+v", gaps)
	}
}
