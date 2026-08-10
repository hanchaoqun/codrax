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
