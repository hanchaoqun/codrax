package tracefinding

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCompleteRootCausesFromAnswerTable(t *testing.T) {
	scope := types.TraceQueryWindowScope{Role: types.TraceQueryWindowScopeRequestedPrincipal, QueryWindowStartTs: 10, QueryWindowEndTs: 11}
	candidate := func(id string, rank, line int, thread, token, lane string, ms float64) types.TraceFindingCandidateV1 {
		return types.TraceFindingCandidateV1{PrimaryEligible: true, Decision: types.TraceCauseDecision{
			CandidateID: id, Rank: rank, SubjectName: thread, ArtifactLabel: "target.ftrace",
			Token:           types.TraceCausalTokenSnapshot{Token: token, Lane: lane},
			Magnitude:       &types.TypedMagnitude{Value: ms, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: types.TraceImpactCaliberEffectiveAttribution},
			CausalQualifier: types.TraceCausalQualifierFrameUnproven,
			EvidenceFacts:   &types.TraceCauseEvidenceFacts{WindowScope: &scope, ArtifactLabel: "target.ftrace", LineStart: line, LineEnd: line + 1},
		}}
	}
	first := candidate("first", 1, 10, "Worker-4", "class_verification", "cpu_work", 21.219)
	upload := candidate("upload", 2, 20, "RenderThread-8", "texture_upload", "cpu_work", 10.695)
	io := candidate("io", 3, 30, "Worker-4", "io_wait", "io_blocking", 3.144)
	nearby := candidate("nearby", 4, 40, "Worker-5", "io_wait", "io_blocking", 6)
	nearby.PrimaryEligible = false
	wrongCaliber := candidate("projection", 5, 50, "Worker-6", "io_wait", "io_blocking", 7)
	wrongCaliber.Decision.Magnitude.Caliber = types.TraceImpactCaliberWindowProjection
	unmatched := candidate("unmatched", 6, 60, "Worker-7", "io_wait", "io_blocking", 8)
	contract := &types.TraceFindingContract{RootCauseReportEnabled: true, Candidates: []types.TraceFindingCandidateV1{first, upload, io, nearby, wrongCaliber, unmatched}}
	report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
		RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: "first", Description: "保留原文"}},
	}, contract)
	if err != nil {
		t.Fatal(err)
	}
	table := types.AnswerBlock{
		Kind: types.BlockTable, Title: "链上根因座位（窗口 10.000000..11.000000，有效归因值）",
		Columns: []string{"座位", "线程", "类别", "有效归因"},
		Items: []types.AnswerBlockItem{
			{Cells: []string{"#1", "Worker-4", "类校验", "21.219 ms"}},
			{Cells: []string{"#2", "RenderThread-8", "纹理上传", "10.695 ms"}},
			{Cells: []string{"#3", "Worker-4", "IO 等待", "3.144 ms"}},
			{Cells: []string{"#4", "Worker-5", "IO 等待", "6.000 ms"}},
			{Cells: []string{"#5", "Worker-6", "IO 等待", "7.000 ms"}},
			{Cells: []string{"#6", "Worker-7", "IO 等待", "8.001 ms"}},
		},
	}
	got := CompleteRootCausesFromAnswerTable(report, &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{table}}, contract)
	if len(got.RootCauses) != 3 || len(report.RootCauses) != 1 || !reflect.DeepEqual(got.RootCauses[0], report.RootCauses[0]) {
		t.Fatalf("expected the original selection and two typed additions: %+v", got.RootCauses)
	}
	for i, want := range []struct {
		line string
		ms   float64
	}{{"第 20–21 行", 10.695}, {"第 30–31 行", 3.144}} {
		cause := got.RootCauses[i+1]
		if cause.Rank != i+2 || *cause.ImpactSeconds != want.ms/1000 ||
			cause.ImpactCaliber != types.TraceImpactCaliberEffectiveAttribution ||
			cause.CausalQualifier != types.TraceCausalQualifierFrameUnproven ||
			!strings.Contains(strings.Join(cause.Evidence, "\n"), want.line) {
			t.Fatalf("typed fields or evidence changed: %+v", cause)
		}
	}
	hidden := table
	hidden.Text = "| 已显示 |\n|---|\n| 正文表格 |"
	if got := CompleteRootCausesFromAnswerTable(report, &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{hidden}}, contract); got != report {
		t.Fatal("cells shadowed by the visible Markdown table must not complete the selection")
	}
	empty := &types.TraceRootCauseReportV2{SchemaVersion: 2}
	if got := CompleteRootCausesFromAnswerTable(empty, &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{table}}, contract); got != empty {
		t.Fatal("an explicit empty selection must be preserved")
	}
	table.Title = "链上根因座位（窗口 10.100000..11.000000，有效归因值）"
	if got := CompleteRootCausesFromAnswerTable(report, &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{table}}, contract); got != report {
		t.Fatal("a different report window must not complete the selection")
	}
}
