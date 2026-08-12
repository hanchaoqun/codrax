package stageauthority

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLoadReadModeReturnsExactAdjacentAuthority(t *testing.T) {
	repo := writeReadModeAuthorityFixture(t)
	authority, ok := LoadReadMode(repo)
	if !ok {
		t.Fatal("expected verified read-mode authority")
	}
	if len(authority.Main) != 4 || len(authority.Precedence) != 3 {
		t.Fatalf("unexpected authority cardinality: main=%d precedence=%d", len(authority.Main), len(authority.Precedence))
	}
	want := []string{
		"StageAnalyze->StageExplore",
		"StageExplore->StageExtract",
		"StageExtract->StageFinalize",
	}
	for i, relation := range authority.Precedence {
		got := relation.From.StageIdent + "->" + relation.To.StageIdent
		if got != want[i] || relation.SourceFile != types.ReadModePipelineEnumsFile ||
			relation.LineStart <= 0 || relation.LineEnd < relation.LineStart {
			t.Fatalf("precedence[%d]=%+v, want %s with exact source", i, relation, want[i])
		}
	}
}

func TestLoadReadModeFailsClosedOnSequenceOrBindingDrift(t *testing.T) {
	tests := []struct {
		name string
		file string
		old  string
		new  string
	}{
		{name: "sequence", file: types.ReadModePipelineEnumsFile, old: "StageAnalyze, StageExplore", new: "StageExplore, StageAnalyze"},
		{name: "binding semantics", file: types.ReadModePipelineStageBindingFile, old: strconv.Quote(types.ReadModeMainStageBindings()[0].Responsibility), new: strconv.Quote("different responsibility")},
		{name: "conditional membership", file: types.ReadModePipelineStageBindingFile, old: "StageLogTriage, StagePerfTriage", new: "StagePerfTriage, StageLogTriage"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := writeReadModeAuthorityFixture(t)
			path := filepath.Join(repo, filepath.FromSlash(tc.file))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(strings.Replace(string(data), tc.old, tc.new, 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, ok := LoadReadMode(repo); ok {
				t.Fatal("drifted checkout must not produce authority")
			}
		})
	}
}

func TestLoadReadModeStateCarriersVerifiesExactFieldOwnership(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, filepath.FromSlash(types.ReadModePipelineContextFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package types
type MutableState struct { answerDocumentV2 *AnswerDocumentV2 }
type BusContext struct {
 Mutable *MutableState
 PipelineStage PipelineStage
 ActiveAgent AgentName
 EvidenceItems []EvidenceItem
 AnswerChains []AnswerChain
 AnswerSymbols []AnswerSymbol
 StageReports []StageReport
 AnalysisIR *AnalysisIR
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, ok := LoadReadModeStateCarriers(repo)
	if !ok || len(rows) != len(readModeStateCarrierFieldSpecs) {
		t.Fatalf("state carriers=%+v ok=%v", rows, ok)
	}
	if rows[0].Owner != "BusContext" || rows[0].Field != "Mutable" || rows[0].Type != "*MutableState" || rows[0].File != types.ReadModePipelineContextFile || rows[0].Line <= 0 {
		t.Fatalf("unexpected first carrier row: %+v", rows[0])
	}
	if rows[len(rows)-1].Owner != "MutableState" || rows[len(rows)-1].Field != "answerDocumentV2" {
		t.Fatalf("structured answer carrier missing: %+v", rows)
	}

	drifted := strings.Replace(source, "EvidenceItems []EvidenceItem", "EvidenceItems []string", 1)
	if err := os.WriteFile(path, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	if rows, ok := LoadReadModeStateCarriers(repo); ok || rows != nil {
		t.Fatalf("drifted field type must fail closed: %+v", rows)
	}
}

func TestMatchesRequiredMainStageParticipantSlate(t *testing.T) {
	authority, ok := LoadReadMode(writeReadModeAuthorityFixture(t))
	if !ok {
		t.Fatal("expected checkout-verified read-mode authority")
	}
	rm := types.RequestModel{
		Intent:        types.IntentExplain,
		PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantContextOnly},
		}},
	}
	if !MatchesRequiredMainStageParticipantSlate(rm, authority.Main) {
		t.Fatal("complete typed main-stage slate should activate verified precedence relevance")
	}
	for _, participant := range rm.DiagramHint.Participants[:4] {
		if !ParticipantHasIncidentPrecedence(rm, participant, authority.Precedence) {
			t.Fatalf("stage participant %q should be incident to verified precedence", participant.Identity)
		}
	}
	if ParticipantHasIncidentPrecedence(rm, rm.DiagramHint.Participants[4], authority.Precedence) {
		t.Fatal("context-only carrier must not acquire stage relation authority")
	}

	rm.DiagramHint.Participants[3].Role = types.DiagramParticipantContextOnly
	if MatchesRequiredMainStageParticipantSlate(rm, authority.Main) {
		t.Fatal("partial incident slate must fail closed")
	}
	rm.DiagramHint.Participants[3].Role = types.DiagramParticipantIncidentRequired
	rm.Intent = types.IntentTrace
	if MatchesRequiredMainStageParticipantSlate(rm, authority.Main) {
		t.Fatal("Trace must not activate current-source stage authority")
	}
}

func TestRelevantToRequiredReadModeWorkflowUsesTypedDimensionAndGroundedAuthoritySource(t *testing.T) {
	authority, ok := LoadReadMode(writeReadModeAuthorityFixture(t))
	if !ok {
		t.Fatal("expected checkout-verified read-mode authority")
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true,
			Participants: []types.DiagramParticipantHint{{Identity: "Orchestrator.Run", Role: types.DiagramParticipantIncidentRequired}}},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []types.RequestedAnswerDimension{{
				Index: 1, Label: "stage", SourceQuote: "stage", Required: true,
				Role: types.RequestedAnswerDimensionStageWorkflow,
			}},
		},
	}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: types.ReadModePipelineOrchestratorFile, LineStart: 100,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "runReadSchedulerLoop",
		GroundingStatus: types.GroundingGrounded,
	}}
	if !RelevantToRequiredReadModeWorkflow(rm, evidence, authority.Main) {
		t.Fatal("typed required workflow plus grounded current-pipeline source must activate verified stage precedence")
	}
	if RelevantToRequiredReadModeWorkflow(rm, nil, authority.Main) {
		t.Fatal("typed dimension without grounded authority source must fail closed")
	}
	rm.Intent = types.IntentTrace
	if RelevantToRequiredReadModeWorkflow(rm, evidence, authority.Main) {
		t.Fatal("Trace must remain outside current-source stage authority")
	}
}

func TestSelectRequiredReadModeWorkflowUsesTypedEndpointSpanWithoutProse(t *testing.T) {
	authority, ok := LoadReadMode(writeReadModeAuthorityFixture(t))
	if !ok {
		t.Fatal("expected checkout-verified read-mode authority")
	}
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: types.ReadModePipelineOrchestratorFile, LineStart: 100,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "runReadSchedulerLoop",
		GroundingStatus: types.GroundingGrounded,
	}}
	rm := types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true,
			Participants: []types.DiagramParticipantHint{
				{Identity: "codrax", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "analyze", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "Mermaid", Role: types.DiagramParticipantIncidentRequired},
			}},
	}
	selection := SelectRequiredReadModeWorkflow(rm, evidence, authority)
	if len(selection.Main) != 4 || len(selection.Precedence) != 3 {
		t.Fatalf("typed analyze/finalizer endpoints should select the contiguous main span: %+v", selection)
	}
	if got := selection.Precedence[0].From.StageIdent + "->" + selection.Precedence[2].To.StageIdent; got != "StageAnalyze->StageFinalize" {
		t.Fatalf("selected wrong endpoint span: %s", got)
	}

	rm.DiagramHint.Participants = []types.DiagramParticipantHint{
		{Identity: "explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired},
	}
	selection = SelectRequiredReadModeWorkflow(rm, evidence, authority)
	if len(selection.Main) != 3 || len(selection.Precedence) != 2 || selection.Main[0].StageIdent != "StageExplore" {
		t.Fatalf("partial typed endpoints should select only their contiguous span: %+v", selection)
	}

	if got := SelectRequiredReadModeWorkflow(rm, nil, authority); len(got.Precedence) != 0 {
		t.Fatalf("endpoint span without grounded current-pipeline evidence must fail closed: %+v", got)
	}
	rm.DiagramHint.Participants = rm.DiagramHint.Participants[:1]
	if got := SelectRequiredReadModeWorkflow(rm, evidence, authority); len(got.Precedence) != 0 {
		t.Fatalf("one endpoint cannot mint a stage path: %+v", got)
	}
	rm.Intent = types.IntentTrace
	rm.DiagramHint.Participants = []types.DiagramParticipantHint{
		{Identity: "analyze", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired},
	}
	if got := SelectRequiredReadModeWorkflow(rm, evidence, authority); len(got.Precedence) != 0 {
		t.Fatalf("Trace must remain isolated from current-source stage selection: %+v", got)
	}
}

func writeReadModeAuthorityFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	dir := filepath.Join(repo, "internal", "types")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bindingSource := "package types\n\ntype StageBinding struct{}\n\nvar builtinStageBindings = []StageBinding{"
	stageIdents := make([]string, 0, 4)
	for _, binding := range types.ReadModeMainStageBindings() {
		stageIdent, agentIdent, ok := BindingIdentifiers(binding)
		if !ok {
			t.Fatalf("unexpected binding: %+v", binding)
		}
		stageIdents = append(stageIdents, stageIdent)
		artifacts := make([]string, 0, len(binding.PrimaryArtifacts))
		for _, artifact := range binding.PrimaryArtifacts {
			artifacts = append(artifacts, strconv.Quote(artifact))
		}
		bindingSource += fmt.Sprintf("\n\t{Stage: %s, Agent: %s, Skill: %q, Terminal: %t, Responsibility: %q, PrimaryArtifacts: []string{%s}},",
			stageIdent, agentIdent, binding.Skill, binding.Terminal, binding.Responsibility, strings.Join(artifacts, ", "))
	}
	bindingSource += "\n}\n\nfunc ReadModeConditionalPreStageBindings() []StageBinding {\n\tstages := []PipelineStage{StageLogTriage, StagePerfTriage}\n\t_ = stages\n\treturn nil\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "stage_binding.go"), []byte(bindingSource), 0o644); err != nil {
		t.Fatal(err)
	}
	enumsSource := "package types\n\ntype PipelineStage string\n\nfunc AllMainStages() []PipelineStage {\n\treturn []PipelineStage{" + strings.Join(stageIdents, ", ") + "}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "enums.go"), []byte(enumsSource), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}
