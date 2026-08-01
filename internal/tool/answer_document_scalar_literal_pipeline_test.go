package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEmitAnswerDocumentV2_ScalarLiteralCitationSurvivesLateCarrierAndPoolPasses
// pins the complete full-emit composition seen in B22-F r1. The scalar repair
// is not complete merely because its helper changes citation_ref: the exact
// value citation must remain reachable after deterministic enumeration carrier
// materialization, unused-pool pruning, and the shared persist chokepoint.
func TestEmitAnswerDocumentV2_ScalarLiteralCitationSurvivesLateCarrierAndPoolPasses(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "cmd"), 0o755); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "internal", "config"), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "cmd", "root.go"), []byte(
		"package cmd\n\nconst defaultMaxSteps = 50\n\nfunc apply() {}\n"), 0o644); err != nil {
		t.Fatalf("write root fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "internal", "config", "runtime.go"), []byte(
		"package config\n\ntype RuntimeSettings struct {\n\tPipelineMaxStepsCeil *int\n\tPipelineMaxSteps *int\n}\n"), 0o644); err != nil {
		t.Fatalf("write runtime fixture: %v", err)
	}

	mu := types.NewMutableState("config scalar pipeline")
	mu.SetRepoRoot(repo)
	mu.AppendEvidence([]types.EvidenceItem{
		{
			ID: "value", Kind: types.EvidenceDirect, Source: "cmd/root.go", LineStart: 3,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "defaultMaxSteps",
			Snippet: "const defaultMaxSteps = 50", GroundingStatus: types.GroundingGrounded,
		},
		{
			ID: "sibling", Kind: types.EvidenceDirect, Source: "internal/config/runtime.go", LineStart: 4,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "PipelineMaxStepsCeil",
			Snippet: "PipelineMaxStepsCeil *int", GroundingStatus: types.GroundingGrounded,
		},
		{
			ID: "target", Kind: types.EvidenceDirect, Source: "internal/config/runtime.go", LineStart: 5,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "PipelineMaxSteps",
			Snippet: "PipelineMaxSteps *int", GroundingStatus: types.GroundingGrounded,
		},
		{
			ID: "example", Kind: types.EvidenceDirect, Source: "codrax.yaml.example", LineStart: 1,
			AnchorKind: types.AnchorTextReference, DiagramRole: types.EvidenceDiagramRoleConfig,
			Snippet: "# pipeline_max_steps default 50", GroundingStatus: types.GroundingGrounded,
		},
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "pipeline_max_steps 解析链路关键节点",
		Value: "3",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"RuntimeSettings.PipelineMaxSteps",
			"--pipeline-max-steps CLI 标志注册",
			"orch.SetMaxSteps 传递",
		},
		SupportRefs: []string{
			"internal/config/runtime.go:5",
			"cmd/root.go:5",
			"cmd/root.go:5",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	bus := &types.BusContext{
		Mutable:  mu,
		RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentConfigQuery,
			Scenario: types.ScenarioConfigTrace,
			Language: "zh",
			AnalyzerHints: types.AnalyzerHints{
				Kind: string(types.ReqConfigMapping),
			},
		}},
	}
	raw := json.RawMessage(`{
		"blocks": [
			{"id":"summary","kind":"summary","surface_role":"principal","text":"pipeline_max_steps 解析链路"},
			{"id":"default","kind":"scalar","surface_role":"principal","text":"50","facet_ids":["resolved_literal_or_symbol"],"claim_uses":[{"claim_form":"literal_value_fact"}],"items":[{"id":"v","citation_ref":1}]},
			{"id":"precedence","kind":"table","surface_role":"principal","text":"| 层级 | 值 |\n|---|---|\n| code default | 50 |\n| yaml | configured |\n| CLI | override |"}
		],
		"citations": [
			{"file":"cmd/root.go","line":3,"quote":"const defaultMaxSteps = 50"},
			{"file":"internal/config/runtime.go","line":4,"quote":"PipelineMaxStepsCeil *int"},
			{"file":"internal/config/runtime.go","line":5,"quote":"PipelineMaxSteps *int"},
			{"file":"cmd/root.go","line":5,"quote":"func apply() {}"},
			{"file":"codrax.yaml.example","line":1,"quote":"# pipeline_max_steps default 50"}
		]
	}`)

	res, err := (&EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil {
		t.Fatalf("emit transport error: %v", err)
	}
	if !res.Success {
		t.Fatalf("emit rejected: %s", res.Summary)
	}
	doc := mu.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("answer document not persisted")
	}
	var scalar *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].ID == "default" {
			scalar = &doc.Blocks[i]
			break
		}
	}
	if scalar == nil || len(scalar.Items) != 1 {
		t.Fatalf("scalar citation carrier lost: %+v", scalar)
	}
	ref := scalar.Items[0].CitationRef
	if ref < 0 || ref >= len(doc.Citations) {
		t.Fatalf("scalar citation ref lost after full pipeline: ref=%d citations=%+v", ref, doc.Citations)
	}
	cit := doc.Citations[ref]
	if cit.File != "cmd/root.go" || cit.Line != 3 {
		t.Fatalf("scalar citation=%+v, want exact value-bearing cmd/root.go:3", cit)
	}
}
