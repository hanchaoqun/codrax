package types

import "testing"

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
