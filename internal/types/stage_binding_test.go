package types

import (
	"strings"
	"testing"
)

func TestReadModeStageBindingsFollowCanonicalStageOrder(t *testing.T) {
	main := ReadModeMainStageBindings()
	mainStages := AllMainStages()
	if len(main) != len(mainStages) {
		t.Fatalf("main binding count = %d, want %d", len(main), len(mainStages))
	}
	for i, stage := range mainStages {
		if main[i].Stage != stage {
			t.Fatalf("main binding %d stage = %q, want %q", i, main[i].Stage, stage)
		}
	}

	pre := ReadModeConditionalPreStageBindings()
	wantPre := []PipelineStage{StageLogTriage, StagePerfTriage}
	if len(pre) != len(wantPre) {
		t.Fatalf("pre binding count = %d, want %d", len(pre), len(wantPre))
	}
	for i, stage := range wantPre {
		if pre[i].Stage != stage {
			t.Fatalf("pre binding %d stage = %q, want %q", i, pre[i].Stage, stage)
		}
	}
}

func TestReadModePipelineAuthorityFiles(t *testing.T) {
	got := ReadModePipelineAuthorityFiles()
	want := []string{
		ReadModePipelineEnumsFile,
		ReadModePipelineStageBindingFile,
		ReadModePipelineTopologyFile,
	}
	if len(got) != len(want) {
		t.Fatalf("authority file count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("authority file %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadModeStageBindingsCarryResponsibilitiesAndArtifacts(t *testing.T) {
	for _, binding := range ReadModeMainStageBindings() {
		if strings.TrimSpace(binding.Responsibility) == "" {
			t.Fatalf("binding %s has empty responsibility", binding.Stage)
		}
		if len(binding.PrimaryArtifacts) == 0 {
			t.Fatalf("binding %s has no primary artifacts", binding.Stage)
		}
	}
	analyze, ok := StageBindingForStage(StageAnalyze)
	if !ok {
		t.Fatal("StageAnalyze binding missing")
	}
	for _, want := range []string{"Classify", "AnalysisIR"} {
		if !strings.Contains(analyze.Responsibility, want) {
			t.Fatalf("StageAnalyze responsibility missing %q: %q", want, analyze.Responsibility)
		}
	}
	for _, want := range []string{"AnalysisIR", "TaskGraph", "EvidencePlan", "AnswerContract"} {
		if !stageBindingHasArtifact(analyze, want) {
			t.Fatalf("StageAnalyze artifacts missing %q: %v", want, analyze.PrimaryArtifacts)
		}
	}
}

func stageBindingHasArtifact(binding StageBinding, want string) bool {
	for _, got := range binding.PrimaryArtifacts {
		if got == want {
			return true
		}
	}
	return false
}
