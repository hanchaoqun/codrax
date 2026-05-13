package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEmitInvestigationComplete_PreCompleteCheck_PendingReadsBlocks
// is the CGEC E1 regression. When the closure has queued a
// PendingRead the tool MUST return a downgrade message AND must NOT
// flip investigationComplete.
func TestEmitInvestigationComplete_PreCompleteCheck_PendingReadsBlocks(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{
		File:      "internal/orchestrator/topology.go",
		Rationale: "chain X anchors here but file unread",
		Origin:    "chain_promotion",
	})
	bus := &types.BusContext{Mutable: mut, RepoRoot: t.TempDir()}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "looks complete",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("expected DOWNGRADED message, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/orchestrator/topology.go") {
		t.Errorf("expected pending file in message, got: %s", res.Summary)
	}
	// Critical: the flag MUST stay false so the explorer continues.
	if mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete must remain false on downgrade")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_NoPendingReads_AllowsCompletion:
// when the closure has nothing pending and AnalysisIR is nil, the
// tool proceeds to set the flag.
func TestEmitInvestigationComplete_PreCompleteCheck_NoPendingReads_Allows(t *testing.T) {
	mut := types.NewMutableState("test")
	bus := &types.BusContext{Mutable: mut, RepoRoot: t.TempDir()}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "looks complete",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("unexpected downgrade when no pending reads: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete should be set when no blockers")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_FieldValueCountRejectsUnreadAggregateLiteral(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "internal/agent/analyzer.go", `
package agent

func build() {
	out.AnswerContract.CitationReq.Required = false
}
`)
	writeTestFile(t, repoRoot, "internal/orchestrator/orchestrator.go", `
package orchestrator

func fallback() any {
	return AnswerContract{
		CitationReq: types.CitationReq{Required: false},
	}
}
`)
	writeTestFile(t, repoRoot, "internal/orchestrator/orchestrator_test.go", `
package orchestrator

func testOnly() {
	_ = types.CitationReq{Required: false}
}
`)
	writeTestFile(t, repoRoot, "docs/design.md", `
CitationReq.Required = false
`)

	mut := types.NewMutableState("本仓库里，把 CitationReq.Required 设置为 false 的生产代码位点一共有几处？")
	mut.SetRepoRoot(repoRoot)
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/agent/analyzer.go": true})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/agent/analyzer.go",
		LineStart:       5,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: repoRoot,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "本仓库里，把 CitationReq.Required 设置为 false 的生产代码位点一共有几处？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"CitationReq.Required"},
					Keywords: []string{"false"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "direct assignment count is covered",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "field/value count coverage is incomplete") {
		t.Fatalf("expected field/value downgrade, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/orchestrator/orchestrator.go:5") {
		t.Fatalf("downgrade should name unread aggregate literal, got: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "orchestrator_test.go") || strings.Contains(res.Summary, "docs/design.md") {
		t.Fatalf("downgrade must filter tests/docs from production candidates, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open after field/value downgrade")
	}
	repairs := closure.PendingRepairs()
	if len(repairs) == 0 || repairs[len(repairs)-1].Kind != types.RepairExpandSearch {
		t.Fatalf("expected expand-search repair, got %+v", repairs)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ChangeImpactRequiresStructuredTargetHandoff(t *testing.T) {
	mut := types.NewMutableState("Which production files need changes if CitationReq.Required changes?")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: "[internal/agent/analyzer.go: showing lines 1914-1916 of 2200 total]\n" +
			" 1914│ \tif isMeasurementScalar || isHistoryLookup || rm.HasExternalOnlyRuntimeArtifact() {\n" +
			" 1915│ \t\tout.AnswerContract.CitationReq.Required = false\n" +
			" 1916│ \t}\n",
	})
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "understructured",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1915,
		LineEnd:         1915,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "Required",
		Condition:       "isMeasurementScalar || isHistoryLookup || rm.HasExternalOnlyRuntimeArtifact()",
		Summary:         "production assignment site sets CitationReq.Required to false",
		Producer:        EmitEvidenceProducer,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsRelationalLookup:    true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"CitationReq.Required"},
				},
				ChangeImpactProfile: &types.ChangeImpactProfile{
					IsChangeImpact:  true,
					Target:          "CitationReq.Required",
					RequestedOutput: types.ImpactOutputFiles,
					Scope:           types.ImpactScopeProduction,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all affected files were inspected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "change-impact target handoff is under-structured") {
		t.Fatalf("expected change-impact handoff downgrade, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/agent/analyzer.go:1915") {
		t.Fatalf("downgrade should name the under-structured line, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open until the target is carried in structured evidence fields")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ChangeImpactFilesRequireAggregateMemberSet(t *testing.T) {
	mut := types.NewMutableState("Which production files need changes if CitationReq.Required changes?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "structured-target",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1915,
		LineEnd:         1915,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		Subject:         "CitationReq.Required",
		Summary:         "production assignment site sets CitationReq.Required to false",
		Producer:        EmitEvidenceProducer,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
			AnalyzerHints: types.AnalyzerHints{
				Kind:     string(types.ReqEnumeration),
				Entities: []string{"CitationReq.Required"},
			},
			ChangeImpactProfile: &types.ChangeImpactProfile{
				IsChangeImpact:  true,
				Target:          "CitationReq.Required",
				RequestedOutput: types.ImpactOutputFiles,
				Scope:           types.ImpactScopeProduction,
			},
		}},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all affected files were inspected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "change-impact principal member_set handoff is missing") {
		t.Fatalf("expected change-impact member_set downgrade, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open until affected files are emitted through aggregate_facts.member_set")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_AllowsChangeImpactFilesAggregateMemberSet(t *testing.T) {
	mut := types.NewMutableState("Which production files need changes if CitationReq.Required changes?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "structured-target",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1915,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		Subject:         "CitationReq.Required",
		Producer:        EmitEvidenceProducer,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
			ChangeImpactProfile: &types.ChangeImpactProfile{
				IsChangeImpact:  true,
				Target:          "CitationReq.Required",
				RequestedOutput: types.ImpactOutputFiles,
				Scope:           types.ImpactScopeProduction,
			},
		}},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all affected files were inspected",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "affected production files",
			"members":      []string{"internal/agent/analyzer.go"},
			"support_refs": []string{"internal/agent/analyzer.go:1915"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "change-impact principal member_set handoff is missing") {
		t.Fatalf("valid aggregate member_set should satisfy the handoff: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should complete after citable member_set handoff")
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateMemberSet || facts[0].Value != "1" {
		t.Fatalf("member_set aggregate should be retained and canonicalized, got %+v", facts)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RejectsUncitedChangeImpactFileMembers(t *testing.T) {
	mut := types.NewMutableState("Which production files need changes if CitationReq.Required changes?")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
			ChangeImpactProfile: &types.ChangeImpactProfile{
				IsChangeImpact:  true,
				Target:          "CitationReq.Required",
				RequestedOutput: types.ImpactOutputFiles,
				Scope:           types.ImpactScopeProduction,
			},
		}},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all affected files were inspected",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "member_set",
			"label":   "affected production files",
			"members": []string{"internal/agent/analyzer.go"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "not usable as citable source-location principal data") {
		t.Fatalf("file members without support refs should be rejected, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open for uncited file members")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ReusesRetainedAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("list all enum types")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       653,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Intent",
			Subject:         "Intent",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       673,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Scenario",
			Subject:         "Scenario",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	accepted := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "enum type names",
		Value:       "2",
		Members:     []string{"Intent", "Scenario"},
		SupportRefs: []string{"Intent @ internal/types/analysis_ir.go:653", "Scenario @ internal/types/analysis_ir.go:673"},
	}}
	mut.SetInvestigationAggregateFacts(accepted)
	mut.SetInvestigationComplete("accepted member set")
	mut.RetainInvestigationAggregateFacts()
	mut.ResetInvestigationComplete()

	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"Intent", "Scenario"},
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "all enum types"},
			},
			AnswerContract: types.AnswerContract{
				MustIncludeTerms: []types.ContractTerm{
					{Text: "Intent", Kind: types.ContractTermSymbol},
					{Text: "Scenario", Kind: types.ContractTermSymbol},
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "closure-only reconcile; prior typed member_set remains authoritative",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "required principal members lack typed handoff") ||
		strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("retained member_set should satisfy later closure-only completion, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("closure-only completion should complete by reusing retained aggregate facts")
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || len(got[0].Members) != 2 || got[0].Members[0] != "Intent" {
		t.Fatalf("retained aggregate facts not preserved after closure-only completion: %+v", got)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_MemberSetSupportRefsCanUseDeterministicToolOutput(t *testing.T) {
	mut := types.NewMutableState("list all enum types")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: strings.Join([]string{
			"internal/types/analysis_ir.go:653:type Intent string",
			"internal/types/analysis_ir.go:673:type Scenario string",
		}, "\n"),
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"Intent", "Scenario"},
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "all enum types"},
			},
			AnswerContract: types.AnswerContract{
				MustIncludeTerms: []types.ContractTerm{
					{Text: "Intent", Kind: types.ContractTermSymbol},
					{Text: "Scenario", Kind: types.ContractTermSymbol},
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "grep output verified both enum type declarations",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "enum type names",
			"value":        "2",
			"members":      []string{"Intent", "Scenario"},
			"support_refs": []string{"Intent @ internal/types/analysis_ir.go:653", "Scenario @ internal/types/analysis_ir.go:673"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("deterministic grep support_refs should satisfy member_set support without per-member emit_evidence: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("deterministic tool-supported member_set should complete")
	}
}

func TestFieldValueCountCandidates_CoversCrossLanguageInitializerSurfaces(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "src/config.ets", `
export const cfg: CitationReq = {
  required: false,
}
`)
	writeTestFile(t, repoRoot, "native/options.cpp", `
struct CitationReq cfg = {
  .required = false,
};
`)
	writeTestFile(t, repoRoot, "cj/config.cj", `
let cfg = CitationReq(
  required: false
)
`)
	writeTestFile(t, repoRoot, "cj/compiled.cjo", `
let cfg = CitationReq(
  required: false
)
`)

	got := scanRepoForFieldValueCountCandidates(repoRoot, fieldValueCountTarget{
		Full:    "CitationReq.required",
		Owner:   "CitationReq",
		Field:   "required",
		Literal: "false",
	})
	var surfaces []string
	for _, c := range got {
		surfaces = append(surfaces, fmt.Sprintf("%s:%d", c.File, c.Line))
	}
	joined := strings.Join(surfaces, "\n")
	for _, want := range []string{
		"src/config.ets:2",
		"native/options.cpp:2",
		"cj/config.cj:2",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing cross-language candidate %s in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, ".cjo") {
		t.Fatalf("compiled Cangjie artifact must not be scanned as source: %s", joined)
	}
}

func TestFieldValueCountTargetFromContext_DoesNotTreatUnrelatedCountAsValueLiteral(t *testing.T) {
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RawRequest: "列出 Top 3 个 Foo.Bar 的调用点",
		Predicates: types.SemanticPredicates{
			IsScalarAnswer:  true,
			IsCountQuestion: true,
		},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"Foo.Bar"},
			Keywords: []string{"3"},
		},
	}}}
	if target, ok := fieldValueCountTargetFromContext(ctx); ok {
		t.Fatalf("unrelated count should not become a field/value target: %+v", target)
	}

	ctx.AnalysisIR.RequestModel.RawRequest = "Foo.timeout = 30 的生产代码位点有几处？"
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Foo.timeout"}
	target, ok := fieldValueCountTargetFromContext(ctx)
	if !ok {
		t.Fatal("code-adjacent numeric literal should become a field/value target")
	}
	if target.Full != "Foo.timeout" || target.Field != "timeout" || target.Literal != "30" {
		t.Fatalf("target = %+v, want Foo.timeout/timeout/30", target)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_CountRequiresDeterministicCommandOutput(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "internal/agent/analyzer.go", `
package agent

func build() {
	out.AnswerContract.CitationReq.Required = false
}
`)

	mut := types.NewMutableState("本仓库里，把 CitationReq.Required 设置为 false 的生产代码位点一共有几处？")
	mut.SetRepoRoot(repoRoot)
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/agent/analyzer.go": true})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/agent/analyzer.go",
		LineStart:       5,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: repoRoot,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "本仓库里，把 CitationReq.Required 设置为 false 的生产代码位点一共有几处？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"CitationReq.Required"},
					Keywords: []string{"false"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all candidate lines were read",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("expected deterministic-count downgrade, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open without command-backed count")
	}

	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "internal/agent/analyzer.go:5:\tout.AnswerContract.CitationReq.Required = false\n",
		Success:  true,
	})
	res, err = tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error after match listing: %v", err)
	}
	if !strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("grep match listing must not clear deterministic-count downgrade: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open after match listing output")
	}

	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "count=1\n",
		Success:  true,
	})
	res, err = tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error after count proof: %v", err)
	}
	if strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("deterministic count proof should clear downgrade: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should close after command-backed count")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RelationalCountAlsoNeedsStructuredProof(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "internal/orchestrator/orchestrator.go", `
package orchestrator

func build() {
	Contract{CitationReq: CitationReq{Required: false}}
}
`)

	mut := types.NewMutableState("CitationReq.Required=false 的位置有几处？")
	mut.SetRepoRoot(repoRoot)
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/orchestrator/orchestrator.go": true})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/orchestrator/orchestrator.go",
		LineStart:       5,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		Subject:         "CitationReq.Required",
		Object:          "false",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: repoRoot,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "CitationReq.Required=false 的位置有几处？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:     true,
					IsCountQuestion:    true,
					IsRelationalLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"CitationReq.Required"},
					Keywords: []string{"false"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "one candidate was classified",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("relational count should still require proof handoff, got: %s", res.Summary)
	}

	params, _ = json.Marshal(map[string]any{
		"reason":      "classified exact member set",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "total_count",
			"label":   "production assignment locations",
			"value":   "1",
			"unit":    "locations",
			"members": []string{"internal/orchestrator/orchestrator.go:5"},
		}},
	})
	res, err = tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error after aggregate handoff: %v", err)
	}
	if strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("structured aggregate handoff should clear deterministic-count downgrade: %s", res.Summary)
	}
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorBlocks:
// when AnalysisIR requires ≥1 citation but the evidence buffer has
// no cite-eligible items inside ReadSet, the tool downgrades.
func TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorBlocks(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	// ReadSet has one file, but evidence has nothing pointing there.
	closure.SetReadSet(map[string]bool{"internal/skill/defaults.go": true})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 1,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "thinks done",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("expected DOWNGRADED for citation floor miss, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete must remain false on citation floor failure")
	}
	vs := closure.ViolationsByKind(types.ViolPreCompleteDowngrade)
	if len(vs) != 1 {
		t.Fatalf("expected one pre-complete downgrade violation, got %d", len(vs))
	}
	if got, want := vs[0].ClusterKey, "root:CitationReq|stage:pre_complete"; got != want {
		t.Fatalf("ClusterKey=%q, want %q", got, want)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ExternalSourceLogWaivesCitationFloor(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "NoMethodError"}},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"cmd/root.go": true})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 1,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "external runtime log already explains the failure chain",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("external-source log should waive citation preflight, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set for external-source log closure")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorPasses_WithEligibleEvidence:
// when ReadSet covers the evidence Source, the floor is satisfied.
func TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorPasses(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/skill/defaults.go": true})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Source:    "internal/skill/defaults.go",
			LineStart: 14,
			LineEnd:   14,
			Kind:      types.EvidenceConcrete,
		},
	})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 1},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all evidence collected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("unexpected downgrade when eligible evidence present: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete should be set when contract preflight passes")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ExplanationFunctionSubject_NoShapeSwap(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/tool/repomap/tool.go": true})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "build-graph-def",
			Source:          "internal/tool/repomap/tool.go",
			LineStart:       133,
			LineEnd:         133,
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "buildOrLoadGraph",
			GroundingStatus: types.GroundingGrounded,
		},
	})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 1,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the mechanism",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("unexpected downgrade: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set when explanation preflight passes")
	}
	if got := closure.Stats().ViewSwapRaised; got != 0 {
		t.Fatalf("ViewSwapRaised=%d, want 0 for explanation anchored on a function", got)
	}
	for _, repair := range closure.PendingRepairs() {
		if repair.Origin == "pre_complete.subject_shape_mismatch" {
			t.Fatalf("unexpected shape-swap repair: %+v", repair)
		}
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ConfigFamilyFunctionSubject_RaisesViewSwap(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/config/runtime.go": true})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "runtime-def",
			Source:          "internal/config/runtime.go",
			LineStart:       32,
			LineEnd:         32,
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			GroundingStatus: types.GroundingGrounded,
		},
	})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentConfigQuery,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName, Confidence: 0.90},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 1,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "resolved the config lookup",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("unexpected downgrade: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set when preflight passes")
	}
	if got := closure.Stats().ViewSwapRaised; got != 1 {
		t.Fatalf("ViewSwapRaised=%d, want 1", got)
	}
	vs := closure.ViolationsByKind(types.ViolViewSwap)
	if len(vs) != 1 {
		t.Fatalf("expected one view-swap violation, got %d", len(vs))
	}
	if got, want := vs[0].ClusterKey, "subject:function_name|from:config_precedence|to:role_lookup|root:answer_subject"; got != want {
		t.Fatalf("ClusterKey=%q, want %q", got, want)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_MultiTopicExplanationAnchorsBlock(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/types/analysis_ir.go": true,
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Source:          "internal/types/analysis_ir.go",
			LineStart:       574,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Criterion",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Source:          "internal/types/analysis_ir.go",
			LineStart:       896,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Hypothesis",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				SubTopics: []types.SubTopic{
					{Summary: "Criterion 的角色", Entities: []string{"Criterion"}},
					{Summary: "Hypothesis 的角色", Entities: []string{"Hypothesis"}},
					{Summary: "AnalysisIR 如何持有 HypothesisSet", Entities: []string{"AnalysisIR.HypothesisSet", "HypothesisSet"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "enough evidence collected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("expected downgrade for incomplete multi-topic anchors, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "HypothesisSet") {
		t.Fatalf("expected missing sub-topic in message, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete must remain false on downgrade")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_WaiverBypassesMultiTopicAnchors(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"attached-log": true,
	})
	mut.AppendEvidence([]types.EvidenceItem{{
		Source:          "attached-log",
		LineStart:       1,
		AnchorKind:      types.AnchorTextReference,
		AnchorSymbol:    "RuntimeError",
		Kind:            types.EvidenceDirect,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
		Origin:          types.ClaimOriginLog,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				SubTopics: []types.SubTopic{
					{Summary: "top-level exception", Entities: []string{"RuntimeError"}},
					{Summary: "missing external service", Entities: []string{"ExternalService"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "attached runtime artifact is sufficient and external to this repo",
		"confidence":  "high",
		"result_kind": "resolved",
		"evidence_floor_waiver": map[string]any{
			"reason":    string(types.EvidenceFloorWaiverExternalLog),
			"rationale": "runtime artifact names services that are not defined in the current repository",
		},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "multi-topic explanation still lacks") {
		t.Fatalf("runtime waiver must bypass repo-anchor skeleton gate, got: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("unexpected downgrade after waiver: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set when runtime waiver bypasses repo anchors")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_SingleTopicExplanationSkipsAnchorSkeleton(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/types/analysis_ir.go": true,
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Source:          "internal/types/analysis_ir.go",
			LineStart:       574,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Criterion",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				SubTopics: []types.SubTopic{
					{Summary: "Criterion 的角色", Entities: []string{"Criterion"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "single-topic explanation is already grounded",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("single-topic explanation must not be blocked by anchor skeleton downgrade: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatal("InvestigationComplete should be set for single-topic explanation")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_MultiTopicExplanationAnchorsUseSharedSurfacePlan(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/types/analysis_ir.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		EvidenceItems: []types.EvidenceItem{
			{
				Source:          "internal/types/analysis_ir.go",
				LineStart:       574,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "Criterion",
				Kind:            types.EvidenceDirect,
				Summary:         "Criterion 定义 hypothesis criterion 的结构。",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
			{
				Source:          "internal/types/analysis_ir.go",
				LineStart:       896,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "Hypothesis",
				Kind:            types.EvidenceDirect,
				Summary:         "Hypothesis 表示单条假设。",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
			{
				Source:          "internal/types/analysis_ir.go",
				LineStart:       933,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "AnalysisIR.HypothesisSet",
				Kind:            types.EvidenceDirect,
				Summary:         "AnalysisIR.HypothesisSet 持有 analyzer 产出的假设集合。",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				SubTopics: []types.SubTopic{
					{Summary: "Criterion 的角色", Entities: []string{"Criterion"}},
					{Summary: "Hypothesis 的角色", Entities: []string{"Hypothesis"}},
					{Summary: "AnalysisIR 如何持有 HypothesisSet", Entities: []string{"AnalysisIR.HypothesisSet", "HypothesisSet"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "enough evidence collected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("shared answer surface plan should accept grounded bus evidence, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set when the shared plan already covers every explanation sub-topic")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadBlocks
// reproduces the 2026-04-18 "explorer calls subagent how" bug at the
// tool level. When the explorer's keyword-search top-K ranked files
// remain unread AND the declared RequirementKind is a breadth-intent,
// the gate queues PendingReads so the LLM's complete call is
// downgraded with the standard "Forced Read List" message.
func TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadBlocks(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 50},
		{Path: "internal/agent/subagent.go", Score: 40},
		{Path: "internal/tool/propose_sub_agents.go", Score: 29},
		{Path: "internal/orchestrator/orchestrator.go", Score: 24},
		{Path: "internal/agent/sub_explorer.go", Score: 23},
	})
	closure := mut.EvidenceClosure()
	// LLM only read 2 of the top-5 — the rest are unread.
	closure.SetReadSet(map[string]bool{
		"internal/agent/explorer.go": true,
		"internal/agent/subagent.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the answer",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("expected DOWNGRADED message when top-K are unread, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "propose_sub_agents.go") {
		t.Errorf("expected unread top-K file in forced-read list, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete must remain false on downgrade")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadSkipsKeywordOnlyAfterReadFocus(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 60},
		{Path: "internal/agent/sub_explorer.go", Score: 50},
		{Path: "internal/agent/explorer_erm.go", Score: 49},
		{Path: "internal/agent/agent.go", Score: 48},
		{Path: "internal/agent/answer_document_evaluator.go", Score: 47},
	})
	mut.SetSearchGraph(&repotypes.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"internal/agent/explorer.go": {
				RelPath: "internal/agent/explorer.go",
			},
			"internal/agent/explorer_erm.go": {
				RelPath: "internal/agent/explorer_erm.go",
			},
			"internal/agent/sub_explorer.go": {
				RelPath: "internal/agent/sub_explorer.go",
			},
			"internal/agent/agent.go": {
				RelPath: "internal/agent/agent.go",
			},
			"internal/agent/answer_document_evaluator.go": {
				RelPath: "internal/agent/answer_document_evaluator.go",
			},
		},
		ImportGraph: map[string][]string{
			"internal/agent/explorer.go": {"internal/agent/explorer_erm.go"},
		},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"internal/agent/explorer.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind:     "mechanism",
					Entities: []string{"ContinuationPrompt"},
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the mechanism",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("keyword-only siblings should not trip phase1_unread after graph focus, got: %s", res.Summary)
	}
	for _, pending := range closure.PendingReads() {
		if pending.Origin == "phase1_unread" {
			t.Fatalf("phase1_unread should not queue keyword-only siblings after focus: %+v", pending)
		}
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete should be set when only non-mandatory ranked files remain unread")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadHonorsCanonicalReadSet(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/tool/repomap/tool.go", Score: 42, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{".\\internal\\tool\\repomap\\tool.go": true})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the mechanism",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("canonical read-set match should prevent phase1_unread downgrade, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete should be set when the ranked file was already read")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_Phase1Unread_ConfigMappingMultiAnchorBlocks(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.Phase1UnreadTopK = 2
	limits.Phase1UnreadMinUnread = 1
	SetAnalysisLimits(limits)

	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "cmd/root.go", Score: 60, ExactEntityRank: 2},
		{Path: "internal/types/config.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"cmd/root.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "config_mapping"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the config flow",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("multi-anchor config_mapping should trigger phase1_unread, got: %s", res.Summary)
	}
	var found bool
	for _, pending := range closure.PendingReads() {
		if pending.Origin == "phase1_unread" && pending.File == "internal/types/config.go" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected phase1_unread PendingRead for config anchor, got %+v", closure.PendingReads())
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete must remain false when config anchor is unread")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_PrimaryAnchorUnreadBlocks(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/tool/repomap/tool.go", Score: 42, ExactEntityRank: 2},
		{Path: "internal/context/builder.go", Score: 41},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the mechanism",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("expected DOWNGRADED when primary anchor is unread, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/tool/repomap/tool.go") {
		t.Fatalf("expected primary anchor file in downgrade message, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete must remain false when primary anchor is unread")
	}
}

func TestPrimaryAnchorPendingRead_CapabilitySurfaceSkipsToolImplementationAnchor(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/analysis/dataflow/engine.go", Score: 60, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism",
					CapabilitySurface: &types.CapabilitySurfaceHint{
						Binding: types.StageBinding{
							Stage: types.StageAnalyze,
							Agent: types.AgentAnalyzer,
							Skill: "analysis-skill",
						},
						Tool: "read_file",
						AuthorityFiles: []string{
							"internal/orchestrator/topology.go",
							"internal/skill/analysis_contract.go",
							"internal/agent/agent.go",
							"internal/agent/analyzer.go",
						},
					},
				},
			},
		},
	}
	raisePrimaryAnchorPendingRead(bus, closure)
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.primary_anchor" {
			t.Fatalf("capability surface should skip tool implementation anchor forcing, got %+v", p)
		}
	}
}

func TestPhase1UnreadPendingReads_CapabilitySurfaceQueuesAuthorityFilesOnly(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.Phase1UnreadTopK = 4
	limits.Phase1UnreadMinUnread = 1
	SetAnalysisLimits(limits)

	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/analysis/dataflow/engine.go", Score: 70, ExactEntityRank: 2},
		{Path: "internal/orchestrator/topology.go", Score: 68, ExactEntityRank: 2},
		{Path: "internal/skill/analysis_contract.go", Score: 66, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"internal/orchestrator/topology.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism",
					CapabilitySurface: &types.CapabilitySurfaceHint{
						Binding: types.StageBinding{
							Stage: types.StageAnalyze,
							Agent: types.AgentAnalyzer,
							Skill: "analysis-skill",
						},
						Tool: "read_file",
						AuthorityFiles: []string{
							"internal/orchestrator/topology.go",
							"internal/skill/analysis_contract.go",
							"internal/agent/agent.go",
							"internal/agent/analyzer.go",
						},
					},
				},
			},
		},
	}
	raisePhase1UnreadPendingReads(bus, closure)
	got := closure.PendingReads()
	if len(got) == 0 {
		t.Fatal("expected unread capability authority files to be queued")
	}
	want := map[string]bool{
		"internal/skill/analysis_contract.go": true,
		"internal/agent/agent.go":             true,
		"internal/agent/analyzer.go":          true,
	}
	for _, p := range got {
		if p.File == "internal/analysis/dataflow/engine.go" {
			t.Fatalf("tool implementation file should not be queued for capability closure: %+v", got)
		}
		if !want[p.File] {
			t.Fatalf("unexpected pending capability file %q in %+v", p.File, got)
		}
		delete(want, p.File)
	}
	if len(want) != 0 {
		t.Fatalf("missing capability authority files from pending queue: %v", want)
	}
}

func TestMultiPathAnchorChecks_CapabilitySurfaceSkipsGate(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/agent.go", Score: 60, ExactEntityRank: 2},
		{Path: "internal/agent/analyzer.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"internal/agent/agent.go":    true,
		"internal/agent/analyzer.go": true,
	})
	closure.SetReadRanges(map[string][]types.LineRange{
		"internal/agent/agent.go":    {{Start: 1, End: 200}},
		"internal/agent/analyzer.go": {{Start: 1, End: 5}},
	})
	closure.SetFileTotalLines(map[string]int{
		"internal/agent/agent.go":    400,
		"internal/agent/analyzer.go": 1200,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism",
					CapabilitySurface: &types.CapabilitySurfaceHint{
						Binding: types.StageBinding{
							Stage: types.StageAnalyze,
							Agent: types.AgentAnalyzer,
							Skill: "analysis-skill",
						},
						Tool: "read_file",
						AuthorityFiles: []string{
							"internal/orchestrator/topology.go",
							"internal/skill/analysis_contract.go",
							"internal/agent/agent.go",
							"internal/agent/analyzer.go",
						},
					},
				},
			},
		},
	}
	applyMultiPathAnchorChecks(bus, closure)
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_anchor" {
			t.Fatalf("capability surface should not demand cross-file coverage parity, got %+v", p)
		}
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_PrimaryAnchorHonorsDispatchReadHistory(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/tool/repomap/tool.go", Score: 42, ExactEntityRank: 2},
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/tool/repomap/tool.go: showing lines 141-160 of 323 total]\n",
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the mechanism",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "internal/tool/repomap/tool.go") {
		t.Fatalf("dispatch read history should satisfy the primary anchor gate, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete should be set when the dispatch already read the anchor")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_Phase1Unread_Registration
// is the negative control: a non-breadth intent (registration) must
// NOT be blocked by the phase1-unread gate even when ranked files are
// unread. Single-lookup intents commonly need only 1-2 files.
func TestEmitInvestigationComplete_PreCompleteCheck_Phase1Unread_Registration(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "a.go", Score: 50},
		{Path: "b.go", Score: 40},
		{Path: "c.go", Score: 30},
		{Path: "d.go", Score: 20},
		{Path: "e.go", Score: 10},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"a.go": true})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "registration"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "found the registration",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, _ := tool.Execute(bus, params)
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("registration intent should not be blocked by phase1-unread gate: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("non-breadth intent should proceed when no other blockers")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_Phase1Unread_AbsenceBypass
// verifies absence_justification bypasses the phase1-unread gate —
// when the LLM honestly declares "no such thing in the repo" there is
// nothing to cite so forcing more reads is noise.
func TestEmitInvestigationComplete_PreCompleteCheck_Phase1Unread_AbsenceBypass(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "a.go", Score: 50},
		{Path: "b.go", Score: 40},
		{Path: "c.go", Score: 30},
		{Path: "d.go", Score: 20},
		{Path: "e.go", Score: 10},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":                "no such mechanism exists",
		"confidence":            "high",
		"result_kind":           "absence",
		"absence_justification": "grep produced zero hits for the claimed symbol",
	})
	res, _ := tool.Execute(bus, params)
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("absence answer must bypass phase1-unread gate: %s", res.Summary)
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadLatchFiresOnce:
// T2.1 — the phase1_unread gate must not keep re-firing on subsequent
// emit_investigation_complete calls within the same pipeline. Once
// the gate has surfaced the unread top-K files + raised the
// RepairExpandSearch directive, a second firing adds no information
// and only amplifies redispatches. The latch lives on EvidenceClosure,
// reset on task entry.
func TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadLatchFiresOnce(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "a.go", Score: 50},
		{Path: "b.go", Score: 40},
		{Path: "c.go", Score: 30},
		{Path: "d.go", Score: 20},
		{Path: "e.go", Score: 10},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"a.go": true}) // only 1 of 5 read
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "first attempt",
		"confidence":  "high",
		"result_kind": "resolved",
	})

	// First call: gate fires, files queued, latch flips.
	res1, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
	if !strings.Contains(res1.Summary, "DOWNGRADED") {
		t.Fatalf("first call must DOWNGRADE with top-K unread, got: %s", res1.Summary)
	}
	if !strings.Contains(res1.Summary, "b.go") || !strings.Contains(res1.Summary, "c.go") {
		t.Errorf("first call summary must list unread top-K files, got: %s", res1.Summary)
	}
	if !closure.Phase1UnreadFired() {
		t.Errorf("latch must be set after first firing")
	}

	// Clear any PendingReads that were queued so the SECOND pre-complete
	// check isn't blocked by leftover state from the first — we want to
	// isolate whether the gate itself fires a second time.
	for _, p := range closure.PendingReads() {
		if p.Origin == "phase1_unread" || p.Origin == "pre_complete.primary_anchor" {
			closure.ClearPendingReadFor(p.File)
		}
	}

	// Second call with same unread top-K setup: gate must NOT re-fire.
	// PendingReads from phase1_unread origin must remain empty.
	res2, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	phase1Queued := false
	for _, p := range closure.PendingReads() {
		if p.Origin == "phase1_unread" {
			phase1Queued = true
			break
		}
	}
	if phase1Queued {
		t.Errorf("latch must suppress second firing; found phase1_unread PendingRead: %s", res2.Summary)
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadLatchResetOnTaskEntry:
// T2.1 — the latch lives on EvidenceClosure and must be cleared when
// the closure is reset for a new task. Otherwise a second pipeline on
// the same REPL process would skip phase1_unread entirely.
func TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadLatchResetOnTaskEntry(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	closure.MarkPhase1UnreadFired()
	if !closure.Phase1UnreadFired() {
		t.Fatalf("precondition: latch should be set")
	}
	closure.Reset()
	if closure.Phase1UnreadFired() {
		t.Errorf("Reset must clear phase1UnreadFired latch")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_AbsenceWaivesCitationFloor:
// absence_justification skips check (b) by contract.
func TestEmitInvestigationComplete_PreCompleteCheck_AbsenceWaivesFloor(t *testing.T) {
	mut := types.NewMutableState("test")
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 1},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":                "the system has no such handler",
		"confidence":            "high",
		"result_kind":           "absence",
		"absence_justification": "no handler with that name exists in the repo",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("absence path should not downgrade on citation floor: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("absence path should still mark complete")
	}
}

// TestPrimaryAnchorPendingRead_ProjectOrientationSkipsGate proves
// that the "primary anchor unread" gate also short-circuits on
// project-orientation questions. Cross-gate consistency: all three
// pre-complete gates (primary_anchor / phase1_unread /
// multi_path_coverage) must agree on what "orientation question"
// means so an orientation answer cannot be partially blocked.
func TestPrimaryAnchorPendingRead_ProjectOrientationSkipsGate(t *testing.T) {
	mut := types.NewMutableState("test")
	// Phase1 ranking with an unread exact-anchor file. Pre-fix the
	// primary_anchor gate would queue this as a PendingRead. The
	// orientation skip must prevent that.
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "explorer.go", Score: 60, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Complexity: types.ComplexitySimple,
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism", // would otherwise trigger
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason": "project overview", "confidence": "high", "result_kind": "resolved",
	})
	if _, err := tool.Execute(bus, params); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.primary_anchor" {
			t.Errorf("orientation must skip primary_anchor gate; got %+v", p)
		}
	}
}

// TestPhase1UnreadPendingReads_ProjectOrientationSkipsGate proves
// the phase1_unread gate also honours the orientation short-circuit.
func TestPhase1UnreadPendingReads_ProjectOrientationSkipsGate(t *testing.T) {
	mut := types.NewMutableState("test")
	// Populate Phase1Ranking with > Phase1UnreadTopK entries, all
	// unread — pre-fix this would trigger the gate.
	ranks := make([]types.Phase1RankedFile, 0, 6)
	for i := 0; i < 6; i++ {
		ranks = append(ranks, types.Phase1RankedFile{
			Path:            fmt.Sprintf("file%d.go", i),
			Score:           float64(60 - i),
			ExactEntityRank: 1,
		})
	}
	mut.SetPhase1Ranking(ranks)
	closure := mut.EvidenceClosure()

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Complexity: types.ComplexitySimple,
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism",
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason": "project overview", "confidence": "high", "result_kind": "resolved",
	})
	if _, err := tool.Execute(bus, params); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, p := range closure.PendingReads() {
		if p.Origin == "phase1_unread" {
			t.Errorf("orientation must skip phase1_unread gate; got %+v", p)
		}
	}
}

// TestMultiPathAnchorChecks_ProjectOrientationSkipsGate locks the
// structural-signal skip: when the analyzer classifies the question
// as intent=explain + complexity=simple + no PrimaryEntities + no
// cross-component, the gate must not fire even when coverage looks
// unbalanced. A project-orientation question ("what does this repo
// do?") answers from README + manifest + entry-point and never
// needs cross-component depth parity.
//
// The detection is structured-signal-only — see
// IsProjectOrientationQuestion. No keyword matching on RawRequest;
// the analyzer LLM's own complexity/intent/predicates classification
// IS the signal.
//
// Cross-gate consistency: this test guards the orientation skip on
// the multi-path symbol-anchored gate (origin
// "pre_complete.multi_path_anchor"); a separate test in this file
// guards the same skip on primary_anchor + phase1_unread.
func TestMultiPathAnchorChecks_ProjectOrientationSkipsGate(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "README.md", Score: 60, ExactEntityRank: 2},
		{Path: "main.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"README.md": true, "main.go": true})
	closure.SetReadRanges(map[string][]types.LineRange{
		"README.md": {{Start: 1, End: 50}},
		"main.go":   {{Start: 1, End: 5}},
	})
	closure.SetFileTotalLines(map[string]int{"README.md": 50, "main.go": 1000})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Complexity: types.ComplexitySimple,
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism",
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason": "project overview", "confidence": "high", "result_kind": "resolved",
	})
	if _, err := tool.Execute(bus, params); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_anchor" {
			t.Errorf("project-orientation question must skip multi-path symbol-anchored gate; got %+v", p)
		}
	}
}

// TestApplyMultiPathAnchorChecks_SingleAnchorSkips guards against
// false-fire when only one primary anchor exists — single-subject
// questions have no parity target.
func TestApplyMultiPathAnchorChecks_SingleAnchorSkips(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 60, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/agent/explorer.go": true})
	closure.SetReadRanges(map[string][]types.LineRange{
		"internal/agent/explorer.go": {{Start: 1, End: 5}},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}
	applyMultiPathAnchorChecks(bus, closure)
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_anchor" {
			t.Fatalf("single anchor must not trigger multi-path symbol-anchored gate, got %+v", p)
		}
	}
}

// TestApplyMultiPathAnchorChecks_AdvisoryHintNonBlocking pins the
// load-bearing UX rule: when both Signal 1 and Signal 2 fail (no
// question-related symbol identifiable AND no grounded evidence
// emitted), the gate emits a NON-BLOCKING advisory directive only —
// it MUST NOT enqueue a PendingRead, otherwise emit_investigation_complete
// would still be blocked and we are back to the pre-2026-05-01
// pathology of stalling completion on signals we cannot defend.
func TestApplyMultiPathAnchorChecks_AdvisoryHintNonBlocking(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "anchor_a.go", Score: 60, ExactEntityRank: 2},
		{Path: "anchor_b.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	// No reads, no evidence — both signals fail; no symbol oracle
	// (Mutable.SearchGraph returns nil) so Signal 1 cannot identify
	// any symbol either. Engine should pick SignalOpaqueAdvisory.
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "mechanism",
					PrimaryEntities: []string{"someEntity"},
				},
			},
		},
	}
	applyMultiPathAnchorChecks(bus, closure)
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_anchor" {
			t.Fatalf("opaque-advisory case MUST NOT enqueue PendingRead (would block completion), got %+v", p)
		}
	}
	// At least one RepairExpandSearch advisory directive must have
	// been emitted so the LLM is informed without being blocked.
	advisoryCount := 0
	for _, r := range closure.ActiveRepairs() {
		if r.Origin == "pre_complete.multi_path_anchor" && r.Kind == types.RepairExpandSearch {
			advisoryCount++
		}
	}
	if advisoryCount == 0 {
		t.Fatalf("opaque-advisory case must emit at least one RepairExpandSearch advisory; got %+v", closure.ActiveRepairs())
	}
}
