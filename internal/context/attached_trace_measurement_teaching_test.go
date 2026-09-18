package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAttachedTraceObservationsDoNotImplyPreviewMeasurements(t *testing.T) {
	for _, tc := range []struct {
		name  string
		agent types.AgentName
		stage types.PipelineStage
		tools []string
	}{
		{"analyzer", types.AgentAnalyzer, types.StageAnalyze, []string{"emit_analysis"}},
		{"triager", types.AgentPerfTriager, types.StagePerfTriage, []string{"read_file", "emit_perf_bundle"}},
		{"explorer", types.AgentExplorer, types.StageExplore, []string{"trace_query", "read_file"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range []int{1, 100} {
				ctx := &types.AgentContext{AgentName: tc.agent, Stage: tc.stage, Objective: "Explain the attached capture", WorkDir: t.TempDir(),
					AttachedHitrace: strings.Repeat("app-42 (42) [000] .... 1.000000: sched_switch: prev_pid=42 prev_state=S ==> next_pid=0\n", size)}
				pc := BuildPromptContext(ctx, &skill.Config{Name: tc.name, ToolSuggestions: tc.tools})
				var attachment string
				for _, section := range pc.UserSections {
					if section.Title == SectionAttachedPerfTrace {
						attachment += section.Content
					}
				}
				for _, want := range []string{"literal event fields", "bounded preview", "deterministic queries", "not as a repository source citation"} {
					if !strings.Contains(attachment, want) {
						t.Errorf("size %d: missing observation boundary %q", size, want)
					}
				}
				for _, retired := range []string{"derive hotspots, stalls", "capturing hotspots, stalls, frame spans"} {
					if strings.Contains(attachment, retired) {
						t.Errorf("retired measurement instruction: %q", retired)
					}
				}
				if tc.name == "explorer" && !strings.Contains(attachment, "Prefer `trace_query` for scheduler state, wakeup chains") {
					t.Error("lost capable-stage trace navigation")
				}
				if tc.name == "triager" && !strings.Contains(attachment, "Prepare a structured summary") {
					t.Error("lost pre-triage task")
				}
			}
		})
	}
}
