package agent

import (
	"errors"
	"strings"
	"testing"

	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAttachedTraceActualAnalyzerMeasurementTeaching(t *testing.T) {
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Objective: "Summarize waits in the attached trace only", WorkDir: t.TempDir(),
		Mutable:         types.NewMutableState("attached capture"),
		AttachedHitrace: strings.Repeat("app-42 (42) [000] .... 1.000000: sched_switch: prev_pid=42 prev_state=S ==> next_pid=0\n", 100)}
	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.EmitAnalysis{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured analyzer trace teaching")}
	agent := NewAnalyzerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
	_, err := agent.Execute(ctx, traceTeachingSkill(t, "analysis-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("initial capture: calls=%d err=%v", capture.calls, err)
	}
	if len(capture.tools) != 1 || capture.tools[0].Name != "emit_analysis" {
		t.Fatalf("fixture did not capture classify-only tool surface: %+v", capture.tools)
	}
	var prompt string
	for _, message := range capture.messages {
		prompt += message.Content + "\n"
	}
	for _, want := range []string{"literal event fields", "deterministic queries", "bounded preview"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual request lacks %q", want)
		}
	}
	if strings.Contains(prompt, "derive hotspots, stalls") {
		t.Error("actual analyzer request demands unavailable measurements")
	}
}
