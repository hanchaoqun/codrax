package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativeResourceStackRequestWindowDefaultReachesActualFinalizer(t *testing.T) {
	dir := t.TempDir()
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	prep := traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})
	m, err := prep.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	start, end := 10.0, 10.05
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		AttachedHitrace: m.Preview(), AttachedTraceMaterial: m, TraceInputPreparer: prep, Mutable: types.NewMutableState("10.000到10.050秒的资源调用栈"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh",
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10.000到10.050秒"},
			RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit", Confidence: 1}}}}}
	// Same model call as the failed natural eval: no time or target arguments.
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), []byte(`{"source":"attached_trace","view":"resource_stack"}`))
	if err != nil || !r.Success {
		t.Fatalf("public native request query: %v %s", err, r.Summary)
	}
	found := false
	for _, record := range r.Observations {
		p, ok := tool.DecodeTraceResourceStack(record)
		if !ok {
			continue
		}
		frames := 0
		for _, event := range p.Events {
			frames += event.Source.FrameCount
		}
		if p.MatchedEvents != 3 || frames != 8 || p.Window.StartTs != start || p.Window.EndTs != end || p.Window.EndInclusive {
			t.Fatalf("request boundary/owner lost: %+v", p)
		}
		found = true
	}
	if !found || len(ctx.Mutable.TraceQueryCallWindows()) != 0 {
		t.Fatal("typed observation missing or inherited window misregistered")
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"### 已观测资源事件与调用栈", "ImageCache::reserve", "UIFrame::render", "free"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("request-window stack omitted from actual finalizer: %q", want)
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("presentation changed evidence")
	}
}
