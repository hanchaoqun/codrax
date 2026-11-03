package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC338PreparedNativeSourceRecordReachesActualAgentRequests(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is required for the original full SQLite fixture")
	}
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, RuntimeAnchor: t.TempDir(), PreviewBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultPerfTriageSettings()
	settings.TwoStepEnabled = false
	lanes := []struct {
		name  types.AgentName
		stage types.PipelineStage
		skill string
		make  func(*Dependencies) Agent
	}{
		{types.AgentPerfTriager, types.StagePerfTriage, "perf-triage-skill", func(d *Dependencies) Agent { return NewPerfTriagerAgent(d, settings) }},
		{types.AgentExplorer, types.StageExplore, "explore-skill", NewExplorerAgent},
	}
	for _, lane := range lanes {
		t.Run(string(lane.name), func(t *testing.T) {
			const question = "请统计附加采集中的内存申请和释放。"
			bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh",
				AttachedTraceMaterial: material, AttachedHitrace: material.Preview(), AttachedHitraceSource: path,
				Mutable: types.NewMutableState(question)}
			ctx := ctxbuilder.BuildAgentContext(bus, lane.name, lane.stage)
			registry := tool.NewRegistry()
			registry.Register(&tool.TraceQuery{})
			registry.Register(&tool.EmitPerfTrace{})
			capture := &traceTeachingCaptureLLM{stop: errors.New("captured native preview request, no model execution")}
			agent := lane.make(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
			_, err := agent.Execute(ctx, traceTeachingSkill(t, lane.skill))
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("did not capture exactly one actual adapter request: calls=%d err=%v", capture.calls, err)
			}
			found := 0
			for _, message := range capture.messages {
				if message.Role != "user" {
					continue
				}
				_, text, ok := strings.Cut(message.Content, "Decoded fields for visible Trace rows")
				if !ok {
					continue
				}
				_, data, ok := strings.Cut(text, "```jsonl\n")
				if !ok {
					t.Fatal("native preview missing JSONL")
				}
				data, _, ok = strings.Cut(data, "\n```")
				if !ok {
					t.Fatal("native preview JSONL not closed")
				}
				for _, line := range strings.Split(data, "\n") {
					var row struct {
						NS      string                         `json:"timestamp_ns"`
						Seconds string                         `json:"timestamp_seconds"`
						Known   *bool                          `json:"source_time_known"`
						Role    string                         `json:"record_time_role"`
						Record  *tracewire.ResourceStackRecord `json:"source_record"`
					}
					if err := json.Unmarshal([]byte(line), &row); err != nil {
						t.Fatal(err)
					}
					if row.Record == nil || row.Record.Event == nil {
						continue
					}
					found++
					if row.NS != "10005000000" || row.Seconds != "10.005000000" || row.Known == nil || !*row.Known || row.Role != "resource_event_time" ||
						row.Record.TimestampNS != 10005000000 || row.Record.EventRowID != 1 || row.Record.Event.FrameCount != 3 || row.Record.Event.ITID != 1 ||
						row.Record.Event.EndNS.Value != "10025000000" || row.Record.Event.Size.Value != "512" || row.Record.Event.Thread != "gallery-main" {
						t.Fatalf("old opaque-decoding error reached the actual request: %+v", row)
					}
				}
				if !strings.Contains(text, "not execution") || !strings.Contains(message.Content, "preview-local") || !strings.Contains(message.Content, "not physical evidence coordinates") {
					t.Fatal("readable facts lost their source/time/preview authority limits")
				}
			}
			if found != 1 || ctx.Mutable.PerfTrace() != nil {
				t.Fatalf("display omitted/duplicated a visible event or installed extracted facts: visible_events=%d", found)
			}
		})
	}
	current, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(current) != sha256.Sum256(original) || material.Validate(context.Background(), material.Preview()) != nil {
		t.Fatal("read-only context projection changed the complete source or its prepared generation")
	}
}
