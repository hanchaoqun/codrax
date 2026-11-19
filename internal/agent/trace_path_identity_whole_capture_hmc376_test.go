package agent

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc376CloneCapture copies the bytes. A hard link would change the original's
// metadata, which every other reader of that capture checks while it parses.
func hmc376CloneCapture(t *testing.T, source, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}

// One complete capture read from three directories of equal name length, through
// the query sequence that publishes one account from several lookups. Everything
// the finalizer is told reads the same once the locators themselves are masked:
// which rows survive each bound, their order, and every list of source records.
func TestHMC376WholeOriginalFinalizerInstructionIsIndependentOfCaptureDirectory(t *testing.T) {
	source := os.Getenv("CODRAX_HMC376_NETEASE_REBOOT_FRAME")
	if source == "" {
		t.Skip("requires CODRAX_HMC376_NETEASE_REBOOT_FRAME (the pinned complete capture)")
	}
	locator := regexp.MustCompile(`strong:[0-9a-z<>:]+|trace-query-result-[0-9a-f]*|[0-9a-f]{8,}`)
	clock := regexp.MustCompile(`20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9:.]+(Z|[-+][0-9:]+)`)
	start, end := 153.763595, 153.887340
	root := t.TempDir()
	repo, work := filepath.Join(root, "r"), filepath.Join(root, "w")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	var first, firstName string
	for _, name := range []string{"a1", "b2", "c3"} {
		dir := filepath.Join(root, "c", name)
		path := filepath.Join(dir, "capture.ftrace")
		hmc376CloneCapture(t, source, path)
		if err := os.RemoveAll(work); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(work, 0o700); err != nil {
			t.Fatal(err)
		}
		request := types.RequestModel{Language: "zh", Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck, PerfTrace: &types.PerfBundle{},
			RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "tid 11077"},
			RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 11077, Source: "user_explicit"}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "153.763595..153.887340"},
		}
		ctx := &types.AgentContext{RepoRoot: repo, WorkDir: work, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
			Mutable: types.NewMutableState("where did the main thread 11077 spend this frame"), AnalysisIR: &types.AnalysisIR{RequestModel: request}}
		ctx.Mutable.SetRequestModel(request)
		for _, minDuration := range []float64{0.5, 5} {
			for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
				params, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 11077, "time_start": start, "time_end": end,
					"max_depth": 4, "max_branches": 8, "max_chain_nodes": 1, "min_duration_ms": minDuration, "limit": 12, "trace_flavor": "harmony_hitrace"})
				if err != nil {
					t.Fatal(err)
				}
				result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
				if err != nil || !result.Success {
					t.Fatalf("%s %s: %v %s", name, view, err, result.Summary)
				}
				ctx.Mutable.AppendDispatchToolResult(result)
			}
		}
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
		system, user := finalizerReaderFieldMessages(t, ctx)
		text := strings.ReplaceAll(strings.ReplaceAll(system+"\n"+user, dir, "<dir>"), work, "<work>")
		text = clock.ReplaceAllString(locator.ReplaceAllString(text, "<digest>"), "<time>")
		if !strings.Contains(text, "source_records=") {
			t.Fatal("control: the instruction must list source records")
		}
		if first == "" {
			first, firstName = text, name
			continue
		}
		if text != first {
			t.Fatalf("the instruction differs between %s and %s: first difference%s", firstName, name, hmc376FirstDifferentLine(first, text))
		}
	}
}
